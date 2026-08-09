package branches

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/workflow"
)

func TestWriteBranchCleanupFailureDetailsRedactsHTTPSCredentials(testInstance *testing.T) {
	stdoutBuffer := &bytes.Buffer{}
	stderrBuffer := &bytes.Buffer{}

	environment := &workflow.Environment{
		Output: stdoutBuffer,
		Errors: stderrBuffer,
	}

	writeBranchCleanupFailureDetails(environment, "/tmp/repo", CleanupSummary{
		FailedBranches: 1,
		Failures: []CleanupFailure{
			{
				BranchName:          "feature/failure",
				RemoteDeletionError: "git command exited with code 128: https://token@github.com/org/repo",
			},
		},
	})

	output := stderrBuffer.String()
	require.Contains(testInstance, output, "https://***@github.com")
	require.NotContains(testInstance, output, "https://token@github.com")
	require.Contains(testInstance, output, "branch=feature/failure")
}
