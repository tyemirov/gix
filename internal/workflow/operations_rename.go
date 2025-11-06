package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/temirov/gix/internal/repos/rename"
	"github.com/temirov/gix/internal/repos/shared"
)

const (
	renameRefreshErrorTemplateConstant = "failed to refresh repository after rename: %w"
)

// RenameOperation normalizes repository directory names to match canonical GitHub names.
type RenameOperation struct {
	RequireCleanWorktree bool
	requireCleanExplicit bool
	IncludeOwner         bool
}

// Name identifies the workflow command handled by this operation.
func (operation *RenameOperation) Name() string {
	return commandFolderRenameKey
}

// Execute applies rename operations for repositories with desired folder names.
func (operation *RenameOperation) Execute(executionContext context.Context, environment *Environment, state *State) error {
	if environment == nil || state == nil {
		return nil
	}

	directoryPlanner := rename.NewDirectoryPlanner()
	dependencies := rename.Dependencies{
		FileSystem: environment.FileSystem,
		GitManager: environment.RepositoryManager,
		Prompter:   environment.Prompter,
		Clock:      shared.SystemClock{},
		Reporter:   environment.Reporter,
	}

	for repositoryIndex := range state.Repositories {
		repository := state.Repositories[repositoryIndex]
		repositoryPath, repositoryPathError := shared.NewRepositoryPath(repository.Path)
		if repositoryPathError != nil {
			return fmt.Errorf("rename directories: %w", repositoryPathError)
		}
		plan := directoryPlanner.Plan(operation.IncludeOwner, repository.Inspection.FinalOwnerRepo, repository.Inspection.DesiredFolderName)
		desiredFolderName := plan.FolderName
		if plan.IsNoop(repository.Path, repository.Inspection.FolderName) {
			desiredFolderName = filepath.Base(repository.Path)
		}
		trimmedFolderName := strings.TrimSpace(desiredFolderName)
		if len(trimmedFolderName) == 0 {
			continue
		}

		assumeYes := false
		if environment.PromptState != nil {
			assumeYes = environment.PromptState.IsAssumeYesEnabled()
		}

		originalPath := repositoryPath.String()

		options := rename.Options{
			RepositoryPath:          repositoryPath,
			DesiredFolderName:       trimmedFolderName,
			CleanPolicy:             shared.CleanWorktreePolicyFromBool(operation.RequireCleanWorktree),
			ConfirmationPolicy:      shared.ConfirmationPolicyFromBool(assumeYes),
			IncludeOwner:            plan.IncludeOwner,
			EnsureParentDirectories: plan.IncludeOwner,
		}

		if executionError := rename.Execute(executionContext, dependencies, options); executionError != nil {
			if logRepositoryOperationError(environment, executionError) {
				continue
			}
			return fmt.Errorf("rename directories: %w", executionError)
		}

		newPath := filepath.Join(filepath.Dir(originalPath), plan.FolderName)
		if !renameCompleted(environment.FileSystem, originalPath, newPath) {
			continue
		}

		if updateError := state.UpdateRepositoryPath(repositoryIndex, newPath); updateError != nil {
			return updateError
		}

		if refreshError := repository.Refresh(executionContext, environment.AuditService); refreshError != nil {
			return fmt.Errorf(renameRefreshErrorTemplateConstant, refreshError)
		}
	}

	return nil
}

// ApplyRequireCleanDefault enables clean-worktree enforcement when no explicit preference was configured.
func (operation *RenameOperation) ApplyRequireCleanDefault(requireClean bool) {
	if operation == nil {
		return
	}
	if operation.requireCleanExplicit {
		return
	}
	operation.RequireCleanWorktree = requireClean
}

func renameCompleted(fileSystem shared.FileSystem, originalPath string, newPath string) bool {
	if fileSystem == nil {
		return false
	}

	newInfo, newStatError := fileSystem.Stat(newPath)
	if newStatError != nil {
		return false
	}

	originalInfo, originalStatError := fileSystem.Stat(originalPath)
	if originalStatError != nil {
		return true
	}

	return os.SameFile(originalInfo, newInfo)
}
