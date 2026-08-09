package taskrunner

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/workflow"
)

type fakeExecutor struct {
	outcome workflow.ExecutionOutcome
	err     error
}

func (executor fakeExecutor) Run(_ context.Context, _ []string, _ []workflow.TaskDefinition, _ workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	return executor.outcome, executor.err
}

func TestRenderSummaryLineSkipsSingleRepository(t *testing.T) {
	summary := RenderSummaryLine(shared.SummaryData{TotalRepositories: 1}, []string{"repo"})
	require.Equal(t, "", summary)
}

func TestRenderSummaryLineFormatsCounts(t *testing.T) {
	data := shared.SummaryData{
		TotalRepositories:    2,
		EventCounts:          map[string]int{"repo_switched": 2},
		LevelCounts:          map[shared.EventLevel]int{shared.EventLevelWarn: 1, shared.EventLevelError: 0},
		DurationHuman:        "1s",
		DurationMilliseconds: 1000,
	}
	summary := RenderSummaryLine(data, nil)
	require.Contains(t, summary, "Summary: total.repos=2")
	require.Contains(t, summary, "repo_switched=2")
	require.Contains(t, summary, "WARN=1")
	require.Contains(t, summary, "ERROR=0")
	require.Contains(t, summary, "duration=1s")
}

func TestSummaryExecutorPrintsSummaryForMultipleRepositories(t *testing.T) {
	buffer := &bytes.Buffer{}
	executor := summaryExecutor{
		delegate: fakeExecutor{
			outcome: workflow.ExecutionOutcome{
				ReporterSummaryData: shared.SummaryData{
					TotalRepositories:    2,
					DurationHuman:        "100ms",
					DurationMilliseconds: 100,
				},
			},
		},
		dependencies: workflow.Dependencies{
			Errors: buffer,
		},
	}

	_, err := executor.Run(context.Background(), []string{"/tmp/repo-one", "/tmp/repo-two"}, nil, workflow.RuntimeOptions{})
	require.NoError(t, err)
	require.Contains(t, buffer.String(), "Summary: total.repos=2")
}
