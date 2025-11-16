package workflow

import (
	pathutils "github.com/temirov/gix/internal/utils/path"
)

var workflowConfigurationRepositoryPathSanitizer = pathutils.NewRepositoryPathSanitizerWithConfiguration(nil, pathutils.RepositoryPathSanitizerConfiguration{PruneNestedPaths: true})

// CommandConfiguration captures configuration values for workflow.
type CommandConfiguration struct {
	Roots           []string          `mapstructure:"roots"`
	AssumeYes       bool              `mapstructure:"assume_yes"`
	RequireClean    bool              `mapstructure:"require_clean"`
	WorkflowWorkers int               `mapstructure:"workflow_workers"`
	Variables       map[string]string `mapstructure:"variables"`
}

// DefaultCommandConfiguration provides default workflow command settings for workflow.
func DefaultCommandConfiguration() CommandConfiguration {
	return CommandConfiguration{
		AssumeYes:       false,
		RequireClean:    false,
		WorkflowWorkers: 1,
	}
}

// Sanitize normalizes configuration values.
func (configuration CommandConfiguration) Sanitize() CommandConfiguration {
	sanitized := configuration
	sanitized.Roots = workflowConfigurationRepositoryPathSanitizer.Sanitize(configuration.Roots)
	if sanitized.WorkflowWorkers < 1 {
		sanitized.WorkflowWorkers = 1
	}
	return sanitized
}
