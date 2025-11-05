package cd

import (
	"strings"

	pathutils "github.com/temirov/gix/internal/utils/path"
)

var commandConfigurationRepositorySanitizer = pathutils.NewRepositoryPathSanitizerWithConfiguration(nil, pathutils.RepositoryPathSanitizerConfiguration{PruneNestedPaths: true})

// CommandConfiguration captures configuration values for the branch-cd command.
type CommandConfiguration struct {
	RepositoryRoots []string `mapstructure:"roots"`
	DefaultBranch   string   `mapstructure:"branch"`
	RemoteName      string   `mapstructure:"remote"`
	CreateIfMissing bool     `mapstructure:"create_if_missing"`
}

// DefaultCommandConfiguration returns the baseline configuration for branch-cd.
func DefaultCommandConfiguration() CommandConfiguration {
	return CommandConfiguration{CreateIfMissing: true}
}

// Sanitize normalizes textual configuration values and repository roots.
func (configuration CommandConfiguration) Sanitize() CommandConfiguration {
	sanitized := configuration
	sanitized.RepositoryRoots = commandConfigurationRepositorySanitizer.Sanitize(configuration.RepositoryRoots)
	sanitized.DefaultBranch = strings.TrimSpace(configuration.DefaultBranch)
	sanitized.RemoteName = strings.TrimSpace(configuration.RemoteName)
	return sanitized
}
