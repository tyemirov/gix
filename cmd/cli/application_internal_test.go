package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	flagutils "github.com/tyemirov/gix/internal/utils/flags"
)

const (
	defaultCommandNameConstant = defaultCommandUseNameConstant
)

func TestApplicationCommonDefaultsApplied(t *testing.T) {
	operations, buildError := newOperationConfigurations([]ApplicationOperationConfiguration{
		{
			Command: []string{"folder", "rename"},
			Options: map[string]any{
				"roots": []string{"/tmp/rename"},
			},
		},
		{
			Command: []string{"workflow"},
			Options: map[string]any{
				"roots": []string{"/tmp/workflow"},
			},
		},
	})
	require.NoError(t, buildError)

	application := &Application{
		logger: zap.NewNop(),
		configuration: ApplicationConfiguration{
			Common: ApplicationCommonConfiguration{
				AssumeYes:    true,
				RequireClean: true,
			},
		},
		operationConfigurations: operations,
	}

	renameConfiguration := application.reposRenameConfiguration()
	require.True(t, renameConfiguration.AssumeYes)
	require.True(t, renameConfiguration.RequireCleanWorktree)
	require.False(t, renameConfiguration.IncludeOwner)

	workflowConfiguration := application.workflowCommandConfiguration()
	require.True(t, workflowConfiguration.AssumeYes)
	require.True(t, workflowConfiguration.RequireClean)
}

func TestApplicationOperationOverridesTakePriority(t *testing.T) {
	operations, buildError := newOperationConfigurations([]ApplicationOperationConfiguration{
		{
			Command: []string{"folder", "rename"},
			Options: map[string]any{
				"assume_yes":    false,
				"require_clean": false,
				"include_owner": true,
				"roots":         []string{"/tmp/rename"},
			},
		},
		{
			Command: []string{"workflow"},
			Options: map[string]any{
				"assume_yes":    false,
				"require_clean": false,
				"roots":         []string{"/tmp/workflow"},
			},
		},
	})
	require.NoError(t, buildError)

	application := &Application{
		logger: zap.NewNop(),
		configuration: ApplicationConfiguration{
			Common: ApplicationCommonConfiguration{
				AssumeYes:    true,
				RequireClean: true,
			},
		},
		operationConfigurations: operations,
	}

	renameConfiguration := application.reposRenameConfiguration()
	require.False(t, renameConfiguration.AssumeYes)
	require.False(t, renameConfiguration.RequireCleanWorktree)
	require.True(t, renameConfiguration.IncludeOwner)

	workflowConfiguration := application.workflowCommandConfiguration()
	require.False(t, workflowConfiguration.AssumeYes)
	require.False(t, workflowConfiguration.RequireClean)
}

func TestOperationConfigurationsErrorOnLegacyCommandNames(t *testing.T) {
	operations, buildError := newOperationConfigurations([]ApplicationOperationConfiguration{
		{
			Command: []string{"repo", "remote", "update-to-canonical"},
			Options: map[string]any{
				"roots": []string{"/tmp/legacy"},
			},
		},
	})
	require.NoError(t, buildError)

	_, lookupError := operations.Lookup(reposRemotesOperationNameConstant)
	var missing MissingOperationConfigurationError
	require.ErrorAs(t, lookupError, &missing)
	require.Equal(t, reposRemotesOperationNameConstant, missing.OperationName)
}

func TestInitializeConfigurationAttachesBranchContext(t *testing.T) {
	application := NewApplication()
	rootCommand := application.rootCommand
	rootCommand.SetContext(context.Background())

	require.NoError(t, rootCommand.PersistentFlags().Set(flagutils.AssumeYesFlagName, "true"))
	require.NoError(t, rootCommand.PersistentFlags().Set(flagutils.RemoteFlagName, "custom-remote"))

	initializationError := application.initializeConfiguration(rootCommand)
	require.NoError(t, initializationError)

	branchContext, branchExists := application.commandContextAccessor.BranchContext(rootCommand.Context())
	require.True(t, branchExists)
	require.Empty(t, branchContext.Name)
	require.True(t, branchContext.RequireClean)

	executionFlags, executionFlagsAvailable := application.commandContextAccessor.ExecutionFlags(rootCommand.Context())
	require.True(t, executionFlagsAvailable)
	require.True(t, executionFlags.AssumeYes)
	require.Equal(t, "custom-remote", executionFlags.Remote)
}

