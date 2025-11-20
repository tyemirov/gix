package cd

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
)

type stubGitExecutor struct {
	recorded  []execshell.CommandDetails
	responses []stubGitResponse
}

type stubGitResponse struct {
	result execshell.ExecutionResult
	err    error
}

func (executor *stubGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.recorded = append(executor.recorded, details)
	if len(executor.responses) == 0 {
		return execshell.ExecutionResult{}, nil
	}

	next := executor.responses[0]
	executor.responses = executor.responses[1:]
	if next.err != nil {
		return execshell.ExecutionResult{}, next.err
	}
	return next.result, nil
}

func (executor *stubGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type scriptedGitExecutor struct {
	recorded     []execshell.CommandDetails
	remoteOutput string
	statusOutput string
}

func (executor *scriptedGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.recorded = append(executor.recorded, details)
	if len(details.Arguments) == 0 {
		return execshell.ExecutionResult{}, nil
	}
	switch details.Arguments[0] {
	case "remote":
		return execshell.ExecutionResult{StandardOutput: executor.remoteOutput}, nil
	case "status":
		return execshell.ExecutionResult{StandardOutput: executor.statusOutput}, nil
	default:
		return execshell.ExecutionResult{}, nil
	}
}

func (executor *scriptedGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func TestChangeExecutesExpectedCommands(t *testing.T) {
	executor := &stubGitExecutor{responses: []stubGitResponse{
		{result: execshell.ExecutionResult{StandardOutput: "origin\n"}},
	}}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	result, changeError := service.Change(context.Background(), Options{RepositoryPath: "/tmp/repo", BranchName: "feature", RemoteName: "origin"})
	require.NoError(t, changeError)
	require.Equal(t, "/tmp/repo", result.RepositoryPath)
	require.Equal(t, "feature", result.BranchName)
	require.False(t, result.BranchCreated)
	require.Empty(t, result.Warnings)
	require.Len(t, executor.recorded, 4)

	require.Equal(t, []string{"remote"}, executor.recorded[0].Arguments)
	require.Equal(t, []string{"fetch", "--prune", "origin"}, executor.recorded[1].Arguments)
	require.Equal(t, []string{"switch", "feature"}, executor.recorded[2].Arguments)
	require.Equal(t, []string{"pull", "--rebase"}, executor.recorded[3].Arguments)
}

func TestChangeCreatesBranchWhenMissing(t *testing.T) {
	executor := &stubGitExecutor{responses: []stubGitResponse{
		{result: execshell.ExecutionResult{StandardOutput: "upstream\n"}},
		{},
		{err: commandFailedError("error: pathspec 'feature' did not match any file(s) known to git")},
		{err: commandFailedError("fatal: Needed a single revision")},
	}}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	result, changeError := service.Change(context.Background(), Options{RepositoryPath: "/tmp/repo", BranchName: "feature", RemoteName: "upstream", CreateIfMissing: true})
	require.NoError(t, changeError)
	require.True(t, result.BranchCreated)
	require.Empty(t, result.Warnings)

	require.Len(t, executor.recorded, 5)
	require.Equal(t, []string{"rev-parse", "--verify", "upstream/feature"}, executor.recorded[3].Arguments)
	require.Equal(t, []string{"switch", "-c", "feature"}, executor.recorded[4].Arguments)
	for _, command := range executor.recorded {
		if len(command.Arguments) > 0 {
			require.NotEqual(t, "pull", command.Arguments[0])
		}
	}
}

func TestChangeCreatesBranchFromRemoteWhenAvailable(t *testing.T) {
	executor := &stubGitExecutor{responses: []stubGitResponse{
		{result: execshell.ExecutionResult{StandardOutput: "origin\n"}},
		{},
		{err: commandFailedError("error: pathspec 'feature' did not match any file(s) known to git")},
	}}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	result, changeError := service.Change(context.Background(), Options{RepositoryPath: "/tmp/repo", BranchName: "feature", RemoteName: "origin", CreateIfMissing: true})
	require.NoError(t, changeError)
	require.True(t, result.BranchCreated)

	require.Len(t, executor.recorded, 6)
	require.Equal(t, []string{"rev-parse", "--verify", "origin/feature"}, executor.recorded[3].Arguments)
	require.Equal(t, []string{"switch", "-c", "feature", "--track", "origin/feature"}, executor.recorded[4].Arguments)
}

func TestChangeValidatesInputs(t *testing.T) {
	service, err := NewService(ServiceDependencies{GitExecutor: &stubGitExecutor{}})
	require.NoError(t, err)

	_, changeError := service.Change(context.Background(), Options{BranchName: "main"})
	require.Error(t, changeError)

	_, changeError = service.Change(context.Background(), Options{RepositoryPath: "/tmp/repo"})
	require.Error(t, changeError)
}

func TestChangeWarnsWhenFetchFails(t *testing.T) {
	executor := &stubGitExecutor{responses: []stubGitResponse{
		{result: execshell.ExecutionResult{StandardOutput: "origin\n"}},
		{err: commandFailedError("ERROR: Repository not found.\nfatal: Could not read from remote repository.\nPlease make sure you have the correct access rights\nand the repository exists.")},
	}}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	result, changeError := service.Change(context.Background(), Options{RepositoryPath: "/tmp/ns-rewrite", BranchName: "main"})
	require.NoError(t, changeError)
	require.Len(t, result.Warnings, 1)
	require.Equal(t, "WARNING: no remote counterpart for ns-rewrite repo", result.Warnings[0])
	require.Len(t, executor.recorded, 3)
	require.Equal(t, []string{"remote"}, executor.recorded[0].Arguments)
	require.Equal(t, []string{"fetch", "--prune", "origin"}, executor.recorded[1].Arguments)
	require.Equal(t, []string{"switch", "main"}, executor.recorded[2].Arguments)
}

func TestChangeWarnsWithGenericFetchError(t *testing.T) {
	executor := &stubGitExecutor{responses: []stubGitResponse{
		{result: execshell.ExecutionResult{StandardOutput: "origin\n"}},
		{err: commandFailedError("fatal: unexpected network failure")},
	}}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	result, changeError := service.Change(context.Background(), Options{RepositoryPath: "/tmp/repo", BranchName: "main"})
	require.NoError(t, changeError)
	require.Len(t, result.Warnings, 1)
	require.Equal(t, "FETCH-SKIP: origin (fatal: unexpected network failure)", result.Warnings[0])
}

func TestChangePreservesFetchMessageWhenGitReportsCouldNotFetch(t *testing.T) {
	executor := &stubGitExecutor{responses: []stubGitResponse{
		{result: execshell.ExecutionResult{StandardOutput: "origin\n"}},
		{err: commandFailedError("fatal: Could not fetch origin")},
	}}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	result, changeError := service.Change(context.Background(), Options{RepositoryPath: "/tmp/repo", BranchName: "main"})
	require.NoError(t, changeError)
	require.Len(t, result.Warnings, 1)
	require.Equal(t, "FETCH-SKIP: origin (fatal: Could not fetch origin)", result.Warnings[0])
}

func TestChangeWarnsWhenPullFails(t *testing.T) {
	executor := &stubGitExecutor{responses: []stubGitResponse{
		{result: execshell.ExecutionResult{StandardOutput: "origin\n"}},
		{result: execshell.ExecutionResult{}},
		{},
		{err: commandFailedError("fatal: Could not read from remote repository\nPlease make sure you have the correct access rights.")},
	}}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	result, changeError := service.Change(context.Background(), Options{RepositoryPath: "/tmp/repo", BranchName: "main"})
	require.NoError(t, changeError)
	require.Len(t, result.Warnings, 1)
	require.Equal(t, "PULL-SKIP: fatal: Could not read from remote repository", result.Warnings[0])
	require.Len(t, executor.recorded, 4)
	require.Equal(t, []string{"pull", "--rebase"}, executor.recorded[3].Arguments)
}

func TestChangeFetchesAllWhenDefaultRemoteMissing(t *testing.T) {
	executor := &stubGitExecutor{responses: []stubGitResponse{
		{result: execshell.ExecutionResult{StandardOutput: "upstream\n"}},
	}}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	result, changeError := service.Change(context.Background(), Options{RepositoryPath: "/tmp/repo", BranchName: "feature"})
	require.NoError(t, changeError)
	require.False(t, result.BranchCreated)
	require.Empty(t, result.Warnings)

	require.Len(t, executor.recorded, 4)
	require.Equal(t, []string{"remote"}, executor.recorded[0].Arguments)
	require.Equal(t, []string{"fetch", "--all", "--prune"}, executor.recorded[1].Arguments)
}

func TestChangeFetchesAllWhenExplicitRemoteMissing(t *testing.T) {
	executor := &stubGitExecutor{responses: []stubGitResponse{
		{result: execshell.ExecutionResult{StandardOutput: "origin\n"}},
	}}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	result, changeError := service.Change(context.Background(), Options{
		RepositoryPath: "/tmp/repo",
		BranchName:     "feature",
		RemoteName:     "canonical",
	})
	require.NoError(t, changeError)
	require.False(t, result.BranchCreated)
	expectedWarning := fmt.Sprintf(fetchFallbackWarningTemplateConstant, "canonical")
	require.Contains(t, result.Warnings, expectedWarning)

	require.Len(t, executor.recorded, 4)
	require.Equal(t, []string{"remote"}, executor.recorded[0].Arguments)
	require.Equal(t, []string{"fetch", "--all", "--prune"}, executor.recorded[1].Arguments)
	require.Equal(t, []string{"switch", "feature"}, executor.recorded[2].Arguments)
	require.Equal(t, []string{"pull", "--rebase"}, executor.recorded[3].Arguments)
}

func TestChangeWarnsWhenExplicitRemoteMissingWithoutAlternatives(t *testing.T) {
	executor := &stubGitExecutor{responses: []stubGitResponse{
		{result: execshell.ExecutionResult{}},
	}}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	result, changeError := service.Change(context.Background(), Options{
		RepositoryPath: "/tmp/repo",
		BranchName:     "feature",
		RemoteName:     "canonical",
	})
	require.NoError(t, changeError)
	require.False(t, result.BranchCreated)
	expectedWarning := fmt.Sprintf(missingConfiguredRemoteWarningTemplate, "canonical")
	require.Contains(t, result.Warnings, expectedWarning)

	require.Len(t, executor.recorded, 2)
	require.Equal(t, []string{"remote"}, executor.recorded[0].Arguments)
	require.Equal(t, []string{"switch", "feature"}, executor.recorded[1].Arguments)
}

func TestChangeSkipsNetworkWhenNoRemotesDetected(t *testing.T) {
	executor := &stubGitExecutor{responses: []stubGitResponse{
		{result: execshell.ExecutionResult{}},
	}}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	result, changeError := service.Change(context.Background(), Options{RepositoryPath: "/tmp/repo", BranchName: "stable"})
	require.NoError(t, changeError)
	require.False(t, result.BranchCreated)
	require.Empty(t, result.Warnings)

	require.Len(t, executor.recorded, 2)
	require.Equal(t, []string{"remote"}, executor.recorded[0].Arguments)
	require.Equal(t, []string{"switch", "stable"}, executor.recorded[1].Arguments)
	for _, recorded := range executor.recorded {
		if len(recorded.Arguments) == 0 {
			continue
		}
		require.NotEqual(t, gitFetchSubcommandConstant, recorded.Arguments[0])
		require.NotEqual(t, gitPullSubcommandConstant, recorded.Arguments[0])
	}
}

func TestChangeFailsWhenRemoteEnumerationFails(t *testing.T) {
	executor := &stubGitExecutor{responses: []stubGitResponse{{err: errors.New("remote list failed")}}}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	_, changeError := service.Change(context.Background(), Options{RepositoryPath: "/tmp/repo", BranchName: "main"})
	require.ErrorContains(t, changeError, "fetch updates")
}

func TestChangeDoesNotCreateBranchWhenSwitchFailsForOtherReasons(t *testing.T) {
	executor := &stubGitExecutor{responses: []stubGitResponse{
		{result: execshell.ExecutionResult{StandardOutput: "origin\n"}},
		{result: execshell.ExecutionResult{}},
		{err: commandFailedError("error: Your local changes to the following files would be overwritten by checkout:\nREADME.md")},
	}}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	_, changeError := service.Change(context.Background(), Options{
		RepositoryPath:  "/tmp/repo",
		BranchName:      "feature",
		CreateIfMissing: true,
	})
	require.Error(t, changeError)
	require.Contains(t, changeError.Error(), "failed to switch to branch \"feature\"")
	require.Contains(t, changeError.Error(), "Your local changes to the following files would be overwritten by checkout")
	require.Len(t, executor.recorded, 3)
}

func TestChangeIncludesDetailsWhenBranchCreationFails(t *testing.T) {
	executor := &stubGitExecutor{responses: []stubGitResponse{
		{result: execshell.ExecutionResult{StandardOutput: "origin\n"}},
		{result: execshell.ExecutionResult{}},
		{err: commandFailedError("error: pathspec 'feature' did not match any file(s) known to git")},
		{},
		{err: commandFailedError("fatal: a branch named 'feature' already exists")},
	}}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	_, changeError := service.Change(context.Background(), Options{
		RepositoryPath:  "/tmp/repo",
		BranchName:      "feature",
		CreateIfMissing: true,
		RemoteName:      "origin",
	})
	require.Error(t, changeError)
	require.Contains(t, changeError.Error(), "failed to create branch \"feature\" from origin")
	require.Contains(t, changeError.Error(), "a branch named 'feature' already exists")
	require.Len(t, executor.recorded, 5)
}

func commandFailedError(message string) error {
	return execshell.CommandFailedError{
		Command: execshell.ShellCommand{
			Name:    execshell.CommandGit,
			Details: execshell.CommandDetails{},
		},
		Result: execshell.ExecutionResult{
			ExitCode:      128,
			StandardError: message,
		},
	}
}
