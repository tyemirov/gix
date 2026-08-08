package cli_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mapstructure "github.com/go-viper/mapstructure/v2"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/tyemirov/gix/cmd/cli"
	repos "github.com/tyemirov/gix/cmd/cli/repos"
	workflowcmd "github.com/tyemirov/gix/cmd/cli/workflow"
	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/gix/internal/branches"
	"github.com/tyemirov/gix/internal/migrate"
	"github.com/tyemirov/gix/internal/packages"
	"github.com/tyemirov/gix/internal/utils"
	workflowpkg "github.com/tyemirov/gix/internal/workflow"
)

const (
	testConfigurationFileNameConstant                        = "config.yml"
	testLLMConfigurationBlockConstant                        = "llm:\n  openai:\n    priority: 2\n    model: gpt-5.6-terra\n    base_url: https://api.openai.com/v1\n    credential: openai-test-secret\n  llm_proxy:\n    priority: 1\n    provider: meta\n    model: muse-spark-1.1\n    base_url: https://llm-proxy.example\n    credential: proxy-test-secret\n"
	testConfigurationHeaderConstant                          = "common:\n  log_level: error\n  log_format: structured\n" + testLLMConfigurationBlockConstant + "operations:\n"
	testConsoleConfigurationHeaderConstant                   = "common:\n  log_level: error\n  log_format: console\n" + testLLMConfigurationBlockConstant + "operations:\n"
	testDebugConfigurationHeaderConstant                     = "common:\n  log_level: debug\n  log_format: structured\n" + testLLMConfigurationBlockConstant + "operations:\n"
	testDebugConsoleConfigurationHeaderConstant              = "common:\n  log_level: debug\n  log_format: console\n" + testLLMConfigurationBlockConstant + "operations:\n"
	testOperationBlockTemplateConstant                       = "  - command: %s\n    with:\n%s"
	testOperationRootsTemplateConstant                       = "      roots:\n        - %s\n"
	testOperationRootDirectoryConstant                       = "/tmp/config-root"
	testPackagesCommandNameConstant                          = "packages"
	testPackagesCommandKeyConstant                           = "packages delete"
	testBranchDefaultCommandNameConstant                     = "default"
	testBranchDefaultCommandKeyConstant                      = "default"
	testBranchCleanupCommandNameConstant                     = "prs-delete"
	testBranchCleanupCommandKeyConstant                      = "prs delete"
	testReposRemotesCommandNameConstant                      = "remote-update-to-canonical"
	testReposRemotesCommandKeyConstant                       = "remote update-to-canonical"
	testReposProtocolCommandNameConstant                     = "remote-update-protocol"
	testReposProtocolCommandKeyConstant                      = "remote update-protocol"
	testReposRenameCommandNameConstant                       = "folder-rename"
	testReposRenameCommandKeyConstant                        = "folder rename"
	testAuditCommandNameConstant                             = "audit"
	testAuditCommandKeyConstant                              = "audit"
	testWorkflowCommandNameConstant                          = "workflow"
	testWorkflowCommandKeyConstant                           = "workflow"
	testRepoReleaseCommandKeyConstant                        = "release"
	testBranchSyncCommandKeyConstant                         = "sync"
	testCommitMessageCommandKeyConstant                      = "message commit"
	testChangelogMessageCommandKeyConstant                   = "message changelog"
	embeddedDefaultsBranchCleanupTestNameConstant            = "BranchCleanupDefaults"
	embeddedDefaultsPackagesTestNameConstant                 = "PackagesDefaults"
	embeddedDefaultsReposRemotesTestNameConstant             = "ReposRemotesDefaults"
	embeddedDefaultsReposProtocolTestNameConstant            = "ReposProtocolDefaults"
	embeddedDefaultsReposRenameTestNameConstant              = "ReposRenameDefaults"
	embeddedDefaultsWorkflowTestNameConstant                 = "WorkflowDefaults"
	embeddedDefaultsBranchDefaultTestNameConstant            = "BranchDefaultDefaults"
	embeddedDefaultsAuditTestNameConstant                    = "AuditDefaults"
	embeddedDefaultRootPathConstant                          = "."
	embeddedDefaultRemoteNameConstant                        = "origin"
	embeddedDefaultPullRequestLimitConstant                  = 100
	configurationInitializedMessageTextConstant              = "configuration initialized"
	configurationInitializedConsoleTemplateConstant          = "%s | log level=%s | log format=%s | config file=%s"
	configurationLogLevelFieldNameConstant                   = "log_level"
	configurationLogFormatFieldNameConstant                  = "log_format"
	configurationFileFieldNameConstant                       = "config_file"
	testUserConfigurationDirectoryNameConstant               = ".gix"
	testXDGConfigHomeDirectoryNameConstant                   = "config"
	applicationSearchPathSubtestNameTemplateConstant         = "%d_%s"
	configurationInitializationForceRequiredTestNameConstant = "ForceRequired"
	configurationInitializationForceEnabledTestNameConstant  = "ForceEnabled"
	configurationInitializationCommandArgumentConstant       = "init"
	configurationInitializationForceFlagConstant             = "--force"
	configurationInitializationExistingContentConstant       = "common:\n  log_level: error\n"
	configurationInitializationErrorMessageFragmentConstant  = "already exists"
	configurationInitializationApplicationNameConstant       = "gix"
	configurationInitializationUserHomeEnvNameConstant       = "HOME"
)

