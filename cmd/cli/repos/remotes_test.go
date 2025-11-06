package repos_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repos "github.com/temirov/gix/cmd/cli/repos"
	"github.com/temirov/gix/internal/repos/shared"
	"github.com/temirov/gix/internal/utils"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
)

const (
	remotesAssumeYesFlagConstant     = "--" + flagutils.AssumeYesFlagName
	remotesRootFlagConstant          = "--" + flagutils.DefaultRootFlagName
	remotesConfiguredRootConstant    = "/tmp/remotes-config-root"
	remotesCLIRepositoryRootConstant = "/tmp/remotes-cli-root"
	remotesDiscoveredRepository      = "/tmp/remotes-repo"
	remotesOriginURLConstant         = "https://github.com/origin/example.git"
	remotesCanonicalRepository       = "canonical/example"
	remotesMetadataDefaultBranch     = "main"
	remotesMissingRootsMessage       = "no repository roots provided; specify --roots or configure defaults"
	remotesRelativeRootConstant      = "relative/remotes-root"
	remotesHomeRootSuffixConstant    = "remotes-home-root"
	remotesOwnerFlagConstant         = "--owner"
	remotesOwnerConstraintConstant   = "canonical"
	remotesOwnerMismatchConstant     = "different"
)

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

func TestRemotesCommandConfigurationPrecedence(testInstance *testing.T) {
	testCases := []struct {
		name                    string
		configuration           repos.RemotesConfiguration
		arguments               []string
		expectedRoots           []string
		expectedRootsBuilder    func(testing.TB) []string
		expectPromptInvocations int
		expectError             bool
		expectedErrorMessage    string
		expectedAssumeYes       bool
		expectedOwnerConstraint string
		expectTaskInvocation    bool
	}{
		{
			name: "configuration_uses_defaults",
			configuration: repos.RemotesConfiguration{
				AssumeYes:       false,
				RepositoryRoots: []string{remotesConfiguredRootConstant},
			},
			arguments:               []string{},
			expectedRoots:           []string{remotesConfiguredRootConstant},
			expectPromptInvocations: 0,
			expectedAssumeYes:       false,
			expectTaskInvocation:    true,
		},
		{
			name: "flags_override_configuration",
			configuration: repos.RemotesConfiguration{
				AssumeYes:       false,
				RepositoryRoots: []string{remotesConfiguredRootConstant},
			},
			arguments: []string{
				remotesAssumeYesFlagConstant,
				remotesRootFlagConstant, remotesCLIRepositoryRootConstant,
			},
			expectedRoots:           []string{remotesCLIRepositoryRootConstant},
			expectPromptInvocations: 0,
			expectedAssumeYes:       true,
			expectTaskInvocation:    true,
		},
		{
			name:                 "error_when_roots_missing",
			configuration:        repos.RemotesConfiguration{},
			arguments:            []string{},
			expectError:          true,
			expectedErrorMessage: remotesMissingRootsMessage,
			expectedAssumeYes:    false,
			expectTaskInvocation: false,
		},
		{
			name: "configuration_expands_home_relative_root",
			configuration: repos.RemotesConfiguration{
				AssumeYes:       true,
				RepositoryRoots: []string{"~/" + remotesHomeRootSuffixConstant},
			},
			arguments: []string{},
			expectedRootsBuilder: func(testingInstance testing.TB) []string {
				homeDirectory, homeError := os.UserHomeDir()
				require.NoError(testingInstance, homeError)
				expandedRoot := filepath.Join(homeDirectory, remotesHomeRootSuffixConstant)
				return []string{expandedRoot}
			},
			expectPromptInvocations: 0,
			expectedAssumeYes:       true,
			expectTaskInvocation:    true,
		},
		{
			name: "arguments_preserve_relative_roots",
			configuration: repos.RemotesConfiguration{
				AssumeYes:       false,
				RepositoryRoots: nil,
			},
			arguments: []string{
				remotesAssumeYesFlagConstant,
				remotesRootFlagConstant, remotesRelativeRootConstant,
			},
			expectedRoots:           []string{remotesRelativeRootConstant},
			expectPromptInvocations: 0,
			expectedAssumeYes:       true,
			expectTaskInvocation:    true,
		},
		{
			name:          "arguments_expand_home_relative_root",
			configuration: repos.RemotesConfiguration{},
			arguments: []string{
				remotesAssumeYesFlagConstant,
				remotesRootFlagConstant, "~/" + remotesHomeRootSuffixConstant,
			},
			expectedRootsBuilder: func(testingInstance testing.TB) []string {
				homeDirectory, homeError := os.UserHomeDir()
				require.NoError(testingInstance, homeError)
				expandedRoot := filepath.Join(homeDirectory, remotesHomeRootSuffixConstant)
				return []string{expandedRoot}
			},
			expectPromptInvocations: 0,
			expectedAssumeYes:       true,
			expectTaskInvocation:    true,
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			discoverer := &fakeRepositoryDiscoverer{repositories: []string{remotesDiscoveredRepository}}
			executor := &fakeGitExecutor{}
			prompter := &recordingPrompter{result: shared.ConfirmationResult{Confirmed: true}}
			runner := &recordingTaskRunner{}

			builder := repos.RemotesCommandBuilder{
				LoggerProvider: func() *zap.Logger { return zap.NewNop() },
				Discoverer:     discoverer,
				GitExecutor:    executor,
				PrompterFactory: func(*cobra.Command) shared.ConfirmationPrompter {
					return prompter
				},
				ConfigurationProvider: func() repos.RemotesConfiguration {
					return testCase.configuration
				},
				TaskRunnerFactory: func(workflow.Dependencies) repos.TaskRunnerExecutor {
					return runner
				},
			}

			command, buildError := builder.Build()
			require.NoError(subtest, buildError)
			bindGlobalRemotesFlags(command)

			command.SetContext(context.Background())
			stdoutBuffer := &bytes.Buffer{}
			stderrBuffer := &bytes.Buffer{}
			command.SetOut(stdoutBuffer)
			command.SetErr(stderrBuffer)
			command.SetArgs(testCase.arguments)

			executionError := command.Execute()
			if testCase.expectError {
				require.Error(subtest, executionError)
				require.Equal(subtest, testCase.expectedErrorMessage, executionError.Error())
				combinedOutput := stdoutBuffer.String() + stderrBuffer.String()
				require.Contains(subtest, combinedOutput, command.UseLine())
				require.Zero(subtest, prompter.calls)
				require.Empty(subtest, runner.definitions)
				return
			}

			require.NoError(subtest, executionError)

			expectedRoots := testCase.expectedRoots
			if testCase.expectedRootsBuilder != nil {
				expectedRoots = testCase.expectedRootsBuilder(subtest)
			}
			require.Equal(subtest, expectedRoots, runner.roots)
			require.Equal(subtest, testCase.expectPromptInvocations, prompter.calls)

			if testCase.expectTaskInvocation {
				require.Len(subtest, runner.definitions, 1)
				require.Len(subtest, runner.definitions[0].Actions, 1)
				action := runner.definitions[0].Actions[0]
				require.Equal(subtest, "repo.remote.update", action.Type)
				if len(strings.TrimSpace(testCase.expectedOwnerConstraint)) > 0 {
					require.Equal(subtest, testCase.expectedOwnerConstraint, action.Options["owner"])
				} else {
					require.NotContains(subtest, action.Options, "owner")
				}
				require.Equal(subtest, testCase.expectedAssumeYes, runner.runtimeOptions.AssumeYes)
			} else {
				require.Empty(subtest, runner.definitions)
			}
		})
	}
}

