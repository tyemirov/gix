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
	DefaultOpenAIModel        = "gpt-4.1"
	ProviderOpenAI            = "openai"
	defaultRequestTimeout     = 60 * time.Second
	transportRequiredMessage  = "llm internal transport is required"
	providerRequiredMessage   = "llm llm_proxy provider is required"
	baseURLRequiredMessage    = "llm base_url is required"
	credentialRequiredMessage = "llm credential is required"
	priorityRequiredMessage   = "llm connection priority must be positive"
	priorityUniqueMessage     = "llm connection priorities must be unique"
	connectionRequiredMessage = "llm requires at least one connection credential"
	connectionOpenAI          = "openai"
	connectionLLMProxy        = "llm_proxy"
)

// Transport identifies how chat requests are sent.
type Transport string

const (
	// TransportOpenAICompatible sends requests to an OpenAI-compatible chat completions endpoint.
	TransportOpenAICompatible Transport = "openai_compatible"
	// TransportLLMProxy sends requests to the MPR LLM Proxy v2 endpoint.
	TransportLLMProxy Transport = "llm_proxy"
)

// OpenAIConnectionProfile stores the direct OpenAI connection defined in config.yml.
type OpenAIConnectionProfile struct {
	Priority   int    `mapstructure:"priority"`
	BaseURL    string `mapstructure:"base_url"`
	Credential string `mapstructure:"credential"`
	Model      string `mapstructure:"model"`
}

// LLMProxyConnectionProfile stores the llm-proxy connection and its upstream selection.
type LLMProxyConnectionProfile struct {
	Priority   int    `mapstructure:"priority"`
	BaseURL    string `mapstructure:"base_url"`
	Credential string `mapstructure:"credential"`
	Provider   string `mapstructure:"provider"`
	Model      string `mapstructure:"model"`
}

// ConnectionProfiles stores the ordered LLM connection candidates.
type ConnectionProfiles struct {
	OpenAI   OpenAIConnectionProfile   `mapstructure:"openai"`
	LLMProxy LLMProxyConnectionProfile `mapstructure:"llm_proxy"`
}

// LLMProxySelection overrides the configured llm-proxy upstream for one operation.
type LLMProxySelection struct {
	Provider string `mapstructure:"provider"`
	Model    string `mapstructure:"model"`
}

// RuntimeConfig stores request behavior shared by every connection candidate.
type RuntimeConfig struct {
	MaxCompletionTokens int
	Temperature         float64
	HTTPClient          llm.HTTPClient
	RequestTimeout      time.Duration
	RetryAttempts       int
	RetryInitialBackoff time.Duration
	RetryMaxBackoff     time.Duration
	RetryBackoffFactor  float64
}

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

// ClientFactory constructs one internal connection client.
type ClientFactory func(Config) (llm.ChatClient, error)

type proxyChatClient struct {
	client                llmproxyclient.Client
	model                 string
	requestTimeoutSeconds int
}

type prioritizedClientCandidate struct {
	name   string
	client llm.ChatClient
}

type prioritizedChatClient struct {
	candidates []prioritizedClientCandidate
}

type prioritizedConfigCandidate struct {
	name     string
	priority int
	config   Config
}

func parseInternalTransport(rawValue string) (Transport, error) {
	trimmedValue := strings.TrimSpace(rawValue)
	if trimmedValue == "" {
		return "", errors.New(transportRequiredMessage)
	}
	transport := Transport(trimmedValue)
	switch transport {
	case TransportOpenAICompatible, TransportLLMProxy:
		return transport, nil
	default:
		return "", fmt.Errorf("unsupported llm transport %q", trimmedValue)
	}
}

// NormalizeProvider validates and normalizes one public provider selector.
func NormalizeProvider(rawProvider string) (string, error) {
	normalizedProvider := strings.ToLower(strings.TrimSpace(rawProvider))
	if normalizedProvider == "" {
		return "", errors.New(providerRequiredMessage)
	}
	return normalizedProvider, nil
}

// Validate checks the complete connection schema before command execution.
func (profiles ConnectionProfiles) Validate() error {
	_, configurationError := profiles.orderedConfigurations(LLMProxySelection{}, RuntimeConfig{})
	return configurationError
}

