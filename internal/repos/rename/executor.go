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
	planSkipAlreadyMessage            = "PLAN-SKIP (already normalized): %s\n"
	planSkipDirtyMessage              = "PLAN-SKIP (dirty worktree): %s\n"
	planSkipParentMissingMessage      = "PLAN-SKIP (target parent missing): %s\n"
	planSkipParentNotDirectoryMessage = "PLAN-SKIP (target parent not directory): %s\n"
	planSkipExistsMessage             = "PLAN-SKIP (target exists): %s\n"
	planCaseOnlyMessage               = "PLAN-CASE-ONLY: %s → %s (two-step move required)\n"
	planReadyMessage                  = "PLAN-OK: %s → %s\n"
	errorParentMissingMessage         = "ERROR: target parent missing: %s\n"
	errorParentNotDirectoryMessage    = "ERROR: target parent is not a directory: %s\n"
	errorTargetExistsMessage          = "ERROR: target exists: %s\n"
	promptTemplate                    = "Rename '%s' → '%s'? [a/N/y] "
	skipMessage                       = "SKIP: %s\n"
	skipDirtyMessage                  = "SKIP (dirty worktree): %s\n"
	skipAlreadyNormalizedMessage      = "SKIP (already normalized): %s\n"
	successMessage                    = "Renamed %s → %s\n"
	failureMessage                    = "ERROR: rename failed for %s → %s\n"
	intermediateRenameTemplate        = "%s.rename.%d"
	parentDirectoryPermissionConstant = fs.FileMode(0o755)
)

// Options configures a rename execution.
type Options struct {
	RepositoryPath          shared.RepositoryPath
	DesiredFolderName       string
	DryRun                  bool
	CleanPolicy             shared.CleanWorktreePolicy
	ConfirmationPolicy      shared.ConfirmationPolicy
	IncludeOwner            bool
	EnsureParentDirectories bool
}

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
	desiredName := strings.TrimSpace(options.DesiredFolderName)
	if len(desiredName) == 0 {
		return nil
	}

	repositoryPath := options.RepositoryPath.String()

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

	if options.DryRun {
		executor.printPlan(executionContext, oldAbsolutePath, newAbsolutePath, options.CleanPolicy.RequireClean(), options.EnsureParentDirectories)
		return nil
	}

	skip, prerequisiteError := executor.evaluatePrerequisites(executionContext, oldAbsolutePath, newAbsolutePath, options.CleanPolicy.RequireClean(), options.EnsureParentDirectories)
	if prerequisiteError != nil {
		return prerequisiteError
	}
	if skip {
		return nil
	}

	if options.ConfirmationPolicy.ShouldPrompt() && executor.dependencies.Prompter != nil {
		prompt := fmt.Sprintf(promptTemplate, oldAbsolutePath, newAbsolutePath)
		confirmationResult, promptError := executor.dependencies.Prompter.Confirm(prompt)
		if promptError != nil {
			executor.printfOutput(failureMessage, oldAbsolutePath, newAbsolutePath)
			return repoerrors.Wrap(
				repoerrors.OperationRenameDirectories,
				oldAbsolutePath,
				repoerrors.ErrUserConfirmationFailed,
				promptError,
			)
		}
		if !confirmationResult.Confirmed {
			executor.printfOutput(skipMessage, oldAbsolutePath)
			return nil
		}
	}

	if ensureError := executor.ensureParentDirectory(newAbsolutePath, options.EnsureParentDirectories); ensureError != nil {
		return ensureError
	}

	if renameError := executor.performRename(oldAbsolutePath, newAbsolutePath); renameError != nil {
		return renameError
	}

	executor.printfOutput(successMessage, oldAbsolutePath, newAbsolutePath)
	return nil
}

// Execute performs the rename workflow using transient executor state.
func Execute(executionContext context.Context, dependencies Dependencies, options Options) error {
	return NewExecutor(dependencies).Execute(executionContext, options)
}

func (executor *Executor) printPlan(executionContext context.Context, oldAbsolutePath string, newAbsolutePath string, requireClean bool, ensureParentDirectories bool) {
	caseOnlyRename := isCaseOnlyRename(oldAbsolutePath, newAbsolutePath)
	parentDetails := executor.parentDirectoryDetails(newAbsolutePath)

	switch {
	case oldAbsolutePath == newAbsolutePath:
		executor.printfOutput(planSkipAlreadyMessage, oldAbsolutePath)
		return
	case requireClean && !executor.isClean(executionContext, oldAbsolutePath):
		executor.printfOutput(planSkipDirtyMessage, oldAbsolutePath)
		return
	case parentDetails.exists && !parentDetails.isDirectory:
		executor.printfOutput(planSkipParentNotDirectoryMessage, parentDetails.path)
		return
	case !ensureParentDirectories && !parentDetails.exists:
		executor.printfOutput(planSkipParentMissingMessage, parentDetails.path)
		return
	case executor.targetExists(newAbsolutePath) && !caseOnlyRename:
		executor.printfOutput(planSkipExistsMessage, newAbsolutePath)
		return
	}

	if caseOnlyRename {
		executor.printfOutput(planCaseOnlyMessage, oldAbsolutePath, newAbsolutePath)
		return
	}

	executor.printfOutput(planReadyMessage, oldAbsolutePath, newAbsolutePath)
}