var requiredCommandKeys = []string{
	testAuditCommandKeyConstant,
	testPackagesCommandKeyConstant,
	testBranchCleanupCommandKeyConstant,
	testReposRenameCommandKeyConstant,
	testReposRemotesCommandKeyConstant,
	testReposProtocolCommandKeyConstant,
	testRepoReleaseCommandKeyConstant,
	testWorkflowCommandKeyConstant,
	testBranchDefaultCommandKeyConstant,
	testBranchSyncCommandKeyConstant,
	testCommitMessageCommandKeyConstant,
	testChangelogMessageCommandKeyConstant,
}

func TestApplicationInitializeConfiguration(t *testing.T) {
	testCases := []struct {
		name                  string
		commandKeys           []string
		expectedErrorSample   error
		expectedOperationName string
		commandUse            string
	}{
		{
			name:        "ValidConfiguration",
			commandKeys: requiredCommandKeys,
			commandUse:  testPackagesCommandNameConstant,
		},
		{
			name: "DuplicateOperationConfiguration",
			commandKeys: append([]string{
				"audit",
				"Audit",
			}, requiredCommandKeys[1:]...),
			expectedErrorSample:   &cli.DuplicateOperationConfigurationError{},
			expectedOperationName: "audit",
			commandUse:            testPackagesCommandNameConstant,
		},
		{
			name: "CommandConfigurationMissingForTargetCommandRejected",
			commandKeys: []string{
				"audit",
				"packages delete",
				"prs delete",
				"folder rename",
				"remote update-to-canonical",
				"remote update-protocol",
				"workflow",
			},
			expectedErrorSample:   &cli.MissingOperationConfigurationError{},
			expectedOperationName: testBranchDefaultCommandKeyConstant,
			commandUse:            testBranchDefaultCommandNameConstant,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			homeDirectory := t.TempDir()
			configurationContent := buildConfigurationContent(testCase.commandKeys)
			configurationPath := filepath.Join(homeDirectory, testUserConfigurationDirectoryNameConstant, testConfigurationFileNameConstant)

			writeConfigurationFile(t, configurationPath, configurationContent)

			t.Setenv("HOME", homeDirectory)

			application := cli.NewApplication()

			executionError := application.InitializeForCommand(testCase.commandUse)

			if testCase.expectedErrorSample == nil {
				require.NoError(t, executionError)
				return
			}

			require.Error(t, executionError)

			switch testCase.expectedErrorSample.(type) {
			case *cli.DuplicateOperationConfigurationError:
				var duplicateError cli.DuplicateOperationConfigurationError
				require.ErrorAs(t, executionError, &duplicateError)
				require.Equal(t, testCase.expectedOperationName, duplicateError.OperationName)
			case *cli.MissingOperationConfigurationError:
				var missingError cli.MissingOperationConfigurationError
				require.ErrorAs(t, executionError, &missingError)
				require.Equal(t, testCase.expectedOperationName, missingError.OperationName)
			default:
				t.Fatalf("unexpected error sample type %T", testCase.expectedErrorSample)
			}
		})
	}
}