func TestRootCommandToggleHelpFormatting(t *testing.T) {
	application := NewApplication()
	usage := application.rootCommand.PersistentFlags().FlagUsages()

	require.Contains(t, usage, "--yes <yes|NO>")
	require.Contains(t, usage, "--init <LOCAL|user>")
	require.NotContains(t, usage, "--init string")
	require.NotContains(t, usage, "[=\"local\"]")
	require.NotContains(t, usage, "__toggle_true__")
	require.NotContains(t, usage, "toggle[")
}

func TestNormalizeInitializationScopeArguments(t *testing.T) {
	testCases := []struct {
		name         string
		input        []string
		expectedArgs []string
	}{
		{
			name:         "NoArguments",
			input:        nil,
			expectedArgs: nil,
		},
		{
			name:         "ImplicitLocalValue",
			input:        []string{"--init"},
			expectedArgs: []string{"--init=local"},
		},
		{
			name:         "ImplicitLocalWithFollowingFlag",
			input:        []string{"--init", "--force"},
			expectedArgs: []string{"--init=local", "--force"},
		},
		{
			name:         "ExplicitLocalValue",
			input:        []string{"--init", "local"},
			expectedArgs: []string{"--init", "local"},
		},
		{
			name:         "ExplicitUserValue",
			input:        []string{"--init=user"},
			expectedArgs: []string{"--init=user"},
		},
		{
			name:         "EmptyAssignmentDefaultsToLocal",
			input:        []string{"--init="},
			expectedArgs: []string{"--init=local"},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			normalized := normalizeInitializationScopeArguments(testCase.input)
			require.Equal(t, testCase.expectedArgs, normalized)
		})
	}
}

