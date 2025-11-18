package packages

import (
	pathutils "github.com/tyemirov/gix/internal/utils/path"
)

var packagesConfigurationRepositoryPathSanitizer = pathutils.NewRepositoryPathSanitizerWithConfiguration(nil, pathutils.RepositoryPathSanitizerConfiguration{PruneNestedPaths: true})

const (
	defaultTokenSourceValueConstant = "env:GITHUB_PACKAGES_TOKEN"
)

// Configuration aggregates settings for packages commands.
type Configuration struct {
	Purge PurgeConfiguration `mapstructure:"purge"`
}

// PurgeConfiguration stores options for purging container versions.
type PurgeConfiguration struct {
	PackageName     string   `mapstructure:"package"`
	RepositoryRoots []string `mapstructure:"roots"`
}

// DefaultConfiguration supplies baseline values for packages configuration.
func DefaultConfiguration() Configuration {
	return Configuration{
		Purge: PurgeConfiguration{},
	}
}

// Sanitize trims configured values and removes empty entries.
func (configuration Configuration) Sanitize() Configuration {
	sanitized := configuration
	sanitized.Purge = configuration.Purge.Sanitize()
	return sanitized
}

// Sanitize trims purge configuration values and removes empty entries.
func (configuration PurgeConfiguration) Sanitize() PurgeConfiguration {
	sanitized := configuration
	sanitized.RepositoryRoots = packagesConfigurationRepositoryPathSanitizer.Sanitize(configuration.RepositoryRoots)
	return sanitized
}
