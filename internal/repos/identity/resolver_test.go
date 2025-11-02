package identity

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/repos/shared"
)

type stubRepositoryManager struct {
	remoteURL string
	urlError  error
}

func (manager *stubRepositoryManager) CheckCleanWorktree(context.Context, string) (bool, error) {
	return true, nil
}

func (manager *stubRepositoryManager) GetCurrentBranch(context.Context, string) (string, error) {
	return "main", nil
}

func (manager *stubRepositoryManager) GetRemoteURL(context.Context, string, string) (string, error) {
	return manager.remoteURL, manager.urlError
}

func (manager *stubRepositoryManager) SetRemoteURL(context.Context, string, string, string) error {
	return nil
}

type stubGitExecutor struct {
	responses map[string]execshell.ExecutionResult
	errors    map[string]error
}

func newStubGitExecutor() *stubGitExecutor {
	return &stubGitExecutor{
		responses: make(map[string]execshell.ExecutionResult),
		errors:    make(map[string]error),
	}
}

func (executor *stubGitExecutor) ExecuteGit(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func (executor *stubGitExecutor) ExecuteGitHubCLI(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	key := strings.Join(details.Arguments, " ")
	if result, exists := executor.responses[key]; exists {
		return result, executor.errors[key]
	}
	return execshell.ExecutionResult{}, nil
}

type stubMetadataResolver struct {
	metadata githubcli.RepositoryMetadata
	err      error
}

func (resolver stubMetadataResolver) ResolveRepoMetadata(context.Context, string) (githubcli.RepositoryMetadata, error) {
	return resolver.metadata, resolver.err
}

func TestResolveRemoteIdentityUsesMetadataCanonicalName(t *testing.T) {
	manager := &stubRepositoryManager{remoteURL: "git@github.com:owner/example.git"}
	resolver := stubMetadataResolver{metadata: githubcli.RepositoryMetadata{NameWithOwner: "canonical/example", DefaultBranch: "master"}}

	result, err := ResolveRemoteIdentity(context.Background(), RemoteResolutionDependencies{
		RepositoryManager: manager,
		MetadataResolver:  resolver,
	}, RemoteResolutionOptions{RepositoryPath: "/repo", RemoteName: shared.OriginRemoteNameConstant})

	require.NoError(t, err)
	require.True(t, result.RemoteDetected)
	require.NotNil(t, result.OwnerRepository)
	require.Equal(t, "canonical/example", result.OwnerRepository.String())
	require.NotNil(t, result.DefaultBranch)
	require.Equal(t, "master", result.DefaultBranch.String())
}

func TestResolveRemoteIdentityFallsBackToSearch(t *testing.T) {
	manager := &stubRepositoryManager{remoteURL: "git@github.com:legacy/example.git"}
	executor := newStubGitExecutor()
	executor.responses["api graphql -f query="+searchRepositoryQueryTemplate+" -F term=example in:name"] = execshell.ExecutionResult{
		StandardOutput: `{"data":{"search":{"nodes":[{"name":"example","nameWithOwner":"canonical/example","defaultBranchRef":{"name":"trunk"}}]}}}`,
	}

	result, err := ResolveRemoteIdentity(context.Background(), RemoteResolutionDependencies{
		RepositoryManager: manager,
		GitExecutor:       executor,
		MetadataResolver:  stubMetadataResolver{err: errors.New("gh: Not Found")},
	}, RemoteResolutionOptions{RepositoryPath: "/repo", RemoteName: shared.OriginRemoteNameConstant})

	require.NoError(t, err)
	require.True(t, result.RemoteDetected)
	require.NotNil(t, result.OwnerRepository)
	require.Equal(t, "canonical/example", result.OwnerRepository.String())
	require.NotNil(t, result.DefaultBranch)
	require.Equal(t, "trunk", result.DefaultBranch.String())
}

func TestResolveRemoteIdentityHandlesMissingRemote(t *testing.T) {
	result, err := ResolveRemoteIdentity(context.Background(), RemoteResolutionDependencies{}, RemoteResolutionOptions{RemoteName: shared.OriginRemoteNameConstant})

	require.NoError(t, err)
	require.False(t, result.RemoteDetected)
}
