package repos_test

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repos "github.com/temirov/gix/cmd/cli/repos"
	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
)

func bindGlobalRemoveFlags(command *cobra.Command) {
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	flagutils.BindExecutionFlags(
		command,
		flagutils.ExecutionDefaults{},
		flagutils.ExecutionFlagDefinitions{
			AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Enabled: true},
		},
	)
}

func TestRemoveCommandConfigurationPrecedence(testInstance *testing.T) {
	const (
		configuredRoot = "/tmp/history-config-root"
		flagRoot       = "/tmp/history-cli-root"
	)

	presetConfig := loadHistoryRemovePreset(testInstance)

	testCases := []struct {
		name                 string
		configuration        repos.RemoveConfiguration
		arguments            []string
		expectedRoots        []string
		expectedPaths        []string
		expectAssumeYes      bool
		expectedRemote       string
		expectPush           bool
		expectRestore        bool
		expectPushMissing    bool
		expectTaskInvocation bool
	}{
		{
			name: "configuration_applies_without_flags",
			configuration: repos.RemoveConfiguration{
				AssumeYes:       false,
				RepositoryRoots: []string{configuredRoot},
				Remote:          "origin",
				Push:            false,
				Restore:         true,
				PushMissing:     false,
			},
			arguments:            []string{"secrets.txt", "./nested/creds.env"},
			expectedRoots:        []string{configuredRoot},
			expectedPaths:        []string{"secrets.txt", "nested/creds.env"},
			expectAssumeYes:      false,
			expectedRemote:       "origin",
			expectPush:           false,
			expectRestore:        true,
			expectPushMissing:    false,
			expectTaskInvocation: true,
		},
		{
			name: "flags_override_configuration",
			configuration: repos.RemoveConfiguration{
				AssumeYes:       false,
				RepositoryRoots: []string{configuredRoot},
				Remote:          "",
				Push:            true,
				Restore:         true,
				PushMissing:     false,
			},
			arguments: []string{
				"--roots", flagRoot,
				"--remote", "upstream",
				"--push", "no",
				"--restore", "no",
				"--push-missing", "yes",
				"config.yml",
			},
			expectedRoots:        []string{flagRoot},
			expectedPaths:        []string{"config.yml"},
			expectAssumeYes:      false,
			expectedRemote:       "upstream",
			expectPush:           false,
			expectRestore:        false,
			expectPushMissing:    true,
			expectTaskInvocation: true,
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			discoverer := &fakeRepositoryDiscoverer{repositories: []string{configuredRoot}}
			executor := &fakeGitExecutor{}
			manager := &fakeGitRepositoryManager{}
			catalog := &fakePresetCatalog{configuration: presetConfig, found: true}
			recording := &historyRecordingExecutor{}

			configCopy := testCase.configuration
			builder := repos.RemoveCommandBuilder{
				LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
				Discoverer:                   discoverer,
				GitExecutor:                  executor,
				GitManager:                   manager,
				HumanReadableLoggingProvider: func() bool { return false },
				ConfigurationProvider: func() repos.RemoveConfiguration {
					return configCopy
				},
				PresetCatalogFactory: func() workflowcmd.PresetCatalog { return catalog },
				WorkflowExecutorFactory: func(nodes []*workflow.OperationNode, _ workflow.Dependencies) repos.WorkflowExecutor {
					if !testCase.expectTaskInvocation {
						return recording
					}
					require.Len(subtest, nodes, 1)
					taskOperation, ok := nodes[0].Operation.(*workflow.TaskOperation)
					require.True(subtest, ok)
					taskDefinitions := taskOperation.Definitions()
					require.Len(subtest, taskDefinitions, 1)
					task := taskDefinitions[0]
					require.True(subtest, task.EnsureClean)
					require.Len(subtest, task.Actions, 1)
					action := task.Actions[0]
					require.Equal(subtest, "repo.history.purge", action.Type)
					pathsValue, ok := action.Options["paths"].([]string)
					require.True(subtest, ok)
					require.ElementsMatch(subtest, testCase.expectedPaths, pathsValue)

					if remoteValue, exists := action.Options["remote"].(string); exists {
						require.Equal(subtest, testCase.expectedRemote, remoteValue)
					} else {
						require.Empty(subtest, testCase.expectedRemote)
					}

					pushValue, ok := action.Options["push"].(bool)
					require.True(subtest, ok)
					require.Equal(subtest, testCase.expectPush, pushValue)

					restoreValue, ok := action.Options["restore"].(bool)
					require.True(subtest, ok)
					require.Equal(subtest, testCase.expectRestore, restoreValue)

					pushMissingValue, ok := action.Options["push_missing"].(bool)
					require.True(subtest, ok)
					require.Equal(subtest, testCase.expectPushMissing, pushMissingValue)
					return recording
				},
			}

			command, buildError := builder.Build()
			require.NoError(subtest, buildError)
			bindGlobalRemoveFlags(command)

			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			command.SetContext(context.Background())
			command.SetArgs(flagutils.NormalizeToggleArguments(testCase.arguments))

			executionError := command.Execute()
			require.NoError(subtest, executionError)

			if testCase.expectTaskInvocation {
				require.Equal(subtest, testCase.expectedRoots, recording.roots)
				require.Equal(subtest, testCase.expectAssumeYes, recording.options.AssumeYes)
			} else {
				require.Nil(subtest, recording.roots)
			}
		})
	}
}

