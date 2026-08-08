package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type stashConflictFixture struct {
	Path        string
	Base        string
	Target      string
	Stashed     string
	Expected    string
	Permissions os.FileMode
}

type syncStashConflictResult struct {
	Output         string
	RunError       error
	RequestPrompts []string
	RepositoryPath string
	TargetCommit   string
}

func TestSyncStashResolvesB089SizedTrackerDeterministically(testInstance *testing.T) {
	testInstance.Helper()

	fixture := b089SizedTrackerFixture()
	result := runSyncStashConflictScenario(testInstance, []stashConflictFixture{fixture}, func(string, int) string { return "" })

	require.NoError(testInstance, result.RunError, result.Output)
	require.Empty(testInstance, result.RequestPrompts)
	require.Contains(testInstance, result.Output, "STASH_CONFLICT")
	require.Contains(testInstance, result.Output, "resolved hunk")
	require.Contains(testInstance, result.Output, "deterministically")
	resolutionOffset := strings.Index(result.Output, "stash conflict resolution completed")
	syncedOffset := strings.Index(result.Output, "SYNCED:")
	require.Greater(testInstance, resolutionOffset, -1)
	require.Greater(testInstance, syncedOffset, resolutionOffset)

	actualContents := readTextFile(testInstance, filepath.Join(result.RepositoryPath, fixture.Path))
	require.Equal(testInstance, fixture.Expected, actualContents)
	require.NotContains(testInstance, actualContents, "<<<<<<<")
	require.Equal(testInstance, result.TargetCommit, strings.TrimSpace(runGit(testInstance, result.RepositoryPath, "rev-parse", "HEAD")))
	require.Equal(testInstance, " M "+fixture.Path, strings.TrimSuffix(runGit(testInstance, result.RepositoryPath, "status", "--porcelain"), "\n"))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, result.RepositoryPath, "diff", "--cached", "--name-only")))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, result.RepositoryPath, "stash", "list")))
}

func TestSyncStashResolvesSemanticHunkThroughBoundedStructuredAI(testInstance *testing.T) {
	testInstance.Helper()

	fixture := semanticStashConflictFixture("resolver.sh")
	result := runSyncStashConflictScenario(testInstance, []stashConflictFixture{fixture}, func(prompt string, _ int) string {
		return structuredHunkResolution(prompt, "setting=target\nsetting=stashed\n")
	})

	require.NoError(testInstance, result.RunError, result.Output)
	require.Len(testInstance, result.RequestPrompts, 1)
	require.Less(testInstance, len(result.RequestPrompts[0]), 20_000)
	require.NotContains(testInstance, result.RequestPrompts[0], "DO_NOT_SEND_WHOLE_FILE_TOP")
	require.NotContains(testInstance, result.RequestPrompts[0], "DO_NOT_SEND_WHOLE_FILE_BOTTOM")
	require.Contains(testInstance, result.RequestPrompts[0], "BASE:\nsetting=base")
	require.Contains(testInstance, result.RequestPrompts[0], "TARGET:\nsetting=target")
	require.Contains(testInstance, result.RequestPrompts[0], "INCOMING:\nsetting=stashed")

	resolvedPath := filepath.Join(result.RepositoryPath, fixture.Path)
	require.Equal(testInstance, fixture.Expected, readTextFile(testInstance, resolvedPath))
	fileInfo, statErr := os.Stat(resolvedPath)
	require.NoError(testInstance, statErr)
	require.Equal(testInstance, os.FileMode(0o755), fileInfo.Mode().Perm())
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, result.RepositoryPath, "stash", "list")))
}