func TestRemotesCommandOwnerOptions(testInstance *testing.T) {
	testCases := []struct {
		name          string
		configuration repos.RemotesConfiguration
		arguments     []string
		expectedOwner string
	}{
		{
			name: "configuration_owner_applies",
			configuration: repos.RemotesConfiguration{
				Owner:           remotesOwnerConstraintConstant,
				AssumeYes:       true,
				RepositoryRoots: []string{remotesConfiguredRootConstant},
			},
			expectedOwner: remotesOwnerConstraintConstant,
		},
		{
			name: "flag_overrides_configuration",
			configuration: repos.RemotesConfiguration{
				Owner:           remotesOwnerMismatchConstant,
				AssumeYes:       true,
				RepositoryRoots: []string{remotesConfiguredRootConstant},
			},
			arguments:     []string{remotesOwnerFlagConstant, remotesOwnerConstraintConstant},
			expectedOwner: remotesOwnerConstraintConstant,
		},
		{
			name: "owner_not_specified",
			configuration: repos.RemotesConfiguration{
				RepositoryRoots: []string{remotesConfiguredRootConstant},
			},
			expectedOwner: "",
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			executor := &fakeGitExecutor{}
			runner := &recordingTaskRunner{}

			builder := repos.RemotesCommandBuilder{
				LoggerProvider: func() *zap.Logger { return zap.NewNop() },
				GitExecutor:    executor,
				ConfigurationProvider: func() repos.RemotesConfiguration {
					return testCase.configuration
				},
				TaskRunnerFactory: func(workflow.Dependencies) repos.TaskRunnerExecutor {
					return runner
				},
			}

			command, buildError := builder.Build()
			require.NoError(subtest, buildError)
			bindGlobalRemotesFlags(command)
			command.SetContext(context.Background())
			command.SetOut(io.Discard)
			command.SetErr(io.Discard)
			command.SetArgs(testCase.arguments)

			require.NoError(subtest, command.Execute())

			require.Len(subtest, runner.definitions, 1)
			require.Len(subtest, runner.definitions[0].Actions, 1)
			action := runner.definitions[0].Actions[0]
			if len(strings.TrimSpace(testCase.expectedOwner)) > 0 {
				require.Equal(subtest, testCase.expectedOwner, action.Options["owner"])
			} else {
				require.NotContains(subtest, action.Options, "owner")
			}
		})
	}
}

func bindGlobalRemotesFlags(command *cobra.Command) {
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	flagutils.BindExecutionFlags(command, flagutils.ExecutionDefaults{}, flagutils.ExecutionFlagDefinitions{
		AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Shorthand: flagutils.AssumeYesFlagShorthand, Enabled: true},
	})
	command.PersistentFlags().String(flagutils.RemoteFlagName, "", flagutils.RemoteFlagUsage)
	command.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		contextAccessor := utils.NewCommandContextAccessor()
		executionFlags := utils.ExecutionFlags{}
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
