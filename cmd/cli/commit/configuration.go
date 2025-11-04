package commit

import (
	"strings"

	rootutils "github.com/tyemirov/gix/internal/utils/roots"
)

const (
	defaultAPIKeyEnvironment = "OPENAI_API_KEY"
	defaultModel             = "gpt-4.1-mini"
	defaultDiffSource        = "staged"
)

// MessageConfiguration captures configuration values for commit message generation.
type MessageConfiguration struct {
	Roots          []string `mapstructure:"roots"`
	APIKeyEnv      string   `mapstructure:"api_key_env"`
	BaseURL        string   `mapstructure:"base_url"`
	Model          string   `mapstructure:"model"`
	MaxTokens      int      `mapstructure:"max_completion_tokens"`
	Temperature    float64  `mapstructure:"temperature"`
	DiffSource     string   `mapstructure:"diff_source"`
	TimeoutSeconds int      `mapstructure:"timeout_seconds"`
}

// DefaultMessageConfiguration provides baseline configuration.
func DefaultMessageConfiguration() MessageConfiguration {
	return MessageConfiguration{
		APIKeyEnv:      defaultAPIKeyEnvironment,
		Model:          defaultModel,
		DiffSource:     defaultDiffSource,
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

	diffSource := strings.ToLower(strings.TrimSpace(configuration.DiffSource))
	if diffSource == "" {
		diffSource = defaultDiffSource
	}
	sanitized.DiffSource = diffSource

	if configuration.MaxTokens < 0 {
		sanitized.MaxTokens = 0
	}

	if configuration.TimeoutSeconds <= 0 {
		sanitized.TimeoutSeconds = 60
	}

	return sanitized
}
