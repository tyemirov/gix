package workflow

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tyemirov/gix/internal/llmclient"
)

func TestTaskLLMClientConfigurationClientUsesProviderConnection(t *testing.T) {
	configurationReader := newOptionReader(map[string]any{
		optionTaskLLMKeyConstant: map[string]any{
			optionTaskLLMProxyKeyConstant: map[string]any{
				optionTaskLLMProviderKeyConstant: llmclient.FallbackProvider,
				optionTaskLLMModelKeyConstant:    "gpt-test",
			},
			optionTaskLLMTimeoutKeyConstant:   12,
			optionTaskLLMMaxTokensKeyConstant: 800,
		},
	})

	configuration, buildErr := buildTaskLLMConfiguration(configurationReader)
	require.NoError(t, buildErr)
	require.NotNil(t, configuration)

	configuration.setConnectionProfiles(llmclient.ConnectionProfiles{
		OpenAI: llmclient.OpenAIConnectionProfile{
			Priority:   1,
			BaseURL:    "https://api.openai.com/v1",
			Credential: "token",
			Model:      "gpt-test",
		},
		LLMProxy: llmclient.LLMProxyConnectionProfile{
			Priority: 2,
			BaseURL:  "https://llm-proxy.example",
			Provider: "meta",
			Model:    "muse-spark-1.1",
		},
	})

	client, clientErr := configuration.Client()
	require.NoError(t, clientErr)
	require.NotNil(t, client)

	cached, cachedErr := configuration.Client()
	require.NoError(t, cachedErr)
	require.Same(t, client, cached)
}

func TestTaskLLMClientConfigurationRejectsFractionalTimeoutSeconds(t *testing.T) {
	testCases := []struct {
		name           string
		timeoutSeconds any
	}{
		{name: "sub-second", timeoutSeconds: 0.5},
		{name: "larger fractional value", timeoutSeconds: 1.9},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			configurationReader := newOptionReader(map[string]any{
				optionTaskLLMKeyConstant: map[string]any{
					optionTaskLLMProxyKeyConstant: map[string]any{
						optionTaskLLMProviderKeyConstant: llmclient.FallbackProvider,
					},
					optionTaskLLMTimeoutKeyConstant: testCase.timeoutSeconds,
				},
			})

			configuration, buildErr := buildTaskLLMConfiguration(configurationReader)

			require.Nil(t, configuration)
			require.EqualError(t, buildErr, "timeout_seconds must be a whole number of seconds")
		})
	}
}

func TestTaskLLMClientConfigurationClientFailsWithoutInjectedCredential(t *testing.T) {
	configurationReader := newOptionReader(map[string]any{
		optionTaskLLMKeyConstant: map[string]any{
			optionTaskLLMProxyKeyConstant: map[string]any{
				optionTaskLLMProviderKeyConstant: llmclient.FallbackProvider,
				optionTaskLLMModelKeyConstant:    "gpt-test",
			},
		},
	})

	configuration, buildErr := buildTaskLLMConfiguration(configurationReader)
	require.NoError(t, buildErr)
	require.NotNil(t, configuration)

	client, clientErr := configuration.Client()
	require.Nil(t, client)
	require.Error(t, clientErr)
}

func TestTaskLLMClientConfigurationRequiresProvider(t *testing.T) {
	configurationReader := newOptionReader(map[string]any{
		optionTaskLLMKeyConstant: map[string]any{
			optionTaskLLMProxyKeyConstant: map[string]any{
				optionTaskLLMModelKeyConstant: "gpt-test",
			},
		},
	})

	configuration, buildErr := buildTaskLLMConfiguration(configurationReader)
	require.Nil(t, configuration)
	require.EqualError(t, buildErr, "llm llm_proxy provider is required")
}

func TestTaskLLMClientConfigurationRejectsObsoleteCredentialFields(t *testing.T) {
	configurationReader := newOptionReader(map[string]any{
		optionTaskLLMKeyConstant: map[string]any{
			optionTaskLLMProxyKeyConstant: map[string]any{
				optionTaskLLMProviderKeyConstant: llmclient.FallbackProvider,
				optionTaskLLMModelKeyConstant:    "gpt-test",
			},
			"api_key_env": "OPENAI_API_KEY",
		},
	})

	configuration, buildErr := buildTaskLLMConfiguration(configurationReader)

	require.Nil(t, configuration)
	require.EqualError(t, buildErr, "unsupported llm configuration key \"api_key_env\"")
}