// NewPrioritizedFactory builds one client that attempts configured connections in priority order.
func NewPrioritizedFactory(
	profiles ConnectionProfiles,
	selection LLMProxySelection,
	runtimeConfiguration RuntimeConfig,
	clientFactory ClientFactory,
) (llm.ChatClient, error) {
	candidates, configurationError := profiles.orderedConfigurations(selection, runtimeConfiguration)
	if configurationError != nil {
		return nil, configurationError
	}
	if clientFactory == nil {
		clientFactory = NewFactory
	}

	clients := make([]prioritizedClientCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		client, clientError := clientFactory(candidate.config)
		if clientError != nil {
			return nil, fmt.Errorf("initialize llm connection %s: %w", candidate.name, clientError)
		}
		clients = append(clients, prioritizedClientCandidate{name: candidate.name, client: client})
	}
	return &prioritizedChatClient{candidates: clients}, nil
}

func (profiles ConnectionProfiles) orderedConfigurations(
	selection LLMProxySelection,
	runtimeConfiguration RuntimeConfig,
) ([]prioritizedConfigCandidate, error) {
	openAIProfile := profiles.OpenAI
	openAIProfile.BaseURL = strings.TrimSpace(openAIProfile.BaseURL)
	openAIProfile.Credential = strings.TrimSpace(openAIProfile.Credential)
	openAIProfile.Model = strings.TrimSpace(openAIProfile.Model)
	if openAIProfile.Priority <= 0 {
		return nil, fmt.Errorf("%s: %s", connectionOpenAI, priorityRequiredMessage)
	}
	if openAIProfile.BaseURL == "" {
		return nil, fmt.Errorf("%s: %s", connectionOpenAI, baseURLRequiredMessage)
	}
	if openAIProfile.Model == "" {
		openAIProfile.Model = DefaultOpenAIModel
	}

	proxyProfile := profiles.LLMProxy
	proxyProfile.BaseURL = strings.TrimSpace(proxyProfile.BaseURL)
	proxyProfile.Credential = strings.TrimSpace(proxyProfile.Credential)
	if proxyProfile.Priority <= 0 {
		return nil, fmt.Errorf("%s: %s", connectionLLMProxy, priorityRequiredMessage)
	}
	if proxyProfile.BaseURL == "" {
		return nil, fmt.Errorf("%s: %s", connectionLLMProxy, baseURLRequiredMessage)
	}
	provider, providerError := NormalizeProvider(proxyProfile.Provider)
	if providerError != nil {
		return nil, providerError
	}
	proxyProfile.Provider = provider
	proxyProfile.Model = strings.TrimSpace(proxyProfile.Model)
	if selectedProvider := strings.TrimSpace(selection.Provider); selectedProvider != "" {
		selectedProvider, selectedProviderError := NormalizeProvider(selectedProvider)
		if selectedProviderError != nil {
			return nil, selectedProviderError
		}
		proxyProfile.Provider = selectedProvider
		proxyProfile.Model = ""
	}
	if selectedModel := strings.TrimSpace(selection.Model); selectedModel != "" {
		proxyProfile.Model = selectedModel
	}

	if openAIProfile.Priority == proxyProfile.Priority {
		return nil, errors.New(priorityUniqueMessage)
	}
	if openAIProfile.Credential == "" && proxyProfile.Credential == "" {
		return nil, errors.New(connectionRequiredMessage)
	}

	candidates := make([]prioritizedConfigCandidate, 0, 2)
	if openAIProfile.Credential != "" {
		candidates = append(candidates, prioritizedConfigCandidate{
			name:     connectionOpenAI,
			priority: openAIProfile.Priority,
			config: runtimeConfiguration.apply(Config{
				Transport: TransportOpenAICompatible,
				Provider:  ProviderOpenAI,
				BaseURL:   openAIProfile.BaseURL,
				APIKey:    openAIProfile.Credential,
				Model:     openAIProfile.Model,
			}),
		})
	}
	if proxyProfile.Credential != "" {
		candidates = append(candidates, prioritizedConfigCandidate{
			name:     connectionLLMProxy,
			priority: proxyProfile.Priority,
			config: runtimeConfiguration.apply(Config{
				Transport: TransportLLMProxy,
				Provider:  proxyProfile.Provider,
				BaseURL:   proxyProfile.BaseURL,
				APIKey:    proxyProfile.Credential,
				Model:     proxyProfile.Model,
			}),
		})
	}
	if len(candidates) == 2 && candidates[0].priority > candidates[1].priority {
		candidates[0], candidates[1] = candidates[1], candidates[0]
	}
	return candidates, nil
}

