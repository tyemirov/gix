package packages_test

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/ghcr"
	packages "github.com/temirov/gix/internal/packages"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
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
	executor packages.PurgeExecutor
	err      error
}

func (resolver stubServiceResolver) Resolve(*zap.Logger) (packages.PurgeExecutor, error) {
	return resolver.executor, resolver.err
}

type stubPurgeExecutor struct{}

func (stubPurgeExecutor) Execute(context.Context, packages.PurgeOptions) (ghcr.PurgeResult, error) {
	return ghcr.PurgeResult{}, nil
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
	service := stubPurgeExecutor{}
	resolver := stubServiceResolver{executor: service}
	metadataResolver := stubMetadataResolver{}

	builder := packages.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() packages.Configuration {
			return packages.Configuration{Purge: packages.PurgeConfiguration{RepositoryRoots: []string{"/src"}}}
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

	err = command.Execute()
	require.NoError(t, err)

	require.Equal(t, []string{"/src"}, runner.roots)
	require.Len(t, runner.definitions, 1)
	require.Len(t, runner.definitions[0].Actions, 1)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, "repo.packages.purge", action.Type)
	require.Equal(t, "", action.Options["package_override"])
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
			return packages.Configuration{Purge: packages.PurgeConfiguration{RepositoryRoots: []string{"/workspace"}}}
		},
		ServiceResolver:            stubServiceResolver{executor: stubPurgeExecutor{}},
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
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)

	err = command.Execute()
	require.NoError(t, err)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, "custom", action.Options["package_override"])
}
