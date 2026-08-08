package syncflow

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/llmclient"

	"github.com/tyemirov/utils/llm"
)

type mockFailingChatClient struct {
	response string
	err      error
}

func (m *mockFailingChatClient) Chat(ctx context.Context, req llm.ChatRequest) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

type mockGitExecutor struct {
	outputs map[string]string
}

func (m *mockGitExecutor) ExecuteGit(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	var combinedArgs string
	if len(details.Arguments) > 0 {
		combinedArgs = details.Arguments[0]
	}
	if out, ok := m.outputs[combinedArgs]; ok {
		return execshell.ExecutionResult{StandardOutput: out}, nil
	}
	return execshell.ExecutionResult{StandardOutput: "mock output"}, nil
}

func (m *mockGitExecutor) ExecuteGitHubCLI(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{StandardOutput: "mock gh output"}, nil
}

func TestSanitizeLLMDescriptionError(t *testing.T) {
	t.Parallel()

	options := worktreeAdoptionCommitMessageOptions{
		ConnectionProfiles: llmclient.ConnectionProfiles{
			OpenAI: llmclient.OpenAIConnectionProfile{
				Credential: "sk-openai-secret-key-12345",
			},
			LLMProxy: llmclient.LLMProxyConnectionProfile{
				Credential: "proxy-secret-token-67890",
			},
		},
	}

	t.Run("nil error", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, sanitizeLLMDescriptionError(nil, options))
	})

	t.Run("query params and credentials redacted", func(t *testing.T) {
		t.Parallel()
		rawErr := errors.New("post https://proxy.example.com/v2/messages?key=secret-key-in-query&api_key=123: sk-openai-secret-key-12345 failed with proxy-secret-token-67890")
		sanitized := sanitizeLLMDescriptionError(rawErr, options)
		require.Error(t, sanitized)

		errStr := sanitized.Error()
		assert.NotContains(t, errStr, "sk-openai-secret-key-12345")
		assert.NotContains(t, errStr, "proxy-secret-token-67890")
		assert.NotContains(t, errStr, "secret-key-in-query")
		assert.Contains(t, errStr, "key=[REDACTED]")
		assert.Contains(t, errStr, "api_key=[REDACTED]")
		assert.Contains(t, errStr, "[REDACTED]")
	})

	t.Run("bearer and header secrets redacted", func(t *testing.T) {
		t.Parallel()
		rawErr := errors.New("http request failed with Bearer secret-bearer-token and Authorization: Bearer second-token")
		sanitized := sanitizeLLMDescriptionError(rawErr, options)
		require.Error(t, sanitized)

		errStr := sanitized.Error()
		assert.NotContains(t, errStr, "secret-bearer-token")
		assert.NotContains(t, errStr, "second-token")
		assert.Contains(t, errStr, "Bearer [REDACTED]")
	})

	t.Run("URL basic auth redacted", func(t *testing.T) {
		t.Parallel()
		rawErr := errors.New("failed to connect to https://user:super-secret-password@proxy.example.com/v1")
		sanitized := sanitizeLLMDescriptionError(rawErr, options)
		require.Error(t, sanitized)

		errStr := sanitized.Error()
		assert.NotContains(t, errStr, "super-secret-password")
		assert.Contains(t, errStr, "https://[REDACTED]@proxy.example.com/v1")
	})
}

func TestGenerateStrictSyncPullRequestBody_EmptyResponse(t *testing.T) {
	t.Parallel()

	mockClient := &mockFailingChatClient{response: "   \n  \t "}
	options := strictSyncPullRequestDescriptionOptions{
		RepositoryPath: t.TempDir(),
		RemoteName:     "origin",
		BaseBranch:     "master",
		BranchName:     "feature",
		CommitMessages: worktreeAdoptionCommitMessageOptions{
			Client: mockClient,
		},
	}

	executor := &mockGitExecutor{
		outputs: map[string]string{
			"log":  "Commit 1\nCommit 2",
			"diff": "1 file changed, 1 insertion(+)",
		},
	}

	_, err := generateStrictSyncPullRequestBody(context.Background(), executor, options)
	require.Error(t, err)
	assert.Contains(t, err.Error(), strictSyncPullRequestDescriptionEmptyResponse)
}

func TestGenerateStrictSyncPullRequestBody_SanitizedLLMError(t *testing.T) {
	t.Parallel()

	mockClient := &mockFailingChatClient{
		err: errors.New("llm proxy error: Post https://proxy.llm.internal/v2/chat?key=secret-proxy-key: 500 Internal Server Error"),
	}
	options := strictSyncPullRequestDescriptionOptions{
		RepositoryPath: t.TempDir(),
		RemoteName:     "origin",
		BaseBranch:     "master",
		BranchName:     "feature",
		CommitMessages: worktreeAdoptionCommitMessageOptions{
			Client: mockClient,
		},
	}

	executor := &mockGitExecutor{
		outputs: map[string]string{
			"log":  "Commit 1",
			"diff": "1 file changed",
		},
	}

	_, err := generateStrictSyncPullRequestBody(context.Background(), executor, options)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "strict sync pull request description.llm:"))
	assert.NotContains(t, err.Error(), "secret-proxy-key")
	assert.Contains(t, err.Error(), "key=[REDACTED]")
}
