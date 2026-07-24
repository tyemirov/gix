package packages

import (
	"github.com/tyemirov/gix/internal/ghcr"
	"go.uber.org/zap"
)

// DefaultPurgeServiceResolver builds purge services using GHCR APIs and token resolution.
type DefaultPurgeServiceResolver struct {
	HTTPClient           ghcr.HTTPClient
	ServiceConfiguration ghcr.ServiceConfiguration
}

// Resolve creates a purge executor using configured collaborators or sensible defaults.
func (resolver *DefaultPurgeServiceResolver) Resolve(logger *zap.Logger) (PurgeExecutor, error) {
	packageService, serviceCreationError := ghcr.NewPackageVersionService(logger, resolver.HTTPClient, resolver.ServiceConfiguration)
	if serviceCreationError != nil {
		return nil, serviceCreationError
	}

	purgeService, purgeServiceError := NewPurgeService(logger, packageService)
	if purgeServiceError != nil {
		return nil, purgeServiceError
	}

	return purgeService, nil
}
