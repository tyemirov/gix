package refresh_test

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/branches/refresh"
	"github.com/tyemirov/gix/internal/execshell"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/workflow"
)

type recordingGitExecutor struct {
	invocationErrors []error
	recordedCommands []execshell.CommandDetails
}

func (executor *recordingGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.recordedCommands = append(executor.recordedCommands, details)
	if len(executor.invocationErrors) == 0 {
		return execshell.ExecutionResult{}, nil
	}
	err := executor.invocationErrors[0]
	executor.invocationErrors = executor.invocationErrors[1:]
	if err != nil {
		return execshell.ExecutionResult{}, err
	}
	return execshell.ExecutionResult{}, nil
}

func (executor *recordingGitExecutor) ExecuteGitHubCLI(_ context.Context, _ execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type constantCleanRepositoryManager struct{}

type erroringRepositoryManager struct{}

func (constantCleanRepositoryManager) CheckCleanWorktree(context.Context, string) (bool, error) {
	return true, nil
}

func (constantCleanRepositoryManager) GetCurrentBranch(context.Context, string) (string, error) {
	return "", nil
}

func (constantCleanRepositoryManager) GetRemoteURL(context.Context, string, string) (string, error) {
	return "", nil
}

func (constantCleanRepositoryManager) SetRemoteURL(context.Context, string, string, string) error {
	return nil
}

func (erroringRepositoryManager) CheckCleanWorktree(context.Context, string) (bool, error) {
	return false, nil
}

func (erroringRepositoryManager) GetCurrentBranch(context.Context, string) (string, error) {
	return "", nil
}

func (erroringRepositoryManager) GetRemoteURL(context.Context, string, string) (string, error) {
	return "", nil
}

func (erroringRepositoryManager) SetRemoteURL(context.Context, string, string, string) error {
	return nil
}

type recordingTaskRunner struct {
	dependencies workflow.Dependencies
	roots        []string
	definitions  []workflow.TaskDefinition
	options      workflow.RuntimeOptions
}

func (runner *recordingTaskRunner) Run(_ context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) error {
	runner.roots = append([]string{}, roots...)
	runner.definitions = append([]workflow.TaskDefinition{}, definitions...)
	runner.options = options
	return nil
}

type failingTaskRunner struct {
	err error
}

func (runner failingTaskRunner) Run(context.Context, []string, []workflow.TaskDefinition, workflow.RuntimeOptions) error {
	return runner.err
}

func TestBuildReturnsCommand(t *testing.T) {
	builder := refresh.CommandBuilder{}
	command, buildError := builder.Build()
	require.NoError(t, buildError)
	require.IsType(t, &cobra.Command{}, command)
}

func TestCommandRequiresBranchName(t *testing.T) {
	builder := refresh.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() refresh.CommandConfiguration {
			return refresh.CommandConfiguration{}
		},
		GitExecutor:          &recordingGitExecutor{},
		GitRepositoryManager: constantCleanRepositoryManager{},
		TaskRunnerFactory: func(workflow.Dependencies) refresh.TaskRunnerExecutor {
			return &recordingTaskRunner{}
		},
	}
	command, buildError := builder.Build()
	require.NoError(t, buildError)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	require.Error(t, command.RunE(command, []string{}))
}

func TestCommandRunsSuccessfully(t *testing.T) {
	temporaryRepository := t.TempDir()
	executor := &recordingGitExecutor{}
	runner := &recordingTaskRunner{}
	builder := refresh.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() refresh.CommandConfiguration {
			return refresh.CommandConfiguration{RepositoryRoots: []string{temporaryRepository}}
		},
		GitExecutor:          executor,
		GitRepositoryManager: constantCleanRepositoryManager{},
		TaskRunnerFactory: func(deps workflow.Dependencies) refresh.TaskRunnerExecutor {
			runner.dependencies = deps
			return runner
		},
	}
	command, buildError := builder.Build()
	require.NoError(t, buildError)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

	require.NoError(t, command.Flags().Set("branch", "main"))

	require.NoError(t, command.RunE(command, []string{}))
	require.Equal(t, []string{temporaryRepository}, runner.roots)
	require.False(t, runner.options.DryRun)
	require.False(t, runner.options.AssumeYes)
	require.Len(t, runner.definitions, 1)
	require.Len(t, runner.definitions[0].Actions, 1)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, "branch.refresh", action.Type)
	require.Equal(t, "main", action.Options["branch"])
	require.False(t, action.Options["stash"].(bool))
	require.False(t, action.Options["commit"].(bool))
	require.True(t, action.Options["require_clean"].(bool))
}

func TestCommandReportsDirtyWorktree(t *testing.T) {
	temporaryRepository := t.TempDir()
	failure := errors.New("refresh failed")
	builder := refresh.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() refresh.CommandConfiguration {
			return refresh.CommandConfiguration{RepositoryRoots: []string{temporaryRepository}, BranchName: "main"}
		},
		GitExecutor:          &recordingGitExecutor{},
		GitRepositoryManager: erroringRepositoryManager{},
		TaskRunnerFactory: func(workflow.Dependencies) refresh.TaskRunnerExecutor {
			return failingTaskRunner{err: failure}
		},
	}
	command, buildError := builder.Build()
	require.NoError(t, buildError)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

	require.ErrorIs(t, command.RunE(command, []string{}), failure)
}

func TestCommandRejectsConflictingFlags(t *testing.T) {
	temporaryRepository := t.TempDir()
	builder := refresh.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() refresh.CommandConfiguration {
			return refresh.CommandConfiguration{RepositoryRoots: []string{temporaryRepository}, BranchName: "main"}
		},
		GitExecutor:          &recordingGitExecutor{},
		GitRepositoryManager: constantCleanRepositoryManager{},
		TaskRunnerFactory: func(workflow.Dependencies) refresh.TaskRunnerExecutor {
			return &recordingTaskRunner{}
		},
	}
	command, buildError := builder.Build()
	require.NoError(t, buildError)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

	require.NoError(t, command.Flags().Set("stash", "true"))
	require.NoError(t, command.Flags().Set("commit", "true"))

	require.Error(t, command.RunE(command, []string{}))
}
