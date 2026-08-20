package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/gitrepo"
	migrate "github.com/tyemirov/gix/internal/migrate"
	"github.com/tyemirov/gix/internal/repos/identity"
)

const (
	defaultMigrationRemoteNameConstant                 = "origin"
	defaultMigrationWorkflowsDirectoryConstant         = ".github/workflows"
	migrationSuccessMessageTemplateConstant            = "WORKFLOW-DEFAULT: %s (%s → %s) safe_to_delete=%t source_deleted=%t\n"
	migrationIdentifierMissingMessageConstant          = "repository identifier unavailable for default-branch target"
	migrationExecutionErrorTemplateConstant            = "default branch update failed: %w"
	migrationRefreshErrorTemplateConstant              = "failed to refresh repository after default branch update: %w"
	migrationDependenciesMissingMessageConstant        = "default branch update requires repository manager, git executor, and GitHub client"
	migrationMultipleTargetsUnsupportedMessageConstant = "default branch update requires exactly one target configuration"
	migrationMetadataResolutionErrorTemplateConstant   = "default branch metadata resolution failed: %w"
	migrationMetadataMissingMessageConstant            = "repository metadata missing default branch for update"
	migrationSkipMessageTemplateConstant               = "WORKFLOW-DEFAULT-SKIP: %s already defaults to %s\n"
	remotePresenceResolutionErrorTemplateConstant      = "default branch remote resolution failed: %w"
	localDefaultResolutionErrorTemplateConstant        = "default branch local detection failed: %w"
	sourceBranchMissingMessageConstant                 = "default branch source not detected for promotion"
)

// BranchMigrationTarget describes branch migration behavior for discovered repositories.
type BranchMigrationTarget struct {
	RemoteName         string
	SourceBranch       string
	TargetBranch       string
	PushToRemote       bool
	DeleteSourceBranch bool
}

// BranchMigrationOperation performs default-branch migrations for configured targets.
type BranchMigrationOperation struct {
	Targets []BranchMigrationTarget
}

// Name identifies the workflow command handled by this operation.
func (operation *BranchMigrationOperation) Name() string {
	return commandDefaultKey
}

