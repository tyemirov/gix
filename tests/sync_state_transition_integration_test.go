package tests

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	syncStateTransitionBranchName           = "feature/state-transition"
	syncStateTransitionTimeout              = 30 * time.Second
	strictSyncInvocationStashMessageForTest = "gix strict-sync invocation stash"
)

type syncStateSnapshot struct {
	Branch       string
	Head         string
	Heads        string
	Status       string
	Index        string
	CachedDiff   string
	WorktreeDiff string
	Stashes      string
	Worktrees    string
	Files        []string
}

type syncGitOperationFixture struct {
	Path       string
	Directory  bool
	Contents   string
	ExtraFiles map[string]string
}

func TestSyncOperatorOwnedPreflightTable(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)

	testCases := []struct {
		Name             string
		Operation        string
		Administrative   syncGitOperationFixture
		UseSibling       bool
		UnmergedIndex    bool
		ExpectedGuidance string
	}{
		{
			Name:             "merge_in_caller",
			Operation:        "merge",
			Administrative:   syncGitOperationFixture{Path: "MERGE_HEAD", Contents: "commit"},
			ExpectedGuidance: "git merge --continue",
		},
		{
			Name:             "revert_in_target_sibling",
			Operation:        "revert",
			Administrative:   syncGitOperationFixture{Path: "REVERT_HEAD", Contents: "commit"},
			UseSibling:       true,
			ExpectedGuidance: "git revert --continue",
		},
		{
			Name:             "cherry_pick_in_caller",
			Operation:        "cherry-pick",
			Administrative:   syncGitOperationFixture{Path: "CHERRY_PICK_HEAD", Contents: "commit"},
			ExpectedGuidance: "git cherry-pick --continue",
		},
		{
			Name:       "rebase_merge_in_target_sibling",
			Operation:  "rebase",
			UseSibling: true,
			Administrative: syncGitOperationFixture{
				Path:       "rebase-merge",
				Directory:  true,
				ExtraFiles: map[string]string{"head-name": "refs/heads/master\n"},
			},
			ExpectedGuidance: "git rebase --continue",
		},
		{
			Name:      "apply_mailbox_in_caller",
			Operation: "apply-mailbox",
			Administrative: syncGitOperationFixture{
				Path:       "rebase-apply",
				Directory:  true,
				ExtraFiles: map[string]string{"applying": "\n"},
			},
			ExpectedGuidance: "git am --continue",
		},
		{
			Name:             "bisect_in_target_sibling",
			Operation:        "bisect",
			Administrative:   syncGitOperationFixture{Path: "BISECT_START", Contents: "refs/heads/master\n"},
			UseSibling:       true,
			ExpectedGuidance: "git bisect reset",
		},
		{
			Name:      "sequencer_in_caller",
			Operation: "sequencer",
			Administrative: syncGitOperationFixture{
				Path:       "sequencer",
				Directory:  true,
				ExtraFiles: map[string]string{"todo": "pick 0000000 pending\n"},
			},
			ExpectedGuidance: "finish or abort",
		},
		{
			Name:             "stash_apply_conflict_in_caller",
			Operation:        "unmerged index",
			UnmergedIndex:    true,
			ExpectedGuidance: "resolve or restore it explicitly",
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		testInstance.Run(testCase.Name, func(testInstance *testing.T) {
			remotePath, repositoryPath := createSyncStateTransitionRepository(testInstance)
			operationPath := repositoryPath
			targetBranch := "master"
			if testCase.UseSibling {
				runGit(testInstance, repositoryPath, "switch", "-c", syncStateTransitionBranchName)
				siblingPath := filepath.Join(filepath.Dir(repositoryPath), "target-sibling")
				runGit(testInstance, repositoryPath, "worktree", "add", siblingPath, "master")
				operationPath = siblingPath
			}

			if testCase.UnmergedIndex {
				createSyncUnmergedStashState(testInstance, operationPath)
			} else {
				createSyncAdministrativeState(testInstance, operationPath, testCase.Administrative)
			}
			repositoryBefore := captureSyncState(testInstance, repositoryPath)
			operationBefore := captureSyncState(testInstance, operationPath)
			var administrativeBefore []string
			if testCase.Administrative.Path != "" {
				administrativeBefore = captureSyncAdministrativeState(testInstance, operationPath, testCase.Administrative.Path)
			}
			remoteBefore := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "refs/remotes/origin/master"))

			gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
			configurationPath := writeSyncMergedBranchConfiguration(testInstance)
			output, runError := runBinaryIntegrationCommand(
				testInstance,
				binaryPath,
				repositoryPath,
				map[string]string{
					pathEnvironmentVariableNameConstant: buildSyncMergedBranchExecutablePath(testInstance),
					syncMergedBranchGitLogVariable:      gitLogPath,
					syncMergedBranchNameVariable:        targetBranch,
					syncMergedBranchMergedVariable:      "false",
				},
				syncStateTransitionTimeout,
				[]string{"--config", configurationPath, "--log-level", "error", "--roots", repositoryPath, "sync", targetBranch},
			)

			require.Error(testInstance, runError, output)
			require.Contains(testInstance, output, "operator-owned Git "+testCase.Operation)
			require.Contains(testInstance, output, operationPath)
			require.Contains(testInstance, output, testCase.ExpectedGuidance)
			require.NotContains(testInstance, output, "SYNCED:")

			gitLog := readTextFile(testInstance, gitLogPath)
			for _, forbiddenInvocation := range []string{
				"fetch --prune",
				"stash push",
				"switch ",
				"reset",
				"add ",
				"commit ",
				"merge ",
				"push ",
			} {
				require.NotContains(testInstance, gitLog, forbiddenInvocation)
			}

			require.Equal(testInstance, repositoryBefore, captureSyncState(testInstance, repositoryPath))
			require.Equal(testInstance, operationBefore, captureSyncState(testInstance, operationPath))
			if testCase.Administrative.Path != "" {
				require.Equal(testInstance, administrativeBefore, captureSyncAdministrativeState(testInstance, operationPath, testCase.Administrative.Path))
			}
			require.Equal(testInstance, remoteBefore, strings.TrimSpace(runGit(testInstance, remotePath, "rev-parse", "refs/heads/master")))
		})
	}
}