func TestSyncStashRetainsConflictAfterTwoLossyHunkResponses(testInstance *testing.T) {
	testInstance.Helper()

	fixture := semanticStashConflictFixture("settings.txt")
	result := runSyncStashConflictScenario(testInstance, []stashConflictFixture{fixture}, func(prompt string, _ int) string {
		return structuredHunkResolution(prompt, "setting=target\n")
	})

	require.Error(testInstance, result.RunError)
	require.Len(testInstance, result.RequestPrompts, 2)
	require.Contains(testInstance, result.RequestPrompts[1], "prior hunk response was rejected")
	require.Contains(testInstance, result.RequestPrompts[1], "does not preserve target and stashed intent")
	require.Contains(testInstance, result.Output, "AI_STASH_HANDOFF")
	require.NotContains(testInstance, result.Output, "SYNCED:")
	require.Equal(testInstance, fixture.Path, strings.TrimSpace(runGit(testInstance, result.RepositoryPath, "diff", "--name-only", "--diff-filter=U")))
	require.Contains(testInstance, readTextFile(testInstance, filepath.Join(result.RepositoryPath, fixture.Path)), "<<<<<<<")
	require.NotEmpty(testInstance, strings.TrimSpace(runGit(testInstance, result.RepositoryPath, "stash", "list")))
	require.Equal(testInstance, result.TargetCommit, strings.TrimSpace(runGit(testInstance, result.RepositoryPath, "rev-parse", "HEAD")))
}

func TestSyncStashDoesNotWriteFirstFileWhenSecondFileFailsValidation(testInstance *testing.T) {
	testInstance.Helper()

	firstFixture := semanticStashConflictFixture("a-settings.txt")
	secondFixture := semanticStashConflictFixture("b-settings.txt")
	result := runSyncStashConflictScenario(testInstance, []stashConflictFixture{firstFixture, secondFixture}, func(prompt string, _ int) string {
		if strings.Contains(prompt, "Path: a-settings.txt") {
			return structuredHunkResolution(prompt, "setting=target\nsetting=stashed\n")
		}
		return structuredHunkResolution(prompt, "setting=target\n")
	})

	require.Error(testInstance, result.RunError)
	require.Len(testInstance, result.RequestPrompts, 3)
	require.NotContains(testInstance, result.Output, "SYNCED:")
	require.Equal(
		testInstance,
		"a-settings.txt\nb-settings.txt",
		strings.TrimSpace(runGit(testInstance, result.RepositoryPath, "diff", "--name-only", "--diff-filter=U")),
	)
	for _, fixture := range []stashConflictFixture{firstFixture, secondFixture} {
		conflictedContents := readTextFile(testInstance, filepath.Join(result.RepositoryPath, fixture.Path))
		require.Contains(testInstance, conflictedContents, "<<<<<<<", fixture.Path)
		require.NotEqual(testInstance, fixture.Expected, conflictedContents, fixture.Path)
	}
	require.NotEmpty(testInstance, strings.TrimSpace(runGit(testInstance, result.RepositoryPath, "stash", "list")))
	require.Equal(testInstance, result.TargetCommit, strings.TrimSpace(runGit(testInstance, result.RepositoryPath, "rev-parse", "HEAD")))
}

func TestSyncStashRejectsBinaryConflictBeforeLLMAndRetainsStash(testInstance *testing.T) {
	testInstance.Helper()

	fixture := stashConflictFixture{
		Path:        "fixture.bin",
		Base:        "base\x00value\n",
		Target:      "target\x00value\n",
		Stashed:     "stashed\x00value\n",
		Permissions: 0o644,
	}
	result := runSyncStashConflictScenario(testInstance, []stashConflictFixture{fixture}, func(string, int) string { return "" })

	require.Error(testInstance, result.RunError)
	require.Empty(testInstance, result.RequestPrompts)
	require.Contains(testInstance, result.Output, "binary and cannot be resolved as text")
	require.Contains(testInstance, result.Output, "AI_STASH_HANDOFF")
	require.NotContains(testInstance, result.Output, "SYNCED:")
	require.Equal(testInstance, fixture.Path, strings.TrimSpace(runGit(testInstance, result.RepositoryPath, "diff", "--name-only", "--diff-filter=U")))
	require.NotEmpty(testInstance, strings.TrimSpace(runGit(testInstance, result.RepositoryPath, "stash", "list")))
	require.Equal(testInstance, result.TargetCommit, strings.TrimSpace(runGit(testInstance, result.RepositoryPath, "rev-parse", "HEAD")))
}

