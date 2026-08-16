package tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	syncMergedBranchIntegrationTimeout          = 20 * time.Second
	syncMergedBranchOwnerRepository             = "owner/project"
	syncMergedBranchRemoteURL                   = "https://github.com/" + syncMergedBranchOwnerRepository + ".git"
	syncMergedBranchGitLogVariable              = "GIX_SYNC_TEST_GIT_LOG"
	syncMergedBranchGitHubLogVariable           = "GIX_SYNC_TEST_GH_LOG"
	syncMergedBranchOperationLogVariable        = "GIX_SYNC_TEST_OPERATION_LOG"
	syncMergedBranchFailPullRequestHeadVariable = "GIX_SYNC_TEST_FAIL_PR_HEAD"
	syncMergedBranchFailGitMatchVariable        = "GIX_SYNC_TEST_FAIL_GIT_MATCH"
	syncMergedBranchFailGitOccurrenceVariable   = "GIX_SYNC_TEST_FAIL_GIT_OCCURRENCE"
	syncMergedBranchFailGitStateVariable        = "GIX_SYNC_TEST_FAIL_GIT_STATE"
	syncMergedBranchConcurrentWorktreeVariable  = "GIX_SYNC_TEST_CONCURRENT_WORKTREE"
	syncMergedBranchStageCaptureMarkerVariable  = "GIX_SYNC_TEST_STAGE_CAPTURE_MARKER"
	syncMergedBranchStageCapturePathVariable    = "GIX_SYNC_TEST_STAGE_CAPTURE_PATH"
	syncMergedBranchStageCaptureResultVariable  = "GIX_SYNC_TEST_STAGE_CAPTURE_RESULT"
	syncMergedBranchNameVariable                = "GIX_SYNC_TEST_BRANCH"
	syncMergedBranchMergedVariable              = "GIX_SYNC_TEST_MERGED"
	syncMergedBranchMergedHeadOIDVariable       = "GIX_SYNC_TEST_MERGED_HEAD_OID"
	syncMergedBranchDefaultBranchVariable       = "GIX_SYNC_TEST_DEFAULT_BRANCH"
	syncMergedBranchFailRepoViewVariable        = "GIX_SYNC_TEST_FAIL_REPO_VIEW"
	syncMergedBranchAPIKeyVariable              = "GIX_SYNC_TEST_LLM_KEY"
)

type syncMergedBranchFixture struct {
	RemotePath     string
	RootPath       string
	RepositoryPath string
	BranchName     string
}

func TestSyncCurrentMergedBranchPromptsAndSyncsMasterBeforeCreatingPullRequest(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	branchName := "feature/squashed-review"
	fixture := createSyncMergedBranchFixture(testInstance, branchName)
	configurationPath := writeSyncMergedBranchConfiguration(testInstance)
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)
	mergedHeadOID := strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "rev-parse", branchName))

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncMergedBranchGitHubLogVariable:     githubLogPath,
				syncMergedBranchNameVariable:          branchName,
				syncMergedBranchMergedHeadOIDVariable: mergedHeadOID,
			},
		},
		syncMergedBranchIntegrationTimeout,
		"y\n",
		[]string{"run", ".", "--config", configurationPath, "--log-level", "error", "sync", "--roots", fixture.RepositoryPath},
	)
	require.NoError(testInstance, runError, output)

	require.Contains(testInstance, output, `Pull request for branch "feature/squashed-review" into master is already merged. Sync master instead?`)
	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (master)", fixture.RepositoryPath))
	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, "squashed review\n", readTextFile(testInstance, filepath.Join(fixture.RepositoryPath, "feature.txt")))

	githubLog := readTextFile(testInstance, githubLogPath)
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state open --head feature/squashed-review")
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state merged --head feature/squashed-review")
	require.NotContains(testInstance, githubLog, "pr create")
}

func TestSyncTransitivelyMergedBranchPromptsAndSyncsMasterWithoutRecordedReviewBase(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	branchName := "feature/stack-child"
	middleBranchName := "feature/stack-middle"
	parentBranchName := "feature/stack-parent"
	fixture := createSyncTransitivelyMergedBranchFixture(testInstance, branchName, middleBranchName, parentBranchName)
	configurationPath := writeSyncMergedBranchConfiguration(testInstance)
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	branchMergedHeadOID := strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "rev-parse", branchName))
	middleMergedHeadOID := strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "rev-parse", middleBranchName))
	parentMergedHeadOID := strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "rev-parse", parentBranchName))
	reviewChain := fmt.Sprintf(
		"merged-pr --base %s --head %s --oid %s\nmerged-pr --base %s --head %s --oid %s\nmerged-pr --base master --head %s --oid %s\n",
		middleBranchName,
		branchName,
		branchMergedHeadOID,
		parentBranchName,
		middleBranchName,
		middleMergedHeadOID,
		parentBranchName,
		parentMergedHeadOID,
	)
	require.NoError(testInstance, os.WriteFile(githubLogPath, []byte(reviewChain), 0o600))
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncMergedBranchGitHubLogVariable: githubLogPath,
				syncMergedBranchNameVariable:      branchName,
				syncMergedBranchMergedVariable:    "false",
			},
		},
		syncMergedBranchIntegrationTimeout,
		"y\n",
		[]string{"run", ".", "--config", configurationPath, "--log-level", "error", "sync", "--roots", fixture.RepositoryPath},
	)
	require.NoError(testInstance, runError, output)

	require.Contains(testInstance, output, `Pull request for branch "feature/stack-child" into master is already merged. Sync master instead?`)
	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (master)", fixture.RepositoryPath))
	require.NotContains(testInstance, output, "SYNC_SWITCH_ROLLBACK")
	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, "child review\n", readTextFile(testInstance, filepath.Join(fixture.RepositoryPath, "child.txt")))

	githubLog := readTextFile(testInstance, githubLogPath)
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state merged --head "+branchName)
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state merged --head "+middleBranchName)
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state merged --head "+parentBranchName)
	require.NotContains(testInstance, githubLog, "pr create")
}

