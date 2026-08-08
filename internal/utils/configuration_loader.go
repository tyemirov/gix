package utils

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	configurationReadErrorTemplateConstant     = "config_file_read_failed: path=%s: %w"
	configurationParseErrorTemplateConstant    = "config_file_parse_failed: path=%s: %w"
	configurationPlaceholderErrorTemplate      = "config_placeholder_missing: names=%s"
	configurationPathRequiredErrorMessage      = "config_file_path_required"
	configurationTargetErrorMessage            = "config_file_target_must_be_non_nil_pointer"
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

	targetValue := reflect.ValueOf(targetConfiguration)
	if !targetValue.IsValid() || targetValue.Kind() != reflect.Pointer || targetValue.IsNil() {
		return LoadedConfiguration{}, fmt.Errorf(configurationParseErrorTemplateConstant, trimmedPath, errors.New(configurationTargetErrorMessage))
	}

	decodedTarget := reflect.New(targetValue.Elem().Type())
	configurationDecoder := yaml.NewDecoder(bytes.NewReader(configurationData))
	configurationDecoder.KnownFields(true)
	if parseError := configurationDecoder.Decode(decodedTarget.Interface()); parseError != nil {
		return LoadedConfiguration{}, fmt.Errorf(configurationParseErrorTemplateConstant, trimmedPath, parseError)
	}

	expansionError := expandConfigurationPlaceholders(decodedTarget, processEnvironment())
	if expansionError != nil {
		return LoadedConfiguration{}, expansionError
	}

	targetValue.Elem().Set(decodedTarget.Elem())

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

func expandConfigurationPlaceholders(configurationValue reflect.Value, environment map[string]string) error {
	missingPlaceholders := map[string]struct{}{}
	expandConfigurationValue(configurationValue, environment, missingPlaceholders, "")
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

func expandConfigurationValue(
	configurationValue reflect.Value,
	environment map[string]string,
	missingPlaceholders map[string]struct{},
	optionalPlaceholderName string,
) {
	if !configurationValue.IsValid() {
		return
	}

	switch configurationValue.Kind() {
	case reflect.Interface:
		if configurationValue.IsNil() || !configurationValue.CanSet() {
			return
		}
		expandedValue := reflect.New(configurationValue.Elem().Type()).Elem()
		expandedValue.Set(configurationValue.Elem())
		expandConfigurationValue(expandedValue, environment, missingPlaceholders, optionalPlaceholderName)
		configurationValue.Set(expandedValue)
	case reflect.Pointer:
		if configurationValue.IsNil() {
			return
		}
		expandConfigurationValue(configurationValue.Elem(), environment, missingPlaceholders, optionalPlaceholderName)
	case reflect.Struct:
		configurationType := configurationValue.Type()
		for fieldIndex := 0; fieldIndex < configurationValue.NumField(); fieldIndex++ {
			fieldDefinition := configurationType.Field(fieldIndex)
			if fieldDefinition.PkgPath != "" {
				continue
			}
			fieldName := configurationYAMLFieldName(fieldDefinition)
			if fieldName == "-" {
				continue
			}
			fieldValue := configurationValue.Field(fieldIndex)
			fieldOptionalPlaceholder := optionalCredentialPlaceholder(fieldName, fieldValue)
			expandConfigurationValue(fieldValue, environment, missingPlaceholders, fieldOptionalPlaceholder)
		}
	case reflect.Map:
		if configurationValue.IsNil() {
			return
		}
		mapIterator := configurationValue.MapRange()
		for mapIterator.Next() {
			mapKey := mapIterator.Key()
			mapValue := mapIterator.Value()
			expandedValue := reflect.New(mapValue.Type()).Elem()
			expandedValue.Set(mapValue)
			mapOptionalPlaceholder := ""
			if mapKey.Kind() == reflect.String {
				mapOptionalPlaceholder = optionalCredentialPlaceholder(mapKey.String(), mapValue)
			}
			expandConfigurationValue(expandedValue, environment, missingPlaceholders, mapOptionalPlaceholder)
			configurationValue.SetMapIndex(mapKey, expandedValue)
		}
	case reflect.Slice, reflect.Array:
		for elementIndex := 0; elementIndex < configurationValue.Len(); elementIndex++ {
			expandConfigurationValue(configurationValue.Index(elementIndex), environment, missingPlaceholders, "")
		}
	case reflect.String:
		if !configurationValue.CanSet() {
			return
		}
		expandedString := configurationPlaceholderPattern.ReplaceAllStringFunc(configurationValue.String(), func(placeholder string) string {
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
		configurationValue.SetString(expandedString)
	}
}

func configurationYAMLFieldName(fieldDefinition reflect.StructField) string {
	fieldName, _, _ := strings.Cut(fieldDefinition.Tag.Get("yaml"), ",")
	if fieldName != "" {
		return fieldName
	}
	return strings.ToLower(fieldDefinition.Name)
}

func optionalCredentialPlaceholder(configurationKey string, configurationValue reflect.Value) string {
	if configurationKey != optionalCredentialConfigurationKeyConstant {
		return ""
	}
	for configurationValue.IsValid() && (configurationValue.Kind() == reflect.Interface || configurationValue.Kind() == reflect.Pointer) {
		if configurationValue.IsNil() {
			return ""
		}
		configurationValue = configurationValue.Elem()
	}
	if !configurationValue.IsValid() || configurationValue.Kind() != reflect.String {
		return ""
	}
	return exactPlaceholderName(configurationValue.String())
}

func exactPlaceholderName(configurationValue string) string {
	matches := configurationPlaceholderPattern.FindStringSubmatch(configurationValue)
	if len(matches) != 2 || matches[0] != configurationValue {
		return ""
	}
	if !configurationPlaceholderNamePattern.MatchString(matches[1]) {
		return ""
	}
	return matches[1]
}
