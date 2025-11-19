package rename

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	repoerrors "github.com/tyemirov/gix/internal/repos/errors"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	planSkipAlreadyMessage            = "PLAN-SKIP (already normalized): %s"
	planSkipDirtyMessage              = "PLAN-SKIP (dirty worktree): %s"
	planSkipParentMissingMessage      = "PLAN-SKIP (target parent missing): %s"
	planSkipParentNotDirectoryMessage = "PLAN-SKIP (target parent not directory): %s"
	planSkipExistsMessage             = "PLAN-SKIP (target exists): %s"
	planCaseOnlyMessage               = "PLAN-CASE-ONLY: %s → %s (two-step move required)"
	planReadyMessage                  = "PLAN-OK: %s → %s"
	errorParentMissingMessage         = "ERROR: target parent missing: %s"
	errorParentNotDirectoryMessage    = "ERROR: target parent is not a directory: %s"
	errorTargetExistsMessage          = "ERROR: target exists: %s"
	promptTemplate                    = "Rename '%s' → '%s'? [a/N/y] "
	skipMessage                       = "SKIP: %s"
	skipDirtyMessage                  = "SKIP (dirty worktree): %s"
	skipAlreadyNormalizedMessage      = "SKIP (already normalized): %s"
	successMessage                    = "Renamed %s → %s"
	failureMessage                    = "ERROR: rename failed for %s → %s"
	intermediateRenameTemplate        = "%s.rename.%d"
	parentDirectoryPermissionConstant = fs.FileMode(0o755)
)

// Dependencies supplies collaborators required to evaluate rename operations.
type Dependencies struct {
	FileSystem shared.FileSystem
	GitManager shared.GitRepositoryManager
	Prompter   shared.ConfirmationPrompter
	Clock      shared.Clock
	Reporter   shared.Reporter
}

// Executor orchestrates rename planning and execution for repositories.
type Executor struct {
	dependencies Dependencies
}

// NewExecutor constructs an Executor from the provided dependencies.
func NewExecutor(dependencies Dependencies) *Executor {
	if dependencies.Clock == nil {
		dependencies.Clock = shared.SystemClock{}
	}
	return &Executor{dependencies: dependencies}
}

// Execute performs the rename workflow using the executor's dependencies.
func (executor *Executor) Execute(executionContext context.Context, options Options) error {
	desiredName := options.desiredFolderName
	if len(desiredName) == 0 {
		return nil
	}

	repositoryPath := options.repositoryPath.String()

	if executor.dependencies.FileSystem == nil {
		return repoerrors.Wrap(
			repoerrors.OperationRenameDirectories,
			repositoryPath,
			repoerrors.ErrFilesystemUnavailable,
			nil,
		)
	}

	oldAbsolutePath, absError := executor.dependencies.FileSystem.Abs(repositoryPath)
	if absError != nil {
		return repoerrors.Wrap(
			repoerrors.OperationRenameDirectories,
			repositoryPath,
			repoerrors.ErrFilesystemUnavailable,
			absError,
		)
	}

	parentDirectory := filepath.Dir(oldAbsolutePath)
	newAbsolutePath := filepath.Join(parentDirectory, desiredName)

	skip, prerequisiteError := executor.evaluatePrerequisites(executionContext, oldAbsolutePath, newAbsolutePath, options.cleanPolicy.RequireClean(), options.ensureParentDirectories)
	if prerequisiteError != nil {
		return prerequisiteError
	}
	if skip {
		return nil
	}

	if options.confirmationPolicy.ShouldPrompt() && executor.dependencies.Prompter != nil {
		prompt := fmt.Sprintf(promptTemplate, oldAbsolutePath, newAbsolutePath)
		confirmationResult, promptError := executor.dependencies.Prompter.Confirm(prompt)
		if promptError != nil {
			executor.reportOutput(failureMessage, oldAbsolutePath, newAbsolutePath)
			return repoerrors.Wrap(
				repoerrors.OperationRenameDirectories,
				oldAbsolutePath,
				repoerrors.ErrUserConfirmationFailed,
				promptError,
			)
		}
		if !confirmationResult.Confirmed {
			executor.reportOutput(skipMessage, oldAbsolutePath)
			return nil
		}
	}

	if ensureError := executor.ensureParentDirectory(newAbsolutePath, options.ensureParentDirectories); ensureError != nil {
		return ensureError
	}

	if renameError := executor.performRename(oldAbsolutePath, newAbsolutePath); renameError != nil {
		return renameError
	}

	executor.reportOutput(successMessage, oldAbsolutePath, newAbsolutePath)
	return nil
}

