package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/llmclient"
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
	require.NotNil(t, workflowConfiguration.ConfiguredWorkflow)
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

func TestBranchSyncUsesGlobalLLMConnectionProfiles(t *testing.T) {
	application := newApplicationConfigurationTestHarness(t, ApplicationConfiguration{
		LLM: applicationTestLLMConfiguration(),
	}, nil)

	configuration := application.branchSyncConfiguration()

	require.Equal(t, 1, configuration.CommitMessage.ConnectionProfiles.LLMProxy.Priority)
	require.Equal(t, "meta", configuration.CommitMessage.ConnectionProfiles.LLMProxy.Provider)
	require.Equal(t, "muse-spark-1.1", configuration.CommitMessage.ConnectionProfiles.LLMProxy.Model)
	require.Equal(t, "proxy-secret", configuration.CommitMessage.ConnectionProfiles.LLMProxy.Credential)
	require.Equal(t, 2, configuration.CommitMessage.ConnectionProfiles.OpenAI.Priority)
}

func TestApplicationGlobalLLMDefaultsUseConfiguredConnections(t *testing.T) {
	application := newApplicationConfigurationTestHarness(t, ApplicationConfiguration{
		LLM: applicationTestLLMConfiguration(),
	}, nil)

	commitConfiguration := application.commitMessageConfiguration()
	require.Equal(t, "meta", commitConfiguration.ConnectionProfiles.LLMProxy.Provider)
	require.Equal(t, "muse-spark-1.1", commitConfiguration.ConnectionProfiles.LLMProxy.Model)
	require.Equal(t, 512, commitConfiguration.MaxTokens)

	changelogConfiguration := application.changelogMessageConfiguration()
	require.Equal(t, "meta", changelogConfiguration.ConnectionProfiles.LLMProxy.Provider)
	require.Equal(t, "muse-spark-1.1", changelogConfiguration.ConnectionProfiles.LLMProxy.Model)
	require.Equal(t, 512, changelogConfiguration.MaxTokens)

	syncConfiguration := application.branchSyncConfiguration()
	require.Equal(t, "meta", syncConfiguration.CommitMessage.ConnectionProfiles.LLMProxy.Provider)
}

func TestApplicationLLMConfigurationRequiresAtLeastOneCredential(t *testing.T) {
	configuration := applicationTestLLMConfiguration()
	configuration.LLMProxy.Credential = ""
	configuration.OpenAI.Credential = ""

	validationError := configuration.validateConnections()

	require.EqualError(t, validationError, "llm requires at least one connection credential")
}

func TestInitializeConfigurationRejectsPublicLLMTransport(t *testing.T) {
	configurationPath := filepath.Join(t.TempDir(), "config.yml")
	configurationContent := `common:
  log_level: error
  log_format: console
llm:
  transport: llm_proxy
  openai:
    priority: 2
    model: gpt-4.1
    base_url: https://api.openai.com/v1
    credential: openai-secret
  llm_proxy:
    priority: 1
    provider: meta
    model: muse-spark-1.1
    base_url: https://llm-proxy.example
    credential: proxy-secret
operations: []
`
	require.NoError(t, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))

	application := NewApplication()
	application.configurationFilePath = configurationPath

	initializationError := application.initializeConfiguration(&cobra.Command{Use: versionCommandUseNameConstant})

	require.Error(t, initializationError)
	require.Contains(t, initializationError.Error(), "transport")
}

func TestInitializeConfigurationRequiresLLMProvider(t *testing.T) {
	configurationPath := filepath.Join(t.TempDir(), "config.yml")
	configurationContent := `common:
  log_level: error
  log_format: console
llm:
  openai:
    priority: 2
    model: gpt-4.1
    base_url: https://api.openai.com/v1
    credential: openai-secret
  llm_proxy:
    priority: 1
    model: muse-spark-1.1
    base_url: https://llm-proxy.example
    credential: proxy-secret
operations: []
`
	require.NoError(t, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))

	application := NewApplication()
	application.configurationFilePath = configurationPath

	initializationError := application.initializeConfiguration(&cobra.Command{Use: versionCommandUseNameConstant})

	require.EqualError(t, initializationError, "invalid llm configuration: llm llm_proxy provider is required")
}

