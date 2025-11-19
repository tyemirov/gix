package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
)

const (
	gitStatusSubcommandConstant               = "status"
	gitStatusPorcelainFlagConstant            = "--porcelain"
	gitRevParseSubcommandConstant             = "rev-parse"
	gitAbbrevRefFlagConstant                  = "--abbrev-ref"
	gitHeadReferenceConstant                  = "HEAD"
	gitCheckoutSubcommandConstant             = "checkout"
	gitBranchSubcommandConstant               = "branch"
	gitDeleteFlagConstant                     = "--delete"
	gitForceFlagConstant                      = "--force"
	gitRemoteSubcommandConstant               = "remote"
	gitRemoteGetURLSubcommandConstant         = "get-url"
	gitRemoteSetURLSubcommandConstant         = "set-url"
	repositoryPathFieldNameConstant           = "repository_path"
	branchNameFieldNameConstant               = "branch_name"
	startPointFieldNameConstant               = "start_point"
	remoteNameFieldNameConstant               = "remote_name"
	remoteURLFieldNameConstant                = "remote_url"
	requiredValueMessageConstant              = "value required"
	executorNotConfiguredMessageConstant      = "git executor not configured"
	repositoryOperationErrorTemplateConstant  = "%s operation failed"
	repositoryOperationErrorWithCauseConstant = "%s operation failed: %s"
	invalidRepositoryInputTemplateConstant    = "%s: %s"
	cleanWorktreeOperationNameConstant        = RepositoryOperationName("CheckCleanWorktree")
	checkoutBranchOperationNameConstant       = RepositoryOperationName("CheckoutBranch")
	createBranchOperationNameConstant         = RepositoryOperationName("CreateBranch")
	deleteBranchOperationNameConstant         = RepositoryOperationName("DeleteBranch")
	currentBranchOperationNameConstant        = RepositoryOperationName("GetCurrentBranch")
	getRemoteURLOperationNameConstant         = RepositoryOperationName("GetRemoteURL")
	setRemoteURLOperationNameConstant         = RepositoryOperationName("SetRemoteURL")
)

// GitCommandExecutor exposes the subset of execshell functionality required by RepositoryManager.
type GitCommandExecutor interface {
	ExecuteGit(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error)
}

// RepositoryManager coordinates Git operations through execshell.
type RepositoryManager struct {
	executor GitCommandExecutor
}

var (
	// ErrGitExecutorNotConfigured indicates the RepositoryManager was constructed without a git executor.
	ErrGitExecutorNotConfigured = errors.New(executorNotConfiguredMessageConstant)
)

// InvalidRepositoryInputError indicates validation failures for repository operations.
type InvalidRepositoryInputError struct {
	FieldName string
	Message   string
}

// Error describes the validation failure.
func (inputError InvalidRepositoryInputError) Error() string {
	return fmt.Sprintf(invalidRepositoryInputTemplateConstant, inputError.FieldName, inputError.Message)
}

// RepositoryOperationName captures descriptive names for repository operations.
type RepositoryOperationName string

// RepositoryOperationError wraps execution failures for git operations.
type RepositoryOperationError struct {
	Operation RepositoryOperationName
	Cause     error
}

// Error describes the repository operation failure.
func (operationError RepositoryOperationError) Error() string {
	if operationError.Cause == nil {
		return fmt.Sprintf(repositoryOperationErrorTemplateConstant, operationError.Operation)
	}
	return fmt.Sprintf(repositoryOperationErrorWithCauseConstant, operationError.Operation, operationError.Cause)
}

// Unwrap exposes the underlying error.
func (operationError RepositoryOperationError) Unwrap() error {
	return operationError.Cause
}

// NewRepositoryManager constructs a RepositoryManager for the provided executor.
func NewRepositoryManager(executor GitCommandExecutor) (*RepositoryManager, error) {
	if executor == nil {
		return nil, ErrGitExecutorNotConfigured
	}
	return &RepositoryManager{executor: executor}, nil
}

// CheckCleanWorktree returns true when the repository has no staged or unstaged changes.
func (manager *RepositoryManager) CheckCleanWorktree(executionContext context.Context, repositoryPath string) (bool, error) {
	status, statusError := manager.WorktreeStatus(executionContext, repositoryPath)
	if statusError != nil {
		return false, statusError
	}
	return len(status) == 0, nil
}

