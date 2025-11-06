package release

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repocli "github.com/temirov/gix/cmd/cli/repos"
	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/utils"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
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
	require.False(t, runner.runtimeOptions.AssumeYes)
}

func TestRetagCommandRequiresMappings(t *testing.T) {
	builder := RetagCommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() CommandConfiguration {
			return CommandConfiguration{}
		},
		GitExecutor: &stubGitExecutor{},
	}
	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	require.Error(t, command.RunE(command, nil))
}

func TestRetagCommandBuildsMappings(t *testing.T) {
	root := t.TempDir()
	runner := &recordingTaskRunner{}
	builder := RetagCommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() CommandConfiguration {
			return CommandConfiguration{RepositoryRoots: []string{root}, RemoteName: "origin", Message: "Retag {{tag}} -> {{target}}"}
		},
		GitExecutor: &stubGitExecutor{},
		TaskRunnerFactory: func(workflow.Dependencies) repocli.TaskRunnerExecutor {
			return runner
		},
	}
	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	flagutils.BindExecutionFlags(command, flagutils.ExecutionDefaults{}, flagutils.ExecutionFlagDefinitions{
		AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Shorthand: flagutils.AssumeYesFlagShorthand, Enabled: true},
	})

	args := []string{
		"--" + retagMappingFlagName, "v1.0.0=main",
		"--" + retagMappingFlagName, "v1.1.0=feature",
	}
	command.SetArgs(args)
	command.SetContext(context.Background())
	require.NoError(t, command.Execute())

	require.Equal(t, []string{root}, runner.roots)
	require.Len(t, runner.definitions, 1)
	actionDefinitions := runner.definitions[0].Actions
	require.Len(t, actionDefinitions, 1)
	action := actionDefinitions[0]
	require.Equal(t, "repo.release.retag", action.Type)

	mappingValue, ok := action.Options["mappings"].([]any)
	require.True(t, ok)
	require.Len(t, mappingValue, 2)
	firstMapping, ok := mappingValue[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "v1.0.0", firstMapping["tag"])
	require.Equal(t, "main", firstMapping["target"])
	require.Equal(t, "Retag v1.0.0 -> main", firstMapping["message"])
	require.Equal(t, "origin", action.Options["remote"])
}
