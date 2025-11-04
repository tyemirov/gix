package branches

import (
	"strings"

	pathutils "github.com/tyemirov/gix/internal/utils/path"
)

var branchConfigurationRepositoryPathSanitizer = pathutils.NewRepositoryPathSanitizerWithConfiguration(nil, pathutils.RepositoryPathSanitizerConfiguration{PruneNestedPaths: true})

// CommandConfiguration captures configuration values for the branch cleanup command.
type CommandConfiguration struct {
	RemoteName       string   `mapstructure:"remote"`
	PullRequestLimit int      `mapstructure:"limit"`
	DryRun           bool     `mapstructure:"dry_run"`
	AssumeYes        bool     `mapstructure:"assume_yes"`
	RepositoryRoots  []string `mapstructure:"roots"`
}

// DefaultCommandConfiguration provides baseline configuration values for branch cleanup.
func DefaultCommandConfiguration() CommandConfiguration {
	return CommandConfiguration{
		RemoteName:       "",
		PullRequestLimit: 0,
		DryRun:           false,
		AssumeYes:        false,
		RepositoryRoots:  nil,
	}
}

// Sanitize trims configuration values without applying implicit defaults.
func (configuration CommandConfiguration) Sanitize() CommandConfiguration {
	sanitized := configuration

	sanitized.RemoteName = strings.TrimSpace(configuration.RemoteName)
	sanitized.RepositoryRoots = branchConfigurationRepositoryPathSanitizer.Sanitize(configuration.RepositoryRoots)

	return sanitized
}
