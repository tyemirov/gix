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

const testRange = "base123..source456"

func TestGenerateUsesLLMDecisionAndRepositoryEvidence(t *testing.T) {
	executor := newDecisionGitExecutor("fix: preserve behavior\n\x1e")
	client := &decisionChatClient{response: `{"bump":"major","reason":"The configuration key removal breaks the public contract."}`}
	generator := Generator{GitExecutor: executor, Client: client}

	result, generateError := generator.Generate(context.Background(), Options{
		RepositoryPath:  "/tmp/repo",
		SinceReference:  "base123",
		SourceReference: "source456",
		BoundaryLabel:   "v1.2.3",
	})

	require.NoError(t, generateError)
	require.Equal(t, BumpMajor, result.Bump)
	require.Equal(t, BumpPatch, result.DeterministicFloor)
	require.Contains(t, result.Reason, "configuration key removal")
	require.Len(t, result.EvidenceSHA256, 64)
	require.Len(t, client.requests, 1)
	require.Contains(t, client.requests[0].Messages[0].Content, "Semantic Versioning 2.0.0")
	require.Contains(t, client.requests[0].Messages[1].Content, "config.yml")
	require.Contains(t, client.requests[0].Messages[1].Content, "Removed the old key")
	require.Contains(t, client.requests[0].Messages[1].Content, "Release range: v1.2.3..source456")
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
				RepositoryPath:  "/tmp/repo",
				SinceReference:  "base123",
				SourceReference: "source456",
				BoundaryLabel:   "v1.2.3",
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
				RepositoryPath:  "/tmp/repo",
				SinceReference:  "base123",
				SourceReference: "source456",
				BoundaryLabel:   "v1.2.3",
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
		RepositoryPath:  "/tmp/repo",
		SinceReference:  "base123",
		SourceReference: "source456",
		BoundaryLabel:   "v1.2.3",
	})

	require.Error(t, generateError)
	require.Contains(t, generateError.Error(), "semver decision evidence packet 1/1.llm")
	require.Contains(t, generateError.Error(), "provider unavailable")
}

func TestGenerateClassifiesEveryCompleteRangeEvidencePacket(t *testing.T) {
	commitSentinel := "refactor: remove the public compatibility contract"
	diffSummarySentinel := "public_contract.go | 200 +----------------"
	changelogSentinel := "- Removed the public compatibility contract."
	executor := newDecisionGitExecutor(strings.Repeat("fix: routine maintenance\n\x1e", 1200) + commitSentinel + "\n\x1e")
	executor.responses["diff --stat "+testRange] = strings.Repeat("internal_file.go | 1 +\n", 700) + diffSummarySentinel + "\n"
	executor.responses["show source456:CHANGELOG.md"] = "# Changelog\n\n## [Unreleased]\n\n" + strings.Repeat("- Fixed internal behavior.\n", 700) + changelogSentinel + "\n\n## [v1.2.3]\n"
	client := &decisionChatClient{responseForRequest: func(request llm.ChatRequest) string {
		if strings.Contains(request.Messages[1].Content, commitSentinel) {
			return `{"bump":"major","reason":"The release removes a public compatibility contract."}`
		}
		return `{"bump":"patch","reason":"This evidence packet contains compatible maintenance."}`
	}}

	result, generateError := (Generator{GitExecutor: executor, Client: client}).Generate(context.Background(), Options{
		RepositoryPath:  "/tmp/repo",
		SinceReference:  "base123",
		SourceReference: "source456",
		BoundaryLabel:   "v1.2.3",
	})

	require.NoError(t, generateError)
	require.Equal(t, BumpMajor, result.Bump)
	require.Greater(t, len(client.requests), 1)
	allRequests := requestContents(client.requests)
	require.Contains(t, allRequests, commitSentinel)
	require.Contains(t, allRequests, diffSummarySentinel)
	require.Contains(t, allRequests, changelogSentinel)
	require.Contains(t, executor.calls, "log --pretty=format:%s%n%b%x1e "+testRange)
	require.Contains(t, executor.calls, "show source456:CHANGELOG.md")
	require.NotContains(t, executor.calls, "show HEAD:CHANGELOG.md")
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
		"show source456:CHANGELOG.md":                 "# Changelog\n\n## [Unreleased]\n\n- Removed the old key.\n\n## [v1.2.3]\n",
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
	response           string
	err                error
	requests           []llm.ChatRequest
	responseForRequest func(llm.ChatRequest) string
}

func (client *decisionChatClient) Chat(_ context.Context, request llm.ChatRequest) (string, error) {
	client.requests = append(client.requests, request)
	if client.responseForRequest != nil {
		return client.responseForRequest(request), client.err
	}
	return client.response, client.err
}

func requestContents(requests []llm.ChatRequest) string {
	contents := make([]string, 0, len(requests))
	for _, request := range requests {
		for _, message := range request.Messages {
			contents = append(contents, message.Content)
		}
	}
	return strings.Join(contents, "\n")
}
