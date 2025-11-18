package branches_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	branches "github.com/tyemirov/gix/internal/branches"
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	rootutils "github.com/tyemirov/gix/internal/utils/roots"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	commandRemoteFlagConstant         = "--" + flagutils.RemoteFlagName
	commandLimitFlagConstant          = "--limit"
	commandRootFlagConstant           = "--" + flagutils.DefaultRootFlagName
	commandAssumeYesFlagConstant      = "--" + flagutils.AssumeYesFlagName
	configurationRemoteNameConstant   = "configured-remote"
	configurationRootConstant         = "/tmp/config-root"
	invalidRemoteErrorMessageConstant = "remote name must not be empty or whitespace"
	invalidLimitErrorMessageConstant  = "limit must be greater than zero"
)

type recordingTaskRunner struct {
	roots          []string
	definitions    []workflow.TaskDefinition
	runtimeOptions workflow.RuntimeOptions
	dependencies   workflow.Dependencies
}

func (runner *recordingTaskRunner) Run(_ context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	runner.roots = append([]string{}, roots...)
	runner.definitions = append([]workflow.TaskDefinition{}, definitions...)
	runner.runtimeOptions = options
	if runner.dependencies.RepositoryDiscoverer != nil {
		_, _ = runner.dependencies.RepositoryDiscoverer.DiscoverRepositories(roots)
	}
	return workflow.ExecutionOutcome{}, nil
}

type fakeRepositoryDiscoverer struct {
	repositories  []string
	receivedRoots []string
}

func (discoverer *fakeRepositoryDiscoverer) DiscoverRepositories(roots []string) ([]string, error) {
	discoverer.receivedRoots = append([]string{}, roots...)
	return append([]string{}, discoverer.repositories...), nil
}

type stubGitExecutor struct{}

