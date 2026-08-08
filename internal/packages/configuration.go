package packages

import (
	pathutils "github.com/tyemirov/gix/v5/internal/utils/path"
)

var packagesConfigurationRepositoryPathSanitizer = pathutils.NewRepositoryPathSanitizerWithConfiguration(nil, pathutils.RepositoryPathSanitizerConfiguration{PruneNestedPaths: true})

// Configuration aggregates settings for packages commands.
type Configuration struct {
	Delete DeleteConfiguration `mapstructure:"delete"`
}

// DeleteConfiguration stores options for deleting versions outside retention.
type DeleteConfiguration struct {
	PackageName     string   `mapstructure:"package"`
	RepositoryRoots []string `mapstructure:"roots"`
	BaseURL         string   `mapstructure:"base_url"`
	Credential      string   `mapstructure:"credential"`
}

// DefaultConfiguration supplies baseline values for packages configuration.
func DefaultConfiguration() Configuration {
	return Configuration{
		Delete: DeleteConfiguration{},
	}
}

// Sanitize trims configured values and removes empty entries.
func (configuration Configuration) Sanitize() Configuration {
	sanitized := configuration
	sanitized.Delete = configuration.Delete.Sanitize()
	return sanitized
}

// Sanitize trims delete configuration values and removes empty entries.
func (configuration DeleteConfiguration) Sanitize() DeleteConfiguration {
	sanitized := configuration
	sanitized.RepositoryRoots = packagesConfigurationRepositoryPathSanitizer.Sanitize(configuration.RepositoryRoots)
	return sanitized
}
