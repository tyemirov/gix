package cd

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/temirov/gix/internal/utils"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
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
	require.Contains(t, command.Use, "[branch]")
}

func TestCommandAllowsMissingBranchAndUsesConfiguredFallback(t *testing.T) {
	builder := CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() CommandConfiguration {
			return CommandConfiguration{DefaultBranch: "main", RepositoryRoots: []string{t.TempDir()}, RemoteName: "origin"}
		},
		GitExecutor: &stubGitExecutor{},
	}
	runner := &recordingTaskRunner{}
	builder.TaskRunnerFactory = func(deps workflow.Dependencies) TaskRunnerExecutor {
		runner.dependencies = deps
		return runner
	}
	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

	contextAccessor := utils.NewCommandContextAccessor()
	command.SetContext(contextAccessor.WithExecutionFlags(context.Background(), utils.ExecutionFlags{}))

	require.NoError(t, command.RunE(command, []string{}))
	require.Len(t, runner.definitions, 1)
	require.Equal(t, "Switch branch to main", runner.definitions[0].Name)
	require.Len(t, runner.definitions[0].Actions, 1)
	action := runner.definitions[0].Actions[0]
	_, explicitExists := action.Options[taskOptionBranchName]
	require.False(t, explicitExists)
	require.Equal(t, "main", action.Options[taskOptionConfiguredDefaultBranch])
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
	command.SetContext(contextAccessor.WithExecutionFlags(context.Background(), utils.ExecutionFlags{}))

	require.NoError(t, command.RunE(command, []string{"feature/foo"}))
	require.Equal(t, []string{temporaryRoot}, runner.roots)
	require.Len(t, runner.definitions, 1)
	require.Len(t, runner.definitions[0].Actions, 1)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, taskTypeBranchChange, action.Type)
	require.Equal(t, "feature/foo", action.Options[taskOptionBranchName])
	require.Equal(t, "origin", action.Options[taskOptionBranchRemote])
}

func TestCommandRefreshFlagsPropagateToAction(t *testing.T) {
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

	require.NoError(t, command.Flags().Set(refreshFlagNameConstant, "true"))
	require.NoError(t, command.Flags().Set(stashFlagNameConstant, "true"))
	require.NoError(t, command.Flags().Set(requireCleanFlagNameConstant, "false"))

	contextAccessor := utils.NewCommandContextAccessor()
	command.SetContext(contextAccessor.WithExecutionFlags(context.Background(), utils.ExecutionFlags{}))

	require.NoError(t, command.RunE(command, []string{"feature/foo"}))
	require.Len(t, runner.definitions, 1)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, taskTypeBranchChange, action.Type)
	require.Equal(t, true, action.Options[taskOptionRefreshEnabled])
	require.Equal(t, true, action.Options[taskOptionStashChanges])
	require.Equal(t, false, action.Options[taskOptionRequireClean])
}

func TestCommandRejectsConflictingRecoveryFlags(t *testing.T) {
	builder := CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() CommandConfiguration {
			return CommandConfiguration{RepositoryRoots: []string{t.TempDir()}, RemoteName: "origin"}
		},
		GitExecutor: &stubGitExecutor{},
	}
	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

	require.NoError(t, command.Flags().Set(stashFlagNameConstant, "true"))
	require.NoError(t, command.Flags().Set(commitFlagNameConstant, "true"))

	contextAccessor := utils.NewCommandContextAccessor()
	command.SetContext(contextAccessor.WithExecutionFlags(context.Background(), utils.ExecutionFlags{}))

	require.Error(t, command.RunE(command, []string{"feature/foo"}))
}

type recordingTaskRunner struct {
	dependencies   workflow.Dependencies
	roots          []string
	definitions    []workflow.TaskDefinition
	runtimeOptions workflow.RuntimeOptions
}

func (runner *recordingTaskRunner) Run(ctx context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	runner.roots = append([]string{}, roots...)
	runner.definitions = append([]workflow.TaskDefinition{}, definitions...)
	runner.runtimeOptions = options
	return workflow.ExecutionOutcome{}, nil
}