func TestApplicationCommandHierarchyAndAliases(t *testing.T) {
	application := NewApplication()
	rootCommand := application.rootCommand

	auditCommand, _, auditError := rootCommand.Find([]string{"a"})
	require.NoError(t, auditError)
	require.Equal(t, auditOperationNameConstant, auditCommand.Name())

	workflowCommand, _, workflowError := rootCommand.Find([]string{"w"})
	require.NoError(t, workflowError)
	require.Equal(t, workflowCommandOperationNameConstant, workflowCommand.Name())

	folderRenameCommand, _, renameError := rootCommand.Find([]string{"folder", "rename"})
	require.NoError(t, renameError)
	require.Equal(t, "rename", folderRenameCommand.Name())
	require.NotNil(t, folderRenameCommand.Parent())
	require.Equal(t, "folder", folderRenameCommand.Parent().Name())

	repoRemoteCanonicalCommand, _, canonicalError := rootCommand.Find([]string{"remote", "update-to-canonical"})
	require.NoError(t, canonicalError)
	require.Equal(t, "update-to-canonical", repoRemoteCanonicalCommand.Name())
	require.NotNil(t, repoRemoteCanonicalCommand.Parent())
	require.Equal(t, "remote", repoRemoteCanonicalCommand.Parent().Name())

	repoRemoteProtocolCommand, _, protocolError := rootCommand.Find([]string{"remote", "update-protocol"})
	require.NoError(t, protocolError)
	require.Equal(t, "update-protocol", repoRemoteProtocolCommand.Name())
	require.NotNil(t, repoRemoteProtocolCommand.Parent())
	require.Equal(t, "remote", repoRemoteProtocolCommand.Parent().Name())

	repoPullRequestsCommand, _, pullRequestsError := rootCommand.Find([]string{"prs", "delete"})
	require.NoError(t, pullRequestsError)
	require.Equal(t, "delete", repoPullRequestsCommand.Name())
	require.NotNil(t, repoPullRequestsCommand.Parent())
	require.Equal(t, "prs", repoPullRequestsCommand.Parent().Name())

	repoPackagesCommand, _, packagesError := rootCommand.Find([]string{"packages", "delete"})
	require.NoError(t, packagesError)
	require.Equal(t, "delete", repoPackagesCommand.Name())
	require.NotNil(t, repoPackagesCommand.Parent())
	require.Equal(t, "packages", repoPackagesCommand.Parent().Name())

	releaseCommand, _, releaseError := rootCommand.Find([]string{"release"})
	require.NoError(t, releaseError)
	require.Equal(t, "release", releaseCommand.Name())
	require.NotNil(t, releaseCommand.Parent())
	require.Equal(t, applicationNameConstant, releaseCommand.Parent().Name())

	branchDefaultCommand, _, branchDefaultError := rootCommand.Find([]string{defaultCommandNameConstant})
	require.NoError(t, branchDefaultError)
	require.Equal(t, defaultCommandNameConstant, branchDefaultCommand.Name())
	require.NotNil(t, branchDefaultCommand.Parent())
	require.Equal(t, applicationNameConstant, branchDefaultCommand.Parent().Name())

	branchChangeCommand, _, branchChangeError := rootCommand.Find([]string{branchChangeTopLevelUseNameConstant})
	require.NoError(t, branchChangeError)
	require.Equal(t, branchChangeTopLevelUseNameConstant, branchChangeCommand.Name())
	require.NotNil(t, branchChangeCommand.Parent())
	require.Equal(t, applicationNameConstant, branchChangeCommand.Parent().Name())

	commitMessageCommand, _, commitMessageError := rootCommand.Find([]string{"message", "commit"})
	require.NoError(t, commitMessageError)
	require.Equal(t, "commit", commitMessageCommand.Name())
	require.NotNil(t, commitMessageCommand.Parent())
	require.Equal(t, "message", commitMessageCommand.Parent().Name())
	require.NotNil(t, commitMessageCommand.Parent().Parent())
	require.Equal(t, applicationNameConstant, commitMessageCommand.Parent().Parent().Name())

	changelogMessageCommand, _, changelogMessageError := rootCommand.Find([]string{"message", "changelog"})
	require.NoError(t, changelogMessageError)
	require.Equal(t, "changelog", changelogMessageCommand.Name())
	require.NotNil(t, changelogMessageCommand.Parent())
	require.Equal(t, "message", changelogMessageCommand.Parent().Name())
	require.NotNil(t, changelogMessageCommand.Parent().Parent())
	require.Equal(t, applicationNameConstant, changelogMessageCommand.Parent().Parent().Name())

	_, _, legacyRenameError := rootCommand.Find([]string{"repo-folders-rename"})
	require.Error(t, legacyRenameError)
	require.Contains(t, legacyRenameError.Error(), "unknown command")

	_, _, legacyRemoteError := rootCommand.Find([]string{"repo-remote-update"})
	require.Error(t, legacyRemoteError)
	require.Contains(t, legacyRemoteError.Error(), "unknown command")

	_, _, legacyProtocolError := rootCommand.Find([]string{"repo-protocol-convert"})
	require.Error(t, legacyProtocolError)
	require.Contains(t, legacyProtocolError.Error(), "unknown command")

	_, _, legacyPullRequestsError := rootCommand.Find([]string{"repo-prs-purge"})
	require.Error(t, legacyPullRequestsError)
	require.Contains(t, legacyPullRequestsError.Error(), "unknown command")

	_, _, legacyPackagesError := rootCommand.Find([]string{"repo-packages-purge"})
	require.Error(t, legacyPackagesError)
	require.Contains(t, legacyPackagesError.Error(), "unknown command")
}

func TestApplicationHierarchicalCommandsLoadExpectedOperations(t *testing.T) {
	application := NewApplication()
	rootCommand := application.rootCommand

	folderRenameCommand, _, renameError := rootCommand.Find([]string{"folder", "rename"})
	require.NoError(t, renameError)
	require.Equal(t, []string{reposRenameOperationNameConstant}, application.operationsRequiredForCommand(folderRenameCommand))

	repoRemoteCanonicalCommand, _, canonicalError := rootCommand.Find([]string{"remote", "update-to-canonical"})
	require.NoError(t, canonicalError)
	require.Equal(t, []string{reposRemotesOperationNameConstant}, application.operationsRequiredForCommand(repoRemoteCanonicalCommand))

	repoRemoteProtocolCommand, _, protocolError := rootCommand.Find([]string{"remote", "update-protocol"})
	require.NoError(t, protocolError)
	require.Equal(t, []string{reposProtocolOperationNameConstant}, application.operationsRequiredForCommand(repoRemoteProtocolCommand))

	repoPullRequestsCommand, _, pullRequestsError := rootCommand.Find([]string{"prs", "delete"})
	require.NoError(t, pullRequestsError)
	require.Equal(t, []string{branchCleanupOperationNameConstant}, application.operationsRequiredForCommand(repoPullRequestsCommand))

	repoPackagesCommand, _, packagesError := rootCommand.Find([]string{"packages", "delete"})
	require.NoError(t, packagesError)
	require.Equal(t, []string{packagesPurgeOperationNameConstant}, application.operationsRequiredForCommand(repoPackagesCommand))

	branchDefaultCommand, _, branchDefaultError := rootCommand.Find([]string{defaultCommandNameConstant})
	require.NoError(t, branchDefaultError)
	require.Equal(t, []string{defaultOperationNameConstant}, application.operationsRequiredForCommand(branchDefaultCommand))

	commitMessageCommand, _, commitMessageError := rootCommand.Find([]string{"message", "commit"})
	require.NoError(t, commitMessageError)
	require.Equal(t, []string{commitMessageOperationNameConstant}, application.operationsRequiredForCommand(commitMessageCommand))

	changelogMessageCommand, _, changelogMessageError := rootCommand.Find([]string{"message", "changelog"})
	require.NoError(t, changelogMessageError)
	require.Equal(t, []string{changelogMessageOperationNameConstant}, application.operationsRequiredForCommand(changelogMessageCommand))
}

