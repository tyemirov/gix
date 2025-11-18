package packages

import (
	"os"
	"strings"

	"github.com/tyemirov/gix/internal/ghcr"
	"go.uber.org/zap"
)

// DefaultPurgeServiceResolver builds purge services using GHCR APIs and token resolution.
type DefaultPurgeServiceResolver struct {
	HTTPClient        ghcr.HTTPClient
	EnvironmentLookup EnvironmentLookup
	FileReader        FileReader
	TokenResolver     TokenResolver
}

const (
	serviceBaseURLEnvironmentVariableNameConstant = "GIX_REPO_PACKAGES_PURGE_BASE_URL"
)

// Resolve creates a purge executor using configured collaborators or sensible defaults.
func (resolver *DefaultPurgeServiceResolver) Resolve(logger *zap.Logger) (PurgeExecutor, error) {
	serviceConfiguration := resolver.resolveServiceConfiguration()
	packageService, serviceCreationError := ghcr.NewPackageVersionService(logger, resolver.HTTPClient, serviceConfiguration)
	if serviceCreationError != nil {
		return nil, serviceCreationError
	}

	resolvedTokenResolver := resolver.TokenResolver
	if resolvedTokenResolver == nil {
		resolvedTokenResolver = NewTokenResolver(resolver.EnvironmentLookup, resolver.FileReader)
	}

	purgeService, purgeServiceError := NewPurgeService(logger, packageService, resolvedTokenResolver)
	if purgeServiceError != nil {
		return nil, purgeServiceError
	}

	return purgeService, nil
}

func (resolver *DefaultPurgeServiceResolver) resolveServiceConfiguration() ghcr.ServiceConfiguration {
	environmentLookup := resolver.EnvironmentLookup
	if environmentLookup == nil {
		environmentLookup = os.LookupEnv
	}

	baseURLValue, exists := environmentLookup(serviceBaseURLEnvironmentVariableNameConstant)
	if !exists {
		return ghcr.ServiceConfiguration{}
	}

	trimmedBaseURL := strings.TrimSpace(baseURLValue)
	if len(trimmedBaseURL) == 0 {
		return ghcr.ServiceConfiguration{}
	}

	return ghcr.ServiceConfiguration{BaseURL: trimmedBaseURL}
}
