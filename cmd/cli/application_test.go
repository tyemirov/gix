package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mapstructure "github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/cmd/cli"
	repos "github.com/temirov/gix/cmd/cli/repos"
	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/audit"
	"github.com/temirov/gix/internal/branches"
	"github.com/temirov/gix/internal/migrate"
	"github.com/temirov/gix/internal/packages"
	"github.com/temirov/gix/internal/utils"
	workflowpkg "github.com/temirov/gix/internal/workflow"
)

const (
	testConfigurationFileNameConstant                        = "config.yaml"
	testConfigurationHeaderConstant                          = "common:\n  log_level: error\n  log_format: structured\noperations:\n"
	testConsoleConfigurationHeaderConstant                   = "common:\n  log_level: error\n  log_format: console\noperations:\n"
	testDebugConfigurationHeaderConstant                     = "common:\n  log_level: debug\n  log_format: structured\noperations:\n"
	testDebugConsoleConfigurationHeaderConstant              = "common:\n  log_level: debug\n  log_format: console\noperations:\n"
	testOperationBlockTemplateConstant                       = "  - command: %s\n    with:\n%s"
	testOperationRootsTemplateConstant                       = "      roots:\n        - %s\n"
	testOperationRootDirectoryConstant                       = "/tmp/config-root"
	testConfigurationSearchPathEnvironmentName               = "GIX_CONFIG_SEARCH_PATH"
	testPackagesCommandNameConstant                          = "packages"
	testPackagesCommandKeyConstant                           = "packages delete"
	testBranchDefaultCommandNameConstant                     = "branch-default"
	testBranchDefaultCommandKeyConstant                      = "branch-default"
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
	testBranchChangeCommandKeyConstant                       = "cd"
	testCommitMessageCommandKeyConstant                      = "commit message"
	testChangelogMessageCommandKeyConstant                   = "changelog message"
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
	testCaseWorkingDirectoryPreferredMessageConstant         = "WorkingDirectoryPreferred"
	testCaseXDGDirectoryFallbackMessageConstant              = "XDGDirectoryFallback"
	testCaseHomeDirectoryFallbackMessageConstant             = "HomeDirectoryFallback"
	applicationSearchPathSubtestNameTemplateConstant         = "%d_%s"
	configurationDirectoryRoleWorkingConstant                = "working"
	configurationDirectoryRoleXDGConstant                    = "xdg"
	configurationDirectoryRoleHomeConstant                   = "home"
	configurationInitializationLocalTestNameConstant         = "LocalScope"
	configurationInitializationUserTestNameConstant          = "UserScope"
	configurationInitializationForceRequiredTestNameConstant = "ForceRequired"
	configurationInitializationForceEnabledTestNameConstant  = "ForceEnabled"
	configurationInitializationArgumentsLocalConstant        = "--init"
	configurationInitializationArgumentsUserConstant         = "--init=user"
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
	testBranchChangeCommandKeyConstant,
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
			name: "CommandConfigurationMissingForTargetCommandIgnored",
			commandKeys: []string{
				"audit",
				"packages delete",
				"prs delete",
				"folder rename",
				"remote update-to-canonical",
				"remote update-protocol",
				"workflow",
			},
			commandUse: testBranchDefaultCommandNameConstant,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			temporaryDirectory := t.TempDir()
			configurationContent := buildConfigurationContent(testCase.commandKeys)
			configurationPath := filepath.Join(temporaryDirectory, testConfigurationFileNameConstant)

			writeConfigurationFile(t, configurationPath, configurationContent)

			t.Setenv(testConfigurationSearchPathEnvironmentName, temporaryDirectory)

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
			configurationDirectory := t.TempDir()
			configurationContent := buildConfigurationContentWithHeader(testCase.configurationHeader, requiredCommandKeys)
			configurationPath := filepath.Join(configurationDirectory, testConfigurationFileNameConstant)

			writeConfigurationFile(t, configurationPath, configurationContent)
			t.Setenv(testConfigurationSearchPathEnvironmentName, configurationDirectory)

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

	testCases := []struct {
		name      string
		arguments []string
		setup     func(*testing.T) string
	}{
		{
			name:      configurationInitializationLocalTestNameConstant,
			arguments: []string{configurationInitializationArgumentsLocalConstant},
			setup: func(t *testing.T) string {
				workingDirectory := t.TempDir()
				originalWorkingDirectory, workingDirectoryError := os.Getwd()
				require.NoError(t, workingDirectoryError)
				require.NoError(t, os.Chdir(workingDirectory))
				t.Cleanup(func() {
					require.NoError(t, os.Chdir(originalWorkingDirectory))
				})

				return filepath.Join(workingDirectory, testConfigurationFileNameConstant)
			},
		},
		{
			name:      configurationInitializationUserTestNameConstant,
			arguments: []string{configurationInitializationArgumentsUserConstant},
			setup: func(t *testing.T) string {
				workingDirectory := t.TempDir()
				originalWorkingDirectory, workingDirectoryError := os.Getwd()
				require.NoError(t, workingDirectoryError)
				require.NoError(t, os.Chdir(workingDirectory))
				t.Cleanup(func() {
					require.NoError(t, os.Chdir(originalWorkingDirectory))
				})

				homeDirectory := t.TempDir()
				t.Setenv(configurationInitializationUserHomeEnvNameConstant, homeDirectory)

				return filepath.Join(homeDirectory, testUserConfigurationDirectoryNameConstant, testConfigurationFileNameConstant)
			},
		},
	}

	for testCaseIndex, testCase := range testCases {
		testInstance.Run(fmt.Sprintf(applicationSearchPathSubtestNameTemplateConstant, testCaseIndex, testCase.name), func(t *testing.T) {
			expectedConfigurationPath := testCase.setup(t)

			originalArguments := os.Args
			os.Args = append([]string{configurationInitializationApplicationNameConstant}, testCase.arguments...)
			t.Cleanup(func() {
				os.Args = originalArguments
			})

			application := cli.NewApplication()
			executionError := application.Execute()
			require.NoError(t, executionError)

			fileContent, readError := os.ReadFile(expectedConfigurationPath)
			require.NoError(t, readError)
			require.Equal(t, embeddedConfigurationContent, fileContent)
		})
	}
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
			arguments:   []string{configurationInitializationArgumentsLocalConstant},
			expectError: true,
		},
		{
			name: configurationInitializationForceEnabledTestNameConstant,
			arguments: []string{
				configurationInitializationArgumentsLocalConstant,
				configurationInitializationForceFlagConstant,
			},
			expectError: false,
		},
	}

	for testCaseIndex, testCase := range testCases {
		testInstance.Run(fmt.Sprintf(applicationSearchPathSubtestNameTemplateConstant, testCaseIndex, testCase.name), func(t *testing.T) {
			workingDirectory := t.TempDir()
			originalWorkingDirectory, workingDirectoryError := os.Getwd()
			require.NoError(t, workingDirectoryError)
			require.NoError(t, os.Chdir(workingDirectory))
			t.Cleanup(func() {
				require.NoError(t, os.Chdir(originalWorkingDirectory))
			})

			configurationPath := filepath.Join(workingDirectory, testConfigurationFileNameConstant)
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
		name                                string
		createWorkingDirectoryConfiguration bool
		createXDGConfiguration              bool
		createHomeConfiguration             bool
		workingDirectoryConfiguration       string
		expectedDirectoryRole               string
	}{
		{
			name:                                testCaseWorkingDirectoryPreferredMessageConstant,
			createWorkingDirectoryConfiguration: true,
			createXDGConfiguration:              true,
			createHomeConfiguration:             true,
			workingDirectoryConfiguration:       testConfigurationHeaderConstant,
			expectedDirectoryRole:               configurationDirectoryRoleWorkingConstant,
		},
		{
			name:                                testCaseXDGDirectoryFallbackMessageConstant,
			createWorkingDirectoryConfiguration: false,
			createXDGConfiguration:              true,
			createHomeConfiguration:             true,
			workingDirectoryConfiguration:       "",
			expectedDirectoryRole:               configurationDirectoryRoleXDGConstant,
		},
		{
			name:                                testCaseHomeDirectoryFallbackMessageConstant,
			createWorkingDirectoryConfiguration: false,
			createXDGConfiguration:              false,
			createHomeConfiguration:             true,
			workingDirectoryConfiguration:       "",
			expectedDirectoryRole:               configurationDirectoryRoleHomeConstant,
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
			testInstance.Setenv(testConfigurationSearchPathEnvironmentName, "")

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

			if testCase.createWorkingDirectoryConfiguration {
				workingDirectoryConfigurationPath := filepath.Join(workingDirectoryPath, testConfigurationFileNameConstant)
				writeConfigurationFile(testInstance, workingDirectoryConfigurationPath, testCase.workingDirectoryConfiguration)
			}

			if testCase.createXDGConfiguration {
				xdgConfigurationPath := filepath.Join(xdgConfigurationDirectoryPath, testConfigurationFileNameConstant)
				writeConfigurationFile(testInstance, xdgConfigurationPath, fullConfigurationContent)
			}

			if testCase.createHomeConfiguration {
				homeConfigurationPath := filepath.Join(homeConfigurationDirectoryPath, testConfigurationFileNameConstant)
				writeConfigurationFile(testInstance, homeConfigurationPath, fullConfigurationContent)
			}

			expectedConfigurationPathByRole := map[string]string{
				configurationDirectoryRoleWorkingConstant: filepath.Join(workingDirectoryPath, testConfigurationFileNameConstant),
				configurationDirectoryRoleXDGConstant:     filepath.Join(xdgConfigurationDirectoryPath, testConfigurationFileNameConstant),
				configurationDirectoryRoleHomeConstant:    filepath.Join(homeConfigurationDirectoryPath, testConfigurationFileNameConstant),
			}

			expectedConfigurationPath, expectedPathKnown := expectedConfigurationPathByRole[testCase.expectedDirectoryRole]
			require.True(testInstance, expectedPathKnown, "unexpected directory role %s", testCase.expectedDirectoryRole)
			expectedConfigurationPath = resolveSymlinkedPath(testInstance, expectedConfigurationPath)

			application := cli.NewApplication()

			stderrCapture := startTestStderrCapture(testInstance)
			initializationError := application.InitializeForCommand(testPackagesCommandNameConstant)
			capturedOutput := stderrCapture.Stop(testInstance)

			require.NoError(testInstance, initializationError)
			require.Empty(testInstance, strings.TrimSpace(capturedOutput))

			configurationFilePath := resolveSymlinkedPath(testInstance, application.ConfigFileUsed())
			require.Equal(testInstance, expectedConfigurationPath, configurationFilePath)
		})
	}
}

