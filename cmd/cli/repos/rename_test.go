package repos_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repos "github.com/temirov/gix/cmd/cli/repos"
	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/repos/shared"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
)

const (
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
	folderRenamePresetName          = "folder-rename"
	renamePresetMissingMessage      = "folder-rename preset not found"
)

type renameRecordingExecutor struct {
	roots   []string
	options workflow.RuntimeOptions
}

func (executor *renameRecordingExecutor) Execute(_ context.Context, roots []string, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	executor.roots = append([]string{}, roots...)
	executor.options = options
	return workflow.ExecutionOutcome{}, nil
}

func TestRenameCommandConfigurationPrecedence(testInstance *testing.T) {
	presetConfig := loadFolderRenamePreset(testInstance)

	testCases := []struct {
		name                 string
		configuration        *repos.RenameConfiguration
		arguments            []string
		expectedRoots        []string
		expectedRootsBuilder func(testing.TB) []string
		expectError          bool
		expectedErrorMessage string
		expectExecution      bool
		expectedAssumeYes    bool
		expectedRequireClean bool
		expectedIncludeOwner bool
	}{
		{
			name: "configuration_supplies_defaults",
			configuration: &repos.RenameConfiguration{
				AssumeYes:            true,
				RequireCleanWorktree: false,
				RepositoryRoots:      []string{renameConfiguredRootConstant},
			},
			expectedRoots:        []string{renameConfiguredRootConstant},
			expectExecution:      true,
			expectedAssumeYes:    true,
			expectedRequireClean: false,
			expectedIncludeOwner: false,
		},
		{
			name: "flags_override_configuration",
			configuration: &repos.RenameConfiguration{
				AssumeYes:            false,
				RequireCleanWorktree: false,
				RepositoryRoots:      []string{renameConfiguredRootConstant},
			},
			arguments: []string{
				renameAssumeYesFlagConstant,
				renameRequireCleanFlagConstant,
				renameIncludeOwnerFlagConstant,
				renameRootFlagConstant, renameCLIRepositoryRootConstant,
			},
			expectedRoots:        []string{renameCLIRepositoryRootConstant},
			expectExecution:      true,
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
				AssumeYes:            true,
				RequireCleanWorktree: true,
				RepositoryRoots:      []string{"~/" + renameHomeRootSuffixConstant},
			},
			expectedRootsBuilder: func(tb testing.TB) []string {
				homeDirectory, homeError := os.UserHomeDir()
				require.NoError(tb, homeError)
				return []string{filepath.Join(homeDirectory, renameHomeRootSuffixConstant)}
			},
			expectExecution:      true,
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
				renameAssumeYesFlagConstant,
				renameRootFlagConstant, renameRelativeRootConstant,
			},
			expectedRoots:        []string{renameRelativeRootConstant},
			expectExecution:      true,
			expectedAssumeYes:    true,
			expectedRequireClean: false,
			expectedIncludeOwner: false,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			catalog := &fakePresetCatalog{configuration: presetConfig, found: true}
			executor := &renameRecordingExecutor{}

			builder := repos.RenameCommandBuilder{
				LoggerProvider: func() *zap.Logger { return zap.NewNop() },
				Discoverer:     &fakeRepositoryDiscoverer{repositories: []string{renameDiscoveredRepositoryPath}},
				GitExecutor:    &fakeGitExecutor{},
				GitManager:     &fakeGitRepositoryManager{},
				FileSystem:     fakeFileSystem{files: map[string]string{}},
				PrompterFactory: func(*cobra.Command) shared.ConfirmationPrompter {
					return nil
				},
				HumanReadableLoggingProvider: func() bool { return false },
				ConfigurationProvider: func() repos.RenameConfiguration {
					if testCase.configuration != nil {
						return *testCase.configuration
					}
					return repos.RenameConfiguration{}
				},
				PresetCatalogFactory: func() workflowcmd.PresetCatalog { return catalog },
				WorkflowExecutorFactory: func(nodes []*workflow.OperationNode, _ workflow.Dependencies) repos.WorkflowExecutor {
					if testCase.expectExecution {
						require.Len(subtest, nodes, 1)
						renameOperation, ok := nodes[0].Operation.(*workflow.RenameOperation)
						require.True(subtest, ok)
						require.Equal(subtest, testCase.expectedRequireClean, renameOperation.RequireCleanWorktree)
						require.Equal(subtest, testCase.expectedIncludeOwner, renameOperation.IncludeOwner)
					}
					return executor
				},
			}

			command, buildError := builder.Build()
			require.NoError(subtest, buildError)
			bindRenameCommandFlags(command)
			command.SetArgs(testCase.arguments)

			err := command.Execute()
			if testCase.expectError {
				require.EqualError(subtest, err, testCase.expectedErrorMessage)
				require.Nil(subtest, executor.roots)
				return
			}
			require.NoError(subtest, err)
			require.Equal(subtest, folderRenamePresetName, catalog.loadedName)

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
			require.True(subtest, executor.options.IncludeNestedRepositories)
			require.True(subtest, executor.options.ProcessRepositoriesByDescendingDepth)
			require.Equal(subtest, testCase.expectedRequireClean, executor.options.CaptureInitialWorktreeStatus)
		})
	}
}

