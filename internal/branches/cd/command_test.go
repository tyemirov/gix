package cd

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/utils"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/workflow"
)

func TestCommandBuilds(t *testing.T) {
	builder := CommandBuilder{}
	command, err := builder.Build()
	require.NoError(t, err)
	require.IsType(t, &cobra.Command{}, command)
}

func TestCommandUsageIncludesBranchPlaceholder(t *testing.T) {
	builder := CommandBuilder{}
	command, err := builder.Build()
	require.NoError(t, err)
	require.Contains(t, command.Use, "<branch>")
}

func TestCommandRequiresBranchArgument(t *testing.T) {
	builder := CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() CommandConfiguration {
			return CommandConfiguration{}
		},
		GitExecutor: &stubGitExecutor{},
	}
	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	require.Error(t, command.RunE(command, []string{}))
}

func TestCommandExecutesAcrossRoots(t *testing.T) {
	temporaryRoot := t.TempDir()
	executor := &stubGitExecutor{}
	runner := &recordingTaskRunner{}
	builder := CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() CommandConfiguration {
			return CommandConfiguration{RepositoryRoots: []string{temporaryRoot}, RemoteName: "origin"}
		},
		GitExecutor: executor,
		TaskRunnerFactory: func(deps workflow.Dependencies) TaskRunnerExecutor {
			runner.dependencies = deps
			return runner
		},
	}
	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

	contextAccessor := utils.NewCommandContextAccessor()
	command.SetContext(contextAccessor.WithExecutionFlags(context.Background(), utils.ExecutionFlags{DryRun: false}))

	require.NoError(t, command.RunE(command, []string{"feature/foo"}))
	require.Equal(t, []string{temporaryRoot}, runner.roots)
	require.False(t, runner.runtimeOptions.DryRun)
	require.Len(t, runner.definitions, 1)
	require.Len(t, runner.definitions[0].Actions, 1)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, taskTypeBranchChange, action.Type)
	require.Equal(t, "feature/foo", action.Options[taskOptionBranchName])
	require.Equal(t, "origin", action.Options[taskOptionBranchRemote])
}

type recordingTaskRunner struct {
	dependencies   workflow.Dependencies
	roots          []string
	definitions    []workflow.TaskDefinition
	runtimeOptions workflow.RuntimeOptions
}

func (runner *recordingTaskRunner) Run(ctx context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) error {
	runner.roots = append([]string{}, roots...)
	runner.definitions = append([]workflow.TaskDefinition{}, definitions...)
	runner.runtimeOptions = options
	return nil
}
