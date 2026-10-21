package semverdecision

import (
	"context"
	"errors"
	"fmt"
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

func TestGenerateMapsIncompatibleGixContractToFixedMajorMinor(t *testing.T) {
	client := &decisionChatClient{response: `{"impact":"incompatible","public_contract":"gix release version policy","reason":"The release changes the Gix version policy."}`}
	result, generateError := (Generator{
		GitExecutor: newDecisionGitExecutor("refactor(release): use one version\n\x1e"),
		Client:      client,
	}).Generate(context.Background(), Options{
		RepositoryPath:  "/tmp/repo",
		SinceReference:  "base123",
		SourceReference: "source456",
		BoundaryLabel:   "v1.1.26",
		FixedMajor:      1,
	})

	require.NoError(t, generateError)
	require.Equal(t, BumpMinor, result.Bump)
	require.Contains(t, client.requests[0].Messages[0].Content, "fixes its major version at v1")
	require.Contains(t, client.requests[1].Messages[0].Content, "incompatible and additive to minor")
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

func TestGenerateExcludesHistoricalUnreleasedEntriesFromChangelogEvidence(t *testing.T) {
	executor := newDecisionGitExecutor("fix(audit): preserve nested checkout folder identity\n\x1e")
	executor.responses["show source456:CHANGELOG.md"] = strings.Join([]string{
		"# Changelog",
		"",
		"## [Unreleased]",
		"",
		"### Features",
		"- Added historical public feature.",
		"",
		"### Bug Fixes",
		"- Preserved containing audit roots for nested linked checkouts.",
	}, "\n")
	executor.responses["diff --unified=0 "+testRange+" -- CHANGELOG.md"] = strings.Join([]string{
		"diff --git a/CHANGELOG.md b/CHANGELOG.md",
		"@@ -27,0 +28 @@",
		"+- Preserved containing audit roots for nested linked checkouts.",
	}, "\n")
	client := &decisionChatClient{responseForRequest: func(request llm.ChatRequest) string {
		if strings.Contains(request.Messages[1].Content, "Added historical public feature") {
			return `{"impact":"additive","public_contract":"historical feature","reason":"The evidence includes an additive feature."}`
		}
		return `{"impact":"compatible","public_contract":"gix audit folder labels","reason":"The release repairs nested checkout labels."}`
	}}

	result, generateError := (Generator{GitExecutor: executor, Client: client}).Generate(context.Background(), Options{
		RepositoryPath:  "/tmp/repo",
		SinceReference:  "base123",
		SourceReference: "source456",
		BoundaryLabel:   "v1.2.0",
		FixedMajor:      1,
	})

	require.NoError(t, generateError)
	require.Equal(t, BumpPatch, result.Bump)
	require.Contains(t, requestContents(client.requests), "Preserved containing audit roots")
	require.NotContains(t, requestContents(client.requests), "Added historical public feature")
	require.Contains(t, executor.calls, "diff --unified=0 "+testRange+" -- CHANGELOG.md")
	require.NotContains(t, executor.calls, "show source456:CHANGELOG.md")
}

func TestGenerateAuditCorrectsModulePathImplementationCandidate(t *testing.T) {
	executor := newDecisionGitExecutor("fix(release): restore root Go install channel\n\x1e")
	executor.responses["diff --stat "+testRange] = " go.mod | 2 +-\n internal/version/product.go | 4 ++++\n"
	executor.responses["diff --unified=3 "+testRange+" -- . :(exclude)CHANGELOG.md"] = "diff --git a/go.mod b/go.mod\n-module example.com/tool/v5\n+module example.com/tool\n"
	executor.responses["diff --unified=0 "+testRange+" -- CHANGELOG.md"] = "@@ -4,0 +5 @@\n+- Restored the canonical root Go installation command.\n"
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
	executor.responses["diff --unified=0 "+testRange+" -- CHANGELOG.md"] = strings.Repeat("+- Fixed internal behavior.\n", 700) + "+" + changelogSentinel + "\n"
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
	require.Contains(t, executor.calls, "diff --unified=0 "+testRange+" -- CHANGELOG.md")
	require.NotContains(t, executor.calls, "show source456:CHANGELOG.md")
}

type decisionGitExecutor struct {
	responses map[string]string
	calls     []string
}

func newDecisionGitExecutor(commitLog string) *decisionGitExecutor {
	return &decisionGitExecutor{responses: map[string]string{
		"log --pretty=format:%s%n%b%x1e " + testRange:                    commitLog,
		"diff --stat " + testRange:                                       " config.yml | 1 -\n",
		"diff --unified=3 " + testRange + " -- . :(exclude)CHANGELOG.md": "diff --git a/config.yml b/config.yml\n-old_key: true\n",
		"diff --unified=0 " + testRange + " -- CHANGELOG.md":             "@@ -4,0 +5 @@\n+- Removed the old key.\n",
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

func decisionOptions() Options {
	return Options{RepositoryPath: "/tmp/repo", SinceReference: "base123", SourceReference: "source456", BoundaryLabel: "v1.2.3"}
}

func TestGenerateAccountsForUnicodeSingleLineAndLateDiff(t *testing.T) {
	executor := newDecisionGitExecutor("fix: update behavior\n\x1e")
	original := "diff --git a/noise.txt b/noise.txt\n@@ -1 +1 @@\n-previous\n+" + strings.Repeat("界", 90000) + "\n" + "diff --git a/z-command.txt b/z-command.txt\n@@ -1 +0,0 @@\n-LATE_CONTRACT_REMOVAL\n"
	executor.responses["diff --unified=3 "+testRange+" -- . :(exclude)CHANGELOG.md"] = original
	client := &decisionChatClient{responseForRequest: func(request llm.ChatRequest) string {
		if strings.Contains(request.Messages[1].Content, "LATE_CONTRACT_REMOVAL") {
			return `{"impact":"incompatible","public_contract":"supported command","reason":"The release removes a supported command."}`
		}
		return `{"impact":"compatible","public_contract":"","reason":"This fragment preserves supported behavior."}`
	}}
	result, err := (Generator{GitExecutor: executor, Client: client}).Generate(context.Background(), decisionOptions())
	require.NoError(t, err)
	require.Equal(t, BumpMajor, result.Bump)
	require.Greater(t, len(result.Packets), 2)
	reconstructed := ""
	for _, packet := range result.Packets {
		require.LessOrEqual(t, len([]rune(packet.Content)), evidencePacketRuneLimit)
		require.Equal(t, textDigest(packet.Content), packet.SHA256)
		for _, fragment := range packet.Fragments {
			if fragment.Kind == "Diff excerpt" {
				require.Equal(t, len([]rune(reconstructed)), fragment.OffsetRunes)
				reconstructed += fragment.Content
			}
		}
	}
	require.Equal(t, original, reconstructed)
	for _, coverage := range result.Coverage {
		require.Equal(t, coverage.RuneCount, coverage.CoveredRunes)
	}
	require.Equal(t, "base123", result.SinceReference)
	require.Equal(t, "source456", result.SourceReference)
}

func TestGenerateRetrievesCommittedCrossFileContextForCandidateAndAudit(t *testing.T) {
	executor := newDecisionGitExecutor("fix: repair behavior\n\x1e")
	executor.responses["ls-tree -z --name-only base123 -- docs/contract.txt"] = "docs/contract.txt\x00"
	executor.responses["cat-file -t base123:docs/contract.txt"] = "blob\n"
	executor.responses["show base123:docs/contract.txt"] = "Supported command accepts old_key.\nOther line.\n"
	client := &decisionChatClient{responseForRequest: func(request llm.ChatRequest) string {
		if strings.Contains(request.Messages[1].Content, "Supported command accepts old_key.") {
			return `{"impact":"incompatible","public_contract":"old_key","reason":"Existing supported inputs now require migration."}`
		}
		return `{"impact":"unresolved","public_contract":"","reason":"The previous supported inputs need source evidence.","context_requests":[{"path":"docs/contract.txt","source":"old","start_line":1,"line_count":1}]}`
	}}
	result, err := (Generator{GitExecutor: executor, Client: client}).Generate(context.Background(), decisionOptions())
	require.NoError(t, err)
	require.Equal(t, BumpMajor, result.Bump)
	require.Len(t, result.Requests, 3)
	require.Len(t, result.Context, 1)
	require.Len(t, result.Findings, 3)
	require.Equal(t, "unresolved", result.Findings[0].Impact)
	require.Contains(t, result.Requests[2].Messages[1].Content, "Supported command accepts old_key.")
	require.NotContains(t, result.Requests[2].Messages[1].Content, "Other line.")
	require.Contains(t, executor.calls, "show base123:docs/contract.txt")
	require.Equal(t, textDigest(result.Context[0].Content), result.Context[0].SHA256)
}

func TestGenerateRejectsUnsafeAndInvalidCommittedContext(t *testing.T) {
	for _, testCase := range []struct {
		name, path, source string
		start, count       int
	}{
		{"parent", "../outside", "old", 1, 1}, {"absolute", "/outside", "old", 1, 1},
		{"normalized", "docs/../outside", "old", 1, 1}, {"backslash", "docs\\outside", "old", 1, 1},
		{"source", "docs/contract.txt", "working", 1, 1}, {"negative", "docs/contract.txt", "old", 0, 1},
		{"range", "docs/contract.txt", "old", 2, 3},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			executor := newDecisionGitExecutor("fix: repair behavior\n\x1e")
			executor.responses["ls-tree -z --name-only base123 -- docs/contract.txt"] = "docs/contract.txt\x00"
			executor.responses["cat-file -t base123:docs/contract.txt"] = "blob\n"
			executor.responses["show base123:docs/contract.txt"] = "One line.\n"
			response := fmt.Sprintf(`{"impact":"unresolved","public_contract":"","reason":"Need source context.","context_requests":[{"path":%q,"source":%q,"start_line":%d,"line_count":%d}]}`, testCase.path, testCase.source, testCase.start, testCase.count)
			result, err := (Generator{GitExecutor: executor, Client: &decisionChatClient{response: response}}).Generate(context.Background(), decisionOptions())
			require.Error(t, err)
			require.Contains(t, err.Error(), "source context")
			require.Len(t, result.Requests, 1)
			require.NotEmpty(t, result.Packets)
			require.Len(t, result.EvidenceSHA256, 64)
		})
	}
}

func TestGenerateRecordsAbsentCommittedSource(t *testing.T) {
	executor := newDecisionGitExecutor("fix: repair behavior\n\x1e")
	client := &decisionChatClient{responseForRequest: func(request llm.ChatRequest) string {
		if strings.Contains(request.Messages[1].Content, "absent=true") {
			return `{"impact":"additive","public_contract":"new option","reason":"The new option is optional."}`
		}
		return `{"impact":"unresolved","public_contract":"","reason":"Check prior source.","context_requests":[{"path":"new.txt","source":"old","start_line":1,"line_count":1}]}`
	}}
	result, err := (Generator{GitExecutor: executor, Client: client}).Generate(context.Background(), decisionOptions())
	require.NoError(t, err)
	require.True(t, result.Context[0].Absent)
	require.NotContains(t, executor.calls, "show base123:new.txt")
}

func TestGenerateFailsClosedForUnresolvedAndBinaryEvidence(t *testing.T) {
	for _, testCase := range []struct{ name, diff, response string }{
		{"unresolved", "", `{"impact":"unresolved","public_contract":"supported behavior","reason":"Public impact is still unclear."}`},
		{"binary", "diff --git a/model.bin b/model.bin\nBinary files a/model.bin and b/model.bin differ\n", `{"impact":"compatible","public_contract":"","reason":"No change."}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			executor := newDecisionGitExecutor("fix: update behavior\n\x1e")
			if testCase.diff != "" {
				executor.responses["diff --unified=3 "+testRange+" -- . :(exclude)CHANGELOG.md"] = testCase.diff
			}
			result, err := (Generator{GitExecutor: executor, Client: &decisionChatClient{response: testCase.response}}).Generate(context.Background(), decisionOptions())
			require.Error(t, err)
			require.Contains(t, err.Error(), "unresolved")
			require.Empty(t, result.Bump)
			require.NotEmpty(t, result.Packets)
			require.Len(t, result.EvidenceSHA256, 64)
		})
	}
}

func TestGenerateRetainsPartialEvidenceOnProviderFailure(t *testing.T) {
	result, err := (Generator{GitExecutor: newDecisionGitExecutor("fix: behavior\n\x1e"), Client: &decisionChatClient{err: errors.New("provider offline")}}).Generate(context.Background(), decisionOptions())
	require.Error(t, err)
	require.Len(t, result.Requests, 1)
	require.Len(t, result.Packets, 1)
	require.NotEmpty(t, result.Coverage)
	require.Len(t, result.EvidenceSHA256, 64)
	require.Empty(t, result.Bump)
}

func TestGenerateEnforcesContextRoundAndRuneBudgets(t *testing.T) {
	for _, testCase := range []struct{ name, content, want string }{
		{"round", "One line.\n", "round limit"},
		{"runes", strings.Repeat("界", contextRuneLimit) + "\n", "rune limit"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			executor := newDecisionGitExecutor("fix: behavior\n\x1e")
			executor.responses["ls-tree -z --name-only source456 -- context.txt"] = "context.txt\x00"
			executor.responses["cat-file -t source456:context.txt"] = "blob\n"
			executor.responses["show source456:context.txt"] = testCase.content
			response := `{"impact":"unresolved","public_contract":"","reason":"Need context.","context_requests":[{"path":"context.txt","source":"new","start_line":1,"line_count":1}]}`
			result, err := (Generator{GitExecutor: executor, Client: &decisionChatClient{response: response}}).Generate(context.Background(), decisionOptions())
			require.Error(t, err)
			require.Contains(t, err.Error(), testCase.want)
			require.LessOrEqual(t, len(result.Requests), 3)
			require.Empty(t, result.Bump)
		})
	}
}

func TestGenerateEnforcesGlobalProviderCallLimit(t *testing.T) {
	executor := newDecisionGitExecutor("fix: behavior\n\x1e")
	executor.responses["diff --unified=3 "+testRange+" -- . :(exclude)CHANGELOG.md"] = "diff --git a/noise.txt b/noise.txt\n@@ -1 +1 @@\n+" + strings.Repeat("x", 1500000) + "\n"
	executor.responses["ls-tree -z --name-only source456 -- context.txt"] = "context.txt\x00"
	executor.responses["cat-file -t source456:context.txt"] = "blob\n"
	executor.responses["show source456:context.txt"] = "Known source.\n"
	client := &decisionChatClient{responseForRequest: func(request llm.ChatRequest) string {
		if strings.Contains(request.Messages[1].Content, "Known source.") {
			return `{"impact":"compatible","public_contract":"","reason":"The supported behavior is preserved."}`
		}
		return `{"impact":"unresolved","public_contract":"","reason":"Need context.","context_requests":[{"path":"context.txt","source":"new","start_line":1,"line_count":1}]}`
	}}
	result, err := (Generator{GitExecutor: executor, Client: client}).Generate(context.Background(), decisionOptions())
	require.Error(t, err)
	require.Contains(t, err.Error(), "provider call limit")
	require.Len(t, result.Requests, providerCallLimit)
	require.LessOrEqual(t, len(client.requests), providerCallLimit)
}

func TestGenerateRejectsOversizeCompleteEvidenceBeforeCalls(t *testing.T) {
	executor := newDecisionGitExecutor("fix: behavior\n\x1e")
	executor.responses["diff --unified=3 "+testRange+" -- . :(exclude)CHANGELOG.md"] = "diff --git a/noise.txt b/noise.txt\n+" + strings.Repeat("x", 2200000) + "\n"
	client := &decisionChatClient{response: `{"impact":"compatible","public_contract":"","reason":"No change."}`}
	result, err := (Generator{GitExecutor: executor, Client: client}).Generate(context.Background(), decisionOptions())
	require.Error(t, err)
	require.Contains(t, err.Error(), "complete evidence requires")
	require.Empty(t, client.requests)
	require.NotEmpty(t, result.Packets)
	require.Equal(t, result.Coverage[3].RuneCount, result.Coverage[3].CoveredRunes)
}

func TestGenerateRetainsMalformedProviderResponse(t *testing.T) {
	raw := "not a valid decision"
	result, err := (Generator{GitExecutor: newDecisionGitExecutor("fix: behavior\n\x1e"), Client: &decisionChatClient{response: raw}}).Generate(context.Background(), decisionOptions())
	require.Error(t, err)
	require.Len(t, result.Responses, 1)
	require.Equal(t, raw, result.Responses[0].Content)
	require.Equal(t, "candidate", result.Responses[0].Stage)
	require.Equal(t, result.Packets[0].ID, result.Responses[0].PacketID)
}

func TestGenerateRejectsOversizeAuditWithoutTruncatingResponse(t *testing.T) {
	response := fmt.Sprintf(`{"impact":"compatible","public_contract":"","reason":%q}`, strings.Repeat("x", 80000))
	client := &decisionChatClient{response: response}
	result, err := (Generator{GitExecutor: newDecisionGitExecutor("fix: behavior\n\x1e"), Client: client}).Generate(context.Background(), decisionOptions())
	require.Error(t, err)
	require.Contains(t, err.Error(), "audit submitted request exceeds 64000 rune limit")
	require.Len(t, client.requests, 1)
	require.Len(t, result.Requests, 2)
	require.Equal(t, response, result.Responses[0].Content)
	require.Contains(t, result.Requests[1].Messages[1].Content, strings.Repeat("x", 80000))
}

func TestGenerateRejectsNonBlobContextWithoutReadingContent(t *testing.T) {
	for _, objectType := range []string{"tree", "commit"} {
		t.Run(objectType, func(t *testing.T) {
			executor := newDecisionGitExecutor("fix: behavior\n\x1e")
			executor.responses["ls-tree -z --name-only source456 -- docs"] = "docs\x00"
			executor.responses["cat-file -t source456:docs"] = objectType + "\n"
			response := `{"impact":"unresolved","public_contract":"","reason":"Need committed context.","context_requests":[{"path":"docs","source":"new","start_line":1,"line_count":1}]}`
			result, err := (Generator{GitExecutor: executor, Client: &decisionChatClient{response: response}}).Generate(context.Background(), decisionOptions())
			require.Error(t, err)
			require.Contains(t, err.Error(), "not a committed blob")
			require.NotContains(t, executor.calls, "show source456:docs")
			require.Len(t, result.Findings, 1)
			require.Empty(t, result.Context)
		})
	}
}