func TestSyncFailureRollbackTable(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)

	testCases := []struct {
		Name              string
		FailureMatch      string
		FailureOccurrence string
		LLMFailure        bool
		UseSibling        bool
		StashConflict     bool
		PublishedFailure  bool
		NewTarget         bool
		NoopPushFailure   bool
		UnrelatedAdvance  bool
		ExpectedOutput    string
	}{
		{
			Name:           "message_failure_after_index_reset",
			LLMFailure:     true,
			ExpectedOutput: "simulated message failure",
		},
		{
			Name:              "second_cluster_commit_failure",
			FailureMatch:      "commit -m",
			FailureOccurrence: "2",
			ExpectedOutput:    "simulated git failure",
		},
		{
			Name:              "failure_preserves_unrelated_sibling_advance",
			FailureMatch:      "commit -m",
			FailureOccurrence: "2",
			UnrelatedAdvance:  true,
			ExpectedOutput:    "simulated git failure",
		},
		{
			Name:           "new_target_message_failure_removes_created_branch",
			LLMFailure:     true,
			NewTarget:      true,
			ExpectedOutput: "simulated message failure",
		},
		{
			Name:              "post_adoption_refetch_failure",
			FailureMatch:      "fetch --prune origin",
			FailureOccurrence: "2",
			UseSibling:        true,
			ExpectedOutput:    "simulated git failure",
		},
		{
			Name:             "stash_finalization_failure_after_publication",
			LLMFailure:       true,
			StashConflict:    true,
			PublishedFailure: true,
			ExpectedOutput:   "simulated message failure",
		},
		{
			Name:            "failure_after_noop_push_rolls_back",
			NoopPushFailure: true,
			ExpectedOutput:  "simulated pull request creation failure",
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		testInstance.Run(testCase.Name, func(testInstance *testing.T) {
			remotePath, repositoryPath := createSyncStateTransitionRepository(testInstance)
			targetBranch := "master"
			snapshotPaths := []string{repositoryPath}
			unrelatedWorktreePath := ""
			unrelatedBranch := ""
			unrelatedHeadBefore := ""
			if testCase.NewTarget {
				targetBranch = "feature/pre-publication-failure"
			}
			if testCase.NoopPushFailure {
				targetBranch = syncStateTransitionBranchName
				runGit(testInstance, repositoryPath, "switch", "-c", targetBranch)
				require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "feature.txt"), []byte("published feature\n"), 0o644))
				runGit(testInstance, repositoryPath, "add", "feature.txt")
				runGit(testInstance, repositoryPath, "commit", "-m", "publish feature")
				runGit(testInstance, repositoryPath, "push", "-u", "origin", targetBranch)
				runGit(testInstance, repositoryPath, "switch", "master")
			} else if testCase.UseSibling {
				targetBranch = syncStateTransitionBranchName
				runGit(testInstance, repositoryPath, "switch", "-c", targetBranch)
				require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "feature.txt"), []byte("published feature\n"), 0o644))
				runGit(testInstance, repositoryPath, "add", "feature.txt")
				runGit(testInstance, repositoryPath, "commit", "-m", "publish feature")
				runGit(testInstance, repositoryPath, "push", "-u", "origin", targetBranch)
				runGit(testInstance, repositoryPath, "switch", "master")
				siblingPath := filepath.Join(filepath.Dir(repositoryPath), "target-sibling")
				runGit(testInstance, repositoryPath, "worktree", "add", siblingPath, targetBranch)
				require.NoError(testInstance, os.WriteFile(filepath.Join(siblingPath, "feature.txt"), []byte("published feature\noperator sibling work\n"), 0o644))
				snapshotPaths = append(snapshotPaths, siblingPath)
			} else if testCase.StashConflict {
				targetBranch = syncStateTransitionBranchName
				runGit(testInstance, repositoryPath, "switch", "-c", targetBranch)
				require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "feature.txt"), []byte("feature review delta\n"), 0o644))
				runGit(testInstance, repositoryPath, "add", "feature.txt")
				runGit(testInstance, repositoryPath, "commit", "-m", "add feature review delta")
				runGit(testInstance, repositoryPath, "push", "-u", "origin", targetBranch)
				runGit(testInstance, repositoryPath, "switch", "master")
				require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\nmaster upstream\n"), 0o644))
				runGit(testInstance, repositoryPath, "add", "README.md")
				runGit(testInstance, repositoryPath, "commit", "-m", "add master upstream line")
				runGit(testInstance, repositoryPath, "push", "origin", "master")
				runGit(testInstance, repositoryPath, "switch", targetBranch)
				require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\nfeature dirty\n"), 0o644))
			} else {
				createSyncDirtyClusters(testInstance, repositoryPath)
			}
			if testCase.UnrelatedAdvance {
				unrelatedBranch = "feature/operator-owned"
				runGit(testInstance, repositoryPath, "branch", unrelatedBranch)
				unrelatedWorktreePath = filepath.Join(filepath.Dir(repositoryPath), "operator-sibling")
				runGit(testInstance, repositoryPath, "worktree", "add", unrelatedWorktreePath, unrelatedBranch)
				require.NoError(testInstance, os.WriteFile(filepath.Join(unrelatedWorktreePath, "operator.txt"), []byte("operator concurrent work\n"), 0o644))
				unrelatedHeadBefore = strings.TrimSpace(runGit(testInstance, unrelatedWorktreePath, "rev-parse", "HEAD"))
			}

			snapshotsBefore := make(map[string]syncStateSnapshot, len(snapshotPaths))
			for snapshotPathIndex := range snapshotPaths {
				snapshotPath := snapshotPaths[snapshotPathIndex]
				snapshotsBefore[snapshotPath] = captureSyncState(testInstance, snapshotPath)
			}
			remoteReference := "refs/heads/" + targetBranch
			remoteBefore := ""
			if !testCase.NewTarget {
				remoteBefore = strings.TrimSpace(runGit(testInstance, remotePath, "rev-parse", remoteReference))
			}

			var requestCount atomic.Int64
			llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				requestCount.Add(1)
				if testCase.LLMFailure {
					http.Error(responseWriter, "simulated message failure", http.StatusInternalServerError)
					return
				}
				responseWriter.Header().Set("Content-Type", "application/json")
				_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fix: preserve transactional sync state"}}]}`))
			}))
			testInstance.Cleanup(llmServer.Close)

			configurationPath := writeDirtySyncMergedBranchConfiguration(testInstance, llmServer.URL)
			gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
			gitFailureStatePath := filepath.Join(testInstance.TempDir(), "git-failure-count")
			githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
			if testCase.UseSibling || testCase.StashConflict {
				require.NoError(testInstance, os.WriteFile(githubLogPath, []byte("created-pr --base master --head "+targetBranch+"\n"), 0o600))
			}
			environment := map[string]string{
				pathEnvironmentVariableNameConstant: buildSyncMergedBranchExecutablePath(testInstance),
				syncMergedBranchAPIKeyVariable:      "test-key",
				syncMergedBranchGitLogVariable:      gitLogPath,
				syncMergedBranchGitHubLogVariable:   githubLogPath,
				syncMergedBranchNameVariable:        targetBranch,
				syncMergedBranchMergedVariable:      "false",
			}
			if testCase.NoopPushFailure {
				environment[syncMergedBranchFailPullRequestHeadVariable] = targetBranch
			}
			if testCase.UnrelatedAdvance {
				environment[syncMergedBranchConcurrentWorktreeVariable] = unrelatedWorktreePath
			}
			if testCase.FailureMatch != "" {
				environment[syncMergedBranchFailGitMatchVariable] = testCase.FailureMatch
				environment[syncMergedBranchFailGitOccurrenceVariable] = testCase.FailureOccurrence
				environment[syncMergedBranchFailGitStateVariable] = gitFailureStatePath
			}

			commandArguments := []string{"--config", configurationPath, "--log-level", "error", "--roots", repositoryPath, "sync", targetBranch, "--body", "Exercise transactional rollback."}
			if testCase.StashConflict {
				commandArguments = append(commandArguments, "--stash")
			}
			output, runError := runBinaryIntegrationCommand(
				testInstance,
				binaryPath,
				repositoryPath,
				environment,
				syncStateTransitionTimeout,
				commandArguments,
			)

			require.Error(testInstance, runError, output)
			require.Contains(testInstance, output, testCase.ExpectedOutput)
			require.NotContains(testInstance, output, "SYNCED:")
			if testCase.PublishedFailure {
				require.Contains(testInstance, output, "SYNC_SWITCH_HANDOFF")
				require.Equal(testInstance, targetBranch, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
				require.Contains(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")), "UU README.md")
				require.Contains(testInstance, runGit(testInstance, repositoryPath, "stash", "list"), strictSyncInvocationStashMessageForTest)
				require.NotEqual(testInstance, remoteBefore, strings.TrimSpace(runGit(testInstance, remotePath, "rev-parse", remoteReference)))
			} else {
				for snapshotPathIndex := range snapshotPaths {
					snapshotPath := snapshotPaths[snapshotPathIndex]
					expectedSnapshot := snapshotsBefore[snapshotPath]
					actualSnapshot := captureSyncState(testInstance, snapshotPath)
					if testCase.UnrelatedAdvance {
						unrelatedHeadAfter := strings.TrimSpace(runGit(testInstance, unrelatedWorktreePath, "rev-parse", "HEAD"))
						expectedSnapshot.Heads = strings.Replace(
							expectedSnapshot.Heads,
							unrelatedHeadBefore+" refs/heads/"+unrelatedBranch,
							unrelatedHeadAfter+" refs/heads/"+unrelatedBranch,
							1,
						)
						expectedSnapshot.Worktrees = strings.Replace(
							expectedSnapshot.Worktrees,
							"worktree "+unrelatedWorktreePath+"\nHEAD "+unrelatedHeadBefore+"\nbranch refs/heads/"+unrelatedBranch,
							"worktree "+unrelatedWorktreePath+"\nHEAD "+unrelatedHeadAfter+"\nbranch refs/heads/"+unrelatedBranch,
							1,
						)
					}
					require.Equal(testInstance, expectedSnapshot, actualSnapshot)
				}
			}
			if !testCase.PublishedFailure {
				if testCase.NewTarget {
					require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--list", targetBranch)))
				} else {
					require.Equal(testInstance, remoteBefore, strings.TrimSpace(runGit(testInstance, remotePath, "rev-parse", remoteReference)))
				}
			}
			if testCase.UnrelatedAdvance {
				require.NotEqual(testInstance, unrelatedHeadBefore, strings.TrimSpace(runGit(testInstance, unrelatedWorktreePath, "rev-parse", "HEAD")))
				require.Equal(testInstance, "operator: concurrent unrelated work", strings.TrimSpace(runGit(testInstance, unrelatedWorktreePath, "log", "-1", "--format=%s")))
				require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, unrelatedWorktreePath, "status", "--porcelain")))
			}
		})
	}
}

