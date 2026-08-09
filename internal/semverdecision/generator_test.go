package semverdecision

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/v5/internal/execshell"
	"github.com/tyemirov/utils/llm"
)

const testRange = "v1.2.3..HEAD"

func TestGenerateUsesLLMDecisionAndRepositoryEvidence(t *testing.T) {
	executor := newDecisionGitExecutor("fix: preserve behavior\n\x1e")
	client := &decisionChatClient{response: `{"bump":"major","reason":"The configuration key removal breaks the public contract."}`}
	generator := Generator{GitExecutor: executor, Client: client}

	result, generateError := generator.Generate(context.Background(), Options{
		RepositoryPath: "/tmp/repo",
		SinceReference: "v1.2.3",
	})

	require.NoError(t, generateError)
	require.Equal(t, BumpMajor, result.Bump)
	require.Equal(t, BumpPatch, result.DeterministicFloor)
	require.Contains(t, result.Reason, "configuration key removal")
	require.NotNil(t, client.request)
	require.Contains(t, client.request.Messages[0].Content, "Semantic Versioning 2.0.0")
	require.Contains(t, client.request.Messages[1].Content, "config.yml")
	require.Contains(t, client.request.Messages[1].Content, "Removed the old key")
}

func TestGenerateEnforcesConventionalCommitFloor(t *testing.T) {
	testCases := []struct {
		name          string
		commitLog     string
		expectedFloor Bump
	}{
		{name: "feature", commitLog: "feat(api): add endpoint\n\x1e", expectedFloor: BumpMinor},
		{name: "breaking_header", commitLog: "feat(config)!: remove key\n\x1e", expectedFloor: BumpMajor},
		{name: "breaking_footer", commitLog: "fix: update config\n\nBREAKING CHANGE: remove key\n\x1e", expectedFloor: BumpMajor},
		{
			name:          "breaking_footer_beyond_model_excerpt",
			commitLog:     strings.Repeat("fix: routine maintenance\n\x1e", 1200) + "fix: update config\n\nBREAKING CHANGE: remove key\n\x1e",
			expectedFloor: BumpMajor,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			generator := Generator{
				GitExecutor: newDecisionGitExecutor(testCase.commitLog),
				Client:      &decisionChatClient{response: `{"bump":"patch","reason":"The change appears compatible."}`},
			}

			result, generateError := generator.Generate(context.Background(), Options{
				RepositoryPath: "/tmp/repo",
				SinceReference: "v1.2.3",
			})

			require.NoError(t, generateError)
			require.Equal(t, testCase.expectedFloor, result.Bump)
			require.Equal(t, testCase.expectedFloor, result.DeterministicFloor)
			require.Contains(t, result.Reason, "requires at least")
		})
	}
}

func TestGenerateRejectsInvalidLLMDecision(t *testing.T) {
	testCases := []struct {
		name     string
		response string
	}{
		{name: "markdown", response: "```json\n{\"bump\":\"minor\",\"reason\":\"feature\"}\n```"},
		{name: "unknown_bump", response: `{"bump":"automatic","reason":"unspecified"}`},
		{name: "empty_reason", response: `{"bump":"patch","reason":""}`},
		{name: "extra_field", response: `{"bump":"patch","reason":"compatible","version":"v1.2.4"}`},
		{name: "extra_value", response: `{"bump":"patch","reason":"compatible"} {"bump":"major","reason":"extra"}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			generator := Generator{
				GitExecutor: newDecisionGitExecutor("fix: preserve behavior\n\x1e"),
				Client:      &decisionChatClient{response: testCase.response},
			}

			_, generateError := generator.Generate(context.Background(), Options{
				RepositoryPath: "/tmp/repo",
				SinceReference: "v1.2.3",
			})

			require.Error(t, generateError)
			require.Contains(t, generateError.Error(), "semver decision response")
		})
	}
}

func TestGeneratePreservesLLMFailure(t *testing.T) {
	generator := Generator{
		GitExecutor: newDecisionGitExecutor("fix: preserve behavior\n\x1e"),
		Client:      &decisionChatClient{err: errors.New("provider unavailable")},
	}

	_, generateError := generator.Generate(context.Background(), Options{
		RepositoryPath: "/tmp/repo",
		SinceReference: "v1.2.3",
	})

	require.Error(t, generateError)
	require.Contains(t, generateError.Error(), "semver decision.llm")
	require.Contains(t, generateError.Error(), "provider unavailable")
}

type decisionGitExecutor struct {
	responses map[string]string
	calls     []string
}

func newDecisionGitExecutor(commitLog string) *decisionGitExecutor {
	return &decisionGitExecutor{responses: map[string]string{
		"log --pretty=format:%s%n%b%x1e " + testRange: commitLog,
		"diff --stat " + testRange:                    " config.yml | 1 -\n",
		"diff --unified=3 " + testRange:               "diff --git a/config.yml b/config.yml\n-old_key: true\n",
		"show HEAD:CHANGELOG.md":                      "# Changelog\n\n## [Unreleased]\n\n- Removed the old key.\n\n## [v1.2.3]\n",
	}}
}

func (executor *decisionGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	key := strings.Join(details.Arguments, " ")
	executor.calls = append(executor.calls, key)
	return execshell.ExecutionResult{StandardOutput: executor.responses[key]}, nil
}

func (executor *decisionGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type decisionChatClient struct {
	response string
	err      error
	request  *llm.ChatRequest
}

func (client *decisionChatClient) Chat(_ context.Context, request llm.ChatRequest) (string, error) {
	requestCopy := request
	client.request = &requestCopy
	return client.response, client.err
}
