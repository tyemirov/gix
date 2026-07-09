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
		"if [ \"$1\" = \"status\" ] && [ \"$2\" = \"--porcelain\" ]; then",
		"  " + realGitPath + " \"$@\"",
		"  status_exit=$?",
		"  if [ \"$status_exit\" -eq 0 ]; then",
		"    printf '%s\\n' '!! python/llm_proxy_client/__pycache__/client.cpython-313.pyc' '!! python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc'",
		"  fi",
		"  exit \"$status_exit\"",
		"fi",
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

	statusConfigCommand := exec.Command("git", "-C", repositoryPath, "config", "status.showUntrackedFiles", "all")
	statusConfigCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, statusConfigCommand.Run())

	remoteAddCommand := exec.Command("git", "-C", repositoryPath, "remote", "add", "origin", remotePath)
	remoteAddCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, remoteAddCommand.Run())

	gitignorePath := filepath.Join(repositoryPath, ".gitignore")
	require.NoError(testInstance, os.WriteFile(gitignorePath, []byte("__pycache__/\n"), 0o644))

	readmePath := filepath.Join(repositoryPath, "README.md")
	require.NoError(testInstance, os.WriteFile(readmePath, []byte("initial\nmiddle\nstable\n"), 0o644))

	addCommand := exec.Command("git", "-C", repositoryPath, "add", ".gitignore", "README.md")
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

	upstreamFilePath := filepath.Join(upstreamPath, "README.md")
	require.NoError(testInstance, os.WriteFile(upstreamFilePath, []byte("remote update\nmiddle\nstable\n"), 0o644))

	upstreamAddCommand := exec.Command("git", "-C", upstreamPath, "add", "README.md")
	upstreamAddCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, upstreamAddCommand.Run())

	upstreamCommitCommand := exec.Command("git", "-C", upstreamPath, "commit", "-m", "remote update")
	upstreamCommitCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, upstreamCommitCommand.Run())

	upstreamPushCommand := exec.Command("git", "-C", upstreamPath, "push", "origin", "master")
	upstreamPushCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, upstreamPushCommand.Run())

	require.NoError(testInstance, os.WriteFile(readmePath, []byte("initial\nmiddle\nmodified locally\n"), 0o644))
	eggInfoPath := filepath.Join(repositoryPath, "python", "llm_proxy_client.egg-info")
	require.NoError(testInstance, os.MkdirAll(eggInfoPath, 0o755))
	require.NoError(testInstance, os.WriteFile(filepath.Join(eggInfoPath, "PKG-INFO"), []byte("metadata\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(eggInfoPath, "SOURCES.txt"), []byte("sources\n"), 0o644))
	clientCachePath := filepath.Join(repositoryPath, "python", "llm_proxy_client", "__pycache__")
	require.NoError(testInstance, os.MkdirAll(clientCachePath, 0o755))
	require.NoError(testInstance, os.WriteFile(filepath.Join(clientCachePath, "client.cpython-313.pyc"), []byte("cache\n"), 0o644))
	testCachePath := filepath.Join(repositoryPath, "python", "tests", "__pycache__")
	require.NoError(testInstance, os.MkdirAll(testCachePath, 0o755))
	require.NoError(testInstance, os.WriteFile(filepath.Join(testCachePath, "test_client.cpython-313-pytest-9.0.3.pyc"), []byte("cache\n"), 0o644))

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
	require.NotContains(testInstance, output, "would be overwritten by checkout")

	invocationLogContents, readError := os.ReadFile(gitInvocationLog)
	require.NoError(testInstance, readError)
	invocationLog := string(invocationLogContents)
	require.Contains(testInstance, invocationLog, "check-ignore --stdin")
	require.Contains(testInstance, invocationLog, "switch -c "+expectedGeneratedBranchName+"\n")
	require.NotContains(testInstance, invocationLog, "switch -c "+expectedGeneratedBranchName+" origin/master")
	require.Contains(testInstance, invocationLog, "add --all -- README.md")
	require.Contains(testInstance, invocationLog, "add --all -- python/llm_proxy_client.egg-info/PKG-INFO python/llm_proxy_client.egg-info/SOURCES.txt")
	require.NotContains(testInstance, invocationLog, "add --all -- python/llm_proxy_client/__pycache__")
	require.NotContains(testInstance, invocationLog, "add --all -- python/tests/__pycache__")
	require.Contains(testInstance, invocationLog, "commit -m docs: sync dirty work")
	require.NotContains(testInstance, string(invocationLogContents), "pull --ff-only")
	require.NotContains(testInstance, string(invocationLogContents), "pull --rebase")

	localFileContents, localReadError := os.ReadFile(readmePath)
	require.NoError(testInstance, localReadError)
	require.Equal(testInstance, "initial\nmiddle\nmodified locally\n", string(localFileContents))
	require.Equal(testInstance, expectedGeneratedBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
	require.Equal(testInstance, "docs: sync dirty work", strings.TrimSpace(runGit(testInstance, repositoryPath, "log", "-1", "--pretty=%s")))
}

func TestSyncExplicitNewBranchCommitsDirtyNonBaseWorktreeInClusters(testInstance *testing.T) {
	testInstance.Helper()

	const (
		sourceBranchName = "feature/source-work"
		targetBranchName = "feature/clustered-work"
		pullRequestBody  = "Publish the clustered dirty work for review."
	)

	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	require.NoError(testInstance, os.MkdirAll(filepath.Join(repositoryPath, "pkg"), 0o755))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "pkg", "existing.go"), []byte("package pkg\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md", "pkg/existing.go")
	runGit(testInstance, repositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", sourceBranchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("source branch\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "source branch work")
	sourceCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))

	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("source branch\nuncommitted docs\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "pkg", "new.go"), []byte("package pkg\n\nconst Added = true\n"), 0o644))

	commitMessages := []string{
		"docs: save uncommitted guide changes",
		"feat: add clustered package work",
	}
	var responseIndex atomic.Int64
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		currentResponseIndex := int(responseIndex.Add(1) - 1)
		if currentResponseIndex >= len(commitMessages) {
			http.Error(responseWriter, "unexpected LLM request", http.StatusBadRequest)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(responseWriter, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, commitMessages[currentResponseIndex])
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

	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)
	runSync := func(branchName string, body string) (string, error) {
		return runIntegrationCommandWithInput(
			testInstance,
			repositoryRoot,
			integrationCommandOptions{
				PathVariable: pathVariable,
				EnvironmentOverrides: map[string]string{
					syncRefreshIntegrationAPIKeyVariable: "test-key",
					syncMergedBranchGitLogVariable:       gitLogPath,
					syncMergedBranchGitHubLogVariable:    githubLogPath,
					syncMergedBranchNameVariable:         branchName,
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
				branchName,
				"--body",
				body,
				"--roots",
				repositoryPath,
			},
		)
	}

	output, runError := runSync(targetBranchName, pullRequestBody)
	require.NoError(testInstance, runError, output)

	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (%s)", repositoryPath, targetBranchName))
	require.Equal(testInstance, targetBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, sourceCommit, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", sourceBranchName)))
	runGit(testInstance, repositoryPath, "merge-base", "--is-ancestor", sourceCommit, targetBranchName)
	require.Equal(testInstance, "2", strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-list", "--count", sourceCommit+".."+targetBranchName)))
	require.Equal(testInstance, commitMessages, strings.Split(strings.TrimSpace(runGit(testInstance, repositoryPath, "log", "--reverse", "--format=%s", sourceCommit+".."+targetBranchName)), "\n"))

	commitHashes := strings.Fields(runGit(testInstance, repositoryPath, "rev-list", "--reverse", sourceCommit+".."+targetBranchName))
	require.Len(testInstance, commitHashes, 2)
	require.Equal(testInstance, []string{commitHashes[0], sourceCommit}, strings.Fields(runGit(testInstance, repositoryPath, "rev-list", "--parents", "-n", "1", commitHashes[0])))
	require.Equal(testInstance, []string{commitHashes[1], commitHashes[0]}, strings.Fields(runGit(testInstance, repositoryPath, "rev-list", "--parents", "-n", "1", commitHashes[1])))
	require.Equal(testInstance, commitHashes[1], strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", targetBranchName)))
	require.Equal(testInstance, []string{"README.md"}, strings.Fields(runGit(testInstance, repositoryPath, "show", "--name-only", "--format=", commitHashes[0])))
	require.Equal(testInstance, []string{"pkg/new.go"}, strings.Fields(runGit(testInstance, repositoryPath, "show", "--name-only", "--format=", commitHashes[1])))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
	require.Equal(testInstance, "origin/"+targetBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")))
	gitLog := readTextFile(testInstance, gitLogPath)
	firstCommitIndex := strings.Index(gitLog, "commit -m "+commitMessages[0])
	secondCommitIndex := strings.Index(gitLog, "commit -m "+commitMessages[1])
	baseMergeIndex := strings.Index(gitLog, "merge --no-edit origin/master")
	pushIndex := strings.Index(gitLog, "push -u origin "+targetBranchName)
	require.NotEqual(testInstance, -1, firstCommitIndex)
	require.NotEqual(testInstance, -1, secondCommitIndex)
	require.NotEqual(testInstance, -1, baseMergeIndex)
	require.NotEqual(testInstance, -1, pushIndex)
	require.Less(testInstance, firstCommitIndex, secondCommitIndex)
	require.Less(testInstance, secondCommitIndex, baseMergeIndex)
	require.Less(testInstance, baseMergeIndex, pushIndex)

	cleanTargetBranchName := "feature/clean-child-work"
	cleanPullRequestBody := "Publish the clean child branch for review."
	cleanOutput, cleanRunError := runSync(cleanTargetBranchName, cleanPullRequestBody)
	require.NoError(testInstance, cleanRunError, cleanOutput)
	require.Contains(testInstance, cleanOutput, fmt.Sprintf("SYNCED: %s (%s)", repositoryPath, cleanTargetBranchName))
	require.Equal(testInstance, cleanTargetBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, commitHashes[1], strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", cleanTargetBranchName)))
	require.Equal(testInstance, "0", strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-list", "--count", targetBranchName+".."+cleanTargetBranchName)))
	require.Equal(testInstance, "origin/"+cleanTargetBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
	require.Equal(testInstance, int64(len(commitMessages)), responseIndex.Load())

	githubLog := readTextFile(testInstance, githubLogPath)
	require.Contains(testInstance, githubLog, "pr create --repo owner/project --base master --head "+targetBranchName+" --title "+targetBranchName+" --body "+pullRequestBody)
	require.Contains(testInstance, githubLog, "pr create --repo owner/project --base master --head "+cleanTargetBranchName+" --title "+cleanTargetBranchName+" --body "+cleanPullRequestBody)
}

func TestSyncFiltersTrackedIgnoredDirtyPathsBeforeStaging(testInstance *testing.T) {
	testInstance.Helper()

	expectedGeneratedBranchName := "gix/sync-tracked-dirty-work"
	repositoryRoot := integrationRepositoryRoot(testInstance)
	realGitPath, lookupError := exec.LookPath("git")
	require.NoError(testInstance, lookupError)

	gitInvocationLog := filepath.Join(testInstance.TempDir(), syncRefreshIntegrationGitInvocationLog)
	gitStubScript := []byte(strings.Join([]string{
		"#!/bin/sh",
		"echo \"$@\" >> " + gitInvocationLog,
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

	gitignorePath := filepath.Join(repositoryPath, ".gitignore")
	require.NoError(testInstance, os.WriteFile(gitignorePath, []byte("__pycache__/\n"), 0o644))
	eggInfoPath := filepath.Join(repositoryPath, "python", "llm_proxy_client.egg-info")
	eggInfoPackageFile := filepath.Join(eggInfoPath, "PKG-INFO")
	eggInfoSourcesFile := filepath.Join(eggInfoPath, "SOURCES.txt")
	require.NoError(testInstance, os.MkdirAll(eggInfoPath, 0o755))
	require.NoError(testInstance, os.WriteFile(eggInfoPackageFile, []byte("metadata before\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(eggInfoSourcesFile, []byte("sources before\n"), 0o644))
	clientCacheFile := filepath.Join(repositoryPath, "python", "llm_proxy_client", "__pycache__", "client.cpython-313.pyc")
	testCacheFile := filepath.Join(repositoryPath, "python", "tests", "__pycache__", "test_client.cpython-313-pytest-9.0.3.pyc")
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(clientCacheFile), 0o755))
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(testCacheFile), 0o755))
	require.NoError(testInstance, os.WriteFile(clientCacheFile, []byte("client cache before\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(testCacheFile, []byte("test cache before\n"), 0o644))

	addCommand := exec.Command("git", "-C", repositoryPath, "add", ".gitignore", "python/llm_proxy_client.egg-info/PKG-INFO", "python/llm_proxy_client.egg-info/SOURCES.txt")
	addCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, addCommand.Run())
	forceAddCommand := exec.Command("git", "-C", repositoryPath, "add", "-f", "python/llm_proxy_client/__pycache__/client.cpython-313.pyc", "python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc")
	forceAddCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, forceAddCommand.Run())

	commitCommand := exec.Command("git", "-C", repositoryPath, "commit", "-m", "initial commit")
	commitCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, commitCommand.Run())

	pushCommand := exec.Command("git", "-C", repositoryPath, "push", "-u", "origin", "master")
	pushCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, pushCommand.Run())

	require.NoError(testInstance, os.Remove(eggInfoPackageFile))
	require.NoError(testInstance, os.Remove(eggInfoSourcesFile))
	require.NoError(testInstance, os.WriteFile(clientCacheFile, []byte("client cache after\n"), 0o644))
	require.NoError(testInstance, os.Remove(testCacheFile))

	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"docs: sync tracked dirty work"}}]}`))
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
	require.Contains(testInstance, output, "strict sync requires a GitHub repository remote")
	require.NotContains(testInstance, output, "failed to stage dirty sync cluster")
	require.NotContains(testInstance, output, "The following paths are ignored")

	invocationLogContents, readError := os.ReadFile(gitInvocationLog)
	require.NoError(testInstance, readError)
	invocationLog := string(invocationLogContents)
	require.Contains(testInstance, invocationLog, "check-ignore --stdin")
	require.Contains(testInstance, invocationLog, "ls-files --cached --ignored --exclude-standard -- python/llm_proxy_client.egg-info/PKG-INFO python/llm_proxy_client.egg-info/SOURCES.txt python/llm_proxy_client/__pycache__/client.cpython-313.pyc python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc")
	require.Contains(testInstance, invocationLog, "restore --staged --worktree -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc")
	require.Contains(testInstance, invocationLog, "switch -c "+expectedGeneratedBranchName+"\n")
	require.NotContains(testInstance, invocationLog, "switch -c "+expectedGeneratedBranchName+" origin/master")
	require.Contains(testInstance, invocationLog, "add --all -- python/llm_proxy_client.egg-info/PKG-INFO python/llm_proxy_client.egg-info/SOURCES.txt")
	require.NotContains(testInstance, invocationLog, "add --all -- python/llm_proxy_client/__pycache__")
	require.NotContains(testInstance, invocationLog, "add --all -- python/tests/__pycache__")
	require.Contains(testInstance, invocationLog, "commit -m docs: sync tracked dirty work")

	require.Equal(testInstance, expectedGeneratedBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, "docs: sync tracked dirty work", strings.TrimSpace(runGit(testInstance, repositoryPath, "log", "-1", "--pretty=%s")))
	nameStatusOutput := strings.TrimSpace(runGit(testInstance, repositoryPath, "show", "--name-status", "--pretty=format:", "HEAD"))
	require.ElementsMatch(testInstance, []string{
		"D\tpython/llm_proxy_client.egg-info/PKG-INFO",
		"D\tpython/llm_proxy_client.egg-info/SOURCES.txt",
	}, strings.Split(nameStatusOutput, "\n"))

	statusOutput := runGit(testInstance, repositoryPath, "status", "--porcelain")
	require.Empty(testInstance, strings.TrimSpace(statusOutput))
	clientCacheContents, clientCacheReadError := os.ReadFile(clientCacheFile)
	require.NoError(testInstance, clientCacheReadError)
	require.Equal(testInstance, "client cache before\n", string(clientCacheContents))
	testCacheContents, testCacheReadError := os.ReadFile(testCacheFile)
	require.NoError(testInstance, testCacheReadError)
	require.Equal(testInstance, "test cache before\n", string(testCacheContents))
}
