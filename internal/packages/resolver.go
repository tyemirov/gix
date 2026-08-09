package packages

import (
	"github.com/tyemirov/gix/internal/ghcr"
	"go.uber.org/zap"
)

// DefaultRetentionServiceResolver builds retention services using the GHCR API.
type DefaultRetentionServiceResolver struct {
	HTTPClient           ghcr.HTTPClient
	ServiceConfiguration ghcr.ServiceConfiguration
}

// Resolve creates a retention executor using configured collaborators.
func (resolver *DefaultRetentionServiceResolver) Resolve(logger *zap.Logger) (RetentionExecutor, error) {
	packageService, serviceCreationError := ghcr.NewPackageVersionService(logger, resolver.HTTPClient, resolver.ServiceConfiguration)
	if serviceCreationError != nil {
		return nil, serviceCreationError
	}

	retentionService, retentionServiceError := NewRetentionService(logger, packageService)
	if retentionServiceError != nil {
		return nil, retentionServiceError
	}

	return retentionService, nil
}