func b089SizedTrackerFixture() stashConflictFixture {
	var baseBuilder strings.Builder
	baseBuilder.WriteString("# ISSUES\n\n## BugFixes\n")
	for issueIndex := 0; issueIndex < 2880; issueIndex++ {
		_, _ = fmt.Fprintf(&baseBuilder, "- [x] [B%04d] Stable issue content that must remain byte exact.\n", issueIndex)
	}
	baseBuilder.WriteString("\n## Improvements\n- [ ] [B089] Base provider contract.\n")
	baseContents := baseBuilder.String()

	targetContents := strings.Replace(baseContents, "- [ ] [B089] Base provider contract.", "- [x] [B089] Target provider contract.", 1)
	var targetAdditions strings.Builder
	for issueIndex := 0; issueIndex < 191; issueIndex++ {
		_, _ = fmt.Fprintf(&targetAdditions, "- [x] [I%04d] Target branch improvement.\n", issueIndex)
	}
	targetContents += targetAdditions.String()

	var stashedAdditions strings.Builder
	for issueIndex := 0; issueIndex < 226; issueIndex++ {
		_, _ = fmt.Fprintf(&stashedAdditions, "- [ ] [M%04d] Stashed operator improvement.\n", issueIndex)
	}
	stashedContents := baseContents + stashedAdditions.String()

	return stashConflictFixture{
		Path:        filepath.ToSlash(filepath.Join(".mprlab", "ISSUES.md")),
		Base:        baseContents,
		Target:      targetContents,
		Stashed:     stashedContents,
		Expected:    targetContents + stashedAdditions.String(),
		Permissions: 0o644,
	}
}

func semanticStashConflictFixture(path string) stashConflictFixture {
	var prefixBuilder strings.Builder
	prefixBuilder.WriteString("DO_NOT_SEND_WHOLE_FILE_TOP\n")
	for lineIndex := 0; lineIndex < 32; lineIndex++ {
		_, _ = fmt.Fprintf(&prefixBuilder, "stable prefix line %02d\n", lineIndex)
	}
	prefix := prefixBuilder.String()

	var suffixBuilder strings.Builder
	for lineIndex := 0; lineIndex < 32; lineIndex++ {
		_, _ = fmt.Fprintf(&suffixBuilder, "stable suffix line %02d\n", lineIndex)
	}
	suffixBuilder.WriteString("DO_NOT_SEND_WHOLE_FILE_BOTTOM\n")
	suffix := suffixBuilder.String()

	return stashConflictFixture{
		Path:        path,
		Base:        prefix + "setting=base\n" + suffix,
		Target:      prefix + "setting=target\n" + suffix,
		Stashed:     prefix + "setting=stashed\n" + suffix,
		Expected:    prefix + "setting=target\nsetting=stashed\n" + suffix,
		Permissions: 0o755,
	}
}

