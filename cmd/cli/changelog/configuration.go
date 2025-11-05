package changelog

import (
	"strings"

	rootutils "github.com/tyemirov/gix/internal/utils/roots"
)

const (
	defaultAPIKeyEnvironment = "OPENAI_API_KEY"
	defaultModel             = "gpt-4.1-mini"
)

// MessageConfiguration captures configuration values for changelog generation.
type MessageConfiguration struct {
	Roots          []string `mapstructure:"roots"`
	APIKeyEnv      string   `mapstructure:"api_key_env"`
	BaseURL        string   `mapstructure:"base_url"`
	Model          string   `mapstructure:"model"`
	MaxTokens      int      `mapstructure:"max_completion_tokens"`
	Temperature    float64  `mapstructure:"temperature"`
	TimeoutSeconds int      `mapstructure:"timeout_seconds"`
	Version        string   `mapstructure:"version"`
	ReleaseDate    string   `mapstructure:"release_date"`
	SinceReference string   `mapstructure:"since_reference"`
	SinceDate      string   `mapstructure:"since_date"`
}

// DefaultMessageConfiguration provides baseline configuration.
func DefaultMessageConfiguration() MessageConfiguration {
	return MessageConfiguration{
		APIKeyEnv:      defaultAPIKeyEnvironment,
		Model:          defaultModel,
		MaxTokens:      0,
		Temperature:    0,
		TimeoutSeconds: 60,
	}
}

// Sanitize normalizes configuration values.
func (configuration MessageConfiguration) Sanitize() MessageConfiguration {
	sanitized := configuration
	sanitized.Roots = rootutils.SanitizeConfigured(configuration.Roots)

	apiKeyEnv := strings.TrimSpace(configuration.APIKeyEnv)
	if apiKeyEnv == "" {
		apiKeyEnv = defaultAPIKeyEnvironment
	}
	sanitized.APIKeyEnv = apiKeyEnv

	sanitized.BaseURL = strings.TrimSpace(configuration.BaseURL)

	model := strings.TrimSpace(configuration.Model)
	if model == "" {
		model = defaultModel
	}
	sanitized.Model = model

	if configuration.MaxTokens < 0 {
		sanitized.MaxTokens = 0
	}

	if configuration.TimeoutSeconds <= 0 {
		sanitized.TimeoutSeconds = 60
	}

	if configuration.Temperature < 0 {
		sanitized.Temperature = 0
	}

	sanitized.Version = strings.TrimSpace(configuration.Version)
	sanitized.ReleaseDate = strings.TrimSpace(configuration.ReleaseDate)
	sanitized.SinceReference = strings.TrimSpace(configuration.SinceReference)
	sanitized.SinceDate = strings.TrimSpace(configuration.SinceDate)

	return sanitized
}
