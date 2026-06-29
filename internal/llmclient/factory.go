package llmclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tyemirov/llm-proxy/pkg/llmproxyclient"
	"github.com/tyemirov/utils/llm"
)

const (
	DefaultAPIKeyEnvironment         = "OPENAI_API_KEY"
	DefaultBaseURL                   = "https://api.openai.com/v1"
	DefaultLLMProxyAPIKeyEnvironment = "LLM_PROXY_SECRET"
	DefaultLLMProxyBaseURL           = "https://llm-proxy-api.mprlab.com"
	DefaultModel                     = "gpt-4.1"
	defaultRequestTimeout            = 60 * time.Second
)

// Provider identifies the transport used for chat requests.
type Provider string

const (
	// ProviderOpenAICompatible sends requests to an OpenAI-compatible chat completions endpoint.
	ProviderOpenAICompatible Provider = "openai_compatible"
	// ProviderLLMProxy sends requests to the MPR LLM Proxy v2 endpoint.
	ProviderLLMProxy Provider = "llm_proxy"
	// DefaultProvider is the embedded provider used when no user configuration overrides it.
	DefaultProvider = ProviderOpenAICompatible
)

// Config describes the configured chat client.
type Config struct {
	Provider            Provider
	BaseURL             string
	APIKey              string
	Model               string
	MaxCompletionTokens int
	Temperature         float64
	HTTPClient          llm.HTTPClient
	RequestTimeout      time.Duration
	RetryAttempts       int
	RetryInitialBackoff time.Duration
	RetryMaxBackoff     time.Duration
	RetryBackoffFactor  float64
}

type proxyChatClient struct {
	client llmproxyclient.Client
	model  string
}

// NewProvider constructs a provider from a configuration value.
func NewProvider(rawValue string) (Provider, error) {
	trimmedValue := strings.TrimSpace(rawValue)
	if trimmedValue == "" {
		return DefaultProvider, nil
	}
	provider := Provider(trimmedValue)
	switch provider {
	case ProviderOpenAICompatible, ProviderLLMProxy:
		return provider, nil
	default:
		return "", fmt.Errorf("unsupported llm provider %q", trimmedValue)
	}
}

// NormalizeProviderName trims a provider string and applies the embedded default for empty values.
func NormalizeProviderName(rawValue string) string {
	trimmedValue := strings.TrimSpace(rawValue)
	if trimmedValue == "" {
		return string(DefaultProvider)
	}
	return trimmedValue
}

// DefaultAPIKeyEnvironmentForProviderName returns the canonical secret environment variable for a provider.
func DefaultAPIKeyEnvironmentForProviderName(rawProvider string) string {
	if Provider(NormalizeProviderName(rawProvider)) == ProviderLLMProxy {
		return DefaultLLMProxyAPIKeyEnvironment
	}
	return DefaultAPIKeyEnvironment
}

// DefaultBaseURLForProviderName returns the canonical endpoint for a provider.
func DefaultBaseURLForProviderName(rawProvider string) string {
	if Provider(NormalizeProviderName(rawProvider)) == ProviderLLMProxy {
		return DefaultLLMProxyBaseURL
	}
	return DefaultBaseURL
}

// NewFactory creates the configured chat client.
func NewFactory(configuration Config) (llm.ChatClient, error) {
	provider, providerError := NewProvider(string(configuration.Provider))
	if providerError != nil {
		return nil, providerError
	}
	switch provider {
	case ProviderLLMProxy:
		return newProxyChatClient(configuration)
	case ProviderOpenAICompatible:
		return llm.NewFactory(configuration.toOpenAICompatibleConfig())
	default:
		return nil, fmt.Errorf("unsupported llm provider %q", provider)
	}
}

func (configuration Config) toOpenAICompatibleConfig() llm.Config {
	return llm.Config{
		BaseURL:             strings.TrimSpace(configuration.BaseURL),
		APIKey:              configuration.APIKey,
		Model:               configuration.Model,
		MaxCompletionTokens: configuration.MaxCompletionTokens,
		Temperature:         configuration.Temperature,
		HTTPClient:          configuration.HTTPClient,
		RequestTimeout:      configuration.RequestTimeout,
		RetryAttempts:       configuration.RetryAttempts,
		RetryInitialBackoff: configuration.RetryInitialBackoff,
		RetryMaxBackoff:     configuration.RetryMaxBackoff,
		RetryBackoffFactor:  configuration.RetryBackoffFactor,
	}
}

func newProxyChatClient(configuration Config) (llm.ChatClient, error) {
	if configuration.Temperature > 0 {
		return nil, errors.New("llm proxy client does not support temperature")
	}
	timeout := configuration.RequestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	baseURL := strings.TrimSpace(configuration.BaseURL)
	if baseURL == "" {
		baseURL = DefaultLLMProxyBaseURL
	}
	proxyConfiguration, configurationError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL: baseURL,
		Secret:  configuration.APIKey,
		Timeout: timeout,
	})
	if configurationError != nil {
		return nil, fmt.Errorf("initialize llm proxy client: %w", configurationError)
	}
	httpClient := configuration.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	client, clientError := llmproxyclient.NewClient(proxyConfiguration, httpClient)
	if clientError != nil {
		return nil, fmt.Errorf("initialize llm proxy client: %w", clientError)
	}
	return proxyChatClient{
		client: client,
		model:  strings.TrimSpace(configuration.Model),
	}, nil
}

func (client proxyChatClient) Chat(ctx context.Context, request llm.ChatRequest) (string, error) {
	if request.ResponseFormat != nil {
		return "", errors.New("llm proxy client does not support response_format")
	}
	if request.Temperature != nil && *request.Temperature > 0 {
		return "", errors.New("llm proxy client does not support temperature")
	}
	messages := make([]llmproxyclient.MessageInput, 0, len(request.Messages))
	for _, message := range request.Messages {
		messages = append(messages, llmproxyclient.MessageInput{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	model := strings.TrimSpace(request.Model)
	if model == "" {
		model = client.model
	}
	maxTokens := request.MaxTokens
	proxyRequest, requestError := llmproxyclient.NewMessagesRequest(llmproxyclient.MessagesRequestInput{
		Messages:  messages,
		Model:     model,
		WebSearch: false,
		MaxTokens: positiveIntPointer(maxTokens),
	})
	if requestError != nil {
		return "", fmt.Errorf("build llm proxy request: %w", requestError)
	}
	response, responseError := client.client.PostMessages(ctx, proxyRequest)
	if responseError != nil {
		return "", fmt.Errorf("send llm proxy request: %w", responseError)
	}
	return strings.TrimSpace(response), nil
}

func positiveIntPointer(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}
