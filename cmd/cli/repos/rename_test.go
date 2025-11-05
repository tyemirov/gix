package repos_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repos "github.com/tyemirov/gix/cmd/cli/repos"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/utils"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	renameDryRunFlagConstant        = "--" + flagutils.DryRunFlagName
	renameAssumeYesFlagConstant     = "--" + flagutils.AssumeYesFlagName
	renameRequireCleanFlagConstant  = "--require-clean"
	renameIncludeOwnerFlagConstant  = "--owner"
	renameRootFlagConstant          = "--" + flagutils.DefaultRootFlagName
	renameConfiguredRootConstant    = "/tmp/rename-config-root"
	renameCLIRepositoryRootConstant = "/tmp/rename-cli-root"
	renameDiscoveredRepositoryPath  = "/tmp/rename-repo"
	renameMissingRootsMessage       = "no repository roots provided; specify --roots or configure defaults"
	renameRelativeRootConstant      = "relative/rename-root"
	renameHomeRootSuffixConstant    = "rename-home-root"
)

type renameRecordingTaskRunner struct {
	roots          []string
	definitions    []workflow.TaskDefinition
	runtimeOptions workflow.RuntimeOptions
}

func (runner *renameRecordingTaskRunner) Run(_ context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) error {
	runner.roots = append([]string{}, roots...)
	runner.definitions = append([]workflow.TaskDefinition{}, definitions...)
	runner.runtimeOptions = options
	return nil
}

func TestRenameCommandConfigurationPrecedence(testInstance *testing.T) {
	testCases := []struct {
		name                   string
		configuration          *repos.RenameConfiguration
		arguments              []string
		discoveredRepositories []string
		expectedRoots          []string
		expectedRootsBuilder   func(testing.TB) []string
		expectError            bool
		expectedErrorMessage   string
		expectTaskInvocation   bool
		expectedDryRun         bool
		expectedAssumeYes      bool
		expectedRequireClean   bool
		expectedIncludeOwner   bool
	}{
		{
			name: "configuration_supplies_defaults",
			configuration: &repos.RenameConfiguration{
				DryRun:               true,
				AssumeYes:            true,
				RequireCleanWorktree: false,
				RepositoryRoots:      []string{renameConfiguredRootConstant},
			},
			expectedRoots:        []string{renameConfiguredRootConstant},
			expectTaskInvocation: true,
			expectedDryRun:       true,
			expectedAssumeYes:    true,
			expectedRequireClean: false,
			expectedIncludeOwner: false,
		},
		{
			name: "flags_override_configuration",
			configuration: &repos.RenameConfiguration{
				DryRun:               false,
				AssumeYes:            false,
				RequireCleanWorktree: false,
				RepositoryRoots:      []string{renameConfiguredRootConstant},
			},
			arguments: []string{
				renameDryRunFlagConstant,
				renameAssumeYesFlagConstant,
				renameRequireCleanFlagConstant,
				renameIncludeOwnerFlagConstant,
				renameRootFlagConstant, renameCLIRepositoryRootConstant,
			},
			expectedRoots:        []string{renameCLIRepositoryRootConstant},
			expectTaskInvocation: true,
			expectedDryRun:       true,
			expectedAssumeYes:    true,
			expectedRequireClean: true,
			expectedIncludeOwner: true,
		},
		{
			name:                 "error_when_roots_missing",
			configuration:        &repos.RenameConfiguration{},
			expectError:          true,
			expectedErrorMessage: renameMissingRootsMessage,
		},
		{
			name: "configuration_expands_home_relative_root",
			configuration: &repos.RenameConfiguration{
				DryRun:               true,
				AssumeYes:            true,
				RequireCleanWorktree: true,
				RepositoryRoots:      []string{"~/" + renameHomeRootSuffixConstant},
			},
			expectedRootsBuilder: func(testingInstance testing.TB) []string {
				homeDirectory, homeError := os.UserHomeDir()
				require.NoError(testingInstance, homeError)
				return []string{filepath.Join(homeDirectory, renameHomeRootSuffixConstant)}
			},
			expectTaskInvocation: true,
			expectedDryRun:       true,
			expectedAssumeYes:    true,
			expectedRequireClean: true,
			expectedIncludeOwner: false,
		},
		{
			name: "arguments_preserve_relative_root",
			configuration: &repos.RenameConfiguration{
				RepositoryRoots: []string{},
			},
			arguments: []string{
				renameDryRunFlagConstant,
				renameAssumeYesFlagConstant,
				renameRootFlagConstant, renameRelativeRootConstant,
			},
			expectedRoots:        []string{renameRelativeRootConstant},
			expectTaskInvocation: true,
			expectedDryRun:       true,
			expectedAssumeYes:    true,
			expectedRequireClean: false,
			expectedIncludeOwner: false,
		},
		{
			name: "configuration_enables_include_owner",
			configuration: &repos.RenameConfiguration{
				DryRun:               false,
				AssumeYes:            true,
				RequireCleanWorktree: false,
				IncludeOwner:         true,
				RepositoryRoots:      []string{renameConfiguredRootConstant},
			},
			expectedRoots:        []string{renameConfiguredRootConstant},
			expectTaskInvocation: true,
			expectedDryRun:       false,
			expectedAssumeYes:    true,
			expectedRequireClean: false,
			expectedIncludeOwner: true,
		},
		{
			name: "flag_disables_include_owner",
			configuration: &repos.RenameConfiguration{
				DryRun:               false,
				AssumeYes:            true,
				RequireCleanWorktree: false,
				IncludeOwner:         true,
				RepositoryRoots:      []string{renameConfiguredRootConstant},
			},
			arguments: []string{
				renameAssumeYesFlagConstant,
				renameIncludeOwnerFlagConstant + "=false",
			},
			expectedRoots:        []string{renameConfiguredRootConstant},
			expectTaskInvocation: true,
			expectedDryRun:       false,
			expectedAssumeYes:    true,
			expectedRequireClean: false,
			expectedIncludeOwner: false,
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			repositories := testCase.discoveredRepositories
			if len(repositories) == 0 {
				repositories = []string{renameDiscoveredRepositoryPath}
			}
			discoverer := &fakeRepositoryDiscoverer{repositories: repositories}
			executor := &fakeGitExecutor{}
			manager := &fakeGitRepositoryManager{
				remoteURL:                  "",
				currentBranch:              "",
				cleanWorktree:              true,
				cleanWorktreeSet:           true,
				panicOnCurrentBranchLookup: true,
			}
			prompter := &recordingPrompter{result: shared.ConfirmationResult{Confirmed: true}}
			runner := &renameRecordingTaskRunner{}

			var configurationProvider func() repos.RenameConfiguration
			if testCase.configuration != nil {
				configurationCopy := *testCase.configuration
				configurationProvider = func() repos.RenameConfiguration {
					return configurationCopy
				}
			}

			builder := repos.RenameCommandBuilder{
				LoggerProvider: func() *zap.Logger { return zap.NewNop() },
				Discoverer:     discoverer,
				GitExecutor:    executor,
				GitManager:     manager,
				PrompterFactory: func(*cobra.Command) shared.ConfirmationPrompter {
					return prompter
				},
				HumanReadableLoggingProvider: func() bool { return false },
				ConfigurationProvider:        configurationProvider,
				TaskRunnerFactory: func(workflow.Dependencies) repos.TaskRunnerExecutor {
					return runner
				},
			}

			command, buildError := builder.Build()
			require.NoError(subtest, buildError)
			bindGlobalRenameFlags(command)

			command.SetContext(context.Background())
			normalizedArguments := flagutils.NormalizeToggleArguments(testCase.arguments)
			command.SetArgs(normalizedArguments)

			executionError := command.Execute()
			if testCase.expectError {
				require.Error(subtest, executionError)
				require.Equal(subtest, testCase.expectedErrorMessage, executionError.Error())
				require.Empty(subtest, runner.definitions)
				return
			}

			require.NoError(subtest, executionError)

			if testCase.expectTaskInvocation {
				require.Len(subtest, runner.definitions, 1)
				require.Equal(subtest, "Rename repository directories", runner.definitions[0].Name)
				require.Len(subtest, runner.definitions[0].Actions, 1)
				action := runner.definitions[0].Actions[0]
				require.Equal(subtest, "repo.folder.rename", action.Type)
				require.Equal(subtest, testCase.expectedRequireClean, action.Options["require_clean"])
				require.Equal(subtest, testCase.expectedIncludeOwner, action.Options["include_owner"])
				require.True(subtest, runner.runtimeOptions.IncludeNestedRepositories)
				require.True(subtest, runner.runtimeOptions.ProcessRepositoriesByDescendingDepth)
				require.Equal(subtest, testCase.expectedRequireClean, runner.runtimeOptions.CaptureInitialWorktreeStatus)
				require.Equal(subtest, testCase.expectedDryRun, runner.runtimeOptions.DryRun)
				require.Equal(subtest, testCase.expectedAssumeYes, runner.runtimeOptions.AssumeYes)
			} else {
				require.Empty(subtest, runner.definitions)
			}

			expectedRoots := testCase.expectedRoots
			if testCase.expectedRootsBuilder != nil {
				expectedRoots = testCase.expectedRootsBuilder(subtest)
			}
			if len(expectedRoots) > 0 {
				require.Equal(subtest, expectedRoots, runner.roots)
			}
		})
	}
}

