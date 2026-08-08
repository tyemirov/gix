package llmclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tyemirov/llm-proxy/pkg/llmproxycontract"
	"github.com/tyemirov/utils/llm"
)

type rewriteHTTPClient struct {
	target *url.URL
}

type failingChatClient struct {
	err error
}

type scriptedChatClient struct {
	requests       []llm.ChatRequest
	responses      []string
	responseErrors []error
}

func (client failingChatClient) Chat(context.Context, llm.ChatRequest) (string, error) {
	return "", client.err
}

func (client *scriptedChatClient) Chat(_ context.Context, request llm.ChatRequest) (string, error) {
	requestIndex := len(client.requests)
	client.requests = append(client.requests, request)
	return client.responses[requestIndex], client.responseErrors[requestIndex]
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

func TestNewFactoryUsesLLMProxyV2ForInternalRouteAndProvider(t *testing.T) {
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
		require.Equal(t, "meta", request.URL.Query().Get("provider"))
		require.Equal(t, "text/plain", request.Header.Get("Accept"))
		require.Equal(t, "1", request.Header.Get(llmproxycontract.HeaderRequestTimeoutSeconds))
		require.NoError(t, json.NewDecoder(request.Body).Decode(&capturedBody))
		_, _ = responseWriter.Write([]byte("  docs: sync dirty work\n"))
	}))
	t.Cleanup(server.Close)

	targetURL, parseError := url.Parse(server.URL)
	require.NoError(t, parseError)
	client, clientError := NewFactory(Config{
		Transport:           TransportLLMProxy,
		Provider:            "meta",
		BaseURL:             "https://llm-proxy.example",
		APIKey:              "test-secret",
		Model:               "muse-spark-1.1",
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
	require.Equal(t, "muse-spark-1.1", capturedBody.Model)
	require.False(t, capturedBody.WebSearch)
	require.Equal(t, 80, capturedBody.MaxTokens)
	require.Equal(t, "system", capturedBody.Messages[0].Role)
	require.Equal(t, "Return a commit message.", capturedBody.Messages[0].Content)
	require.Equal(t, "user", capturedBody.Messages[1].Role)
	require.Equal(t, "Diff", capturedBody.Messages[1].Content)
}

func TestNewFactoryOmitsLLMProxyModelForProviderDefault(t *testing.T) {
	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		require.NoError(t, json.NewDecoder(request.Body).Decode(&capturedBody))
		_, _ = responseWriter.Write([]byte("provider default"))
	}))
	t.Cleanup(server.Close)

	targetURL, parseError := url.Parse(server.URL)
	require.NoError(t, parseError)
	client, clientError := NewFactory(Config{
		Transport:      TransportLLMProxy,
		Provider:       "meta",
		BaseURL:        "https://llm-proxy.example",
		APIKey:         "test-secret",
		HTTPClient:     rewriteHTTPClient{target: targetURL},
		RequestTimeout: time.Second,
	})
	require.NoError(t, clientError)

	_, responseError := client.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Diff"}},
	})

	require.NoError(t, responseError)
	require.NotContains(t, capturedBody, "model")
}

