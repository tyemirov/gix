package workflow

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tyemirov/gix/v5/internal/llmclient"
	"github.com/tyemirov/utils/llm"
)

const (
	optionTaskLLMKeyConstant          = "llm"
	optionTaskLLMProxyKeyConstant     = "llm_proxy"
	optionTaskLLMProviderKeyConstant  = "provider"
	optionTaskLLMModelKeyConstant     = "model"
	optionTaskLLMTimeoutKeyConstant   = "timeout_seconds"
	optionTaskLLMMaxTokensKeyConstant = "max_completion_tokens"
	optionTaskLLMEffortKeyConstant    = "effort"
)

var supportedTaskLLMConfigurationKeys = map[string]struct{}{
	optionTaskLLMProxyKeyConstant:     {},
	optionTaskLLMTimeoutKeyConstant:   {},
	optionTaskLLMMaxTokensKeyConstant: {},
	optionTaskLLMEffortKeyConstant:    {},
}

var supportedTaskLLMProxyConfigurationKeys = map[string]struct{}{
	optionTaskLLMProviderKeyConstant: {},
	optionTaskLLMModelKeyConstant:    {},
}

// TaskLLMClientConfiguration describes the client parameters for workflow task actions.
type TaskLLMClientConfiguration struct {
	llmProxy            llmclient.LLMProxySelection
	connectionProfiles  llmclient.ConnectionProfiles
	effort              string
	maxCompletionTokens int
	timeout             time.Duration

	clientOnce sync.Once
	client     llm.ChatClient
	clientErr  error
}

func buildTaskLLMConfiguration(reader optionReader) (*TaskLLMClientConfiguration, error) {
	rawConfiguration, exists, err := reader.mapValue(optionTaskLLMKeyConstant)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	if keyError := validateTaskLLMConfigurationKeys(rawConfiguration); keyError != nil {
		return nil, keyError
	}

	configReader := newOptionReader(rawConfiguration)
	rawProxyConfiguration, proxyExists, proxyConfigurationError := configReader.mapValue(optionTaskLLMProxyKeyConstant)
	if proxyConfigurationError != nil {
		return nil, proxyConfigurationError
	}
	if !proxyExists {
		_, providerError := llmclient.NormalizeProvider("")
		return nil, providerError
	}
	if proxyKeyError := validateTaskLLMProxyConfigurationKeys(rawProxyConfiguration); proxyKeyError != nil {
		return nil, proxyKeyError
	}
	proxyReader := newOptionReader(rawProxyConfiguration)

	providerName, _, providerErr := proxyReader.stringValue(optionTaskLLMProviderKeyConstant)
	if providerErr != nil {
		return nil, providerErr
	}
	providerName, providerNormalizationError := llmclient.NormalizeProvider(providerName)
	if providerNormalizationError != nil {
		return nil, providerNormalizationError
	}

	model, _, modelErr := proxyReader.stringValue(optionTaskLLMModelKeyConstant)
	if modelErr != nil {
		return nil, modelErr
	}

	timeout, timeoutErr := parseOptionalDurationSeconds(rawConfiguration[optionTaskLLMTimeoutKeyConstant])
	if timeoutErr != nil {
		return nil, timeoutErr
	}

	maxTokens, maxTokensErr := parseOptionalInt(rawConfiguration[optionTaskLLMMaxTokensKeyConstant])
	if maxTokensErr != nil {
		return nil, maxTokensErr
	}

	effort, _, effortErr := configReader.stringValue(optionTaskLLMEffortKeyConstant)
	if effortErr != nil {
		return nil, effortErr
	}

	return &TaskLLMClientConfiguration{
		llmProxy:            llmclient.LLMProxySelection{Provider: providerName, Model: model},
		effort:              effort,
		maxCompletionTokens: maxTokens,
		timeout:             timeout,
	}, nil
}

func validateTaskLLMProxyConfigurationKeys(rawConfiguration map[string]any) error {
	return validateTaskLLMKeys(rawConfiguration, supportedTaskLLMProxyConfigurationKeys)
}

func validateTaskLLMConfigurationKeys(rawConfiguration map[string]any) error {
	return validateTaskLLMKeys(rawConfiguration, supportedTaskLLMConfigurationKeys)
}

func validateTaskLLMKeys(rawConfiguration map[string]any, supportedKeys map[string]struct{}) error {
	unsupportedKeys := make([]string, 0)
	for rawKey := range rawConfiguration {
		normalizedKey := strings.ToLower(strings.TrimSpace(rawKey))
		if _, supported := supportedKeys[normalizedKey]; supported {
			continue
		}
		unsupportedKeys = append(unsupportedKeys, normalizedKey)
	}
	if len(unsupportedKeys) == 0 {
		return nil
	}
	sort.Strings(unsupportedKeys)
	return fmt.Errorf("unsupported llm configuration key %q", unsupportedKeys[0])
}

// Client returns a cached LLM client configured from the workflow options.
func (configuration *TaskLLMClientConfiguration) Client() (llm.ChatClient, error) {
	if configuration == nil {
		return nil, errors.New("llm client configuration is not available")
	}

	configuration.clientOnce.Do(func() {
		runtimeConfiguration := llmclient.RuntimeConfig{
			Effort:         configuration.effort,
			RequestTimeout: configuration.timeout,
		}
		if configuration.maxCompletionTokens > 0 {
			runtimeConfiguration.MaxCompletionTokens = configuration.maxCompletionTokens
		}

		client, clientErr := llmclient.NewPrioritizedFactory(
			configuration.connectionProfiles,
			configuration.llmProxy,
			runtimeConfiguration,
			nil,
		)
		if clientErr != nil {
			configuration.clientErr = clientErr
			return
		}
		configuration.client = client
	})

	return configuration.client, configuration.clientErr
}

func (configuration *TaskLLMClientConfiguration) setConnectionProfiles(profiles llmclient.ConnectionProfiles) {
	if configuration == nil {
		return
	}
	configuration.connectionProfiles = profiles
}

func parseOptionalDurationSeconds(raw any) (time.Duration, error) {
	if raw == nil {
		return 0, nil
	}

	seconds, err := parseFloat(raw, optionTaskLLMTimeoutKeyConstant)
	if err != nil {
		return 0, err
	}
	if seconds < 0 {
		return 0, fmt.Errorf("%s must be non-negative", optionTaskLLMTimeoutKeyConstant)
	}
	if math.Mod(seconds, 1) != 0 {
		return 0, fmt.Errorf("%s must be a whole number of seconds", optionTaskLLMTimeoutKeyConstant)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func parseOptionalInt(raw any) (int, error) {
	if raw == nil {
		return 0, nil
	}
	value, err := parseFloat(raw, optionTaskLLMMaxTokensKeyConstant)
	if err != nil {
		return 0, err
	}
	if value < 0 {
		return 0, fmt.Errorf("%s must be non-negative", optionTaskLLMMaxTokensKeyConstant)
	}
	return int(value), nil
}

func parseFloat(raw any, key string) (float64, error) {
	switch typed := raw.(type) {
	case int:
		return float64(typed), nil
	case int32:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	case float32:
		return float64(typed), nil
	case float64:
		return typed, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, fmt.Errorf("%s cannot be empty", key)
		}
		value, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, fmt.Errorf("%s must be numeric", key)
		}
		return value, nil
	default:
		return 0, fmt.Errorf("%s must be numeric", key)
	}
}