// Execute performs the rename workflow using transient executor state.
func Execute(executionContext context.Context, dependencies Dependencies, options Options) error {
	return NewExecutor(dependencies).Execute(executionContext, options)
}

func (executor *Executor) evaluatePrerequisites(executionContext context.Context, oldAbsolutePath string, newAbsolutePath string, requireClean bool, ensureParentDirectories bool) (bool, error) {
	caseOnlyRename := isCaseOnlyRename(oldAbsolutePath, newAbsolutePath)
	parentDetails := executor.parentDirectoryDetails(newAbsolutePath)

	if oldAbsolutePath == newAbsolutePath {
		executor.reportOutput(skipAlreadyNormalizedMessage, oldAbsolutePath)
		return true, nil
	}

	if requireClean {
		dirtyEntries, dirtyError := executor.worktreeStatus(executionContext, oldAbsolutePath)
		if dirtyError != nil {
			executor.reportOutput(skipDirtyMessage, newDirtyWorktreeArgument(oldAbsolutePath, []string{dirtyError.Error()}))
			return true, nil
		}
		if len(dirtyEntries) > 0 {
			executor.reportOutput(skipDirtyMessage, newDirtyWorktreeArgument(oldAbsolutePath, dirtyEntries))
			return true, nil
		}
	}

	if parentDetails.exists && !parentDetails.isDirectory {
		executor.reportOutput(errorParentNotDirectoryMessage, parentDetails.path)
		return true, repoerrors.WrapMessage(
			repoerrors.OperationRenameDirectories,
			parentDetails.path,
			repoerrors.ErrParentNotDirectory,
			fmt.Sprintf(errorParentNotDirectoryMessage, parentDetails.path),
		)
	}

	if !ensureParentDirectories && !parentDetails.exists {
		executor.reportOutput(errorParentMissingMessage, parentDetails.path)
		return true, repoerrors.WrapMessage(
			repoerrors.OperationRenameDirectories,
			parentDetails.path,
			repoerrors.ErrParentMissing,
			fmt.Sprintf(errorParentMissingMessage, parentDetails.path),
		)
	}

	if executor.targetExists(newAbsolutePath) && !caseOnlyRename {
		executor.reportOutput(errorTargetExistsMessage, newAbsolutePath)
		return true, repoerrors.WrapMessage(
			repoerrors.OperationRenameDirectories,
			newAbsolutePath,
			repoerrors.ErrTargetExists,
			fmt.Sprintf(errorTargetExistsMessage, newAbsolutePath),
		)
	}

	return false, nil
}

func (executor *Executor) worktreeStatus(executionContext context.Context, repositoryPath string) ([]string, error) {
	if executor.dependencies.GitManager == nil {
		return nil, repoerrors.Wrap(
			repoerrors.OperationRenameDirectories,
			repositoryPath,
			repoerrors.ErrGitManagerUnavailable,
			nil,
		)
	}

	return executor.dependencies.GitManager.WorktreeStatus(executionContext, repositoryPath)
}

