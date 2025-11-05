package repos_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

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

func (manager *fakeGitRepositoryManager) WorktreeStatus(context.Context, string) ([]string, error) {
	if manager.cleanWorktreeSet && !manager.cleanWorktree {
		return []string{" M file.txt"}, nil
	}
	return nil, nil
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

type fakeFileSystem struct {
	files map[string]string
}

func (fs fakeFileSystem) Stat(path string) (os.FileInfo, error) {
	if _, exists := fs.files[path]; exists {
		return fakeFileInfo{name: filepath.Base(path)}, nil
	}
	return nil, os.ErrNotExist
}

func (fs fakeFileSystem) Rename(string, string) error {
	return nil
}

func (fs fakeFileSystem) Abs(path string) (string, error) {
	return filepath.Abs(path)
}

func (fs fakeFileSystem) MkdirAll(string, os.FileMode) error {
	return nil
}

func (fs fakeFileSystem) ReadFile(path string) ([]byte, error) {
	if content, exists := fs.files[path]; exists {
		return []byte(content), nil
	}
	return nil, os.ErrNotExist
}

func (fs fakeFileSystem) WriteFile(string, []byte, os.FileMode) error {
	return nil
}

type fakeFileInfo struct {
	name string
}

func (info fakeFileInfo) Name() string       { return info.name }
func (info fakeFileInfo) Size() int64        { return 0 }
func (info fakeFileInfo) Mode() os.FileMode  { return 0 }
func (info fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (info fakeFileInfo) IsDir() bool        { return false }
func (info fakeFileInfo) Sys() any           { return nil }