func TestSyncTransitivelyMergedBranchFollowsDeletedParentFromStaleLocalTip(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	branchName := "feature/deleted-stack-child"
	parentBranchName := "feature/deleted-stack-parent"
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	publisherPath := filepath.Join(workspacePath, "publisher")
	repositoryRootPath := filepath.Join(workspacePath, "roots")
	repositoryPath := filepath.Join(repositoryRootPath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, publisherPath)

	require.NoError(testInstance, os.WriteFile(filepath.Join(publisherPath, "README.md"), []byte("initial\n"), 0o644))
	runGit(testInstance, publisherPath, "add", "README.md")
	runGit(testInstance, publisherPath, "commit", "-m", "initial commit")
	runGit(testInstance, publisherPath, "push", "-u", "origin", "master")

	runGit(testInstance, publisherPath, "switch", "-c", parentBranchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(publisherPath, "parent.txt"), []byte("parent review\n"), 0o644))
	runGit(testInstance, publisherPath, "add", "parent.txt")
	runGit(testInstance, publisherPath, "commit", "-m", "parent branch commit")
	runGit(testInstance, publisherPath, "push", "-u", "origin", parentBranchName)
	staleParentHeadOID := strings.TrimSpace(runGit(testInstance, publisherPath, "rev-parse", parentBranchName))

	runGit(testInstance, publisherPath, "switch", "-c", branchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(publisherPath, "child.txt"), []byte("child review\n"), 0o644))
	runGit(testInstance, publisherPath, "add", "child.txt")
	runGit(testInstance, publisherPath, "commit", "-m", "child branch commit")
	runGit(testInstance, publisherPath, "push", "-u", "origin", branchName)
	branchMergedHeadOID := strings.TrimSpace(runGit(testInstance, publisherPath, "rev-parse", branchName))

	require.NoError(testInstance, os.MkdirAll(repositoryRootPath, 0o755))
	runGitWithDir(testInstance, "", "clone", localFileURL(remotePath), repositoryPath)
	configureGitIdentity(testInstance, repositoryPath)
	runGit(testInstance, repositoryPath, "switch", "-c", parentBranchName, "--track", "origin/"+parentBranchName)
	runGit(testInstance, repositoryPath, "switch", "-c", branchName, "--track", "origin/"+branchName)

	runGit(testInstance, publisherPath, "switch", parentBranchName)
	runGit(testInstance, publisherPath, "merge", "--no-ff", branchName, "-m", "merge child review")
	runGit(testInstance, publisherPath, "push", "origin", parentBranchName)
	parentMergedHeadOID := strings.TrimSpace(runGit(testInstance, publisherPath, "rev-parse", parentBranchName))
	runGitWithDir(testInstance, "", "--git-dir", remotePath, "update-ref", "refs/pull/9/head", parentMergedHeadOID)
	runGit(testInstance, publisherPath, "switch", "master")
	runGit(testInstance, publisherPath, "merge", "--no-ff", branchName, "-m", "merge reviewed stack")
	runGit(testInstance, publisherPath, "push", "origin", "master")
	runGit(testInstance, publisherPath, "push", "origin", "--delete", branchName, parentBranchName)

	missingHeadCommand := exec.Command("git", "-C", repositoryPath, "cat-file", "-e", parentMergedHeadOID+"^{commit}")
	missingHeadCommand.Env = buildGitCommandEnvironment(nil)
	require.Error(testInstance, missingHeadCommand.Run())
	runGit(testInstance, repositoryPath, "switch", branchName)

	configurationPath := writeSyncMergedBranchConfiguration(testInstance)
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
	reviewChain := fmt.Sprintf(
		"merged-pr --base %s --head %s --oid %s\nmerged-pr --base master --head %s --oid %s\n",
		parentBranchName,
		branchName,
		branchMergedHeadOID,
		parentBranchName,
		parentMergedHeadOID,
	)
	require.NoError(testInstance, os.WriteFile(githubLogPath, []byte(reviewChain), 0o600))
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncMergedBranchGitLogVariable:    gitLogPath,
				syncMergedBranchGitHubLogVariable: githubLogPath,
				syncMergedBranchNameVariable:      branchName,
				syncMergedBranchMergedVariable:    "false",
			},
		},
		syncMergedBranchIntegrationTimeout,
		"y\n",
		[]string{"run", ".", "--config", configurationPath, "--log-level", "error", "sync", "--roots", repositoryPath},
	)
	require.NoError(testInstance, runError, output)

	require.Contains(testInstance, output, `Pull request for branch "feature/deleted-stack-child" into master is already merged. Sync master instead?`)
	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (master)", repositoryPath))
	require.NotContains(testInstance, output, "SYNC_SWITCH_ROLLBACK")
	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, staleParentHeadOID, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", parentBranchName)))
	require.Equal(testInstance, parentMergedHeadOID, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", parentMergedHeadOID+"^{commit}")))
	require.Equal(testInstance, "child review\n", readTextFile(testInstance, filepath.Join(repositoryPath, "child.txt")))

	githubLog := readTextFile(testInstance, githubLogPath)
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state merged --head "+branchName)
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state merged --head "+parentBranchName)
	require.NotContains(testInstance, githubLog, "pr create")
	require.Contains(testInstance, readTextFile(testInstance, gitLogPath), "fetch --no-tags origin refs/pull/9/head")
}