func (executor *Executor) parentDirectoryDetails(path string) parentDirectoryInformation {
	parentPath := filepath.Dir(path)
	details := parentDirectoryInformation{path: parentPath}

	if executor.dependencies.FileSystem == nil {
		return details
	}

	info, statError := executor.dependencies.FileSystem.Stat(parentPath)
	if statError != nil {
		return details
	}

	details.exists = true
	details.isDirectory = info.IsDir()
	return details
}

func (executor *Executor) targetExists(path string) bool {
	if executor.dependencies.FileSystem == nil {
		return false
	}
	_, statError := executor.dependencies.FileSystem.Stat(path)
	return statError == nil
}

func (executor *Executor) ensureParentDirectory(newAbsolutePath string, ensureParentDirectories bool) error {
	if !ensureParentDirectories {
		return nil
	}

	parentDetails := executor.parentDirectoryDetails(newAbsolutePath)
	if parentDetails.exists {
		if parentDetails.isDirectory {
			return nil
		}
		executor.reportOutput(errorParentNotDirectoryMessage, parentDetails.path)
		return repoerrors.WrapMessage(
			repoerrors.OperationRenameDirectories,
			parentDetails.path,
			repoerrors.ErrParentNotDirectory,
			fmt.Sprintf(errorParentNotDirectoryMessage, parentDetails.path),
		)
	}

	if executor.dependencies.FileSystem == nil {
		executor.reportOutput(errorParentMissingMessage, parentDetails.path)
		return repoerrors.WrapMessage(
			repoerrors.OperationRenameDirectories,
			parentDetails.path,
			repoerrors.ErrFilesystemUnavailable,
			fmt.Sprintf(errorParentMissingMessage, parentDetails.path),
		)
	}

	if creationError := executor.dependencies.FileSystem.MkdirAll(parentDetails.path, parentDirectoryPermissionConstant); creationError != nil {
		executor.reportOutput(errorParentMissingMessage, parentDetails.path)
		return repoerrors.WrapMessage(
			repoerrors.OperationRenameDirectories,
			parentDetails.path,
			repoerrors.ErrParentCreationFailed,
			fmt.Sprintf(errorParentMissingMessage, parentDetails.path),
		)
	}

	return nil
}

func (executor *Executor) performRename(oldAbsolutePath string, newAbsolutePath string) error {
	if executor.dependencies.FileSystem == nil {
		return repoerrors.WrapMessage(
			repoerrors.OperationRenameDirectories,
			oldAbsolutePath,
			repoerrors.ErrFilesystemUnavailable,
			fmt.Sprintf(failureMessage, oldAbsolutePath, newAbsolutePath),
		)
	}

	if executor.dependencies.Clock == nil {
		executor.dependencies.Clock = shared.SystemClock{}
	}

	if executor.dependencies.GitManager == nil {
		return repoerrors.WrapMessage(
			repoerrors.OperationRenameDirectories,
			oldAbsolutePath,
			repoerrors.ErrGitManagerUnavailable,
			fmt.Sprintf(failureMessage, oldAbsolutePath, newAbsolutePath),
		)
	}

	if renameError := executor.dependencies.FileSystem.Rename(oldAbsolutePath, newAbsolutePath); renameError == nil {
		return nil
	}

	var lastError error
	for attempt := 0; attempt < 5; attempt++ {
		intermediate := fmt.Sprintf(intermediateRenameTemplate, oldAbsolutePath, attempt)
		if attemptError := executor.dependencies.FileSystem.Rename(oldAbsolutePath, intermediate); attemptError != nil {
			lastError = attemptError
			continue
		}
		if attemptError := executor.dependencies.FileSystem.Rename(intermediate, newAbsolutePath); attemptError == nil {
			return nil
		} else {
			lastError = attemptError
		}
	}

	message := fmt.Sprintf(failureMessage, oldAbsolutePath, newAbsolutePath)
	if lastError != nil {
		trimmed := strings.TrimSuffix(message, "\n")
		message = fmt.Sprintf("%s: %v\n", trimmed, lastError)
	}
	executor.reportOutput("%s", message)

	return repoerrors.WrapMessage(
		repoerrors.OperationRenameDirectories,
		oldAbsolutePath,
		repoerrors.ErrRenameFailed,
		message,
	)
}

