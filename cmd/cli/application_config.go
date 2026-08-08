package cli

import (
	"errors"
	"fmt"
	"strings"

	mapstructure "github.com/go-viper/mapstructure/v2"

	"github.com/tyemirov/gix/internal/llmclient"
	workflowpkg "github.com/tyemirov/gix/internal/workflow"
)

// DuplicateOperationConfigurationError indicates that the configuration file defines the same operation multiple times.
type DuplicateOperationConfigurationError struct {
	OperationName string
}

// Error implements the error interface.
func (errorDetails DuplicateOperationConfigurationError) Error() string {
	return fmt.Sprintf(duplicateOperationConfigurationTemplateConstant, errorDetails.OperationName)
}

// MissingOperationConfigurationError indicates that a referenced operation configuration is absent.
type MissingOperationConfigurationError struct {
	OperationName string
}

// Error implements the error interface.
func (errorDetails MissingOperationConfigurationError) Error() string {
	return fmt.Sprintf(missingOperationConfigurationTemplateConstant, errorDetails.OperationName)
}

// InvalidOperationConfigurationError indicates that an operation contains unsupported configuration.
type InvalidOperationConfigurationError struct {
	OperationName string
	Cause         error
}

// Error implements the error interface.
func (errorDetails InvalidOperationConfigurationError) Error() string {
	return fmt.Sprintf(invalidOperationConfigurationTemplateConstant, errorDetails.OperationName, errorDetails.Cause)
}

// Unwrap exposes the underlying schema error.
func (errorDetails InvalidOperationConfigurationError) Unwrap() error {
	return errorDetails.Cause
}

// ApplicationConfiguration describes the persisted configuration for the CLI entrypoint.
type ApplicationConfiguration struct {
	Common     ApplicationCommonConfiguration      `mapstructure:"common"`
	GitHub     ApplicationGitHubConfiguration      `mapstructure:"github"`
	LLM        ApplicationLLMConfiguration         `mapstructure:"llm"`
	Operations []ApplicationOperationConfiguration `mapstructure:"operations"`
	Workflow   []ApplicationWorkflowStep           `mapstructure:"workflow"`
}

// ApplicationWorkflowStep wraps one typed workflow step from the shared configuration file.
type ApplicationWorkflowStep struct {
	Step workflowpkg.StepConfiguration `mapstructure:"step"`
}

// ApplicationGitHubConfiguration stores the concrete credential used by GitHub CLI operations.
type ApplicationGitHubConfiguration struct {
	Credential string `mapstructure:"credential"`
}

// ApplicationCommonConfiguration stores logging and execution defaults shared across commands.
type ApplicationCommonConfiguration struct {
	LogLevel     string `mapstructure:"log_level"`
	LogFormat    string `mapstructure:"log_format"`
	AssumeYes    bool   `mapstructure:"assume_yes"`
	RequireClean bool   `mapstructure:"require_clean"`
}

// ApplicationLLMConfiguration stores language-model defaults shared across LLM-backed commands.
type ApplicationLLMConfiguration struct {
	OpenAI              llmclient.OpenAIConnectionProfile   `mapstructure:"openai"`
	LLMProxy            llmclient.LLMProxyConnectionProfile `mapstructure:"llm_proxy"`
	Effort              string                              `mapstructure:"effort"`
	MaxCompletionTokens int                                 `mapstructure:"max_completion_tokens"`
	TimeoutSeconds      int                                 `mapstructure:"timeout_seconds"`
}

func (configuration ApplicationLLMConfiguration) connectionProfiles() llmclient.ConnectionProfiles {
	profiles := llmclient.ConnectionProfiles{
		OpenAI:   configuration.OpenAI,
		LLMProxy: configuration.LLMProxy,
	}
	if profiles.OpenAI.MaxCompletionTokens == 0 {
		profiles.OpenAI.MaxCompletionTokens = configuration.MaxCompletionTokens
	}
	if profiles.LLMProxy.MaxCompletionTokens == 0 {
		profiles.LLMProxy.MaxCompletionTokens = configuration.MaxCompletionTokens
	}
	return profiles
}

