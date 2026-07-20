package tests

import (
	"fmt"
	"io"
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

func TestSyncExplicitMasterCommitsDirtyMasterWorktreeAndMergesRemote(testInstance *testing.T) {
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
		"if [ \"$1\" = \"status\" ] && [ \"$2\" = \"--porcelain=v1\" ] && [ \"$3\" = \"-z\" ]; then",
		"  " + realGitPath + " \"$@\"",
		"  status_exit=$?",
		"  if [ \"$status_exit\" -eq 0 ]; then",
		"    printf '%s\\0' '!! python/llm_proxy_client/__pycache__/client.cpython-313.pyc' '!! python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc'",
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
      roots:
        - .
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

	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	commandArguments := []string{
		"--config",
		configurationPath,
		syncRefreshIntegrationLogLevelFlag,
		syncRefreshIntegrationErrorLogLevel,
		"sync",
		"master",
	}

	output, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		repositoryPath,
		map[string]string{
			pathEnvironmentVariableNameConstant:  pathVariable,
			syncRefreshIntegrationAPIKeyVariable: "test-key",
		},
		syncRefreshIntegrationTimeout,
		commandArguments,
	)
	require.NoError(testInstance, runError, output)
	testInstance.Logf("sync output:\n%s", output)
	require.Contains(testInstance, output, "SYNCED: . (master)")
	require.NotContains(testInstance, output, "worktree is dirty")
	require.NotContains(testInstance, output, "would be overwritten by checkout")

	invocationLogContents, readError := os.ReadFile(gitInvocationLog)
	require.NoError(testInstance, readError)
	invocationLog := string(invocationLogContents)
	require.Contains(testInstance, invocationLog, "check-ignore --stdin")
	require.NotContains(testInstance, invocationLog, "switch -c "+expectedGeneratedBranchName)
	require.NotContains(testInstance, invocationLog, "stash push --include-untracked")
	require.Contains(testInstance, invocationLog, "add --all -- README.md")
	require.Contains(testInstance, invocationLog, "add --all -- python/llm_proxy_client.egg-info/PKG-INFO python/llm_proxy_client.egg-info/SOURCES.txt")
	require.NotContains(testInstance, invocationLog, "add --all -- python/llm_proxy_client/__pycache__")
	require.NotContains(testInstance, invocationLog, "add --all -- python/tests/__pycache__")
	require.Contains(testInstance, invocationLog, "commit -m docs: sync dirty work")
	require.Contains(testInstance, invocationLog, "merge --no-edit origin/master")
	require.Contains(testInstance, invocationLog, "push origin master")
	require.NotContains(testInstance, invocationLog, "pull --rebase")

	localFileContents, localReadError := os.ReadFile(readmePath)
	require.NoError(testInstance, localReadError)
	require.Equal(testInstance, "remote update\nmiddle\nmodified locally\n", string(localFileContents))
	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "master")), strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "origin/master")))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
	require.Contains(testInstance, runGit(testInstance, repositoryPath, "log", "-5", "--format=%s"), "docs: sync dirty work")
}

func TestSyncExplicitMasterCommitsDirtyMasterWorktreeToMaster(testInstance *testing.T) {
	testInstance.Helper()

	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")
	baseCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))

	require.NoError(testInstance, os.MkdirAll(filepath.Join(repositoryPath, ".mprlab"), 0o755))
	require.NoError(testInstance, os.MkdirAll(filepath.Join(repositoryPath, "scripts"), 0o755))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, ".mprlab", "resources.yml"), []byte("tenant: kamu\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "scripts", "site.sh"), []byte("deploy_kamu_backend\n"), 0o755))

	responses := []string{
		"feat(deploy): add kamu tenant resources",
		"fix(site): update backend deployment",
	}
	var responseIndex atomic.Int64
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		currentResponseIndex := int(responseIndex.Add(1) - 1)
		if currentResponseIndex >= len(responses) {
			http.Error(responseWriter, "unexpected LLM request", http.StatusBadRequest)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(responseWriter, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, responses[currentResponseIndex])
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

	gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: buildSyncMergedBranchExecutablePath(testInstance),
			EnvironmentOverrides: map[string]string{
				syncRefreshIntegrationAPIKeyVariable: "test-key",
				syncMergedBranchGitLogVariable:       gitLogPath,
				syncMergedBranchGitHubLogVariable:    githubLogPath,
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
			"master",
			"--roots",
			repositoryPath,
		},
	)
	require.NoError(testInstance, runError, output)

	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (master)", repositoryPath))
	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, "2", strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-list", "--count", baseCommit+"..master")))
	require.Equal(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "master")), strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "origin/master")))
	require.Equal(testInstance, strings.Join([]string{responses[1], responses[0]}, "\n"), strings.TrimSpace(runGit(testInstance, repositoryPath, "log", "--format=%s", baseCommit+"..master")))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
	require.Equal(testInstance, int64(len(responses)), responseIndex.Load())

	gitLog := readTextFile(testInstance, gitLogPath)
	require.NotContains(testInstance, gitLog, "switch -c gix/")
	require.Contains(testInstance, gitLog, "push origin master")
	require.NotContains(testInstance, readTextFile(testInstance, githubLogPath), "pr create ")
}

