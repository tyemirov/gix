package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const syncHelpIntegrationTimeout = 20 * time.Second

func TestSyncHelpDescribesBranchSelectionContract(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	output := runIntegrationCommand(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{},
		syncHelpIntegrationTimeout,
		[]string{"run", ".", "sync", "--help"},
	)

	require.Contains(
		testInstance,
		output,
		"When a branch is specified, sync uses that branch. Without a branch argument on the default branch, sync creates a new branch only when there are uncommitted changes or unpublished local commits. Otherwise, sync updates the current branch.",
	)
	require.Contains(
		testInstance,
		output,
		"Branch names and default-branch status add no other special behavior. Sync pulls the latest remote changes, merges pending work into the selected branch, commits the result, and pushes that branch.",
	)
	require.Contains(testInstance, output, "A GitHub push rejection preserves the local commits and selected branch. Sync reports the rejection, suggests removing branch protection or opening a new pull request, and returns success with exit code 0.")
	require.Contains(testInstance, output, "A missing explicit branch starts at the current HEAD.")
	require.NotContains(testInstance, output, "A rejected push fails")
	require.NotContains(testInstance, output, "a dirty current default branch keeps the generated PR rescue flow")
	require.NotContains(testInstance, output, "A clean or stashed missing branch is rejected")
	require.Contains(testInstance, output, "Each dirty-cluster request checkpoints the active checkout, HEAD, staged paths, and exact semantic index state, including ownership flags and intent-to-add entries, before waiting for the model.")
	require.Contains(testInstance, output, "After the model returns, Gix holds the exact worktree index lock while rechecking ownership and commits only a private copy of that validated index.")
	require.Contains(testInstance, output, "A concurrent checkout or index change stops before commit and preserves the outside state plus transaction snapshots under SYNC_SWITCH_HANDOFF; stop the other writer before retrying.")
	require.Contains(testInstance, output, "Dirty auto-commit is rejected on a known-merged branch; use --stash to preserve that work through the merged handoff before creating a new review branch.")
	require.Contains(testInstance, output, "Before fetch or content, index, ref, or checkout mutation, strict sync validates live worktree ownership, repairs only missing canonical Git links, then resolves exact per-worktree administrative paths and rejects operator-owned merge, revert, cherry-pick, rebase, apply-mailbox, bisect, sequencer, or unmerged-index state; ordinary refs with administrative names do not count.")
	require.Contains(testInstance, output, "The strict-sync transaction snapshots the caller and target sibling checkout, commit, index, tracked files, untracked files, stashes, and topology, then journals only refs and worktrees it mutates.")
	require.Contains(testInstance, output, "A local operation failure before publication restores that owned state without rewinding unrelated refs. A GitHub push rejection does not start rollback.")
	require.Contains(testInstance, output, "An actual remote ref update or pull-request creation marks publication, after which failure preserves the published recovery state and reports a handoff.")
	require.Contains(testInstance, output, "Invocation-owned stashes are restored with their index and validated before SYNCED is reported.")
	require.Contains(testInstance, output, "sync constructs unchanged regions and independent line changes locally.")
	require.Contains(testInstance, output, "A text conflict without markers requires an explicit OURS or THEIRS file selection.")
	require.Contains(testInstance, output, "Every two-sided candidate requires a separate semantic audit.")
	require.Contains(testInstance, output, "Each region permits four provider requests across decisions, audits, and context expansion.")
	require.Contains(testInstance, output, "Repeated rejected candidates and provider failures stop semantic repair.")
	require.Contains(testInstance, output, "Complete Go, JSON, and YAML files pass syntax checks before staging.")

}
