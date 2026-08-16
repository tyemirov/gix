package migrate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubauth"
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
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

type dirtyPreflightExecutor struct {
	gitCommands []string
}

func (executor *dirtyPreflightExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.gitCommands = append(executor.gitCommands, strings.Join(details.Arguments, " "))
	if len(details.Arguments) > 0 && details.Arguments[0] == "status" {
		return execshell.ExecutionResult{StandardOutput: " M README.md\x00"}, nil
	}
	return execshell.ExecutionResult{}, nil
}

func (*dirtyPreflightExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	panic("GitHub CLI mutation reached after dirty preflight")
}

type dirtyPreflightGitHubOperations struct{}

func (dirtyPreflightGitHubOperations) ResolveRepoMetadata(context.Context, string) (githubcli.RepositoryMetadata, error) {
	panic("metadata resolution reached after dirty preflight")
}

func (dirtyPreflightGitHubOperations) GetPagesConfig(context.Context, string) (githubcli.PagesStatus, error) {
	panic("Pages lookup reached after dirty preflight")
}

func (dirtyPreflightGitHubOperations) UpdatePagesConfig(context.Context, string, githubcli.PagesConfiguration) error {
	panic("Pages mutation reached after dirty preflight")
}

func (dirtyPreflightGitHubOperations) ListPullRequests(context.Context, string, githubcli.PullRequestListOptions) ([]githubcli.PullRequest, error) {
	panic("pull-request lookup reached after dirty preflight")
}

func (dirtyPreflightGitHubOperations) UpdatePullRequestBase(context.Context, string, int, string) error {
	panic("pull-request mutation reached after dirty preflight")
}

func (dirtyPreflightGitHubOperations) ClosePullRequest(context.Context, string, int) error {
	panic("pull-request mutation reached after dirty preflight")
}

func (dirtyPreflightGitHubOperations) SetDefaultBranch(context.Context, string, string) error {
	panic("default-branch mutation reached after dirty preflight")
}

func (dirtyPreflightGitHubOperations) CheckBranchProtection(context.Context, string, string) (bool, error) {
	panic("branch-protection lookup reached after dirty preflight")
}

