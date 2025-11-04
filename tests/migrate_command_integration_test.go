package tests

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
	migrate "github.com/tyemirov/gix/internal/migrate"
	migratecli "github.com/tyemirov/gix/internal/migrate/cli"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/workflow"
)

var integrationBoundRootFlagHolders []*flagutils.RootFlagValues

func TestBranchDefaultCommandInvokesTaskRunner(t *testing.T) {
	t.Helper()

	root := "/tmp/integration-root"
	discoverer := &integrationRepositoryDiscoverer{repositories: []string{root}}
	executor := &integrationGitExecutor{}
	manager := integrationGitRepositoryManager{}
	runner := &integrationRecordingTaskRunner{}

	builder := migratecli.CommandBuilder{
		LoggerProvider:       func() *zap.Logger { return zap.NewNop() },
		Discoverer:           discoverer,
		GitExecutor:          executor,
		GitRepositoryManager: manager,
		ConfigurationProvider: func() migrate.CommandConfiguration {
			return migrate.CommandConfiguration{RepositoryRoots: []string{root}, TargetBranch: "master"}
		},
		TaskRunnerFactory: func(workflow.Dependencies) migratecli.TaskRunnerExecutor { return runner },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindRootAndExecutionFlags(command)

	command.SetContext(context.Background())
	command.SetArgs([]string{"--" + flagutils.DefaultRootFlagName, root})

	executionError := command.Execute()
	require.NoError(t, executionError)

	require.Len(t, runner.definitions, 1)
	require.Equal(t, "branch.default", runner.definitions[0].Actions[0].Type)
}

func TestBranchDefaultCommandRespectsFlags(t *testing.T) {
	t.Helper()

	root := "/tmp/integration-root-flags"
	discoverer := &integrationRepositoryDiscoverer{repositories: []string{root}}
	executor := &integrationGitExecutor{}
	manager := integrationGitRepositoryManager{}
	runner := &integrationRecordingTaskRunner{}

	builder := migratecli.CommandBuilder{
		LoggerProvider:       func() *zap.Logger { return zap.NewNop() },
		Discoverer:           discoverer,
		GitExecutor:          executor,
		GitRepositoryManager: manager,
		ConfigurationProvider: func() migrate.CommandConfiguration {
			return migrate.CommandConfiguration{}
		},
		TaskRunnerFactory: func(workflow.Dependencies) migratecli.TaskRunnerExecutor { return runner },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindRootAndExecutionFlags(command)

	command.SetContext(context.Background())
	command.SetArgs([]string{
		"stable",
		"--" + flagutils.DefaultRootFlagName, root,
		"--" + flagutils.DryRunFlagName + "=yes",
	})

	executionError := command.Execute()
	require.NoError(t, executionError)

	require.Equal(t, "stable", runner.definitions[0].Actions[0].Options["target"])
	require.True(t, runner.runtimeOptions.DryRun)
}

type integrationRecordingTaskRunner struct {
	roots          []string
	definitions    []workflow.TaskDefinition
	runtimeOptions workflow.RuntimeOptions
}

func (runner *integrationRecordingTaskRunner) Run(_ context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) error {
	runner.roots = append([]string{}, roots...)
	runner.definitions = append([]workflow.TaskDefinition{}, definitions...)
	runner.runtimeOptions = options
	return nil
}

type integrationRepositoryDiscoverer struct {
	repositories []string
}

func (discoverer *integrationRepositoryDiscoverer) DiscoverRepositories([]string) ([]string, error) {
	return append([]string{}, discoverer.repositories...), nil
}

type integrationGitExecutor struct{}

func (integrationGitExecutor) ExecuteGit(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func (integrationGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type integrationGitRepositoryManager struct{}

func (integrationGitRepositoryManager) CheckCleanWorktree(context.Context, string) (bool, error) {
	return true, nil
}
func (integrationGitRepositoryManager) GetCurrentBranch(context.Context, string) (string, error) {
	return "main", nil
}
func (integrationGitRepositoryManager) GetRemoteURL(context.Context, string, string) (string, error) {
	return "", nil
}
func (integrationGitRepositoryManager) SetRemoteURL(context.Context, string, string, string) error {
	return nil
}

func bindRootAndExecutionFlags(command *cobra.Command) {
	rootValues := flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Name: flagutils.DefaultRootFlagName, Enabled: true})
	integrationBoundRootFlagHolders = append(integrationBoundRootFlagHolders, rootValues)
	flagutils.BindExecutionFlags(
		command,
		flagutils.ExecutionDefaults{},
		flagutils.ExecutionFlagDefinitions{
			DryRun:    flagutils.ExecutionFlagDefinition{Name: flagutils.DryRunFlagName, Usage: flagutils.DryRunFlagUsage, Enabled: true},
			AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Shorthand: flagutils.AssumeYesFlagShorthand, Enabled: true},
		},
	)
}