func TestRenameCommandPresetErrorsSurface(testInstance *testing.T) {
	presetConfig := loadFolderRenamePreset(testInstance)

	testCases := []struct {
		name      string
		catalog   fakePresetCatalog
		expectErr string
	}{
		{
			name:      "missing_preset",
			catalog:   fakePresetCatalog{found: false},
			expectErr: renamePresetMissingMessage,
		},
		{
			name:      "load_error",
			catalog:   fakePresetCatalog{found: true, loadError: errors.New("boom")},
			expectErr: "unable to load folder-rename preset: boom",
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
			expectErr: "unable to build folder-rename workflow: unsupported workflow command: unknown",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			catalog := testCase.catalog
			if len(catalog.configuration.Steps) == 0 && catalog.found && catalog.loadError == nil && testCase.name != "build_error" {
				catalog.configuration = presetConfig
			}

			builder := repos.RenameCommandBuilder{
				LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
				Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{renameDiscoveredRepositoryPath}},
				GitExecutor:                  &fakeGitExecutor{},
				GitManager:                   &fakeGitRepositoryManager{},
				FileSystem:                   fakeFileSystem{files: map[string]string{}},
				PrompterFactory:              func(*cobra.Command) shared.ConfirmationPrompter { return nil },
				HumanReadableLoggingProvider: func() bool { return false },
				ConfigurationProvider: func() repos.RenameConfiguration {
					return repos.RenameConfiguration{RepositoryRoots: []string{renameConfiguredRootConstant}}
				},
				PresetCatalogFactory: func() workflowcmd.PresetCatalog { return &catalog },
			}

			command, buildError := builder.Build()
			require.NoError(subtest, buildError)
			bindRenameCommandFlags(command)
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			err := command.Execute()
			require.EqualError(subtest, err, testCase.expectErr)
		})
	}
}

func loadFolderRenamePreset(testingInstance testing.TB) workflow.Configuration {
	presetContent := []byte(`workflow:
  - step:
      name: folder-rename
      command: ["folder", "rename"]
      with:
        require_clean: false
        include_owner: false
`)
	configuration, err := workflow.ParseConfiguration(presetContent)
	require.NoError(testingInstance, err)
	return configuration
}

func TestFolderRenamePresetUsesBooleanOptions(testInstance *testing.T) {
	configuration := loadFolderRenamePreset(testInstance)
	require.Len(testInstance, configuration.Steps, 1)

	options := configuration.Steps[0].Options
	require.Contains(testInstance, options, "require_clean")
	require.Contains(testInstance, options, "include_owner")
	require.IsType(testInstance, true, options["require_clean"])
	require.IsType(testInstance, true, options["include_owner"])
}

func bindRenameCommandFlags(command *cobra.Command) {
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
