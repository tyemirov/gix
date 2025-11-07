package cd

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/temirov/gix/internal/audit"
	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/gitrepo"
	"github.com/temirov/gix/internal/repos/shared"
	"github.com/temirov/gix/internal/workflow"
)

type recordingReporter struct {
	events []shared.Event
}

func (reporter *recordingReporter) Report(event shared.Event) {
	reporter.events = append(reporter.events, event)
}

func (reporter *recordingReporter) Summary() string {
	return ""
}

func (reporter *recordingReporter) SummaryData() shared.SummaryData {
	return shared.SummaryData{}
}

func (reporter *recordingReporter) PrintSummary() {
}

func (reporter *recordingReporter) RecordEvent(string, shared.EventLevel) {
}

func (reporter *recordingReporter) RecordOperationDuration(string, time.Duration) {
}

func TestHandleBranchChangeActionUsesRepositoryDefault(t *testing.T) {
	executor := &stubGitExecutor{
		responses: []stubGitResponse{
			{result: execShellOutput("origin\n")},
		},
	}
	reporter := &recordingReporter{}
	environment := &workflow.Environment{
		GitExecutor: executor,
		Logger:      zap.NewNop(),
		Output:      io.Discard,
		Errors:      io.Discard,
		Reporter:    reporter,
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			RemoteDefaultBranch: "main",
		},
	}

	parameters := map[string]any{
		taskOptionBranchRemote: shared.OriginRemoteNameConstant,
	}

	require.NoError(t, handleBranchChangeAction(context.Background(), environment, repository, parameters))
	require.GreaterOrEqual(t, len(executor.recorded), 3)
	require.Equal(t, []string{"switch", "main"}, executor.recorded[2].Arguments)
	require.Len(t, reporter.events, 1)
	require.Equal(t, branchResolutionSourceRemoteDefault, reporter.events[0].Details["source"])
}

func TestHandleBranchChangeActionUsesConfiguredFallback(t *testing.T) {
	executor := &stubGitExecutor{
		responses: []stubGitResponse{
			{result: execShellOutput("origin\n")},
		},
	}
	reporter := &recordingReporter{}
	environment := &workflow.Environment{
		GitExecutor: executor,
		Logger:      zap.NewNop(),
		Output:      io.Discard,
		Errors:      io.Discard,
		Reporter:    reporter,
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			RemoteDefaultBranch: "",
		},
	}

	parameters := map[string]any{
		taskOptionBranchRemote:            shared.OriginRemoteNameConstant,
		taskOptionConfiguredDefaultBranch: "develop",
	}

	require.NoError(t, handleBranchChangeAction(context.Background(), environment, repository, parameters))
	require.GreaterOrEqual(t, len(executor.recorded), 3)
	require.Equal(t, []string{"switch", "develop"}, executor.recorded[2].Arguments)
	require.Len(t, reporter.events, 1)
	require.Equal(t, branchResolutionSourceConfigured, reporter.events[0].Details["source"])
}

func TestHandleBranchChangeActionErrorsWhenBranchCannotBeResolved(t *testing.T) {
	executor := &stubGitExecutor{
		responses: []stubGitResponse{
			{result: execShellOutput("origin\n")},
		},
	}
	environment := &workflow.Environment{
		GitExecutor: executor,
		Logger:      zap.NewNop(),
		Output:      io.Discard,
		Errors:      io.Discard,
	}
	repository := &workflow.RepositoryState{
		Path:       "/tmp/project",
		Inspection: audit.RepositoryInspection{},
	}

	parameters := map[string]any{
		taskOptionBranchRemote: shared.OriginRemoteNameConstant,
	}

	require.Error(t, handleBranchChangeAction(context.Background(), environment, repository, parameters))
}

func execShellOutput(output string) execshell.ExecutionResult {
	return execshell.ExecutionResult{StandardOutput: output}
}

func TestHandleBranchChangeActionRefreshesBranch(t *testing.T) {
	executor := &stubGitExecutor{
		responses: []stubGitResponse{
			{result: execShellOutput("origin\n")},
		},
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerError)
	reporter := &recordingReporter{}
	buffer := &strings.Builder{}
	environment := &workflow.Environment{
		GitExecutor:       executor,
		RepositoryManager: gitManager,
		Logger:            zap.NewNop(),
		Output:            buffer,
		Errors:            io.Discard,
		Reporter:          reporter,
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
	}

	parameters := map[string]any{
		taskOptionBranchName:     "feature/foo",
		taskOptionBranchRemote:   shared.OriginRemoteNameConstant,
		taskOptionRefreshEnabled: true,
		taskOptionRequireClean:   true,
	}

	require.NoError(t, handleBranchChangeAction(context.Background(), environment, repository, parameters))
	require.Len(t, executor.recorded, 8)
	require.Equal(t, []string{"remote"}, executor.recorded[0].Arguments)
	require.Equal(t, []string{"fetch", "--prune", "origin"}, executor.recorded[1].Arguments)
	require.Equal(t, []string{"switch", "feature/foo"}, executor.recorded[2].Arguments)
	require.Equal(t, []string{"pull", "--rebase"}, executor.recorded[3].Arguments)
	require.Equal(t, []string{"status", "--porcelain"}, executor.recorded[4].Arguments)
	require.Equal(t, []string{"fetch", "--prune"}, executor.recorded[5].Arguments)
	require.Equal(t, []string{"checkout", "feature/foo"}, executor.recorded[6].Arguments)
	require.Equal(t, []string{"pull", "--ff-only"}, executor.recorded[7].Arguments)
	require.Contains(t, buffer.String(), "REFRESHED: /tmp/project (feature/foo)")
	require.Len(t, reporter.events, 1)
	require.Equal(t, "true", reporter.events[0].Details["refresh"])
	require.Equal(t, "true", reporter.events[0].Details["require_clean"])
}
