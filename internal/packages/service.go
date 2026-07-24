package packages

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/ghcr"
)

const (
	packageServiceMissingErrorMessageConstant   = "package version service must be provided"
	ownerOptionMissingErrorMessageConstant      = "owner option must be provided"
	packageOptionMissingErrorMessageConstant    = "package option must be provided"
	ownerTypeOptionMissingErrorMessageConstant  = "owner type option must be provided"
	credentialOptionMissingErrorMessageConstant = "packages credential must be provided"
	purgeServiceStartMessageConstant            = "Executing repo-packages-purge operation"
	purgeServiceSummaryMessageConstant          = "repo-packages-purge operation completed"
	ownerLogFieldNameConstant                   = "owner"
	packageLogFieldNameConstant                 = "package"
	ownerTypeLogFieldNameConstant               = "owner_type"
	deletedVersionsLogFieldNameConstant         = "deleted_versions"
	untaggedVersionsLogFieldNameConstant        = "untagged_versions"
	totalVersionsLogFieldNameConstant           = "total_versions"
	purgeExecutionErrorTemplateConstant         = "unable to purge package versions: %w"
)

// PackageVersionAPI describes the GHCR operations used by the purge service.
type PackageVersionAPI interface {
	PurgeUntaggedVersions(executionContext context.Context, request ghcr.PurgeRequest) (ghcr.PurgeResult, error)
}

// PurgeOptions represents validated inputs for package purging.
type PurgeOptions struct {
	Owner       string
	PackageName string
	OwnerType   ghcr.OwnerType
	Credential  string
}

// PurgeExecutor defines the behavior required by the command layer.
type PurgeExecutor interface {
	Execute(executionContext context.Context, options PurgeOptions) (ghcr.PurgeResult, error)
}

// PurgeService orchestrates configuration validation, token resolution, and API invocation.
type PurgeService struct {
	logger         *zap.Logger
	packageService PackageVersionAPI
}

// NewPurgeService constructs a purge service with required collaborators.
func NewPurgeService(logger *zap.Logger, packageService PackageVersionAPI) (*PurgeService, error) {
	if packageService == nil {
		return nil, errors.New(packageServiceMissingErrorMessageConstant)
	}

	resolvedLogger := logger
	if resolvedLogger == nil {
		resolvedLogger = zap.NewNop()
	}

	return &PurgeService{
		logger:         resolvedLogger,
		packageService: packageService,
	}, nil
}

// Execute performs the purge workflow for the provided options.
func (service *PurgeService) Execute(executionContext context.Context, options PurgeOptions) (ghcr.PurgeResult, error) {
	trimmedOwner := strings.TrimSpace(options.Owner)
	if len(trimmedOwner) == 0 {
		return ghcr.PurgeResult{}, errors.New(ownerOptionMissingErrorMessageConstant)
	}

	trimmedPackageName := strings.TrimSpace(options.PackageName)
	if len(trimmedPackageName) == 0 {
		return ghcr.PurgeResult{}, errors.New(packageOptionMissingErrorMessageConstant)
	}

	if len(strings.TrimSpace(string(options.OwnerType))) == 0 {
		return ghcr.PurgeResult{}, errors.New(ownerTypeOptionMissingErrorMessageConstant)
	}

	trimmedCredential := strings.TrimSpace(options.Credential)
	if len(trimmedCredential) == 0 {
		return ghcr.PurgeResult{}, errors.New(credentialOptionMissingErrorMessageConstant)
	}

	service.logger.Info(
		purgeServiceStartMessageConstant,
		zap.String(ownerLogFieldNameConstant, trimmedOwner),
		zap.String(packageLogFieldNameConstant, trimmedPackageName),
		zap.String(ownerTypeLogFieldNameConstant, string(options.OwnerType)),
	)

	purgeRequest := ghcr.PurgeRequest{
		Owner:       trimmedOwner,
		PackageName: trimmedPackageName,
		OwnerType:   options.OwnerType,
		Token:       trimmedCredential,
	}

	purgeResult, purgeError := service.packageService.PurgeUntaggedVersions(executionContext, purgeRequest)
	if purgeError != nil {
		return ghcr.PurgeResult{}, fmt.Errorf(purgeExecutionErrorTemplateConstant, purgeError)
	}

	service.logger.Info(
		purgeServiceSummaryMessageConstant,
		zap.Int(totalVersionsLogFieldNameConstant, purgeResult.TotalVersions),
		zap.Int(untaggedVersionsLogFieldNameConstant, purgeResult.UntaggedVersions),
		zap.Int(deletedVersionsLogFieldNameConstant, purgeResult.DeletedVersions),
	)

	return purgeResult, nil
}
