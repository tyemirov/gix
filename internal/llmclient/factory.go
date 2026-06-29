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
	providerRequiresProxyTransport   = "llm provider requires llm_proxy transport"
)

// Transport identifies how chat requests are sent.
type Transport string

const (
	// TransportOpenAICompatible sends requests to an OpenAI-compatible chat completions endpoint.
	TransportOpenAICompatible Transport = "openai_compatible"
	// TransportLLMProxy sends requests to the MPR LLM Proxy v2 endpoint.
	TransportLLMProxy Transport = "llm_proxy"
	// DefaultTransport is the embedded transport used when no user configuration overrides it.
	DefaultTransport = TransportOpenAICompatible
)

// Config describes the configured chat client.
type Config struct {
	Transport           Transport
	Provider            string
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

// NewTransport constructs a transport from a configuration value.
func NewTransport(rawValue string) (Transport, error) {
	trimmedValue := strings.TrimSpace(rawValue)
	if trimmedValue == "" {
		return DefaultTransport, nil
	}
	transport := Transport(trimmedValue)
	switch transport {
	case TransportOpenAICompatible, TransportLLMProxy:
		return transport, nil
	default:
		return "", fmt.Errorf("unsupported llm transport %q", trimmedValue)
	}
}

// NormalizeTransportName trims a transport string and applies the embedded default for empty values.
func NormalizeTransportName(rawValue string) string {
	trimmedValue := strings.TrimSpace(rawValue)
	if trimmedValue == "" {
		return string(DefaultTransport)
	}
	return trimmedValue
}

// DefaultAPIKeyEnvironmentForTransportName returns the canonical secret environment variable for a transport.
func DefaultAPIKeyEnvironmentForTransportName(rawTransport string) string {
	if Transport(NormalizeTransportName(rawTransport)) == TransportLLMProxy {
		return DefaultLLMProxyAPIKeyEnvironment
	}
	return DefaultAPIKeyEnvironment
}

// DefaultBaseURLForTransportName returns the canonical endpoint for a transport.
func DefaultBaseURLForTransportName(rawTransport string) string {
	if Transport(NormalizeTransportName(rawTransport)) == TransportLLMProxy {
		return DefaultLLMProxyBaseURL
	}
	return DefaultBaseURL
}

// ValidateProviderForTransport verifies that proxy provider routing is attached to the proxy transport.
func ValidateProviderForTransport(transport Transport, provider string) error {
	if strings.TrimSpace(provider) == "" {
		return nil
	}
	if transport == TransportLLMProxy {
		return nil
	}
	return errors.New(providerRequiresProxyTransport)
}

// NewFactory creates the configured chat client.
func NewFactory(configuration Config) (llm.ChatClient, error) {
	transport, transportError := NewTransport(string(configuration.Transport))
	if transportError != nil {
		return nil, transportError
	}
	if providerError := ValidateProviderForTransport(transport, configuration.Provider); providerError != nil {
		return nil, providerError
	}
	switch transport {
	case TransportLLMProxy:
		return newProxyChatClient(configuration)
	case TransportOpenAICompatible:
		return llm.NewFactory(configuration.toOpenAICompatibleConfig())
	default:
		return nil, fmt.Errorf("unsupported llm transport %q", transport)
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
		BaseURL:  baseURL,
		Secret:   configuration.APIKey,
		Provider: strings.TrimSpace(configuration.Provider),
		Timeout:  timeout,
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