func TestApplicationInitializationLoggingModes(testInstance *testing.T) {
	testCases := []struct {
		name                string
		configurationHeader string
		assertion           func(*testing.T, string, string)
	}{
		{
			name:                "StructuredDefaultSilent",
			configurationHeader: testConfigurationHeaderConstant,
			assertion: func(t *testing.T, capturedOutput string, configurationPath string) {
				t.Helper()
				require.Empty(t, strings.TrimSpace(capturedOutput))
			},
		},
		{
			name:                "ConsoleDefaultSilent",
			configurationHeader: testConsoleConfigurationHeaderConstant,
			assertion: func(t *testing.T, capturedOutput string, configurationPath string) {
				t.Helper()
				require.Empty(t, strings.TrimSpace(capturedOutput))
			},
		},
		{
			name:                "StructuredDebugLogging",
			configurationHeader: testDebugConfigurationHeaderConstant,
			assertion: func(t *testing.T, capturedOutput string, configurationPath string) {
				t.Helper()

				trimmedOutput := strings.TrimSpace(capturedOutput)
				require.NotEmpty(t, trimmedOutput)

				logLines := strings.Split(trimmedOutput, "\n")
				require.Len(t, logLines, 1)

				var logEntry map[string]any
				require.NoError(t, json.Unmarshal([]byte(logLines[0]), &logEntry))

				levelValue, levelExists := logEntry["level"].(string)
				require.True(t, levelExists)
				require.Equal(t, "debug", strings.ToLower(levelValue))

				messageValue, messageValueExists := logEntry["msg"].(string)
				require.True(t, messageValueExists)
				require.Equal(t, configurationInitializedMessageTextConstant, messageValue)

				logLevelValue, logLevelExists := logEntry[configurationLogLevelFieldNameConstant].(string)
				require.True(t, logLevelExists)
				require.Equal(t, string(utils.LogLevelDebug), logLevelValue)

				logFormatValue, logFormatExists := logEntry[configurationLogFormatFieldNameConstant].(string)
				require.True(t, logFormatExists)
				require.Equal(t, string(utils.LogFormatStructured), logFormatValue)

				configurationFileValue, configurationFileExists := logEntry[configurationFileFieldNameConstant].(string)
				require.True(t, configurationFileExists)
				require.Equal(t, configurationPath, configurationFileValue)
			},
		},
		{
			name:                "ConsoleDebugLogging",
			configurationHeader: testDebugConsoleConfigurationHeaderConstant,
			assertion: func(t *testing.T, capturedOutput string, configurationPath string) {
				t.Helper()

				trimmedOutput := strings.TrimSpace(capturedOutput)
				require.NotEmpty(t, trimmedOutput)

				require.NotContains(t, trimmedOutput, "\""+configurationLogLevelFieldNameConstant+"\"")

				pathCandidates := []string{configurationPath}
				resolvedCandidatePath := resolveSymlinkedPath(t, configurationPath)
				if len(resolvedCandidatePath) > 0 && resolvedCandidatePath != configurationPath {
					pathCandidates = append(pathCandidates, resolvedCandidatePath)
				}

				var (
					bannerLine    string
					bannerMatched bool
				)

				for _, candidatePath := range pathCandidates {
					expectedBanner := fmt.Sprintf(
						configurationInitializedConsoleTemplateConstant,
						configurationInitializedMessageTextConstant,
						string(utils.LogLevelDebug),
						string(utils.LogFormatConsole),
						candidatePath,
					)

					if !strings.Contains(trimmedOutput, expectedBanner) {
						continue
					}

					bannerMatched = true

					for _, candidateLine := range strings.Split(trimmedOutput, "\n") {
						if strings.Contains(candidateLine, expectedBanner) {
							bannerLine = strings.TrimSpace(candidateLine)
							break
						}
					}

					if len(bannerLine) > 0 {
						break
					}
				}

				require.True(t, bannerMatched, "configuration initialization banner missing for expected paths: %v\nOutput:\n%s", pathCandidates, trimmedOutput)
				require.NotEmpty(t, bannerLine)
				require.True(t, strings.HasPrefix(bannerLine, "DEBUG"))
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		testInstance.Run(testCase.name, func(t *testing.T) {
			homeDirectory := t.TempDir()
			configurationDirectory := filepath.Join(homeDirectory, testUserConfigurationDirectoryNameConstant)
			configurationContent := buildConfigurationContentWithHeader(testCase.configurationHeader, requiredCommandKeys)
			configurationPath := filepath.Join(configurationDirectory, testConfigurationFileNameConstant)

			writeConfigurationFile(t, configurationPath, configurationContent)
			t.Setenv("HOME", homeDirectory)

			application := cli.NewApplication()
			stderrCapture := startTestStderrCapture(t)
			initializationError := application.InitializeForCommand(testPackagesCommandNameConstant)
			capturedOutput := stderrCapture.Stop(t)

			require.NoError(t, initializationError)

			rawConfigPath := application.ConfigFileUsed()
			expectedConfigPath := resolveSymlinkedPath(t, configurationPath)
			resolvedConfigPath := resolveSymlinkedPath(t, rawConfigPath)
			require.Equal(t, expectedConfigPath, resolvedConfigPath)

			testCase.assertion(t, capturedOutput, rawConfigPath)
		})
	}
}

func TestApplicationConfigurationInitializationCreatesConfiguration(testInstance *testing.T) {
	embeddedConfigurationContent, _ := cli.EmbeddedDefaultConfiguration()
	require.NotEmpty(testInstance, embeddedConfigurationContent)

	homeDirectory := testInstance.TempDir()
	testInstance.Setenv(configurationInitializationUserHomeEnvNameConstant, homeDirectory)
	expectedConfigurationPath := filepath.Join(homeDirectory, testUserConfigurationDirectoryNameConstant, testConfigurationFileNameConstant)

	originalArguments := os.Args
	os.Args = []string{configurationInitializationApplicationNameConstant, configurationInitializationCommandArgumentConstant}
	testInstance.Cleanup(func() {
		os.Args = originalArguments
	})

	application := cli.NewApplication()
	executionError := application.Execute()
	require.NoError(testInstance, executionError)

	fileContent, readError := os.ReadFile(expectedConfigurationPath)
	require.NoError(testInstance, readError)
	require.Equal(testInstance, embeddedConfigurationContent, fileContent)
}

func TestApplicationConfigurationInitializationForceHandling(testInstance *testing.T) {
	embeddedConfigurationContent, _ := cli.EmbeddedDefaultConfiguration()
	require.NotEmpty(testInstance, embeddedConfigurationContent)

	testCases := []struct {
		name        string
		arguments   []string
		expectError bool
	}{
		{
			name:        configurationInitializationForceRequiredTestNameConstant,
			arguments:   []string{configurationInitializationCommandArgumentConstant},
			expectError: true,
		},
		{
			name: configurationInitializationForceEnabledTestNameConstant,
			arguments: []string{
				configurationInitializationCommandArgumentConstant,
				configurationInitializationForceFlagConstant,
			},
			expectError: false,
		},
	}

	for testCaseIndex, testCase := range testCases {
		testInstance.Run(fmt.Sprintf(applicationSearchPathSubtestNameTemplateConstant, testCaseIndex, testCase.name), func(t *testing.T) {
			homeDirectory := t.TempDir()
			t.Setenv(configurationInitializationUserHomeEnvNameConstant, homeDirectory)
			configurationPath := filepath.Join(homeDirectory, testUserConfigurationDirectoryNameConstant, testConfigurationFileNameConstant)
			require.NoError(t, os.MkdirAll(filepath.Dir(configurationPath), 0o755))
			writeError := os.WriteFile(configurationPath, []byte(configurationInitializationExistingContentConstant), 0o600)
			require.NoError(t, writeError)

			originalArguments := os.Args
			os.Args = append([]string{configurationInitializationApplicationNameConstant}, testCase.arguments...)
			t.Cleanup(func() {
				os.Args = originalArguments
			})

			application := cli.NewApplication()
			executionError := application.Execute()

			if testCase.expectError {
				require.Error(t, executionError)
				require.Contains(t, executionError.Error(), configurationInitializationErrorMessageFragmentConstant)

				fileContent, readError := os.ReadFile(configurationPath)
				require.NoError(t, readError)
				require.Equal(t, configurationInitializationExistingContentConstant, string(fileContent))
				return
			}

			require.NoError(t, executionError)

			fileContent, readError := os.ReadFile(configurationPath)
			require.NoError(t, readError)
			require.Equal(t, embeddedConfigurationContent, fileContent)
		})
	}
}

func TestApplicationConfigurationSearchPaths(testInstance *testing.T) {
	fullConfigurationContent := buildConfigurationContent(requiredCommandKeys)
	testCases := []struct {
		name                    string
		createUserConfiguration bool
		expectMissingError      bool
	}{
		{
			name:                    "UserConfigurationPreferredOverWorkingAndXDG",
			createUserConfiguration: true,
		},
		{
			name:               "WorkingAndXDGConfigurationsIgnored",
			expectMissingError: true,
		},
	}

	for testCaseIndex, testCase := range testCases {
		testCase := testCase
		testInstance.Run(fmt.Sprintf(applicationSearchPathSubtestNameTemplateConstant, testCaseIndex, testCase.name), func(testInstance *testing.T) {
			workingDirectoryPath := testInstance.TempDir()
			homeDirectoryPath := testInstance.TempDir()
			xdgConfigHomeDirectoryPath := filepath.Join(homeDirectoryPath, testXDGConfigHomeDirectoryNameConstant)

			testInstance.Setenv("HOME", homeDirectoryPath)
			testInstance.Setenv("XDG_CONFIG_HOME", xdgConfigHomeDirectoryPath)

			homeConfigurationDirectoryPath := filepath.Join(homeDirectoryPath, testUserConfigurationDirectoryNameConstant)
			xdgConfigurationDirectoryPath := filepath.Join(xdgConfigHomeDirectoryPath, testUserConfigurationDirectoryNameConstant)

			require.NoError(testInstance, os.MkdirAll(homeConfigurationDirectoryPath, 0o755))
			require.NoError(testInstance, os.MkdirAll(xdgConfigurationDirectoryPath, 0o755))

			previousWorkingDirectoryPath, workingDirectoryResolveError := os.Getwd()
			require.NoError(testInstance, workingDirectoryResolveError)
			require.NoError(testInstance, os.Chdir(workingDirectoryPath))
			testInstance.Cleanup(func() {
				require.NoError(testInstance, os.Chdir(previousWorkingDirectoryPath))
			})

			workingDirectoryConfigurationPath := filepath.Join(workingDirectoryPath, testConfigurationFileNameConstant)
			writeConfigurationFile(testInstance, workingDirectoryConfigurationPath, fullConfigurationContent)
			xdgConfigurationPath := filepath.Join(xdgConfigurationDirectoryPath, testConfigurationFileNameConstant)
			writeConfigurationFile(testInstance, xdgConfigurationPath, fullConfigurationContent)

			if testCase.createUserConfiguration {
				homeConfigurationPath := filepath.Join(homeConfigurationDirectoryPath, testConfigurationFileNameConstant)
				writeConfigurationFile(testInstance, homeConfigurationPath, fullConfigurationContent)
			}

			application := cli.NewApplication()

			stderrCapture := startTestStderrCapture(testInstance)
			initializationError := application.InitializeForCommand(testPackagesCommandNameConstant)
			capturedOutput := stderrCapture.Stop(testInstance)

			require.Empty(testInstance, strings.TrimSpace(capturedOutput))
			if testCase.expectMissingError {
				require.Error(testInstance, initializationError)
				require.Contains(testInstance, initializationError.Error(), "requires config.yml")
				require.Empty(testInstance, application.ConfigFileUsed())
				return
			}

			require.NoError(testInstance, initializationError)
			expectedConfigurationPath := resolveSymlinkedPath(testInstance, filepath.Join(homeConfigurationDirectoryPath, testConfigurationFileNameConstant))
			require.Equal(testInstance, expectedConfigurationPath, resolveSymlinkedPath(testInstance, application.ConfigFileUsed()))
		})
	}
}

func TestApplicationConfigurationCliFlagOverridesScopes(t *testing.T) {
	workingDirectory := t.TempDir()
	homeDirectory := t.TempDir()

	t.Setenv("HOME", homeDirectory)

	require.NoError(t, os.MkdirAll(filepath.Join(homeDirectory, testUserConfigurationDirectoryNameConstant), 0o755))

	originalWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(t, workingDirectoryError)
	require.NoError(t, os.Chdir(workingDirectory))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalWorkingDirectory))
	})

	localConfigurationPath := filepath.Join(workingDirectory, testConfigurationFileNameConstant)
	userConfigurationPath := filepath.Join(homeDirectory, testUserConfigurationDirectoryNameConstant, testConfigurationFileNameConstant)

	buildHeader := func(logLevel string) string {
		return fmt.Sprintf("common:\n  log_level: %s\n  log_format: structured\n%soperations:\n", logLevel, testLLMConfigurationBlockConstant)
	}

	writeConfigurationFile(t, localConfigurationPath, buildConfigurationContentWithHeader(buildHeader("info"), requiredCommandKeys))
	writeConfigurationFile(t, userConfigurationPath, buildConfigurationContentWithHeader(buildHeader("error"), requiredCommandKeys))

	cliConfigurationDirectory := t.TempDir()
	cliConfigurationPath := filepath.Join(cliConfigurationDirectory, testConfigurationFileNameConstant)
	writeConfigurationFile(t, cliConfigurationPath, buildConfigurationContentWithHeader(buildHeader("debug"), requiredCommandKeys))

	originalArgs := os.Args
	t.Cleanup(func() {
		os.Args = originalArgs
	})

	os.Args = []string{configurationInitializationApplicationNameConstant, "--config", cliConfigurationPath}

	stdoutReader, stdoutWriter, stdoutPipeError := os.Pipe()
	require.NoError(t, stdoutPipeError)
	stderrReader, stderrWriter, stderrPipeError := os.Pipe()
	require.NoError(t, stderrPipeError)

	originalStdout := os.Stdout
	originalStderr := os.Stderr
	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter

	application := cli.NewApplication()
	executionError := application.Execute()

	require.NoError(t, stdoutWriter.Close())
	require.NoError(t, stderrWriter.Close())
	os.Stdout = originalStdout
	os.Stderr = originalStderr

	_, stdoutReadError := io.ReadAll(stdoutReader)
	require.NoError(t, stdoutReadError)
	require.NoError(t, stdoutReader.Close())

	_, stderrReadError := io.ReadAll(stderrReader)
	require.NoError(t, stderrReadError)
	require.NoError(t, stderrReader.Close())

	require.NoError(t, executionError)

	expectedConfigPath := resolveSymlinkedPath(t, cliConfigurationPath)
	actualConfigPath := resolveSymlinkedPath(t, application.ConfigFileUsed())
	require.Equal(t, expectedConfigPath, actualConfigPath)
}