func TestApplicationOperationLLMProxyProviderOverrideClearsConfiguredModel(t *testing.T) {
	application := newApplicationConfigurationTestHarness(t, ApplicationConfiguration{
		LLM: applicationTestLLMConfiguration(),
	}, []ApplicationOperationConfiguration{
		{
			Command: []string{"message", "commit"},
			Options: map[string]any{
				"llm_proxy": map[string]any{
					"provider": llmclient.ProviderOpenAI,
				},
			},
		},
	})

	commitConfiguration := application.commitMessageConfiguration()
	require.Equal(t, llmclient.ProviderOpenAI, commitConfiguration.LLMProxy.Provider)
	require.Empty(t, commitConfiguration.LLMProxy.Model)
	require.Equal(t, "meta", commitConfiguration.ConnectionProfiles.LLMProxy.Provider)
	require.Equal(t, "muse-spark-1.1", commitConfiguration.ConnectionProfiles.LLMProxy.Model)

	changelogConfiguration := application.changelogMessageConfiguration()
	require.Empty(t, changelogConfiguration.LLMProxy.Provider)
	require.Equal(t, "meta", changelogConfiguration.ConnectionProfiles.LLMProxy.Provider)
}

func TestApplicationSyncCommitMessageLLMProxyProviderOverrideClearsConfiguredModel(t *testing.T) {
	application := newApplicationConfigurationTestHarness(t, ApplicationConfiguration{
		LLM: applicationTestLLMConfiguration(),
	}, []ApplicationOperationConfiguration{
		{
			Command: []string{"sync"},
			Options: map[string]any{
				"commit_message": map[string]any{
					"llm_proxy": map[string]any{
						"provider": llmclient.ProviderOpenAI,
					},
				},
			},
		},
	})

	configuration := application.branchSyncConfiguration()
	require.Equal(t, llmclient.ProviderOpenAI, configuration.CommitMessage.LLMProxy.Provider)
	require.Empty(t, configuration.CommitMessage.LLMProxy.Model)
	require.Equal(t, "meta", configuration.CommitMessage.ConnectionProfiles.LLMProxy.Provider)
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

func TestOperationConfigurationsRejectEmptyCommandPath(t *testing.T) {
	_, buildError := newOperationConfigurations([]ApplicationOperationConfiguration{
		{Command: []string{"", " "}, Options: map[string]any{"roots": []string{"."}}},
	})

	var invalidConfiguration InvalidOperationConfigurationError
	require.ErrorAs(t, buildError, &invalidConfiguration)
	require.Empty(t, invalidConfiguration.OperationName)
	require.ErrorContains(t, buildError, "configured command path is required")
}

func newApplicationConfigurationTestHarness(t *testing.T, configuration ApplicationConfiguration, definitions []ApplicationOperationConfiguration) *Application {
	t.Helper()

	configuredOperations, buildError := newOperationConfigurations(definitions)
	require.NoError(t, buildError)
	return &Application{
		logger:                            zap.NewNop(),
		configuration:                     configuration,
		configuredOperationConfigurations: configuredOperations,
		operationConfigurations:           configuredOperations,
	}
}

func applicationTestLLMConfiguration() ApplicationLLMConfiguration {
	return ApplicationLLMConfiguration{
		OpenAI: llmclient.OpenAIConnectionProfile{
			Priority:   2,
			BaseURL:    "https://api.openai.com/v1",
			Credential: "openai-secret",
			Model:      llmclient.DefaultOpenAIModel,
		},
		LLMProxy: llmclient.LLMProxyConnectionProfile{
			Priority:   1,
			BaseURL:    "https://llm-proxy.example",
			Credential: "proxy-secret",
			Provider:   "meta",
			Model:      "muse-spark-1.1",
		},
		MaxCompletionTokens: 512,
		TimeoutSeconds:      60,
	}
}

func configureApplicationWithTestConfig(t *testing.T, application *Application) {
	t.Helper()
	configurationPath := filepath.Join(t.TempDir(), "config.yml")
	configurationContent := embeddedDefaultTestConfiguration()
	require.NoError(t, os.WriteFile(configurationPath, configurationContent, 0o600))
	application.configurationFilePath = configurationPath
}

func TestInitializeConfigurationAttachesBranchContext(t *testing.T) {
	application := NewApplication()
	configurationPath := filepath.Join(t.TempDir(), "config.yml")
	configurationContent := embeddedDefaultTestConfiguration()
	require.NoError(t, os.WriteFile(configurationPath, configurationContent, 0o600))
	application.configurationFilePath = configurationPath
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

func embeddedDefaultTestConfiguration() []byte {
	configurationContent, _ := EmbeddedDefaultConfiguration()
	return []byte(strings.NewReplacer(
		"${GH_TOKEN}", "test-github-key",
		"${GITHUB_PACKAGES_TOKEN}", "test-packages-key",
		"${OPENAI_API_KEY}", "test-openai-key",
		"${LLM_PROXY_SECRET_KEY}", "test-proxy-key",
	).Replace(string(configurationContent)))
}

func TestConfigurationDiscoveryPrefersSystemFileOverUserFile(t *testing.T) {
	originalSystemConfigurationPath := systemConfigurationFilePath
	systemConfigurationFilePath = filepath.Join(t.TempDir(), "etc", "gix", "config.yml")
	t.Cleanup(func() {
		systemConfigurationFilePath = originalSystemConfigurationPath
	})

	homeDirectory := t.TempDir()
	t.Setenv("HOME", homeDirectory)
	userConfigurationPath := filepath.Join(homeDirectory, userConfigurationDirectoryNameConstant, configurationFileNameConstant)
	require.NoError(t, os.MkdirAll(filepath.Dir(systemConfigurationFilePath), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(userConfigurationPath), 0o755))
	systemConfiguration := embeddedDefaultTestConfiguration()
	userConfiguration := []byte(strings.Replace(string(systemConfiguration), "log_level: error", "log_level: debug", 1))
	require.NoError(t, os.WriteFile(systemConfigurationFilePath, systemConfiguration, 0o600))
	require.NoError(t, os.WriteFile(userConfigurationPath, userConfiguration, 0o600))

	application := NewApplication()
	command := &cobra.Command{Use: "version"}
	require.NoError(t, application.initializeConfiguration(command))
	require.Equal(t, systemConfigurationFilePath, application.ConfigFileUsed())
}

func TestConfigurationInitializationSystemFlagWritesSystemFile(t *testing.T) {
	originalSystemConfigurationPath := systemConfigurationFilePath
	systemConfigurationFilePath = filepath.Join(t.TempDir(), "etc", "gix", "config.yml")
	t.Cleanup(func() {
		systemConfigurationFilePath = originalSystemConfigurationPath
	})

	application := NewApplication()
	executionError := application.ExecuteWithOptions(ExecutionOptions{
		Arguments:      []string{"init", "--system"},
		Context:        context.Background(),
		StandardInput:  strings.NewReader(""),
		StandardOutput: io.Discard,
		StandardError:  io.Discard,
		ExitOnVersion:  false,
	})

	require.NoError(t, executionError)
	require.FileExists(t, systemConfigurationFilePath)
	configurationContent, readError := os.ReadFile(systemConfigurationFilePath)
	require.NoError(t, readError)
	require.Contains(t, string(configurationContent), `credential: "${LLM_PROXY_SECRET_KEY}"`)
}

func TestRootCommandToggleHelpFormatting(t *testing.T) {
	application := NewApplication()
	usage := application.rootCommand.PersistentFlags().FlagUsages()

	require.Contains(t, usage, "--yes <yes|NO>")
	require.NotContains(t, usage, "--init")
	require.NotContains(t, usage, "--init string")
	require.NotContains(t, usage, "__toggle_true__")
	require.NotContains(t, usage, "toggle[")

	initCommand, _, findError := application.rootCommand.Find([]string{"init"})
	require.NoError(t, findError)
	initUsage := initCommand.Flags().FlagUsages()

	require.Contains(t, initUsage, "--system <yes|NO>")
	require.NotContains(t, initUsage, "--local")
	require.NotContains(t, initUsage, "--user")
	require.Contains(t, initUsage, "--force <yes|NO>")
	require.NotContains(t, initUsage, "__toggle_true__")
	require.NotContains(t, initUsage, "toggle[")
}

func TestNormalizeWebArguments(t *testing.T) {
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
			name:         "PlainWebFlagUnchanged",
			input:        []string{"--web"},
			expectedArgs: []string{"--web"},
		},
		{
			name:         "ExplicitPortFlagUnchanged",
			input:        []string{"--web", "--port", "18080"},
			expectedArgs: []string{"--web", "--port", "18080"},
		},
		{
			name:         "LegacyPositionalPortPreservedForValidation",
			input:        []string{"--web", "19090"},
			expectedArgs: []string{"--web", "--", "19090"},
		},
		{
			name:         "BindFlagUnchanged",
			input:        []string{"--web", "--bind", "0.0.0.0"},
			expectedArgs: []string{"--web", "--bind", "0.0.0.0"},
		},
		{
			name:         "RootsFlagUnchanged",
			input:        []string{"--web", "--roots", "/tmp/fleet"},
			expectedArgs: []string{"--web", "--roots", "/tmp/fleet"},
		},
		{
			name:         "BindAndPortFlagsUnchanged",
			input:        []string{"--web", "--port", "8081"},
			expectedArgs: []string{"--web", "--port", "8081"},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			normalized := normalizeWebArguments(testCase.input)
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

	branchSyncCommand, _, branchSyncError := rootCommand.Find([]string{branchSyncTopLevelUseNameConstant})
	require.NoError(t, branchSyncError)
	require.Equal(t, branchSyncTopLevelUseNameConstant, branchSyncCommand.Name())
	require.NotNil(t, branchSyncCommand.Parent())
	require.Equal(t, applicationNameConstant, branchSyncCommand.Parent().Name())

	_, _, legacyCdError := rootCommand.Find([]string{"cd"})
	require.Error(t, legacyCdError)
	require.Contains(t, legacyCdError.Error(), "unknown command")

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
	require.Equal(t, []string{packagesDeleteOperationNameConstant}, application.operationsRequiredForCommand(repoPackagesCommand))

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

func TestBranchSyncCommandUsageIncludesBranchPlaceholder(t *testing.T) {
	application := NewApplication()
	rootCommand := application.rootCommand

	branchSyncCommand, _, branchSyncError := rootCommand.Find([]string{branchSyncTopLevelUseNameConstant})
	require.NoError(t, branchSyncError)

	require.True(t, strings.HasPrefix(strings.TrimSpace(branchSyncCommand.Use), branchSyncTopLevelUseNameConstant))
	require.Contains(t, branchSyncCommand.Use, "[remote-url|branch]")
	require.Contains(t, branchSyncCommand.Long, "PR-backed work branches")
	require.Contains(t, branchSyncCommand.Example, "gix "+branchSyncTopLevelUseNameConstant)
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

func TestRepoReleaseConfigurationUsesConfigFileValues(t *testing.T) {
	application := NewApplication()
	configurationPath := filepath.Join(t.TempDir(), "config.yml")
	configurationContent := `common:
  log_level: error
  log_format: console
  assume_yes: false
  require_clean: false
llm:
  openai:
    priority: 2
    model: gpt-4.1
    base_url: https://api.openai.com/v1
    credential: openai-secret
  llm_proxy:
    priority: 1
    provider: meta
    model: muse-spark-1.1
    base_url: https://llm-proxy.example
    credential: proxy-secret
operations:
  - command: ["release"]
    with:
      roots: ["./configured"]
      remote: upstream
`
	require.NoError(t, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))
	application.configurationFilePath = configurationPath

	command := &cobra.Command{Use: "release"}
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

	require.NoError(t, application.initializeConfiguration(command))

	configuration := application.repoReleaseConfiguration()
	require.Equal(t, []string{"./configured"}, configuration.RepositoryRoots)
	require.Equal(t, "upstream", configuration.RemoteName)
}

func TestInitializeConfigurationRejectsMissingRequiredOperation(t *testing.T) {
	temporaryDirectory := t.TempDir()
	configurationPath := filepath.Join(temporaryDirectory, "config.yml")

	configurationContent := `common:
  log_level: info
  log_format: console
llm:
  openai:
    priority: 2
    model: gpt-4.1
    base_url: https://api.openai.com/v1
    credential: openai-secret
  llm_proxy:
    priority: 1
    provider: meta
    model: muse-spark-1.1
    base_url: https://llm-proxy.example
    credential: proxy-secret
operations:
  - command: ["folder", "rename"]
    with:
      roots:
        - ./custom
`
	require.NoError(t, os.WriteFile(configurationPath, []byte(configurationContent), 0o644))

	application := NewApplication()
	application.configurationFilePath = configurationPath

	command := &cobra.Command{Use: "release"}
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

	initializationError := application.initializeConfiguration(command)
	require.Error(t, initializationError)
	require.Contains(t, initializationError.Error(), repoReleaseOperationNameConstant)
}

func TestInitializeConfigurationRejectsObsoleteOperationField(t *testing.T) {
	configurationPath := filepath.Join(t.TempDir(), "config.yml")
	configurationContent := `common:
  log_level: error
  log_format: console
llm:
  openai:
    priority: 2
    model: gpt-4.1
    base_url: https://api.openai.com/v1
    credential: openai-secret
  llm_proxy:
    priority: 1
    provider: meta
    model: muse-spark-1.1
    base_url: https://llm-proxy.example
    credential: proxy-secret
operations:
  - command: ["message", "commit"]
    with:
      roots:
        - .
      api_key_env: OPENAI_API_KEY
`
	require.NoError(t, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))

	application := NewApplication()
	application.configurationFilePath = configurationPath

	command := &cobra.Command{Use: versionCommandUseNameConstant}
	initializationError := application.initializeConfiguration(command)

	require.Error(t, initializationError)
	require.Contains(t, initializationError.Error(), commitMessageOperationNameConstant)
	require.Contains(t, initializationError.Error(), "api_key_env")
}

func TestResolveConfigurationFileRequiresCanonicalExtension(t *testing.T) {
	for _, extension := range []string{".yaml", ".YML"} {
		t.Run(extension, func(t *testing.T) {
			configurationPath := filepath.Join(t.TempDir(), "config"+extension)
			require.NoError(t, os.WriteFile(configurationPath, []byte("common: {}\n"), 0o600))

			application := NewApplication()
			application.configurationFilePath = configurationPath

			_, resolutionError := application.resolveConfigurationFile(nil)
			require.EqualError(t, resolutionError, "configuration file must use .yml: "+configurationPath)
		})
	}
}

func TestInitializeConfigurationRejectsObsoleteWorkflowLLMField(t *testing.T) {
	configurationPath := filepath.Join(t.TempDir(), "config.yml")
	configurationContent := `common:
  log_level: error
  log_format: console
llm:
  openai:
    priority: 2
    model: gpt-4.1
    base_url: https://api.openai.com/v1
    credential: openai-secret
  llm_proxy:
    priority: 1
    provider: meta
    model: muse-spark-1.1
    base_url: https://llm-proxy.example
    credential: proxy-secret
operations: []
workflow:
  - step:
      command: ["tasks", "apply"]
      with:
        llm:
          transport: openai_compatible
          llm_proxy:
            provider: openai
        tasks:
          - name: Generate message
            ensure_clean: false
`
	require.NoError(t, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))

	application := NewApplication()
	application.configurationFilePath = configurationPath

	command := &cobra.Command{Use: versionCommandUseNameConstant}
	initializationError := application.initializeConfiguration(command)

	require.Error(t, initializationError)
	require.Contains(t, initializationError.Error(), `unsupported llm configuration key "transport"`)
}
