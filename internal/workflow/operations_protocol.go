package workflow

import (
	"context"
	"fmt"

	repoerrors "github.com/tyemirov/gix/internal/repos/errors"
	"github.com/tyemirov/gix/internal/repos/identity"
	conversion "github.com/tyemirov/gix/internal/repos/protocol"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	protocolRefreshErrorTemplateConstant = "failed to refresh repository after protocol conversion: %w"
)

// ProtocolConversionOperation converts repository remotes between protocols.
type ProtocolConversionOperation struct {
	FromProtocol shared.RemoteProtocol
	ToProtocol   shared.RemoteProtocol
}

// Name identifies the operation type.
func (operation *ProtocolConversionOperation) Name() string {
	return string(OperationTypeProtocolConversion)
}

// Execute applies the protocol conversion to repositories matching the source protocol.
func (operation *ProtocolConversionOperation) Execute(executionContext context.Context, environment *Environment, state *State) error {
	if environment == nil || state == nil {
		return nil
	}

	dependencies := conversion.Dependencies{
		GitManager: environment.RepositoryManager,
		Prompter:   environment.Prompter,
		Reporter:   environment.Reporter,
	}

	for repositoryIndex := range state.Repositories {
		repository := state.Repositories[repositoryIndex]

		actualProtocol, actualProtocolError := shared.ParseRemoteProtocol(string(repository.Inspection.RemoteProtocol))
		if actualProtocolError != nil {
			return fmt.Errorf("protocol conversion: %w", actualProtocolError)
		}

		if actualProtocol != operation.FromProtocol {
			continue
		}

		assumeYes := false
		if environment.PromptState != nil {
			assumeYes = environment.PromptState.IsAssumeYesEnabled()
		}

		repositoryPath, repositoryPathError := shared.NewRepositoryPath(repository.Path)
		if repositoryPathError != nil {
			return fmt.Errorf("protocol conversion: %w", repositoryPathError)
		}

		originOwnerRepository, originOwnerError := shared.ParseOwnerRepositoryOptional(repository.Inspection.OriginOwnerRepo)
		if originOwnerError != nil {
			return fmt.Errorf("protocol conversion: %w", originOwnerError)
		}

		canonicalOwnerRepository, canonicalOwnerError := shared.ParseOwnerRepositoryOptional(repository.Inspection.CanonicalOwnerRepo)
		if canonicalOwnerError != nil {
			return fmt.Errorf("protocol conversion: %w", canonicalOwnerError)
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
			return fmt.Errorf("protocol conversion: %w", remoteResolutionError)
		}

		if !remoteResolution.RemoteDetected {
			skipMessage := fmt.Sprintf("SKIP: remote '%s' not configured", shared.OriginRemoteNameConstant)
			skipError := repoerrors.WrapMessage(
				repoerrors.OperationProtocolConvert,
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
				repoerrors.OperationProtocolConvert,
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

		options := conversion.Options{
			RepositoryPath:           repositoryPath,
			OriginOwnerRepository:    originOwnerRepository,
			CanonicalOwnerRepository: canonicalOwnerRepository,
			CurrentProtocol:          operation.FromProtocol,
			TargetProtocol:           operation.ToProtocol,
			DryRun:                   environment.DryRun,
			ConfirmationPolicy:       shared.ConfirmationPolicyFromBool(assumeYes),
		}

		if executionError := conversion.Execute(executionContext, dependencies, options); executionError != nil {
			if logRepositoryOperationError(environment, executionError) {
				continue
			}
			return fmt.Errorf("protocol conversion: %w", executionError)
		}

		if environment.DryRun {
			continue
		}

		if refreshError := repository.Refresh(executionContext, environment.AuditService); refreshError != nil {
			return fmt.Errorf(protocolRefreshErrorTemplateConstant, refreshError)
		}
	}

	return nil
}
