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
	require.Contains(testInstance, output, "Dirty auto-commit is rejected on a known-merged branch; use --stash to preserve that work through the merged handoff before creating a new review branch.")
}