func TestSyncExplicitMasterRejectsMissingRemoteBaseBeforeCommitting(testInstance *testing.T) {
	testInstance.Helper()

	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	readmePath := filepath.Join(repositoryPath, "README.md")
	require.NoError(testInstance, os.WriteFile(readmePath, []byte("initial\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "initial commit")
	baseCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	require.NoError(testInstance, os.WriteFile(readmePath, []byte("dirty local change\n"), 0o644))

	var requestCount atomic.Int64
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		requestCount.Add(1)
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"docs: commit dirty work"}}]}`))
	}))
	testInstance.Cleanup(llmServer.Close)

	configurationPath := writeDirtySyncMergedBranchConfiguration(testInstance, llmServer.URL)
	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: buildSyncMergedBranchExecutablePath(testInstance),
			EnvironmentOverrides: map[string]string{
				syncMergedBranchAPIKeyVariable: "test-key",
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
			"master",
			"--roots",
			repositoryPath,
		},
	)
	require.Error(testInstance, runError)
	require.Contains(testInstance, output, `remote base branch "origin/master" does not exist`)
	require.Zero(testInstance, requestCount.Load())
	require.Equal(testInstance, baseCommit, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD")))
	require.Equal(testInstance, "M README.md", strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
}

func TestSyncExplicitNewBranchCreatesStackedPullRequestsAndCommitsDirtyWorkInClusters(testInstance *testing.T) {
	testInstance.Helper()

	const (
		grandparentBranchName = "feature/grandparent-work"
		sourceBranchName      = "feature/source-work"
		targetBranchName      = "feature/clustered-work"
		rescueBranchName      = "feature/post-merge-work"
		sourcePullRequestBody = "Publish the source branch for review before stacking the clustered work."
		pullRequestBody       = "Publish the clustered dirty work for review."
		rescuePullRequestBody = "Publish the work preserved through the merged-branch handoff."
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

	runGit(testInstance, repositoryPath, "switch", "-c", grandparentBranchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "grandparent.txt"), []byte("grandparent branch work\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "grandparent.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "grandparent branch work")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", grandparentBranchName)
	grandparentCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))

	runGit(testInstance, repositoryPath, "switch", "-c", sourceBranchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("source branch\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "source branch work")
	sourceCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	runGit(testInstance, repositoryPath, "config", "branch."+sourceBranchName+".gix-review-base", grandparentBranchName)
	runGit(testInstance, repositoryPath, "branch", "-D", grandparentBranchName)
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--list", grandparentBranchName)))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--remotes", "--list", "origin/"+sourceBranchName)))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "for-each-ref", "--format=%(upstream:short)", "refs/heads/"+sourceBranchName)))

	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("source branch\nuncommitted docs\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "pkg", "new.go"), []byte("package pkg\n\nconst Added = true\n"), 0o644))

	commitMessages := []string{
		"docs: save uncommitted guide changes",
		"feat: add clustered package work",
	}
	allCommitMessages := append(append([]string{}, commitMessages...), "docs: preserve post-merge work")
	var responseIndex atomic.Int64
	var sourceDescriptionRequests atomic.Int64
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		requestBody, readError := io.ReadAll(request.Body)
		if readError != nil {
			http.Error(responseWriter, readError.Error(), http.StatusBadRequest)
			return
		}
		if strings.Contains(string(requestBody), "expert maintainer writing pull request descriptions") {
			sourceDescriptionRequests.Add(1)
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(responseWriter, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, sourcePullRequestBody)
			return
		}
		currentResponseIndex := int(responseIndex.Add(1) - 1)
		if currentResponseIndex >= len(allCommitMessages) {
			http.Error(responseWriter, "unexpected LLM request", http.StatusBadRequest)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(responseWriter, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, allCommitMessages[currentResponseIndex])
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
	operationLogPath := filepath.Join(testInstance.TempDir(), "operations.log")
	require.NoError(testInstance, os.WriteFile(githubLogPath, []byte("created-pr --base master --head "+grandparentBranchName+"\n"), 0o600))
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)
	runSync := func(branchName string, body string, failPullRequestHead string) (string, error) {
		return runIntegrationCommandWithInput(
			testInstance,
			repositoryRoot,
			integrationCommandOptions{
				PathVariable: pathVariable,
				EnvironmentOverrides: map[string]string{
					syncRefreshIntegrationAPIKeyVariable:        "test-key",
					syncMergedBranchGitLogVariable:              gitLogPath,
					syncMergedBranchGitHubLogVariable:           githubLogPath,
					syncMergedBranchOperationLogVariable:        operationLogPath,
					syncMergedBranchFailPullRequestHeadVariable: failPullRequestHead,
					syncMergedBranchNameVariable:                branchName,
					syncMergedBranchMergedVariable:              "false",
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

	failedOutput, failedRunError := runSync(targetBranchName, pullRequestBody, targetBranchName)
	require.Error(testInstance, failedRunError)
	require.Contains(testInstance, failedOutput, "simulated pull request creation failure for "+targetBranchName)
	require.Equal(testInstance, sourceBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "config", "--get", "branch."+targetBranchName+".gix-review-base")))

	output, runError := runSync(targetBranchName, pullRequestBody, "")
	require.NoError(testInstance, runError, output)

	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (%s)", repositoryPath, targetBranchName))
	require.Equal(testInstance, targetBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--list", grandparentBranchName)))
	require.Equal(testInstance, grandparentCommit, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "origin/"+grandparentBranchName)))
	require.Equal(testInstance, sourceCommit, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", sourceBranchName)))
	require.Equal(testInstance, sourceCommit, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "origin/"+sourceBranchName)))
	require.Equal(testInstance, "origin/"+sourceBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "--abbrev-ref", sourceBranchName+"@{upstream}")))
	runGit(testInstance, repositoryPath, "merge-base", "--is-ancestor", grandparentCommit, sourceBranchName)
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
	baseMergeIndex := strings.Index(gitLog, "merge --no-edit origin/"+sourceBranchName)
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
	cleanOutput, cleanRunError := runSync(cleanTargetBranchName, cleanPullRequestBody, "")
	require.Error(testInstance, cleanRunError)
	require.Contains(testInstance, cleanOutput, "cannot create stacked branch \""+cleanTargetBranchName+"\" from \""+targetBranchName+"\": no changes would remain for its pull request")
	require.Equal(testInstance, targetBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--list", cleanTargetBranchName)))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
	require.Equal(testInstance, int64(len(commitMessages)), responseIndex.Load())
	require.Equal(testInstance, int64(1), sourceDescriptionRequests.Load())

	githubLog := readTextFile(testInstance, githubLogPath)
	sourcePullRequestCommand := "pr create --repo owner/project --base " + grandparentBranchName + " --head " + sourceBranchName + " --title " + sourceBranchName + " --body " + sourcePullRequestBody
	targetPullRequestCommand := "pr create --repo owner/project --base " + sourceBranchName + " --head " + targetBranchName + " --title " + targetBranchName + " --body " + pullRequestBody
	require.Contains(testInstance, githubLog, sourcePullRequestCommand)
	require.Contains(testInstance, githubLog, targetPullRequestCommand)
	require.NotContains(testInstance, githubLog, "pr create --repo owner/project --base "+targetBranchName+" --head "+cleanTargetBranchName)
	require.NotContains(testInstance, githubLog, "pr create --repo owner/project --base master --head "+sourceBranchName+" ")
	require.NotContains(testInstance, githubLog, "pr create --repo owner/project --base master --head "+targetBranchName+" ")
	require.Equal(testInstance, 1, strings.Count(githubLog, "pr create --repo owner/project --base "+grandparentBranchName+" --head "+sourceBranchName+" "))
	require.Equal(testInstance, 2, strings.Count(githubLog, "pr create --repo owner/project --base "+sourceBranchName+" --head "+targetBranchName+" "))
	require.Equal(testInstance, 1, strings.Count(githubLog, "created-pr --base master --head "+grandparentBranchName))
	require.Equal(testInstance, 1, strings.Count(githubLog, "created-pr --base "+grandparentBranchName+" --head "+sourceBranchName))
	require.Equal(testInstance, 1, strings.Count(githubLog, "created-pr --base "+sourceBranchName+" --head "+targetBranchName))
	require.Equal(testInstance, 3, strings.Count(githubLog, "created-pr "))

	operationLog := readTextFile(testInstance, operationLogPath)
	parentPushIndex := strings.Index(operationLog, "git push -u origin "+sourceBranchName)
	parentPullRequestIndex := strings.Index(operationLog, "gh-created --base "+grandparentBranchName+" --head "+sourceBranchName)
	targetCreationIndex := strings.Index(operationLog, "git switch -c "+targetBranchName)
	targetPushIndex := strings.Index(operationLog, "git push -u origin "+targetBranchName)
	targetPullRequestIndex := strings.Index(operationLog, "gh-created --base "+sourceBranchName+" --head "+targetBranchName)
	require.NotEqual(testInstance, -1, parentPushIndex)
	require.NotEqual(testInstance, -1, parentPullRequestIndex)
	require.NotEqual(testInstance, -1, targetCreationIndex)
	require.NotEqual(testInstance, -1, targetPushIndex)
	require.NotEqual(testInstance, -1, targetPullRequestIndex)
	require.Less(testInstance, parentPushIndex, parentPullRequestIndex)
	require.Less(testInstance, parentPullRequestIndex, targetCreationIndex)
	require.Less(testInstance, targetCreationIndex, targetPushIndex)
	require.Less(testInstance, targetPushIndex, targetPullRequestIndex)
	require.NotContains(testInstance, operationLog, "git push -u origin "+grandparentBranchName)
	require.NotContains(testInstance, operationLog, "git switch -c "+cleanTargetBranchName)

	runGit(testInstance, repositoryPath, "push", "origin", "--delete", targetBranchName)
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--remotes", "--list", "origin/"+targetBranchName)))
	mergedPullRequests := readTextFile(testInstance, githubLogPath) +
		"merged-pr --base " + grandparentBranchName + " --head " + sourceBranchName + "\n" +
		"merged-pr --base " + sourceBranchName + " --head " + targetBranchName + "\n"
	require.NoError(testInstance, os.WriteFile(githubLogPath, []byte(mergedPullRequests), 0o600))
	runMergedSync := func(branchName string, additionalArguments ...string) (string, error) {
		commandArguments := []string{
			syncRefreshIntegrationRunCommand,
			syncRefreshIntegrationModulePath,
			"--config",
			configurationPath,
			syncRefreshIntegrationLogLevelFlag,
			syncRefreshIntegrationErrorLogLevel,
			"sync",
			branchName,
		}
		commandArguments = append(commandArguments, additionalArguments...)
		commandArguments = append(commandArguments, "--roots", repositoryPath, "--yes")
		return runIntegrationCommandWithInput(
			testInstance,
			repositoryRoot,
			integrationCommandOptions{
				PathVariable: pathVariable,
				EnvironmentOverrides: map[string]string{
					syncRefreshIntegrationAPIKeyVariable: "test-key",
					syncMergedBranchGitLogVariable:       gitLogPath,
					syncMergedBranchGitHubLogVariable:    githubLogPath,
					syncMergedBranchOperationLogVariable: operationLogPath,
					syncMergedBranchNameVariable:         branchName,
					syncMergedBranchMergedVariable:       "false",
				},
			},
			syncMergedBranchIntegrationTimeout,
			"",
			commandArguments,
		)
	}
	githubLogSuffix := func(previousLog string) string {
		currentLog := readTextFile(testInstance, githubLogPath)
		require.True(testInstance, strings.HasPrefix(currentLog, previousLog))
		return currentLog[len(previousLog):]
	}
	preRecoveryPullRequestCreateCount := strings.Count(mergedPullRequests, "pr create ")
	preRecoveryCreatedPullRequestCount := strings.Count(mergedPullRequests, "created-pr ")
	targetCommitBeforeMergedHandoff := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", targetBranchName))
	dirtyHeadBeforeMergedHandoff := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	dirtyBranchBeforeMergedHandoff := strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current"))
	strandedWorkContents := "must remain uncommitted\n"
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "stranded.txt"), []byte(strandedWorkContents), 0o644))
	dirtyMergedLog := readTextFile(testInstance, githubLogPath)
	dirtyMergedOutput, dirtyMergedRunError := runMergedSync(targetBranchName)
	require.Error(testInstance, dirtyMergedRunError)
	require.Contains(testInstance, dirtyMergedOutput, "cannot commit uncommitted changes on merged branch \""+targetBranchName+"\"")
	require.Equal(testInstance, targetCommitBeforeMergedHandoff, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", targetBranchName)))
	require.Equal(testInstance, dirtyHeadBeforeMergedHandoff, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD")))
	require.Equal(testInstance, dirtyBranchBeforeMergedHandoff, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, strandedWorkContents, readTextFile(testInstance, filepath.Join(repositoryPath, "stranded.txt")))
	require.Equal(testInstance, "?? stranded.txt", strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
	dirtyMergedSuffix := githubLogSuffix(dirtyMergedLog)
	require.Contains(testInstance, dirtyMergedSuffix, "pr list --repo owner/project --state merged --head "+targetBranchName)
	require.NotContains(testInstance, dirtyMergedSuffix, "pr create ")

	runGit(testInstance, repositoryPath, "push", "origin", "--delete", sourceBranchName)
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--remotes", "--list", "origin/"+sourceBranchName)))
	stashedHandoffLog := readTextFile(testInstance, githubLogPath)
	stashedHandoffOutput, stashedHandoffError := runMergedSync(targetBranchName, "--stash")
	require.NoError(testInstance, stashedHandoffError, stashedHandoffOutput)
	require.Contains(testInstance, stashedHandoffOutput, fmt.Sprintf("SYNCED: %s (%s)", repositoryPath, grandparentBranchName))
	require.Equal(testInstance, grandparentBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, grandparentCommit, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD")))
	require.Equal(testInstance, "origin/"+grandparentBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")))
	require.Equal(testInstance, strandedWorkContents, readTextFile(testInstance, filepath.Join(repositoryPath, "stranded.txt")))
	require.Equal(testInstance, "?? stranded.txt", strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
	stashedHandoffSuffix := githubLogSuffix(stashedHandoffLog)
	require.Contains(testInstance, stashedHandoffSuffix, "pr list --repo owner/project --state merged --base "+sourceBranchName+" --head "+targetBranchName)
	require.Contains(testInstance, stashedHandoffSuffix, "pr list --repo owner/project --state merged --base "+grandparentBranchName+" --head "+sourceBranchName)
	require.NotContains(testInstance, stashedHandoffSuffix, "pr create ")
	require.Equal(testInstance, preRecoveryPullRequestCreateCount, strings.Count(readTextFile(testInstance, githubLogPath), "pr create "))
	require.Equal(testInstance, preRecoveryCreatedPullRequestCount, strings.Count(readTextFile(testInstance, githubLogPath), "created-pr "))

	rescueOutput, rescueRunError := runSync(rescueBranchName, rescuePullRequestBody, "")
	require.NoError(testInstance, rescueRunError, rescueOutput)
	require.Contains(testInstance, rescueOutput, fmt.Sprintf("SYNCED: %s (%s)", repositoryPath, rescueBranchName))
	require.Equal(testInstance, rescueBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, strandedWorkContents, runGit(testInstance, repositoryPath, "show", rescueBranchName+":stranded.txt"))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
	require.Equal(testInstance, grandparentBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "config", "--get", "branch."+rescueBranchName+".gix-review-base")))
	require.Equal(testInstance, int64(len(allCommitMessages)), responseIndex.Load())
	rescueGitHubLog := readTextFile(testInstance, githubLogPath)
	require.Contains(testInstance, rescueGitHubLog, "created-pr --base "+grandparentBranchName+" --head "+rescueBranchName)
	require.NotContains(testInstance, rescueGitHubLog, "pr create --repo owner/project --base master --head "+rescueBranchName+" ")
	pullRequestCreateCount := strings.Count(rescueGitHubLog, "pr create ")
	createdPullRequestCount := strings.Count(rescueGitHubLog, "created-pr ")

	mergedLog := readTextFile(testInstance, githubLogPath)
	mergedOutput, mergedRunError := runMergedSync(targetBranchName)
	require.NoError(testInstance, mergedRunError, mergedOutput)
	require.Contains(testInstance, mergedOutput, fmt.Sprintf("SYNCED: %s (%s)", repositoryPath, grandparentBranchName))
	require.NotContains(testInstance, mergedOutput, "parent branch pull request is already merged")
	require.Equal(testInstance, grandparentBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, grandparentCommit, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD")))
	require.Equal(testInstance, "origin/"+grandparentBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")))
	require.Equal(testInstance, sourceBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "config", "--get", "branch."+targetBranchName+".gix-review-base")))
	mergedSuffix := githubLogSuffix(mergedLog)
	require.Contains(testInstance, mergedSuffix, "pr list --repo owner/project --state merged --head "+targetBranchName)
	require.Contains(testInstance, mergedSuffix, "pr list --repo owner/project --state merged --base "+sourceBranchName+" --head "+targetBranchName)
	require.Contains(testInstance, mergedSuffix, "pr list --repo owner/project --state merged --head "+sourceBranchName)
	require.Contains(testInstance, mergedSuffix, "pr list --repo owner/project --state merged --base "+grandparentBranchName+" --head "+sourceBranchName)
	require.NotContains(testInstance, mergedSuffix, "pr create ")
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--remotes", "--list", "origin/"+targetBranchName)))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--remotes", "--list", "origin/"+sourceBranchName)))

	parentMergedLog := readTextFile(testInstance, githubLogPath)
	parentMergedOutput, parentMergedRunError := runMergedSync(sourceBranchName)
	require.NoError(testInstance, parentMergedRunError, parentMergedOutput)
	require.Contains(testInstance, parentMergedOutput, fmt.Sprintf("SYNCED: %s (%s)", repositoryPath, grandparentBranchName))
	require.NotContains(testInstance, parentMergedOutput, "parent branch pull request is already merged")
	require.Equal(testInstance, grandparentBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, grandparentCommit, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD")))
	require.Equal(testInstance, "origin/"+grandparentBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")))
	parentMergedSuffix := githubLogSuffix(parentMergedLog)
	require.Contains(testInstance, parentMergedSuffix, "pr list --repo owner/project --state merged --head "+sourceBranchName)
	require.Contains(testInstance, parentMergedSuffix, "pr list --repo owner/project --state merged --base "+grandparentBranchName+" --head "+sourceBranchName)
	require.NotContains(testInstance, parentMergedSuffix, "pr create ")
	finalGitHubLog := readTextFile(testInstance, githubLogPath)
	require.Equal(testInstance, pullRequestCreateCount, strings.Count(finalGitHubLog, "pr create "))
	require.Equal(testInstance, createdPullRequestCount, strings.Count(finalGitHubLog, "created-pr "))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--remotes", "--list", "origin/"+targetBranchName)))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--remotes", "--list", "origin/"+sourceBranchName)))
}

func TestSyncExplicitMasterFromDirtyFeatureBranchCommitsToMaster(testInstance *testing.T) {
	testInstance.Helper()

	const (
		sourceBranchName = "feature/source-work"
	)

	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")
	baseCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))

	runGit(testInstance, repositoryPath, "switch", "-c", sourceBranchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "feature.txt"), []byte("committed feature work\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "feature.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "feature source commit")
	sourceCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\nrescued dirty work\n"), 0o644))

	responses := []string{"docs: commit dirty work to explicit master"}
	var responseIndex atomic.Int64
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		currentResponseIndex := int(responseIndex.Add(1) - 1)
		if currentResponseIndex >= len(responses) {
			http.Error(responseWriter, "unexpected LLM request", http.StatusBadRequest)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(responseWriter, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, responses[currentResponseIndex])
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

	gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)
	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncRefreshIntegrationAPIKeyVariable: "test-key",
				syncMergedBranchGitLogVariable:       gitLogPath,
				syncMergedBranchGitHubLogVariable:    githubLogPath,
				syncMergedBranchNameVariable:         "unused-generated-branch",
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
			"master",
			"--roots",
			repositoryPath,
		},
	)
	require.NoError(testInstance, runError, output)

	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (master)", repositoryPath))
	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, sourceCommit, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", sourceBranchName)))
	require.Equal(testInstance, baseCommit, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "master^")))
	require.Equal(testInstance, "docs: commit dirty work to explicit master", strings.TrimSpace(runGit(testInstance, repositoryPath, "log", "-1", "--pretty=%s")))
	require.Equal(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "master")), strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "origin/master")))
	require.Equal(testInstance, "initial\nrescued dirty work\n", readTextFile(testInstance, filepath.Join(repositoryPath, "README.md")))
	_, featureFileError := os.Stat(filepath.Join(repositoryPath, "feature.txt"))
	require.True(testInstance, os.IsNotExist(featureFileError))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
	require.Equal(testInstance, int64(len(responses)), responseIndex.Load())

	gitLog := readTextFile(testInstance, gitLogPath)
	require.Contains(testInstance, gitLog, "stash push --include-untracked")
	require.Contains(testInstance, gitLog, "switch master")
	require.Contains(testInstance, gitLog, "stash pop")
	require.NotContains(testInstance, gitLog, "switch -c gix/")
	require.Contains(testInstance, gitLog, "commit -m docs: commit dirty work to explicit master")
	require.Contains(testInstance, gitLog, "push origin master")

	githubLog := readTextFile(testInstance, githubLogPath)
	require.NotContains(testInstance, githubLog, "pr create ")
}

func TestSyncRejectsTruncatedLongFileMergeResolutionBeforeCommitOrPush(testInstance *testing.T) {
	testInstance.Helper()

	const (
		conflictedFileName = "ISSUES.md"
		stableTailLine     = "- [ ] [B1100] Preserve final unrelated issue."
	)

	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	var baseContentBuilder strings.Builder
	baseContentBuilder.WriteString("# ISSUES\n\n- [ ] [B000] base conflict entry\n")
	for issueIndex := 1; issueIndex < 1100; issueIndex++ {
		_, _ = fmt.Fprintf(&baseContentBuilder, "- [ ] [B%04d] Stable unrelated issue %04d.\n", issueIndex, issueIndex)
	}
	baseContentBuilder.WriteString(stableTailLine + "\n")
	baseContent := baseContentBuilder.String()
	conflictedFilePath := filepath.Join(repositoryPath, conflictedFileName)
	require.NoError(testInstance, os.WriteFile(conflictedFilePath, []byte(baseContent), 0o644))
	runGit(testInstance, repositoryPath, "add", conflictedFileName)
	runGit(testInstance, repositoryPath, "commit", "-m", "seed long issue tracker")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")
	baseCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))

	upstreamPath := filepath.Join(workspacePath, "upstream")
	runGitWithDir(testInstance, "", "clone", remotePath, upstreamPath)
	configureGitIdentity(testInstance, upstreamPath)
	remoteContent := strings.Replace(baseContent, "- [ ] [B000] base conflict entry", "- [x] [B000] remote conflict entry", 1)
	require.NoError(testInstance, os.WriteFile(filepath.Join(upstreamPath, conflictedFileName), []byte(remoteContent), 0o644))
	runGit(testInstance, upstreamPath, "add", conflictedFileName)
	runGit(testInstance, upstreamPath, "commit", "-m", "resolve issue remotely")
	runGit(testInstance, upstreamPath, "push", "origin", "master")

	localContent := strings.Replace(baseContent, "- [ ] [B000] base conflict entry", "- [-] [B000] local conflict entry", 1)
	require.NoError(testInstance, os.WriteFile(conflictedFilePath, []byte(localContent), 0o644))
	truncatedResolution := "# ISSUES\n\n- [x] [B000] combined conflict entry\n"
	responses := []string{
		"docs: update issue tracker entry",
		truncatedResolution,
	}
	var responseIndex atomic.Int64
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		currentResponseIndex := int(responseIndex.Add(1) - 1)
		if currentResponseIndex >= len(responses) {
			http.Error(responseWriter, "unexpected LLM request", http.StatusBadRequest)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(responseWriter, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, responses[currentResponseIndex])
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

	gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)
	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncRefreshIntegrationAPIKeyVariable: "test-key",
				syncMergedBranchGitLogVariable:       gitLogPath,
				syncMergedBranchGitHubLogVariable:    githubLogPath,
				syncMergedBranchNameVariable:         "unused-generated-branch",
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
			"master",
			"--roots",
			repositoryPath,
		},
	)
	require.Error(testInstance, runError)
	require.Contains(testInstance, output, "MERGE_CONFLICT")
	require.Contains(testInstance, output, "AI_MERGE_RESOLUTION")
	require.Contains(testInstance, output, "AI_MERGE_VALIDATION")
	require.Contains(testInstance, output, "AI_MERGE_HANDOFF")
	require.Contains(testInstance, output, "does not preserve non-conflicting content")
	require.Contains(testInstance, output, "git merge --abort")
	require.Equal(testInstance, int64(len(responses)), responseIndex.Load())

	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, conflictedFileName, strings.TrimSpace(runGit(testInstance, repositoryPath, "diff", "--name-only", "--diff-filter=U")))
	headWithParents := strings.Fields(runGit(testInstance, repositoryPath, "rev-list", "--parents", "-n", "1", "HEAD"))
	require.Len(testInstance, headWithParents, 2)
	require.Equal(testInstance, baseCommit, headWithParents[1])

	conflictState := readTextFile(testInstance, conflictedFilePath)
	require.Contains(testInstance, conflictState, "<<<<<<< HEAD")
	require.Contains(testInstance, conflictState, stableTailLine)
	require.NotEqual(testInstance, truncatedResolution, conflictState)

	gitLog := readTextFile(testInstance, gitLogPath)
	require.Contains(testInstance, gitLog, "merge --no-edit origin/master")
	require.NotContains(testInstance, gitLog, "commit --no-edit")
	require.NotContains(testInstance, gitLog, "push origin master")
	if githubLogBytes, githubLogReadError := os.ReadFile(githubLogPath); githubLogReadError == nil {
		require.NotContains(testInstance, string(githubLogBytes), "pr create")
	} else {
		require.True(testInstance, os.IsNotExist(githubLogReadError))
	}
}

func TestSyncTimesOutAIMergeResolutionWithVisibleRecoveryHandoff(testInstance *testing.T) {
	testInstance.Helper()

	const (
		conflictedFileName = "ISSUES.md"
		stableTailLine     = "stable unrelated tail content"
	)

	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	baseContent := "# ISSUES\n\nbase issue\n" + stableTailLine + "\n"
	conflictedFilePath := filepath.Join(repositoryPath, conflictedFileName)
	require.NoError(testInstance, os.WriteFile(conflictedFilePath, []byte(baseContent), 0o644))
	runGit(testInstance, repositoryPath, "add", conflictedFileName)
	runGit(testInstance, repositoryPath, "commit", "-m", "seed merge timeout fixture")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")
	baseCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))

	upstreamPath := filepath.Join(workspacePath, "upstream")
	runGitWithDir(testInstance, "", "clone", remotePath, upstreamPath)
	configureGitIdentity(testInstance, upstreamPath)
	remoteContent := strings.Replace(baseContent, "base issue", "remote issue", 1)
	require.NoError(testInstance, os.WriteFile(filepath.Join(upstreamPath, conflictedFileName), []byte(remoteContent), 0o644))
	runGit(testInstance, upstreamPath, "add", conflictedFileName)
	runGit(testInstance, upstreamPath, "commit", "-m", "change issue remotely")
	runGit(testInstance, upstreamPath, "push", "origin", "master")

	localContent := strings.Replace(baseContent, "base issue", "local issue", 1)
	require.NoError(testInstance, os.WriteFile(conflictedFilePath, []byte(localContent), 0o644))

	var responseIndex atomic.Int64
	resolutionStarted := make(chan time.Time, 1)
	releaseResolution := make(chan struct{})
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		if responseIndex.Add(1) == 1 {
			responseWriter.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(responseWriter, `{"choices":[{"message":{"role":"assistant","content":"docs: update local issue"}}]}`)
			return
		}
		resolutionStarted <- time.Now()
		select {
		case <-request.Context().Done():
		case <-releaseResolution:
		}
	}))
	testInstance.Cleanup(llmServer.Close)
	testInstance.Cleanup(func() {
		close(releaseResolution)
	})

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
      timeout_seconds: 2
`, syncRefreshIntegrationAPIKeyVariable, llmServer.URL)
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))

	gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)
	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncRefreshIntegrationAPIKeyVariable: "test-key",
				syncMergedBranchGitLogVariable:       gitLogPath,
				syncMergedBranchGitHubLogVariable:    githubLogPath,
				syncMergedBranchNameVariable:         "unused-generated-branch",
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
			"master",
			"--roots",
			repositoryPath,
		},
	)
	require.Error(testInstance, runError)
	require.Contains(testInstance, output, "MERGE_CONFLICT")
	require.Contains(testInstance, output, "AI_MERGE_RESOLUTION")
	require.Contains(testInstance, output, "deadline 2s")
	require.Contains(testInstance, output, "still resolving "+conflictedFileName+" with AI")
	require.Contains(testInstance, output, "AI_MERGE_HANDOFF")
	require.Contains(testInstance, output, "AI merge resolution timed out after 2s")
	require.Contains(testInstance, output, "git merge --abort")
	require.Equal(testInstance, int64(2), responseIndex.Load())

	select {
	case startedAt := <-resolutionStarted:
		require.Less(testInstance, time.Since(startedAt), 5*time.Second, output)
	default:
		testInstance.Fatal("expected the merge-resolution request to start")
	}

	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, conflictedFileName, strings.TrimSpace(runGit(testInstance, repositoryPath, "diff", "--name-only", "--diff-filter=U")))
	headWithParents := strings.Fields(runGit(testInstance, repositoryPath, "rev-list", "--parents", "-n", "1", "HEAD"))
	require.Len(testInstance, headWithParents, 2)
	require.Equal(testInstance, baseCommit, headWithParents[1])

	conflictState := readTextFile(testInstance, conflictedFilePath)
	require.Contains(testInstance, conflictState, "<<<<<<< HEAD")
	require.Contains(testInstance, conflictState, stableTailLine)

	gitLog := readTextFile(testInstance, gitLogPath)
	require.Contains(testInstance, gitLog, "merge --no-edit origin/master")
	require.NotContains(testInstance, gitLog, "commit --no-edit")
	require.NotContains(testInstance, gitLog, "push origin master")
	if githubLogBytes, githubLogReadError := os.ReadFile(githubLogPath); githubLogReadError == nil {
		require.NotContains(testInstance, string(githubLogBytes), "pr create")
	} else {
		require.True(testInstance, os.IsNotExist(githubLogReadError))
	}
}