func TestSyncReusedMergedBranchHeadCreatesNewPullRequestInsteadOfHandoff(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	branchName := "feature/reused-stack-child"
	middleBranchName := "feature/reused-stack-middle"
	parentBranchName := "feature/reused-stack-parent"
	fixture := createSyncTransitivelyMergedBranchFixture(testInstance, branchName, middleBranchName, parentBranchName)
	mergedBranchHeadOID := strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "rev-parse", branchName))
	middleMergedHeadOID := strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "rev-parse", middleBranchName))
	parentMergedHeadOID := strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "rev-parse", parentBranchName))

	reusedPath := filepath.Join(fixture.RepositoryPath, "reused.txt")
	require.NoError(testInstance, os.WriteFile(reusedPath, []byte("new review work\n"), 0o644))
	runGit(testInstance, fixture.RepositoryPath, "add", "reused.txt")
	runGit(testInstance, fixture.RepositoryPath, "commit", "-m", "reuse child branch for new review")
	runGit(testInstance, fixture.RepositoryPath, "push", "origin", branchName)
	reusedBranchHeadOID := strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "rev-parse", branchName))
	require.NotEqual(testInstance, mergedBranchHeadOID, reusedBranchHeadOID)

	configurationPath := writeSyncMergedBranchConfiguration(testInstance)
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	reviewChain := fmt.Sprintf(
		"merged-pr --base %s --head %s --oid %s\nmerged-pr --base %s --head %s --oid %s\nmerged-pr --base master --head %s --oid %s\n",
		middleBranchName,
		branchName,
		mergedBranchHeadOID,
		parentBranchName,
		middleBranchName,
		middleMergedHeadOID,
		parentBranchName,
		parentMergedHeadOID,
	)
	require.NoError(testInstance, os.WriteFile(githubLogPath, []byte(reviewChain), 0o600))
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)
	pullRequestBody := "Publish the reused branch's new commits for review."

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncMergedBranchGitHubLogVariable: githubLogPath,
				syncMergedBranchNameVariable:      branchName,
				syncMergedBranchMergedVariable:    "false",
			},
		},
		syncMergedBranchIntegrationTimeout,
		"y\n",
		[]string{"run", ".", "--config", configurationPath, "--log-level", "error", "sync", "--body", pullRequestBody, "--roots", fixture.RepositoryPath},
	)
	require.NoError(testInstance, runError, output)

	require.NotContains(testInstance, output, "is already merged. Sync")
	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (%s)", fixture.RepositoryPath, branchName))
	require.Equal(testInstance, branchName, strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, "new review work\n", readTextFile(testInstance, reusedPath))

	githubLog := readTextFile(testInstance, githubLogPath)
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state merged --head "+branchName)
	require.Contains(testInstance, githubLog, "pr create --repo owner/project --base master --head "+branchName+" --title "+branchName+" --body "+pullRequestBody)
}

func TestSyncDirtyCurrentMergedBranchRejectsCommitBeforeHandoff(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	branchName := "feature/squashed-review-dirty"
	fixture := createSyncMergedBranchFixture(testInstance, branchName)
	originalHead := strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "rev-parse", "HEAD"))
	require.NoError(testInstance, os.WriteFile(filepath.Join(fixture.RepositoryPath, "README.md"), []byte("dirty work for a new review\n"), 0o644))

	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		http.Error(responseWriter, "merged branches must be rejected before commit message generation", http.StatusInternalServerError)
	}))
	testInstance.Cleanup(llmServer.Close)

	configurationPath := writeDirtySyncMergedBranchConfiguration(testInstance, llmServer.URL)
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncMergedBranchAPIKeyVariable:        "test-key",
				syncMergedBranchGitHubLogVariable:     githubLogPath,
				syncMergedBranchNameVariable:          branchName,
				syncMergedBranchMergedHeadOIDVariable: originalHead,
			},
		},
		syncMergedBranchIntegrationTimeout,
		"",
		[]string{"run", ".", "--config", configurationPath, "--log-level", "error", "sync", "--roots", fixture.RepositoryPath},
	)
	require.Error(testInstance, runError)
	require.Contains(testInstance, output, `cannot commit uncommitted changes on merged branch "`+branchName+`"`)
	require.Equal(testInstance, originalHead, strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "rev-parse", "HEAD")))
	require.Equal(testInstance, "M README.md", strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "status", "--porcelain")))

	githubLog := readTextFile(testInstance, githubLogPath)
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state open --head "+branchName)
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state merged --head "+branchName)
	require.NotContains(testInstance, githubLog, "pr create")
}

