package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/gix/internal/audit/cli"
	"github.com/tyemirov/gix/internal/execshell"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	rootFlagArgumentConstant       = "--" + flagutils.DefaultRootFlagName
	includeAllFlagArgumentConstant = "--all"
)

var boundRootFlagValues []*flagutils.RootFlagValues

func TestCommandBuildsAuditTaskFromConfiguration(t *testing.T) {
	t.Helper()

	root := "/tmp/audit-root"
	discoverer := &fakeRepositoryDiscoverer{repositories: []string{root}}
	executor := &stubGitExecutor{}
	manager := stubGitRepositoryManager{}
	runner := &recordingTaskRunner{}

	builder := cli.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     discoverer,
		GitExecutor:    executor,
		GitManager:     manager,
		ConfigurationProvider: func() audit.CommandConfiguration {
			return audit.CommandConfiguration{Roots: []string{root}}
		},
		TaskRunnerFactory: func(workflow.Dependencies) cli.TaskRunnerExecutor { return runner },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindRootAndExecutionFlags(command)

	command.SetContext(context.Background())
	command.SetArgs([]string{})

	executionError := command.Execute()
	require.NoError(t, executionError)

	require.Len(t, runner.definitions, 1)
	require.Len(t, runner.definitions[0].Actions, 1)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, "audit.report", action.Type)
	require.Equal(t, false, action.Options["include_all"])
	require.Equal(t, false, action.Options["debug"])
}

func TestCommandFlagsOverrideConfiguration(t *testing.T) {
	t.Helper()

	configuredRoot := "/tmp/configured"
	flagRoot := "/tmp/flagged"

	discoverer := &fakeRepositoryDiscoverer{repositories: []string{flagRoot}}
	executor := &stubGitExecutor{}
	manager := stubGitRepositoryManager{}
	runner := &recordingTaskRunner{}

	builder := cli.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     discoverer,
		GitExecutor:    executor,
		GitManager:     manager,
		ConfigurationProvider: func() audit.CommandConfiguration {
			return audit.CommandConfiguration{Roots: []string{configuredRoot}}
		},
		TaskRunnerFactory: func(workflow.Dependencies) cli.TaskRunnerExecutor { return runner },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindRootAndExecutionFlags(command)

	command.SetContext(context.Background())
	command.SetArgs([]string{
		rootFlagArgumentConstant, flagRoot,
		includeAllFlagArgumentConstant,
	})

	executionError := command.Execute()
	require.NoError(t, executionError)

	require.Len(t, runner.definitions, 1)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, "audit.report", action.Type)
	require.Equal(t, true, action.Options["include_all"])
}

func TestCommandDisplaysHelpWhenRootsMissing(t *testing.T) {
	t.Helper()

	builder := cli.CommandBuilder{
		LoggerProvider:        func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() audit.CommandConfiguration { return audit.CommandConfiguration{} },
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
	require.Contains(t, executionError.Error(), "no repository roots provided")
	require.Contains(t, outputBuffer.String(), command.UseLine())
}

type recordingTaskRunner struct {
	definitions    []workflow.TaskDefinition
	runtimeOptions workflow.RuntimeOptions
}

func (runner *recordingTaskRunner) Run(_ context.Context, _ []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	runner.definitions = append([]workflow.TaskDefinition{}, definitions...)
	runner.runtimeOptions = options
	return workflow.ExecutionOutcome{}, nil
}

type fakeRepositoryDiscoverer struct {
	repositories []string
}

func (discoverer *fakeRepositoryDiscoverer) DiscoverRepositories([]string) ([]string, error) {
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

func bindRootAndExecutionFlags(command *cobra.Command) {
	values := flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Name: flagutils.DefaultRootFlagName, Enabled: true})
	boundRootFlagValues = append(boundRootFlagValues, values)
	flagutils.BindExecutionFlags(
		command,
		flagutils.ExecutionDefaults{},
		flagutils.ExecutionFlagDefinitions{
			AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Shorthand: flagutils.AssumeYesFlagShorthand, Enabled: true},
		},
	)
}
