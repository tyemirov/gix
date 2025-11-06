package history_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/repos/filesystem"
	"github.com/temirov/gix/internal/repos/history"
	"github.com/temirov/gix/internal/repos/shared"
)

type scriptedGitExecutor struct {
	responses      map[string]execshell.ExecutionResult
	errorFactories map[string]func(execshell.CommandDetails) error
	commands       []execshell.CommandDetails
}

func newScriptedGitExecutor() *scriptedGitExecutor {
	return &scriptedGitExecutor{
		responses:      map[string]execshell.ExecutionResult{},
		errorFactories: map[string]func(execshell.CommandDetails) error{},
	}
}

func commandKey(arguments []string) string {
	return strings.Join(arguments, " ")
}

func (executor *scriptedGitExecutor) setResponse(arguments []string, result execshell.ExecutionResult) {
	executor.responses[commandKey(arguments)] = result
}

func (executor *scriptedGitExecutor) setError(arguments []string, factory func(execshell.CommandDetails) error) {
	executor.errorFactories[commandKey(arguments)] = factory
}

func (executor *scriptedGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	key := commandKey(details.Arguments)
	if factory, exists := executor.errorFactories[key]; exists {
		return execshell.ExecutionResult{}, factory(details)
	}
	if result, exists := executor.responses[key]; exists {
		return result, nil
	}
	return execshell.ExecutionResult{}, nil
}