func TestSyncExplicitMasterReleasesMainWorktreeAndSwitchesLinkedWorktree(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	mainRepositoryPath := filepath.Join(workspacePath, "main", "project")
	linkedRootPath := filepath.Join(workspacePath, "linked")
	linkedWorktreePath := filepath.Join(linkedRootPath, "project-linked")
	branchName := "feature/linked-sync"
	createSyncGitHubBackedRepository(testInstance, remotePath, mainRepositoryPath)

	require.NoError(testInstance, os.WriteFile(filepath.Join(mainRepositoryPath, "README.md"), []byte("initial\n"), 0o644))
	runGit(testInstance, mainRepositoryPath, "add", "README.md")
	runGit(testInstance, mainRepositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, mainRepositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, mainRepositoryPath, "switch", "-c", branchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(mainRepositoryPath, "feature.txt"), []byte("feature work\n"), 0o644))
	runGit(testInstance, mainRepositoryPath, "add", "feature.txt")
	runGit(testInstance, mainRepositoryPath, "commit", "-m", "feature work")
	runGit(testInstance, mainRepositoryPath, "push", "-u", "origin", branchName)
	runGit(testInstance, mainRepositoryPath, "switch", "master")
	require.NoError(testInstance, os.MkdirAll(linkedRootPath, 0o755))
	runGit(testInstance, mainRepositoryPath, "worktree", "add", linkedWorktreePath, branchName)

	configurationPath := writeSyncMergedBranchConfiguration(testInstance)
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncMergedBranchGitHubLogVariable: githubLogPath,
				syncMergedBranchNameVariable:      branchName,
			},
		},
		syncMergedBranchIntegrationTimeout,
		"",
		[]string{"run", ".", "--config", configurationPath, "--log-level", "error", "sync", "master", "--roots", linkedWorktreePath},
	)
	require.NoError(testInstance, runError, output)

	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (master)", linkedWorktreePath))
	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, linkedWorktreePath, "branch", "--show-current")))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, mainRepositoryPath, "branch", "--show-current")))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, mainRepositoryPath, "status", "--porcelain")))
	require.NotContains(testInstance, output, "main working tree")
}

func TestSyncExistingRemoteBranchWithoutPullRequestCreatesPullRequest(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	branchName := "feature/unreviewed-remote"
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", branchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "feature.txt"), []byte("needs review\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "feature.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "feature needs review")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", branchName)
	runGit(testInstance, repositoryPath, "switch", "master")

	configurationPath := writeSyncMergedBranchConfiguration(testInstance)
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)
	pullRequestBody := "Publish the existing remote branch for review."

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncMergedBranchGitHubLogVariable: githubLogPath,
				syncMergedBranchNameVariable:      branchName,
				syncMergedBranchMergedVariable:    "false",
			},
		},
		syncMergedBranchIntegrationTimeout,
		"",
		[]string{"run", ".", "--config", configurationPath, "--log-level", "error", "sync", branchName, "--body", pullRequestBody, "--roots", repositoryPath},
	)
	require.NoError(testInstance, runError, output)

	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (%s)", repositoryPath, branchName))
	require.Equal(testInstance, branchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	githubLog := readTextFile(testInstance, githubLogPath)
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state open --head feature/unreviewed-remote")
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state merged --head feature/unreviewed-remote")
	require.Contains(testInstance, githubLog, "pr create --repo owner/project --base master --head feature/unreviewed-remote --title feature/unreviewed-remote --body Publish the existing remote branch for review.")
}

func TestSyncDirtyExistingRemoteBranchWithoutPullRequestCommitsAndCreatesPullRequest(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	branchName := "bugfix/transactional-remote-release-state"
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", branchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "release.txt"), []byte("prepared release\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "release.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "prepare release state")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", branchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "release.txt"), []byte("prepared release\ntransactional remote state\n"), 0o644))

	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fix: preserve transactional remote release state"}}]}`))
	}))
	testInstance.Cleanup(llmServer.Close)

	configurationPath := writeDirtySyncMergedBranchConfiguration(testInstance, llmServer.URL)
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)
	pullRequestBody := "Keep remote release state transactional."

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncMergedBranchAPIKeyVariable:    "test-key",
				syncMergedBranchGitHubLogVariable: githubLogPath,
				syncMergedBranchNameVariable:      branchName,
				syncMergedBranchMergedVariable:    "false",
			},
		},
		syncMergedBranchIntegrationTimeout,
		"",
		[]string{"run", ".", "--config", configurationPath, "--log-level", "error", "sync", "--body", pullRequestBody, "--roots", repositoryPath},
	)
	require.NoError(testInstance, runError, output)

	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (%s)", repositoryPath, branchName))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
	require.Equal(testInstance, "2", strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-list", "--count", "origin/master.."+branchName)))
	require.Equal(testInstance, "fix: preserve transactional remote release state", strings.TrimSpace(runGit(testInstance, repositoryPath, "log", "-1", "--format=%s")))

	githubLog := readTextFile(testInstance, githubLogPath)
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state open --head "+branchName)
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state merged --head "+branchName)
	require.Contains(testInstance, githubLog, "pr create --repo owner/project --base master --head "+branchName+" --title "+branchName+" --body "+pullRequestBody)
}