func TestApplicationConfigurationCliFlagOverridesScopes(t *testing.T) {
	workingDirectory := t.TempDir()
	homeDirectory := t.TempDir()
	xdgConfigHome := filepath.Join(homeDirectory, testXDGConfigHomeDirectoryNameConstant)

	t.Setenv("HOME", homeDirectory)
	t.Setenv("XDG_CONFIG_HOME", xdgConfigHome)
	t.Setenv(testConfigurationSearchPathEnvironmentName, "")

	require.NoError(t, os.MkdirAll(filepath.Join(homeDirectory, testUserConfigurationDirectoryNameConstant), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(xdgConfigHome, testUserConfigurationDirectoryNameConstant), 0o755))

	originalWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(t, workingDirectoryError)
	require.NoError(t, os.Chdir(workingDirectory))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalWorkingDirectory))
	})

	localConfigurationPath := filepath.Join(workingDirectory, testConfigurationFileNameConstant)
	xdgConfigurationPath := filepath.Join(xdgConfigHome, testUserConfigurationDirectoryNameConstant, testConfigurationFileNameConstant)
	userConfigurationPath := filepath.Join(homeDirectory, testUserConfigurationDirectoryNameConstant, testConfigurationFileNameConstant)

	buildHeader := func(logLevel string) string {
		return fmt.Sprintf("common:\n  log_level: %s\n  log_format: structured\noperations:\n", logLevel)
	}

	writeConfigurationFile(t, localConfigurationPath, buildConfigurationContentWithHeader(buildHeader("info"), requiredCommandKeys))
	writeConfigurationFile(t, xdgConfigurationPath, buildConfigurationContentWithHeader(buildHeader("warn"), requiredCommandKeys))
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

func TestApplicationEmbeddedDefaultsProvideCommandConfigurations(testInstance *testing.T) {
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

				var configuration packages.PurgeConfiguration
				decodeOperationOptions(assertionTarget, options, &configuration)
				sanitized := configuration.Sanitize()

				assertions := require.New(assertionTarget)
				assertions.Equal([]string{embeddedDefaultRootPathConstant}, sanitized.RepositoryRoots)
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
	}

	for _, testCase := range testCases {
		testCase := testCase
		testInstance.Run(testCase.name, func(t *testing.T) {
			t.Setenv(testConfigurationSearchPathEnvironmentName, t.TempDir())

			application := cli.NewApplication()
			initializationError := application.InitializeForCommand(testCase.commandUse)
			require.NoError(t, initializationError)

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

	configurationData, configurationType := cli.EmbeddedDefaultConfiguration()
	viperInstance := viper.New()
	viperInstance.SetConfigType(configurationType)

	readError := viperInstance.ReadConfig(bytes.NewReader(configurationData))
	require.NoError(testingInstance, readError)

	var configuration cli.ApplicationConfiguration
	unmarshalError := viperInstance.Unmarshal(&configuration)
	require.NoError(testingInstance, unmarshalError)

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
