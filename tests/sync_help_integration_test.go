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
		"A missing explicit branch is created on top of the current branch, then aligned with the remote base before publication.",
	)
}
