package utils_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/utils"
)

const (
	testArgumentOneConstant                    = "--version"
	testWorkingDirectoryConstant               = "."
	testEnvironmentKeyConstant                 = "EXAMPLE_KEY"
	testEnvironmentValueConstant               = "example-value"
	testStandardInputValueConstant             = "input"
	testWrapperGitCaseNameConstant             = "git_wrapper"
	testWrapperGitHubCaseNameConstant          = "github_wrapper"
	testWrapperCurlCaseNameConstant            = "curl_wrapper"
	testNilRunnerCaseNameConstant              = "nil_runner"
	testGitCommandCaseNameConstant             = "git_version"
	testGitVersionSubstringConstant            = "git version"
	commandExecutorSubtestNameTemplateConstant = "%d_%s"
)

type recordingProcessRunner struct {
	executedCommands []utils.ExecutableCommand
	result           utils.CommandResult
	runError         error
}

func (runner *recordingProcessRunner) Run(executionContext context.Context, command utils.ExecutableCommand) (utils.CommandResult, error) {
	runner.executedCommands = append(runner.executedCommands, command)
	return runner.result, runner.runError
}

func TestCommandExecutorWrappers(testInstance *testing.T) {
	testOptions := utils.CommandOptions{
		Arguments:            []string{testArgumentOneConstant},
		WorkingDirectory:     testWorkingDirectoryConstant,
		EnvironmentVariables: map[string]string{testEnvironmentKeyConstant: testEnvironmentValueConstant},
		StandardInput:        []byte(testStandardInputValueConstant),
	}

	testCases := []struct {
		name   string
		tool   utils.ExternalToolName
		invoke func(executor *utils.CommandExecutor) (utils.CommandResult, error)
	}{
		{
			name: testWrapperGitCaseNameConstant,
			tool: utils.ExternalToolGit,
			invoke: func(executor *utils.CommandExecutor) (utils.CommandResult, error) {
				return executor.ExecuteGitCommand(context.Background(), testOptions)
			},
		},
		{
			name: testWrapperGitHubCaseNameConstant,
			tool: utils.ExternalToolGitHubCLI,
			invoke: func(executor *utils.CommandExecutor) (utils.CommandResult, error) {
				return executor.ExecuteGitHubCommand(context.Background(), testOptions)
			},
		},
		{
			name: testWrapperCurlCaseNameConstant,
			tool: utils.ExternalToolCurl,
			invoke: func(executor *utils.CommandExecutor) (utils.CommandResult, error) {
				return executor.ExecuteCurlCommand(context.Background(), testOptions)
			},
		},
	}

	for testCaseIndex, testCase := range testCases {
		testInstance.Run(fmt.Sprintf(commandExecutorSubtestNameTemplateConstant, testCaseIndex, testCase.name), func(testInstance *testing.T) {
			recordingRunner := &recordingProcessRunner{}
			executor := utils.NewCommandExecutor(recordingRunner)
			_, executionError := testCase.invoke(executor)
			require.NoError(testInstance, executionError)
			require.Len(testInstance, recordingRunner.executedCommands, 1)
			executedCommand := recordingRunner.executedCommands[0]
			require.Equal(testInstance, testCase.tool, executedCommand.ToolName)
			require.Equal(testInstance, testOptions.Arguments, executedCommand.Arguments)
			require.Equal(testInstance, testOptions.WorkingDirectory, executedCommand.WorkingDirectory)
			require.Equal(testInstance, testOptions.EnvironmentVariables, executedCommand.EnvironmentVariables)
			require.Equal(testInstance, testOptions.StandardInput, executedCommand.StandardInput)
		})
	}
}

func TestCommandExecutorExecuteWithNilRunner(testInstance *testing.T) {
	testInstance.Run(testNilRunnerCaseNameConstant, func(testInstance *testing.T) {
		executor := utils.NewCommandExecutor(nil)
		_, executionError := executor.Execute(context.Background(), utils.ExecutableCommand{})
		require.Error(testInstance, executionError)
	})
}

func TestOSExternalProcessRunnerRun(testInstance *testing.T) {
	testInstance.Run(testGitCommandCaseNameConstant, func(testInstance *testing.T) {
		processRunner := utils.NewOSExternalProcessRunner()
		executor := utils.NewCommandExecutor(processRunner)

		executionContext, cancelFunction := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelFunction()

		result, executionError := executor.ExecuteGitCommand(executionContext, utils.CommandOptions{Arguments: []string{testArgumentOneConstant}})
		require.NoError(testInstance, executionError)
		require.Equal(testInstance, 0, result.ExitCode)
		require.True(testInstance, strings.Contains(result.StandardOutput, testGitVersionSubstringConstant) || strings.Contains(result.StandardError, testGitVersionSubstringConstant))
	})
}
