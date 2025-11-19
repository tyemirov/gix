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
	packageServiceMissingErrorMessageConstant    = "package version service must be provided"
	tokenResolverMissingErrorMessageConstant     = "token resolver must be provided"
	ownerOptionMissingErrorMessageConstant       = "owner option must be provided"
	packageOptionMissingErrorMessageConstant     = "package option must be provided"
	ownerTypeOptionMissingErrorMessageConstant   = "owner type option must be provided"
	tokenSourceOptionMissingErrorMessageConstant = "token source reference must be provided"
	purgeServiceStartMessageConstant             = "Executing repo-packages-purge operation"
	purgeServiceSummaryMessageConstant           = "repo-packages-purge operation completed"
	ownerLogFieldNameConstant                    = "owner"
	packageLogFieldNameConstant                  = "package"
	ownerTypeLogFieldNameConstant                = "owner_type"
	deletedVersionsLogFieldNameConstant          = "deleted_versions"
	untaggedVersionsLogFieldNameConstant         = "untagged_versions"
	totalVersionsLogFieldNameConstant            = "total_versions"
	tokenResolutionErrorTemplateConstant         = "unable to resolve authentication token: %w"
	purgeExecutionErrorTemplateConstant          = "unable to purge package versions: %w"
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
	TokenSource TokenSourceConfiguration
}

// PurgeExecutor defines the behavior required by the command layer.
type PurgeExecutor interface {
	Execute(executionContext context.Context, options PurgeOptions) (ghcr.PurgeResult, error)
}

// PurgeService orchestrates configuration validation, token resolution, and API invocation.
type PurgeService struct {
	logger         *zap.Logger
	packageService PackageVersionAPI
	tokenResolver  TokenResolver
}

// NewPurgeService constructs a purge service with required collaborators.
func NewPurgeService(logger *zap.Logger, packageService PackageVersionAPI, tokenResolver TokenResolver) (*PurgeService, error) {
	if packageService == nil {
		return nil, errors.New(packageServiceMissingErrorMessageConstant)
	}
	if tokenResolver == nil {
		return nil, errors.New(tokenResolverMissingErrorMessageConstant)
	}

	resolvedLogger := logger
	if resolvedLogger == nil {
		resolvedLogger = zap.NewNop()
	}

	return &PurgeService{
		logger:         resolvedLogger,
		packageService: packageService,
		tokenResolver:  tokenResolver,
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

	trimmedTokenSource := strings.TrimSpace(options.TokenSource.Reference)
	if len(trimmedTokenSource) == 0 {
		return ghcr.PurgeResult{}, errors.New(tokenSourceOptionMissingErrorMessageConstant)
	}

	service.logger.Info(
		purgeServiceStartMessageConstant,
		zap.String(ownerLogFieldNameConstant, trimmedOwner),
		zap.String(packageLogFieldNameConstant, trimmedPackageName),
		zap.String(ownerTypeLogFieldNameConstant, string(options.OwnerType)),
	)

	resolvedToken, tokenResolutionError := service.tokenResolver.ResolveToken(executionContext, options.TokenSource)
	if tokenResolutionError != nil {
		return ghcr.PurgeResult{}, fmt.Errorf(tokenResolutionErrorTemplateConstant, tokenResolutionError)
	}

	purgeRequest := ghcr.PurgeRequest{
		Owner:       trimmedOwner,
		PackageName: trimmedPackageName,
		OwnerType:   options.OwnerType,
		Token:       resolvedToken,
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
