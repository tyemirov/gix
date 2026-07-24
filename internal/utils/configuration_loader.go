package utils

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	mapstructure "github.com/go-viper/mapstructure/v2"
	"gopkg.in/yaml.v3"
)

const (
	configurationReadErrorTemplateConstant     = "config_file_read_failed: path=%s: %v"
	configurationParseErrorTemplateConstant    = "config_file_parse_failed: path=%s: %v"
	configurationPlaceholderErrorTemplate      = "config_placeholder_missing: names=%s"
	configurationPathRequiredErrorMessage      = "config_file_path_required"
	optionalCredentialConfigurationKeyConstant = "credential"
)

var (
	configurationPlaceholderPattern     = regexp.MustCompile(`\$\{([^}]+)\}`)
	configurationPlaceholderNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// ConfigurationLoader loads one strict YAML configuration file.
type ConfigurationLoader struct{}

// LoadedConfiguration surfaces metadata about the resolved configuration.
type LoadedConfiguration struct {
	ConfigFileUsed string
}

// NewConfigurationLoader creates a strict file-only configuration loader.
func NewConfigurationLoader() *ConfigurationLoader {
	return &ConfigurationLoader{}
}

// LoadConfiguration expands file placeholders and decodes one exact configuration schema.
func (loader *ConfigurationLoader) LoadConfiguration(configurationFilePath string, targetConfiguration any) (LoadedConfiguration, error) {
	trimmedPath := strings.TrimSpace(configurationFilePath)
	if trimmedPath == "" {
		return LoadedConfiguration{}, errors.New(configurationPathRequiredErrorMessage)
	}

	configurationData, readError := os.ReadFile(trimmedPath)
	if readError != nil {
		return LoadedConfiguration{}, fmt.Errorf(configurationReadErrorTemplateConstant, trimmedPath, readError)
	}

	var configurationDocument yaml.Node
	if parseError := yaml.Unmarshal(configurationData, &configurationDocument); parseError != nil {
		return LoadedConfiguration{}, fmt.Errorf(configurationParseErrorTemplateConstant, trimmedPath, parseError)
	}

	expansionError := expandConfigurationPlaceholders(&configurationDocument, processEnvironment())
	if expansionError != nil {
		return LoadedConfiguration{}, expansionError
	}

	rawConfiguration := map[string]any{}
	if parseError := configurationDocument.Decode(&rawConfiguration); parseError != nil {
		return LoadedConfiguration{}, fmt.Errorf(configurationParseErrorTemplateConstant, trimmedPath, parseError)
	}
	decoder, decoderError := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          "mapstructure",
		Result:           targetConfiguration,
		ErrorUnused:      true,
		WeaklyTypedInput: true,
	})
	if decoderError != nil {
		return LoadedConfiguration{}, fmt.Errorf(configurationParseErrorTemplateConstant, trimmedPath, decoderError)
	}
	if decodeError := decoder.Decode(rawConfiguration); decodeError != nil {
		return LoadedConfiguration{}, fmt.Errorf(configurationParseErrorTemplateConstant, trimmedPath, decodeError)
	}

	return LoadedConfiguration{ConfigFileUsed: trimmedPath}, nil
}

func processEnvironment() map[string]string {
	environment := map[string]string{}
	for _, assignment := range os.Environ() {
		variableName, variableValue, _ := strings.Cut(assignment, "=")
		environment[variableName] = variableValue
	}
	return environment
}

func expandConfigurationPlaceholders(configurationDocument *yaml.Node, environment map[string]string) error {
	missingPlaceholders := map[string]struct{}{}
	expandConfigurationNode(configurationDocument, environment, missingPlaceholders, "")
	if len(missingPlaceholders) == 0 {
		return nil
	}

	missingNames := make([]string, 0, len(missingPlaceholders))
	for placeholderName := range missingPlaceholders {
		missingNames = append(missingNames, placeholderName)
	}
	sort.Strings(missingNames)
	return fmt.Errorf(configurationPlaceholderErrorTemplate, strings.Join(missingNames, ","))
}

func expandConfigurationNode(
	configurationNode *yaml.Node,
	environment map[string]string,
	missingPlaceholders map[string]struct{},
	optionalPlaceholderName string,
) {
	if configurationNode == nil {
		return
	}

	switch configurationNode.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, childNode := range configurationNode.Content {
			expandConfigurationNode(childNode, environment, missingPlaceholders, "")
		}
	case yaml.MappingNode:
		for childIndex := 0; childIndex+1 < len(configurationNode.Content); childIndex += 2 {
			keyNode := configurationNode.Content[childIndex]
			valueNode := configurationNode.Content[childIndex+1]
			optionalCredentialName := ""
			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == optionalCredentialConfigurationKeyConstant {
				optionalCredentialName = exactPlaceholderName(valueNode)
			}
			expandConfigurationNode(valueNode, environment, missingPlaceholders, optionalCredentialName)
		}
	case yaml.ScalarNode:
		configurationNode.Value = configurationPlaceholderPattern.ReplaceAllStringFunc(configurationNode.Value, func(placeholder string) string {
			matches := configurationPlaceholderPattern.FindStringSubmatch(placeholder)
			placeholderName := matches[1]
			if !configurationPlaceholderNamePattern.MatchString(placeholderName) {
				missingPlaceholders[placeholderName] = struct{}{}
				return placeholder
			}
			if placeholderValue, exists := environment[placeholderName]; exists {
				return placeholderValue
			}
			if placeholderName == optionalPlaceholderName {
				return ""
			}
			missingPlaceholders[placeholderName] = struct{}{}
			return placeholder
		})
	}
}

func exactPlaceholderName(configurationNode *yaml.Node) string {
	if configurationNode == nil || configurationNode.Kind != yaml.ScalarNode {
		return ""
	}
	matches := configurationPlaceholderPattern.FindStringSubmatch(configurationNode.Value)
	if len(matches) != 2 || matches[0] != configurationNode.Value {
		return ""
	}
	if !configurationPlaceholderNamePattern.MatchString(matches[1]) {
		return ""
	}
	return matches[1]
}
