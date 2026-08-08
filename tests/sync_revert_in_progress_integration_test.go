package tests

import (
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

func TestSyncRejectsResolvedRevertBeforeMutatingRepository(testInstance *testing.T) {
	testInstance.Helper()

	const branchName = "bugfix/B095-app-owned-deploy-bundle"

	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	trackedPath := filepath.Join(repositoryPath, "deploy.txt")
	require.NoError(testInstance, os.WriteFile(trackedPath, []byte("base deployment\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "deploy.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "seed deployment")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", branchName)
	require.NoError(testInstance, os.WriteFile(trackedPath, []byte("app-owned deployment\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "deploy.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "add app-owned deployment")
	revertedCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))

	require.NoError(testInstance, os.WriteFile(trackedPath, []byte("gateway-owned deployment\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "deploy.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "replace app-owned deployment")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", branchName)

	revertCommand := exec.Command("git", "-C", repositoryPath, "revert", revertedCommit)
	revertCommand.Env = buildGitCommandEnvironment(nil)
	revertOutput, revertError := revertCommand.CombinedOutput()
	require.Error(testInstance, revertError, string(revertOutput))
	require.Contains(testInstance, string(revertOutput), "CONFLICT")

	require.NoError(testInstance, os.WriteFile(trackedPath, []byte("resolved revert\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "deploy.txt")
	require.NoError(testInstance, os.WriteFile(trackedPath, []byte("resolved revert\noperator follow-up\n"), 0o644))
	untrackedPath := filepath.Join(repositoryPath, "operator-notes.txt")
	require.NoError(testInstance, os.WriteFile(untrackedPath, []byte("preserve this note\n"), 0o644))

	headBefore := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	branchBefore := strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current"))
	revertHeadBefore := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "--verify", "REVERT_HEAD"))
	statusBefore := runGit(testInstance, repositoryPath, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	indexBefore := runGit(testInstance, repositoryPath, "ls-files", "--stage", "-z")
	cachedDiffBefore := runGit(testInstance, repositoryPath, "diff", "--cached", "--binary")
	worktreeDiffBefore := runGit(testInstance, repositoryPath, "diff", "--binary")
	trackedContentsBefore, trackedReadError := os.ReadFile(trackedPath)
	require.NoError(testInstance, trackedReadError)
	untrackedContentsBefore, untrackedReadError := os.ReadFile(untrackedPath)
	require.NoError(testInstance, untrackedReadError)

	var requestCount atomic.Int64
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		requestCount.Add(1)
		http.Error(responseWriter, "LLM must not be called while a revert is active", http.StatusInternalServerError)
	}))
	testInstance.Cleanup(llmServer.Close)

	configurationPath := writeDirtySyncMergedBranchConfiguration(testInstance, llmServer.URL)
	gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	require.NoError(testInstance, os.WriteFile(gitLogPath, nil, 0o600))
	require.NoError(testInstance, os.WriteFile(githubLogPath, nil, 0o600))

	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	output, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		repositoryPath,
		map[string]string{
			pathEnvironmentVariableNameConstant:  buildSyncMergedBranchExecutablePath(testInstance),
			syncMergedBranchGitLogVariable:       gitLogPath,
			syncMergedBranchGitHubLogVariable:    githubLogPath,
			syncMergedBranchNameVariable:         branchName,
			syncMergedBranchMergedVariable:       "false",
			syncRefreshIntegrationAPIKeyVariable: "test-key",
		},
		syncRefreshIntegrationTimeout,
		[]string{
			"--config",
			configurationPath,
			"--roots",
			repositoryPath,
			"sync",
		},
	)

	require.Error(testInstance, runError, output)
	require.Contains(testInstance, output, "operator-owned Git revert is in progress")
	require.Contains(testInstance, output, "git revert --continue")
	require.Contains(testInstance, output, "git revert --abort")
	require.Contains(testInstance, output, "git revert --quit")
	require.NotContains(testInstance, output, "cannot switch branch while reverting")
	require.Zero(testInstance, requestCount.Load())

	gitLogContents, gitLogReadError := os.ReadFile(gitLogPath)
	require.NoError(testInstance, gitLogReadError)
	gitLog := string(gitLogContents)
	require.Contains(testInstance, gitLog, "rev-parse --git-path REVERT_HEAD")
	require.Contains(
		testInstance,
		gitLog,
		"rev-parse --verify --quiet --end-of-options "+revertHeadBefore+"^{commit}",
	)
	for _, forbiddenInvocation := range []string{
		"fetch --prune origin",
		"switch " + branchName,
		"stash ",
		"reset",
		"add ",
		"commit ",
		"merge ",
		"push ",
	} {
		require.NotContains(testInstance, gitLog, forbiddenInvocation)
	}

	githubLogContents, githubLogReadError := os.ReadFile(githubLogPath)
	require.NoError(testInstance, githubLogReadError)
	require.NotContains(testInstance, string(githubLogContents), "pr ")

	require.Equal(testInstance, headBefore, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD")))
	require.Equal(testInstance, branchBefore, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, revertHeadBefore, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "--verify", "REVERT_HEAD")))
	require.Equal(testInstance, statusBefore, runGit(testInstance, repositoryPath, "status", "--porcelain=v1", "-z", "--untracked-files=all"))
	require.Equal(testInstance, indexBefore, runGit(testInstance, repositoryPath, "ls-files", "--stage", "-z"))
	require.Equal(testInstance, cachedDiffBefore, runGit(testInstance, repositoryPath, "diff", "--cached", "--binary"))
	require.Equal(testInstance, worktreeDiffBefore, runGit(testInstance, repositoryPath, "diff", "--binary"))

	trackedContentsAfter, trackedAfterReadError := os.ReadFile(trackedPath)
	require.NoError(testInstance, trackedAfterReadError)
	require.Equal(testInstance, trackedContentsBefore, trackedContentsAfter)
	untrackedContentsAfter, untrackedAfterReadError := os.ReadFile(untrackedPath)
	require.NoError(testInstance, untrackedAfterReadError)
	require.Equal(testInstance, untrackedContentsBefore, untrackedContentsAfter)

	require.Equal(
		testInstance,
		"MM deploy.txt\x00?? operator-notes.txt\x00",
		statusBefore,
	)
}

func TestSyncRejectsResolvedRevertInSiblingBeforeMutatingRepository(testInstance *testing.T) {
	testInstance.Helper()

	const branchName = "bugfix/B095-app-owned-deploy-bundle"

	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	siblingPath := filepath.Join(workspacePath, "project-deploy-bundle")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	trackedPath := filepath.Join(repositoryPath, "deploy.txt")
	require.NoError(testInstance, os.WriteFile(trackedPath, []byte("base deployment\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "deploy.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "seed deployment")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", branchName)
	require.NoError(testInstance, os.WriteFile(trackedPath, []byte("app-owned deployment\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "deploy.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "add app-owned deployment")
	revertedCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))

	require.NoError(testInstance, os.WriteFile(trackedPath, []byte("gateway-owned deployment\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "deploy.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "replace app-owned deployment")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", branchName)
	remoteBranchBefore := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "origin/"+branchName))

	runGit(testInstance, repositoryPath, "switch", "master")
	runGit(testInstance, repositoryPath, "worktree", "add", siblingPath, branchName)
	siblingTrackedPath := filepath.Join(siblingPath, "deploy.txt")

	revertCommand := exec.Command("git", "-C", siblingPath, "revert", revertedCommit)
	revertCommand.Env = buildGitCommandEnvironment(nil)
	revertOutput, revertError := revertCommand.CombinedOutput()
	require.Error(testInstance, revertError, string(revertOutput))
	require.Contains(testInstance, string(revertOutput), "CONFLICT")

	require.NoError(testInstance, os.WriteFile(siblingTrackedPath, []byte("resolved sibling revert\n"), 0o644))
	runGit(testInstance, siblingPath, "add", "deploy.txt")
	require.NoError(testInstance, os.WriteFile(siblingTrackedPath, []byte("resolved sibling revert\noperator follow-up\n"), 0o644))
	untrackedPath := filepath.Join(siblingPath, "operator-notes.txt")
	require.NoError(testInstance, os.WriteFile(untrackedPath, []byte("preserve this sibling note\n"), 0o644))

	mainHeadBefore := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	mainBranchBefore := strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current"))
	mainStatusBefore := runGit(testInstance, repositoryPath, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	siblingHeadBefore := strings.TrimSpace(runGit(testInstance, siblingPath, "rev-parse", "HEAD"))
	siblingBranchBefore := strings.TrimSpace(runGit(testInstance, siblingPath, "branch", "--show-current"))
	revertHeadBefore := strings.TrimSpace(runGit(testInstance, siblingPath, "rev-parse", "--verify", "REVERT_HEAD"))
	siblingStatusBefore := runGit(testInstance, siblingPath, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	siblingIndexBefore := runGit(testInstance, siblingPath, "ls-files", "--stage", "-z")
	siblingCachedDiffBefore := runGit(testInstance, siblingPath, "diff", "--cached", "--binary")
	siblingWorktreeDiffBefore := runGit(testInstance, siblingPath, "diff", "--binary")
	trackedContentsBefore, trackedReadError := os.ReadFile(siblingTrackedPath)
	require.NoError(testInstance, trackedReadError)
	untrackedContentsBefore, untrackedReadError := os.ReadFile(untrackedPath)
	require.NoError(testInstance, untrackedReadError)

	var requestCount atomic.Int64
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		requestCount.Add(1)
		http.Error(responseWriter, "LLM must not be called while a sibling revert is active", http.StatusInternalServerError)
	}))
	testInstance.Cleanup(llmServer.Close)

	configurationPath := writeDirtySyncMergedBranchConfiguration(testInstance, llmServer.URL)
	gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	require.NoError(testInstance, os.WriteFile(gitLogPath, nil, 0o600))
	require.NoError(testInstance, os.WriteFile(githubLogPath, nil, 0o600))

	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	output, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		repositoryPath,
		map[string]string{
			pathEnvironmentVariableNameConstant:  buildSyncMergedBranchExecutablePath(testInstance),
			syncMergedBranchGitLogVariable:       gitLogPath,
			syncMergedBranchGitHubLogVariable:    githubLogPath,
			syncMergedBranchNameVariable:         branchName,
			syncMergedBranchMergedVariable:       "false",
			syncRefreshIntegrationAPIKeyVariable: "test-key",
		},
		syncRefreshIntegrationTimeout,
		[]string{
			"--config",
			configurationPath,
			"--roots",
			repositoryPath,
			"sync",
			branchName,
		},
	)

	require.Error(testInstance, runError, output)
	require.Contains(testInstance, output, "operator-owned Git revert is in progress")
	require.Contains(testInstance, output, siblingPath)
	require.Contains(testInstance, output, "git revert --continue")
	require.Contains(testInstance, output, "git revert --abort")
	require.Contains(testInstance, output, "git revert --quit")
	require.Zero(testInstance, requestCount.Load())

	gitLogContents, gitLogReadError := os.ReadFile(gitLogPath)
	require.NoError(testInstance, gitLogReadError)
	gitLog := string(gitLogContents)
	require.GreaterOrEqual(testInstance, strings.Count(gitLog, "rev-parse --git-path REVERT_HEAD"), 2)
	for _, forbiddenInvocation := range []string{
		"fetch --prune origin",
		"switch " + branchName,
		"stash ",
		"reset",
		"add ",
		"commit ",
		"merge ",
		"push ",
	} {
		require.NotContains(testInstance, gitLog, forbiddenInvocation)
	}

	githubLogContents, githubLogReadError := os.ReadFile(githubLogPath)
	require.NoError(testInstance, githubLogReadError)
	require.NotContains(testInstance, string(githubLogContents), "pr ")

	require.Equal(testInstance, mainHeadBefore, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD")))
	require.Equal(testInstance, mainBranchBefore, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, mainStatusBefore, runGit(testInstance, repositoryPath, "status", "--porcelain=v1", "-z", "--untracked-files=all"))
	require.Equal(testInstance, remoteBranchBefore, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "origin/"+branchName)))
	require.DirExists(testInstance, siblingPath)
	require.Equal(testInstance, siblingHeadBefore, strings.TrimSpace(runGit(testInstance, siblingPath, "rev-parse", "HEAD")))
	require.Equal(testInstance, siblingBranchBefore, strings.TrimSpace(runGit(testInstance, siblingPath, "branch", "--show-current")))
	require.Equal(testInstance, revertHeadBefore, strings.TrimSpace(runGit(testInstance, siblingPath, "rev-parse", "--verify", "REVERT_HEAD")))
	require.Equal(testInstance, siblingStatusBefore, runGit(testInstance, siblingPath, "status", "--porcelain=v1", "-z", "--untracked-files=all"))
	require.Equal(testInstance, siblingIndexBefore, runGit(testInstance, siblingPath, "ls-files", "--stage", "-z"))
	require.Equal(testInstance, siblingCachedDiffBefore, runGit(testInstance, siblingPath, "diff", "--cached", "--binary"))
	require.Equal(testInstance, siblingWorktreeDiffBefore, runGit(testInstance, siblingPath, "diff", "--binary"))

	trackedContentsAfter, trackedAfterReadError := os.ReadFile(siblingTrackedPath)
	require.NoError(testInstance, trackedAfterReadError)
	require.Equal(testInstance, trackedContentsBefore, trackedContentsAfter)
	untrackedContentsAfter, untrackedAfterReadError := os.ReadFile(untrackedPath)
	require.NoError(testInstance, untrackedAfterReadError)
	require.Equal(testInstance, untrackedContentsBefore, untrackedContentsAfter)
	require.Equal(
		testInstance,
		"MM deploy.txt\x00?? operator-notes.txt\x00",
		siblingStatusBefore,
	)
}

func TestSyncIgnoresOrdinaryBranchNamedRevertHead(testInstance *testing.T) {
	testInstance.Helper()

	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	readmePath := filepath.Join(repositoryPath, "README.md")
	require.NoError(testInstance, os.WriteFile(readmePath, []byte("clean repository\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "seed repository")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")
	runGit(testInstance, repositoryPath, "branch", "REVERT_HEAD")
	headBefore := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	ordinaryBranchBefore := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "refs/heads/REVERT_HEAD"))

	var requestCount atomic.Int64
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		requestCount.Add(1)
		http.Error(responseWriter, "LLM must not be called for clean master sync", http.StatusInternalServerError)
	}))
	testInstance.Cleanup(llmServer.Close)

	configurationPath := writeDirtySyncMergedBranchConfiguration(testInstance, llmServer.URL)
	gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	require.NoError(testInstance, os.WriteFile(gitLogPath, nil, 0o600))
	require.NoError(testInstance, os.WriteFile(githubLogPath, nil, 0o600))

	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	output, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		repositoryPath,
		map[string]string{
			pathEnvironmentVariableNameConstant:  buildSyncMergedBranchExecutablePath(testInstance),
			syncMergedBranchGitLogVariable:       gitLogPath,
			syncMergedBranchGitHubLogVariable:    githubLogPath,
			syncMergedBranchNameVariable:         "master",
			syncMergedBranchMergedVariable:       "false",
			syncRefreshIntegrationAPIKeyVariable: "test-key",
		},
		syncRefreshIntegrationTimeout,
		[]string{
			"--config",
			configurationPath,
			"--roots",
			repositoryPath,
			"sync",
			"master",
		},
	)

	require.NoError(testInstance, runError, output)
	require.Contains(testInstance, output, "SYNCED:")
	require.NotContains(testInstance, output, "operator-owned Git revert is in progress")
	require.Zero(testInstance, requestCount.Load())
	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, headBefore, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD")))
	require.Equal(testInstance, ordinaryBranchBefore, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "refs/heads/REVERT_HEAD")))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))

	gitLogContents, gitLogReadError := os.ReadFile(gitLogPath)
	require.NoError(testInstance, gitLogReadError)
	require.Contains(testInstance, string(gitLogContents), "rev-parse --git-path REVERT_HEAD")
}