func TestRemoveCommandPresetErrorsSurface(testInstance *testing.T) {
	presetConfig := loadHistoryRemovePreset(testInstance)

	testCases := []struct {
		name      string
		catalog   fakePresetCatalog
		expectErr string
	}{
		{
			name:      "missing_preset",
			catalog:   fakePresetCatalog{found: false},
			expectErr: "history-remove preset not found",
		},
		{
			name:      "load_error",
			catalog:   fakePresetCatalog{found: true, loadError: errors.New("boom")},
			expectErr: "unable to load history-remove preset: boom",
		},
		{
			name: "build_error",
			catalog: fakePresetCatalog{
				found: true,
				configuration: workflow.Configuration{
					Steps: []workflow.StepConfiguration{
						{Command: []string{"unknown"}},
					},
				},
			},
			expectErr: "unable to build history-remove workflow: unsupported workflow command: unknown",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			catalog := testCase.catalog
			if len(catalog.configuration.Steps) == 0 && catalog.found && catalog.loadError == nil && testCase.name != "build_error" {
				catalog.configuration = presetConfig
			}

			builder := repos.RemoveCommandBuilder{
				LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
				Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{"/tmp/history-config-root"}},
				GitExecutor:                  &fakeGitExecutor{},
				GitManager:                   &fakeGitRepositoryManager{},
				FileSystem:                   fakeFileSystem{files: map[string]string{}},
				HumanReadableLoggingProvider: func() bool { return false },
				ConfigurationProvider: func() repos.RemoveConfiguration {
					return repos.RemoveConfiguration{
						RepositoryRoots: []string{"/tmp/history-config-root"},
						Remote:          "origin",
						Push:            true,
						Restore:         true,
						PushMissing:     false,
					}
				},
				PresetCatalogFactory: func() workflowcmd.PresetCatalog { return &catalog },
			}

			command, buildError := builder.Build()
			require.NoError(subtest, buildError)
			bindGlobalRemoveFlags(command)
			command.SetArgs([]string{"secrets.txt"})

			err := command.Execute()
			require.EqualError(subtest, err, testCase.expectErr)
		})
	}
}

type historyRecordingExecutor struct {
	roots   []string
	options workflow.RuntimeOptions
}

func (executor *historyRecordingExecutor) Execute(_ context.Context, roots []string, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	executor.roots = append([]string{}, roots...)
	executor.options = options
	return workflow.ExecutionOutcome{}, nil
}

func loadHistoryRemovePreset(testingInstance testing.TB) workflow.Configuration {
	presetContent := []byte(`workflow:
  - step:
      name: history-remove
      command: ["tasks", "apply"]
      with:
        tasks:
          - name: Remove repository history paths
            ensure_clean: true
            actions:
              - type: repo.history.purge
                options:
                  paths: []
                  remote: ""
                  push: true
                  restore: true
                  push_missing: false
`)
	configuration, err := workflow.ParseConfiguration(presetContent)
	require.NoError(testingInstance, err)
	return configuration
}
