package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	integrationInfoMessageConstant                     = "\"msg\":\"gix CLI executed\""
	integrationDebugMessageConstant                    = "\"msg\":\"gix CLI diagnostics\""
	integrationLogLevelEnvKeyConstant                  = "GIX_COMMON_LOG_LEVEL"
	integrationConfigFileNameConstant                  = "config.yaml"
	integrationConfigTemplateConstant                  = "common:\n  log_level: %s\n  log_format: %s\n"
	integrationDefaultCaseNameConstant                 = "default_info"
	integrationConfigCaseNameConstant                  = "config_debug"
	integrationEnvironmentCaseNameConstant             = "environment_error"
	integrationDebugLevelConstant                      = "debug"
	integrationErrorLevelConstant                      = "error"
	integrationCommandTimeout                          = 5 * time.Second
	integrationConfigFlagTemplateConstant              = "--config=%s"
	integrationEnvironmentAssignmentTemplateConstant   = "%s=%s"
	integrationSubtestNameTemplateConstant             = "%d_%s"
	integrationHelpUsagePrefixConstant                 = "Usage:"
	integrationHelpDescriptionSnippetConstant          = "gix ships reusable helpers that integrate Git, GitHub CLI, and related tooling."
	integrationHelpCaseNameConstant                    = "help_output"
	integrationStructuredLogCaseNameConstant           = "structured_default"
	integrationConsoleLogCaseNameConstant              = "console_format"
	integrationConsoleLogFlagConstant                  = "--log-format=console"
	integrationStructuredLogFormatConstant             = "structured"
	integrationConfigurationInitializedSnippetConstant = "configuration initialized"
	integrationGoBinaryNameConstant                    = "go"
	integrationGoRunSubcommandConstant                 = "run"
	integrationCurrentDirectoryArgumentConstant        = "."
	integrationStderrPipeCaseNameConstant              = "stderr_pipe"
)

func writeIntegrationConfiguration(
	testInstance *testing.T,
	directory string,
	logLevel string,
) string {
	if len(logLevel) == 0 {
		logLevel = integrationErrorLevelConstant
	}

	configurationPath := filepath.Join(directory, integrationConfigFileNameConstant)
	configurationContent := fmt.Sprintf(
		integrationConfigTemplateConstant,
		logLevel,
		integrationStructuredLogFormatConstant,
	)

	writeError := os.WriteFile(configurationPath, []byte(configurationContent), 0o600)
	require.NoError(testInstance, writeError)

	return configurationPath
}

func TestCLIIntegrationLogLevels(testInstance *testing.T) {
	testCases := []struct {
		name                 string
		configurationLevel   string
		environmentLevel     string
		extraArguments       []string
		expectedInfoVisible  bool
		expectedDebugVisible bool
	}{
		{
			name:                 integrationDefaultCaseNameConstant,
			configurationLevel:   "",
			environmentLevel:     "",
			extraArguments:       nil,
			expectedInfoVisible:  false,
			expectedDebugVisible: false,
		},
		{
			name:                 integrationConfigCaseNameConstant,
			configurationLevel:   integrationDebugLevelConstant,
			environmentLevel:     "",
			extraArguments:       nil,
			expectedInfoVisible:  true,
			expectedDebugVisible: true,
		},
		{
			name:                 integrationEnvironmentCaseNameConstant,
			configurationLevel:   "",
			environmentLevel:     integrationErrorLevelConstant,
			extraArguments:       nil,
			expectedInfoVisible:  false,
			expectedDebugVisible: false,
		},
		{
			name:                 "flag_log_level",
			configurationLevel:   "",
			environmentLevel:     "",
			extraArguments:       []string{"--log-level=debug"},
			expectedInfoVisible:  true,
			expectedDebugVisible: true,
		},
	}

	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRootDirectory := filepath.Dir(currentWorkingDirectory)

	for testCaseIndex, testCase := range testCases {
		testInstance.Run(fmt.Sprintf(integrationSubtestNameTemplateConstant, testCaseIndex, testCase.name), func(testInstance *testing.T) {
			arguments := []string{integrationGoRunSubcommandConstant, integrationCurrentDirectoryArgumentConstant}
			environment := os.Environ()
			tempDirectory := testInstance.TempDir()

			configurationPath := writeIntegrationConfiguration(testInstance, tempDirectory, testCase.configurationLevel)
			arguments = append(arguments, fmt.Sprintf(integrationConfigFlagTemplateConstant, configurationPath))
			if len(testCase.extraArguments) > 0 {
				arguments = append(arguments, testCase.extraArguments...)
			}

			if len(testCase.environmentLevel) > 0 {
				environment = append(environment, fmt.Sprintf(integrationEnvironmentAssignmentTemplateConstant, integrationLogLevelEnvKeyConstant, testCase.environmentLevel))
			}

			executionContext, cancelFunction := context.WithTimeout(context.Background(), integrationCommandTimeout)
			defer cancelFunction()

			command := exec.CommandContext(executionContext, integrationGoBinaryNameConstant, arguments...)
			command.Dir = repositoryRootDirectory
			command.Env = environment

			outputBytes, runError := command.CombinedOutput()
			outputText := string(outputBytes)
			require.NoError(testInstance, runError, outputText)

			if testCase.expectedInfoVisible {
				require.Contains(testInstance, outputText, integrationInfoMessageConstant)
			} else {
				require.NotContains(testInstance, outputText, integrationInfoMessageConstant)
			}

			if testCase.expectedDebugVisible {
				require.Contains(testInstance, outputText, integrationDebugMessageConstant)
			} else {
				require.NotContains(testInstance, outputText, integrationDebugMessageConstant)
			}
		})
	}
}