// WorktreeStatus returns the porcelain status entries for the repository.
func (manager *RepositoryManager) WorktreeStatus(executionContext context.Context, repositoryPath string) ([]string, error) {
	trimmedPath := strings.TrimSpace(repositoryPath)
	if len(trimmedPath) == 0 {
		return nil, InvalidRepositoryInputError{FieldName: repositoryPathFieldNameConstant, Message: requiredValueMessageConstant}
	}

	commandDetails := execshell.CommandDetails{
		Arguments:        []string{gitStatusSubcommandConstant, gitStatusPorcelainFlagConstant},
		WorkingDirectory: trimmedPath,
	}

	executionResult, executionError := manager.executor.ExecuteGit(executionContext, commandDetails)
	if executionError != nil {
		return nil, RepositoryOperationError{Operation: cleanWorktreeOperationNameConstant, Cause: executionError}
	}

	trimmedOutput := strings.TrimSpace(executionResult.StandardOutput)
	if len(trimmedOutput) == 0 {
		return nil, nil
	}

	lines := strings.Split(trimmedOutput, "\n")
	entries := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); len(trimmed) > 0 {
			entries = append(entries, trimmed)
		}
	}
	return entries, nil
}

// CheckoutBranch checks out an existing branch.
func (manager *RepositoryManager) CheckoutBranch(executionContext context.Context, repositoryPath string, branchName string) error {
	trimmedPath := strings.TrimSpace(repositoryPath)
	if len(trimmedPath) == 0 {
		return InvalidRepositoryInputError{FieldName: repositoryPathFieldNameConstant, Message: requiredValueMessageConstant}
	}

	trimmedBranch := strings.TrimSpace(branchName)
	if len(trimmedBranch) == 0 {
		return InvalidRepositoryInputError{FieldName: branchNameFieldNameConstant, Message: requiredValueMessageConstant}
	}

	commandDetails := execshell.CommandDetails{
		Arguments:        []string{gitCheckoutSubcommandConstant, trimmedBranch},
		WorkingDirectory: trimmedPath,
	}

	_, executionError := manager.executor.ExecuteGit(executionContext, commandDetails)
	if executionError != nil {
		return RepositoryOperationError{Operation: checkoutBranchOperationNameConstant, Cause: executionError}
	}
	return nil
}

// CreateBranch creates a new branch optionally from a start point.
func (manager *RepositoryManager) CreateBranch(executionContext context.Context, repositoryPath string, branchName string, startPoint string) error {
	trimmedPath := strings.TrimSpace(repositoryPath)
	if len(trimmedPath) == 0 {
		return InvalidRepositoryInputError{FieldName: repositoryPathFieldNameConstant, Message: requiredValueMessageConstant}
	}

	trimmedBranch := strings.TrimSpace(branchName)
	if len(trimmedBranch) == 0 {
		return InvalidRepositoryInputError{FieldName: branchNameFieldNameConstant, Message: requiredValueMessageConstant}
	}

	commandArguments := []string{gitBranchSubcommandConstant, trimmedBranch}
	trimmedStartPoint := strings.TrimSpace(startPoint)
	if len(trimmedStartPoint) > 0 {
		commandArguments = append(commandArguments, trimmedStartPoint)
	}

	commandDetails := execshell.CommandDetails{
		Arguments:        commandArguments,
		WorkingDirectory: trimmedPath,
	}

	_, executionError := manager.executor.ExecuteGit(executionContext, commandDetails)
	if executionError != nil {
		return RepositoryOperationError{Operation: createBranchOperationNameConstant, Cause: executionError}
	}
	return nil
}