func TestSyncRejectsTruncatedMarkerFreeModifyDeleteResolutionBeforeCommitOrPush(testInstance *testing.T) {
	testInstance.Helper()

	const (
		conflictedFileName = "NOTES.md"
		stableTailLine     = "stable unrelated tail content"
	)

	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	baseContent := "base heading\n" + stableTailLine + "\n"
	conflictedFilePath := filepath.Join(repositoryPath, conflictedFileName)
	require.NoError(testInstance, os.WriteFile(conflictedFilePath, []byte(baseContent), 0o644))
	runGit(testInstance, repositoryPath, "add", conflictedFileName)
	runGit(testInstance, repositoryPath, "commit", "-m", "seed marker-free conflict fixture")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")
	baseCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))

	upstreamPath := filepath.Join(workspacePath, "upstream")
	runGitWithDir(testInstance, "", "clone", remotePath, upstreamPath)
	configureGitIdentity(testInstance, upstreamPath)
	remoteContent := "remote heading\n" + stableTailLine + "\n"
	require.NoError(testInstance, os.WriteFile(filepath.Join(upstreamPath, conflictedFileName), []byte(remoteContent), 0o644))
	runGit(testInstance, upstreamPath, "add", conflictedFileName)
	runGit(testInstance, upstreamPath, "commit", "-m", "modify notes remotely")
	runGit(testInstance, upstreamPath, "push", "origin", "master")

	require.NoError(testInstance, os.Remove(conflictedFilePath))
	truncatedResolution := "remote heading\n"
	responses := []string{
		"docs: delete obsolete notes",
		truncatedResolution,
	}
	var responseIndex atomic.Int64
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		currentResponseIndex := int(responseIndex.Add(1) - 1)
		if currentResponseIndex >= len(responses) {
			http.Error(responseWriter, "unexpected LLM request", http.StatusBadRequest)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(responseWriter, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, responses[currentResponseIndex])
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

	gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)
	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncRefreshIntegrationAPIKeyVariable: "test-key",
				syncMergedBranchGitLogVariable:       gitLogPath,
				syncMergedBranchGitHubLogVariable:    githubLogPath,
				syncMergedBranchNameVariable:         "unused-generated-branch",
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
			"master",
			"--roots",
			repositoryPath,
		},
	)
	require.Error(testInstance, runError)
	require.Contains(testInstance, output, "does not preserve non-conflicting content")
	require.Equal(testInstance, int64(len(responses)), responseIndex.Load())

	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, conflictedFileName, strings.TrimSpace(runGit(testInstance, repositoryPath, "diff", "--name-only", "--diff-filter=U")))
	headWithParents := strings.Fields(runGit(testInstance, repositoryPath, "rev-list", "--parents", "-n", "1", "HEAD"))
	require.Len(testInstance, headWithParents, 2)
	require.Equal(testInstance, baseCommit, headWithParents[1])

	conflictState := readTextFile(testInstance, conflictedFilePath)
	require.Equal(testInstance, remoteContent, conflictState)
	require.NotContains(testInstance, conflictState, "<<<<<<<")
	require.Contains(testInstance, conflictState, stableTailLine)
	require.NotEqual(testInstance, truncatedResolution, conflictState)

	gitLog := readTextFile(testInstance, gitLogPath)
	require.Contains(testInstance, gitLog, "merge --no-edit origin/master")
	require.NotContains(testInstance, gitLog, "commit --no-edit")
	require.NotContains(testInstance, gitLog, "push origin master")
	if githubLogBytes, githubLogReadError := os.ReadFile(githubLogPath); githubLogReadError == nil {
		require.NotContains(testInstance, string(githubLogBytes), "pr create")
	} else {
		require.True(testInstance, os.IsNotExist(githubLogReadError))
	}
}

