package dependencies_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/dependencies"
	"github.com/tyemirov/gix/internal/repos/discovery"
	"github.com/tyemirov/gix/internal/repos/filesystem"
)

func TestResolveRepositoryDiscoverer(t *testing.T) {
	t.Parallel()

	existing := discovery.NewFilesystemRepositoryDiscoverer()
	require.Equal(t, existing, dependencies.ResolveRepositoryDiscoverer(existing))

	resolved := dependencies.ResolveRepositoryDiscoverer(nil)
	require.NotNil(t, resolved)
	require.IsType(t, discovery.NewFilesystemRepositoryDiscoverer(), resolved)
}

func TestResolveFileSystem(t *testing.T) {
	t.Parallel()

	existing := filesystem.OSFileSystem{}
	require.Equal(t, existing, dependencies.ResolveFileSystem(existing))

	resolved := dependencies.ResolveFileSystem(nil)
	require.IsType(t, filesystem.OSFileSystem{}, resolved)
}

func TestResolveGitExecutor(t *testing.T) {
	t.Parallel()

	existing := stubGitExecutor{}
	reused, reuseError := dependencies.ResolveGitExecutor(existing, nil, false)
	require.NoError(t, reuseError)
	require.Equal(t, existing, reused)

	defaultExecutor, defaultError := dependencies.ResolveGitExecutor(nil, zap.NewNop(), false)
	require.NoError(t, defaultError)
	require.IsType(t, &execshell.ShellExecutor{}, defaultExecutor)

	_, loggerError := dependencies.ResolveGitExecutor(nil, nil, false)
	require.ErrorIs(t, loggerError, execshell.ErrLoggerNotConfigured)
}

func TestResolveGitRepositoryManager(t *testing.T) {
	t.Parallel()

	existing := stubRepositoryManager{}
	reused, reuseError := dependencies.ResolveGitRepositoryManager(existing, nil)
	require.NoError(t, reuseError)
	require.Equal(t, existing, reused)

	manager, managerError := dependencies.ResolveGitRepositoryManager(nil, stubGitExecutor{})
	require.NoError(t, managerError)
	require.IsType(t, &gitrepo.RepositoryManager{}, manager)

	_, missingExecutorError := dependencies.ResolveGitRepositoryManager(nil, nil)
	require.ErrorIs(t, missingExecutorError, gitrepo.ErrGitExecutorNotConfigured)
}

func TestResolveGitHubResolver(t *testing.T) {
	t.Parallel()

	existing := stubGitHubResolver{}
	resolved, resolveError := dependencies.ResolveGitHubResolver(existing, nil)
	require.NoError(t, resolveError)
	require.Equal(t, existing, resolved)

	resolver, resolverError := dependencies.ResolveGitHubResolver(nil, stubGitExecutor{})
	require.NoError(t, resolverError)
	require.IsType(t, &githubcli.Client{}, resolver)

	_, executorError := dependencies.ResolveGitHubResolver(nil, nil)
	require.ErrorIs(t, executorError, githubcli.ErrExecutorNotConfigured)
}

type stubGitExecutor struct{}

func (stubGitExecutor) ExecuteGit(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func (stubGitExecutor) ExecuteGitHubCLI(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type stubRepositoryManager struct{}

func (stubRepositoryManager) CheckCleanWorktree(executionContext context.Context, repositoryPath string) (bool, error) {
	return true, nil
}

func (stubRepositoryManager) WorktreeStatus(executionContext context.Context, repositoryPath string) ([]string, error) {
	return nil, nil
}

func (stubRepositoryManager) GetCurrentBranch(executionContext context.Context, repositoryPath string) (string, error) {
	return "main", nil
}

func (stubRepositoryManager) GetRemoteURL(executionContext context.Context, repositoryPath string, remoteName string) (string, error) {
	return "https://example.com", nil
}

func (stubRepositoryManager) SetRemoteURL(executionContext context.Context, repositoryPath string, remoteName string, remoteURL string) error {
	return nil
}

type stubGitHubResolver struct{}

func (stubGitHubResolver) ResolveRepoMetadata(executionContext context.Context, repository string) (githubcli.RepositoryMetadata, error) {
	return githubcli.RepositoryMetadata{}, nil
}
