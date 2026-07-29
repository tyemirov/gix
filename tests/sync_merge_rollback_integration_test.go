package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncRejectedPullRequestBaseResolutionRollsBackCleanBranch(testInstance *testing.T) {
	testInstance.Helper()

	const (
		baseBranchName   = "feature/review-base"
		targetBranchName = "feature/review-target"
		conflictedPath   = "CHANGELOG.md"
	)

	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	initialContent := "stable preface\nbase release note\nstable epilogue\n"
	targetContent := strings.Replace(initialContent, "base release note", "target release note", 1)
	incomingContent := strings.Replace(initialContent, "base release note", "incoming release note", 1)
	conflictedFilePath := filepath.Join(repositoryPath, conflictedPath)
	require.NoError(testInstance, os.WriteFile(conflictedFilePath, []byte(initialContent), 0o644))
	runGit(testInstance, repositoryPath, "add", conflictedPath)
	runGit(testInstance, repositoryPath, "commit", "-m", "seed release notes")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", baseBranchName)
	require.NoError(testInstance, os.WriteFile(conflictedFilePath, []byte(incomingContent), 0o644))
	runGit(testInstance, repositoryPath, "add", conflictedPath)
	runGit(testInstance, repositoryPath, "commit", "-m", "update incoming release note")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", baseBranchName)

	runGit(testInstance, repositoryPath, "switch", "master")
	runGit(testInstance, repositoryPath, "switch", "-c", targetBranchName)
	require.NoError(testInstance, os.WriteFile(conflictedFilePath, []byte(targetContent), 0o644))
	runGit(testInstance, repositoryPath, "add", conflictedPath)
	runGit(testInstance, repositoryPath, "commit", "-m", "update target release note")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", targetBranchName)
	targetCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))

	var requestCount atomic.Int64
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		requestCount.Add(1)
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(
			responseWriter,
			`{"choices":[{"message":{"role":"assistant","content":%q}}]}`,
			semanticMergeResponse("truncated resolution\n"),
		)
	}))
	testInstance.Cleanup(llmServer.Close)

	configurationPath := filepath.Join(testInstance.TempDir(), "config.yml")
	configurationContent := fmt.Sprintf(`common:
  log_level: error
  log_format: console
  require_clean: true
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
      require_clean: true
`, llmServer.URL)
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))

	gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	require.NoError(
		testInstance,
		os.WriteFile(
			githubLogPath,
			[]byte("created-pr --base "+baseBranchName+" --head "+targetBranchName+"\n"),
			0o600,
		),
	)

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: buildSyncMergedBranchExecutablePath(testInstance),
			EnvironmentOverrides: map[string]string{
				syncRefreshIntegrationAPIKeyVariable: "test-key",
				syncMergedBranchGitLogVariable:       gitLogPath,
				syncMergedBranchGitHubLogVariable:    githubLogPath,
				syncMergedBranchNameVariable:         targetBranchName,
				syncMergedBranchMergedVariable:       "false",
			},
		},
		syncMergedBranchIntegrationTimeout,
		"",
		[]string{
			syncRefreshIntegrationRunCommand,
			syncRefreshIntegrationModulePath,
			"--config",
			configurationPath,
			syncRefreshIntegrationLogLevelFlag,
			syncRefreshIntegrationErrorLogLevel,
			"sync",
			targetBranchName,
			"--roots",
			repositoryPath,
		},
	)
	require.Error(testInstance, runError)
	require.Contains(testInstance, output, "AI_MERGE_ROLLBACK")
	require.Contains(testInstance, output, "does not preserve OURS replacement intent")
	require.Contains(testInstance, output, "all semantic attempts exhausted")
	require.Contains(testInstance, output, "failed merge was aborted")
	require.NotContains(testInstance, output, "AI_MERGE_HANDOFF")
	require.Equal(testInstance, int64(mergeConflictResolutionAttemptCountForTest), requestCount.Load())

	require.Equal(testInstance, targetBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, targetCommit, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD")))
	require.Equal(testInstance, targetContent, readTextFile(testInstance, conflictedFilePath))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))

	mergeHeadCommand := exec.Command("git", "-C", repositoryPath, "rev-parse", "--verify", "MERGE_HEAD")
	mergeHeadCommand.Env = buildGitCommandEnvironment(nil)
	mergeHeadOutput, mergeHeadError := mergeHeadCommand.CombinedOutput()
	require.Error(testInstance, mergeHeadError, string(mergeHeadOutput))

	gitLog := readTextFile(testInstance, gitLogPath)
	require.Contains(testInstance, gitLog, "merge --no-edit origin/"+baseBranchName)
	require.Contains(testInstance, gitLog, "merge --abort")
	require.NotContains(testInstance, gitLog, "commit --no-edit")
	require.NotContains(testInstance, gitLog, "push origin "+targetBranchName)
}
