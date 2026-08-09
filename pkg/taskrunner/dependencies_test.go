package taskrunner

import (
	"bytes"
	"context"
	"io/fs"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
)

func TestBuildDependenciesSkipsGitHubResolver(t *testing.T) {
	config := DependenciesConfig{
		LoggerProvider:       func() *zap.Logger { return zap.NewNop() },
		RepositoryDiscoverer: stubRepositoryDiscoverer{},
		GitExecutor:          stubGitExecutor{},
		GitRepositoryManager: stubRepositoryManager{},
		FileSystem:           stubFileSystem{},
	}

	result, err := BuildDependencies(
		config,
		DependenciesOptions{
			SkipGitHubResolver: true,
			Output:             &bytes.Buffer{},
			Errors:             &bytes.Buffer{},
		},
	)
	require.NoError(t, err)
	require.Nil(t, result.GitHubResolver)
	require.Nil(t, result.Workflow.GitHubClient)
}

func TestBuildDependenciesRequiresOutputWriter(t *testing.T) {
	config := DependenciesConfig{
		LoggerProvider:       func() *zap.Logger { return zap.NewNop() },
		RepositoryDiscoverer: stubRepositoryDiscoverer{},
		GitExecutor:          stubGitExecutor{},
		GitRepositoryManager: stubRepositoryManager{},
		FileSystem:           stubFileSystem{},
	}

	_, err := BuildDependencies(config, DependenciesOptions{SkipGitHubResolver: true})
	require.ErrorIs(t, err, errOutputWriterMissing)
}

func TestBuildDependenciesRequiresErrorWriter(t *testing.T) {
	config := DependenciesConfig{
		LoggerProvider:       func() *zap.Logger { return zap.NewNop() },
		RepositoryDiscoverer: stubRepositoryDiscoverer{},
		GitExecutor:          stubGitExecutor{},
		GitRepositoryManager: stubRepositoryManager{},
		FileSystem:           stubFileSystem{},
	}

	_, err := BuildDependencies(
		config,
		DependenciesOptions{
			SkipGitHubResolver: true,
			Output:             &bytes.Buffer{},
		},
	)
	require.ErrorIs(t, err, errErrorWriterMissing)
}

func TestBuildDependenciesAlwaysEnablesHumanLogging(t *testing.T) {
	config := DependenciesConfig{
		LoggerProvider:       func() *zap.Logger { return zap.NewNop() },
		RepositoryDiscoverer: stubRepositoryDiscoverer{},
		GitExecutor:          stubGitExecutor{},
		GitRepositoryManager: stubRepositoryManager{},
		FileSystem:           stubFileSystem{},
		HumanReadableLoggingProvider: func() bool {
			return false
		},
	}

	result, err := BuildDependencies(
		config,
		DependenciesOptions{
			Output:             &bytes.Buffer{},
			Errors:             &bytes.Buffer{},
			SkipGitHubResolver: true,
		},
	)
	require.NoError(t, err)
	require.True(t, result.Workflow.HumanReadableLogging)
}

type stubGitExecutor struct{}

func (stubGitExecutor) ExecuteGit(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func (stubGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type stubRepositoryManager struct{}

func (stubRepositoryManager) CheckCleanWorktree(context.Context, string) (bool, error) {
	return true, nil
}

func (stubRepositoryManager) WorktreeStatus(context.Context, string) ([]string, error) {
	return nil, nil
}

func (stubRepositoryManager) GetCurrentBranch(context.Context, string) (string, error) {
	return "main", nil
}

func (stubRepositoryManager) GetRemoteURL(context.Context, string, string) (string, error) {
	return "", nil
}

func (stubRepositoryManager) SetRemoteURL(context.Context, string, string, string) error {
	return nil
}

type stubRepositoryDiscoverer struct{}

func (stubRepositoryDiscoverer) DiscoverRepositories([]string) ([]string, error) {
	return nil, nil
}

type stubFileSystem struct{}

func (stubFileSystem) Stat(string) (fs.FileInfo, error) { return nil, nil }

func (stubFileSystem) Rename(string, string) error { return nil }

func (stubFileSystem) Abs(path string) (string, error) { return path, nil }

func (stubFileSystem) MkdirAll(string, fs.FileMode) error { return nil }

func (stubFileSystem) ReadFile(string) ([]byte, error) { return nil, nil }

func (stubFileSystem) WriteFile(string, []byte, fs.FileMode) error { return nil }
