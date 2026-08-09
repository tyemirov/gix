package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/gitrepo"
)

func TestCommandRunOperationExecutesCommand(testInstance *testing.T) {
	testCases := []struct {
		name                   string
		command                any
		workingDirectory       string
		expectedCommandName    execshell.CommandName
		expectedArguments      []string
		expectedWorkingDirBase string
	}{
		{
			name:                   "uses repo root by default",
			command:                []any{"go", "mod", "tidy"},
			expectedCommandName:    execshell.CommandName("go"),
			expectedArguments:      []string{"mod", "tidy"},
			expectedWorkingDirBase: "/repositories/sample",
		},
		{
			name:                   "uses configured working directory",
			command:                []any{"go", "get", "-u", "./..."},
			workingDirectory:       "modules",
			expectedCommandName:    execshell.CommandName("go"),
			expectedArguments:      []string{"get", "-u", "./..."},
			expectedWorkingDirBase: "/repositories/sample/modules",
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		testInstance.Run(testCase.name, func(subTestInstance *testing.T) {
			executor := &recordingShellExecutor{clean: true, branch: "main"}
			operationOptions := map[string]any{
				"command": testCase.command,
			}
			if testCase.workingDirectory != "" {
				operationOptions["working_directory"] = testCase.workingDirectory
			}
			operation, buildErr := buildCommandRunOperation(operationOptions)
			require.NoError(subTestInstance, buildErr)

			repository := NewRepositoryState(audit.RepositoryInspection{
				Path:           "/repositories/sample",
				FinalOwnerRepo: "octocat/sample",
			})
			state := &State{Repositories: []*RepositoryState{repository}}
			environment := &Environment{GitExecutor: executor}

			require.NoError(subTestInstance, operation.Execute(context.Background(), environment, state))

			require.Len(subTestInstance, executor.commands, 1)
			executed := executor.commands[0]
			require.Equal(subTestInstance, testCase.expectedCommandName, executed.Name)
			require.Equal(subTestInstance, testCase.expectedArguments, executed.Details.Arguments)
			require.Equal(subTestInstance, testCase.expectedWorkingDirBase, executed.Details.WorkingDirectory)
		})
	}
}

func TestCommandRunOperationRequiresCleanWorktree(testInstance *testing.T) {
	executor := &recordingShellExecutor{clean: false, branch: "main"}
	manager, managerErr := gitrepo.NewRepositoryManager(executor)
	require.NoError(testInstance, managerErr)

	operation, buildErr := buildCommandRunOperation(map[string]any{
		"command":      []any{"go", "get", "-u", "./..."},
		"ensure_clean": true,
	})
	require.NoError(testInstance, buildErr)

	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	state := &State{Repositories: []*RepositoryState{repository}}
	environment := &Environment{
		GitExecutor:       executor,
		RepositoryManager: manager,
	}

	require.ErrorIs(testInstance, operation.Execute(context.Background(), environment, state), errRepositorySkipped)
	require.Empty(testInstance, executor.commands)
}
