package workflow

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/temirov/gix/internal/execshell"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	workflowpkg "github.com/temirov/gix/internal/workflow"
)

func TestPresetCommandExecuteSuccess(t *testing.T) {
	t.Helper()

	executor := &stubOperationExecutor{}
	commandHelper := PresetCommand{
		LoggerProvider:       zap.NewNop,
		RepositoryDiscoverer: stubRepositoryDiscoverer{},
		GitExecutor:          stubGitExecutor{},
		GitRepositoryManager: stubGitRepositoryManager{},
		PresetCatalogFactory: func() PresetCatalog {
			return stubPresetCatalog{configuration: buildTestPresetConfiguration()}
		},
		WorkflowExecutorFactory: func(nodes []*workflowpkg.OperationNode, dependencies workflowpkg.Dependencies) OperationExecutor {
			executor.nodes = nodes
			executor.dependencies = dependencies
			return executor
		},
	}

	cobraCommand := &cobra.Command{Use: "test"}
	cobraCommand.Flags().StringSlice(flagutils.DefaultRootFlagName, nil, "roots")

	configureCalled := false
	prepareCalled := false

	request := PresetCommandRequest{
		Command:                 cobraCommand,
		ConfiguredAssumeYes:     true,
		ConfiguredRoots:         []string{"/workspace"},
		PresetName:              "files-add",
		PresetMissingMessage:    "files-add missing",
		PresetLoadErrorTemplate: "load: %w",
		BuildErrorTemplate:      "build: %w",
		Configure: func(ctx PresetCommandContext) (PresetCommandResult, error) {
			require.True(t, ctx.AssumeYes)
			require.Equal(t, []string{"/workspace"}, ctx.Roots)
			require.NotNil(t, ctx.Configuration)
			configureCalled = true

			ctx.Configuration.Steps[0].Options["custom"] = "value"
			runtimeOptions := ctx.RuntimeOptions()
			runtimeOptions.CaptureInitialWorktreeStatus = true

			return PresetCommandResult{
				Configuration:  ctx.Configuration,
				RuntimeOptions: runtimeOptions,
				PrepareOperations: func(nodes []*workflowpkg.OperationNode) error {
					prepareCalled = true
					return nil
				},
			}, nil
		},
	}

	err := commandHelper.Execute(request)
	require.NoError(t, err)
	require.True(t, configureCalled)
	require.True(t, prepareCalled)
	require.True(t, executor.invoked)
	require.Equal(t, []string{"/workspace"}, executor.roots)
	require.True(t, executor.options.CaptureInitialWorktreeStatus)
	require.Len(t, executor.nodes, 1)
}

func TestPresetCommandMissingPreset(t *testing.T) {
	t.Helper()

	commandHelper := PresetCommand{
		PresetCatalogFactory: func() PresetCatalog {
			return stubPresetCatalog{}
		},
	}

	cobraCommand := &cobra.Command{Use: "test"}
	cobraCommand.Flags().StringSlice(flagutils.DefaultRootFlagName, nil, "roots")

	err := commandHelper.Execute(PresetCommandRequest{
		Command:              cobraCommand,
		ConfiguredRoots:      []string{"/workspace"},
		PresetName:           "missing-preset",
		PresetMissingMessage: "preset not found",
	})

	require.Error(t, err)
	require.Equal(t, "preset not found", err.Error())
}

type stubPresetCatalog struct {
	configuration workflowpkg.Configuration
}

func (catalog stubPresetCatalog) List() []PresetMetadata {
	return nil
}

func (catalog stubPresetCatalog) Load(name string) (workflowpkg.Configuration, bool, error) {
	if len(catalog.configuration.Steps) == 0 {
		return workflowpkg.Configuration{}, false, nil
	}
	return catalog.configuration, true, nil
}

type stubOperationExecutor struct {
	invoked      bool
	roots        []string
	options      workflowpkg.RuntimeOptions
	nodes        []*workflowpkg.OperationNode
	dependencies workflowpkg.Dependencies
}

func (executor *stubOperationExecutor) Execute(ctx context.Context, roots []string, options workflowpkg.RuntimeOptions) (workflowpkg.ExecutionOutcome, error) {
	executor.invoked = true
	executor.roots = roots
	executor.options = options
	return workflowpkg.ExecutionOutcome{}, nil
}

type stubRepositoryDiscoverer struct{}

func (stubRepositoryDiscoverer) DiscoverRepositories(roots []string) ([]string, error) {
	return roots, nil
}

type stubGitExecutor struct{}

func (stubGitExecutor) ExecuteGit(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func (stubGitExecutor) ExecuteGitHubCLI(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type stubGitRepositoryManager struct{}

func (stubGitRepositoryManager) CheckCleanWorktree(ctx context.Context, repositoryPath string) (bool, error) {
	return true, nil
}

func (stubGitRepositoryManager) WorktreeStatus(ctx context.Context, repositoryPath string) ([]string, error) {
	return nil, nil
}

func (stubGitRepositoryManager) GetCurrentBranch(ctx context.Context, repositoryPath string) (string, error) {
	return "main", nil
}

func (stubGitRepositoryManager) GetRemoteURL(ctx context.Context, repositoryPath string, remoteName string) (string, error) {
	return "origin", nil
}

func (stubGitRepositoryManager) SetRemoteURL(ctx context.Context, repositoryPath string, remoteName string, remoteURL string) error {
	return nil
}

func buildTestPresetConfiguration() workflowpkg.Configuration {
	return workflowpkg.Configuration{
		Steps: []workflowpkg.StepConfiguration{
			{
				Command: []string{"tasks", "apply"},
				Options: map[string]any{
					"tasks": []any{
						map[string]any{
							"name": "demo task",
							"files": []any{
								map[string]any{
									"path":        "README.md",
									"content":     "hello world",
									"mode":        "overwrite",
									"permissions": 420,
								},
							},
						},
					},
				},
			},
		},
	}
}
