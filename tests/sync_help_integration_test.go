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
	require.Contains(testInstance, output, "sync reconstructs untouched bytes locally and directly accepts only cases with no two-sided semantic choice: identical sides and a change on only one side.")
	require.Contains(testInstance, output, "Every marker-bearing region changed by both sides requires semantic LLM audit.")
	require.Contains(testInstance, output, "A text conflict without markers requires an explicit OURS or THEIRS file selection.")
	require.Contains(testInstance, output, "Related issue insertions use source selection keys.")
	require.Contains(testInstance, output, "Validation requires an exact source alternative, including punctuation and whitespace.")
	require.Contains(testInstance, output, "Other concurrent insertions and compatible token edits start from lossless locally derived candidates.")
	require.Contains(testInstance, output, "Conflicting replacements start from the local alternative plus each compatible incoming edit.")
	require.Contains(testInstance, output, "Sync sends every derived candidate directly to semantic audit.")
	require.Contains(testInstance, output, "Candidate generation remains only when local token analysis cannot derive a valid candidate.")
	require.Contains(testInstance, output, "A provider round with no response stops semantic repair and starts rollback.")
	require.Contains(testInstance, output, "A locally valid audit correction completes immediately.")
	require.Contains(testInstance, output, "When exact replacement-intent proof is unavailable for a structurally valid correction, sync retains that exact correction for repair in the next semantic audit.")
	require.Contains(testInstance, output, "An approval cannot accept that candidate. Only a later locally valid correction completes it.")
	require.Contains(testInstance, output, "Responses that fail hard validation supply feedback for the next bounded attempt.")
	require.Contains(testInstance, output, "Rollback occurs only after every safe candidate is exhausted, or after provider failure, cancellation, or an unrecoverable local failure, and always stops before push.")
}