func bindGlobalRenameFlags(command *cobra.Command) {
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	flagutils.BindExecutionFlags(command, flagutils.ExecutionDefaults{}, flagutils.ExecutionFlagDefinitions{
		DryRun:    flagutils.ExecutionFlagDefinition{Name: flagutils.DryRunFlagName, Usage: flagutils.DryRunFlagUsage, Enabled: true},
		AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Shorthand: flagutils.AssumeYesFlagShorthand, Enabled: true},
	})
	command.PersistentFlags().String(flagutils.RemoteFlagName, "", flagutils.RemoteFlagUsage)
	command.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		contextAccessor := utils.NewCommandContextAccessor()
		executionFlags := utils.ExecutionFlags{}
		if dryRunValue, dryRunChanged, dryRunError := flagutils.BoolFlag(cmd, flagutils.DryRunFlagName); dryRunError == nil {
			executionFlags.DryRun = dryRunValue
			executionFlags.DryRunSet = dryRunChanged
		}
		if assumeYesValue, assumeYesChanged, assumeYesError := flagutils.BoolFlag(cmd, flagutils.AssumeYesFlagName); assumeYesError == nil {
			executionFlags.AssumeYes = assumeYesValue
			executionFlags.AssumeYesSet = assumeYesChanged
		}
		if remoteValue, remoteChanged, remoteError := flagutils.StringFlag(cmd, flagutils.RemoteFlagName); remoteError == nil {
			executionFlags.Remote = strings.TrimSpace(remoteValue)
			executionFlags.RemoteSet = remoteChanged && len(strings.TrimSpace(remoteValue)) > 0
		}
		updatedContext := contextAccessor.WithExecutionFlags(cmd.Context(), executionFlags)
		cmd.SetContext(updatedContext)
		return nil
	}
}
