package audit

import (
	pathutils "github.com/tyemirov/gix/internal/utils/path"
)

var auditConfigurationRepositoryPathSanitizer = pathutils.NewRepositoryPathSanitizerWithConfiguration(nil, pathutils.RepositoryPathSanitizerConfiguration{PruneNestedPaths: true})

// CommandConfiguration captures persistent settings for the audit command.
type CommandConfiguration struct {
	Roots      []string `mapstructure:"roots"`
	Debug      bool     `mapstructure:"debug"`
	IncludeAll bool     `mapstructure:"all"`
}

// DefaultCommandConfiguration returns baseline configuration values for the audit command.
func DefaultCommandConfiguration() CommandConfiguration {
	return CommandConfiguration{
		Roots:      nil,
		Debug:      false,
		IncludeAll: false,
	}
}

// Sanitize trims whitespace and applies defaults to unset configuration values.
func (configuration CommandConfiguration) Sanitize() CommandConfiguration {
	sanitized := configuration

	sanitized.Roots = auditConfigurationRepositoryPathSanitizer.Sanitize(configuration.Roots)

	return sanitized
}