func TestCanonicalConfigurationTemplateProvidesCompleteCommandConfigurations(testInstance *testing.T) {
	embeddedConfiguration := decodeEmbeddedApplicationConfiguration(testInstance)
	require.Equal(testInstance, "${GH_TOKEN}", embeddedConfiguration.GitHub.Credential)
	require.Equal(testInstance, 2, embeddedConfiguration.LLM.OpenAI.Priority)
	require.Equal(testInstance, "gpt-5.6-terra", embeddedConfiguration.LLM.OpenAI.Model)
	require.Equal(testInstance, "https://api.openai.com/v1", embeddedConfiguration.LLM.OpenAI.BaseURL)
	require.Equal(testInstance, "${OPENAI_API_KEY}", embeddedConfiguration.LLM.OpenAI.Credential)
	require.Equal(testInstance, "high", embeddedConfiguration.LLM.OpenAI.Effort)
	require.Equal(testInstance, 1, embeddedConfiguration.LLM.LLMProxy.Priority)
	require.Equal(testInstance, "meta", embeddedConfiguration.LLM.LLMProxy.Provider)
	require.Equal(testInstance, "muse-spark-1.1", embeddedConfiguration.LLM.LLMProxy.Model)
	require.Equal(testInstance, "https://llm-proxy-api.mprlab.com", embeddedConfiguration.LLM.LLMProxy.BaseURL)
	require.Equal(testInstance, "${LLM_PROXY_SECRET_KEY}", embeddedConfiguration.LLM.LLMProxy.Credential)

	operationIndex := buildEmbeddedOperationIndex(testInstance)

	testCases := []struct {
		name       string
		commandUse string
		commandKey string
		assertion  func(testing.TB, map[string]any)
	}{
		{
			name:       embeddedDefaultsBranchCleanupTestNameConstant,
			commandUse: testBranchCleanupCommandNameConstant,
			commandKey: testBranchCleanupCommandKeyConstant,
			assertion: func(assertionTarget testing.TB, options map[string]any) {
				assertionTarget.Helper()

				var configuration branches.CommandConfiguration
				decodeOperationOptions(assertionTarget, options, &configuration)
				sanitized := configuration.Sanitize()

				assertions := require.New(assertionTarget)
				assertions.Equal(embeddedDefaultRemoteNameConstant, sanitized.RemoteName)
				assertions.Equal(embeddedDefaultPullRequestLimitConstant, sanitized.PullRequestLimit)
				assertions.Equal([]string{embeddedDefaultRootPathConstant}, sanitized.RepositoryRoots)
			},
		},
		{
			name:       embeddedDefaultsPackagesTestNameConstant,
			commandUse: testPackagesCommandNameConstant,
			commandKey: testPackagesCommandKeyConstant,
			assertion: func(assertionTarget testing.TB, options map[string]any) {
				assertionTarget.Helper()

				var configuration packages.DeleteConfiguration
				decodeOperationOptions(assertionTarget, options, &configuration)
				sanitized := configuration.Sanitize()

				assertions := require.New(assertionTarget)
				assertions.Equal([]string{embeddedDefaultRootPathConstant}, sanitized.RepositoryRoots)
				assertions.Equal("https://api.github.com", sanitized.BaseURL)
				assertions.Equal("${GITHUB_PACKAGES_TOKEN}", sanitized.Credential)
			},
		},
		{
			name:       embeddedDefaultsReposRemotesTestNameConstant,
			commandUse: testReposRemotesCommandNameConstant,
			commandKey: testReposRemotesCommandKeyConstant,
			assertion: func(assertionTarget testing.TB, options map[string]any) {
				assertionTarget.Helper()

				var configuration repos.RemotesConfiguration
				decodeOperationOptions(assertionTarget, options, &configuration)

				assertions := require.New(assertionTarget)
				assertions.Equal([]string{embeddedDefaultRootPathConstant}, configuration.RepositoryRoots)
			},
		},
		{
			name:       embeddedDefaultsReposProtocolTestNameConstant,
			commandUse: testReposProtocolCommandNameConstant,
			commandKey: testReposProtocolCommandKeyConstant,
			assertion: func(assertionTarget testing.TB, options map[string]any) {
				assertionTarget.Helper()

				var configuration repos.ProtocolConfiguration
				decodeOperationOptions(assertionTarget, options, &configuration)

				assertions := require.New(assertionTarget)
				assertions.Equal([]string{embeddedDefaultRootPathConstant}, configuration.RepositoryRoots)
				assertions.Empty(strings.TrimSpace(configuration.FromProtocol))
				assertions.Empty(strings.TrimSpace(configuration.ToProtocol))
			},
		},
		{
			name:       embeddedDefaultsReposRenameTestNameConstant,
			commandUse: testReposRenameCommandNameConstant,
			commandKey: testReposRenameCommandKeyConstant,
			assertion: func(assertionTarget testing.TB, options map[string]any) {
				assertionTarget.Helper()

				var configuration repos.RenameConfiguration
				decodeOperationOptions(assertionTarget, options, &configuration)

				assertions := require.New(assertionTarget)
				assertions.Equal([]string{embeddedDefaultRootPathConstant}, configuration.RepositoryRoots)
			},
		},
		{
			name:       embeddedDefaultsWorkflowTestNameConstant,
			commandUse: testWorkflowCommandNameConstant,
			commandKey: testWorkflowCommandKeyConstant,
			assertion: func(assertionTarget testing.TB, options map[string]any) {
				assertionTarget.Helper()

				var configuration workflowcmd.CommandConfiguration
				decodeOperationOptions(assertionTarget, options, &configuration)
				sanitized := configuration.Sanitize()

				assertions := require.New(assertionTarget)
				assertions.Equal([]string{embeddedDefaultRootPathConstant}, sanitized.Roots)
			},
		},
		{
			name:       embeddedDefaultsBranchDefaultTestNameConstant,
			commandUse: testBranchDefaultCommandNameConstant,
			commandKey: testBranchDefaultCommandKeyConstant,
			assertion: func(assertionTarget testing.TB, options map[string]any) {
				assertionTarget.Helper()

				var configuration migrate.CommandConfiguration
				decodeOperationOptions(assertionTarget, options, &configuration)
				sanitized := configuration.Sanitize()

				assertions := require.New(assertionTarget)
				assertions.Equal([]string{embeddedDefaultRootPathConstant}, sanitized.RepositoryRoots)
				assertions.Equal(migrate.BranchMaster, migrate.BranchName(sanitized.TargetBranch))
			},
		},
		{
			name:       embeddedDefaultsAuditTestNameConstant,
			commandUse: testAuditCommandNameConstant,
			commandKey: testAuditCommandKeyConstant,
			assertion: func(assertionTarget testing.TB, options map[string]any) {
				assertionTarget.Helper()

				var configuration audit.CommandConfiguration
				decodeOperationOptions(assertionTarget, options, &configuration)
				sanitized := configuration.Sanitize()

				assertions := require.New(assertionTarget)
				assertions.Equal([]string{embeddedDefaultRootPathConstant}, sanitized.Roots)
			},
		},
		{
			name:       "CommitMessageDefaults",
			commandUse: testCommitMessageCommandKeyConstant,
			commandKey: testCommitMessageCommandKeyConstant,
			assertion: func(assertionTarget testing.TB, options map[string]any) {
				assertionTarget.Helper()

				assertions := require.New(assertionTarget)
				assertions.NotContains(options, "provider")
				assertions.NotContains(options, "api_key_env")
				assertions.NotContains(options, "base_url")
				assertions.Equal(256, options["max_completion_tokens"])
			},
		},
		{
			name:       "ChangelogMessageDefaults",
			commandUse: testChangelogMessageCommandKeyConstant,
			commandKey: testChangelogMessageCommandKeyConstant,
			assertion: func(assertionTarget testing.TB, options map[string]any) {
				assertionTarget.Helper()

				assertions := require.New(assertionTarget)
				assertions.NotContains(options, "provider")
				assertions.NotContains(options, "api_key_env")
				assertions.NotContains(options, "base_url")
				assertions.Equal(1200, options["max_completion_tokens"])
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		testInstance.Run(testCase.name, func(t *testing.T) {
			normalizedCommandKey := normalizeCommandKey(testCase.commandKey)
			operationOptions, exists := operationIndex[normalizedCommandKey]
			require.True(t, exists)

			testCase.assertion(t, operationOptions)
		})
	}
}

func resolveSymlinkedPath(testingInstance testing.TB, candidatePath string) string {
	testingInstance.Helper()
	trimmedPath := strings.TrimSpace(candidatePath)
	if len(trimmedPath) == 0 {
		return ""
	}

	resolvedPath, resolveError := filepath.EvalSymlinks(trimmedPath)
	require.NoError(testingInstance, resolveError)
	return resolvedPath
}

func buildConfigurationContent(commandKeys []string) string {
	return buildConfigurationContentWithHeader(testConfigurationHeaderConstant, commandKeys)
}

func buildConfigurationContentWithHeader(commonHeader string, commandKeys []string) string {
	configurationBuilder := strings.Builder{}
	configurationBuilder.WriteString(commonHeader)

	for _, commandKey := range commandKeys {
		rootsBlock := fmt.Sprintf(testOperationRootsTemplateConstant, testOperationRootDirectoryConstant)
		commandLiteral := formatCommandArray(commandKey)
		operationBlock := fmt.Sprintf(testOperationBlockTemplateConstant, commandLiteral, rootsBlock)
		configurationBuilder.WriteString(operationBlock)
	}

	return configurationBuilder.String()
}

func formatCommandArray(commandKey string) string {
	parts := strings.Fields(commandKey)
	if len(parts) == 0 {
		return "[]"
	}
	quotedParts := make([]string, len(parts))
	for index := range parts {
		quotedParts[index] = fmt.Sprintf("\"%s\"", parts[index])
	}
	return fmt.Sprintf("[%s]", strings.Join(quotedParts, ", "))
}

func normalizeCommandKey(commandKey string) string {
	parts := strings.Fields(commandKey)
	return workflowpkg.CommandPathKey(parts)
}

func writeConfigurationFile(t *testing.T, configurationPath string, configurationContent string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(configurationPath), 0o755))
	writeError := os.WriteFile(configurationPath, []byte(configurationContent), 0o600)
	require.NoError(t, writeError)
}