func (configuration ApplicationLLMConfiguration) validateConnections() error {
	if configuration.MaxCompletionTokens < 0 {
		return errors.New("llm max_completion_tokens must be non-negative")
	}
	return configuration.connectionProfiles().Validate()
}

// ApplicationOperationConfiguration captures reusable operation defaults from the configuration file.
type ApplicationOperationConfiguration struct {
	Command []string       `mapstructure:"command"`
	Options map[string]any `mapstructure:"with"`
}

// OperationConfigurations stores reusable operation defaults indexed by normalized operation name.
type OperationConfigurations struct {
	entries map[string]map[string]any
}

// Clone returns an independent copy of the operation configuration index.
func (configurations OperationConfigurations) Clone() OperationConfigurations {
	if len(configurations.entries) == 0 {
		return OperationConfigurations{}
	}
	copiedEntries := make(map[string]map[string]any, len(configurations.entries))
	for operationName, operationOptions := range configurations.entries {
		copiedOptions := make(map[string]any, len(operationOptions))
		for optionKey, optionValue := range operationOptions {
			copiedOptions[optionKey] = optionValue
		}
		copiedEntries[operationName] = copiedOptions
	}
	return OperationConfigurations{entries: copiedEntries}
}

type configurationInitializationPlan struct {
	DirectoryPath string
	FilePath      string
}

func newOperationConfigurations(definitions []ApplicationOperationConfiguration) (OperationConfigurations, error) {
	entries := make(map[string]map[string]any)
	seenOperations := make(map[string]struct{})
	for definitionIndex := range definitions {
		normalizedName := workflowpkg.CommandPathKey(definitions[definitionIndex].Command)
		if len(normalizedName) == 0 {
			return OperationConfigurations{}, InvalidOperationConfigurationError{
				Cause: errors.New(operationConfigurationCommandRequiredMessageConstant),
			}
		}

		if _, exists := seenOperations[normalizedName]; exists {
			return OperationConfigurations{}, DuplicateOperationConfigurationError{OperationName: normalizedName}
		}
		seenOperations[normalizedName] = struct{}{}

		options := make(map[string]any)
		for optionKey, optionValue := range definitions[definitionIndex].Options {
			options[optionKey] = optionValue
		}

		entries[normalizedName] = options
	}

	return OperationConfigurations{entries: entries}, nil
}

// Lookup returns the configuration options for the provided operation name or an error if the configuration is absent.
func (configurations OperationConfigurations) Lookup(operationName string) (map[string]any, error) {
	normalizedName := normalizeOperationName(operationName)
	if len(normalizedName) == 0 {
		return nil, MissingOperationConfigurationError{OperationName: operationName}
	}

	if configurations.entries == nil {
		return nil, MissingOperationConfigurationError{OperationName: normalizedName}
	}

	options, exists := configurations.entries[normalizedName]
	if !exists {
		return nil, MissingOperationConfigurationError{OperationName: normalizedName}
	}

	duplicatedOptions := make(map[string]any, len(options))
	for optionKey, optionValue := range options {
		duplicatedOptions[optionKey] = optionValue
	}

	return duplicatedOptions, nil
}

func (configurations OperationConfigurations) decode(operationName string, target any) error {
	if target == nil {
		return nil
	}

	options, lookupError := configurations.Lookup(operationName)
	if lookupError != nil {
		return lookupError
	}

	if len(options) == 0 {
		return nil
	}

	decoder, decoderError := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          "mapstructure",
		Result:           target,
		WeaklyTypedInput: true,
		ErrorUnused:      true,
	})
	if decoderError != nil {
		return decoderError
	}

	return decoder.Decode(options)
}

func normalizeOperationName(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}
