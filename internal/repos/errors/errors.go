package errors

import (
	stdErrors "errors"
	"fmt"
)

// Operation identifies the logical operation producing a contextual error.
type Operation string

const (
	// OperationProtocolConvert denotes remote protocol conversion executors.
	OperationProtocolConvert Operation = "repo.protocol.convert"
	// OperationCanonicalRemote denotes canonical remote update executors.
	OperationCanonicalRemote Operation = "repo.remote.update"
	// OperationRenameDirectories denotes repository directory rename executors.
	OperationRenameDirectories Operation = "repo.folder.rename"
	// OperationHistoryPurge denotes history rewrite executors.
	OperationHistoryPurge Operation = "repo.history.purge"
	// OperationNamespaceRewrite denotes namespace rewrite executors.
	OperationNamespaceRewrite Operation = "repo.namespace.rewrite"
)

// Sentinel describes a stable error code shared across executors.
type Sentinel string

// Error returns the sentinel code string.
func (sentinel Sentinel) Error() string {
	return string(sentinel)
}

// Code exposes the sentinel code string.
func (sentinel Sentinel) Code() string {
	return string(sentinel)
}

// OperationError annotates an error produced by a repository executor with operation metadata.
type OperationError struct {
	operation Operation
	subject   string
	err       error
	message   string
}

// Error implements the error interface.
func (operationError OperationError) Error() string {
	if len(operationError.message) > 0 {
		if len(operationError.subject) == 0 {
			return fmt.Sprintf("%s: %s", operationError.operation, operationError.message)
		}
		return fmt.Sprintf("%s[%s]: %s", operationError.operation, operationError.subject, operationError.message)
	}
	if len(operationError.subject) == 0 {
		return fmt.Sprintf("%s: %v", operationError.operation, operationError.err)
	}
	return fmt.Sprintf("%s[%s]: %v", operationError.operation, operationError.subject, operationError.err)
}

// Unwrap exposes the underlying error chain.
func (operationError OperationError) Unwrap() error {
	return operationError.err
}

// Operation returns the originating operation identifier.
func (operationError OperationError) Operation() Operation {
	return operationError.operation
}

// Subject returns the domain subject (typically repository path) related to the error.
func (operationError OperationError) Subject() string {
	return operationError.subject
}

// Code surfaces the sentinel code of the wrapped error when present.
func (operationError OperationError) Code() string {
	if coder, found := findSentinel(operationError.err); found {
		return coder.Code()
	}
	return ""
}

// Message exposes the formatted message when provided via WrapMessage.
func (operationError OperationError) Message() string {
	return operationError.message
}

// Wrap constructs an OperationError combining the provided metadata with the base sentinel.
func Wrap(operation Operation, subject string, sentinel Sentinel, detail error) error {
	if len(sentinel) == 0 {
		return OperationError{operation: operation, subject: subject, err: detail}
	}
	baseError := error(sentinel)
	if detail != nil {
		baseError = fmt.Errorf("%w: %v", sentinel, detail)
	}
	return OperationError{operation: operation, subject: subject, err: baseError}
}

// WrapMessage constructs an OperationError combining the provided metadata with a formatted message.
func WrapMessage(operation Operation, subject string, sentinel Sentinel, message string) error {
	if len(message) == 0 {
		return Wrap(operation, subject, sentinel, nil)
	}
	return OperationError{operation: operation, subject: subject, err: fmt.Errorf("%w: %s", sentinel, message), message: message}
}

func findSentinel(err error) (Sentinel, bool) {
	if err == nil {
		return "", false
	}
	var sentinel Sentinel
	if stdErrors.As(err, &sentinel) {
		return sentinel, true
	}
	return "", false
}