func buildEmbeddedOperationIndex(testingInstance testing.TB) map[string]map[string]any {
	testingInstance.Helper()

	configuration := decodeEmbeddedApplicationConfiguration(testingInstance)
	operationIndex := make(map[string]map[string]any)

	for _, operation := range configuration.Operations {
		commandKey := workflowpkg.CommandPathKey(operation.Command)
		if len(commandKey) == 0 {
			continue
		}

		duplicatedOptions := make(map[string]any, len(operation.Options))
		for optionKey, optionValue := range operation.Options {
			duplicatedOptions[optionKey] = optionValue
		}

		operationIndex[commandKey] = duplicatedOptions
	}

	return operationIndex
}

func decodeEmbeddedApplicationConfiguration(testingInstance testing.TB) cli.ApplicationConfiguration {
	testingInstance.Helper()

	configurationData, _ := cli.EmbeddedDefaultConfiguration()
	rawConfiguration := map[string]any{}
	require.NoError(testingInstance, yaml.Unmarshal(configurationData, &rawConfiguration))

	var configuration cli.ApplicationConfiguration
	decoder, decoderError := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          "mapstructure",
		Result:           &configuration,
		ErrorUnused:      true,
		WeaklyTypedInput: true,
	})
	require.NoError(testingInstance, decoderError)
	require.NoError(testingInstance, decoder.Decode(rawConfiguration))

	return configuration
}