// Execute performs branch migration workflows for configured targets.
func (operation *BranchMigrationOperation) Execute(executionContext context.Context, environment *Environment, state *State) error {
	if environment == nil || state == nil {
		return nil
	}

	if environment.RepositoryManager == nil || environment.GitExecutor == nil || environment.GitHubClient == nil {
		return errors.New(migrationDependenciesMissingMessageConstant)
	}

	serviceDependencies := migrate.ServiceDependencies{
		Logger:            environment.Logger,
		RepositoryManager: environment.RepositoryManager,
		GitHubClient:      environment.GitHubClient,
		GitExecutor:       environment.GitExecutor,
	}

	migrationService, serviceError := migrate.NewService(serviceDependencies)
	if serviceError != nil {
		return fmt.Errorf(migrationExecutionErrorTemplateConstant, serviceError)
	}

	if len(operation.Targets) == 0 {
		return nil
	}
	if len(operation.Targets) > 1 {
		return errors.New(migrationMultipleTargetsUnsupportedMessageConstant)
	}

	target := operation.Targets[0]
	targetBranchValue := strings.TrimSpace(target.TargetBranch)
	if len(targetBranchValue) == 0 {
		return errors.New(branchMigrationTargetRequiredMessageConstant)
	}

	repositories := state.CloneRepositories()

	for repositoryIndex := range repositories {
		repositoryState := repositories[repositoryIndex]
		if repositoryState == nil {
			continue
		}

		repositoryPath := strings.TrimSpace(repositoryState.Path)
		if len(repositoryPath) == 0 {
			continue
		}

		remoteName := strings.TrimSpace(target.RemoteName)
		if len(remoteName) == 0 {
			remoteName = defaultMigrationRemoteNameConstant
		}

		localDefaultBranch, localDefaultError := resolveLocalDefaultBranch(executionContext, environment.RepositoryManager, repositoryState)
		if localDefaultError != nil {
			return fmt.Errorf(localDefaultResolutionErrorTemplateConstant, localDefaultError)
		}

		remoteResolution, remoteResolutionError := identity.ResolveRemoteIdentity(
			executionContext,
			identity.RemoteResolutionDependencies{
				RepositoryManager: environment.RepositoryManager,
				GitExecutor:       environment.GitExecutor,
				MetadataResolver:  environment.GitHubClient,
			},
			identity.RemoteResolutionOptions{
				RepositoryPath:            repositoryPath,
				RemoteName:                remoteName,
				ReportedOwnerRepository:   repositoryState.Inspection.FinalOwnerRepo,
				ReportedDefaultBranchName: repositoryState.Inspection.RemoteDefaultBranch,
			},
		)
		if remoteResolutionError != nil {
			return fmt.Errorf(remotePresenceResolutionErrorTemplateConstant, remoteResolutionError)
		}

		remoteAvailable := remoteResolution.RemoteDetected && remoteResolution.OwnerRepository != nil
		repositoryIdentifier := ""
		if remoteResolution.OwnerRepository != nil {
			repositoryIdentifier = remoteResolution.OwnerRepository.String()
		}

		remoteDefaultBranch := ""
		if remoteResolution.DefaultBranch != nil {
			remoteDefaultBranch = remoteResolution.DefaultBranch.String()
		}
		if !remoteAvailable {
			remoteDefaultBranch = ""
		}

		sourceBranchValue := strings.TrimSpace(target.SourceBranch)
		if len(sourceBranchValue) == 0 {
			if remoteAvailable && len(remoteDefaultBranch) > 0 {
				sourceBranchValue = remoteDefaultBranch
			} else if len(localDefaultBranch) > 0 {
				sourceBranchValue = localDefaultBranch
			}
		}

		if remoteAvailable && len(sourceBranchValue) == 0 {
			if remoteResolution.DefaultBranch == nil {
				return errors.New(sourceBranchMissingMessageConstant)
			}
			sourceBranchValue = remoteResolution.DefaultBranch.String()
		}

		if len(sourceBranchValue) == 0 {
			return errors.New(sourceBranchMissingMessageConstant)
		}

		sourceBranch := migrate.BranchName(sourceBranchValue)
		targetBranch := migrate.BranchName(targetBranchValue)

		skipMigration := false
		if remoteAvailable && len(remoteDefaultBranch) > 0 {
			skipMigration = strings.EqualFold(targetBranchValue, remoteDefaultBranch)
		} else if len(localDefaultBranch) > 0 {
			skipMigration = strings.EqualFold(targetBranchValue, localDefaultBranch)
		}

		if skipMigration {
			if environment.Output != nil {
				fmt.Fprintf(environment.Output, migrationSkipMessageTemplateConstant, repositoryState.Path, targetBranchValue)
			}
			continue
		}

		options := migrate.MigrationOptions{
			RepositoryPath:       repositoryPath,
			RepositoryRemoteName: remoteName,
			RepositoryIdentifier: repositoryIdentifier,
			WorkflowsDirectory:   defaultMigrationWorkflowsDirectoryConstant,
			SourceBranch:         sourceBranch,
			TargetBranch:         targetBranch,
			PushUpdates:          target.PushToRemote && remoteAvailable,
			DeleteSourceBranch:   target.DeleteSourceBranch && remoteAvailable,
		}

		result, executionError := migrationService.Execute(executionContext, options)
		if executionError != nil {
			var updateError migrate.DefaultBranchUpdateError
			if errors.As(executionError, &updateError) {
				return executionError
			}
			return fmt.Errorf(migrationExecutionErrorTemplateConstant, executionError)
		}

		if environment.Output != nil {
			fmt.Fprintf(environment.Output, migrationSuccessMessageTemplateConstant, repositoryState.Path, sourceBranchValue, targetBranchValue, result.SafetyStatus.SafeToDelete, result.SourceBranchDeleted)
			for _, warning := range result.Warnings {
				fmt.Fprintln(environment.Output, warning)
			}
		}

		if environment.AuditService != nil && remoteAvailable && result.DefaultBranchUpdated {
			if refreshError := repositoryState.Refresh(executionContext, environment.AuditService); refreshError != nil {
				return fmt.Errorf(migrationRefreshErrorTemplateConstant, refreshError)
			}
		}
	}

	return nil
}

func resolveLocalDefaultBranch(executionContext context.Context, manager *gitrepo.RepositoryManager, repositoryState *RepositoryState) (string, error) {
	if manager == nil {
		return "", errors.New(migrationDependenciesMissingMessageConstant)
	}
	if repositoryState == nil {
		return "", nil
	}
	if value := strings.TrimSpace(repositoryState.Inspection.LocalBranch); len(value) > 0 {
		return value, nil
	}
	if len(strings.TrimSpace(repositoryState.Path)) == 0 {
		return "", nil
	}
	branchName, branchError := manager.GetCurrentBranch(executionContext, repositoryState.Path)
	if branchError != nil {
		return "", branchError
	}
	return strings.TrimSpace(branchName), nil
}
