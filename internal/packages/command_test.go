package packages_test

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/v5/internal/execshell"
	"github.com/tyemirov/gix/v5/internal/ghcr"
	packages "github.com/tyemirov/gix/v5/internal/packages"
	flagutils "github.com/tyemirov/gix/v5/internal/utils/flags"
	"github.com/tyemirov/gix/v5/internal/workflow"
)

type recordingTaskRunner struct {
	dependencies workflow.Dependencies
	roots        []string
	definitions  []workflow.TaskDefinition
	options      workflow.RuntimeOptions
}

func (runner *recordingTaskRunner) Run(ctx context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	runner.roots = append([]string{}, roots...)
	runner.definitions = append([]workflow.TaskDefinition{}, definitions...)
	runner.options = options
	return workflow.ExecutionOutcome{}, nil
}

type stubServiceResolver struct {
	executor packages.RetentionExecutor
	err      error
}

func (resolver stubServiceResolver) Resolve(*zap.Logger) (packages.RetentionExecutor, error) {
	return resolver.executor, resolver.err
}

type stubRetentionExecutor struct{}

func (stubRetentionExecutor) Execute(context.Context, packages.RetentionOptions) (ghcr.RetentionResult, error) {
	return ghcr.RetentionResult{}, nil
}

type stubMetadataResolver struct{}

func (stubMetadataResolver) ResolveMetadata(context.Context, string) (packages.RepositoryMetadata, error) {
	return packages.RepositoryMetadata{Owner: "owner", OwnerType: ghcr.UserOwnerType, DefaultPackageName: "default"}, nil
}

type stubDiscoverer struct{}

func (stubDiscoverer) DiscoverRepositories(roots []string) ([]string, error) {
	return append([]string{}, roots...), nil
}

type stubGitExecutor struct{}

func (stubGitExecutor) ExecuteGit(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func (stubGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func TestCommandBuildsTaskDefinition(t *testing.T) {
	runner := &recordingTaskRunner{}
	service := stubRetentionExecutor{}
	resolver := stubServiceResolver{executor: service}
	metadataResolver := stubMetadataResolver{}

	builder := packages.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() packages.Configuration {
			return packages.Configuration{Delete: packages.DeleteConfiguration{
				RepositoryRoots: []string{"/src"},
				BaseURL:         "https://api.github.test",
				Credential:      "configured-token",
			}}
		},
		ServiceResolver:            resolver,
		RepositoryMetadataResolver: metadataResolver,
		RepositoryDiscoverer:       stubDiscoverer{},
		GitExecutor:                stubGitExecutor{},
		TaskRunnerFactory: func(deps workflow.Dependencies) packages.TaskRunnerExecutor {
			runner.dependencies = deps
			return runner
		},
	}

	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindExecutionFlags(command, flagutils.ExecutionDefaults{}, flagutils.ExecutionFlagDefinitions{})
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Name: flagutils.DefaultRootFlagName, Usage: flagutils.DefaultRootFlagUsage, Enabled: true})
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	command.SetContext(context.Background())
	require.NoError(t, command.Flags().Set("keep", "3"))

	err = command.Execute()
	require.NoError(t, err)

	require.Equal(t, []string{"/src"}, runner.roots)
	require.Len(t, runner.definitions, 1)
	require.Len(t, runner.definitions[0].Actions, 1)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, "repo.packages.retention", action.Type)
	require.Equal(t, "", action.Options["package_override"])
	require.Equal(t, "configured-token", action.Options["credential"])
	require.Equal(t, 3, action.Options["keep_count"].(ghcr.KeepCount).Value())
}

func TestCommandErrorsOnUnexpectedArguments(t *testing.T) {
	builder := packages.CommandBuilder{}
	command, err := builder.Build()
	require.NoError(t, err)
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)

	err = command.RunE(command, []string{"unexpected"})
	require.Error(t, err)
}

func TestCommandHonorsPackageFlag(t *testing.T) {
	runner := &recordingTaskRunner{}
	builder := packages.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() packages.Configuration {
			return packages.Configuration{Delete: packages.DeleteConfiguration{
				RepositoryRoots: []string{"/workspace"},
				BaseURL:         "https://api.github.test",
				Credential:      "configured-token",
			}}
		},
		ServiceResolver:            stubServiceResolver{executor: stubRetentionExecutor{}},
		RepositoryMetadataResolver: stubMetadataResolver{},
		RepositoryDiscoverer:       stubDiscoverer{},
		GitExecutor:                stubGitExecutor{},
		TaskRunnerFactory: func(deps workflow.Dependencies) packages.TaskRunnerExecutor {
			runner.dependencies = deps
			return runner
		},
	}

	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Name: flagutils.DefaultRootFlagName, Usage: flagutils.DefaultRootFlagUsage, Enabled: true})
	require.NoError(t, command.Flags().Set("package", "custom"))
	require.NoError(t, command.Flags().Set("keep", "3"))
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)

	err = command.Execute()
	require.NoError(t, err)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, "custom", action.Options["package_override"])
}
