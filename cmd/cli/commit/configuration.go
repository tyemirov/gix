package commit

import (
	"strings"

	"github.com/tyemirov/gix/v5/internal/llmclient"
	rootutils "github.com/tyemirov/gix/v5/internal/utils/roots"
)

const defaultDiffSource = "staged"

// MessageConfiguration captures configuration values for commit message generation.
type MessageConfiguration struct {
	Roots              []string                     `mapstructure:"roots"`
	LLMProxy           llmclient.LLMProxySelection  `mapstructure:"llm_proxy"`
	Effort             string                       `mapstructure:"effort"`
	MaxTokens          int                          `mapstructure:"max_completion_tokens"`
	DiffSource         string                       `mapstructure:"diff_source"`
	TimeoutSeconds     int                          `mapstructure:"timeout_seconds"`
	ConnectionProfiles llmclient.ConnectionProfiles `mapstructure:"-"`
}

// DefaultMessageConfiguration provides baseline configuration.
func DefaultMessageConfiguration() MessageConfiguration {
	return MessageConfiguration{
		DiffSource: defaultDiffSource,
	}
}

// Sanitize normalizes configuration values.
func (configuration MessageConfiguration) Sanitize() MessageConfiguration {
	sanitized := configuration
	sanitized.Roots = rootutils.SanitizeConfigured(configuration.Roots)

	sanitized.LLMProxy.Provider = strings.TrimSpace(configuration.LLMProxy.Provider)
	sanitized.LLMProxy.Model = strings.TrimSpace(configuration.LLMProxy.Model)

	diffSource := strings.ToLower(strings.TrimSpace(configuration.DiffSource))
	if diffSource == "" {
		diffSource = defaultDiffSource
	}
	sanitized.DiffSource = diffSource

	if configuration.MaxTokens < 0 {
		sanitized.MaxTokens = 0
	}

	return sanitized
}