func (executor *Executor) evaluatePrerequisites(executionContext context.Context, oldAbsolutePath string, newAbsolutePath string, requireClean bool, ensureParentDirectories bool) (bool, error) {
	caseOnlyRename := isCaseOnlyRename(oldAbsolutePath, newAbsolutePath)
	parentDetails := executor.parentDirectoryDetails(newAbsolutePath)

	if oldAbsolutePath == newAbsolutePath {
		executor.printfOutput(skipAlreadyNormalizedMessage, oldAbsolutePath)
		return true, nil
	}

	if requireClean && !executor.isClean(executionContext, oldAbsolutePath) {
		executor.printfOutput(skipDirtyMessage, oldAbsolutePath)
		return true, nil
	}

	if parentDetails.exists && !parentDetails.isDirectory {
		executor.printfOutput(errorParentNotDirectoryMessage, parentDetails.path)
		return true, repoerrors.WrapMessage(
			repoerrors.OperationRenameDirectories,
			parentDetails.path,
			repoerrors.ErrParentNotDirectory,
			fmt.Sprintf(errorParentNotDirectoryMessage, parentDetails.path),
		)
	}

	if !ensureParentDirectories && !parentDetails.exists {
		executor.printfOutput(errorParentMissingMessage, parentDetails.path)
		return true, repoerrors.WrapMessage(
			repoerrors.OperationRenameDirectories,
			parentDetails.path,
			repoerrors.ErrParentMissing,
			fmt.Sprintf(errorParentMissingMessage, parentDetails.path),
		)
	}

	if executor.targetExists(newAbsolutePath) && !caseOnlyRename {
		executor.printfOutput(errorTargetExistsMessage, newAbsolutePath)
		return true, repoerrors.WrapMessage(
			repoerrors.OperationRenameDirectories,
			newAbsolutePath,
			repoerrors.ErrTargetExists,
			fmt.Sprintf(errorTargetExistsMessage, newAbsolutePath),
		)
	}

	return false, nil
}

func (executor *Executor) isClean(executionContext context.Context, repositoryPath string) bool {
	if executor.dependencies.GitManager == nil {
		return false
	}

	clean, cleanError := executor.dependencies.GitManager.CheckCleanWorktree(executionContext, repositoryPath)
	if cleanError != nil {
		return false
	}
	return clean
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
		executor.printfOutput(errorParentNotDirectoryMessage, parentDetails.path)
		return repoerrors.WrapMessage(
			repoerrors.OperationRenameDirectories,
			parentDetails.path,
			repoerrors.ErrParentNotDirectory,
			fmt.Sprintf(errorParentNotDirectoryMessage, parentDetails.path),
		)
	}

	if executor.dependencies.FileSystem == nil {
		executor.printfOutput(errorParentMissingMessage, parentDetails.path)
		return repoerrors.WrapMessage(
			repoerrors.OperationRenameDirectories,
			parentDetails.path,
			repoerrors.ErrFilesystemUnavailable,
			fmt.Sprintf(errorParentMissingMessage, parentDetails.path),
		)
	}

	if creationError := executor.dependencies.FileSystem.MkdirAll(parentDetails.path, parentDirectoryPermissionConstant); creationError != nil {
		executor.printfOutput(errorParentMissingMessage, parentDetails.path)
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
	executor.printfOutput("%s", message)

	return repoerrors.WrapMessage(
		repoerrors.OperationRenameDirectories,
		oldAbsolutePath,
		repoerrors.ErrRenameFailed,
		message,
	)
}

func (executor *Executor) printfOutput(format string, arguments ...any) {
	if executor.dependencies.Reporter == nil {
		return
	}
	executor.dependencies.Reporter.Printf(format, arguments...)
}

func isCaseOnlyRename(oldPath string, newPath string) bool {
	return strings.EqualFold(oldPath, newPath) && oldPath != newPath
}

// parentDirectoryInformation describes the state of a parent directory for rename planning.
type parentDirectoryInformation struct {
	path        string
	exists      bool
	isDirectory bool
}
