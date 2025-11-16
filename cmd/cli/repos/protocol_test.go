package repos_test

import (
	"context"
	"errors"
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
	protocolPresetName             = "remote-update-protocol"
)

func TestProtocolCommandConfigurationPrecedence(testInstance *testing.T) {
	presetConfig := loadProtocolPreset(testInstance)

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
			catalog := &fakePresetCatalog{configuration: presetConfig, found: true}
			executor := &protocolRecordingExecutor{}

			builder := repos.ProtocolCommandBuilder{
				LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
				Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{remotesDiscoveredRepository}},
				GitExecutor:                  &fakeGitExecutor{},
				GitManager:                   &fakeGitRepositoryManager{},
				HumanReadableLoggingProvider: func() bool { return false },
				ConfigurationProvider: func() repos.ProtocolConfiguration {
					return testCase.configuration
				},
				PresetCatalogFactory: func() workflowcmd.PresetCatalog { return catalog },
				WorkflowExecutorFactory: func(nodes []*workflow.OperationNode, _ workflow.Dependencies) repos.WorkflowExecutor {
					if !testCase.expectTaskInvocation {
						return executor
					}
					require.Len(subtest, nodes, 1)
					conversionOp, ok := nodes[0].Operation.(*workflow.ProtocolConversionOperation)
					require.True(subtest, ok)
					require.Equal(subtest, testCase.expectedFrom, string(conversionOp.FromProtocol))
					require.Equal(subtest, testCase.expectedTo, string(conversionOp.ToProtocol))
					return executor
				},
			}

			command, buildError := builder.Build()
			require.NoError(subtest, buildError)
			bindGlobalProtocolFlags(command)

			command.SetArgs(testCase.arguments)

			executionError := command.Execute()
			if testCase.expectError {
				require.Error(subtest, executionError)
				require.Equal(subtest, testCase.expectedErrorMessage, executionError.Error())
				require.Nil(subtest, executor.roots)
				return
			}

			require.NoError(subtest, executionError)
			require.Equal(subtest, protocolPresetName, catalog.loadedName)

			expectedRoots := testCase.expectedRoots
			if testCase.expectedRootsBuilder != nil {
				expectedRoots = testCase.expectedRootsBuilder(subtest)
			}

			if !testCase.expectTaskInvocation {
				require.Nil(subtest, executor.roots)
				return
			}

			require.Equal(subtest, expectedRoots, executor.roots)
			require.Equal(subtest, testCase.expectedAssumeYes, executor.options.AssumeYes)
		})
	}
}

func TestProtocolCommandPresetErrorsSurface(testInstance *testing.T) {
	presetConfig := loadProtocolPreset(testInstance)

	testCases := []struct {
		name      string
		catalog   fakePresetCatalog
		expectErr string
	}{
		{
			name:      "missing_preset",
			catalog:   fakePresetCatalog{found: false},
			expectErr: "remote-update-protocol preset not found",
		},
		{
			name:      "load_error",
			catalog:   fakePresetCatalog{found: true, loadError: errors.New("boom")},
			expectErr: "unable to load remote-update-protocol preset: boom",
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
			expectErr: "unable to build remote-update-protocol workflow: unsupported workflow command: unknown",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			catalog := testCase.catalog
			if len(catalog.configuration.Steps) == 0 && catalog.found && catalog.loadError == nil && testCase.name != "build_error" {
				catalog.configuration = presetConfig
			}

			builder := repos.ProtocolCommandBuilder{
				LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
				Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{remotesDiscoveredRepository}},
				GitExecutor:                  &fakeGitExecutor{},
				GitManager:                   &fakeGitRepositoryManager{},
				HumanReadableLoggingProvider: func() bool { return false },
				ConfigurationProvider: func() repos.ProtocolConfiguration {
					return repos.ProtocolConfiguration{
						RepositoryRoots: []string{protocolConfiguredRootConstant},
						FromProtocol:    string(shared.RemoteProtocolHTTPS),
						ToProtocol:      string(shared.RemoteProtocolSSH),
					}
				},
				PresetCatalogFactory: func() workflowcmd.PresetCatalog { return &catalog },
			}

			command, buildError := builder.Build()
			require.NoError(subtest, buildError)
			bindGlobalProtocolFlags(command)
			err := command.Execute()
			require.EqualError(subtest, err, testCase.expectErr)
		})
	}
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
