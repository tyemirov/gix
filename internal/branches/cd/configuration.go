package cd

import (
	"strings"

	pathutils "github.com/temirov/gix/internal/utils/path"
)

var commandConfigurationRepositorySanitizer = pathutils.NewRepositoryPathSanitizerWithConfiguration(nil, pathutils.RepositoryPathSanitizerConfiguration{PruneNestedPaths: true})

// CommandConfiguration captures configuration values for the cd command.
type CommandConfiguration struct {
	RepositoryRoots []string `mapstructure:"roots"`
	DefaultBranch   string   `mapstructure:"branch"`
	RemoteName      string   `mapstructure:"remote"`
	CreateIfMissing bool     `mapstructure:"create_if_missing"`
	RefreshEnabled  bool     `mapstructure:"refresh"`
	RequireClean    bool     `mapstructure:"require_clean"`
	StashChanges    bool     `mapstructure:"stash"`
	CommitChanges   bool     `mapstructure:"commit"`
}

// DefaultCommandConfiguration returns the baseline configuration for cd.
func DefaultCommandConfiguration() CommandConfiguration {
	return CommandConfiguration{
		CreateIfMissing: true,
		RequireClean:    true,
	}
}

// Sanitize normalizes textual configuration values and repository roots.
func (configuration CommandConfiguration) Sanitize() CommandConfiguration {
	sanitized := configuration
	sanitized.RepositoryRoots = commandConfigurationRepositorySanitizer.Sanitize(configuration.RepositoryRoots)
	sanitized.DefaultBranch = strings.TrimSpace(configuration.DefaultBranch)
	sanitized.RemoteName = strings.TrimSpace(configuration.RemoteName)
	return sanitized
}