func TestNewFactoryUsesConfiguredLLMProxyCompletionTokensWithoutRequestOverride(t *testing.T) {
	var capturedBody struct {
		MaxTokens int `json:"max_tokens"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		require.NoError(t, json.NewDecoder(request.Body).Decode(&capturedBody))
		_, _ = responseWriter.Write([]byte("configured token budget"))
	}))
	t.Cleanup(server.Close)

	targetURL, parseError := url.Parse(server.URL)
	require.NoError(t, parseError)
	client, clientError := NewFactory(Config{
		Transport:           TransportLLMProxy,
		Provider:            "meta",
		BaseURL:             "https://llm-proxy.example",
		APIKey:              "test-secret",
		MaxCompletionTokens: 64,
		HTTPClient:          rewriteHTTPClient{target: targetURL},
		RequestTimeout:      time.Second,
	})
	require.NoError(t, clientError)

	_, responseError := client.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Diff"}},
	})

	require.NoError(t, responseError)
	require.Equal(t, 64, capturedBody.MaxTokens)
}

func TestNewFactoryDefaultsOpenAIModelForDirectConnection(t *testing.T) {
	var capturedBody struct {
		Model string `json:"model"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/chat/completions", request.URL.Path)
		require.Equal(t, "Bearer test-token", request.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(request.Body).Decode(&capturedBody))
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"feat: direct transport"}}]}`))
	}))
	t.Cleanup(server.Close)

	client, clientError := NewFactory(Config{
		Transport:      TransportOpenAICompatible,
		Provider:       FallbackProvider,
		BaseURL:        server.URL,
		APIKey:         "test-token",
		Model:          "gpt-5.6-terra",
		RequestTimeout: time.Second,
	})
	require.NoError(t, clientError)

	response, responseError := client.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "Diff"}},
	})

	require.NoError(t, responseError)
	require.Equal(t, "feat: direct transport", response)
	require.Equal(t, "gpt-5.6-terra", capturedBody.Model)
}

func TestNewFactoryRejectsProviderWithoutLLMProxyTransport(t *testing.T) {
	_, clientError := NewFactory(Config{
		Transport: TransportOpenAICompatible,
		Provider:  "deepseek",
		BaseURL:   "https://api.openai.com/v1",
		APIKey:    "test-secret",
		Model:     "gpt-5.6-terra",
	})

	require.EqualError(t, clientError, `llm provider "deepseek" is incompatible with internal transport "openai_compatible"`)
}

func TestConnectionProfilesOrderProviderOwnedCandidates(t *testing.T) {
	profiles := ConnectionProfiles{
		OpenAI: OpenAIConnectionProfile{
			Priority:   2,
			BaseURL:    "https://api.openai.com/v1",
			Credential: "openai-secret",
			Model:      "gpt-5.6-terra",
		},
		LLMProxy: LLMProxyConnectionProfile{
			Priority:   1,
			BaseURL:    "https://llm-proxy.example",
			Credential: "proxy-secret",
			Provider:   "meta",
		},
	}

	candidates, configurationError := profiles.orderedConfigurations(LLMProxySelection{}, RuntimeConfig{})

	require.NoError(t, configurationError)
	require.Len(t, candidates, 2)
	require.Equal(t, connectionLLMProxy, candidates[0].name)
	require.Equal(t, TransportLLMProxy, candidates[0].config.Transport)
	require.Equal(t, "meta", candidates[0].config.Provider)
	require.Empty(t, candidates[0].config.Model)
	require.Equal(t, FallbackProvider, candidates[1].name)
	require.Equal(t, TransportOpenAICompatible, candidates[1].config.Transport)
	require.Equal(t, "gpt-5.6-terra", candidates[1].config.Model)
}

func TestConnectionProfilesValidatePriorityAndProxyProvider(t *testing.T) {
	validProfiles := ConnectionProfiles{
		OpenAI: OpenAIConnectionProfile{
			Priority: 1,
			BaseURL:  "https://api.openai.com/v1",
			Model:    "gpt-5.6-terra",
		},
		LLMProxy: LLMProxyConnectionProfile{
			Priority:   2,
			BaseURL:    "https://llm-proxy.example",
			Credential: "proxy-secret",
			Provider:   "meta",
		},
	}

	missingProviderProfiles := validProfiles
	missingProviderProfiles.LLMProxy.Provider = ""
	require.EqualError(t, missingProviderProfiles.Validate(), "llm llm_proxy provider is required")

	duplicatePriorityProfiles := validProfiles
	duplicatePriorityProfiles.LLMProxy.Priority = duplicatePriorityProfiles.OpenAI.Priority
	require.EqualError(t, duplicatePriorityProfiles.Validate(), priorityUniqueMessage)

	noCredentialProfiles := validProfiles
	noCredentialProfiles.LLMProxy.Credential = ""
	require.EqualError(t, noCredentialProfiles.Validate(), connectionRequiredMessage)

	negativeOpenAICompletionTokens := validProfiles
	negativeOpenAICompletionTokens.OpenAI.MaxCompletionTokens = -1
	require.EqualError(t, negativeOpenAICompletionTokens.Validate(), "openai: llm max_completion_tokens must be non-negative")

	negativeProxyCompletionTokens := validProfiles
	negativeProxyCompletionTokens.LLMProxy.MaxCompletionTokens = -1
	require.EqualError(t, negativeProxyCompletionTokens.Validate(), "llm_proxy: llm max_completion_tokens must be non-negative")
}

func TestPrioritizedChatClientReportsEveryFailedConnection(t *testing.T) {
	client := prioritizedChatClient{
		candidates: []prioritizedClientCandidate{
			{name: FallbackProvider, client: failingChatClient{err: errors.New("direct unavailable")}},
			{name: connectionLLMProxy, client: failingChatClient{err: errors.New("proxy unavailable")}},
		},
	}

	response, responseError := client.Chat(context.Background(), llm.ChatRequest{})

	require.Empty(t, response)
	require.ErrorContains(t, responseError, "all llm connections failed")
	require.ErrorContains(t, responseError, "openai: direct unavailable")
	require.ErrorContains(t, responseError, "llm_proxy: proxy unavailable")
}

func TestOpenAIChatClientRetriesEmptyResponseWithResolvedBudget(t *testing.T) {
	baseClient := &scriptedChatClient{
		responses:      []string{"", "recovered response"},
		responseErrors: []error{fmt.Errorf("normal retries exhausted: %w", llm.ErrEmptyResponse), nil},
	}
	client := openAIChatClient{client: baseClient}

	response, responseError := client.Chat(context.Background(), llm.ChatRequest{
		Messages:  []llm.Message{{Role: "user", Content: "Diff"}},
		MaxTokens: 1_200,
	})

	require.NoError(t, responseError)
	require.Equal(t, "recovered response", response)
	require.Len(t, baseClient.requests, 2)
	require.Equal(t, 1_200, baseClient.requests[0].MaxTokens)
	require.Equal(t, 1_200, baseClient.requests[1].MaxTokens)
}

func TestOpenAIChatClientDoesNotRecoverOrdinaryFailure(t *testing.T) {
	ordinaryError := errors.New("openai authentication failed")
	baseClient := &scriptedChatClient{
		responses:      []string{""},
		responseErrors: []error{ordinaryError},
	}
	client := openAIChatClient{client: baseClient}

	response, responseError := client.Chat(context.Background(), llm.ChatRequest{})

	require.Empty(t, response)
	require.ErrorIs(t, responseError, ordinaryError)
	require.Len(t, baseClient.requests, 1)
}

func TestOpenAIChatClientPreservesCancellationBeforeRecovery(t *testing.T) {
	baseClient := &scriptedChatClient{
		responses:      []string{""},
		responseErrors: []error{fmt.Errorf("normal retries exhausted: %w", llm.ErrEmptyResponse)},
	}
	client := openAIChatClient{client: baseClient}
	cancelledContext, cancel := context.WithCancel(context.Background())
	cancel()

	response, responseError := client.Chat(cancelledContext, llm.ChatRequest{})

	require.Empty(t, response)
	require.ErrorContains(t, responseError, "openai empty-response recovery cancelled")
	require.ErrorIs(t, responseError, llm.ErrEmptyResponse)
	require.ErrorIs(t, responseError, context.Canceled)
	require.Len(t, baseClient.requests, 1)
}

func TestOpenAIChatClientPreservesBothEmptyResponseRounds(t *testing.T) {
	primaryError := fmt.Errorf("normal retries exhausted: %w", llm.ErrEmptyResponse)
	recoveryError := errors.New("recovery transport failed")
	baseClient := &scriptedChatClient{
		responses:      []string{"", ""},
		responseErrors: []error{primaryError, recoveryError},
	}
	client := openAIChatClient{client: baseClient}

	response, responseError := client.Chat(context.Background(), llm.ChatRequest{})

	require.Empty(t, response)
	require.ErrorContains(t, responseError, "openai empty-response recovery failed")
	require.ErrorIs(t, responseError, llm.ErrEmptyResponse)
	require.ErrorIs(t, responseError, recoveryError)
	require.Len(t, baseClient.requests, 2)
}

func TestReasoningEffortHTTPClientInjectsReasoningEffortPayload(t *testing.T) {
	t.Parallel()

	var capturedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{}}`))
	}))
	t.Cleanup(server.Close)

	targetURL, parseError := url.Parse(server.URL)
	require.NoError(t, parseError)

	client, clientError := NewFactory(Config{
		Transport:  TransportOpenAICompatible,
		Provider:   FallbackProvider,
		BaseURL:    "https://api.openai.com/v1",
		APIKey:     "test-key",
		Model:      "gpt-5.6-terra",
		Effort:     "high",
		HTTPClient: rewriteHTTPClient{target: targetURL},
	})
	require.NoError(t, clientError)

	response, responseError := client.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, responseError)
	require.Equal(t, "ok", response)
	require.Equal(t, "high", capturedBody["reasoning_effort"])
}

