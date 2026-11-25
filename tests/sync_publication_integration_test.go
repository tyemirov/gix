package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	syncRejectDefaultPushVariable = "GIX_SYNC_TEST_REJECT_DEFAULT_PUSH"
)

func TestSyncPublicationExecutionFailurePreservesWork(testInstance *testing.T) {
	testInstance.Parallel()

	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	for _, phase := range []string{"push", "pull_request"} {
		testInstance.Run(phase, func(testInstance *testing.T) {
			fixture := newSyncFixture(testInstance, "qqq")
			originalHead := strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD"))
			require.NoError(testInstance, os.WriteFile(filepath.Join(fixture.repository, "README.md"), []byte("pending work\n"), 0o644))

			if phase == "push" {
				fixture.environment[syncMergedBranchFailGitMatchVariable] = "push -u origin gix/"
				fixture.environment[syncMergedBranchFailGitOccurrenceVariable] = "1"
				fixture.environment[syncMergedBranchFailGitStateVariable] = filepath.Join(fixture.workspace, "push-failure")
			} else {
				fixture.environment[syncMergedBranchFailPullRequestHeadVariable] = "gix/preserve-file-work"
			}
			output, runError := fixture.run(testInstance, binaryPath, "sync")
			require.Error(testInstance, runError, output)
			require.NotContains(testInstance, output, "SYNCED:")
			require.Equal(testInstance, originalHead, strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "qqq")))
			require.Equal(testInstance, originalHead, strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", "qqq")))
			require.Equal(testInstance, "pending work\n", readTextFile(testInstance, filepath.Join(fixture.repository, "README.md")))
			currentBranch := strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current"))
			if phase == "push" {
				require.Contains(testInstance, output, "SYNC_SWITCH_ROLLBACK")
				require.Equal(testInstance, "qqq", currentBranch)
				require.Equal(testInstance, "M README.md", strings.TrimSpace(runGit(testInstance, fixture.repository, "status", "--porcelain")))
				require.Equal(testInstance, "qqq", strings.TrimSpace(runGit(testInstance, fixture.repository, "for-each-ref", "--format=%(refname:short)", "refs/heads/")))
			} else {
				require.Contains(testInstance, output, "SYNC_SWITCH_HANDOFF")
				require.NotContains(testInstance, output, "SYNC_SWITCH_ROLLBACK")
				require.NotEqual(testInstance, "qqq", currentBranch)
				require.Equal(testInstance, strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD")), strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", currentBranch)))
				delete(fixture.environment, syncMergedBranchFailPullRequestHeadVariable)
				output, runError = fixture.run(testInstance, binaryPath, "sync")
				require.NoError(testInstance, runError, output)
				require.Contains(testInstance, readTextFile(testInstance, fixture.githubLog), "created-pr --base qqq --head "+currentBranch)
			}
		})
	}
}

