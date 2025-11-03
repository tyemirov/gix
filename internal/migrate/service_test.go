package migrate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/githubauth"
	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/gitrepo"
)

type stubCommandExecutor struct{}

func (stubCommandExecutor) ExecuteGit(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func (stubCommandExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type stubGitCommandExecutor struct{}

func (stubGitCommandExecutor) ExecuteGit(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type recordingGitHubOperations struct {
	pagesError         error
	listError          error
	retargetErrors     map[int]error
	protectionError    error
	defaultBranchError error
	defaultBranchSet   bool
	pullRequests       []githubcli.PullRequest
	retargetedNumbers  []int
}

func (operations *recordingGitHubOperations) ResolveRepoMetadata(context.Context, string) (githubcli.RepositoryMetadata, error) {
	return githubcli.RepositoryMetadata{}, nil
}

func (operations *recordingGitHubOperations) GetPagesConfig(context.Context, string) (githubcli.PagesStatus, error) {
	if operations.pagesError != nil {
		return githubcli.PagesStatus{}, operations.pagesError
	}
	return githubcli.PagesStatus{}, nil
}

func (operations *recordingGitHubOperations) UpdatePagesConfig(context.Context, string, githubcli.PagesConfiguration) error {
	return nil
}

func (operations *recordingGitHubOperations) ListPullRequests(context.Context, string, githubcli.PullRequestListOptions) ([]githubcli.PullRequest, error) {
	if operations.listError != nil {
		return nil, operations.listError
	}
	return append([]githubcli.PullRequest(nil), operations.pullRequests...), nil
}

func (operations *recordingGitHubOperations) UpdatePullRequestBase(_ context.Context, _ string, pullRequestNumber int, _ string) error {
	operations.retargetedNumbers = append(operations.retargetedNumbers, pullRequestNumber)
	if operations.retargetErrors != nil {
		if err, exists := operations.retargetErrors[pullRequestNumber]; exists {
			return err
		}
	}
	return nil
}

func (operations *recordingGitHubOperations) SetDefaultBranch(context.Context, string, string) error {
	if operations.defaultBranchError != nil {
		return operations.defaultBranchError
	}
	operations.defaultBranchSet = true
	return nil
}

func (operations *recordingGitHubOperations) CheckBranchProtection(context.Context, string, string) (bool, error) {
	if operations.protectionError != nil {
		return false, operations.protectionError
	}
	return false, nil
}

func makeCommandFailedError(message string) error {
	return execshell.CommandFailedError{
		Command: execshell.ShellCommand{Name: execshell.CommandGit},
		Result: execshell.ExecutionResult{
			ExitCode:      128,
			StandardError: message,
		},
	}
}

const testGitHubTokenValue = "test-token"

func TestServiceExecuteContinuesWhenPagesLookupFails(testInstance *testing.T) {
	testInstance.Setenv(githubauth.EnvGitHubCLIToken, testGitHubTokenValue)
	testInstance.Setenv(githubauth.EnvGitHubToken, testGitHubTokenValue)

	repositoryExecutor := stubGitCommandExecutor{}
	repositoryManager, managerError := gitrepo.NewRepositoryManager(repositoryExecutor)
	require.NoError(testInstance, managerError)

	pagesLookupError := githubcli.OperationError{
		Operation: githubcli.OperationName("GetPagesConfig"),
		Cause:     errors.New("gh command exited with code 1"),
	}

	githubOperations := &recordingGitHubOperations{pagesError: pagesLookupError}

	service, serviceError := NewService(ServiceDependencies{
		Logger:            zap.NewNop(),
		RepositoryManager: repositoryManager,
		GitHubClient:      githubOperations,
		GitExecutor:       stubCommandExecutor{},
	})
	require.NoError(testInstance, serviceError)

	options := MigrationOptions{
		RepositoryPath:       testInstance.TempDir(),
		RepositoryRemoteName: "origin",
		RepositoryIdentifier: "owner/example",
		WorkflowsDirectory:   ".github/workflows",
		SourceBranch:         BranchMain,
		TargetBranch:         BranchMaster,
		PushUpdates:          false,
		DeleteSourceBranch:   false,
	}

	result, executionError := service.Execute(context.Background(), options)
	require.NoError(testInstance, executionError)
	require.False(testInstance, result.PagesConfigurationUpdated)
	require.True(testInstance, result.DefaultBranchUpdated)
	require.True(testInstance, githubOperations.defaultBranchSet)
	require.Len(testInstance, result.Warnings, 1)
	require.Contains(testInstance, result.Warnings[0], "PAGES-SKIP")
}

func TestServiceExecuteWarnsWhenRetargetFails(testInstance *testing.T) {
	testInstance.Setenv(githubauth.EnvGitHubCLIToken, testGitHubTokenValue)
	testInstance.Setenv(githubauth.EnvGitHubToken, testGitHubTokenValue)

	repositoryExecutor := stubGitCommandExecutor{}
	repositoryManager, managerError := gitrepo.NewRepositoryManager(repositoryExecutor)
	require.NoError(testInstance, managerError)

	retargetError := makeCommandFailedError("fatal: cannot update PR")

	githubOperations := &recordingGitHubOperations{
		pullRequests:   []githubcli.PullRequest{{Number: 42}},
		retargetErrors: map[int]error{42: retargetError},
	}

	service, serviceError := NewService(ServiceDependencies{
		Logger:            zap.NewNop(),
		RepositoryManager: repositoryManager,
		GitHubClient:      githubOperations,
		GitExecutor:       stubCommandExecutor{},
	})
	require.NoError(testInstance, serviceError)

	options := MigrationOptions{
		RepositoryPath:       testInstance.TempDir(),
		RepositoryRemoteName: "origin",
		RepositoryIdentifier: "owner/example",
		WorkflowsDirectory:   ".github/workflows",
		SourceBranch:         BranchMain,
		TargetBranch:         BranchMaster,
		PushUpdates:          false,
		DeleteSourceBranch:   false,
	}

	result, executionError := service.Execute(context.Background(), options)
	require.NoError(testInstance, executionError)
	require.Contains(testInstance, strings.Join(result.Warnings, " "), "PR-RETARGET-SKIP")
}

func TestServiceExecuteWarnsWhenBranchProtectionFails(testInstance *testing.T) {
	testInstance.Setenv(githubauth.EnvGitHubCLIToken, testGitHubTokenValue)
	testInstance.Setenv(githubauth.EnvGitHubToken, testGitHubTokenValue)

	repositoryExecutor := stubGitCommandExecutor{}
	repositoryManager, managerError := gitrepo.NewRepositoryManager(repositoryExecutor)
	require.NoError(testInstance, managerError)

	githubOperations := &recordingGitHubOperations{
		protectionError: makeCommandFailedError("fatal: protection read failed"),
	}

	service, serviceError := NewService(ServiceDependencies{
		Logger:            zap.NewNop(),
		RepositoryManager: repositoryManager,
		GitHubClient:      githubOperations,
		GitExecutor:       stubCommandExecutor{},
	})
	require.NoError(testInstance, serviceError)

	options := MigrationOptions{
		RepositoryPath:       testInstance.TempDir(),
		RepositoryRemoteName: "origin",
		RepositoryIdentifier: "owner/example",
		WorkflowsDirectory:   ".github/workflows",
		SourceBranch:         BranchMain,
		TargetBranch:         BranchMaster,
		PushUpdates:          false,
		DeleteSourceBranch:   false,
	}

	result, executionError := service.Execute(context.Background(), options)
	require.NoError(testInstance, executionError)
	require.Contains(testInstance, strings.Join(result.Warnings, " "), "PROTECTION-SKIP")
	require.False(testInstance, result.SafetyStatus.SafeToDelete)
}

func TestServiceExecuteReturnsActionableDefaultBranchError(testInstance *testing.T) {
	testInstance.Setenv(githubauth.EnvGitHubCLIToken, testGitHubTokenValue)
	testInstance.Setenv(githubauth.EnvGitHubToken, testGitHubTokenValue)

	repositoryExecutor := stubGitCommandExecutor{}
	repositoryManager, managerError := gitrepo.NewRepositoryManager(repositoryExecutor)
	require.NoError(testInstance, managerError)

	commandFailure := execshell.CommandFailedError{
		Command: execshell.ShellCommand{Name: execshell.CommandGitHub},
		Result: execshell.ExecutionResult{
			ExitCode:      1,
			StandardError: "GraphQL: branch not found",
		},
	}

	defaultBranchError := githubcli.OperationError{
		Operation: githubcli.OperationName("UpdateDefaultBranch"),
		Cause:     commandFailure,
	}

	githubOperations := &recordingGitHubOperations{
		defaultBranchError: defaultBranchError,
	}

	service, serviceError := NewService(ServiceDependencies{
		Logger:            zap.NewNop(),
		RepositoryManager: repositoryManager,
		GitHubClient:      githubOperations,
		GitExecutor:       stubCommandExecutor{},
	})
	require.NoError(testInstance, serviceError)

	repositoryPath := testInstance.TempDir()

	options := MigrationOptions{
		RepositoryPath:       repositoryPath,
		RepositoryRemoteName: "origin",
		RepositoryIdentifier: "owner/example",
		WorkflowsDirectory:   ".github/workflows",
		SourceBranch:         BranchMain,
		TargetBranch:         BranchMaster,
		PushUpdates:          false,
		DeleteSourceBranch:   false,
	}

	_, executionError := service.Execute(context.Background(), options)
	require.Error(testInstance, executionError)

	var updateError DefaultBranchUpdateError
	require.ErrorAs(testInstance, executionError, &updateError)
	require.Equal(testInstance, repositoryPath, updateError.RepositoryPath)
	require.Equal(testInstance, options.RepositoryIdentifier, updateError.RepositoryIdentifier)
	require.Equal(testInstance, options.SourceBranch, updateError.SourceBranch)
	require.Equal(testInstance, options.TargetBranch, updateError.TargetBranch)

	errorMessage := executionError.Error()
	require.Contains(testInstance, errorMessage, "DEFAULT-BRANCH-UPDATE")
	require.Contains(testInstance, errorMessage, repositoryPath)
	require.Contains(testInstance, errorMessage, options.RepositoryIdentifier)
	require.Contains(testInstance, errorMessage, "source=main")
	require.Contains(testInstance, errorMessage, "target=master")
	require.Contains(testInstance, errorMessage, "GraphQL: branch not found")
}

func TestServiceExecuteSkipsDefaultBranchWhenRepositoryMissing(testInstance *testing.T) {
	testInstance.Setenv(githubauth.EnvGitHubCLIToken, testGitHubTokenValue)
	testInstance.Setenv(githubauth.EnvGitHubToken, testGitHubTokenValue)

	repositoryExecutor := stubGitCommandExecutor{}
	repositoryManager, managerError := gitrepo.NewRepositoryManager(repositoryExecutor)
	require.NoError(testInstance, managerError)

	commandFailure := execshell.CommandFailedError{
		Command: execshell.ShellCommand{Name: execshell.CommandGitHub},
		Result: execshell.ExecutionResult{
			ExitCode:      1,
			StandardError: "gh: Not Found (HTTP 404)",
		},
	}

	githubOperations := &recordingGitHubOperations{
		defaultBranchError: githubcli.OperationError{
			Operation: githubcli.OperationName("UpdateDefaultBranch"),
			Cause:     commandFailure,
		},
	}

	service, serviceError := NewService(ServiceDependencies{
		Logger:            zap.NewNop(),
		RepositoryManager: repositoryManager,
		GitHubClient:      githubOperations,
		GitExecutor:       stubCommandExecutor{},
	})
	require.NoError(testInstance, serviceError)

	options := MigrationOptions{
		RepositoryPath:       testInstance.TempDir(),
		RepositoryRemoteName: "origin",
		RepositoryIdentifier: "owner/example",
		WorkflowsDirectory:   ".github/workflows",
		SourceBranch:         BranchMain,
		TargetBranch:         BranchMaster,
		PushUpdates:          false,
		DeleteSourceBranch:   false,
	}

	result, executionError := service.Execute(context.Background(), options)

	require.NoError(testInstance, executionError)
	require.False(testInstance, result.DefaultBranchUpdated)
	require.False(testInstance, githubOperations.defaultBranchSet)
	require.Empty(testInstance, result.Warnings)
}

func TestServiceExecuteSkipsRemoteOperationsWhenIdentifierMissing(testInstance *testing.T) {
	repositoryExecutor := stubGitCommandExecutor{}
	repositoryManager, managerError := gitrepo.NewRepositoryManager(repositoryExecutor)
	require.NoError(testInstance, managerError)

	githubOperations := &recordingGitHubOperations{}

	service, serviceError := NewService(ServiceDependencies{
		Logger:            zap.NewNop(),
		RepositoryManager: repositoryManager,
		GitHubClient:      githubOperations,
		GitExecutor:       stubCommandExecutor{},
	})
	require.NoError(testInstance, serviceError)

	repositoryPath := testInstance.TempDir()

	options := MigrationOptions{
		RepositoryPath:       repositoryPath,
		RepositoryRemoteName: "origin",
		RepositoryIdentifier: "",
		WorkflowsDirectory:   ".github/workflows",
		SourceBranch:         BranchMain,
		TargetBranch:         BranchMaster,
		PushUpdates:          false,
		DeleteSourceBranch:   false,
	}

	result, executionError := service.Execute(context.Background(), options)

	require.NoError(testInstance, executionError)
	require.False(testInstance, result.DefaultBranchUpdated)
	require.False(testInstance, githubOperations.defaultBranchSet)
	require.Empty(testInstance, result.Warnings)
}

func TestServiceExecuteFailsWhenGitHubTokenMissing(testInstance *testing.T) {
	testInstance.Setenv(githubauth.EnvGitHubCLIToken, "")
	testInstance.Setenv(githubauth.EnvGitHubToken, "")
	testInstance.Setenv(githubauth.EnvGitHubAPIToken, "")

	repositoryExecutor := stubGitCommandExecutor{}
	repositoryManager, managerError := gitrepo.NewRepositoryManager(repositoryExecutor)
	require.NoError(testInstance, managerError)

	githubOperations := &recordingGitHubOperations{}

	service, serviceError := NewService(ServiceDependencies{
		Logger:            zap.NewNop(),
		RepositoryManager: repositoryManager,
		GitHubClient:      githubOperations,
		GitExecutor:       stubCommandExecutor{},
	})
	require.NoError(testInstance, serviceError)

	options := MigrationOptions{
		RepositoryPath:       testInstance.TempDir(),
		RepositoryRemoteName: "origin",
		RepositoryIdentifier: "owner/example",
		WorkflowsDirectory:   ".github/workflows",
		SourceBranch:         BranchMain,
		TargetBranch:         BranchMaster,
		PushUpdates:          false,
		DeleteSourceBranch:   false,
	}

	_, executionError := service.Execute(context.Background(), options)
	require.Error(testInstance, executionError)

	var updateError DefaultBranchUpdateError
	require.ErrorAs(testInstance, executionError, &updateError)

	var missingTokenError githubauth.MissingTokenError
	require.ErrorAs(testInstance, executionError, &missingTokenError)
	require.True(testInstance, missingTokenError.CriticalRequirement())

	errorMessage := updateError.Error()
	require.Contains(testInstance, errorMessage, "DEFAULT-BRANCH-UPDATE")
	require.Contains(testInstance, errorMessage, "missing GitHub authentication token")
	require.False(testInstance, githubOperations.defaultBranchSet)
}
