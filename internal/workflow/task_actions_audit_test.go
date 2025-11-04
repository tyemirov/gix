package workflow

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubcli"
)

type stubRepositoryDiscoverer struct {
	repositories  []string
	recordedRoots [][]string
}

func (discoverer *stubRepositoryDiscoverer) DiscoverRepositories(roots []string) ([]string, error) {
	copied := append([]string(nil), roots...)
	discoverer.recordedRoots = append(discoverer.recordedRoots, copied)
	if len(discoverer.repositories) > 0 {
		return discoverer.repositories, nil
	}
	return roots, nil
}

type stubGitExecutor struct{}

func (executor *stubGitExecutor) ExecuteGit(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{StandardOutput: "true"}, nil
}

func (executor *stubGitExecutor) ExecuteGitHubCLI(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type stubGitRepositoryManager struct {
	remoteURL string
}

func (manager *stubGitRepositoryManager) CheckCleanWorktree(ctx context.Context, repositoryPath string) (bool, error) {
	return true, nil
}

func (manager *stubGitRepositoryManager) GetCurrentBranch(ctx context.Context, repositoryPath string) (string, error) {
	return "main", nil
}

func (manager *stubGitRepositoryManager) GetRemoteURL(ctx context.Context, repositoryPath string, remoteName string) (string, error) {
	return manager.remoteURL, nil
}

func (manager *stubGitRepositoryManager) SetRemoteURL(ctx context.Context, repositoryPath string, remoteName string, remoteURL string) error {
	return nil
}

type stubGitHubMetadataResolver struct {
	metadata githubcli.RepositoryMetadata
}

func (resolver *stubGitHubMetadataResolver) ResolveRepoMetadata(ctx context.Context, repository string) (githubcli.RepositoryMetadata, error) {
	return resolver.metadata, nil
}

func TestHandleAuditReportActionUsesUpdatedRepositoryPaths(testInstance *testing.T) {
	testInstance.Parallel()

	executionContext := context.Background()
	temporaryDirectory := testInstance.TempDir()
	originalPath := filepath.Join(temporaryDirectory, "original")
	renamedPath := filepath.Join(temporaryDirectory, "renamed")
	outputPath := filepath.Join(temporaryDirectory, "audit.csv")

	discoverer := &stubRepositoryDiscoverer{repositories: []string{renamedPath}}
	gitExecutor := &stubGitExecutor{}
	gitRepositoryManager := &stubGitRepositoryManager{remoteURL: "https://github.com/example/repo.git"}
	metadataResolver := &stubGitHubMetadataResolver{metadata: githubcli.RepositoryMetadata{NameWithOwner: "example/repo", DefaultBranch: "main"}}

	auditService := audit.NewService(discoverer, gitRepositoryManager, gitExecutor, metadataResolver, &bytes.Buffer{}, &bytes.Buffer{})
	environment := &Environment{
		AuditService: auditService,
		Output:       &bytes.Buffer{},
		State: &State{
			Roots:        []string{originalPath},
			Repositories: []*RepositoryState{{Path: renamedPath}},
		},
	}

	repository := &RepositoryState{Path: originalPath}
	parameters := map[string]any{
		"output": outputPath,
		"depth":  string(audit.InspectionDepthMinimal),
	}

	executionError := handleAuditReportAction(executionContext, environment, repository, parameters)
	require.NoError(testInstance, executionError)
	require.True(testInstance, environment.auditReportExecuted)
	require.Len(testInstance, discoverer.recordedRoots, 1)
	require.Equal(testInstance, []string{renamedPath}, discoverer.recordedRoots[0])
}

func TestHandleAuditReportActionFallsBackToStateRoots(testInstance *testing.T) {
	testInstance.Parallel()

	executionContext := context.Background()
	temporaryDirectory := testInstance.TempDir()
	stateRoot := filepath.Join(temporaryDirectory, "repository")
	outputPath := filepath.Join(temporaryDirectory, "audit.csv")

	discoverer := &stubRepositoryDiscoverer{}
	gitExecutor := &stubGitExecutor{}
	gitRepositoryManager := &stubGitRepositoryManager{remoteURL: "https://github.com/example/repo.git"}
	metadataResolver := &stubGitHubMetadataResolver{metadata: githubcli.RepositoryMetadata{NameWithOwner: "example/repo", DefaultBranch: "main"}}

	auditService := audit.NewService(discoverer, gitRepositoryManager, gitExecutor, metadataResolver, &bytes.Buffer{}, &bytes.Buffer{})
	environment := &Environment{
		AuditService: auditService,
		Output:       &bytes.Buffer{},
		State: &State{
			Roots:        []string{stateRoot},
			Repositories: []*RepositoryState{},
		},
	}

	parameters := map[string]any{
		"output": outputPath,
		"depth":  string(audit.InspectionDepthMinimal),
	}

	executionError := handleAuditReportAction(executionContext, environment, &RepositoryState{}, parameters)
	require.NoError(testInstance, executionError)
	require.Len(testInstance, discoverer.recordedRoots, 1)
	require.Equal(testInstance, []string{stateRoot}, discoverer.recordedRoots[0])
}