func TestSyncDirtyEmptyLocalBranchCommitsTrackedExampleEnvMatchedByIgnoreRule(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	branchName := "bugfix/llm-env-inventory"
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	exampleEnvRelativePath := filepath.Join("configs", ".env.hecateapi.example")
	exampleEnvPath := filepath.Join(repositoryPath, exampleEnvRelativePath)
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(exampleEnvPath), 0o755))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, ".gitignore"), []byte("configs/.env.*\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(exampleEnvPath, []byte("LLM_MODEL=goofy-old-model\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", ".gitignore")
	runGit(testInstance, repositoryPath, "add", "-f", exampleEnvRelativePath)
	runGit(testInstance, repositoryPath, "commit", "-m", "document LLM environment")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", branchName)
	expectedExampleEnv := "LLM_MODEL=goofy-current-model\n"
	require.NoError(testInstance, os.WriteFile(exampleEnvPath, []byte(expectedExampleEnv), 0o644))
	require.Equal(
		testInstance,
		filepath.ToSlash(exampleEnvRelativePath),
		strings.TrimSpace(runGit(testInstance, repositoryPath, "ls-files", "--cached", "--ignored", "--exclude-standard", "--", exampleEnvRelativePath)),
	)

	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fix: update Hecate LLM environment inventory"}}]}`))
	}))
	testInstance.Cleanup(llmServer.Close)

	configurationPath := writeDirtySyncMergedBranchConfiguration(testInstance, llmServer.URL)
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)
	pullRequestBody := "Update the Hecate LLM environment inventory."

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncMergedBranchAPIKeyVariable:    "test-key",
				syncMergedBranchGitHubLogVariable: githubLogPath,
				syncMergedBranchNameVariable:      branchName,
				syncMergedBranchMergedVariable:    "false",
			},
		},
		syncMergedBranchIntegrationTimeout,
		"",
		[]string{"run", ".", "--config", configurationPath, "--log-level", "error", "sync", "--body", pullRequestBody, "--roots", repositoryPath},
	)
	require.NoError(testInstance, runError, output)

	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (%s)", repositoryPath, branchName))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
	require.Equal(testInstance, "1", strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-list", "--count", "origin/master.."+branchName)))
	require.Equal(testInstance, "fix: update Hecate LLM environment inventory", strings.TrimSpace(runGit(testInstance, repositoryPath, "log", "-1", "--format=%s")))
	require.Equal(testInstance, expectedExampleEnv, readTextFile(testInstance, exampleEnvPath))
	require.Equal(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", branchName)), strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "origin/"+branchName)))

	githubLog := readTextFile(testInstance, githubLogPath)
	require.Contains(testInstance, githubLog, "pr create --repo owner/project --base master --head "+branchName+" --title "+branchName+" --body "+pullRequestBody)
}

func TestSyncDirtyExistingRemoteBranchStagesDeletedPathContainingSpaces(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	branchName := "gix/update-dockerignore-to-include-owned-configs-workflows"
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	logoRelativePath := filepath.Join("legacy", "managing-director", "IMD Logo.png")
	logoPath := filepath.Join(repositoryPath, logoRelativePath)
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(logoPath), 0o755))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(logoPath, []byte("logo\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md", logoRelativePath)
	runGit(testInstance, repositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", branchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "workflow.txt"), []byte("owned workflow\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "workflow.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "add owned workflow")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", branchName)
	require.NoError(testInstance, os.Remove(logoPath))

	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fix: remove obsolete managing director logo"}}]}`))
	}))
	testInstance.Cleanup(llmServer.Close)

	configurationPath := writeDirtySyncMergedBranchConfiguration(testInstance, llmServer.URL)
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)
	pullRequestBody := "Remove the obsolete managing director logo."

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncMergedBranchAPIKeyVariable:    "test-key",
				syncMergedBranchGitHubLogVariable: githubLogPath,
				syncMergedBranchGitLogVariable:    gitLogPath,
				syncMergedBranchNameVariable:      branchName,
				syncMergedBranchMergedVariable:    "false",
			},
		},
		syncMergedBranchIntegrationTimeout,
		"",
		[]string{"run", ".", "--config", configurationPath, "--log-level", "error", "sync", "--body", pullRequestBody, "--roots", repositoryPath},
	)
	require.NoError(testInstance, runError, output)

	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (%s)", repositoryPath, branchName))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
	require.Equal(testInstance, "2", strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-list", "--count", "origin/master.."+branchName)))
	require.NotContains(testInstance, runGit(testInstance, repositoryPath, "ls-tree", "-r", "--name-only", "HEAD"), filepath.ToSlash(logoRelativePath))
	require.Contains(testInstance, runGit(testInstance, repositoryPath, "show", "--format=", "--name-status", "HEAD"), "D\t"+filepath.ToSlash(logoRelativePath))

	gitLog := readTextFile(testInstance, gitLogPath)
	require.Contains(testInstance, gitLog, "add --force --all -- "+filepath.ToSlash(logoRelativePath))
	require.NotContains(testInstance, gitLog, `add --force --all -- "`+filepath.ToSlash(logoRelativePath)+`"`)
}