func TestCLIIntegrationDisplaysHelpWhenNoArgumentsProvided(testInstance *testing.T) {
	testCases := []struct {
		name             string
		expectedSnippets []string
	}{
		{
			name: integrationHelpCaseNameConstant,
			expectedSnippets: []string{
				integrationHelpUsagePrefixConstant,
				integrationHelpDescriptionSnippetConstant,
			},
		},
	}

	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRootDirectory := filepath.Dir(currentWorkingDirectory)

	for testCaseIndex, testCase := range testCases {
		testInstance.Run(fmt.Sprintf(integrationSubtestNameTemplateConstant, testCaseIndex, testCase.name), func(testInstance *testing.T) {
			commandArguments := []string{integrationGoRunSubcommandConstant, integrationCurrentDirectoryArgumentConstant}
			tempDirectory := testInstance.TempDir()
			configurationPath := writeIntegrationConfiguration(testInstance, tempDirectory, integrationDebugLevelConstant)
			commandArguments = append(commandArguments, fmt.Sprintf(integrationConfigFlagTemplateConstant, configurationPath))
			executionContext, cancelFunction := context.WithTimeout(context.Background(), integrationCommandTimeout)
			defer cancelFunction()

			command := exec.CommandContext(executionContext, integrationGoBinaryNameConstant, commandArguments...)
			command.Dir = repositoryRootDirectory
			command.Env = os.Environ()

			outputBytes, runError := command.CombinedOutput()
			outputText := string(outputBytes)
			require.NoError(testInstance, runError, outputText)

			for _, expectedSnippet := range testCase.expectedSnippets {
				require.Contains(testInstance, outputText, expectedSnippet)
			}
		})
	}
}

func TestCLIIntegrationRejectsLegacyCdCommand(testInstance *testing.T) {
	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRootDirectory := filepath.Dir(currentWorkingDirectory)

	temporaryDirectory := testInstance.TempDir()
	configurationPath := writeIntegrationConfiguration(testInstance, temporaryDirectory, integrationErrorLevelConstant)

	executionContext, cancelFunction := context.WithTimeout(context.Background(), integrationCommandTimeout)
	defer cancelFunction()

	commandArguments := []string{
		integrationGoRunSubcommandConstant,
		integrationCurrentDirectoryArgumentConstant,
		fmt.Sprintf(integrationConfigFlagTemplateConstant, configurationPath),
		"cd",
	}
	command := exec.CommandContext(executionContext, integrationGoBinaryNameConstant, commandArguments...)
	command.Dir = repositoryRootDirectory
	command.Env = os.Environ()

	outputBytes, runError := command.CombinedOutput()
	outputText := string(outputBytes)

	require.Error(testInstance, runError, outputText)
	require.Contains(testInstance, outputText, "unknown command")
	require.Contains(testInstance, outputText, "cd")
}

