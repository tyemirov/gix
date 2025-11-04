package repos_test

import (
	"context"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
)

type fakeRepositoryDiscoverer struct {
	repositories  []string
	receivedRoots []string
}

func (discoverer *fakeRepositoryDiscoverer) DiscoverRepositories(roots []string) ([]string, error) {
	discoverer.receivedRoots = append([]string{}, roots...)
	return append([]string{}, discoverer.repositories...), nil
}

type fakeGitExecutor struct{}

func (executor *fakeGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	if len(details.Arguments) > 0 && details.Arguments[0] == "rev-parse" {
		return execshell.ExecutionResult{StandardOutput: "true\n"}, nil
	}
	return execshell.ExecutionResult{StandardOutput: ""}, nil
}

func (executor *fakeGitExecutor) ExecuteGitHubCLI(_ context.Context, _ execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{StandardOutput: ""}, nil
}

type remoteUpdateCall struct {
	repositoryPath string
	remoteURL      string
}

type fakeGitRepositoryManager struct {
	remoteURL                  string
	currentBranch              string
	setCalls                   []remoteUpdateCall
	cleanWorktree              bool
	cleanWorktreeSet           bool
	checkCleanCalls            int
	panicOnCurrentBranchLookup bool
}

func (manager *fakeGitRepositoryManager) CheckCleanWorktree(context.Context, string) (bool, error) {
	manager.checkCleanCalls++
	if manager.cleanWorktreeSet {
		return manager.cleanWorktree, nil
	}
	return true, nil
}

func (manager *fakeGitRepositoryManager) GetCurrentBranch(context.Context, string) (string, error) {
	if manager.panicOnCurrentBranchLookup {
		panic("GetCurrentBranch should not be called during minimal inspection")
	}
	return manager.currentBranch, nil
}

func (manager *fakeGitRepositoryManager) GetRemoteURL(context.Context, string, string) (string, error) {
	return manager.remoteURL, nil
}

func (manager *fakeGitRepositoryManager) SetRemoteURL(_ context.Context, repositoryPath string, _ string, remoteURL string) error {
	manager.setCalls = append(manager.setCalls, remoteUpdateCall{repositoryPath: repositoryPath, remoteURL: remoteURL})
	manager.remoteURL = remoteURL
	return nil
}

type recordingPrompter struct {
	result shared.ConfirmationResult
	err    error
	calls  int
}

func (prompter *recordingPrompter) Confirm(string) (shared.ConfirmationResult, error) {
	prompter.calls++
	if prompter.err != nil {
		return shared.ConfirmationResult{}, prompter.err
	}
	return prompter.result, nil
}
