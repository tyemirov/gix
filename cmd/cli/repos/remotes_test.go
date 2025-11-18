package repos_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repos "github.com/tyemirov/gix/cmd/cli/repos"
	workflowcmd "github.com/tyemirov/gix/cmd/cli/workflow"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	remotesAssumeYesFlagConstant     = "--" + flagutils.AssumeYesFlagName
	remotesRootFlagConstant          = "--" + flagutils.DefaultRootFlagName
	remotesConfiguredRootConstant    = "/tmp/remotes-config-root"
	remotesCLIRepositoryRootConstant = "/tmp/remotes-cli-root"
	remotesDiscoveredRepository      = "/tmp/remotes-repo"
	remotesMetadataDefaultBranch     = "main"
	remotesMissingRootsMessage       = "no repository roots provided; specify --roots or configure defaults"
	remotesRelativeRootConstant      = "relative/remotes-root"
	remotesHomeRootSuffixConstant    = "remotes-home-root"
	remotesOwnerFlagConstant         = "--owner"
	remotesOwnerConstraintConstant   = "canonical"
	remotesOwnerMismatchConstant     = "different"
	remoteCanonicalPresetName        = "remote-update-to-canonical"
)

type remotesRecordingExecutor struct {
	roots   []string
	options workflow.RuntimeOptions
}

func (executor *remotesRecordingExecutor) Execute(_ context.Context, roots []string, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	executor.roots = append([]string{}, roots...)
	executor.options = options
	return workflow.ExecutionOutcome{}, nil
}

func TestRemotesCommandConfigurationPrecedence(testInstance *testing.T) {
	presetConfig := loadRemoteCanonicalPreset(testInstance)

	testCases := []struct {
		name                 string
		configuration        repos.RemotesConfiguration
		arguments            []string
		expectedRoots        []string
		expectedRootsBuilder func(testing.TB) []string
		expectError          bool
		expectedErrorMessage string
		expectedAssumeYes    bool
		expectedOwner        string
		expectExecution      bool
	}{
		{
			name: "configuration_uses_defaults",
			configuration: repos.RemotesConfiguration{
				AssumeYes:       false,
				RepositoryRoots: []string{remotesConfiguredRootConstant},
			},
			expectedRoots:     []string{remotesConfiguredRootConstant},
			expectedAssumeYes: false,
			expectExecution:   true,
		},
		{
			name: "flags_override_configuration",
			configuration: repos.RemotesConfiguration{
				AssumeYes:       false,
				Owner:           remotesOwnerMismatchConstant,
				RepositoryRoots: []string{remotesConfiguredRootConstant},
			},
			arguments: []string{
				remotesAssumeYesFlagConstant,
				remotesOwnerFlagConstant, remotesOwnerConstraintConstant,
				remotesRootFlagConstant, remotesCLIRepositoryRootConstant,
			},
			expectedRoots:     []string{remotesCLIRepositoryRootConstant},
			expectedAssumeYes: true,
			expectedOwner:     remotesOwnerConstraintConstant,
			expectExecution:   true,
		},
		{
			name:                 "error_when_roots_missing",
			configuration:        repos.RemotesConfiguration{},
			expectError:          true,
			expectedErrorMessage: remotesMissingRootsMessage,
		},
		{
			name: "configuration_expands_home_relative_root",
			configuration: repos.RemotesConfiguration{
				AssumeYes:       true,
				RepositoryRoots: []string{"~/" + remotesHomeRootSuffixConstant},
			},
			expectedRootsBuilder: func(tb testing.TB) []string {
				homeDirectory, homeError := os.UserHomeDir()
				require.NoError(tb, homeError)
				return []string{filepath.Join(homeDirectory, remotesHomeRootSuffixConstant)}
			},
			expectedAssumeYes: true,
			expectExecution:   true,
		},
		{
			name: "arguments_preserve_relative_root",
			configuration: repos.RemotesConfiguration{
				RepositoryRoots: nil,
			},
			arguments: []string{
				remotesAssumeYesFlagConstant,
				remotesRootFlagConstant, remotesRelativeRootConstant,
			},
			expectedRoots:     []string{remotesRelativeRootConstant},
			expectedAssumeYes: true,
			expectExecution:   true,
		},
		{
			name:          "arguments_expand_home_relative_root",
			configuration: repos.RemotesConfiguration{},
			arguments: []string{
				remotesAssumeYesFlagConstant,
				remotesRootFlagConstant, "~/" + remotesHomeRootSuffixConstant,
			},
			expectedRootsBuilder: func(tb testing.TB) []string {
				homeDirectory, homeError := os.UserHomeDir()
				require.NoError(tb, homeError)
				return []string{filepath.Join(homeDirectory, remotesHomeRootSuffixConstant)}
			},
			expectedAssumeYes: true,
			expectExecution:   true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			catalog := &fakePresetCatalog{configuration: presetConfig, found: true}
			executor := &remotesRecordingExecutor{}

			builder := repos.RemotesCommandBuilder{
				LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
				Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{remotesDiscoveredRepository}},
				GitExecutor:                  &fakeGitExecutor{},
				GitManager:                   &fakeGitRepositoryManager{},
				HumanReadableLoggingProvider: func() bool { return false },
				ConfigurationProvider: func() repos.RemotesConfiguration {
					return testCase.configuration
				},
				PresetCatalogFactory: func() workflowcmd.PresetCatalog { return catalog },
				WorkflowExecutorFactory: func(nodes []*workflow.OperationNode, _ workflow.Dependencies) workflowcmd.OperationExecutor {
					if !testCase.expectExecution {
						return executor
					}
					require.Len(subtest, nodes, 1)
					canonicalOp, ok := nodes[0].Operation.(*workflow.CanonicalRemoteOperation)
					require.True(subtest, ok)
					require.Equal(subtest, strings.TrimSpace(testCase.expectedOwner), canonicalOp.OwnerConstraint)
					return executor
				},
			}

			command, buildError := builder.Build()
			require.NoError(subtest, buildError)
			bindRemotesCommandFlags(command)
			command.SetArgs(testCase.arguments)

			err := command.Execute()
			if testCase.expectError {
				require.EqualError(subtest, err, testCase.expectedErrorMessage)
				require.Nil(subtest, executor.roots)
				return
			}

			require.NoError(subtest, err)
			require.Equal(subtest, remoteCanonicalPresetName, catalog.loadedName)

			if !testCase.expectExecution {
				require.Nil(subtest, executor.roots)
				return
			}

			expectedRoots := testCase.expectedRoots
			if testCase.expectedRootsBuilder != nil {
				expectedRoots = testCase.expectedRootsBuilder(subtest)
			}
			require.Equal(subtest, expectedRoots, executor.roots)
			require.Equal(subtest, testCase.expectedAssumeYes, executor.options.AssumeYes)
		})
	}
}

