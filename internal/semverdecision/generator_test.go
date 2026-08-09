package semverdecision

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/utils/llm"
)

const testRange = "base123..source456"

func TestGenerateUsesLLMDecisionAndRepositoryEvidence(t *testing.T) {
	executor := newDecisionGitExecutor("fix: preserve behavior\n\x1e")
	client := &decisionChatClient{response: `{"impact":"incompatible","public_contract":"config.yml old_key","reason":"The release removes the supported old_key configuration key."}`}
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
	require.Contains(t, result.Reason, "supported old_key configuration key")
	require.Len(t, result.EvidenceSHA256, 64)
	require.Len(t, client.requests, 2)
	require.Contains(t, client.requests[0].Messages[0].Content, "Semantic Versioning 2.0.0")
	require.Contains(t, client.requests[0].Messages[0].Content, "Commit types, scopes, exclamation marks")
	require.Contains(t, client.requests[0].Messages[1].Content, "config.yml")
	require.Contains(t, client.requests[0].Messages[1].Content, "Removed the old key")
	require.Contains(t, client.requests[0].Messages[1].Content, "Release range: v1.2.3..source456")
	require.Contains(t, client.requests[1].Messages[0].Content, "Independently audit")
	require.Contains(t, client.requests[1].Messages[1].Content, "Candidate decision:")
}

func TestGenerateDoesNotTreatConventionalCommitLabelsAsPublicContractEvidence(t *testing.T) {
	testCases := []struct {
		name      string
		commitLog string
	}{
		{name: "feature", commitLog: "feat(internal): reorganize packages\n\x1e"},
		{name: "breaking_header", commitLog: "refactor(build)!: reorganize packages\n\x1e"},
		{name: "breaking_footer", commitLog: "refactor: reorganize packages\n\nBREAKING CHANGE: update internal imports\n\x1e"},
		{
			name:      "breaking_footer_beyond_model_excerpt",
			commitLog: strings.Repeat("fix: routine maintenance\n\x1e", 1200) + "refactor: reorganize packages\n\nBREAKING CHANGE: update internal imports\n\x1e",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			generator := Generator{
				GitExecutor: newDecisionGitExecutor(testCase.commitLog),
				Client:      &decisionChatClient{response: `{"impact":"compatible","public_contract":"","reason":"The release changes implementation details only."}`},
			}

			result, generateError := generator.Generate(context.Background(), Options{
				RepositoryPath:  "/tmp/repo",
				SinceReference:  "base123",
				SourceReference: "source456",
				BoundaryLabel:   "v1.2.3",
			})

			require.NoError(t, generateError)
			require.Equal(t, BumpPatch, result.Bump)
			require.Equal(t, BumpPatch, result.DeterministicFloor)
			require.Contains(t, result.Reason, "implementation details")
		})
	}
}

func TestGenerateAuditCorrectsModulePathImplementationCandidate(t *testing.T) {
	executor := newDecisionGitExecutor("fix(release): restore root Go install channel\n\x1e")
	executor.responses["diff --stat "+testRange] = " go.mod | 2 +-\n internal/version/product.go | 4 ++++\n"
	executor.responses["diff --unified=3 "+testRange] = "diff --git a/go.mod b/go.mod\n-module example.com/tool/v5\n+module example.com/tool\n"
	executor.responses["show source456:CHANGELOG.md"] = "# Changelog\n\n## [Unreleased]\n\n- Restored the canonical root Go installation command.\n"
	client := &decisionChatClient{responseForRequest: func(request llm.ChatRequest) string {
		if strings.Contains(request.Messages[0].Content, "Independently audit") {
			return `{"impact":"compatible","public_contract":"go install example.com/tool@latest","reason":"The release restores the canonical installation route."}`
		}
		return `{"impact":"incompatible","public_contract":"Go module path","reason":"The module path changes."}`
	}}

	result, generateError := (Generator{GitExecutor: executor, Client: client}).Generate(context.Background(), Options{
		RepositoryPath:  "/tmp/repo",
		SinceReference:  "base123",
		SourceReference: "source456",
		BoundaryLabel:   "v6.0.0",
	})

	require.NoError(t, generateError)
	require.Equal(t, BumpPatch, result.Bump)
	require.Equal(t, BumpPatch, result.DeterministicFloor)
	require.Contains(t, result.Reason, "restores")
	require.Len(t, client.requests, 2)
	require.Contains(t, client.requests[1].Messages[0].Content, "Go module paths")
}

func TestGenerateRejectsInvalidLLMDecision(t *testing.T) {
	testCases := []struct {
		name     string
		response string
	}{
		{name: "markdown", response: "```json\n{\"impact\":\"additive\",\"public_contract\":\"CLI reports\",\"reason\":\"feature\"}\n```"},
		{name: "unknown_impact", response: `{"impact":"automatic","public_contract":"","reason":"unspecified"}`},
		{name: "empty_reason", response: `{"impact":"compatible","public_contract":"","reason":""}`},
		{name: "missing_public_contract", response: `{"impact":"incompatible","public_contract":"","reason":"removed behavior"}`},
		{name: "extra_field", response: `{"impact":"compatible","public_contract":"","reason":"compatible","version":"v1.2.4"}`},
		{name: "extra_value", response: `{"impact":"compatible","public_contract":"","reason":"compatible"} {"impact":"incompatible","public_contract":"config","reason":"extra"}`},
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
	require.Contains(t, generateError.Error(), "semver decision evidence packet 1/1 candidate.llm")
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
			return `{"impact":"incompatible","public_contract":"public compatibility contract","reason":"The release removes a public compatibility contract."}`
		}
		return `{"impact":"compatible","public_contract":"","reason":"This evidence packet contains compatible maintenance."}`
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
