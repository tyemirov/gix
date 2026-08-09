package refresh

import (
	"strings"

	pathutils "github.com/tyemirov/gix/internal/utils/path"
)

var refreshConfigurationRepositoryPathSanitizer = pathutils.NewRepositoryPathSanitizerWithConfiguration(nil, pathutils.RepositoryPathSanitizerConfiguration{PruneNestedPaths: true})

// CommandConfiguration captures configuration values for the branch refresh command.
type CommandConfiguration struct {
	RepositoryRoots []string `mapstructure:"roots"`
	BranchName      string   `mapstructure:"branch"`
}

// DefaultCommandConfiguration returns empty defaults for the branch refresh command.
func DefaultCommandConfiguration() CommandConfiguration {
	return CommandConfiguration{}
}

// Sanitize trims textual configuration values and normalizes repository roots.
func (configuration CommandConfiguration) Sanitize() CommandConfiguration {
	sanitized := configuration
	sanitized.BranchName = strings.TrimSpace(configuration.BranchName)
	sanitized.RepositoryRoots = refreshConfigurationRepositoryPathSanitizer.Sanitize(configuration.RepositoryRoots)
	return sanitized
}
