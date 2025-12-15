package workflow

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tyemirov/utils/llm"
)

const (
	optionTaskLLMKeyConstant            = "llm"
	optionTaskLLMModelKeyConstant       = "model"
	optionTaskLLMBaseURLKeyConstant     = "base_url"
	optionTaskLLMAPIKeyEnvKeyConstant   = "api_key_env"
	optionTaskLLMTimeoutKeyConstant     = "timeout_seconds"
	optionTaskLLMMaxTokensKeyConstant   = "max_completion_tokens"
	optionTaskLLMTemperatureKeyConstant = "temperature"
)

// TaskLLMClientConfiguration describes the client parameters for workflow task actions.
type TaskLLMClientConfiguration struct {
	baseURL             string
	model               string
	apiKeyEnv           string
	maxCompletionTokens int
	temperature         float64
	hasTemperature      bool
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

	configReader := newOptionReader(rawConfiguration)

	model, modelExists, modelErr := configReader.stringValue(optionTaskLLMModelKeyConstant)
	if modelErr != nil {
		return nil, modelErr
	}
	if !modelExists || model == "" {
		return nil, errors.New("llm configuration requires model")
	}

	baseURL, _, baseURLErr := configReader.stringValue(optionTaskLLMBaseURLKeyConstant)
	if baseURLErr != nil {
		return nil, baseURLErr
	}

	apiKeyEnv, apiKeyExists, apiKeyErr := configReader.stringValue(optionTaskLLMAPIKeyEnvKeyConstant)
	if apiKeyErr != nil {
		return nil, apiKeyErr
	}
	if !apiKeyExists || apiKeyEnv == "" {
		return nil, errors.New("llm configuration requires api_key_env")
	}

	timeout, timeoutErr := parseOptionalDurationSeconds(rawConfiguration[optionTaskLLMTimeoutKeyConstant])
	if timeoutErr != nil {
		return nil, timeoutErr
	}

	maxTokens, maxTokensErr := parseOptionalInt(rawConfiguration[optionTaskLLMMaxTokensKeyConstant])
	if maxTokensErr != nil {
		return nil, maxTokensErr
	}

	temperature, hasTemperature, temperatureErr := parseOptionalFloat(rawConfiguration[optionTaskLLMTemperatureKeyConstant])
	if temperatureErr != nil {
		return nil, temperatureErr
	}

	return &TaskLLMClientConfiguration{
		baseURL:             strings.TrimSpace(baseURL),
		model:               model,
		apiKeyEnv:           apiKeyEnv,
		maxCompletionTokens: maxTokens,
		temperature:         temperature,
		hasTemperature:      hasTemperature,
		timeout:             timeout,
	}, nil
}

// Client returns a cached LLM client configured from the workflow options.
func (configuration *TaskLLMClientConfiguration) Client() (llm.ChatClient, error) {
	if configuration == nil {
		return nil, errors.New("llm client configuration is not available")
	}

	configuration.clientOnce.Do(func() {
		apiKey := strings.TrimSpace(os.Getenv(configuration.apiKeyEnv))
		if apiKey == "" {
			configuration.clientErr = fmt.Errorf("llm api key env %s is empty", configuration.apiKeyEnv)
			return
		}

		clientConfiguration := llm.Config{
			BaseURL:        configuration.baseURL,
			APIKey:         apiKey,
			Model:          configuration.model,
			RequestTimeout: configuration.timeout,
		}
		if configuration.maxCompletionTokens > 0 {
			clientConfiguration.MaxCompletionTokens = configuration.maxCompletionTokens
		}
		if configuration.hasTemperature {
			clientConfiguration.Temperature = configuration.temperature
		}

		client, clientErr := llm.NewFactory(clientConfiguration)
		if clientErr != nil {
			configuration.clientErr = clientErr
			return
		}
		configuration.client = client
	})

	return configuration.client, configuration.clientErr
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

func parseOptionalFloat(raw any) (float64, bool, error) {
	if raw == nil {
		return 0, false, nil
	}
	value, err := parseFloat(raw, optionTaskLLMTemperatureKeyConstant)
	if err != nil {
		return 0, false, err
	}
	return value, true, nil
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