func TestConnectionProfilesPropagatesEffort(t *testing.T) {
	t.Parallel()

	profiles := ConnectionProfiles{
		OpenAI: OpenAIConnectionProfile{
			Priority:   1,
			BaseURL:    "https://api.openai.com/v1",
			Credential: "openai-secret",
			Model:      "gpt-5.6-terra",
			Effort:     "high",
		},
		LLMProxy: LLMProxyConnectionProfile{
			Priority:   2,
			BaseURL:    "https://llm-proxy.example",
			Credential: "proxy-secret",
			Provider:   "meta",
			Effort:     "medium",
		},
	}

	candidates, err := profiles.orderedConfigurations(LLMProxySelection{}, RuntimeConfig{})
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	require.Equal(t, "high", candidates[0].config.Effort)
	require.Equal(t, "medium", candidates[1].config.Effort)
}

func TestConnectionProfilesResolveCompletionTokensByRuntimeThenProvider(t *testing.T) {
	t.Parallel()

	profiles := ConnectionProfiles{
		OpenAI: OpenAIConnectionProfile{
			Priority:            1,
			BaseURL:             "https://api.openai.com/v1",
			Credential:          "openai-secret",
			Model:               "gpt-5.6-terra",
			MaxCompletionTokens: 16_384,
		},
		LLMProxy: LLMProxyConnectionProfile{
			Priority:            2,
			BaseURL:             "https://llm-proxy.example",
			Credential:          "proxy-secret",
			Provider:            "meta",
			MaxCompletionTokens: 1_200,
		},
	}

	providerCandidates, providerError := profiles.orderedConfigurations(LLMProxySelection{}, RuntimeConfig{})
	require.NoError(t, providerError)
	require.Equal(t, 16_384, providerCandidates[0].config.MaxCompletionTokens)
	require.Equal(t, 1_200, providerCandidates[1].config.MaxCompletionTokens)

	commandCandidates, commandError := profiles.orderedConfigurations(LLMProxySelection{}, RuntimeConfig{MaxCompletionTokens: 32_768})
	require.NoError(t, commandError)
	require.Equal(t, 32_768, commandCandidates[0].config.MaxCompletionTokens)
	require.Equal(t, 32_768, commandCandidates[1].config.MaxCompletionTokens)
}