func TestSyncSuccessfulFinalizationTable(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)

	testCases := []struct {
		Name string
		Mode string
	}{
		{Name: "ordinary_administrative_ref_names", Mode: "refs"},
		{Name: "dirty_clusters_commit_and_remove_transaction_snapshot", Mode: "commit"},
		{Name: "stash_restores_exact_index_and_preserves_existing_stashes", Mode: "stash"},
		{Name: "stash_conflict_completes_semantic_finalization_before_success", Mode: "stash_conflict"},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		testInstance.Run(testCase.Name, func(testInstance *testing.T) {
			remotePath, repositoryPath := createSyncStateTransitionRepository(testInstance)
			targetBranch := "master"
			expectedStatus := ""
			expectedIndex := ""
			expectedFiles := []string(nil)
			expectedResolvedContents := ""
			expectedStashes := strings.TrimSpace(runGit(testInstance, repositoryPath, "stash", "list", "--format=%H %s"))
			arguments := []string{"sync", targetBranch}

			var requestCount atomic.Int64
			llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				currentRequest := requestCount.Add(1)
				responseWriter.Header().Set("Content-Type", "application/json")
				if testCase.Mode == "stash_conflict" {
					response := "GIX_MERGE_REVIEW_APPROVED"
					if currentRequest%2 == 1 {
						response = "GIX_MERGE_RESOLUTION_CONTENT_BEGIN\nmaster upstream\nfeature dirty\nGIX_MERGE_RESOLUTION_CONTENT_END"
					}
					_, _ = fmt.Fprintf(responseWriter, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, response)
					return
				}
				_, _ = responseWriter.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fix: finalize strict sync transaction"}}]}`))
			}))
			testInstance.Cleanup(llmServer.Close)
			configurationPath := writeDirtySyncMergedBranchConfiguration(testInstance, llmServer.URL)

			switch testCase.Mode {
			case "refs":
				for _, referenceName := range syncAdministrativeReferenceNamesForTest() {
					runGit(testInstance, repositoryPath, "branch", referenceName)
					runGit(testInstance, repositoryPath, "tag", referenceName+"-tag")
				}
				configurationPath = writeSyncMergedBranchConfiguration(testInstance)
			case "commit":
				createSyncDirtyClusters(testInstance, repositoryPath)
			case "stash":
				runGit(testInstance, repositoryPath, "switch", "-c", syncStateTransitionBranchName)
				require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "existing-stash.txt"), []byte("operator stash\n"), 0o644))
				runGit(testInstance, repositoryPath, "stash", "push", "--include-untracked", "-m", "operator-existing")
				readmePath := filepath.Join(repositoryPath, "README.md")
				require.NoError(testInstance, os.WriteFile(readmePath, []byte("staged operator work\n"), 0o644))
				runGit(testInstance, repositoryPath, "add", "README.md")
				require.NoError(testInstance, os.WriteFile(readmePath, []byte("staged operator work\nunstaged operator follow-up\n"), 0o644))
				require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "operator-note.txt"), []byte("untracked operator note\n"), 0o644))
				expectedStatus = runGit(testInstance, repositoryPath, "status", "--porcelain=v1", "-z", "--untracked-files=all")
				expectedIndex = runGit(testInstance, repositoryPath, "ls-files", "--stage", "-z")
				expectedFiles = snapshotSyncFiles(testInstance, repositoryPath)
				expectedStashes = strings.TrimSpace(runGit(testInstance, repositoryPath, "stash", "list", "--format=%H %s"))
				arguments = append(arguments, "--stash")
			case "stash_conflict":
				runGit(testInstance, repositoryPath, "switch", "-c", syncStateTransitionBranchName)
				runGit(testInstance, repositoryPath, "switch", "master")
				require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\nmaster upstream\n"), 0o644))
				runGit(testInstance, repositoryPath, "add", "README.md")
				runGit(testInstance, repositoryPath, "commit", "-m", "add master upstream line")
				runGit(testInstance, repositoryPath, "push", "origin", "master")
				runGit(testInstance, repositoryPath, "switch", syncStateTransitionBranchName)
				require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\nfeature dirty\n"), 0o644))
				expectedResolvedContents = "initial\nmaster upstream\nfeature dirty\n"
				arguments = append(arguments, "--stash")
			default:
				testInstance.Fatalf("unsupported successful transition mode %q", testCase.Mode)
			}

			gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
			output, runError := runBinaryIntegrationCommand(
				testInstance,
				binaryPath,
				repositoryPath,
				map[string]string{
					pathEnvironmentVariableNameConstant: buildSyncMergedBranchExecutablePath(testInstance),
					syncMergedBranchAPIKeyVariable:      "test-key",
					syncMergedBranchGitLogVariable:      gitLogPath,
					syncMergedBranchNameVariable:        targetBranch,
					syncMergedBranchMergedVariable:      "false",
				},
				syncStateTransitionTimeout,
				append([]string{"--config", configurationPath, "--log-level", "error", "--roots", repositoryPath}, arguments...),
			)

			require.NoError(testInstance, runError, output)
			require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (%s)", repositoryPath, targetBranch))
			require.Equal(testInstance, targetBranch, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
			require.Equal(testInstance, expectedStashes, strings.TrimSpace(runGit(testInstance, repositoryPath, "stash", "list", "--format=%H %s")))
			require.Equal(
				testInstance,
				strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", targetBranch)),
				strings.TrimSpace(runGit(testInstance, remotePath, "rev-parse", "refs/heads/"+targetBranch)),
			)

			switch testCase.Mode {
			case "refs":
				require.Zero(testInstance, requestCount.Load())
				for _, referenceName := range syncAdministrativeReferenceNamesForTest() {
					require.NotEmpty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "refs/heads/"+referenceName)))
				}
			case "commit":
				require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
				require.Equal(testInstance, int64(2), requestCount.Load())
			case "stash":
				require.Equal(testInstance, expectedStatus, runGit(testInstance, repositoryPath, "status", "--porcelain=v1", "-z", "--untracked-files=all"))
				require.Equal(testInstance, expectedIndex, runGit(testInstance, repositoryPath, "ls-files", "--stage", "-z"))
				require.Equal(testInstance, expectedFiles, snapshotSyncFiles(testInstance, repositoryPath))
				require.Zero(testInstance, requestCount.Load())
			case "stash_conflict":
				require.Equal(testInstance, expectedResolvedContents, readTextFile(testInstance, filepath.Join(repositoryPath, "README.md")))
				require.NotEmpty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
				require.GreaterOrEqual(testInstance, requestCount.Load(), int64(1))
			}
		})
	}
}

