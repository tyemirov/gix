package release

import (
	"strings"

	rootutils "github.com/tyemirov/gix/internal/utils/roots"
)

const (
	defaultRemoteName = "origin"
)

// CommandConfiguration captures configuration values for repo release operations.
type CommandConfiguration struct {
	RepositoryRoots []string `mapstructure:"roots"`
	RemoteName      string   `mapstructure:"remote"`
	Message         string   `mapstructure:"message"`
}

// DefaultCommandConfiguration returns baseline configuration.
func DefaultCommandConfiguration() CommandConfiguration {
	return CommandConfiguration{RemoteName: defaultRemoteName}
}

// Sanitize trims textual configuration values and normalizes roots.
func (configuration CommandConfiguration) Sanitize() CommandConfiguration {
	sanitized := configuration
	sanitized.RepositoryRoots = rootutils.SanitizeConfigured(configuration.RepositoryRoots)
	sanitized.RemoteName = strings.TrimSpace(configuration.RemoteName)
	sanitized.Message = strings.TrimSpace(configuration.Message)
	return sanitized
}
