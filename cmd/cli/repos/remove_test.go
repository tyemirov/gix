package repos_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repos "github.com/tyemirov/gix/cmd/cli/repos"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/workflow"
)

type recordingHistoryTaskRunner struct {
	roots          []string
	definitions    []workflow.TaskDefinition
	runtimeOptions workflow.RuntimeOptions
	invocations    int
}

func (runner *recordingHistoryTaskRunner) Run(_ context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) error {
	runner.invocations++
	runner.roots = append([]string{}, roots...)
	runner.definitions = append([]workflow.TaskDefinition{}, definitions...)
	runner.runtimeOptions = options
	return nil
}

func bindGlobalRemoveFlags(command *cobra.Command) {
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	flagutils.BindExecutionFlags(
		command,
		flagutils.ExecutionDefaults{},
		flagutils.ExecutionFlagDefinitions{
			DryRun:    flagutils.ExecutionFlagDefinition{Name: flagutils.DryRunFlagName, Usage: flagutils.DryRunFlagUsage, Enabled: true},
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
		expectDryRun         bool
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
				DryRun:          true,
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
			expectDryRun:         true,
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
				DryRun:          false,
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
				"--dry-run", "yes",
				"config.yml",
			},
			expectedRoots:        []string{flagRoot},
			expectedPaths:        []string{"config.yml"},
			expectDryRun:         true,
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
			runner := &recordingHistoryTaskRunner{}

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
				TaskRunnerFactory: func(workflow.Dependencies) repos.TaskRunnerExecutor {
					return runner
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
				require.Equal(subtest, 1, runner.invocations)
				require.Equal(subtest, testCase.expectedRoots, runner.roots)
				require.Len(subtest, runner.definitions, 1)

				task := runner.definitions[0]
				require.Equal(subtest, "Remove repository history paths", task.Name)
				require.True(subtest, task.EnsureClean)
				require.Len(subtest, task.Actions, 1)

				action := task.Actions[0]
				require.Equal(subtest, "repo.history.purge", action.Type)

				pathsValue, ok := action.Options["paths"].([]string)
				require.True(subtest, ok)
				require.ElementsMatch(subtest, testCase.expectedPaths, pathsValue)

				remoteValue, remoteExists := action.Options["remote"].(string)
				if remoteExists {
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

				require.Equal(subtest, testCase.expectDryRun, runner.runtimeOptions.DryRun)
				require.Equal(subtest, testCase.expectAssumeYes, runner.runtimeOptions.AssumeYes)
			} else {
				require.Equal(subtest, 0, runner.invocations)
			}
		})
	}
}
