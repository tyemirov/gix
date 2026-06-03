package syncflow

import (
	"context"
	"os"
	"strings"
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

func TestCommandUsageIncludesRemoteOrBranchPlaceholder(t *testing.T) {
	builder := CommandBuilder{}
	command, err := builder.Build()
	require.NoError(t, err)
	require.Contains(t, command.Use, "[remote-url|branch]")
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
	require.Equal(t, "Sync main", runner.definitions[0].Name)
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
	require.Equal(t, taskTypeBranchSync, action.Type)
	require.Equal(t, "feature/foo", action.Options[taskOptionBranchName])
	require.Equal(t, "origin", action.Options[taskOptionBranchRemote])
	require.Equal(t, true, action.Options[taskOptionRequirePullRequest])
	require.Equal(t, "master", action.Options[taskOptionBaseBranch])
}

func TestCommandPropagatesPullRequestTitleAndBodyOptions(t *testing.T) {
	temporaryRoot := t.TempDir()
	runner := &recordingTaskRunner{}
	builder := CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() CommandConfiguration {
			return CommandConfiguration{RepositoryRoots: []string{temporaryRoot}, RemoteName: "origin"}
		},
		GitExecutor: &stubGitExecutor{},
		TaskRunnerFactory: func(deps workflow.Dependencies) TaskRunnerExecutor {
			runner.dependencies = deps
			return runner
		},
	}
	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

	require.NoError(t, command.Flags().Set(taskOptionPullRequestTitle, "docs: explain sync"))
	require.NoError(t, command.Flags().Set(taskOptionPullRequestBody, "Explain the reviewer-facing reason."))

	contextAccessor := utils.NewCommandContextAccessor()
	command.SetContext(contextAccessor.WithExecutionFlags(context.Background(), utils.ExecutionFlags{}))

	require.NoError(t, command.RunE(command, []string{"feature/foo"}))
	require.Len(t, runner.definitions, 1)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, "docs: explain sync", action.Options[taskOptionPullRequestTitle])
	require.Equal(t, "Explain the reviewer-facing reason.", action.Options[taskOptionPullRequestBody])
}

func TestCommandPropagatesConfiguredPullRequestTitleAndBodyOptions(t *testing.T) {
	temporaryRoot := t.TempDir()
	runner := &recordingTaskRunner{}
	builder := CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() CommandConfiguration {
			return CommandConfiguration{
				RepositoryRoots: []string{temporaryRoot},
				RemoteName:      "origin",
				PullRequest: PullRequestConfiguration{
					Title: "docs: explain configured sync",
					Body:  "Explain the configured reviewer-facing reason.",
				},
			}
		},
		GitExecutor: &stubGitExecutor{},
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
	require.Len(t, runner.definitions, 1)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, "docs: explain configured sync", action.Options[taskOptionPullRequestTitle])
	require.Equal(t, "Explain the configured reviewer-facing reason.", action.Options[taskOptionPullRequestBody])
}

func TestCommandStashFlagEnablesRefreshAndPropagatesOptions(t *testing.T) {
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

	require.NoError(t, command.Flags().Set(stashFlagNameConstant, "true"))
	require.NoError(t, command.Flags().Set(requireCleanFlagNameConstant, "false"))

	contextAccessor := utils.NewCommandContextAccessor()
	command.SetContext(contextAccessor.WithExecutionFlags(context.Background(), utils.ExecutionFlags{}))

	require.NoError(t, command.RunE(command, []string{"feature/foo"}))
	require.Len(t, runner.definitions, 1)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, taskTypeBranchSync, action.Type)
	require.Equal(t, true, action.Options[taskOptionRefreshEnabled])
	require.Equal(t, true, action.Options[taskOptionStashChanges])
	require.Equal(t, false, action.Options[taskOptionRequireClean])
	require.Equal(t, true, action.Options[taskOptionRequirePullRequest])
}

func TestCommandPropagatesRequireCleanFalseWithoutRecoveryFlag(t *testing.T) {
	temporaryRoot := t.TempDir()
	runner := &recordingTaskRunner{}
	builder := CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() CommandConfiguration {
			return CommandConfiguration{RepositoryRoots: []string{temporaryRoot}, RemoteName: "origin"}
		},
		GitExecutor: &stubGitExecutor{},
		TaskRunnerFactory: func(deps workflow.Dependencies) TaskRunnerExecutor {
			runner.dependencies = deps
			return runner
		},
	}
	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	require.NoError(t, command.Flags().Set(requireCleanFlagNameConstant, "false"))

	contextAccessor := utils.NewCommandContextAccessor()
	command.SetContext(contextAccessor.WithExecutionFlags(context.Background(), utils.ExecutionFlags{}))

	require.NoError(t, command.RunE(command, []string{"feature/foo"}))
	require.Len(t, runner.definitions, 1)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, false, action.Options[taskOptionRequireClean])
	_, refreshExists := action.Options[taskOptionRefreshEnabled]
	require.False(t, refreshExists)
}

func TestCommandRemoteTargetClonesIntoEmptyDirectory(t *testing.T) {
	originalWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(t, workingDirectoryError)
	temporaryDirectory := t.TempDir()
	require.NoError(t, os.Chdir(temporaryDirectory))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalWorkingDirectory))
	})

	executor := &stubGitExecutor{}
	builder := CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() CommandConfiguration {
			return CommandConfiguration{RepositoryRoots: []string{temporaryDirectory}, RemoteName: "origin"}
		},
		GitExecutor: executor,
	}
	command, err := builder.Build()
	require.NoError(t, err)
	output := &strings.Builder{}
	command.SetOut(output)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

	contextAccessor := utils.NewCommandContextAccessor()
	command.SetContext(contextAccessor.WithExecutionFlags(context.Background(), utils.ExecutionFlags{}))

	require.NoError(t, command.RunE(command, []string{"https://github.com/owner/project.git"}))
	require.Len(t, executor.recorded, 2)
	require.Equal(t, []string{"rev-parse", "--is-inside-work-tree"}, executor.recorded[0].Arguments)
	require.Equal(t, []string{"clone", "https://github.com/owner/project.git", "."}, executor.recorded[1].Arguments)
	require.Contains(t, output.String(), "CLONED: https://github.com/owner/project.git")
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