func (executor *Executor) reportOutput(format string, arguments ...any) {
	if executor.dependencies.Reporter == nil {
		return
	}
	message := fmt.Sprintf(format, arguments...)
	trimmed := strings.TrimSpace(message)

	event := shared.Event{
		Level:   shared.EventLevelInfo,
		Code:    shared.EventCodeFolderPlan,
		Message: trimmed,
		Details: map[string]string{},
	}

	switch format {
	case successMessage:
		oldPath := fmt.Sprintf("%v", arguments[0])
		newPath := fmt.Sprintf("%v", arguments[1])
		event.Code = shared.EventCodeFolderRename
		event.Message = fmt.Sprintf("%s → %s", oldPath, newPath)
		event.RepositoryPath = oldPath
		event.Details["old_path"] = oldPath
		event.Details["new_path"] = newPath
	case failureMessage:
		oldPath := fmt.Sprintf("%v", arguments[0])
		newPath := fmt.Sprintf("%v", arguments[1])
		event.Code = shared.EventCodeFolderError
		event.Level = shared.EventLevelError
		event.Message = trimmed
		event.RepositoryPath = oldPath
		event.Details["old_path"] = oldPath
		event.Details["new_path"] = newPath
		event.Details["reason"] = "rename_failed"
	case skipMessage:
		repositoryPath := fmt.Sprintf("%v", arguments[0])
		event.Code = shared.EventCodeFolderSkip
		event.Message = fmt.Sprintf("skipped %s", repositoryPath)
		event.RepositoryPath = repositoryPath
		event.Details["reason"] = "user_declined"
	case skipDirtyMessage:
		repositoryPath := fmt.Sprintf("%v", arguments[0])
		event.Code = shared.EventCodeFolderSkip
		dirtyEntries := normalizeDirtyEntries(arguments)
		event.Message = buildDirtyWorktreeMessage("skipped due to dirty worktree", dirtyEntries)
		event.RepositoryPath = repositoryPath
		event.Details["reason"] = "dirty_worktree"
		if len(dirtyEntries) > 0 {
			event.Details["dirty_entries"] = strings.Join(dirtyEntries, "; ")
		}
	case skipAlreadyNormalizedMessage:
		repositoryPath := fmt.Sprintf("%v", arguments[0])
		event.Code = shared.EventCodeFolderSkip
		event.Message = "already normalized"
		event.RepositoryPath = repositoryPath
		event.Details["reason"] = "already_normalized"
	case planSkipAlreadyMessage:
		repositoryPath := fmt.Sprintf("%v", arguments[0])
		event.Code = shared.EventCodeFolderPlan
		event.Message = "already normalized"
		event.RepositoryPath = repositoryPath
		event.Details["reason"] = "already_normalized"
	case planSkipDirtyMessage:
		repositoryPath := fmt.Sprintf("%v", arguments[0])
		event.Code = shared.EventCodeFolderPlan
		dirtyEntries := normalizeDirtyEntries(arguments)
		event.Message = buildDirtyWorktreeMessage("skip: dirty worktree", dirtyEntries)
		event.RepositoryPath = repositoryPath
		event.Details["reason"] = "dirty_worktree"
		if len(dirtyEntries) > 0 {
			event.Details["dirty_entries"] = strings.Join(dirtyEntries, "; ")
		}
	case planSkipParentMissingMessage:
		parentPath := fmt.Sprintf("%v", arguments[0])
		event.Code = shared.EventCodeFolderPlan
		event.Message = "skip: parent missing"
		event.RepositoryPath = parentPath
		event.Details["reason"] = "parent_missing"
	case planSkipParentNotDirectoryMessage:
		parentPath := fmt.Sprintf("%v", arguments[0])
		event.Code = shared.EventCodeFolderPlan
		event.Message = "skip: parent not directory"
		event.RepositoryPath = parentPath
		event.Details["reason"] = "parent_not_directory"
	case planSkipExistsMessage:
		targetPath := fmt.Sprintf("%v", arguments[0])
		event.Code = shared.EventCodeFolderPlan
		event.Message = "skip: target exists"
		event.RepositoryPath = targetPath
		event.Details["reason"] = "target_exists"
	case errorParentNotDirectoryMessage:
		parentPath := fmt.Sprintf("%v", arguments[0])
		event.Code = shared.EventCodeFolderError
		event.Level = shared.EventLevelError
		event.Message = "parent is not a directory"
		event.RepositoryPath = parentPath
		event.Details["reason"] = "parent_not_directory"
	case errorParentMissingMessage:
		parentPath := fmt.Sprintf("%v", arguments[0])
		event.Code = shared.EventCodeFolderError
		event.Level = shared.EventLevelError
		event.Message = "parent missing"
		event.RepositoryPath = parentPath
		event.Details["reason"] = "parent_missing"
	case errorTargetExistsMessage:
		targetPath := fmt.Sprintf("%v", arguments[0])
		event.Code = shared.EventCodeFolderError
		event.Level = shared.EventLevelError
		event.Message = "target exists"
		event.RepositoryPath = targetPath
		event.Details["reason"] = "target_exists"
	default:
		// Formats such as "%s" carry already formatted messages.
		switch {
		case strings.HasPrefix(trimmed, "ERROR:"):
			event.Code = shared.EventCodeFolderError
			event.Level = shared.EventLevelError
		case strings.HasPrefix(trimmed, "SKIP"):
			event.Code = shared.EventCodeFolderSkip
		case strings.HasPrefix(trimmed, "Renamed"):
			event.Code = shared.EventCodeFolderRename
		default:
			event.Code = shared.EventCodeFolderPlan
		}
	}

	executor.dependencies.Reporter.Report(event)
}

