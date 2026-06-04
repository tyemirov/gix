package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	syncRefreshIntegrationTimeout          = 10 * time.Second
	syncRefreshIntegrationRunCommand       = "run"
	syncRefreshIntegrationModulePath       = "."
	syncRefreshIntegrationLogLevelFlag     = "--log-level"
	syncRefreshIntegrationErrorLogLevel    = "error"
	syncRefreshIntegrationGitInvocationLog = "git-invocations.log"
	syncRefreshIntegrationAPIKeyVariable   = "TEST_GIX_SYNC_REFRESH_LLM_KEY"
)

func TestSyncCommitsDirtyMasterWorktreeOnGeneratedBranch(testInstance *testing.T) {
	testInstance.Helper()

	expectedGeneratedBranchName := "gix/sync-dirty-work"
	repositoryRoot := integrationRepositoryRoot(testInstance)
	realGitPath, lookupError := exec.LookPath("git")
	require.NoError(testInstance, lookupError)

	gitInvocationLog := filepath.Join(testInstance.TempDir(), syncRefreshIntegrationGitInvocationLog)
	gitStubScript := []byte(strings.Join([]string{
		"#!/bin/sh",
		"echo \"$@\" >> " + gitInvocationLog,
		"if [ \"$1\" = \"pull\" ] && [ \"$2\" = \"--rebase\" ]; then exit 42; fi",
		"exec " + realGitPath + " \"$@\"",
	}, "\n") + "\n")
	pathVariable := buildStubbedExecutablePath(testInstance, "git", string(gitStubScript))

	remotePath := filepath.Join(testInstance.TempDir(), "remote.git")
	remoteInitCommand := exec.Command("git", "init", "--bare", remotePath)
	remoteInitCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, remoteInitCommand.Run())

	repositoryPath := filepath.Join(testInstance.TempDir(), "worktree")
	initCommand := exec.Command("git", "init", "--initial-branch=master", repositoryPath)
	initCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, initCommand.Run())

	configNameCommand := exec.Command("git", "-C", repositoryPath, "config", "user.name", "Sync Refresh")
	configNameCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, configNameCommand.Run())

	configEmailCommand := exec.Command("git", "-C", repositoryPath, "config", "user.email", "sync-refresh@example.com")
	configEmailCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, configEmailCommand.Run())

	remoteAddCommand := exec.Command("git", "-C", repositoryPath, "remote", "add", "origin", remotePath)
	remoteAddCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, remoteAddCommand.Run())

	readmePath := filepath.Join(repositoryPath, "README.md")
	require.NoError(testInstance, os.WriteFile(readmePath, []byte("initial\n"), 0o644))

	addCommand := exec.Command("git", "-C", repositoryPath, "add", "README.md")
	addCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, addCommand.Run())

	commitCommand := exec.Command("git", "-C", repositoryPath, "commit", "-m", "initial commit")
	commitCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, commitCommand.Run())

	pushCommand := exec.Command("git", "-C", repositoryPath, "push", "-u", "origin", "master")
	pushCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, pushCommand.Run())

	upstreamPath := filepath.Join(testInstance.TempDir(), "upstream")
	cloneCommand := exec.Command("git", "clone", remotePath, upstreamPath)
	cloneCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, cloneCommand.Run())

	upstreamNameCommand := exec.Command("git", "-C", upstreamPath, "config", "user.name", "Sync Refresh")
	upstreamNameCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, upstreamNameCommand.Run())

	upstreamEmailCommand := exec.Command("git", "-C", upstreamPath, "config", "user.email", "sync-refresh@example.com")
	upstreamEmailCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, upstreamEmailCommand.Run())

	upstreamFilePath := filepath.Join(upstreamPath, "UPSTREAM.md")
	require.NoError(testInstance, os.WriteFile(upstreamFilePath, []byte("remote update\n"), 0o644))

	upstreamAddCommand := exec.Command("git", "-C", upstreamPath, "add", "UPSTREAM.md")
	upstreamAddCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, upstreamAddCommand.Run())

	upstreamCommitCommand := exec.Command("git", "-C", upstreamPath, "commit", "-m", "remote update")
	upstreamCommitCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, upstreamCommitCommand.Run())

	upstreamPushCommand := exec.Command("git", "-C", upstreamPath, "push", "origin", "master")
	upstreamPushCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, upstreamPushCommand.Run())

	require.NoError(testInstance, os.WriteFile(readmePath, []byte("modified locally\n"), 0o644))

	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"docs: sync dirty work"}}]}`))
	}))
	testInstance.Cleanup(llmServer.Close)

	configurationPath := filepath.Join(testInstance.TempDir(), "config.yaml")
	configurationContent := fmt.Sprintf(`common:
  log_level: error
  log_format: console
  require_clean: false
operations:
  - command: ["sync"]
    with:
      remote: origin
  - command: ["message", "commit"]
    with:
      api_key_env: %s
      base_url: %q
      model: mock-model
      diff_source: staged
      max_completion_tokens: 64
      temperature: 0
      timeout_seconds: 5
`, syncRefreshIntegrationAPIKeyVariable, llmServer.URL)
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))

	commandArguments := []string{
		syncRefreshIntegrationRunCommand,
		syncRefreshIntegrationModulePath,
		"--config",
		configurationPath,
		syncRefreshIntegrationLogLevelFlag,
		syncRefreshIntegrationErrorLogLevel,
		"sync",
		"master",
		"--roots",
		repositoryPath,
	}

	output, runError := runFailingIntegrationCommand(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncRefreshIntegrationAPIKeyVariable: "test-key",
			},
		},
		syncRefreshIntegrationTimeout,
		commandArguments,
	)
	require.Error(testInstance, runError)
	testInstance.Logf("sync output:\n%s", output)
	require.Contains(testInstance, output, "strict sync requires a GitHub repository remote")
	require.NotContains(testInstance, output, "worktree is dirty")

	invocationLogContents, readError := os.ReadFile(gitInvocationLog)
	require.NoError(testInstance, readError)
	invocationLog := string(invocationLogContents)
	require.Contains(testInstance, invocationLog, "switch -c "+expectedGeneratedBranchName+" origin/master")
	require.Contains(testInstance, invocationLog, "add --all -- README.md")
	require.Contains(testInstance, invocationLog, "commit -m docs: sync dirty work")
	require.NotContains(testInstance, string(invocationLogContents), "pull --ff-only")
	require.NotContains(testInstance, string(invocationLogContents), "pull --rebase")

	localFileContents, localReadError := os.ReadFile(readmePath)
	require.NoError(testInstance, localReadError)
	require.Equal(testInstance, "modified locally\n", string(localFileContents))
	require.Equal(testInstance, expectedGeneratedBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
	require.Equal(testInstance, "docs: sync dirty work", strings.TrimSpace(runGit(testInstance, repositoryPath, "log", "-1", "--pretty=%s")))
}
