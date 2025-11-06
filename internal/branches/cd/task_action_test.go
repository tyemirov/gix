package cd

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/temirov/gix/internal/audit"
	"github.com/temirov/gix/internal/execshell"
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

func (reporter *recordingReporter) PrintSummary() {
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
