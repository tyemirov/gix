package syncflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
)

type strictSyncPreflightExecutor struct {
	commands     []execshell.CommandDetails
	executionErr error
}

func (executor *strictSyncPreflightExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	return execshell.ExecutionResult{}, executor.executionErr
}

func (executor *strictSyncPreflightExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func TestStrictSyncRevertPreflight(testInstance *testing.T) {
	testCases := []struct {
		Name                 string
		ExecutionError       error
		ExpectedErrorSamples []string
	}{
		{
			Name: "ActiveRevert",
			ExpectedErrorSamples: []string{
				"operator-owned Git revert is in progress",
				"git revert --continue",
				"git revert --abort",
				"git revert --quit",
			},
		},
		{
			Name: "NoActiveRevert",
			ExecutionError: execshell.CommandFailedError{
				Result: execshell.ExecutionResult{ExitCode: 1},
			},
		},
		{
			Name: "InspectionFailure",
			ExecutionError: execshell.CommandFailedError{
				Result: execshell.ExecutionResult{
					ExitCode:      128,
					StandardError: "fatal: cannot inspect repository state",
				},
			},
			ExpectedErrorSamples: []string{
				"inspect operator-owned Git revert before strict sync",
				"cannot inspect repository state",
			},
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		testInstance.Run(testCase.Name, func(testInstance *testing.T) {
			executor := &strictSyncPreflightExecutor{executionErr: testCase.ExecutionError}

			preflightErr := ensureNoOperatorOwnedRevert(context.Background(), executor, "/tmp/project")

			if len(testCase.ExpectedErrorSamples) == 0 {
				require.NoError(testInstance, preflightErr)
			} else {
				require.Error(testInstance, preflightErr)
				for sampleIndex := range testCase.ExpectedErrorSamples {
					require.Contains(testInstance, preflightErr.Error(), testCase.ExpectedErrorSamples[sampleIndex])
				}
			}
			require.Len(testInstance, executor.commands, 1)
			require.Equal(
				testInstance,
				[]string{"rev-parse", "--verify", "--quiet", "REVERT_HEAD"},
				executor.commands[0].Arguments,
			)
			require.Equal(testInstance, "/tmp/project", executor.commands[0].WorkingDirectory)
		})
	}
}