// DeleteBranch removes a local branch. When forceDelete is true the deletion is forced.
func (manager *RepositoryManager) DeleteBranch(executionContext context.Context, repositoryPath string, branchName string, forceDelete bool) error {
	trimmedPath := strings.TrimSpace(repositoryPath)
	if len(trimmedPath) == 0 {
		return InvalidRepositoryInputError{FieldName: repositoryPathFieldNameConstant, Message: requiredValueMessageConstant}
	}

	trimmedBranch := strings.TrimSpace(branchName)
	if len(trimmedBranch) == 0 {
		return InvalidRepositoryInputError{FieldName: branchNameFieldNameConstant, Message: requiredValueMessageConstant}
	}

	commandArguments := []string{gitBranchSubcommandConstant, gitDeleteFlagConstant}
	if forceDelete {
		commandArguments = append(commandArguments, gitForceFlagConstant)
	}
	commandArguments = append(commandArguments, trimmedBranch)

	commandDetails := execshell.CommandDetails{
		Arguments:        commandArguments,
		WorkingDirectory: trimmedPath,
	}

	_, executionError := manager.executor.ExecuteGit(executionContext, commandDetails)
	if executionError != nil {
		return RepositoryOperationError{Operation: deleteBranchOperationNameConstant, Cause: executionError}
	}
	return nil
}

// GetCurrentBranch resolves the current branch name.
func (manager *RepositoryManager) GetCurrentBranch(executionContext context.Context, repositoryPath string) (string, error) {
	trimmedPath := strings.TrimSpace(repositoryPath)
	if len(trimmedPath) == 0 {
		return "", InvalidRepositoryInputError{FieldName: repositoryPathFieldNameConstant, Message: requiredValueMessageConstant}
	}

	commandDetails := execshell.CommandDetails{
		Arguments:        []string{gitRevParseSubcommandConstant, gitAbbrevRefFlagConstant, gitHeadReferenceConstant},
		WorkingDirectory: trimmedPath,
	}

	executionResult, executionError := manager.executor.ExecuteGit(executionContext, commandDetails)
	if executionError != nil {
		return "", RepositoryOperationError{Operation: currentBranchOperationNameConstant, Cause: executionError}
	}

	return strings.TrimSpace(executionResult.StandardOutput), nil
}

// GetRemoteURL returns the configured remote URL for the given remote name.
func (manager *RepositoryManager) GetRemoteURL(executionContext context.Context, repositoryPath string, remoteName string) (string, error) {
	trimmedPath := strings.TrimSpace(repositoryPath)
	if len(trimmedPath) == 0 {
		return "", InvalidRepositoryInputError{FieldName: repositoryPathFieldNameConstant, Message: requiredValueMessageConstant}
	}

	trimmedRemote := strings.TrimSpace(remoteName)
	if len(trimmedRemote) == 0 {
		return "", InvalidRepositoryInputError{FieldName: remoteNameFieldNameConstant, Message: requiredValueMessageConstant}
	}

	commandDetails := execshell.CommandDetails{
		Arguments:        []string{gitRemoteSubcommandConstant, gitRemoteGetURLSubcommandConstant, trimmedRemote},
		WorkingDirectory: trimmedPath,
	}

	executionResult, executionError := manager.executor.ExecuteGit(executionContext, commandDetails)
	if executionError != nil {
		return "", RepositoryOperationError{Operation: getRemoteURLOperationNameConstant, Cause: executionError}
	}

	return strings.TrimSpace(executionResult.StandardOutput), nil
}

// SetRemoteURL sets the remote URL for a remote.
func (manager *RepositoryManager) SetRemoteURL(executionContext context.Context, repositoryPath string, remoteName string, remoteURL string) error {
	trimmedPath := strings.TrimSpace(repositoryPath)
	if len(trimmedPath) == 0 {
		return InvalidRepositoryInputError{FieldName: repositoryPathFieldNameConstant, Message: requiredValueMessageConstant}
	}

	trimmedRemote := strings.TrimSpace(remoteName)
	if len(trimmedRemote) == 0 {
		return InvalidRepositoryInputError{FieldName: remoteNameFieldNameConstant, Message: requiredValueMessageConstant}
	}

	trimmedRemoteURL := strings.TrimSpace(remoteURL)
	if len(trimmedRemoteURL) == 0 {
		return InvalidRepositoryInputError{FieldName: remoteURLFieldNameConstant, Message: requiredValueMessageConstant}
	}

	commandDetails := execshell.CommandDetails{
		Arguments:        []string{gitRemoteSubcommandConstant, gitRemoteSetURLSubcommandConstant, trimmedRemote, trimmedRemoteURL},
		WorkingDirectory: trimmedPath,
	}

	_, executionError := manager.executor.ExecuteGit(executionContext, commandDetails)
	if executionError != nil {
		return RepositoryOperationError{Operation: setRemoteURLOperationNameConstant, Cause: executionError}
	}
	return nil
}