func createSyncMergedBranchFixture(testInstance *testing.T, branchName string) syncMergedBranchFixture {
	testInstance.Helper()

	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryRootPath := filepath.Join(workspacePath, "roots")
	repositoryPath := filepath.Join(repositoryRootPath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", branchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "feature.txt"), []byte("squashed review\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "feature.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "feature branch commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", branchName)

	runGit(testInstance, repositoryPath, "switch", "master")
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "feature.txt"), []byte("squashed review\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "feature.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "squash merged feature")
	runGit(testInstance, repositoryPath, "push", "origin", "master")
	runGit(testInstance, repositoryPath, "switch", branchName)

	return syncMergedBranchFixture{
		RemotePath:     remotePath,
		RootPath:       repositoryRootPath,
		RepositoryPath: repositoryPath,
		BranchName:     branchName,
	}
}

func createSyncTransitivelyMergedBranchFixture(testInstance *testing.T, branchName string, middleBranchName string, parentBranchName string) syncMergedBranchFixture {
	testInstance.Helper()

	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryRootPath := filepath.Join(workspacePath, "roots")
	repositoryPath := filepath.Join(repositoryRootPath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", parentBranchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "parent.txt"), []byte("parent review\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "parent.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "parent branch commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", parentBranchName)

	runGit(testInstance, repositoryPath, "switch", "-c", middleBranchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "middle.txt"), []byte("middle review\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "middle.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "middle branch commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", middleBranchName)

	runGit(testInstance, repositoryPath, "switch", "-c", branchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "child.txt"), []byte("child review\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "child.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "child branch commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", branchName)

	runGit(testInstance, repositoryPath, "switch", middleBranchName)
	runGit(testInstance, repositoryPath, "merge", "--no-ff", branchName, "-m", "merge child review")
	runGit(testInstance, repositoryPath, "push", "origin", middleBranchName)
	runGit(testInstance, repositoryPath, "switch", parentBranchName)
	runGit(testInstance, repositoryPath, "merge", "--no-ff", middleBranchName, "-m", "merge middle review")
	runGit(testInstance, repositoryPath, "push", "origin", parentBranchName)
	runGit(testInstance, repositoryPath, "switch", "master")
	runGit(testInstance, repositoryPath, "merge", "--no-ff", parentBranchName, "-m", "merge parent review")
	runGit(testInstance, repositoryPath, "push", "origin", "master")
	runGit(testInstance, repositoryPath, "switch", branchName)

	return syncMergedBranchFixture{
		RemotePath:     remotePath,
		RootPath:       repositoryRootPath,
		RepositoryPath: repositoryPath,
		BranchName:     branchName,
	}
}

func createSyncGitHubBackedRepository(testInstance *testing.T, remotePath string, repositoryPath string) {
	testInstance.Helper()

	require.NoError(testInstance, os.MkdirAll(filepath.Dir(remotePath), 0o755))
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(repositoryPath), 0o755))
	runGitWithDir(testInstance, "", "init", "--bare", remotePath)
	runGitWithDir(testInstance, "", "init", "--initial-branch=master", repositoryPath)
	configureGitIdentity(testInstance, repositoryPath)
	runGit(testInstance, repositoryPath, "remote", "add", "origin", localFileURL(remotePath))
}

func writeSyncMergedBranchConfiguration(testInstance *testing.T) string {
	testInstance.Helper()

	configurationPath := filepath.Join(testInstance.TempDir(), "config.yml")
	configurationContent := `common:
  log_level: error
  log_format: console
github:
  credential: test-github-key
llm:
  openai:
    priority: 1
    model: gpt-5.6-terra
    base_url: https://api.openai.com/v1
    credential: integration-openai-key
  llm_proxy:
    priority: 2
    provider: meta
    model: muse-spark-1.1
    base_url: https://llm-proxy.example
    credential: integration-proxy-key
  max_completion_tokens: 1200
operations:
  - command: ["sync"]
    with:
      remote: origin
      require_clean: true
`
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))
	return configurationPath
}

func writeDirtySyncMergedBranchConfiguration(testInstance *testing.T, baseURL string) string {
	testInstance.Helper()

	configurationPath := filepath.Join(testInstance.TempDir(), "config.yml")
	configurationContent := fmt.Sprintf(`common:
  log_level: error
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
  effort: "high"
  timeout_seconds: 5
operations:
  - command: ["sync"]
    with:
      remote: origin
  - command: ["message", "commit"]
    with:
      diff_source: staged
      max_completion_tokens: 64
      effort: "high"
      timeout_seconds: 5
`, baseURL)
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))
	return configurationPath
}