func (stubGitExecutor) ExecuteGit(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func (stubGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type stubGitRepositoryManager struct{}

func (stubGitRepositoryManager) CheckCleanWorktree(context.Context, string) (bool, error) {
	return true, nil
}
func (stubGitRepositoryManager) WorktreeStatus(context.Context, string) ([]string, error) {
	return nil, nil
}
func (stubGitRepositoryManager) GetCurrentBranch(context.Context, string) (string, error) {
	return "main", nil
}
func (stubGitRepositoryManager) GetRemoteURL(context.Context, string, string) (string, error) {
	return "", nil
}
func (stubGitRepositoryManager) SetRemoteURL(context.Context, string, string, string) error {
	return nil
}

type stubPrompter struct{}

func (stubPrompter) Confirm(string) (shared.ConfirmationResult, error) {
	return shared.ConfirmationResult{Confirmed: true}, nil
}

func TestCommandConfigurationPrecedence(t *testing.T) {
	root := "/tmp/branches-root"
	discoverer := &fakeRepositoryDiscoverer{repositories: []string{root}}
	executor := &stubGitExecutor{}
	manager := stubGitRepositoryManager{}
	runner := &recordingTaskRunner{}

	configuration := branches.CommandConfiguration{
		RemoteName:       configurationRemoteNameConstant,
		PullRequestLimit: 42,
		RepositoryRoots:  []string{configurationRootConstant},
		AssumeYes:        false,
	}

	builder := branches.CommandBuilder{
		LoggerProvider:        func() *zap.Logger { return zap.NewNop() },
		Discoverer:            discoverer,
		GitExecutor:           executor,
		GitManager:            manager,
		PrompterFactory:       func(*cobra.Command) shared.ConfirmationPrompter { return stubPrompter{} },
		ConfigurationProvider: func() branches.CommandConfiguration { return configuration },
		TaskRunnerFactory: func(deps workflow.Dependencies) branches.TaskRunnerExecutor {
			runner.dependencies = deps
			return runner
		},
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindGlobalBranchFlags(command)
	command.SetContext(context.Background())
	command.SetArgs([]string{})

	executionError := command.Execute()
	require.NoError(t, executionError)

	require.Equal(t, configuration.RepositoryRoots, runner.roots)
	require.Len(t, runner.definitions, 1)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, "repo.branches.cleanup", action.Type)
	require.Equal(t, configurationRemoteNameConstant, action.Options["remote"])
	require.Equal(t, strconv.Itoa(configuration.PullRequestLimit), action.Options["limit"])
	require.False(t, runner.runtimeOptions.AssumeYes)
	require.True(t, runner.runtimeOptions.SkipRepositoryMetadata)
}

func TestCommandFlagsOverrideConfiguration(t *testing.T) {
	discoverer := &fakeRepositoryDiscoverer{repositories: []string{"/tmp/branches"}}
	executor := &stubGitExecutor{}
	manager := stubGitRepositoryManager{}
	runner := &recordingTaskRunner{}

	builder := branches.CommandBuilder{
		LoggerProvider:  func() *zap.Logger { return zap.NewNop() },
		Discoverer:      discoverer,
		GitExecutor:     executor,
		GitManager:      manager,
		PrompterFactory: func(*cobra.Command) shared.ConfirmationPrompter { return stubPrompter{} },
		ConfigurationProvider: func() branches.CommandConfiguration {
			return branches.CommandConfiguration{
				RemoteName:       configurationRemoteNameConstant,
				PullRequestLimit: 12,
				RepositoryRoots:  []string{configurationRootConstant},
			}
		},
		TaskRunnerFactory: func(deps workflow.Dependencies) branches.TaskRunnerExecutor {
			runner.dependencies = deps
			return runner
		},
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindGlobalBranchFlags(command)
	command.SetContext(context.Background())
	command.SetArgs([]string{commandRemoteFlagConstant, "override-remote", commandLimitFlagConstant, "7", commandAssumeYesFlagConstant, commandRootFlagConstant, "/tmp/other"})

	executionError := command.Execute()
	require.NoError(t, executionError)

	require.Equal(t, []string{"/tmp/other"}, runner.roots)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, "override-remote", action.Options["remote"])
	require.Equal(t, "7", action.Options["limit"])
	require.True(t, runner.runtimeOptions.AssumeYes)
	require.True(t, runner.runtimeOptions.SkipRepositoryMetadata)
}

func TestCommandHandlesExplicitYesValue(t *testing.T) {
	discoverer := &fakeRepositoryDiscoverer{repositories: []string{"/tmp/branches"}}
	executor := &stubGitExecutor{}
	manager := stubGitRepositoryManager{}
	runner := &recordingTaskRunner{}

	builder := branches.CommandBuilder{
		LoggerProvider:  func() *zap.Logger { return zap.NewNop() },
		Discoverer:      discoverer,
		GitExecutor:     executor,
		GitManager:      manager,
		PrompterFactory: func(*cobra.Command) shared.ConfirmationPrompter { return stubPrompter{} },
		ConfigurationProvider: func() branches.CommandConfiguration {
			return branches.CommandConfiguration{
				RemoteName:       configurationRemoteNameConstant,
				PullRequestLimit: 12,
				RepositoryRoots:  []string{configurationRootConstant},
			}
		},
		TaskRunnerFactory: func(deps workflow.Dependencies) branches.TaskRunnerExecutor {
			runner.dependencies = deps
			return runner
		},
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindGlobalBranchFlags(command)
	command.SetContext(context.Background())
	command.SetArgs([]string{commandAssumeYesFlagConstant, "yes"})

	executionError := command.Execute()
	require.Error(t, executionError)
	require.Equal(t, rootutils.PositionalRootsUnsupportedMessage(), executionError.Error())
}

func TestCommandErrorsWhenRemoteInvalid(t *testing.T) {
	builder := branches.CommandBuilder{}
	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindGlobalBranchFlags(command)
	command.SetContext(context.Background())
	command.SetArgs([]string{commandRootFlagConstant, "/tmp/root", commandRemoteFlagConstant, "   "})

	executionError := command.Execute()
	require.Error(t, executionError)
	require.Equal(t, invalidRemoteErrorMessageConstant, executionError.Error())
}

func TestCommandErrorsWhenLimitInvalid(t *testing.T) {
	builder := branches.CommandBuilder{}
	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindGlobalBranchFlags(command)
	command.SetContext(context.Background())
	command.SetArgs([]string{commandRootFlagConstant, "/tmp/root", commandLimitFlagConstant, "0"})

	executionError := command.Execute()
	require.Error(t, executionError)
	require.Equal(t, invalidLimitErrorMessageConstant, executionError.Error())
}

func TestCommandErrorsWhenRootsMissing(t *testing.T) {
	builder := branches.CommandBuilder{}
	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindGlobalBranchFlags(command)
	command.SetContext(context.Background())
	command.SetArgs([]string{})

	executionError := command.Execute()
	require.Error(t, executionError)
	require.Equal(t, rootutils.MissingRootsMessage(), executionError.Error())
}

func bindGlobalBranchFlags(command *cobra.Command) {
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	flagutils.BindExecutionFlags(command, flagutils.ExecutionDefaults{}, flagutils.ExecutionFlagDefinitions{
		AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Shorthand: flagutils.AssumeYesFlagShorthand, Enabled: true},
	})
	flagutils.EnsureRemoteFlag(command, configurationRemoteNameConstant, "remote")
}
