package syncflow

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
)

type mergeConflictRollbackExecutor struct {
	commands   []execshell.CommandDetails
	contextErr error
	abortErr   error
}

func (executor *mergeConflictRollbackExecutor) ExecuteGit(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	executor.contextErr = ctx.Err()
	return execshell.ExecutionResult{}, executor.abortErr
}

func (executor *mergeConflictRollbackExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func TestRollbackFailedMergeUsesCleanupContextAfterCancellation(t *testing.T) {
	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()

	executor := &mergeConflictRollbackExecutor{}
	var reportedEvent shared.Event
	service := mergeConflictResolutionService{
		executor:       executor,
		repositoryPath: "/tmp/project",
		reporter: func(level shared.EventLevel, code string, message string, details map[string]string) {
			reportedEvent = shared.Event{Level: level, Code: code, Message: message, Details: details}
		},
	}

	rollbackErr := service.rollbackFailedMerge(
		canceledContext,
		errors.New("resolution canceled"),
		"origin/feature/base",
		"feature/target",
	)

	require.NoError(t, rollbackErr)
	require.NoError(t, executor.contextErr)
	require.Len(t, executor.commands, 1)
	require.Equal(t, []string{"merge", "--abort"}, executor.commands[0].Arguments)
	require.Equal(t, shared.EventCodeAIMergeRollback, reportedEvent.Code)
	require.Contains(t, reportedEvent.Message, "failed merge was aborted")
	require.Equal(t, "feature/target", reportedEvent.Details["target_branch"])
}

func TestRollbackFailedMergeReportsResolutionAndRollbackFailures(t *testing.T) {
	executor := &mergeConflictRollbackExecutor{abortErr: errors.New("index is read-only")}
	var reportedEvent shared.Event
	service := mergeConflictResolutionService{
		executor:       executor,
		repositoryPath: "/tmp/project",
		reporter: func(level shared.EventLevel, code string, message string, details map[string]string) {
			reportedEvent = shared.Event{Level: level, Code: code, Message: message, Details: details}
		},
	}

	rollbackErr := service.rollbackFailedMerge(
		context.Background(),
		errors.New("lossy resolution"),
		"origin/feature/base",
		"feature/target",
	)

	require.Error(t, rollbackErr)
	require.Contains(t, rollbackErr.Error(), "automatic merge rollback failed")
	require.Contains(t, rollbackErr.Error(), "index is read-only")
	require.Equal(t, shared.EventCodeAIMergeHandoff, reportedEvent.Code)
	require.Contains(t, reportedEvent.Message, "lossy resolution")
	require.Contains(t, reportedEvent.Message, "index is read-only")
	require.Equal(t, "lossy resolution", reportedEvent.Details["reason"])
	require.Contains(t, reportedEvent.Details["rollback_reason"], "automatic merge rollback failed")
}
