package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tyemirov/gix/internal/repos/rename"
	"github.com/tyemirov/gix/internal/repos/shared"
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

	for _, repository := range state.Repositories {
		if repository == nil {
			continue
		}
		if err := operation.ExecuteForRepository(executionContext, environment, repository); err != nil {
			return err
		}
	}

	return nil
}

// ExecuteForRepository applies rename operations for a single repository.
func (operation *RenameOperation) ExecuteForRepository(
	executionContext context.Context,
	environment *Environment,
	repository *RepositoryState,
) error {
	if environment == nil || repository == nil {
		return nil
	}

	directoryPlanner := rename.NewDirectoryPlanner()
	dependencies := rename.Dependencies{
		FileSystem: environment.FileSystem,
		GitManager: environment.RepositoryManager,
		Prompter:   environment.Prompter,
		Clock:      shared.SystemClock{},
		Reporter:   environment.stepScopedReporter(),
	}

	repositoryPath, repositoryPathError := shared.NewRepositoryPath(repository.Path)
	if repositoryPathError != nil {
		return fmt.Errorf("rename directories: %w", repositoryPathError)
	}

	ownerRepository, _ := shared.ParseOwnerRepositoryOptional(repository.Inspection.FinalOwnerRepo)
	plan := directoryPlanner.Plan(operation.IncludeOwner, ownerRepository, repository.Inspection.DesiredFolderName)
	desiredFolderName := plan.FolderName
	if plan.IsNoop(repository.Path, repository.Inspection.FolderName) {
		desiredFolderName = filepath.Base(repository.Path)
	}
	trimmedFolderName := strings.TrimSpace(desiredFolderName)
	if len(trimmedFolderName) == 0 {
		return nil
	}

	assumeYes := false
	if environment.PromptState != nil {
		assumeYes = environment.PromptState.IsAssumeYesEnabled()
	}

	originalPath := repositoryPath.String()

	options, optionsError := rename.NewOptions(rename.OptionsDefinition{
		RepositoryPath:          repositoryPath,
		DesiredFolderName:       trimmedFolderName,
		CleanPolicy:             shared.CleanWorktreePolicyFromBool(operation.RequireCleanWorktree),
		ConfirmationPolicy:      shared.ConfirmationPolicyFromBool(assumeYes),
		IncludeOwner:            plan.IncludeOwner,
		EnsureParentDirectories: plan.IncludeOwner,
	})
	if optionsError != nil {
		return fmt.Errorf("rename directories: %w", optionsError)
	}

	if executionError := rename.Execute(executionContext, dependencies, options); executionError != nil {
		if logRepositoryOperationError(environment, executionError) {
			return nil
		}
		return fmt.Errorf("rename directories: %w", executionError)
	}

	newPath := filepath.Join(filepath.Dir(originalPath), plan.FolderName)
	if !renameCompleted(environment.FileSystem, originalPath, newPath) {
		return nil
	}

	repository.Path = newPath

	if refreshError := repository.Refresh(executionContext, environment.AuditService); refreshError != nil {
		return fmt.Errorf(renameRefreshErrorTemplateConstant, refreshError)
	}

	return nil
}

// IsRepositoryScoped reports repository-level execution behavior.
func (operation *RenameOperation) IsRepositoryScoped() bool {
	return true
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
