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

// Name identifies the workflow command handled by this operation.
func (operation *ProtocolConversionOperation) Name() string {
	return commandRemoteConvertProtocolKey
}

// Execute applies the protocol conversion to repositories matching the source protocol.
func (operation *ProtocolConversionOperation) Execute(executionContext context.Context, environment *Environment, state *State) error {
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

// ExecuteForRepository converts remotes for a single repository.
func (operation *ProtocolConversionOperation) ExecuteForRepository(
	executionContext context.Context,
	environment *Environment,
	repository *RepositoryState,
) error {
	if environment == nil || repository == nil {
		return nil
	}

	dependencies := conversion.Dependencies{
		GitManager: environment.RepositoryManager,
		Prompter:   environment.Prompter,
		Reporter:   environment.stepScopedReporter(),
	}

	actualProtocol, actualProtocolError := shared.ParseRemoteProtocol(string(repository.Inspection.RemoteProtocol))
	if actualProtocolError != nil {
		return fmt.Errorf("protocol conversion: %w", actualProtocolError)
	}

	if actualProtocol != operation.FromProtocol {
		message := fmt.Sprintf("Protocol Conversion (no-op: current protocol %s does not match from %s)", actualProtocol, operation.FromProtocol)
		environment.ReportRepositoryEvent(
			repository,
			shared.EventLevelInfo,
			shared.EventCodeProtocolSkip,
			message,
			map[string]string{
				"reason":           "protocol_mismatch",
				"current_protocol": string(actualProtocol),
				"from_protocol":    string(operation.FromProtocol),
				"target_protocol":  string(operation.ToProtocol),
			},
		)
		return nil
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
		return nil
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
		return nil
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
		ConfirmationPolicy:       shared.ConfirmationPolicyFromBool(assumeYes),
	}

	if executionError := conversion.Execute(executionContext, dependencies, options); executionError != nil {
		if logRepositoryOperationError(environment, executionError) {
			return nil
		}
		return fmt.Errorf("protocol conversion: %w", executionError)
	}

	if refreshError := repository.Refresh(executionContext, environment.AuditService); refreshError != nil {
		return fmt.Errorf(protocolRefreshErrorTemplateConstant, refreshError)
	}

	return nil
}

// IsRepositoryScoped reports repository-level execution behavior.
func (operation *ProtocolConversionOperation) IsRepositoryScoped() bool {
	return true
}