func TestReleaseCommandUsageIncludesTagPlaceholder(t *testing.T) {
	application := NewApplication()
	rootCommand := application.rootCommand

	releaseCommand, _, releaseError := rootCommand.Find([]string{"release"})
	require.NoError(t, releaseError)

	require.True(t, strings.HasPrefix(strings.TrimSpace(releaseCommand.Use), repoReleaseCommandUseNameConstant))
	require.Contains(t, releaseCommand.Use, "<tag>")
	require.Contains(t, releaseCommand.Long, "Provide the tag as the first argument")
	require.Contains(t, releaseCommand.Example, "gix release")
}

func TestBranchChangeCommandUsageIncludesBranchPlaceholder(t *testing.T) {
	application := NewApplication()
	rootCommand := application.rootCommand

	branchChangeCommand, _, branchChangeError := rootCommand.Find([]string{branchChangeTopLevelUseNameConstant})
	require.NoError(t, branchChangeError)

	require.True(t, strings.HasPrefix(strings.TrimSpace(branchChangeCommand.Use), branchChangeTopLevelUseNameConstant))
	require.Contains(t, branchChangeCommand.Use, "[branch]")
	require.Contains(t, branchChangeCommand.Long, "Provide the branch name as the first argument")
	require.Contains(t, branchChangeCommand.Example, "gix "+branchChangeTopLevelUseNameConstant)
}

func TestWorkflowCommandUsageIncludesConfigurationPlaceholder(t *testing.T) {
	application := NewApplication()
	rootCommand := application.rootCommand

	workflowCommand, _, workflowError := rootCommand.Find([]string{"w"})
	require.NoError(t, workflowError)

	require.Contains(t, workflowCommand.Use, "<configuration|preset>")
	require.Contains(t, workflowCommand.Long, "embedded presets")
	require.Contains(t, workflowCommand.Example, "gix workflow")
}

func TestRepoReleaseConfigurationUsesEmbeddedDefaults(t *testing.T) {
	application := NewApplication()

	command := &cobra.Command{Use: "test-command"}
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

	require.NoError(t, application.initializeConfiguration(command))

	configuration := application.repoReleaseConfiguration()
	require.Equal(t, []string{"."}, configuration.RepositoryRoots)
	require.Equal(t, "origin", configuration.RemoteName)
}

func TestInitializeConfigurationMergesEmbeddedRepoReleaseDefaults(t *testing.T) {
	temporaryDirectory := t.TempDir()
	configurationPath := filepath.Join(temporaryDirectory, "config.yaml")

	configurationContent := `common:
  log_level: info
  log_format: console
operations:
  - command: ["folder", "rename"]
    with:
      roots:
        - ./custom
`
	require.NoError(t, os.WriteFile(configurationPath, []byte(configurationContent), 0o644))

	application := NewApplication()
	application.configurationFilePath = configurationPath

	command := &cobra.Command{Use: "test-command"}
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

	require.NoError(t, application.initializeConfiguration(command))

	options, lookupError := application.operationConfigurations.Lookup(repoReleaseOperationNameConstant)
	require.NoError(t, lookupError)
	require.NotNil(t, options)

	releaseConfiguration := application.repoReleaseConfiguration()
	require.Equal(t, []string{"."}, releaseConfiguration.RepositoryRoots)
	require.Equal(t, "origin", releaseConfiguration.RemoteName)
}