func TestCLIIntegrationRespectsLogFormatFlag(testInstance *testing.T) {
	testCases := []struct {
		name                string
		additionalArguments []string
		expectStructured    bool
	}{
		{
			name:                integrationStructuredLogCaseNameConstant,
			additionalArguments: []string{},
			expectStructured:    true,
		},
		{
			name:                integrationConsoleLogCaseNameConstant,
			additionalArguments: []string{integrationConsoleLogFlagConstant},
			expectStructured:    false,
		},
	}

	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRootDirectory := filepath.Dir(currentWorkingDirectory)

	for testCaseIndex, testCase := range testCases {
		testInstance.Run(fmt.Sprintf(integrationSubtestNameTemplateConstant, testCaseIndex, testCase.name), func(testInstance *testing.T) {
			commandArguments := []string{integrationGoRunSubcommandConstant, integrationCurrentDirectoryArgumentConstant}
			tempDirectory := testInstance.TempDir()
			configurationPath := writeIntegrationConfiguration(testInstance, tempDirectory, "")
			commandArguments = append(commandArguments, fmt.Sprintf(integrationConfigFlagTemplateConstant, configurationPath))
			commandArguments = append(commandArguments, "--log-level=debug")
			commandArguments = append(commandArguments, testCase.additionalArguments...)

			executionContext, cancelFunction := context.WithTimeout(context.Background(), integrationCommandTimeout)
			defer cancelFunction()

			command := exec.CommandContext(executionContext, integrationGoBinaryNameConstant, commandArguments...)
			command.Dir = repositoryRootDirectory
			command.Env = os.Environ()

			commandOutput, runError := command.CombinedOutput()
			outputText := string(commandOutput)
			require.NoError(testInstance, runError, outputText)

			logLineFound := false
			outputLines := strings.Split(outputText, "\n")
			for _, outputLine := range outputLines {
				trimmedLine := strings.TrimSpace(outputLine)
				if len(trimmedLine) == 0 {
					continue
				}

				if !strings.Contains(trimmedLine, integrationConfigurationInitializedSnippetConstant) {
					continue
				}

				logLineFound = true
				isStructuredLog := json.Valid([]byte(trimmedLine))
				if testCase.expectStructured {
					require.True(testInstance, isStructuredLog, trimmedLine)
				} else {
					require.False(testInstance, isStructuredLog, trimmedLine)
				}

				break
			}

			require.True(testInstance, logLineFound, outputText)
		})
	}
}

func TestCLIIntegrationHandlesPipeForStandardError(testInstance *testing.T) {
	testCases := []struct {
		name string
	}{
		{
			name: integrationStderrPipeCaseNameConstant,
		},
	}

	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRootDirectory := filepath.Dir(currentWorkingDirectory)

	for testCaseIndex, testCase := range testCases {
		testInstance.Run(fmt.Sprintf(integrationSubtestNameTemplateConstant, testCaseIndex, testCase.name), func(testInstance *testing.T) {
			commandArguments := []string{integrationGoRunSubcommandConstant, integrationCurrentDirectoryArgumentConstant}

			executionContext, cancelFunction := context.WithTimeout(context.Background(), integrationCommandTimeout)
			defer cancelFunction()

			command := exec.CommandContext(executionContext, integrationGoBinaryNameConstant, commandArguments...)
			command.Dir = repositoryRootDirectory
			command.Env = os.Environ()

			var stdoutBuffer bytes.Buffer
			command.Stdout = &stdoutBuffer

			pipeReader, pipeWriter, pipeError := os.Pipe()
			require.NoError(testInstance, pipeError)

			defer func() {
				closeError := pipeReader.Close()
				require.NoError(testInstance, closeError)
			}()

			command.Stderr = pipeWriter

			runError := command.Run()

			closeError := pipeWriter.Close()
			require.NoError(testInstance, closeError)

			stderrBytes, readError := io.ReadAll(pipeReader)

			require.NoError(testInstance, runError, string(stderrBytes))
			require.NoError(testInstance, readError)
		})
	}
}