func syncMergedBranchGitHubStubScript() string {
	return `#!/bin/sh
printf '%s\n' "$*" >>"$GIX_SYNC_TEST_GH_LOG"
if [ -n "$GIX_SYNC_TEST_OPERATION_LOG" ]; then
  printf 'gh %s\n' "$*" >>"$GIX_SYNC_TEST_OPERATION_LOG"
fi

if [ "$1" = "repo" ] && [ "$2" = "view" ]; then
  if [ "$GIX_SYNC_TEST_FAIL_REPO_VIEW" = "true" ]; then
    printf 'simulated repository metadata failure\n' >&2
    exit 1
  fi
  default_branch="${GIX_SYNC_TEST_DEFAULT_BRANCH:-master}"
  printf '{"nameWithOwner":"owner/project","description":"","defaultBranchRef":{"name":"%s"},"isInOrganization":false}\n' "$default_branch"
  exit 0
fi

find_pull_request_marker() {
  marker_kind="$1"
  expected_head="$2"
  expected_base="$3"
  awk -v kind="$marker_kind" -v expected_head="$expected_head" -v expected_base="$expected_base" '
    $1 == kind {
      marker_base = ""
      marker_head = ""
      for (field = 2; field <= NF; field += 2) {
        if ($field == "--base") {
          marker_base = $(field + 1)
        }
        if ($field == "--head") {
          marker_head = $(field + 1)
        }
      }
      if (marker_head == expected_head && (expected_base == "" || marker_base == expected_base)) {
        matched_marker = $0
      }
    }
    END {
      if (matched_marker != "") {
        print matched_marker
      }
    }
  ' "$GIX_SYNC_TEST_GH_LOG"
}

if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
  state=""
  base=""
  head=""
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --state)
        state="$2"
        shift
        ;;
      --base)
        base="$2"
        shift
        ;;
      --head)
        head="$2"
        shift
        ;;
    esac
    shift
  done
  merged_pull_request="$(find_pull_request_marker "merged-pr" "$head" "$base")"
  if [ "$state" = "open" ]; then
    if [ -n "$merged_pull_request" ]; then
      printf '%s\n' '[]'
      exit 0
    fi
    created_pull_request="$(find_pull_request_marker "created-pr" "$head" "$base")"
    if [ -n "$created_pull_request" ]; then
      base="$(printf '%s\n' "$created_pull_request" | sed -n 's/.*--base \([^ ]*\) --head.*/\1/p')"
      printf '[{"number":10,"title":"Open","headRefName":"%s","baseRefName":"%s"}]\n' "$head" "$base"
      exit 0
    fi
    printf '%s\n' '[]'
    exit 0
  fi
  if [ "$state" = "merged" ]; then
    if [ -n "$merged_pull_request" ]; then
      base="$(printf '%s\n' "$merged_pull_request" | sed -n 's/.*--base \([^ ]*\) --head.*/\1/p')"
      head_oid="$(printf '%s\n' "$merged_pull_request" | sed -n 's/.*--oid \([^ ]*\).*/\1/p')"
      printf '[{"number":9,"title":"Merged","headRefName":"%s","headRefOid":"%s","baseRefName":"%s"}]\n' "$head" "$head_oid" "$base"
      exit 0
    fi
    if [ "$GIX_SYNC_TEST_MERGED" = "false" ]; then
      printf '%s\n' '[]'
      exit 0
    fi
    if [ "$head" != "$GIX_SYNC_TEST_BRANCH" ] || { [ -n "$base" ] && [ "$base" != "master" ]; }; then
      printf '%s\n' '[]'
      exit 0
    fi
    printf '[{"number":9,"title":"Merged","headRefName":"%s","headRefOid":"%s","baseRefName":"master"}]\n' "$GIX_SYNC_TEST_BRANCH" "$GIX_SYNC_TEST_MERGED_HEAD_OID"
    exit 0
  fi
fi

if [ "$1" = "pr" ] && [ "$2" = "create" ]; then
  base=""
  head=""
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --base)
        base="$2"
        shift
        ;;
      --head)
        head="$2"
        shift
        ;;
    esac
    shift
  done
  if [ -n "$GIX_SYNC_TEST_FAIL_PR_HEAD" ] && [ "$head" = "$GIX_SYNC_TEST_FAIL_PR_HEAD" ]; then
    printf 'simulated pull request creation failure for %s\n' "$head" >&2
    exit 1
  fi
  printf 'created-pr --base %s --head %s\n' "$base" "$head" >>"$GIX_SYNC_TEST_GH_LOG"
  if [ -n "$GIX_SYNC_TEST_OPERATION_LOG" ]; then
    printf 'gh-created --base %s --head %s\n' "$base" "$head" >>"$GIX_SYNC_TEST_OPERATION_LOG"
  fi
  printf '%s\n' 'https://github.com/owner/project/pull/10'
  exit 0
fi

printf 'unexpected gh invocation: %s\n' "$*" >&2
exit 1
`
}

