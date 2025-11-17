package repos_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repos "github.com/temirov/gix/cmd/cli/repos"
	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/repos/shared"
	"github.com/temirov/gix/internal/utils"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
)

const (
	protocolFromFlagConstant       = "--from"
	protocolToFlagConstant         = "--to"
	protocolYesFlagConstant        = "--" + flagutils.AssumeYesFlagName
	protocolRootFlagConstant       = "--" + flagutils.DefaultRootFlagName
	protocolConfiguredRootConstant = "/tmp/protocol-config-root"
	protocolMissingRootsMessage    = "no repository roots provided; specify --roots or configure defaults"
	protocolRelativeRootConstant   = "relative/protocol-root"
	protocolHomeRootSuffixConstant = "protocol-home-root"
)

func TestProtocolCommandConfigurationPrecedence(testInstance *testing.T) {
	testInstance.Parallel()

	testCases := []struct {
		name                 string
		configuration        repos.ProtocolConfiguration
		arguments            []string
		expectedRoots        []string
		expectedRootsBuilder func(testing.TB) []string
		expectTaskInvocation bool
		expectedAssumeYes    bool
		expectedFrom         string
		expectedTo           string
		expectError          bool
		expectedErrorMessage string
	}{
		{
			name: "error_when_roots_missing",
			configuration: repos.ProtocolConfiguration{
				FromProtocol: string(shared.RemoteProtocolHTTPS),
				ToProtocol:   string(shared.RemoteProtocolSSH),
			},
			arguments:            []string{},
			expectError:          true,
			expectedErrorMessage: protocolMissingRootsMessage,
		},
		{
			name: "configuration_supplies_protocols",
			configuration: repos.ProtocolConfiguration{
				AssumeYes:       false,
				RepositoryRoots: []string{protocolConfiguredRootConstant},
				FromProtocol:    string(shared.RemoteProtocolHTTPS),
				ToProtocol:      string(shared.RemoteProtocolSSH),
			},
			expectedRoots:        []string{protocolConfiguredRootConstant},
			expectTaskInvocation: true,
			expectedAssumeYes:    false,
			expectedFrom:         string(shared.RemoteProtocolHTTPS),
			expectedTo:           string(shared.RemoteProtocolSSH),
		},
		{
			name: "flags_override_configuration",
			configuration: repos.ProtocolConfiguration{
				AssumeYes:       false,
				RepositoryRoots: []string{protocolConfiguredRootConstant},
				FromProtocol:    string(shared.RemoteProtocolSSH),
				ToProtocol:      string(shared.RemoteProtocolHTTPS),
			},
			arguments: []string{
				protocolFromFlagConstant, string(shared.RemoteProtocolHTTPS),
				protocolToFlagConstant, string(shared.RemoteProtocolSSH),
				protocolYesFlagConstant,
				protocolRootFlagConstant, remotesCLIRepositoryRootConstant,
			},
			expectedRoots:        []string{remotesCLIRepositoryRootConstant},
			expectTaskInvocation: true,
			expectedAssumeYes:    true,
			expectedFrom:         string(shared.RemoteProtocolHTTPS),
			expectedTo:           string(shared.RemoteProtocolSSH),
		},
		{
			name: "configuration_triggers_remote_update",
			configuration: repos.ProtocolConfiguration{
				AssumeYes:       true,
				RepositoryRoots: []string{protocolConfiguredRootConstant},
				FromProtocol:    string(shared.RemoteProtocolHTTPS),
				ToProtocol:      string(shared.RemoteProtocolSSH),
			},
			expectedRoots:        []string{protocolConfiguredRootConstant},
			expectTaskInvocation: true,
			expectedAssumeYes:    true,
			expectedFrom:         string(shared.RemoteProtocolHTTPS),
			expectedTo:           string(shared.RemoteProtocolSSH),
		},
		{
			name: "configuration_expands_home_relative_root",
			configuration: repos.ProtocolConfiguration{
				AssumeYes:       true,
				RepositoryRoots: []string{"~/" + protocolHomeRootSuffixConstant},
				FromProtocol:    string(shared.RemoteProtocolHTTPS),
				ToProtocol:      string(shared.RemoteProtocolSSH),
			},
			arguments:            []string{},
			expectedRootsBuilder: protocolHomeRootBuilder,
			expectTaskInvocation: true,
			expectedAssumeYes:    true,
			expectedFrom:         string(shared.RemoteProtocolHTTPS),
			expectedTo:           string(shared.RemoteProtocolSSH),
		},
		{
			name: "arguments_preserve_relative_roots",
			configuration: repos.ProtocolConfiguration{
				FromProtocol: string(shared.RemoteProtocolHTTPS),
				ToProtocol:   string(shared.RemoteProtocolSSH),
			},
			arguments: []string{
				protocolFromFlagConstant, string(shared.RemoteProtocolHTTPS),
				protocolToFlagConstant, string(shared.RemoteProtocolSSH),
				protocolYesFlagConstant,
				protocolRootFlagConstant, protocolRelativeRootConstant,
			},
			expectedRoots:        []string{protocolRelativeRootConstant},
			expectTaskInvocation: true,
			expectedAssumeYes:    true,
			expectedFrom:         string(shared.RemoteProtocolHTTPS),
			expectedTo:           string(shared.RemoteProtocolSSH),
		},
		{
			name: "arguments_expand_home_relative_root",
			configuration: repos.ProtocolConfiguration{
				FromProtocol: string(shared.RemoteProtocolHTTPS),
				ToProtocol:   string(shared.RemoteProtocolSSH),
			},
			arguments: []string{
				protocolFromFlagConstant, string(shared.RemoteProtocolHTTPS),
				protocolToFlagConstant, string(shared.RemoteProtocolSSH),
				protocolYesFlagConstant,
				protocolRootFlagConstant, "~/" + protocolHomeRootSuffixConstant,
			},
			expectedRootsBuilder: protocolHomeRootBuilder,
			expectTaskInvocation: true,
			expectedAssumeYes:    true,
			expectedFrom:         string(shared.RemoteProtocolHTTPS),
			expectedTo:           string(shared.RemoteProtocolSSH),
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			presetConfig := loadProtocolPreset(subtest)
			discoverer := &fakeRepositoryDiscoverer{repositories: []string{remotesDiscoveredRepository}}
			executor := &fakeGitExecutor{}
			manager := &fakeGitRepositoryManager{remoteURL: "", currentBranch: remotesMetadataDefaultBranch, panicOnCurrentBranchLookup: true}
			recording := &protocolRecordingExecutor{}

			builder := repos.ProtocolCommandBuilder{
				LoggerProvider: func() *zap.Logger { return zap.NewNop() },
				Discoverer:     discoverer,
				GitExecutor:    executor,
				GitManager:     manager,
				ConfigurationProvider: func() repos.ProtocolConfiguration {
					return testCase.configuration
				},
				PresetCatalogFactory: func() workflowcmd.PresetCatalog {
					return &fakePresetCatalog{configuration: presetConfig, found: true}
				},
				WorkflowExecutorFactory: func(nodes []*workflow.OperationNode, _ workflow.Dependencies) workflowcmd.OperationExecutor {
					if !testCase.expectTaskInvocation {
						return recording
					}
					require.Len(subtest, nodes, 1)
					conversionOp, ok := nodes[0].Operation.(*workflow.ProtocolConversionOperation)
					require.True(subtest, ok)
					require.Equal(subtest, shared.RemoteProtocol(testCase.expectedFrom), conversionOp.FromProtocol)
					require.Equal(subtest, shared.RemoteProtocol(testCase.expectedTo), conversionOp.ToProtocol)
					return recording
				},
			}

			command, buildError := builder.Build()
			require.NoError(subtest, buildError)
			bindGlobalProtocolFlags(command)

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
				return
			}

			require.NoError(subtest, executionError)

			expectedRoots := testCase.expectedRoots
			if testCase.expectedRootsBuilder != nil {
				expectedRoots = testCase.expectedRootsBuilder(subtest)
			}

			if testCase.expectTaskInvocation {
				require.Equal(subtest, expectedRoots, recording.roots)
				require.Equal(subtest, testCase.expectedAssumeYes, recording.options.AssumeYes)
			} else {
				require.Empty(subtest, recording.roots)
			}
		})
	}
}

type protocolRecordingExecutor struct {
	roots   []string
	options workflow.RuntimeOptions
}

func (executor *protocolRecordingExecutor) Execute(_ context.Context, roots []string, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	executor.roots = append([]string{}, roots...)
	executor.options = options
	return workflow.ExecutionOutcome{}, nil
}

func loadProtocolPreset(testingInstance testing.TB) workflow.Configuration {
	presetContent := []byte(`workflow:
  - step:
      name: remote-update-protocol
      command: ["remote", "update-protocol"]
      with:
        from: ""
        to: ""
`)
	configuration, err := workflow.ParseConfiguration(presetContent)
	require.NoError(testingInstance, err)
	return configuration
}

func bindGlobalProtocolFlags(command *cobra.Command) {
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

func protocolHomeRootBuilder(testingInstance testing.TB) []string {
	homeDirectory, homeError := os.UserHomeDir()
	require.NoError(testingInstance, homeError)
	expandedRoot := filepath.Join(homeDirectory, protocolHomeRootSuffixConstant)
	return []string{expandedRoot}
}
