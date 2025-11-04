package workflow

import (
	"context"
	"fmt"

	repoerrors "github.com/temirov/gix/internal/repos/errors"
	"github.com/temirov/gix/internal/repos/identity"
	"github.com/temirov/gix/internal/repos/remotes"
	"github.com/temirov/gix/internal/repos/shared"
)

const (
	canonicalRemoteRefreshErrorTemplateConstant = "failed to refresh repository after canonical remote update: %w"
)

// CanonicalRemoteOperation updates origin URLs to their canonical GitHub equivalents.
type CanonicalRemoteOperation struct {
	OwnerConstraint string
}

// Name identifies the workflow command handled by this operation.
func (operation *CanonicalRemoteOperation) Name() string {
	return commandRepoRemoteCanonicalKey
}

// Execute applies canonical remote updates using inspection metadata.
func (operation *CanonicalRemoteOperation) Execute(executionContext context.Context, environment *Environment, state *State) error {
	if environment == nil || state == nil {
		return nil
	}

	dependencies := remotes.Dependencies{
		GitManager: environment.RepositoryManager,
		Prompter:   environment.Prompter,
		Reporter:   environment.Reporter,
	}

	for repositoryIndex := range state.Repositories {
		repository := state.Repositories[repositoryIndex]
		originOwnerRepository, originOwnerError := shared.ParseOwnerRepositoryOptional(repository.Inspection.OriginOwnerRepo)
		if originOwnerError != nil {
			return fmt.Errorf("canonical remote update: %w", originOwnerError)
		}
		canonicalOwnerRepository, canonicalOwnerError := shared.ParseOwnerRepositoryOptional(repository.Inspection.CanonicalOwnerRepo)
		if canonicalOwnerError != nil {
			return fmt.Errorf("canonical remote update: %w", canonicalOwnerError)
		}

		remoteResolution, remoteResolutionError := identity.ResolveRemoteIdentity(
			executionContext,
			identity.RemoteResolutionDependencies{
				RepositoryManager: environment.RepositoryManager,
				GitExecutor:       environment.GitExecutor,
				MetadataResolver:  environment.GitHubClient,
			},
			identity.RemoteResolutionOptions{
				RepositoryPath:            repository.Path,
				RemoteName:                shared.OriginRemoteNameConstant,
				ReportedOwnerRepository:   repository.Inspection.FinalOwnerRepo,
				ReportedDefaultBranchName: repository.Inspection.RemoteDefaultBranch,
			},
		)
		if remoteResolutionError != nil {
			return fmt.Errorf("canonical remote update: %w", remoteResolutionError)
		}

		if !remoteResolution.RemoteDetected {
			skipMessage := fmt.Sprintf("SKIP: remote '%s' not configured", shared.OriginRemoteNameConstant)
			skipError := repoerrors.WrapMessage(
				repoerrors.OperationCanonicalRemote,
				repository.Path,
				repoerrors.ErrRemoteMissing,
				skipMessage,
			)
			logRepositoryOperationError(environment, skipError)
			continue
		}

		if remoteResolution.OwnerRepository == nil {
			skipMessage := fmt.Sprintf("SKIP: remote metadata unavailable for remote '%s'", shared.OriginRemoteNameConstant)
			metadataError := repoerrors.WrapMessage(
				repoerrors.OperationCanonicalRemote,
				repository.Path,
				repoerrors.ErrOriginOwnerMissing,
				skipMessage,
			)
			logRepositoryOperationError(environment, metadataError)
			continue
		}

		if canonicalOwnerRepository == nil && remoteResolution.OwnerRepository != nil {
			canonicalOwnerRepository = remoteResolution.OwnerRepository
		}
		if originOwnerRepository == nil && remoteResolution.OwnerRepository != nil {
			originOwnerRepository = remoteResolution.OwnerRepository
		}
		if originOwnerRepository == nil && canonicalOwnerRepository == nil {
			continue
		}
		assumeYes := false
		if environment.PromptState != nil {
			assumeYes = environment.PromptState.IsAssumeYesEnabled()
		}

		repositoryPath, repositoryPathError := shared.NewRepositoryPath(repository.Path)
		if repositoryPathError != nil {
			return fmt.Errorf("canonical remote update: %w", repositoryPathError)
		}

		currentRemoteURL, currentRemoteURLError := shared.ParseRemoteURLOptional(repository.Inspection.OriginURL)
		if currentRemoteURLError != nil {
			return fmt.Errorf("canonical remote update: %w", currentRemoteURLError)
		}

		remoteProtocol, remoteProtocolError := shared.ParseRemoteProtocol(string(repository.Inspection.RemoteProtocol))
		if remoteProtocolError != nil {
			return fmt.Errorf("canonical remote update: %w", remoteProtocolError)
		}

		ownerConstraint, ownerConstraintError := shared.ParseOwnerSlugOptional(operation.OwnerConstraint)
		if ownerConstraintError != nil {
			return fmt.Errorf("canonical remote update: %w", ownerConstraintError)
		}

		options := remotes.Options{
			RepositoryPath:           repositoryPath,
			CurrentOriginURL:         currentRemoteURL,
			OriginOwnerRepository:    originOwnerRepository,
			CanonicalOwnerRepository: canonicalOwnerRepository,
			RemoteProtocol:           remoteProtocol,
			DryRun:                   environment.DryRun,
			ConfirmationPolicy:       shared.ConfirmationPolicyFromBool(assumeYes),
			OwnerConstraint:          ownerConstraint,
		}

		if executionError := remotes.Execute(executionContext, dependencies, options); executionError != nil {
			if logRepositoryOperationError(environment, executionError) {
				continue
			}
			return fmt.Errorf("canonical remote update: %w", executionError)
		}

		if environment.DryRun {
			continue
		}

		if refreshError := repository.Refresh(executionContext, environment.AuditService); refreshError != nil {
			return fmt.Errorf(canonicalRemoteRefreshErrorTemplateConstant, refreshError)
		}
	}

	return nil
}
