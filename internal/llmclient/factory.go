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
	DefaultAPIKeyEnvironment = "LLM_PROXY_SECRET"
	DefaultBaseURL           = "https://llm-proxy-api.mprlab.com"
	DefaultModel             = "gpt-4.1"
	defaultRequestTimeout    = 60 * time.Second
)

type proxyChatClient struct {
	client llmproxyclient.Client
	model  string
}

// NewFactory creates the configured chat client.
func NewFactory(configuration llm.Config) (llm.ChatClient, error) {
	if isDefaultLLMProxyBaseURL(configuration.BaseURL) {
		return newProxyChatClient(configuration)
	}
	return llm.NewFactory(configuration)
}

func isDefaultLLMProxyBaseURL(baseURL string) bool {
	return strings.TrimRight(strings.TrimSpace(baseURL), "/") == DefaultBaseURL
}

func newProxyChatClient(configuration llm.Config) (llm.ChatClient, error) {
	if configuration.Temperature > 0 {
		return nil, errors.New("llm proxy client does not support temperature")
	}
	timeout := configuration.RequestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	proxyConfiguration, configurationError := llmproxyclient.NewConfig(llmproxyclient.ConfigInput{
		BaseURL: configuration.BaseURL,
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