var (
	// ErrGitManagerUnavailable indicates a missing Git repository manager dependency.
	ErrGitManagerUnavailable Sentinel = "git_manager_unavailable"
	// ErrOriginOwnerMissing indicates the current origin owner/repository tuple was unavailable.
	ErrOriginOwnerMissing Sentinel = "origin_owner_missing"
	// ErrCanonicalOwnerMissing indicates canonical owner/repository metadata was unavailable.
	ErrCanonicalOwnerMissing Sentinel = "canonical_owner_missing"
	// ErrUnknownProtocol indicates a remote protocol that cannot be converted.
	ErrUnknownProtocol Sentinel = "unknown_protocol"
	// ErrRemoteURLBuildFailed indicates a failure building the target remote URL.
	ErrRemoteURLBuildFailed Sentinel = "remote_url_build_failed"
	// ErrUserConfirmationFailed indicates an error occurred while prompting the user.
	ErrUserConfirmationFailed Sentinel = "user_confirmation_failed"
	// ErrRemoteUpdateFailed indicates failure updating a remote URL.
	ErrRemoteUpdateFailed Sentinel = "remote_update_failed"
	// ErrRemoteEnumerationFailed indicates failure enumerating repository remotes.
	ErrRemoteEnumerationFailed Sentinel = "remote_enumeration_failed"
	// ErrFetchFailed indicates a git fetch step failed.
	ErrFetchFailed Sentinel = "fetch_failed"
	// ErrPullFailed indicates a git pull step failed.
	ErrPullFailed Sentinel = "pull_failed"
	// ErrSwitchFailed indicates a branch switch failed.
	ErrSwitchFailed Sentinel = "branch_switch_failed"
	// ErrCreateBranchFailed indicates branch creation failed.
	ErrCreateBranchFailed Sentinel = "branch_create_failed"
	// ErrFilesystemUnavailable indicates a missing filesystem dependency.
	ErrFilesystemUnavailable Sentinel = "filesystem_unavailable"
	// ErrParentMissing indicates a required parent directory was absent.
	ErrParentMissing Sentinel = "parent_missing"
	// ErrParentNotDirectory indicates a parent path was not a directory.
	ErrParentNotDirectory Sentinel = "parent_not_directory"
	// ErrTargetExists indicates rename target already existed.
	ErrTargetExists Sentinel = "target_exists"
	// ErrDirtyWorktree indicates a dirty worktree prevented progress.
	ErrDirtyWorktree Sentinel = "dirty_worktree"
	// ErrPathsRequired indicates history purge executed without any paths.
	ErrPathsRequired Sentinel = "paths_required"
	// ErrExecutorDependenciesMissing indicates required executor dependencies were unavailable.
	ErrExecutorDependenciesMissing Sentinel = "executor_dependencies_missing"
	// ErrRenameFailed indicates that the filesystem rename operation failed.
	ErrRenameFailed Sentinel = "rename_failed"
	// ErrParentCreationFailed indicates failure creating parent directories before rename.
	ErrParentCreationFailed Sentinel = "parent_creation_failed"
	// ErrHistoryPrerequisiteFailed indicates history purge prerequisites were not satisfied.
	ErrHistoryPrerequisiteFailed Sentinel = "history_prerequisite_failed"
	// ErrHistoryRewriteFailed indicates history rewrite commands failed.
	ErrHistoryRewriteFailed Sentinel = "history_rewrite_failed"
	// ErrHistoryPushFailed indicates forced pushes failed during history purge.
	ErrHistoryPushFailed Sentinel = "history_push_failed"
	// ErrHistoryRestoreFailed indicates restoring upstream branches failed during history purge.
	ErrHistoryRestoreFailed Sentinel = "history_restore_failed"
	// ErrHistoryGitIgnoreUpdateFailed indicates `.gitignore` updates failed during history purge.
	ErrHistoryGitIgnoreUpdateFailed Sentinel = "history_gitignore_update_failed"
	// ErrHistoryInspectionFailed indicates repository history inspection failed prior to rewrite.
	ErrHistoryInspectionFailed Sentinel = "history_inspection_failed"
	// ErrGitRepositoryMissing indicates required git metadata was not found.
	ErrGitRepositoryMissing Sentinel = "git_repository_missing"
	// ErrNamespaceRewriteFailed indicates namespace rewrite failed.
	ErrNamespaceRewriteFailed Sentinel = "namespace_rewrite_failed"
)
