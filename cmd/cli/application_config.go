package cli

import (
	"fmt"
	"strings"

	mapstructure "github.com/go-viper/mapstructure/v2"

	"github.com/tyemirov/gix/internal/utils"
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

// ApplicationConfiguration describes the persisted configuration for the CLI entrypoint.
type ApplicationConfiguration struct {
	Common     ApplicationCommonConfiguration      `mapstructure:"common"`
	Operations []ApplicationOperationConfiguration `mapstructure:"operations"`
}

// ApplicationCommonConfiguration stores logging and execution defaults shared across commands.
type ApplicationCommonConfiguration struct {
	LogLevel     string `mapstructure:"log_level"`
	LogFormat    string `mapstructure:"log_format"`
	AssumeYes    bool   `mapstructure:"assume_yes"`
	RequireClean bool   `mapstructure:"require_clean"`
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

// MergeDefaults ensures default operation configurations are available when not overridden.
func (configurations OperationConfigurations) MergeDefaults(defaults OperationConfigurations) OperationConfigurations {
	if len(defaults.entries) == 0 {
		return configurations
	}
	if configurations.entries == nil {
		configurations.entries = map[string]map[string]any{}
	}
	for defaultName, defaultOptions := range defaults.entries {
		if _, exists := configurations.entries[defaultName]; exists {
			continue
		}
		copiedOptions := make(map[string]any, len(defaultOptions))
		for optionKey, optionValue := range defaultOptions {
			copiedOptions[optionKey] = optionValue
		}
		configurations.entries[defaultName] = copiedOptions
	}
	return configurations
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
			continue
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
	})
	if decoderError != nil {
		return decoderError
	}

	return decoder.Decode(options)
}

func normalizeOperationName(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func loadEmbeddedOperationConfigurations() OperationConfigurations {
	configurationData, configurationType := EmbeddedDefaultConfiguration()
	if len(configurationData) == 0 {
		return OperationConfigurations{}
	}

	loader := utils.NewConfigurationLoader(configurationNameConstant, configurationTypeConstant, environmentPrefixConstant, nil)
	loader.SetEmbeddedConfiguration(configurationData, configurationType)

	var configuration ApplicationConfiguration
	if _, err := loader.LoadConfiguration("", nil, &configuration); err != nil {
		return OperationConfigurations{}
	}

	embeddedConfigurations, configurationError := newOperationConfigurations(configuration.Operations)
	if configurationError != nil {
		return OperationConfigurations{}
	}

	return embeddedConfigurations
}