func decodeOperationOptions(testingInstance testing.TB, options map[string]any, target any) {
	testingInstance.Helper()

	decoder, decoderError := mapstructure.NewDecoder(&mapstructure.DecoderConfig{TagName: "mapstructure", Result: target})
	require.NoError(testingInstance, decoderError)

	decodeError := decoder.Decode(options)
	require.NoError(testingInstance, decodeError)
}

type testStderrCapture struct {
	originalDescriptor *os.File
	reader             *os.File
	writer             *os.File
}

func startTestStderrCapture(testingInstance testing.TB) testStderrCapture {
	testingInstance.Helper()

	reader, writer, pipeError := os.Pipe()
	require.NoError(testingInstance, pipeError)

	capture := testStderrCapture{
		originalDescriptor: os.Stderr,
		reader:             reader,
		writer:             writer,
	}

	os.Stderr = writer

	return capture
}

func (capture *testStderrCapture) Stop(testingInstance testing.TB) string {
	testingInstance.Helper()

	os.Stderr = capture.originalDescriptor

	require.NoError(testingInstance, capture.writer.Close())

	capturedBytes, readError := io.ReadAll(capture.reader)
	require.NoError(testingInstance, readError)

	require.NoError(testingInstance, capture.reader.Close())

	return string(capturedBytes)
}