func runSyncStashConflictScenario(
	testInstance *testing.T,
	fixtures []stashConflictFixture,
	responder func(prompt string, requestIndex int) string,
) syncStashConflictResult {
	testInstance.Helper()

	repositoryRoot := integrationRepositoryRoot(testInstance)
	remotePath := filepath.Join(testInstance.TempDir(), "remote.git")
	runGitWithDir(testInstance, "", "init", "--bare", remotePath)

	repositoryPath := filepath.Join(testInstance.TempDir(), "worktree")
	runGitWithDir(testInstance, "", "init", "--initial-branch=master", repositoryPath)
	configureGitIdentity(testInstance, repositoryPath)
	runGit(testInstance, repositoryPath, "remote", "add", "origin", remotePath)

	for _, fixture := range fixtures {
		writeStashConflictFixture(testInstance, repositoryPath, fixture.Path, fixture.Base, fixture.Permissions)
		runGit(testInstance, repositoryPath, "add", fixture.Path)
	}
	runGit(testInstance, repositoryPath, "commit", "-m", "seed stash conflict fixtures")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")
	runGit(testInstance, repositoryPath, "switch", "-c", "feature/stashed-work")

	runGit(testInstance, repositoryPath, "switch", "master")
	for _, fixture := range fixtures {
		writeStashConflictFixture(testInstance, repositoryPath, fixture.Path, fixture.Target, fixture.Permissions)
		runGit(testInstance, repositoryPath, "add", fixture.Path)
	}
	runGit(testInstance, repositoryPath, "commit", "-m", "change target branch fixtures")
	runGit(testInstance, repositoryPath, "push", "origin", "master")
	targetCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "master"))

	runGit(testInstance, repositoryPath, "switch", "feature/stashed-work")
	for _, fixture := range fixtures {
		writeStashConflictFixture(testInstance, repositoryPath, fixture.Path, fixture.Stashed, fixture.Permissions)
	}

	var requestMutex sync.Mutex
	requestPrompts := make([]string, 0)
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		prompt, promptErr := resolverPromptFromRequest(request)
		if promptErr != nil {
			http.Error(responseWriter, promptErr.Error(), http.StatusBadRequest)
			return
		}
		requestMutex.Lock()
		requestIndex := len(requestPrompts)
		requestPrompts = append(requestPrompts, prompt)
		requestMutex.Unlock()
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(
			responseWriter,
			`{"choices":[{"message":{"role":"assistant","content":%q}}]}`,
			responder(prompt, requestIndex),
		)
	}))
	testInstance.Cleanup(llmServer.Close)

	configurationPath := filepath.Join(testInstance.TempDir(), "config.yml")
	configurationContents := fmt.Sprintf(`common:
  log_level: info
  log_format: console
  require_clean: false
github:
  credential: test-github-key
llm:
  openai:
    priority: 1
    model: mock-model
    base_url: %q
    credential: test-key
  llm_proxy:
    priority: 2
    provider: meta
    model: muse-spark-1.1
    base_url: "https://llm-proxy.example"
    credential: test-proxy-key
  max_completion_tokens: 64
  temperature: 0
  timeout_seconds: 5
operations:
  - command: ["sync"]
    with:
      remote: origin
      roots:
        - .
`, llmServer.URL)
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContents), 0o600))

	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	output, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		repositoryPath,
		nil,
		syncRefreshIntegrationTimeout,
		[]string{
			"--config",
			configurationPath,
			"--log-level",
			"info",
			"sync",
			"master",
			"--stash",
		},
	)
	requestMutex.Lock()
	capturedPrompts := append([]string(nil), requestPrompts...)
	requestMutex.Unlock()
	return syncStashConflictResult{
		Output:         output,
		RunError:       runError,
		RequestPrompts: capturedPrompts,
		RepositoryPath: repositoryPath,
		TargetCommit:   targetCommit,
	}
}

func writeStashConflictFixture(testInstance *testing.T, repositoryPath string, relativePath string, content string, permissions os.FileMode) {
	testInstance.Helper()
	absolutePath := filepath.Join(repositoryPath, filepath.FromSlash(relativePath))
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(absolutePath), 0o755))
	require.NoError(testInstance, os.WriteFile(absolutePath, []byte(content), permissions))
	require.NoError(testInstance, os.Chmod(absolutePath, permissions))
}

func resolverPromptFromRequest(request *http.Request) (string, error) {
	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if decodeErr := json.NewDecoder(request.Body).Decode(&payload); decodeErr != nil {
		return "", fmt.Errorf("decode resolver request: %w", decodeErr)
	}
	for _, message := range payload.Messages {
		if strings.Contains(message.Content, "Hunk ID: ") {
			return strings.Join(resolverUserMessages(payload.Messages), "\n"), nil
		}
	}
	return "", fmt.Errorf("resolver request did not contain a hunk id")
}

func resolverUserMessages(messages []struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}) []string {
	userMessages := make([]string, 0, len(messages))
	for _, message := range messages {
		if message.Role == "user" {
			userMessages = append(userMessages, message.Content)
		}
	}
	return userMessages
}

func structuredHunkResolution(prompt string, content string) string {
	hunkID := resolverPromptValue(prompt, "Hunk ID: ")
	responseBytes, marshalErr := json.Marshal(map[string]string{
		"hunk_id": hunkID,
		"content": content,
	})
	if marshalErr != nil {
		panic(marshalErr)
	}
	return string(responseBytes)
}

func resolverPromptValue(prompt string, prefix string) string {
	valueOffset := strings.Index(prompt, prefix)
	if valueOffset < 0 {
		return ""
	}
	value := prompt[valueOffset+len(prefix):]
	if lineEnd := strings.IndexByte(value, '\n'); lineEnd >= 0 {
		value = value[:lineEnd]
	}
	return strings.TrimSpace(value)
}