func (configuration RuntimeConfig) apply(candidate Config) Config {
	candidate.MaxCompletionTokens = configuration.MaxCompletionTokens
	candidate.Temperature = configuration.Temperature
	candidate.HTTPClient = configuration.HTTPClient
	candidate.RequestTimeout = configuration.RequestTimeout
	candidate.RetryAttempts = configuration.RetryAttempts
	candidate.RetryInitialBackoff = configuration.RetryInitialBackoff
	candidate.RetryMaxBackoff = configuration.RetryMaxBackoff
	candidate.RetryBackoffFactor = configuration.RetryBackoffFactor
	return candidate
}

func validateProviderTransport(transport Transport, provider string) error {
	switch transport {
	case TransportOpenAICompatible:
		if provider == ProviderOpenAI {
			return nil
		}
	case TransportLLMProxy:
		return nil
	}
	return fmt.Errorf("llm provider %q is incompatible with internal transport %q", provider, transport)
}

func normalizeFactoryConfiguration(configuration Config) (Config, error) {
	transport, transportError := parseInternalTransport(string(configuration.Transport))
	if transportError != nil {
		return Config{}, transportError
	}
	provider, providerError := NormalizeProvider(configuration.Provider)
	if providerError != nil {
		return Config{}, providerError
	}
	if providerTransportError := validateProviderTransport(transport, provider); providerTransportError != nil {
		return Config{}, providerTransportError
	}
	configuration.Transport = transport
	configuration.Provider = provider
	configuration.BaseURL = strings.TrimSpace(configuration.BaseURL)
	configuration.APIKey = strings.TrimSpace(configuration.APIKey)
	configuration.Model = strings.TrimSpace(configuration.Model)
	if configuration.Transport == TransportOpenAICompatible && configuration.Model == "" {
		configuration.Model = DefaultOpenAIModel
	}
	return configuration, nil
}

// NewFactory creates the configured chat client.
func NewFactory(configuration Config) (llm.ChatClient, error) {
	normalizedConfiguration, normalizationError := normalizeFactoryConfiguration(configuration)
	if normalizationError != nil {
		return nil, normalizationError
	}
	if normalizedConfiguration.BaseURL == "" {
		return nil, errors.New(baseURLRequiredMessage)
	}
	if normalizedConfiguration.APIKey == "" {
		return nil, errors.New(credentialRequiredMessage)
	}
	switch normalizedConfiguration.Transport {
	case TransportLLMProxy:
		return newProxyChatClient(normalizedConfiguration)
	case TransportOpenAICompatible:
		return llm.NewFactory(normalizedConfiguration.toOpenAICompatibleConfig())
	default:
		return nil, fmt.Errorf("unsupported llm transport %q", normalizedConfiguration.Transport)
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
	proxyConfiguration, configurationError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL:  strings.TrimSpace(configuration.BaseURL),
		Secret:   configuration.APIKey,
		Provider: strings.TrimSpace(configuration.Provider),
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
		client:                client,
		model:                 strings.TrimSpace(configuration.Model),
		requestTimeoutSeconds: int(timeout / time.Second),
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
		Messages:              messages,
		Model:                 model,
		WebSearch:             false,
		MaxTokens:             positiveIntPointer(maxTokens),
		RequestTimeoutSeconds: &client.requestTimeoutSeconds,
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

func (client prioritizedChatClient) Chat(ctx context.Context, request llm.ChatRequest) (string, error) {
	attemptErrors := make([]error, 0, len(client.candidates))
	for _, candidate := range client.candidates {
		response, responseError := candidate.client.Chat(ctx, request)
		if responseError == nil {
			return response, nil
		}
		attemptError := fmt.Errorf("%s: %w", candidate.name, responseError)
		if contextError := ctx.Err(); contextError != nil {
			return "", fmt.Errorf("llm connection attempt cancelled: %w", attemptError)
		}
		attemptErrors = append(attemptErrors, attemptError)
	}
	return "", fmt.Errorf("all llm connections failed: %w", errors.Join(attemptErrors...))
}

func positiveIntPointer(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}