func syncAdministrativeReferenceNamesForTest() []string {
	return []string{"MERGE_HEAD", "REVERT_HEAD", "CHERRY_PICK_HEAD", "rebase-merge", "rebase-apply", "BISECT_START", "sequencer"}
}

func createSyncStateTransitionRepository(testInstance *testing.T) (string, string) {
	testInstance.Helper()
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "seed strict sync state")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")
	return remotePath, repositoryPath
}

func createSyncDirtyClusters(testInstance *testing.T, repositoryPath string) {
	testInstance.Helper()
	require.NoError(testInstance, os.MkdirAll(filepath.Join(repositoryPath, "docs"), 0o755))
	require.NoError(testInstance, os.MkdirAll(filepath.Join(repositoryPath, "pkg"), 0o755))
	documentPath := filepath.Join(repositoryPath, "docs", "contract.md")
	require.NoError(testInstance, os.WriteFile(documentPath, []byte("staged contract\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "docs/contract.md")
	require.NoError(testInstance, os.WriteFile(documentPath, []byte("staged contract\nunstaged clarification\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "pkg", "state.go"), []byte("package state\n"), 0o644))
}

func createSyncAdministrativeState(testInstance *testing.T, repositoryPath string, fixture syncGitOperationFixture) {
	testInstance.Helper()
	administrativePath := resolveSyncAdministrativePath(testInstance, repositoryPath, fixture.Path)
	if fixture.Directory {
		require.NoError(testInstance, os.MkdirAll(administrativePath, 0o700))
		for relativePath, contents := range fixture.ExtraFiles {
			targetPath := filepath.Join(administrativePath, relativePath)
			require.NoError(testInstance, os.MkdirAll(filepath.Dir(targetPath), 0o700))
			require.NoError(testInstance, os.WriteFile(targetPath, []byte(contents), 0o600))
		}
		return
	}

	contents := fixture.Contents
	if contents == "commit" {
		contents = strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD")) + "\n"
	}
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(administrativePath), 0o700))
	require.NoError(testInstance, os.WriteFile(administrativePath, []byte(contents), 0o600))
}

func createSyncUnmergedStashState(testInstance *testing.T, repositoryPath string) {
	testInstance.Helper()
	readmePath := filepath.Join(repositoryPath, "README.md")
	require.NoError(testInstance, os.WriteFile(readmePath, []byte("operator stash contents\n"), 0o644))
	runGit(testInstance, repositoryPath, "stash", "push", "--include-untracked", "-m", "operator-conflict")
	require.NoError(testInstance, os.WriteFile(readmePath, []byte("committed conflicting contents\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "create conflicting head")

	applyCommand := exec.Command("git", "-C", repositoryPath, "stash", "apply", "--index", "stash@{0}")
	applyCommand.Env = buildGitCommandEnvironment(nil)
	applyOutput, applyErr := applyCommand.CombinedOutput()
	require.Error(testInstance, applyErr, string(applyOutput))
	require.Contains(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")), "UU README.md")
}

func captureSyncState(testInstance *testing.T, repositoryPath string) syncStateSnapshot {
	testInstance.Helper()
	return syncStateSnapshot{
		Branch:       runGit(testInstance, repositoryPath, "branch", "--show-current"),
		Head:         runGit(testInstance, repositoryPath, "rev-parse", "HEAD"),
		Heads:        runGit(testInstance, repositoryPath, "show-ref", "--heads"),
		Status:       runGit(testInstance, repositoryPath, "status", "--porcelain=v1", "-z", "--untracked-files=all"),
		Index:        runGit(testInstance, repositoryPath, "ls-files", "--stage", "-z"),
		CachedDiff:   runGit(testInstance, repositoryPath, "diff", "--cached", "--binary"),
		WorktreeDiff: runGit(testInstance, repositoryPath, "diff", "--binary"),
		Stashes:      runGit(testInstance, repositoryPath, "stash", "list", "--format=%H %s"),
		Worktrees:    runGit(testInstance, repositoryPath, "worktree", "list", "--porcelain"),
		Files:        snapshotSyncFiles(testInstance, repositoryPath),
	}
}

func snapshotSyncFiles(testInstance *testing.T, repositoryPath string) []string {
	testInstance.Helper()
	files := make([]string, 0)
	walkErr := filepath.WalkDir(repositoryPath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, relativeErr := filepath.Rel(repositoryPath, path)
		if relativeErr != nil {
			return relativeErr
		}
		if relativePath == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, targetErr := os.Readlink(path)
			if targetErr != nil {
				return targetErr
			}
			files = append(files, fmt.Sprintf("%s symlink %s", relativePath, target))
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		files = append(files, fmt.Sprintf("%s %o %x", relativePath, info.Mode().Perm(), sha256.Sum256(contents)))
		return nil
	})
	require.NoError(testInstance, walkErr)
	sort.Strings(files)
	return files
}

func captureSyncAdministrativeState(testInstance *testing.T, repositoryPath string, administrativeName string) []string {
	testInstance.Helper()
	administrativePath := resolveSyncAdministrativePath(testInstance, repositoryPath, administrativeName)
	entries := make([]string, 0)
	walkErr := filepath.WalkDir(administrativePath, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, relativeErr := filepath.Rel(administrativePath, path)
		if relativeErr != nil {
			return relativeErr
		}
		if entry.IsDir() {
			entries = append(entries, relativePath+"/")
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		entries = append(entries, fmt.Sprintf("%s:%x", relativePath, sha256.Sum256(contents)))
		return nil
	})
	require.NoError(testInstance, walkErr)
	sort.Strings(entries)
	return entries
}

func resolveSyncAdministrativePath(testInstance *testing.T, repositoryPath string, administrativeName string) string {
	testInstance.Helper()
	resolvedPath := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "--git-path", administrativeName))
	require.NotEmpty(testInstance, resolvedPath)
	if filepath.IsAbs(resolvedPath) {
		return filepath.Clean(resolvedPath)
	}
	return filepath.Join(repositoryPath, resolvedPath)
}
