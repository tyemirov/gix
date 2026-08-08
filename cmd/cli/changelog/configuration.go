package changelog

import (
	"strings"

	"github.com/tyemirov/gix/internal/llmclient"
	rootutils "github.com/tyemirov/gix/internal/utils/roots"
)

// MessageConfiguration captures configuration values for changelog generation.
type MessageConfiguration struct {
	Roots              []string                     `mapstructure:"roots"`
	LLMProxy           llmclient.LLMProxySelection  `mapstructure:"llm_proxy"`
	Effort             string                       `mapstructure:"effort"`
	MaxTokens          int                          `mapstructure:"max_completion_tokens"`
	TimeoutSeconds     int                          `mapstructure:"timeout_seconds"`
	Version            string                       `mapstructure:"version"`
	ReleaseDate        string                       `mapstructure:"release_date"`
	SinceReference     string                       `mapstructure:"since_reference"`
	SinceDate          string                       `mapstructure:"since_date"`
	ConnectionProfiles llmclient.ConnectionProfiles `mapstructure:"-"`
}

// DefaultMessageConfiguration provides baseline configuration.
func DefaultMessageConfiguration() MessageConfiguration {
	return MessageConfiguration{
		MaxTokens: 0,
	}
}

// Sanitize normalizes configuration values.
func (configuration MessageConfiguration) Sanitize() MessageConfiguration {
	sanitized := configuration
	sanitized.Roots = rootutils.SanitizeConfigured(configuration.Roots)

	sanitized.LLMProxy.Provider = strings.TrimSpace(configuration.LLMProxy.Provider)
	sanitized.LLMProxy.Model = strings.TrimSpace(configuration.LLMProxy.Model)
	sanitized.Effort = strings.TrimSpace(configuration.Effort)

	if configuration.MaxTokens < 0 {
		sanitized.MaxTokens = 0
	}

	sanitized.Version = strings.TrimSpace(configuration.Version)
	sanitized.ReleaseDate = strings.TrimSpace(configuration.ReleaseDate)
	sanitized.SinceReference = strings.TrimSpace(configuration.SinceReference)
	sanitized.SinceDate = strings.TrimSpace(configuration.SinceDate)

	return sanitized
}