func TestSyncPullRequestCreateRecovery(t *testing.T) {
	t.Parallel()

	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	const baseBranch = "qqq"
	const reviewBranch = "review/recover"
	const matchingPullRequest = `{"number":10,"headRefName":"review/recover","baseRefName":"qqq","headRepository":{"nameWithOwner":"owner/project"},"isCrossRepository":false}`
	const renamedPullRequest = `{"number":10,"headRefName":"review/recover","baseRefName":"qqq","headRepository":{"nameWithOwner":"owner/renamed-project"},"isCrossRepository":false}`
	const sameRepositoryWithoutHeadMetadata = `{"number":10,"headRefName":"review/recover","baseRefName":"qqq","headRepository":null,"isCrossRepository":false}`
	const incompletePullRequest = `{"number":14,"headRefName":"review/recover","baseRefName":"qqq","headRepository":null,"isCrossRepository":null}`
	const unrelatedPullRequests = `[
		{"number":11,"headRefName":"review/recover","baseRefName":"qqq","headRepository":{"nameWithOwner":"fork/project"},"isCrossRepository":true},
		{"number":12,"headRefName":"review/Recover","baseRefName":"qqq","headRepository":{"nameWithOwner":"owner/project"},"isCrossRepository":false},
		{"number":13,"headRefName":"review/recover","baseRefName":"other-base","headRepository":{"nameWithOwner":"owner/project"},"isCrossRepository":false},
		{"number":15,"headRefName":"review/recover","baseRefName":"qqq","headRepository":null,"isCrossRepository":true},
		{"number":16,"headRefName":"different-head","baseRefName":"qqq","headRepository":null}
	]`

	for _, scenario := range []struct {
		name             string
		firstOutcome     string
		secondOutcome    string
		readback         string
		expectedAttempts int
		expectedReads    int
		expectedError    string
		confirmed        bool
		stash            bool
	}{
		{name: "retry_after_absence", firstOutcome: "internal_error", expectedAttempts: 2, expectedReads: 1},
		{name: "preflight_internal_error", firstOutcome: "preflight_internal_error", expectedAttempts: 2, expectedReads: 1},
		{name: "first_attempt_created", firstOutcome: "created_with_internal_error", expectedAttempts: 1, expectedReads: 1, confirmed: true, stash: true},
		{name: "renamed_repository", firstOutcome: "created_with_internal_error", readback: "[" + renamedPullRequest + "]", expectedAttempts: 1, expectedReads: 1, confirmed: true},
		{name: "exact_match_after_incomplete", firstOutcome: "created_with_internal_error", readback: "[" + incompletePullRequest + "," + matchingPullRequest + "]", expectedAttempts: 1, expectedReads: 1, confirmed: true},
		{name: "same_repository_without_head_metadata", firstOutcome: "created_with_internal_error", readback: "[" + sameRepositoryWithoutHeadMetadata + "]", expectedAttempts: 1, expectedReads: 1, confirmed: true},
		{name: "retry_created", firstOutcome: "internal_error", secondOutcome: "created_with_internal_error", expectedAttempts: 2, expectedReads: 2, confirmed: true},
		{name: "concurrent_create", firstOutcome: "internal_error", secondOutcome: "concurrent_duplicate", expectedAttempts: 2, expectedReads: 2, confirmed: true},
		{name: "ignore_unrelated_pull_requests", firstOutcome: "internal_error", readback: unrelatedPullRequests, expectedAttempts: 2, expectedReads: 1},
		{name: "retry_exhausted", firstOutcome: "internal_error", secondOutcome: "internal_error", expectedAttempts: 2, expectedReads: 2, expectedError: "TEST-REQUEST-2"},
		{name: "lookup_failed", firstOutcome: "internal_error", readback: "error", expectedAttempts: 1, expectedReads: 1, expectedError: "simulated pull request lookup failure"},
		{name: "lookup_returned_null", firstOutcome: "internal_error", readback: "null", expectedAttempts: 1, expectedReads: 1, expectedError: "pull request list is null"},
		{name: "lookup_incomplete_identity", firstOutcome: "internal_error", readback: "[" + incompletePullRequest + "]", expectedAttempts: 1, expectedReads: 1, expectedError: "incomplete pull request identity"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			fixture := newSyncFixture(t, baseBranch)
			runGit(t, fixture.repository, "switch", "-c", reviewBranch)
			fixture.commitFile(t, "review.txt", "published review work\n")
			reviewTip := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
			runGit(t, fixture.repository, "push", "-u", "origin", reviewBranch)
			runGit(t, fixture.repository, "switch", baseBranch)
			fixture.environment["GIX_SYNC_TEST_FIRST_PR_OUTCOME"] = scenario.firstOutcome
			fixture.environment["GIX_SYNC_TEST_SECOND_PR_OUTCOME"] = scenario.secondOutcome
			fixture.environment["GIX_SYNC_TEST_PR_READBACK"] = scenario.readback

			arguments := []string{"--log-level", "info", "sync", reviewBranch, "--title", "Review work", "--body", "Published files."}
			if scenario.stash {
				writeFile(t, filepath.Join(fixture.repository, "pending.txt"), "staged work\n")
				runGit(t, fixture.repository, "add", "pending.txt")
				writeFile(t, filepath.Join(fixture.repository, "pending.txt"), "unstaged work\n")
				arguments = append(arguments, "--stash")
			}
			output, runError := fixture.run(t, binary, arguments...)
			log := readTextFile(t, fixture.githubLog)
			var createCommands []string
			var readbackCommands []string
			const readbackCommand = "pr list --repo owner/project --state open --base " + baseBranch + " --head " + reviewBranch + " "
			for _, line := range strings.Split(log, "\n") {
				if strings.HasPrefix(line, "pr create ") {
					createCommands = append(createCommands, line)
				} else if len(createCommands) > 0 && strings.HasPrefix(line, readbackCommand) {
					readbackCommands = append(readbackCommands, line)
				}
			}
			require.Len(t, createCommands, scenario.expectedAttempts, output)
			require.Len(t, readbackCommands, scenario.expectedReads, log)
			for _, command := range readbackCommands {
				require.Contains(t, command, "isCrossRepository")
			}
			if len(createCommands) == 2 {
				require.Equal(t, createCommands[0], createCommands[1])
			}
			require.Equal(t, reviewTip, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", reviewBranch)))
			require.Equal(t, reviewTip, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", reviewBranch)))
			require.Empty(t, runGit(t, fixture.repository, "stash", "list"))

			if scenario.expectedError != "" {
				require.Error(t, runError, output)
				require.Contains(t, output, "TEST-REQUEST-1")
				require.Contains(t, output, scenario.expectedError)
				require.Contains(t, output, "SYNC_SWITCH_ROLLBACK")
				require.NotContains(t, output, "SYNCED:")
				require.Equal(t, baseBranch, strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
				require.NotContains(t, log, "created-pr ")
				return
			}

			require.NoError(t, runError, output)
			require.NotContains(t, output, "SYNC_SWITCH_ROLLBACK")
			require.Contains(t, output, "SYNCED:")
			require.Equal(t, reviewBranch, strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
			require.Equal(t, 1, strings.Count(log, "created-pr "))
			if scenario.confirmed {
				require.Contains(t, output, "confirmed open pull request #10")
				require.NotContains(t, output, "(pull request created)")
			} else {
				require.Contains(t, output, "(pull request created)")
			}
			if scenario.stash {
				require.Equal(t, "staged work\n", runGit(t, fixture.repository, "show", ":pending.txt"))
				require.Equal(t, "unstaged work\n", readTextFile(t, filepath.Join(fixture.repository, "pending.txt")))
			}
		})
	}
}

func TestSyncExplicitDefaultPushRejectionPreservesCommittedWork(testInstance *testing.T) {
	testInstance.Parallel()

	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	const branch = "qqq"
	fixture := newSyncFixture(testInstance, branch)
	originalHead := strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD"))
	fixture.environment[syncRejectDefaultPushVariable] = "true"
	writeFile(testInstance, filepath.Join(fixture.repository, "README.md"), "staged\n")
	runGit(testInstance, fixture.repository, "add", "README.md")
	writeFile(testInstance, filepath.Join(fixture.repository, "README.md"), "complete pending work\n")
	writeFile(testInstance, filepath.Join(fixture.repository, "new.txt"), "untracked pending work\n")
	output, runError := fixture.run(testInstance, binaryPath, "sync", branch)
	require.Error(testInstance, runError, output)
	require.NotContains(testInstance, output, "SYNCED:")
	require.NotContains(testInstance, output, "SYNCED:")
	require.Contains(testInstance, output, "GH006: Protected branch update failed")
	require.Contains(testInstance, output, "remove branch protection")
	require.Contains(testInstance, output, "open a new pull request")
	require.NotContains(testInstance, output, "SYNC_SWITCH_ROLLBACK")
	require.Equal(testInstance, branch, strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current")))
	require.Equal(testInstance, branch, strings.TrimSpace(runGit(testInstance, fixture.repository, "for-each-ref", "--format=%(refname:short)", "refs/heads/")))
	head := strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD"))
	require.NotEqual(testInstance, originalHead, head)
	runGit(testInstance, fixture.repository, "merge-base", "--is-ancestor", originalHead, head)
	require.Equal(testInstance, originalHead, strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", "HEAD")))
	require.Equal(testInstance, "complete pending work\n", runGit(testInstance, fixture.repository, "show", "HEAD:README.md"))
	require.Equal(testInstance, "untracked pending work\n", runGit(testInstance, fixture.repository, "show", "HEAD:new.txt"))
	require.Empty(testInstance, runGit(testInstance, fixture.repository, "status", "--porcelain"))
	require.Empty(testInstance, runGit(testInstance, fixture.repository, "stash", "list"))
	require.Contains(testInstance, readTextFile(testInstance, fixture.gitLog), "push origin "+branch+" ")
	require.NotContains(testInstance, readTextFile(testInstance, fixture.githubLog), "pr create ")

	delete(fixture.environment, syncRejectDefaultPushVariable)
	output, runError = fixture.run(testInstance, binaryPath, "sync", branch)
	require.NoError(testInstance, runError, output)
	require.Equal(testInstance, head, strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", branch)))
	require.Equal(testInstance, head, strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD")))
}

func TestSyncExplicitDefaultWithRealRemoteRejection(t *testing.T) {
	t.Parallel()

	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	const branch = "qqq"
	fixture := newSyncFixture(t, branch)
	original := strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", branch))
	upstream := filepath.Join(fixture.workspace, "upstream")
	runGitWithDir(t, "", "clone", fixture.remote, upstream)
	configureGitIdentity(t, upstream)
	writeFile(t, filepath.Join(upstream, "remote.txt"), "latest remote data\n")
	runGit(t, upstream, "add", "remote.txt")
	runGit(t, upstream, "commit", "-m", "advance remote")
	runGit(t, upstream, "push", "origin", branch)
	remoteHead := strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", branch))
	runGit(t, fixture.remote, "config", "receive.hideRefs", "refs/heads/"+branch)
	writeFile(t, filepath.Join(fixture.repository, "README.md"), "committed despite remote refusal\n")
	output, err := fixture.run(t, binary, "sync", branch)
	require.Error(t, err, output)
	require.NotContains(t, output, "SYNCED:")
	require.Contains(t, output, "SYNC_PUSH_REJECTED")
	require.Contains(t, output, "deny updating a hidden ref")
	require.Contains(t, output, "remote rejected the push and did not receive the changes")
	require.NotContains(t, output, "SYNC_SWITCH_ROLLBACK")
	require.Equal(t, branch, strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
	head := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
	require.NotEqual(t, original, head)
	runGit(t, fixture.repository, "merge-base", "--is-ancestor", remoteHead, head)
	require.Equal(t, "latest remote data\n", runGit(t, fixture.repository, "show", "HEAD:remote.txt"))
	require.Equal(t, "committed despite remote refusal\n", runGit(t, fixture.repository, "show", "HEAD:README.md"))
	require.Equal(t, remoteHead, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", branch)))
	require.Empty(t, runGit(t, fixture.repository, "status", "--porcelain"))
	require.Empty(t, runGit(t, fixture.repository, "stash", "list"))
	// A rejected retry retains the same unpublished commit.
	output, err = fixture.run(t, binary, "sync", branch)
	require.Error(t, err, output)
	require.NotContains(t, output, "SYNCED:")
	require.Contains(t, output, "SYNC_PUSH_REJECTED")
	require.Equal(t, head, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD")))
	runGit(t, fixture.remote, "config", "--unset", "receive.hideRefs")
	output, err = fixture.run(t, binary, "sync", branch)
	require.NoError(t, err, output)
	require.Equal(t, head, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", branch)))
	require.Equal(t, "committed despite remote refusal\n", runGit(t, fixture.remote, "show", branch+":README.md"))
}

func TestSyncExplicitNondefaultWithRemoteRejection(t *testing.T) {
	t.Parallel()

	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	const branch = "work"
	fixture := newSyncFixture(t, "trunk")
	initial := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
	runGit(t, fixture.repository, "switch", "-c", branch)
	runGit(t, fixture.repository, "push", "-u", "origin", branch)
	runGit(t, fixture.remote, "config", "receive.hideRefs", "refs/heads/"+branch)
	writeFile(t, filepath.Join(fixture.repository, "README.md"), "pending work\n")
	output, err := fixture.run(t, binary, "sync", branch)
	require.Error(t, err, output)
	require.NotContains(t, output, "SYNCED:")
	require.Contains(t, output, "SYNC_PUSH_REJECTED")
	require.Contains(t, output, "remove branch protection")
	require.Contains(t, output, "open a new pull request")
	require.NotContains(t, output, "SYNC_SWITCH_ROLLBACK")
	require.Equal(t, branch, strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
	require.Equal(t, "pending work\n", runGit(t, fixture.repository, "show", "HEAD:README.md"))
	require.Equal(t, initial, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", branch)))
	require.Equal(t, initial, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "trunk")))
	require.NotContains(t, readTextFile(t, fixture.githubLog), "pr create ")
	require.Empty(t, runGit(t, fixture.repository, "status", "--porcelain"))
	runGit(t, fixture.remote, "config", "--unset", "receive.hideRefs")
	output, err = fixture.run(t, binary, "sync", branch)
	require.NoError(t, err, output)
	require.Equal(t, "pending work\n", runGit(t, fixture.remote, "show", branch+":README.md"))
	require.Equal(t, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD")), strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", branch)))
}

func TestSyncExplicitDefaultWithConcurrentRemotePush(t *testing.T) {
	t.Parallel()

	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	fixture := newSyncFixture(t, "qqq")
	upstream := filepath.Join(fixture.workspace, "upstream")
	runGitWithDir(t, "", "clone", fixture.remote, upstream)
	configureGitIdentity(t, upstream)
	writeFile(t, filepath.Join(upstream, "concurrent.txt"), "concurrent remote work\n")
	writeFile(t, filepath.Join(fixture.repository, "README.md"), "pending work\n")
	fixture.environment["GIX_SYNC_TEST_ADVANCE_REMOTE_CHECKOUT"] = upstream
	output, err := fixture.run(t, binary, "sync", "qqq")
	require.Error(t, err, output)
	require.NotContains(t, output, "SYNCED:")
	require.Contains(t, output, "SYNC_PUSH_REJECTED")
	require.Contains(t, output, "[rejected]")
	require.NotContains(t, output, "SYNC_SWITCH_ROLLBACK")
	require.Equal(t, "qqq", strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
	require.Equal(t, "pending work\n", runGit(t, fixture.repository, "show", "HEAD:README.md"))
	require.Equal(t, "initial\n", runGit(t, fixture.remote, "show", "qqq:README.md"))
	head := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
	remoteHead := strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "qqq"))
	require.NotEqual(t, head, remoteHead)
	require.Empty(t, runGit(t, fixture.repository, "status", "--porcelain"))
	delete(fixture.environment, "GIX_SYNC_TEST_ADVANCE_REMOTE_CHECKOUT")
	output, err = fixture.run(t, binary, "sync", "qqq")
	require.NoError(t, err, output)
	runGit(t, fixture.repository, "merge-base", "--is-ancestor", head, "HEAD")
	runGit(t, fixture.repository, "merge-base", "--is-ancestor", remoteHead, "HEAD")
	require.Equal(t, "pending work\n", runGit(t, fixture.remote, "show", "qqq:README.md"))
	require.Equal(t, "concurrent remote work\n", runGit(t, fixture.remote, "show", "qqq:concurrent.txt"))
	require.Equal(t, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD")), strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "qqq")))
}

func TestSyncExplicitDefaultRejectionFromAnotherBranch(t *testing.T) {
	t.Parallel()

	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, source := range []string{"main", "master", "feature/source"} {
		t.Run(source, func(t *testing.T) {
			fixture := newSyncFixture(t, "qqq")
			remoteHead := strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "qqq"))
			runGit(t, fixture.repository, "switch", "-c", source)
			fixture.commitFile(t, "source-only.txt", "source work\n")
			sourceHead := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
			writeFile(t, filepath.Join(fixture.repository, "README.md"), "pending work\n")
			fixture.environment[syncRejectDefaultPushVariable] = "true"
			output, err := fixture.run(t, binary, "sync", "qqq")
			require.Error(t, err, output)
			require.NotContains(t, output, "SYNCED:")
			require.Contains(t, output, "SYNC_PUSH_REJECTED")
			require.NotContains(t, output, "SYNC_SWITCH_ROLLBACK")
			require.Equal(t, "qqq", strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
			require.Equal(t, sourceHead, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", source)))
			require.Equal(t, remoteHead, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "qqq")))
			require.NoFileExists(t, filepath.Join(fixture.repository, "source-only.txt"))
			require.Equal(t, "pending work\n", runGit(t, fixture.repository, "show", "HEAD:README.md"))
			require.Empty(t, runGit(t, fixture.repository, "status", "--porcelain"))
			require.Empty(t, runGit(t, fixture.repository, "stash", "list"))
			require.NotContains(t, readTextFile(t, fixture.githubLog), "pr create ")
		})
	}
}

func TestSyncDefaultPublicationReviewRegressions(t *testing.T) {
	t.Parallel()

	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, narrow := range []bool{false, true} {
		t.Run(fmt.Sprintf("new_branch_preserves_prior_review_narrow_%t", narrow), func(t *testing.T) {
			t.Parallel()
			fixture := newSyncFixture(t, "qqq")
			fixture.commitFile(t, "local.txt", "local work\n")
			writeFile(t, filepath.Join(fixture.repository, "README.md"), "pending work\n")
			output, err := fixture.run(t, binary, "sync")
			require.NoError(t, err, output)
			review := strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current"))
			head := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
			runGit(t, fixture.repository, "switch", "qqq")
			if narrow {
				runGit(t, fixture.repository, "config", "remote.origin.fetch", "+refs/heads/qqq:refs/remotes/origin/qqq")
			}
			output, err = fixture.run(t, binary, "sync")
			require.NoError(t, err, output)
			current := strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current"))
			require.NotEqual(t, review, current)
			require.NotEqual(t, "qqq", current)
			require.Equal(t, head, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", review)))
			require.Equal(t, "pending work\n", runGit(t, fixture.remote, "show", review+":README.md"))
			require.Equal(t, "initial\n", runGit(t, fixture.remote, "show", current+":README.md"))
			require.Equal(t, "local work\n", runGit(t, fixture.remote, "show", current+":local.txt"))
			require.Equal(t, 2, strings.Count(readTextFile(t, fixture.githubLog), "created-pr --base qqq --head "))
		})
	}
	for _, explicit := range []bool{false, true} {
		for _, newer := range []bool{false, true} {
			for _, pruned := range []bool{false, true} {
				t.Run(fmt.Sprintf("squash_explicit_%t_newer_%t_pruned_%t", explicit, newer, pruned), func(t *testing.T) {
					t.Parallel()
					fixture := newSyncFixture(t, "qqq")
					fixture.commitFile(t, "local.txt", "local work\n")
					if explicit {
						require.NoError(t, os.WriteFile(filepath.Join(fixture.repository, "README.md"), []byte("pending work\n"), 0o644))
					}

					output, err := fixture.run(t, binary, "sync")
					require.NoError(t, err, output)
					review := strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current"))
					head := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
					upstream := filepath.Join(fixture.workspace, "merger")
					runGitWithDir(t, "", "clone", fixture.remote, upstream)
					configureGitIdentity(t, upstream)
					runGit(t, upstream, "merge", "--squash", "origin/"+review)
					runGit(t, upstream, "commit", "-m", "squash reviewed work")
					require.NoError(t, os.WriteFile(filepath.Join(upstream, "remote.txt"), []byte("later remote work\n"), 0o644))
					runGit(t, upstream, "add", "remote.txt")
					runGit(t, upstream, "commit", "-m", "later remote work")
					runGit(t, upstream, "push", "origin", "qqq")
					runGit(t, fixture.remote, "update-ref", "refs/pull/9/head", head)
					if pruned {
						runGit(t, upstream, "push", "origin", "--delete", review)
					}
					log := readTextFile(t, fixture.githubLog) + fmt.Sprintf("merged-pr --base qqq --head %s --oid %s\n", review, head)
					require.NoError(t, os.WriteFile(fixture.githubLog, []byte(log), 0o600))
					if newer {
						runGit(t, fixture.repository, "switch", "qqq")
						fixture.commitFile(t, "newer.txt", "unreviewed local work\n")
						runGit(t, fixture.repository, "switch", review)
					}
					arguments := []string{"sync"}
					if explicit {
						arguments = append(arguments, "qqq")
					}
					arguments = append(arguments, "-y")
					output, err = fixture.run(t, binary, arguments...)
					require.NoError(t, err, output)
					require.Contains(t, output, "SYNCED:")
					current := strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current"))
					t.Cleanup(func() {
						if t.Failed() {
							t.Log(output, readTextFile(t, fixture.githubLog), readTextFile(t, fixture.gitLog))
						}
					})
					require.Equal(t, "qqq", current)
					require.Equal(t, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "qqq")), strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD")))
					require.Equal(t, 1, strings.Count(readTextFile(t, fixture.githubLog), "created-pr --base qqq --head "))
					if newer {
						require.Equal(t, "unreviewed local work\n", readTextFile(t, filepath.Join(fixture.repository, "newer.txt")))
					}
					require.Equal(t, "local work\n", readTextFile(t, filepath.Join(fixture.repository, "local.txt")))
					require.Equal(t, "later remote work\n", readTextFile(t, filepath.Join(fixture.repository, "remote.txt")))
				})
			}
		}
	}
	t.Run("behind_ff_false", func(t *testing.T) {
		t.Parallel()
		fixture := newSyncFixture(t, "default")
		upstream := filepath.Join(fixture.workspace, "merger")
		runGitWithDir(t, "", "clone", fixture.remote, upstream)
		configureGitIdentity(t, upstream)
		runGit(t, upstream, "commit", "--allow-empty", "-m", "remote work")
		runGit(t, upstream, "push", "origin", "default")
		runGit(t, fixture.repository, "config", "merge.ff", "false")

		output, err := fixture.run(t, binary, "sync", "default")
		require.NoError(t, err, output)
		require.Equal(t, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "default")), strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD")))
		require.NotContains(t, readTextFile(t, fixture.githubLog), "pr create ")
	})
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprintf("selected_remote_explicit_%t", explicit), func(t *testing.T) {
			t.Parallel()
			fixture := newSyncFixture(t, "qqq")
			upstream := filepath.Join(fixture.workspace, "upstream.git")
			runGitWithDir(t, "", "clone", "--bare", fixture.remote, upstream)
			runGit(t, fixture.repository, "remote", "add", "upstream", upstream)
			originHead := strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "qqq"))
			fixture.commitFile(t, "local.txt", "local work\n")

			arguments := []string{"sync", "--remote", "upstream"}
			if explicit {
				arguments = append(arguments, "qqq")
			}
			output, err := fixture.run(t, binary, arguments...)
			require.NoError(t, err, output)
			current := strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current"))
			log := readTextFile(t, fixture.githubLog)
			require.NotContains(t, log, "api repos/upstream/project/branches/qqq ")
			require.NotContains(t, log, "api repos/owner/project/branches/qqq ")
			if !explicit {
				require.NotEqual(t, "qqq", current)
				require.Contains(t, log, "pr create --repo upstream/project ")
				require.NotContains(t, log, "pr create --repo owner/project ")
				runGit(t, fixture.repository, "switch", "qqq")
				output, err = fixture.run(t, binary, "sync", "--remote", "upstream")
				require.NoError(t, err, output)
				require.NotEqual(t, current, strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
				require.Equal(t, 2, strings.Count(readTextFile(t, fixture.githubLog), "created-pr --base qqq --head "))
			} else {
				require.Equal(t, "qqq", current)
				require.NotContains(t, log, "pr create ")
			}
			require.Equal(t, originHead, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "qqq")))
			require.Equal(t, strings.TrimSpace(runGit(t, upstream, "rev-parse", current)), strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD")))
		})
	}
}

func TestSyncRejectedPushRestoresInvocationStash(t *testing.T) {
	t.Parallel()
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, stacked := range []bool{false, true} {
		t.Run(fmt.Sprintf("parent_%t", stacked), func(t *testing.T) {
			fixture := newSyncFixture(t, "qqq")
			branch := "work"
			if stacked {
				branch = "parent"
			}
			runGit(t, fixture.repository, "switch", "-c", branch)
			runGit(t, fixture.repository, "push", "-u", "origin", branch)
			remoteHead := strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", branch))
			writeFile(t, filepath.Join(fixture.repository, "saved.txt"), "existing stash bytes\n")
			runGit(t, fixture.repository, "stash", "push", "-u", "-m", "existing operator stash")
			existingStash := runGit(t, fixture.repository, "stash", "list", "--format=%H %s")
			fixture.commitFile(t, "local.txt", "unpublished local commit\n")
			localHead := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
			writeFile(t, filepath.Join(fixture.repository, "README.md"), "staged bytes\n")
			runGit(t, fixture.repository, "add", "README.md")
			writeFile(t, filepath.Join(fixture.repository, "README.md"), "unstaged bytes\n")
			writeFile(t, filepath.Join(fixture.repository, "untracked.txt"), "untracked bytes\n")
			before := runGit(t, fixture.repository, "status", "--porcelain=v1", "-z", "--untracked-files=all")
			runGit(t, fixture.remote, "config", "receive.hideRefs", "refs/heads/"+branch)
			target := branch
			if stacked {
				target = "child"
			}
			output, err := fixture.run(t, binary, "sync", target, "--stash=yes")
			require.Error(t, err, output)
			require.Contains(t, output, "SYNC_PUSH_REJECTED")
			require.NotContains(t, output, "SYNCED:")
			require.NotContains(t, output, "SYNC_SWITCH_ROLLBACK")
			require.Equal(t, target, strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
			require.Equal(t, localHead, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD")))
			require.Equal(t, remoteHead, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", branch)))
			require.Equal(t, "unpublished local commit\n", runGit(t, fixture.repository, "show", "HEAD:local.txt"))
			require.Equal(t, "staged bytes\n", runGit(t, fixture.repository, "show", ":README.md"))
			require.Equal(t, "unstaged bytes\n", readTextFile(t, filepath.Join(fixture.repository, "README.md")))
			require.Equal(t, "untracked bytes\n", readTextFile(t, filepath.Join(fixture.repository, "untracked.txt")))
			require.Equal(t, before, runGit(t, fixture.repository, "status", "--porcelain=v1", "-z", "--untracked-files=all"))
			require.Equal(t, existingStash, runGit(t, fixture.repository, "stash", "list", "--format=%H %s"))
			require.NotContains(t, readTextFile(t, fixture.githubLog), "pr create ")
			if stacked {
				require.Empty(t, runGit(t, fixture.remote, "for-each-ref", "--format=%(refname)", "refs/heads/child"))
			}
			runGit(t, fixture.remote, "config", "--unset", "receive.hideRefs")
			output, err = fixture.run(t, binary, "sync", target, "--stash=yes")
			require.NoError(t, err, output)
			require.Equal(t, localHead, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", target)))
			require.Equal(t, before, runGit(t, fixture.repository, "status", "--porcelain=v1", "-z", "--untracked-files=all"))
			require.Equal(t, existingStash, runGit(t, fixture.repository, "stash", "list", "--format=%H %s"))
		})
	}
}

func TestSyncRejectedPushFinalizationFailurePreservesRecovery(t *testing.T) {
	t.Parallel()
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, failure := range []struct{ name, match, occurrence string }{
		{"invocation_stash_restore", "stash apply --index", "2"},
		{"invocation_stash_drop", "stash drop", "1"},
		{"snapshot_cleanup", "stash drop", "2"},
	} {
		t.Run(failure.name, func(t *testing.T) {
			fixture := newSyncFixture(t, "qqq")
			remoteHead := strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "qqq"))
			fixture.commitFile(t, "local.txt", "preserved local commit\n")
			localHead := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
			writeFile(t, filepath.Join(fixture.repository, "README.md"), "staged recovery bytes\n")
			runGit(t, fixture.repository, "add", "README.md")
			writeFile(t, filepath.Join(fixture.repository, "README.md"), "unstaged recovery bytes\n")
			writeFile(t, filepath.Join(fixture.repository, "untracked.txt"), "untracked recovery bytes\n")
			runGit(t, fixture.remote, "config", "receive.hideRefs", "refs/heads/qqq")
			fixture.environment[syncMergedBranchFailGitMatchVariable] = failure.match
			fixture.environment[syncMergedBranchFailGitOccurrenceVariable] = failure.occurrence
			fixture.environment[syncMergedBranchFailGitStateVariable] = filepath.Join(fixture.workspace, "finalization-failure")
			output, err := fixture.run(t, binary, "sync", "qqq", "--stash=yes")
			require.Error(t, err, output)
			require.Contains(t, output, "SYNC_PUSH_REJECTED")
			require.Contains(t, output, "simulated git failure")
			require.Contains(t, output, "SYNC_SWITCH_HANDOFF")
			require.Contains(t, output, "push of branch \"qqq\" to remote \"origin\" was rejected")
			require.NotContains(t, output, "SYNC_SWITCH_ROLLBACK")
			require.NotContains(t, output, "published remote changes")
			require.NotContains(t, output, "SYNCED:")
			require.Equal(t, "qqq", strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
			require.Equal(t, localHead, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD")))
			require.Equal(t, remoteHead, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "qqq")))
			require.Equal(t, "preserved local commit\n", runGit(t, fixture.repository, "show", "HEAD:local.txt"))
			stashes := strings.Fields(runGit(t, fixture.repository, "stash", "list", "--format=%H"))
			require.NotEmpty(t, stashes)
			for _, stash := range stashes {
				require.Equal(t, "unstaged recovery bytes\n", runGit(t, fixture.repository, "show", stash+":README.md"))
				require.Equal(t, "staged recovery bytes\n", runGit(t, fixture.repository, "show", stash+"^2:README.md"))
				require.Equal(t, "untracked recovery bytes\n", runGit(t, fixture.repository, "show", stash+"^3:untracked.txt"))
			}
		})
	}
}

func TestSyncNewBranchPushRejectionPreservesDestination(t *testing.T) {
	t.Parallel()
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	fixture := newSyncFixture(t, "qqq")
	originalHead := strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "qqq"))
	fixture.commitFile(t, "local.txt", "existing unpublished commit\n")
	existingHead := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
	runGit(t, fixture.remote, "config", "receive.hideRefs", "refs/heads/new-review")
	writeFile(t, filepath.Join(fixture.repository, "README.md"), "saved new branch bytes\n")
	output, err := fixture.run(t, binary, "sync", "new-review")
	require.Error(t, err, output)
	require.Contains(t, output, "SYNC_PUSH_REJECTED")
	require.NotContains(t, output, "SYNCED:")
	require.NotContains(t, output, "SYNC_SWITCH_ROLLBACK")
	require.Equal(t, "new-review", strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
	head := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
	runGit(t, fixture.repository, "merge-base", "--is-ancestor", existingHead, head)
	require.Equal(t, existingHead, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "qqq")))
	require.Equal(t, originalHead, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "qqq")))
	require.Empty(t, runGit(t, fixture.remote, "for-each-ref", "--format=%(refname)", "refs/heads/new-review"))
	require.Equal(t, "saved new branch bytes\n", runGit(t, fixture.repository, "show", "HEAD:README.md"))
	require.Equal(t, "existing unpublished commit\n", runGit(t, fixture.repository, "show", "HEAD:local.txt"))
	require.Empty(t, runGit(t, fixture.repository, "status", "--porcelain"))
	require.Empty(t, runGit(t, fixture.repository, "stash", "list"))
	require.NotContains(t, readTextFile(t, fixture.githubLog), "pr create ")
	runGit(t, fixture.remote, "config", "--unset", "receive.hideRefs")
	output, err = fixture.run(t, binary, "sync")
	require.NoError(t, err, output)
	require.Equal(t, head, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "new-review")))
	require.Equal(t, "saved new branch bytes\n", runGit(t, fixture.remote, "show", "new-review:README.md"))
}
