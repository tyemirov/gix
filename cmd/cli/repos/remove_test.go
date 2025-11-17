package repos_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repos "github.com/temirov/gix/cmd/cli/repos"
	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
)

type historyRecordingExecutor struct {
	roots   []string
	options workflow.RuntimeOptions
}

func (executor *historyRecordingExecutor) Execute(_ context.Context, roots []string, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	executor.roots = append([]string{}, roots...)
	executor.options = options
	return workflow.ExecutionOutcome{}, nil
}

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
			presetConfig := loadHistoryPreset(subtest)
			discoverer := &fakeRepositoryDiscoverer{repositories: []string{configuredRoot}}
			executor := &fakeGitExecutor{}
			manager := &fakeGitRepositoryManager{}
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
				PresetCatalogFactory: func() workflowcmd.PresetCatalog {
					return &fakePresetCatalog{configuration: presetConfig, found: true}
				},
				WorkflowExecutorFactory: func(nodes []*workflow.OperationNode, _ workflow.Dependencies) workflowcmd.OperationExecutor {
					require.Len(subtest, nodes, 1)
					taskOp, ok := nodes[0].Operation.(*workflow.TaskOperation)
					require.True(subtest, ok)
					definitions := taskOp.Definitions()
					require.Len(subtest, definitions, 1)
					task := definitions[0]
					require.True(subtest, task.EnsureClean)
					require.Len(subtest, task.Actions, 1)
					action := task.Actions[0]
					require.Equal(subtest, "repo.history.purge", action.Type)
					actionOptions := action.Options
					require.NotNil(subtest, actionOptions)
					pathsValue, ok := actionOptions["paths"].([]string)
					require.True(subtest, ok)
					require.ElementsMatch(subtest, testCase.expectedPaths, pathsValue)
					if len(testCase.expectedRemote) > 0 {
						require.Equal(subtest, testCase.expectedRemote, actionOptions["remote"])
					} else {
						_, exists := actionOptions["remote"]
						require.False(subtest, exists)
					}
					pushValue, ok := actionOptions["push"].(bool)
					require.True(subtest, ok)
					require.Equal(subtest, testCase.expectPush, pushValue)
					restoreValue, ok := actionOptions["restore"].(bool)
					require.True(subtest, ok)
					require.Equal(subtest, testCase.expectRestore, restoreValue)
					pushMissingValue, ok := actionOptions["push_missing"].(bool)
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
				require.Empty(subtest, recording.roots)
			}
		})
	}
}

func loadHistoryPreset(testingInstance testing.TB) workflow.Configuration {
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
