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
	optionalCredentialLinePattern       = regexp.MustCompile(
		`^\s*` + optionalCredentialConfigurationKeyConstant + `:\s*(?:"\$\{([A-Za-z_][A-Za-z0-9_]*)\}"|'\$\{([A-Za-z_][A-Za-z0-9_]*)\}'|\$\{([A-Za-z_][A-Za-z0-9_]*)\})\s*(?:#.*)?$`,
	)
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

	expandedConfiguration, expansionError := expandConfigurationPlaceholders(string(configurationData), processEnvironment())
	if expansionError != nil {
		return LoadedConfiguration{}, expansionError
	}

	rawConfiguration := map[string]any{}
	if parseError := yaml.Unmarshal([]byte(expandedConfiguration), &rawConfiguration); parseError != nil {
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

func expandConfigurationPlaceholders(configurationContent string, environment map[string]string) (string, error) {
	missingPlaceholders := map[string]struct{}{}
	var expandedConfiguration strings.Builder
	for _, configurationLine := range strings.SplitAfter(configurationContent, "\n") {
		expandedLine := configurationPlaceholderPattern.ReplaceAllStringFunc(configurationLine, func(placeholder string) string {
			matches := configurationPlaceholderPattern.FindStringSubmatch(placeholder)
			placeholderName := matches[1]
			if !configurationPlaceholderNamePattern.MatchString(placeholderName) {
				missingPlaceholders[placeholderName] = struct{}{}
				return placeholder
			}
			if placeholderValue, exists := environment[placeholderName]; exists {
				return placeholderValue
			}
			if optionalCredentialPlaceholder(configurationLine, placeholderName) {
				return ""
			}
			missingPlaceholders[placeholderName] = struct{}{}
			return placeholder
		})
		expandedConfiguration.WriteString(expandedLine)
	}
	if len(missingPlaceholders) == 0 {
		return expandedConfiguration.String(), nil
	}

	missingNames := make([]string, 0, len(missingPlaceholders))
	for placeholderName := range missingPlaceholders {
		missingNames = append(missingNames, placeholderName)
	}
	sort.Strings(missingNames)
	return "", fmt.Errorf(configurationPlaceholderErrorTemplate, strings.Join(missingNames, ","))
}

func optionalCredentialPlaceholder(configurationLine string, placeholderName string) bool {
	lineMatches := optionalCredentialLinePattern.FindStringSubmatch(strings.TrimRight(configurationLine, "\r\n"))
	if len(lineMatches) == 0 {
		return false
	}
	return lineMatches[1] == placeholderName || lineMatches[2] == placeholderName || lineMatches[3] == placeholderName
}