func buildSyncMergedBranchExecutablePath(testInstance *testing.T) string {
	testInstance.Helper()

	stubDirectory := testInstance.TempDir()
	realGitPath, lookupError := exec.LookPath("git")
	require.NoError(testInstance, lookupError)

	gitStubScript := fmt.Sprintf(`#!/bin/sh
real_git_path=%q
if [ -n "$GIX_SYNC_TEST_GIT_LOG" ]; then
  printf '%%s\n' "$*" >>"$GIX_SYNC_TEST_GIT_LOG"
fi
if [ -n "$GIX_SYNC_TEST_OPERATION_LOG" ]; then
  printf 'git %%s\n' "$*" >>"$GIX_SYNC_TEST_OPERATION_LOG"
fi
if [ -n "$GIX_SYNC_TEST_FAIL_GIT_MATCH" ]; then
  case "$*" in
    *"$GIX_SYNC_TEST_FAIL_GIT_MATCH"*)
      failure_count=0
      if [ -f "$GIX_SYNC_TEST_FAIL_GIT_STATE" ]; then
        failure_count="$(cat "$GIX_SYNC_TEST_FAIL_GIT_STATE")"
      fi
      failure_count=$((failure_count + 1))
      printf '%%s\n' "$failure_count" >"$GIX_SYNC_TEST_FAIL_GIT_STATE"
      if [ "$failure_count" -eq "$GIX_SYNC_TEST_FAIL_GIT_OCCURRENCE" ]; then
        if [ -n "$GIX_SYNC_TEST_CONCURRENT_WORKTREE" ]; then
          env -u GIT_INDEX_FILE "$real_git_path" -C "$GIX_SYNC_TEST_CONCURRENT_WORKTREE" add --all
          env -u GIT_INDEX_FILE "$real_git_path" -C "$GIX_SYNC_TEST_CONCURRENT_WORKTREE" commit -m "operator: concurrent unrelated work"
        fi
        printf 'simulated git failure for %%s\n' "$*" >&2
        exit 97
      fi
      ;;
  esac
fi
if [ -n "$GIX_SYNC_TEST_STAGE_CAPTURE_PATH" ] &&
   [ -f "$GIX_SYNC_TEST_STAGE_CAPTURE_MARKER" ] &&
   [ ! -f "$GIX_SYNC_TEST_STAGE_CAPTURE_RESULT" ] &&
   [ "$1" = "ls-files" ]; then
  case " $* " in
    *" --stage "*)
      capture_output_path="$GIX_SYNC_TEST_STAGE_CAPTURE_RESULT.capture"
      stage_output_path="$GIX_SYNC_TEST_STAGE_CAPTURE_RESULT.stage"
      "$real_git_path" "$@" >"$capture_output_path"
      capture_status=$?
      if [ "$capture_status" -ne 0 ]; then
        cat "$capture_output_path"
        exit "$capture_status"
      fi
      "$real_git_path" add -- "$GIX_SYNC_TEST_STAGE_CAPTURE_PATH" >"$stage_output_path" 2>&1
      stage_status=$?
      {
        printf 'exit=%%s\n' "$stage_status"
        cat "$stage_output_path"
      } >"$GIX_SYNC_TEST_STAGE_CAPTURE_RESULT"
      cat "$capture_output_path"
      exit 0
      ;;
  esac
fi
if [ "$1" = "remote" ] && [ "$2" = "get-url" ] && [ "$3" = "origin" ]; then
  printf '%%s\n' %q
  exit 0
fi
exec %q "$@"
`, realGitPath, syncMergedBranchRemoteURL, realGitPath)
	require.NoError(testInstance, os.WriteFile(filepath.Join(stubDirectory, "git"), []byte(gitStubScript), 0o755))
	require.NoError(testInstance, os.WriteFile(filepath.Join(stubDirectory, "gh"), []byte(syncMergedBranchGitHubStubScript()), 0o755))

	currentPath := os.Getenv(pathEnvironmentVariableNameConstant)
	if currentPath == "" {
		return stubDirectory
	}
	return stubDirectory + string(os.PathListSeparator) + currentPath
}

func runIntegrationCommandWithInput(testInstance *testing.T, repositoryRoot string, options integrationCommandOptions, timeout time.Duration, input string, arguments []string) (string, error) {
	testInstance.Helper()

	executionContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	command := exec.CommandContext(executionContext, "go", arguments...)
	command.Dir = repositoryRoot
	command.Env = buildCommandEnvironment(options)
	command.Stdin = strings.NewReader(input)

	outputBytes, runError := command.CombinedOutput()
	return string(outputBytes), runError
}

func localFileURL(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func syncHomeWorkspace(testInstance *testing.T) string {
	testInstance.Helper()
	homeDirectory, homeError := os.UserHomeDir()
	require.NoError(testInstance, homeError)
	workspacePath, workspaceError := os.MkdirTemp(homeDirectory, "gix-sync-merged-branch-")
	require.NoError(testInstance, workspaceError)
	testInstance.Cleanup(func() {
		_ = os.RemoveAll(workspacePath)
	})
	canonicalPath, canonicalError := filepath.EvalSymlinks(workspacePath)
	require.NoError(testInstance, canonicalError)
	return canonicalPath
}

func readTextFile(testInstance *testing.T, path string) string {
	testInstance.Helper()
	contents, readError := os.ReadFile(path)
	require.NoError(testInstance, readError)
	return string(contents)
}
