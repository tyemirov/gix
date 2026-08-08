package syncflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/v5/internal/execshell"
)

type cancelDuringDirtyClusterInspectionExecutor struct {
	delegate   *strictSyncGitExecutor
	cancel     context.CancelFunc
	cancelNext bool
}

func (executor *cancelDuringDirtyClusterInspectionExecutor) ExecuteGit(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	if executor.cancelNext {
		executor.cancelNext = false
		executor.cancel()
		if cancellationErr := ctx.Err(); cancellationErr != nil {
			return execshell.ExecutionResult{}, cancellationErr
		}
	}
	return executor.delegate.ExecuteGit(ctx, details)
}

func (executor *cancelDuringDirtyClusterInspectionExecutor) ExecuteGitHubCLI(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return executor.delegate.ExecuteGitHubCLI(ctx, details)
}

func TestDirtyClusterOwnershipInspectionDetachesBeforeCallerCancellation(testInstance *testing.T) {
	const repositoryPath = "/tmp/project"
	delegate := &strictSyncGitExecutor{stagedPaths: []string{"README.md"}}
	expected, captureErr := captureStrictSyncDirtyClusterCheckpoint(context.Background(), delegate, repositoryPath)
	require.NoError(testInstance, captureErr)

	callerContext, cancelCaller := context.WithCancel(context.Background())
	executor := &cancelDuringDirtyClusterInspectionExecutor{
		delegate:   delegate,
		cancel:     cancelCaller,
		cancelNext: true,
	}

	inspectionErr := validateStrictSyncDirtyClusterCheckpoint(callerContext, executor, repositoryPath, "README.md", expected)

	require.NoError(testInstance, inspectionErr)
	require.ErrorIs(testInstance, callerContext.Err(), context.Canceled)
}
