package workflow

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskLLMClientConfigurationClientUsesEnvironment(t *testing.T) {
	configurationReader := newOptionReader(map[string]any{
		optionTaskLLMKeyConstant: map[string]any{
			optionTaskLLMModelKeyConstant:     "gpt-test",
			optionTaskLLMAPIKeyEnvKeyConstant: "WORKFLOW_TEST_KEY",
			optionTaskLLMTimeoutKeyConstant:   12,
			optionTaskLLMMaxTokensKeyConstant: 800,
		},
	})

	configuration, buildErr := buildTaskLLMConfiguration(configurationReader)
	require.NoError(t, buildErr)
	require.NotNil(t, configuration)

	t.Setenv("WORKFLOW_TEST_KEY", "token")

	client, clientErr := configuration.Client()
	require.NoError(t, clientErr)
	require.NotNil(t, client)

	cached, cachedErr := configuration.Client()
	require.NoError(t, cachedErr)
	require.Same(t, client, cached)
}

func TestTaskLLMClientConfigurationClientFailsWithoutEnvironment(t *testing.T) {
	configurationReader := newOptionReader(map[string]any{
		optionTaskLLMKeyConstant: map[string]any{
			optionTaskLLMModelKeyConstant:     "gpt-test",
			optionTaskLLMAPIKeyEnvKeyConstant: "WORKFLOW_TEST_KEY",
		},
	})

	configuration, buildErr := buildTaskLLMConfiguration(configurationReader)
	require.NoError(t, buildErr)
	require.NotNil(t, configuration)

	client, clientErr := configuration.Client()
	require.Nil(t, client)
	require.Error(t, clientErr)
}