func (executor *scriptedGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type stubRepositoryManager struct {
	remoteURL string
}

func (manager stubRepositoryManager) CheckCleanWorktree(context.Context, string) (bool, error) {
	return true, nil
}

func (manager stubRepositoryManager) WorktreeStatus(context.Context, string) ([]string, error) {
	return nil, nil
}

func (manager stubRepositoryManager) GetCurrentBranch(context.Context, string) (string, error) {
	return "main", nil
}

func (manager stubRepositoryManager) GetRemoteURL(context.Context, string, string) (string, error) {
	return manager.remoteURL, nil
}

func (manager stubRepositoryManager) SetRemoteURL(context.Context, string, string, string) error {
	return nil
}

func TestExecutorSkipsWhenPathsMissing(testInstance *testing.T) {
	executor := newScriptedGitExecutor()
	executor.setResponse([]string{"rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"}, execshell.ExecutionResult{StandardOutput: "origin/main\n"})
	executor.setResponse([]string{"rev-list", "--all", "--", "secrets.txt"}, execshell.ExecutionResult{StandardOutput: ""})

	repoManager := stubRepositoryManager{remoteURL: "https://github.com/example/repo.git"}
	outputBuffer := &strings.Builder{}

	service := history.NewExecutor(history.Dependencies{
		GitExecutor:       executor,
		RepositoryManager: repoManager,
		FileSystem:        filesystem.OSFileSystem{},
		Output:            outputBuffer,
	})

	repoPath := testInstance.TempDir()
	repositoryPath, repositoryPathError := shared.NewRepositoryPath(repoPath)
	require.NoError(testInstance, repositoryPathError)
	remoteNameValue, remoteNameError := shared.NewRemoteName("origin")
	require.NoError(testInstance, remoteNameError)
	options := history.Options{
		RepositoryPath: repositoryPath,
		Paths:          []string{"secrets.txt"},
		RemoteName:     &remoteNameValue,
		Push:           false,
		Restore:        false,
		PushMissing:    false,
	}

	executionError := service.Execute(context.Background(), options)
	require.NoError(testInstance, executionError)

	commandHistory := make([][]string, 0, len(executor.commands))
	for _, details := range executor.commands {
		commandHistory = append(commandHistory, details.Arguments)
	}

	require.Contains(testInstance, commandHistory, []string{"fetch", "--prune", "--tags", "origin"})
	require.Contains(testInstance, commandHistory, []string{"add", ".gitignore"})
	require.Contains(testInstance, commandHistory, []string{"rev-list", "--all", "--", "secrets.txt"})
	require.Contains(testInstance, outputBuffer.String(), "HISTORY-SKIP")
}

func TestExecutorRunsFilterRepoAndPush(testInstance *testing.T) {
	executor := newScriptedGitExecutor()
	executor.setResponse([]string{"rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"}, execshell.ExecutionResult{StandardOutput: "origin/main\n"})
	executor.setResponse([]string{"rev-list", "--all", "--", "missing.txt"}, execshell.ExecutionResult{StandardOutput: ""})
	executor.setResponse([]string{"rev-list", "--all", "--", "secrets.txt"}, execshell.ExecutionResult{StandardOutput: "abcd1234\n"})
	executor.setResponse([]string{"for-each-ref", "--format=%(refname)", "refs/filter-repo/"}, execshell.ExecutionResult{StandardOutput: ""})
	executor.setResponse([]string{"remote"}, execshell.ExecutionResult{StandardOutput: "origin\n"})
	executor.setResponse([]string{"for-each-ref", "--format=%(refname)", "refs/heads/"}, execshell.ExecutionResult{StandardOutput: "refs/heads/main"})
	executor.setResponse([]string{"for-each-ref", "--format=%(upstream:short)", "refs/heads/main"}, execshell.ExecutionResult{StandardOutput: "origin/main\n"})
	executor.setResponse([]string{"show-ref", "--quiet", "refs/remotes/origin/main"}, execshell.ExecutionResult{})

	repoManager := stubRepositoryManager{remoteURL: "git@github.com:example/repo.git"}
	outputBuffer := &strings.Builder{}

	service := history.NewExecutor(history.Dependencies{
		GitExecutor:       executor,
		RepositoryManager: repoManager,
		FileSystem:        filesystem.OSFileSystem{},
		Output:            outputBuffer,
	})

	repoPath := testInstance.TempDir()
	repositoryPath, repositoryPathError := shared.NewRepositoryPath(repoPath)
	require.NoError(testInstance, repositoryPathError)
	options := history.Options{
		RepositoryPath: repositoryPath,
		Paths:          []string{"missing.txt", "secrets.txt"},
		RemoteName:     nil,
		Push:           true,
		Restore:        true,
		PushMissing:    false,
	}

	executionError := service.Execute(context.Background(), options)
	require.NoError(testInstance, executionError)
	require.Contains(testInstance, outputBuffer.String(), "HISTORY-PURGE")

	executedCommands := make([]string, 0, len(executor.commands))
	for _, details := range executor.commands {
		executedCommands = append(executedCommands, strings.Join(details.Arguments, " "))
	}

	require.Contains(testInstance, executedCommands, "fetch --prune --tags origin")
	require.Contains(testInstance, executedCommands, "filter-repo --path missing.txt --path secrets.txt --invert-paths --prune-empty always --force")
	require.Contains(testInstance, executedCommands, "push --force --all origin")
	require.Contains(testInstance, executedCommands, "push --force --tags origin")
}

func TestExecutorFailsWhenFetchingRemoteRefsFails(testInstance *testing.T) {
	executor := newScriptedGitExecutor()
	executor.setResponse([]string{"rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"}, execshell.ExecutionResult{StandardOutput: "origin/main\n"})
	executor.setError([]string{"fetch", "--prune", "--tags", "origin"}, func(details execshell.CommandDetails) error {
		return execshell.CommandFailedError{Command: execshell.ShellCommand{Name: execshell.CommandGit, Details: details}, Result: execshell.ExecutionResult{ExitCode: 128}}
	})

	repoManager := stubRepositoryManager{remoteURL: "https://github.com/example/repo.git"}
	outputBuffer := &strings.Builder{}

	service := history.NewExecutor(history.Dependencies{
		GitExecutor:       executor,
		RepositoryManager: repoManager,
		FileSystem:        filesystem.OSFileSystem{},
		Output:            outputBuffer,
	})

	repoPath := testInstance.TempDir()
	repositoryPath, repositoryPathError := shared.NewRepositoryPath(repoPath)
	require.NoError(testInstance, repositoryPathError)
	remoteNameValue, remoteNameError := shared.NewRemoteName("origin")
	require.NoError(testInstance, remoteNameError)
	options := history.Options{
		RepositoryPath: repositoryPath,
		Paths:          []string{"secrets.txt"},
		RemoteName:     &remoteNameValue,
		Push:           false,
		Restore:        false,
		PushMissing:    false,
	}

	executionError := service.Execute(context.Background(), options)
	require.Error(testInstance, executionError)
}
