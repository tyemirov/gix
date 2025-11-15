package workflow

import (
	pathutils "github.com/temirov/gix/internal/utils/path"
)

var workflowConfigurationRepositoryPathSanitizer = pathutils.NewRepositoryPathSanitizerWithConfiguration(nil, pathutils.RepositoryPathSanitizerConfiguration{PruneNestedPaths: true})

// CommandConfiguration captures configuration values for workflow.
type CommandConfiguration struct {
	Roots             []string          `mapstructure:"roots"`
	AssumeYes         bool              `mapstructure:"assume_yes"`
	RequireClean      bool              `mapstructure:"require_clean"`
	RepositoryWorkers int               `mapstructure:"repository_workers"`
	Variables         map[string]string `mapstructure:"variables"`
}

// DefaultCommandConfiguration provides default workflow command settings for workflow.
func DefaultCommandConfiguration() CommandConfiguration {
	return CommandConfiguration{
		AssumeYes:         false,
		RequireClean:      false,
		RepositoryWorkers: 0,
	}
}

// Sanitize normalizes configuration values.
func (configuration CommandConfiguration) Sanitize() CommandConfiguration {
	sanitized := configuration
	sanitized.Roots = workflowConfigurationRepositoryPathSanitizer.Sanitize(configuration.Roots)
	if sanitized.RepositoryWorkers < 0 {
		sanitized.RepositoryWorkers = 0
	}
	return sanitized
}
