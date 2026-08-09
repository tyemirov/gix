package migrate

import (
	"strings"

	pathutils "github.com/tyemirov/gix/internal/utils/path"
)

var migrateConfigurationRepositoryPathSanitizer = pathutils.NewRepositoryPathSanitizerWithConfiguration(nil, pathutils.RepositoryPathSanitizerConfiguration{PruneNestedPaths: true})

// CommandConfiguration captures persisted configuration for promoting a default branch.
type CommandConfiguration struct {
	EnableDebugLogging bool     `mapstructure:"debug"`
	RepositoryRoots    []string `mapstructure:"roots"`
	TargetBranch       string   `mapstructure:"to"`
}

// DefaultCommandConfiguration returns baseline configuration values for default branch promotion.
func DefaultCommandConfiguration() CommandConfiguration {
	return CommandConfiguration{
		EnableDebugLogging: false,
		RepositoryRoots:    nil,
		TargetBranch:       string(BranchMaster),
	}
}

// Sanitize trims configured values and removes empty entries.
func (configuration CommandConfiguration) Sanitize() CommandConfiguration {
	sanitized := configuration
	sanitized.RepositoryRoots = migrateConfigurationRepositoryPathSanitizer.Sanitize(configuration.RepositoryRoots)
	sanitized.TargetBranch = strings.TrimSpace(configuration.TargetBranch)
	if len(sanitized.TargetBranch) == 0 {
		sanitized.TargetBranch = string(BranchMaster)
	}
	return sanitized
}
