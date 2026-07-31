package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const syncHelpIntegrationTimeout = 20 * time.Second

func TestSyncHelpDescribesMissingBranchCurrentHeadContract(testInstance *testing.T) {
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
		"An explicit branch target is binding: dirty work is committed to that named branch. Explicit gix sync master commits to master, merges origin/master, and pushes master directly.",
	)
	require.Contains(
		testInstance,
		output,
		"A missing explicit branch with dirty work is created on top of the current branch. If the current branch is not master, sync first ensures that its committed HEAD is remote-backed and has an open pull request, then opens the child pull request against that branch.",
	)
	require.Contains(testInstance, output, "The selected parent base is retained for retries after child push or pull-request failure.")
	require.Contains(testInstance, output, "A clean or stashed missing branch is rejected because it would have no child pull request delta.")
	require.Contains(testInstance, output, "Each dirty-cluster request checkpoints the active checkout, HEAD, staged paths, and exact semantic index state, including ownership flags and intent-to-add entries, before waiting for the model.")
	require.Contains(testInstance, output, "After the model returns, Gix holds the exact worktree index lock while rechecking ownership and commits only a private copy of that validated index.")
	require.Contains(testInstance, output, "A concurrent checkout or index change stops before commit and preserves the outside state plus transaction snapshots under SYNC_SWITCH_HANDOFF; stop the other writer before retrying.")
	require.Contains(testInstance, output, "Dirty auto-commit is rejected on a known-merged branch; use --stash to preserve that work through the merged handoff before creating a new review branch.")
	require.Contains(testInstance, output, "Before fetch or content, index, ref, or checkout mutation, strict sync validates live worktree ownership, repairs only missing canonical Git links, then resolves exact per-worktree administrative paths and rejects operator-owned merge, revert, cherry-pick, rebase, apply-mailbox, bisect, sequencer, or unmerged-index state; ordinary refs with administrative names do not count.")
	require.Contains(testInstance, output, "The strict-sync transaction snapshots the caller and target sibling checkout, commit, index, tracked files, untracked files, stashes, and topology, then journals only refs and worktrees it mutates.")
	require.Contains(testInstance, output, "A failure before publication restores that owned state without rewinding unrelated refs; an up-to-date push remains rollback-capable.")
	require.Contains(testInstance, output, "An actual remote ref update or pull-request creation marks publication, after which failure preserves the published recovery state and reports a handoff.")
	require.Contains(testInstance, output, "Invocation-owned stashes are restored with their index and validated before SYNCED is reported.")
	require.Contains(testInstance, output, "sync reconstructs untouched bytes locally and directly accepts only cases with no two-sided semantic choice: identical sides, a change on only one side, and marker-free current-stage decisions.")
	require.Contains(testInstance, output, "Every marker-bearing region changed by both sides requires semantic LLM audit.")
	require.Contains(testInstance, output, "Concurrent insertions and non-overlapping token edits start from lossless locally derived candidates; genuinely overlapping regions use candidate generation plus bounded validation-guided repair.")
	require.Contains(testInstance, output, "rollback occurs only after every safe strategy is exhausted, or after cancellation or an unrecoverable local failure, and always stops before push.")
}
