package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
	migrate "github.com/tyemirov/gix/internal/migrate"
	"github.com/tyemirov/gix/internal/migrate/cli"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	rootFlagArgumentConstant      = "--" + flagutils.DefaultRootFlagName
	dryRunFlagArgumentConstant    = "--" + flagutils.DryRunFlagName
	assumeYesFlagArgumentConstant = "--" + flagutils.AssumeYesFlagName
)

var boundRootFlagHolders []*flagutils.RootFlagValues

func TestCommandUsesConfigurationRootsAndTargetBranch(t *testing.T) {
	t.Helper()

	root := "/tmp/migrate-root"
	discoverer := &fakeRepositoryDiscoverer{repositories: []string{root}}
	executor := &stubGitExecutor{}
	manager := stubGitRepositoryManager{}
	runner := &recordingTaskRunner{}

	builder := cli.CommandBuilder{
		LoggerProvider:       func() *zap.Logger { return zap.NewNop() },
		Discoverer:           discoverer,
		GitExecutor:          executor,
		GitRepositoryManager: manager,
		ConfigurationProvider: func() migrate.CommandConfiguration {
			return migrate.CommandConfiguration{
				RepositoryRoots: []string{root},
				TargetBranch:    "master",
			}
		},
		TaskRunnerFactory: func(workflow.Dependencies) cli.TaskRunnerExecutor { return runner },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindRootAndExecutionFlags(command)

	command.SetContext(context.Background())
	command.SetArgs([]string{rootFlagArgumentConstant, root})

	executionError := command.Execute()
	require.NoError(t, executionError)

	require.Len(t, runner.definitions, 1)
	require.Len(t, runner.definitions[0].Actions, 1)

	action := runner.definitions[0].Actions[0]
	require.Equal(t, "branch.default", action.Type)
	require.Equal(t, "master", action.Options["target"])

	require.False(t, runner.runtimeOptions.DryRun)
	require.False(t, runner.runtimeOptions.AssumeYes)
}

func TestCommandFlagsOverrideConfiguration(t *testing.T) {
	t.Helper()

	configuredRoot := "/tmp/configured-root"
	flagRoot := "/tmp/flag-root"

	discoverer := &fakeRepositoryDiscoverer{repositories: []string{flagRoot}}
	executor := &stubGitExecutor{}
	manager := stubGitRepositoryManager{}
	runner := &recordingTaskRunner{}

	builder := cli.CommandBuilder{
		LoggerProvider:       func() *zap.Logger { return zap.NewNop() },
		Discoverer:           discoverer,
		GitExecutor:          executor,
		GitRepositoryManager: manager,
		ConfigurationProvider: func() migrate.CommandConfiguration {
			return migrate.CommandConfiguration{
				RepositoryRoots: []string{configuredRoot},
				TargetBranch:    "master",
			}
		},
		TaskRunnerFactory: func(workflow.Dependencies) cli.TaskRunnerExecutor { return runner },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindRootAndExecutionFlags(command)

	command.SetContext(context.Background())
	command.SetArgs([]string{
		"stable",
		rootFlagArgumentConstant, flagRoot,
		dryRunFlagArgumentConstant + "=yes",
		assumeYesFlagArgumentConstant,
	})

	executionError := command.Execute()
	require.NoError(t, executionError)

	require.Len(t, runner.definitions, 1)
	require.Equal(t, "stable", runner.definitions[0].Actions[0].Options["target"])
	require.True(t, runner.runtimeOptions.DryRun)
	require.True(t, runner.runtimeOptions.AssumeYes)
}

func TestCommandDisplaysHelpWhenRootsMissing(t *testing.T) {
	t.Helper()

	builder := cli.CommandBuilder{
		LoggerProvider:        func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() migrate.CommandConfiguration { return migrate.CommandConfiguration{} },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindRootAndExecutionFlags(command)

	command.SetContext(context.Background())
	command.SetArgs([]string{})

	outputBuffer := &strings.Builder{}
	command.SetOut(outputBuffer)
	command.SetErr(outputBuffer)

	executionError := command.Execute()
	require.Error(t, executionError)
	require.Equal(t, "no repository roots provided; specify --roots or configure defaults", executionError.Error())
	require.Contains(t, outputBuffer.String(), command.UseLine())
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
func (stubGitRepositoryManager) GetCurrentBranch(context.Context, string) (string, error) {
	return "main", nil
}
func (stubGitRepositoryManager) GetRemoteURL(context.Context, string, string) (string, error) {
	return "", nil
}
func (stubGitRepositoryManager) SetRemoteURL(context.Context, string, string, string) error {
	return nil
}

func bindRootAndExecutionFlags(command *cobra.Command) {
	rootValues := flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Name: flagutils.DefaultRootFlagName, Enabled: true})
	boundRootFlagHolders = append(boundRootFlagHolders, rootValues)
	flagutils.BindExecutionFlags(
		command,
		flagutils.ExecutionDefaults{},
		flagutils.ExecutionFlagDefinitions{
			DryRun:    flagutils.ExecutionFlagDefinition{Name: flagutils.DryRunFlagName, Usage: flagutils.DryRunFlagUsage, Enabled: true},
			AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Shorthand: flagutils.AssumeYesFlagShorthand, Enabled: true},
		},
	)
}