func isCaseOnlyRename(oldPath string, newPath string) bool {
	return strings.EqualFold(oldPath, newPath) && oldPath != newPath
}

type dirtyEntriesProvider interface {
	DirtyEntries() []string
}

type dirtyWorktreeArgument struct {
	repositoryPath string
	entries        []string
}

func newDirtyWorktreeArgument(repositoryPath string, entries []string) dirtyWorktreeArgument {
	duplicate := make([]string, len(entries))
	copy(duplicate, entries)
	return dirtyWorktreeArgument{repositoryPath: repositoryPath, entries: duplicate}
}

func (argument dirtyWorktreeArgument) String() string {
	return argument.repositoryPath
}

func (argument dirtyWorktreeArgument) DirtyEntries() []string {
	duplicate := make([]string, len(argument.entries))
	copy(duplicate, argument.entries)
	return duplicate
}

func normalizeDirtyEntries(arguments []any) []string {
	var rawEntries []string
	for _, argument := range arguments {
		if provider, ok := argument.(dirtyEntriesProvider); ok {
			rawEntries = provider.DirtyEntries()
			break
		}
	}

	if rawEntries == nil && len(arguments) > 1 {
		if entries, ok := arguments[1].([]string); ok {
			rawEntries = append([]string(nil), entries...)
		}
	}

	if rawEntries == nil {
		return nil
	}

	normalized := make([]string, 0, len(rawEntries))
	for _, entry := range rawEntries {
		trimmed := strings.TrimSpace(entry)
		if len(trimmed) == 0 {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func buildDirtyWorktreeMessage(prefix string, entries []string) string {
	if len(entries) == 0 {
		return prefix
	}
	return fmt.Sprintf("%s (%s)", prefix, strings.Join(entries, "; "))
}

// parentDirectoryInformation describes the state of a parent directory for rename planning.
type parentDirectoryInformation struct {
	path        string
	exists      bool
	isDirectory bool
}
