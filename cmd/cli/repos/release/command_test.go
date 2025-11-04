package release

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repocli "github.com/tyemirov/gix/cmd/cli/repos"
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/utils"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/workflow"
)

type stubGitExecutor struct {
	recorded []execshell.CommandDetails
}

func (executor *stubGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.recorded = append(executor.recorded, details)
	return execshell.ExecutionResult{}, nil
}

func (executor *stubGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type recordingTaskRunner struct {
	roots          []string
	definitions    []workflow.TaskDefinition
	runtimeOptions workflow.RuntimeOptions
}

func (runner *recordingTaskRunner) Run(_ context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) error {
	runner.roots = append([]string{}, roots...)
	runner.definitions = append([]workflow.TaskDefinition{}, definitions...)
	runner.runtimeOptions = options
	return nil
}

func TestCommandBuilds(t *testing.T) {
	builder := CommandBuilder{}
	command, err := builder.Build()
	require.NoError(t, err)
	require.IsType(t, &cobra.Command{}, command)
	require.Equal(t, commandUsageTemplate, strings.TrimSpace(command.Use))
	require.NotEmpty(t, strings.TrimSpace(command.Example))
}

func TestCommandRequiresTagArgument(t *testing.T) {
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
	require.Error(t, command.RunE(command, []string{"   "}))
}

func TestCommandRunsAcrossRoots(t *testing.T) {
	executor := &stubGitExecutor{}
	root := t.TempDir()
	runner := &recordingTaskRunner{}
	builder := CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() CommandConfiguration {
			return CommandConfiguration{RepositoryRoots: []string{root}, RemoteName: "origin", Message: "Ship it"}
		},
		GitExecutor: executor,
		TaskRunnerFactory: func(workflow.Dependencies) repocli.TaskRunnerExecutor {
			return runner
		},
	}
	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

	contextAccessor := utils.NewCommandContextAccessor()
	command.SetContext(contextAccessor.WithExecutionFlags(context.Background(), utils.ExecutionFlags{}))

	require.NoError(t, command.RunE(command, []string{"v1.2.3"}))
	require.Equal(t, []string{root}, runner.roots)
	require.Len(t, runner.definitions, 1)
	actionDefinitions := runner.definitions[0].Actions
	require.Len(t, actionDefinitions, 1)
	action := actionDefinitions[0]
	require.Equal(t, "repo.release.tag", action.Type)
	require.Equal(t, "v1.2.3", action.Options["tag"])
	require.Equal(t, "Ship it", action.Options["message"])
	require.Equal(t, "origin", action.Options["remote"])
	require.False(t, runner.runtimeOptions.DryRun)
	require.False(t, runner.runtimeOptions.AssumeYes)
}
