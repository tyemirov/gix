package packages

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/tyemirov/gix/v5/internal/ghcr"
)

const (
	packageServiceMissingErrorMessageConstant   = "package version service must be provided"
	ownerOptionMissingErrorMessageConstant      = "owner option must be provided"
	packageOptionMissingErrorMessageConstant    = "package option must be provided"
	ownerTypeOptionMissingErrorMessageConstant  = "owner type option must be provided"
	credentialOptionMissingErrorMessageConstant = "packages credential must be provided"
	retentionServiceStartMessageConstant        = "Executing package retention operation"
	retentionServiceSummaryMessageConstant      = "Package retention operation completed"
	ownerLogFieldNameConstant                   = "owner"
	packageLogFieldNameConstant                 = "package"
	ownerTypeLogFieldNameConstant               = "owner_type"
	deletedVersionsLogFieldNameConstant         = "deleted_versions"
	retainedVersionsLogFieldNameConstant        = "retained_versions"
	totalVersionsLogFieldNameConstant           = "total_versions"
	retentionExecutionErrorTemplateConstant     = "unable to apply package retention: %w"
)

// PackageVersionAPI describes the GHCR operations used by the retention service.
type PackageVersionAPI interface {
	ApplyRetention(executionContext context.Context, request ghcr.RetentionRequest) (ghcr.RetentionResult, error)
}

// RetentionOptions represents validated inputs for package retention.
type RetentionOptions struct {
	Owner       string
	PackageName string
	OwnerType   ghcr.OwnerType
	Credential  string
	Keep        ghcr.KeepCount
}

// RetentionExecutor defines the behavior required by the command layer.
type RetentionExecutor interface {
	Execute(executionContext context.Context, options RetentionOptions) (ghcr.RetentionResult, error)
}

// RetentionService orchestrates validation and API invocation.
type RetentionService struct {
	logger         *zap.Logger
	packageService PackageVersionAPI
}

// NewRetentionService constructs a retention service with required collaborators.
func NewRetentionService(logger *zap.Logger, packageService PackageVersionAPI) (*RetentionService, error) {
	if packageService == nil {
		return nil, errors.New(packageServiceMissingErrorMessageConstant)
	}

	resolvedLogger := logger
	if resolvedLogger == nil {
		resolvedLogger = zap.NewNop()
	}

	return &RetentionService{
		logger:         resolvedLogger,
		packageService: packageService,
	}, nil
}

// Execute performs the retention workflow for the provided options.
func (service *RetentionService) Execute(executionContext context.Context, options RetentionOptions) (ghcr.RetentionResult, error) {
	trimmedOwner := strings.TrimSpace(options.Owner)
	if len(trimmedOwner) == 0 {
		return ghcr.RetentionResult{}, errors.New(ownerOptionMissingErrorMessageConstant)
	}

	trimmedPackageName := strings.TrimSpace(options.PackageName)
	if len(trimmedPackageName) == 0 {
		return ghcr.RetentionResult{}, errors.New(packageOptionMissingErrorMessageConstant)
	}

	if len(strings.TrimSpace(string(options.OwnerType))) == 0 {
		return ghcr.RetentionResult{}, errors.New(ownerTypeOptionMissingErrorMessageConstant)
	}
	ownerType, ownerTypeError := ghcr.ParseOwnerType(string(options.OwnerType))
	if ownerTypeError != nil {
		return ghcr.RetentionResult{}, ownerTypeError
	}

	trimmedCredential := strings.TrimSpace(options.Credential)
	if len(trimmedCredential) == 0 {
		return ghcr.RetentionResult{}, errors.New(credentialOptionMissingErrorMessageConstant)
	}

	keepCount, keepCountError := ghcr.NewKeepCount(options.Keep.Value())
	if keepCountError != nil {
		return ghcr.RetentionResult{}, keepCountError
	}

	service.logger.Info(
		retentionServiceStartMessageConstant,
		zap.String(ownerLogFieldNameConstant, trimmedOwner),
		zap.String(packageLogFieldNameConstant, trimmedPackageName),
		zap.String(ownerTypeLogFieldNameConstant, string(ownerType)),
	)

	retentionRequest := ghcr.RetentionRequest{
		Owner:       trimmedOwner,
		PackageName: trimmedPackageName,
		OwnerType:   ownerType,
		Token:       trimmedCredential,
		Keep:        keepCount,
	}

	retentionResult, retentionError := service.packageService.ApplyRetention(executionContext, retentionRequest)
	if retentionError != nil {
		return retentionResult, fmt.Errorf(retentionExecutionErrorTemplateConstant, retentionError)
	}

	service.logger.Info(
		retentionServiceSummaryMessageConstant,
		zap.Int(totalVersionsLogFieldNameConstant, retentionResult.TotalVersions),
		zap.Int(retainedVersionsLogFieldNameConstant, retentionResult.RetainedVersions),
		zap.Int(deletedVersionsLogFieldNameConstant, retentionResult.DeletedVersions),
	)

	return retentionResult, nil
}
