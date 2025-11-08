package repos_test

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repos "github.com/temirov/gix/cmd/cli/repos"
	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/repos/filesystem"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
)

type namespaceRecordingExecutor struct {
	roots   []string
	options workflow.RuntimeOptions
}

func (executor *namespaceRecordingExecutor) Execute(_ context.Context, roots []string, options workflow.RuntimeOptions) error {
	executor.roots = append([]string{}, roots...)
	executor.options = options
	return nil
}

type namespacePresetCatalog struct {
	configuration workflow.Configuration
}

func (catalog *namespacePresetCatalog) List() []workflowcmd.PresetMetadata {
	return nil
}

func (catalog *namespacePresetCatalog) Load(name string) (workflow.Configuration, bool, error) {
	return catalog.configuration, true, nil
}

func minimalNamespaceWorkflow() workflow.Configuration {
	return workflow.Configuration{
		Steps: []workflow.StepConfiguration{
			{
				Name:    "namespace-rewrite",
				Command: []string{"tasks", "apply"},
				Options: map[string]any{
					"tasks": []any{
						map[string]any{
							"name":         "Rewrite Go namespace",
							"ensure_clean": false,
							"actions": []any{
								map[string]any{
									"type": "repo.namespace.rewrite",
									"options": map[string]any{
										"old":            "{{ .Environment.namespace_old }}",
										"new":            "{{ .Environment.namespace_new }}",
										"push":           "{{ if .Environment.namespace_push }}{{ .Environment.namespace_push }}{{ else }}true{{ end }}",
										"branch_prefix":  "{{ if .Environment.namespace_branch_prefix }}{{ .Environment.namespace_branch_prefix }}{{ else }}namespace-rewrite{{ end }}",
										"remote":         "{{ if .Environment.namespace_remote }}{{ .Environment.namespace_remote }}{{ else }}origin{{ end }}",
										"commit_message": "{{ .Environment.namespace_commit_message }}",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func extractNamespaceSafeguards(t *testing.T, configuration workflow.Configuration) map[string]any {
	t.Helper()

	if len(configuration.Steps) == 0 {
		return nil
	}

	tasksValue := configuration.Steps[0].Options["tasks"]
	taskEntries, ok := tasksValue.([]any)
	require.True(t, ok)
	require.NotEmpty(t, taskEntries)

	taskEntry, ok := taskEntries[0].(map[string]any)
	require.True(t, ok)

	actionsValue := taskEntry["actions"]
	actionEntries, ok := actionsValue.([]any)
	require.True(t, ok)
	require.NotEmpty(t, actionEntries)

	actionEntry, ok := actionEntries[0].(map[string]any)
	require.True(t, ok)

	optionsValue := actionEntry["options"]
	options, ok := optionsValue.(map[string]any)
	require.True(t, ok)

	rawSafeguards, hasSafeguards := options["safeguards"]
	if !hasSafeguards {
		return nil
	}
	safeguards, ok := rawSafeguards.(map[string]any)
	require.True(t, ok)
	return safeguards
}

func TestNamespaceCommandUsesConfigurationDefaults(t *testing.T) {
	t.Parallel()

	executor := &namespaceRecordingExecutor{}
	presetCatalog := &namespacePresetCatalog{configuration: minimalNamespaceWorkflow()}

	builder := repos.NamespaceCommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     &fakeRepositoryDiscoverer{repositories: []string{"/tmp/cfg-root"}},
		GitExecutor:    &fakeGitExecutor{},
		GitManager:     &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:     filesystem.OSFileSystem{},
		ConfigurationProvider: func() repos.NamespaceConfiguration {
			return repos.NamespaceConfiguration{
				AssumeYes:       true,
				RepositoryRoots: []string{"/tmp/cfg-root"},
				OldPrefix:       "github.com/old/org",
				NewPrefix:       "github.com/new/org",
				Push:            true,
				Remote:          "origin",
				BranchPrefix:    "ns",
				Safeguards:      map[string]any{"require_clean": true},
			}
		},
		PresetCatalogFactory: func() workflowcmd.PresetCatalog {
			return presetCatalog
		},
		ExecutorFactory: func(nodes []*workflow.OperationNode, deps workflow.Dependencies) repos.WorkflowExecutor {
			require.NotEmpty(t, nodes)
			require.NotNil(t, deps.Prompter)
			return executor
		},
	}

	command, err := builder.Build()
	require.NoError(t, err)
	bindGlobalNamespaceFlags(command)

	command.SetContext(context.Background())
	command.SetArgs(nil)

	runErr := command.Execute()
	require.NoError(t, runErr)

	require.Equal(t, []string{"/tmp/cfg-root"}, executor.roots)
	require.True(t, executor.options.AssumeYes)
	require.Equal(t, "github.com/old/org", executor.options.Variables["namespace_old"])
	require.Equal(t, "github.com/new/org", executor.options.Variables["namespace_new"])
	require.Equal(t, "true", executor.options.Variables["namespace_push"])
	require.Equal(t, "ns", executor.options.Variables["namespace_branch_prefix"])
	require.Equal(t, "origin", executor.options.Variables["namespace_remote"])
	_, commitExists := executor.options.Variables["namespace_commit_message"]
	require.False(t, commitExists)

	require.Equal(t, map[string]any{"require_clean": true}, extractNamespaceSafeguards(t, presetCatalog.configuration))
}

func TestNamespaceCommandFlagOverrides(t *testing.T) {
	t.Parallel()

	executor := &namespaceRecordingExecutor{}
	presetCatalog := &namespacePresetCatalog{configuration: minimalNamespaceWorkflow()}

	builder := repos.NamespaceCommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     &fakeRepositoryDiscoverer{repositories: []string{"/tmp/cli-root"}},
		GitExecutor:    &fakeGitExecutor{},
		GitManager:     &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:     filesystem.OSFileSystem{},
		ConfigurationProvider: func() repos.NamespaceConfiguration {
			return repos.NamespaceConfiguration{}
		},
		PresetCatalogFactory: func() workflowcmd.PresetCatalog {
			return presetCatalog
		},
		ExecutorFactory: func(nodes []*workflow.OperationNode, deps workflow.Dependencies) repos.WorkflowExecutor {
			return executor
		},
	}

	command, err := builder.Build()
	require.NoError(t, err)
	bindGlobalNamespaceFlags(command)

	args := flagutils.NormalizeToggleArguments([]string{
		"--" + flagutils.AssumeYesFlagName,
		"--" + flagutils.DefaultRootFlagName, "/tmp/cli-root",
		"--old", "github.com/cli/old",
		"--new", "github.com/cli/new",
		"--branch-prefix", "rewrite",
		"--remote", "upstream",
		"--commit-message", "chore: rewrite",
		"--push=false",
	})
	command.SetContext(context.Background())
	command.SetArgs(args)

	runErr := command.Execute()
	require.NoError(t, runErr)

	require.Equal(t, []string{"/tmp/cli-root"}, executor.roots)
	require.True(t, executor.options.AssumeYes)
	require.Equal(t, "github.com/cli/old", executor.options.Variables["namespace_old"])
	require.Equal(t, "github.com/cli/new", executor.options.Variables["namespace_new"])
	require.Equal(t, "false", executor.options.Variables["namespace_push"])
	require.Equal(t, "rewrite", executor.options.Variables["namespace_branch_prefix"])
	require.Equal(t, "upstream", executor.options.Variables["namespace_remote"])
	require.Equal(t, "chore: rewrite", executor.options.Variables["namespace_commit_message"])
}

func TestNamespaceCommandRequiresPrefixes(t *testing.T) {
	t.Parallel()

	executor := &namespaceRecordingExecutor{}
	presetCatalog := &namespacePresetCatalog{configuration: minimalNamespaceWorkflow()}

	builder := repos.NamespaceCommandBuilder{
		LoggerProvider:        func() *zap.Logger { return zap.NewNop() },
		Discoverer:            &fakeRepositoryDiscoverer{repositories: []string{"/tmp/root"}},
		GitExecutor:           &fakeGitExecutor{},
		GitManager:            &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:            filesystem.OSFileSystem{},
		ConfigurationProvider: func() repos.NamespaceConfiguration { return repos.NamespaceConfiguration{} },
		PresetCatalogFactory: func() workflowcmd.PresetCatalog {
			return presetCatalog
		},
		ExecutorFactory: func(nodes []*workflow.OperationNode, deps workflow.Dependencies) repos.WorkflowExecutor {
			return executor
		},
	}

	command, err := builder.Build()
	require.NoError(t, err)
	bindGlobalNamespaceFlags(command)

	command.SetContext(context.Background())
	command.SetArgs(nil)

	runErr := command.Execute()
	require.Error(t, runErr)
}

func bindGlobalNamespaceFlags(command *cobra.Command) {
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Name: flagutils.DefaultRootFlagName, Usage: flagutils.DefaultRootFlagUsage, Enabled: true, Persistent: false})
	flagutils.BindExecutionFlags(command, flagutils.ExecutionDefaults{}, flagutils.ExecutionFlagDefinitions{
		AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Enabled: true},
	})
}
