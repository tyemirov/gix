package llmclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tyemirov/utils/llm"
)

type rewriteHTTPClient struct {
	target *url.URL
}

func (client rewriteHTTPClient) Do(request *http.Request) (*http.Response, error) {
	rewrittenRequest := request.Clone(request.Context())
	rewrittenURL := *request.URL
	rewrittenURL.Scheme = client.target.Scheme
	rewrittenURL.Host = client.target.Host
	rewrittenRequest.URL = &rewrittenURL
	rewrittenRequest.Host = client.target.Host
	return http.DefaultClient.Do(rewrittenRequest)
}

func TestNewFactoryUsesLLMProxyV2ForExplicitProvider(t *testing.T) {
	var capturedBody struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		Model     string `json:"model"`
		WebSearch bool   `json:"web_search"`
		MaxTokens int    `json:"max_tokens"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/v2", request.URL.Path)
		require.Equal(t, "test-secret", request.URL.Query().Get("key"))
		require.Equal(t, "text/plain", request.Header.Get("Accept"))
		require.NoError(t, json.NewDecoder(request.Body).Decode(&capturedBody))
		_, _ = responseWriter.Write([]byte("  docs: sync dirty work\n"))
	}))
	t.Cleanup(server.Close)

	targetURL, parseError := url.Parse(server.URL)
	require.NoError(t, parseError)
	client, clientError := NewFactory(Config{
		Provider:            ProviderLLMProxy,
		BaseURL:             DefaultLLMProxyBaseURL,
		APIKey:              "test-secret",
		Model:               DefaultModel,
		MaxCompletionTokens: 64,
		HTTPClient:          rewriteHTTPClient{target: targetURL},
		RequestTimeout:      time.Second,
	})
	require.NoError(t, clientError)

	response, responseError := client.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "Return a commit message."},
			{Role: "user", Content: "Diff"},
		},
		MaxTokens: 80,
	})

	require.NoError(t, responseError)
	require.Equal(t, "docs: sync dirty work", response)
	require.Equal(t, DefaultModel, capturedBody.Model)
	require.False(t, capturedBody.WebSearch)
	require.Equal(t, 80, capturedBody.MaxTokens)
	require.Equal(t, "system", capturedBody.Messages[0].Role)
	require.Equal(t, "Return a commit message.", capturedBody.Messages[0].Content)
	require.Equal(t, "user", capturedBody.Messages[1].Role)
	require.Equal(t, "Diff", capturedBody.Messages[1].Content)
}

func TestNewFactoryKeepsOpenAICompatibleTransportForExplicitNonProxyBase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/chat/completions", request.URL.Path)
		require.Equal(t, "Bearer test-token", request.Header.Get("Authorization"))
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"feat: direct transport"}}]}`))
	}))
	t.Cleanup(server.Close)

	client, clientError := NewFactory(Config{
		Provider:       ProviderOpenAICompatible,
		BaseURL:        server.URL,
		APIKey:         "test-token",
		Model:          "mock-model",
		RequestTimeout: time.Second,
	})
	require.NoError(t, clientError)

	response, responseError := client.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Diff"}},
	})

	require.NoError(t, responseError)
	require.Equal(t, "feat: direct transport", response)
}

func TestNewFactoryRejectsTemperatureForLLMProxy(t *testing.T) {
	_, clientError := NewFactory(Config{
		Provider:    ProviderLLMProxy,
		BaseURL:     DefaultLLMProxyBaseURL,
		APIKey:      "test-secret",
		Model:       DefaultModel,
		Temperature: 0.2,
	})

	require.EqualError(t, clientError, "llm proxy client does not support temperature")
}

func TestProviderDefaultsSelectOpenAICompatibleEnvironment(t *testing.T) {
	require.Equal(t, ProviderOpenAICompatible, DefaultProvider)
	require.Equal(t, DefaultAPIKeyEnvironment, DefaultAPIKeyEnvironmentForProviderName(""))
	require.Equal(t, DefaultBaseURL, DefaultBaseURLForProviderName(""))
	require.Equal(t, DefaultLLMProxyAPIKeyEnvironment, DefaultAPIKeyEnvironmentForProviderName(string(ProviderLLMProxy)))
	require.Equal(t, DefaultLLMProxyBaseURL, DefaultBaseURLForProviderName(string(ProviderLLMProxy)))
}