func TestRemotesCommandPresetErrorsSurface(testInstance *testing.T) {
	presetConfig := loadRemoteCanonicalPreset(testInstance)

	testCases := []struct {
		name      string
		catalog   fakePresetCatalog
		expectErr string
	}{
		{
			name:      "missing_preset",
			catalog:   fakePresetCatalog{found: false},
			expectErr: "remote-update-to-canonical preset not found",
		},
		{
			name:      "load_error",
			catalog:   fakePresetCatalog{found: true, loadError: errors.New("boom")},
			expectErr: "unable to load remote-update-to-canonical preset: boom",
		},
		{
			name: "build_error",
			catalog: fakePresetCatalog{
				found: true,
				configuration: workflow.Configuration{
					Steps: []workflow.StepConfiguration{{Command: []string{"unknown"}}},
				},
			},
			expectErr: "unable to build remote-update-to-canonical workflow: unsupported workflow command: unknown",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			catalog := testCase.catalog
			if len(catalog.configuration.Steps) == 0 && catalog.found && catalog.loadError == nil && testCase.name != "build_error" {
				catalog.configuration = presetConfig
			}

			builder := repos.RemotesCommandBuilder{
				LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
				Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{remotesDiscoveredRepository}},
				GitExecutor:                  &fakeGitExecutor{},
				GitManager:                   &fakeGitRepositoryManager{},
				HumanReadableLoggingProvider: func() bool { return false },
				ConfigurationProvider: func() repos.RemotesConfiguration {
					return repos.RemotesConfiguration{RepositoryRoots: []string{remotesConfiguredRootConstant}}
				},
				PresetCatalogFactory: func() workflowcmd.PresetCatalog { return &catalog },
			}

			command, buildError := builder.Build()
			require.NoError(subtest, buildError)
			bindRemotesCommandFlags(command)
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			err := command.Execute()
			require.EqualError(subtest, err, testCase.expectErr)
		})
	}
}

func loadRemoteCanonicalPreset(testingInstance testing.TB) workflow.Configuration {
	presetContent := []byte(`workflow:
  - step:
      name: remote-update-to-canonical
      command: ["remote", "update-to-canonical"]
      with:
        owner: ""
`)
	configuration, err := workflow.ParseConfiguration(presetContent)
	require.NoError(testingInstance, err)
	return configuration
}

func bindRemotesCommandFlags(command *cobra.Command) {
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{
		Name:       flagutils.DefaultRootFlagName,
		Usage:      flagutils.DefaultRootFlagUsage,
		Enabled:    true,
		Persistent: true,
	})
	flagutils.BindExecutionFlags(command, flagutils.ExecutionDefaults{}, flagutils.ExecutionFlagDefinitions{
		AssumeYes: flagutils.ExecutionFlagDefinition{
			Name:      flagutils.AssumeYesFlagName,
			Usage:     flagutils.AssumeYesFlagUsage,
			Shorthand: flagutils.AssumeYesFlagShorthand,
			Enabled:   true,
		},
	})
}