func TestSyncFiltersTrackedIgnoredDirtyPathsBeforeStaging(testInstance *testing.T) {
	testInstance.Helper()

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
	output := runIntegrationCommand(
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
	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (master)", repositoryPath))
	require.NotContains(testInstance, output, "failed to stage dirty sync cluster")
	require.NotContains(testInstance, output, "The following paths are ignored")

	invocationLogContents, readError := os.ReadFile(gitInvocationLog)
	require.NoError(testInstance, readError)
	invocationLog := string(invocationLogContents)
	require.Contains(testInstance, invocationLog, "check-ignore --stdin")
	require.Contains(testInstance, invocationLog, "ls-files --cached --ignored --exclude-standard -- python/llm_proxy_client.egg-info/PKG-INFO python/llm_proxy_client.egg-info/SOURCES.txt python/llm_proxy_client/__pycache__/client.cpython-313.pyc python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc")
	require.Contains(testInstance, invocationLog, "restore --staged --worktree -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc")
	require.NotContains(testInstance, invocationLog, "switch -c gix/")
	require.Contains(testInstance, invocationLog, "add --all -- python/llm_proxy_client.egg-info/PKG-INFO python/llm_proxy_client.egg-info/SOURCES.txt")
	require.NotContains(testInstance, invocationLog, "add --all -- python/llm_proxy_client/__pycache__")
	require.NotContains(testInstance, invocationLog, "add --all -- python/tests/__pycache__")
	require.Contains(testInstance, invocationLog, "commit -m docs: sync tracked dirty work")
	require.Contains(testInstance, invocationLog, "push origin master")

	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "master")), strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "origin/master")))
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