type recordingGitHubOperations struct {
	pagesError         error
	listError          error
	retargetErrors     map[int]error
	closeErrors        map[int]error
	protectionError    error
	defaultBranchError error
	defaultBranchSet   bool
	pullRequests       []githubcli.PullRequest
	retargetedNumbers  []int
	closedNumbers      []int
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

func (operations *recordingGitHubOperations) ClosePullRequest(_ context.Context, _ string, pullRequestNumber int) error {
	operations.closedNumbers = append(operations.closedNumbers, pullRequestNumber)
	if operations.closeErrors != nil {
		if err, exists := operations.closeErrors[pullRequestNumber]; exists {
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

func testGitHubContext() context.Context {
	return githubauth.WithCredential(context.Background(), testGitHubTokenValue)
}

func TestServiceExecuteRejectsDirtyWorktreeBeforeTargetPreparation(testInstance *testing.T) {
	executor := &dirtyPreflightExecutor{}
	repositoryManager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(testInstance, managerError)

	service, serviceError := NewService(ServiceDependencies{
		Logger:            zap.NewNop(),
		RepositoryManager: repositoryManager,
		GitHubClient:      dirtyPreflightGitHubOperations{},
		GitExecutor:       executor,
	})
	require.NoError(testInstance, serviceError)

	_, executionError := service.Execute(testGitHubContext(), MigrationOptions{
		RepositoryPath:       testInstance.TempDir(),
		RepositoryRemoteName: "origin",
		RepositoryIdentifier: "owner/example",
		WorkflowsDirectory:   ".github/workflows",
		SourceBranch:         BranchName("main"),
		TargetBranch:         BranchName("master"),
		PushUpdates:          true,
	})

	require.ErrorIs(testInstance, executionError, errCleanWorktreeRequired)
	require.Equal(testInstance, []string{"status --porcelain=v1 -z"}, executor.gitCommands)
}

func TestServiceExecuteContinuesWhenPagesLookupFails(testInstance *testing.T) {
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
		SourceBranch:         BranchName("main"),
		TargetBranch:         BranchName("master"),
		PushUpdates:          false,
		DeleteSourceBranch:   false,
	}

	result, executionError := service.Execute(testGitHubContext(), options)
	require.NoError(testInstance, executionError)
	require.False(testInstance, result.PagesConfigurationUpdated)
	require.True(testInstance, result.DefaultBranchUpdated)
	require.True(testInstance, githubOperations.defaultBranchSet)
	require.Len(testInstance, result.Warnings, 1)
	require.Contains(testInstance, result.Warnings[0], "PAGES-SKIP")
}

func TestServiceExecuteWarnsWhenRetargetFails(testInstance *testing.T) {
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
		SourceBranch:         BranchName("main"),
		TargetBranch:         BranchName("master"),
		PushUpdates:          false,
		DeleteSourceBranch:   false,
	}

	result, executionError := service.Execute(testGitHubContext(), options)
	require.NoError(testInstance, executionError)
	require.Contains(testInstance, strings.Join(result.Warnings, " "), "PR-RETARGET-SKIP")
}

func TestServiceExecuteClosesPullRequestFromTargetBranch(testInstance *testing.T) {
	repositoryExecutor := stubGitCommandExecutor{}
	repositoryManager, managerError := gitrepo.NewRepositoryManager(repositoryExecutor)
	require.NoError(testInstance, managerError)

	githubOperations := &recordingGitHubOperations{
		pullRequests: []githubcli.PullRequest{
			{Number: 1, HeadRefName: "master", BaseRefName: "main"},
			{Number: 2, HeadRefName: "feature/example", BaseRefName: "main"},
		},
	}
	service, serviceError := NewService(ServiceDependencies{
		Logger:            zap.NewNop(),
		RepositoryManager: repositoryManager,
		GitHubClient:      githubOperations,
		GitExecutor:       stubCommandExecutor{},
	})
	require.NoError(testInstance, serviceError)

	result, executionError := service.Execute(testGitHubContext(), MigrationOptions{
		RepositoryPath:       testInstance.TempDir(),
		RepositoryRemoteName: "origin",
		RepositoryIdentifier: "owner/example",
		WorkflowsDirectory:   ".github/workflows",
		SourceBranch:         BranchName("main"),
		TargetBranch:         BranchName("master"),
	})

	require.NoError(testInstance, executionError)
	require.Equal(testInstance, []int{1}, result.ClosedPullRequests)
	require.Equal(testInstance, []int{2}, result.RetargetedPullRequests)
	require.Equal(testInstance, []int{1}, githubOperations.closedNumbers)
	require.Equal(testInstance, []int{2}, githubOperations.retargetedNumbers)
	require.False(testInstance, result.SafetyStatus.SafeToDelete)
	require.Empty(testInstance, result.Warnings)
}

func TestServiceExecuteKeepsFailedPromotionCloseAsSafetyBlocker(testInstance *testing.T) {
	repositoryExecutor := stubGitCommandExecutor{}
	repositoryManager, managerError := gitrepo.NewRepositoryManager(repositoryExecutor)
	require.NoError(testInstance, managerError)

	closeError := makeCommandFailedError("fatal: cannot close PR")
	githubOperations := &recordingGitHubOperations{
		pullRequests: []githubcli.PullRequest{{Number: 1, HeadRefName: "master", BaseRefName: "main"}},
		closeErrors:  map[int]error{1: closeError},
	}
	service, serviceError := NewService(ServiceDependencies{
		Logger:            zap.NewNop(),
		RepositoryManager: repositoryManager,
		GitHubClient:      githubOperations,
		GitExecutor:       stubCommandExecutor{},
	})
	require.NoError(testInstance, serviceError)

	result, executionError := service.Execute(testGitHubContext(), MigrationOptions{
		RepositoryPath:       testInstance.TempDir(),
		RepositoryRemoteName: "origin",
		RepositoryIdentifier: "owner/example",
		WorkflowsDirectory:   ".github/workflows",
		SourceBranch:         BranchName("main"),
		TargetBranch:         BranchName("master"),
	})

	require.NoError(testInstance, executionError)
	require.Empty(testInstance, result.ClosedPullRequests)
	require.Empty(testInstance, result.RetargetedPullRequests)
	require.Equal(testInstance, []int{1}, githubOperations.closedNumbers)
	require.Empty(testInstance, githubOperations.retargetedNumbers)
	require.False(testInstance, result.SafetyStatus.SafeToDelete)
	require.Contains(testInstance, strings.Join(result.Warnings, " "), "PR-CLOSE-SKIP: #1")
}

func TestServiceExecuteWarnsWhenBranchProtectionFails(testInstance *testing.T) {
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
		SourceBranch:         BranchName("main"),
		TargetBranch:         BranchName("master"),
		PushUpdates:          false,
		DeleteSourceBranch:   false,
	}

	result, executionError := service.Execute(testGitHubContext(), options)
	require.NoError(testInstance, executionError)
	require.Contains(testInstance, strings.Join(result.Warnings, " "), "PROTECTION-SKIP")
	require.False(testInstance, result.SafetyStatus.SafeToDelete)
}

func TestServiceExecuteReturnsActionableDefaultBranchError(testInstance *testing.T) {
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
		SourceBranch:         BranchName("main"),
		TargetBranch:         BranchName("master"),
		PushUpdates:          false,
		DeleteSourceBranch:   false,
	}

	_, executionError := service.Execute(testGitHubContext(), options)
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
		SourceBranch:         BranchName("main"),
		TargetBranch:         BranchName("master"),
		PushUpdates:          false,
		DeleteSourceBranch:   false,
	}

	result, executionError := service.Execute(testGitHubContext(), options)

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
		SourceBranch:         BranchName("main"),
		TargetBranch:         BranchName("master"),
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
	repositoryExecutor := stubGitCommandExecutor{}
	repositoryManager, managerError := gitrepo.NewRepositoryManager(repositoryExecutor)
	require.NoError(testInstance, managerError)

	githubOperations := &recordingGitHubOperations{
		defaultBranchError: githubauth.NewMissingTokenError("default-branch", true),
	}

	service, serviceError := NewService(ServiceDependencies{
		Logger:            zap.NewNop(),
		RepositoryManager: repositoryManager,
		GitHubClient:      githubOperations,
		GitExecutor:       stubCommandExecutor{},
	})
	require.NoError(testInstance, serviceError)

	repositoryPath := testInstance.TempDir()
	workflowsDirectory := filepath.Join(repositoryPath, ".github", "workflows")
	require.NoError(testInstance, os.MkdirAll(workflowsDirectory, 0o755))
	workflowPath := filepath.Join(workflowsDirectory, "ci.yml")
	originalWorkflow := "on:\n  push:\n    branches:\n      - main\n"
	require.NoError(testInstance, os.WriteFile(workflowPath, []byte(originalWorkflow), 0o644))

	options := MigrationOptions{
		RepositoryPath:       repositoryPath,
		RepositoryRemoteName: "origin",
		RepositoryIdentifier: "owner/example",
		WorkflowsDirectory:   ".github/workflows",
		SourceBranch:         BranchName("main"),
		TargetBranch:         BranchName("master"),
		PushUpdates:          true,
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

	workflowContent, readError := os.ReadFile(workflowPath)
	require.NoError(testInstance, readError)
	require.Equal(testInstance, originalWorkflow, string(workflowContent))
}
