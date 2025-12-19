package shared

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStructuredReporterSummaryCountsRepositoriesAndFormatsDuration(t *testing.T) {
	currentTime := time.Date(2025, time.January, 5, 10, 15, 0, 0, time.UTC)
	now := func() time.Time {
		return currentTime
	}

	reporter := NewStructuredReporter(
		&bytes.Buffer{},
		&bytes.Buffer{},
		WithRepositoryHeaders(false),
		WithNowProvider(now),
	)

	reporter.Report(Event{
		Code:           "TEST_EVENT",
		RepositoryPath: "/tmp/repos/sample",
		Message:        "completed",
	})

	currentTime = currentTime.Add(1500 * time.Millisecond)

	summary := reporter.Summary()
	require.Contains(t, summary, "Summary: total.repos=1")
	require.Contains(t, summary, "TEST_EVENT=1")
	require.Contains(t, summary, "WARN=0")
	require.Contains(t, summary, "ERROR=0")
	require.Contains(t, summary, "duration=1.5s")
}

func TestStructuredReporterRecordRepositoryIncludesObservedRoots(t *testing.T) {
	reporter := NewStructuredReporter(&bytes.Buffer{}, &bytes.Buffer{}, WithRepositoryHeaders(false))

	reporter.RecordRepository("", "/tmp/repos/demo")

	summary := reporter.Summary()
	require.Contains(t, summary, "Summary: total.repos=1")
	require.Contains(t, summary, "WARN=0")
	require.Contains(t, summary, "ERROR=0")
}

func TestStructuredReporterRecordEventCountsWithoutEmittingLogs(t *testing.T) {
	reporter := NewStructuredReporter(&bytes.Buffer{}, &bytes.Buffer{}, WithRepositoryHeaders(false))

	reporter.RecordEvent("workflow_operation_success", EventLevelInfo)
	reporter.RecordEvent("workflow_operation_success", EventLevelInfo)
	reporter.RecordEvent("workflow_operation_failure", EventLevelError)

	summary := reporter.Summary()
	require.NotContains(t, summary, "WORKFLOW_OPERATION_SUCCESS=2")
	require.Contains(t, summary, "WORKFLOW_OPERATION_FAILURE=1")
	require.Contains(t, summary, "ERROR=1")
}

func TestStructuredReporterSummaryIncludesStepOutcomes(t *testing.T) {
	reporter := NewStructuredReporter(&bytes.Buffer{}, &bytes.Buffer{}, WithRepositoryHeaders(false))

	reporter.Report(Event{
		Code: EventCodeWorkflowStepSummary,
		Details: map[string]string{
			"step":    "namespace-rewrite",
			"outcome": "applied",
		},
	})

	summary := reporter.Summary()
	require.Contains(t, summary, "STEP_NAMESPACE_REWRITE_APPLIED=1")
}

func TestStructuredReporterSummaryDataIncludesOperationDurations(t *testing.T) {
	currentTime := time.Date(2025, time.January, 5, 10, 15, 0, 0, time.UTC)
	now := func() time.Time {
		return currentTime
	}

	reporter := NewStructuredReporter(&bytes.Buffer{}, &bytes.Buffer{}, WithRepositoryHeaders(false), WithNowProvider(now))
	reporter.RecordRepository("tyemirov/demo", "/tmp/repos/demo")

	reporter.RecordOperationDuration("namespace", 1500*time.Millisecond)
	reporter.RecordOperationDuration("namespace", 500*time.Millisecond)
	reporter.RecordOperationDuration("remote", 250*time.Millisecond)

	currentTime = currentTime.Add(3 * time.Second)

	data := reporter.SummaryData()
	require.Equal(t, 1, data.TotalRepositories)
	require.EqualValues(t, 3000, data.DurationMilliseconds)
	require.Equal(t, "3s", data.DurationHuman)

	namespaceTiming, exists := data.OperationDurations["namespace"]
	require.True(t, exists)
	require.Equal(t, 2, namespaceTiming.Count)
	require.EqualValues(t, 2000, namespaceTiming.TotalDurationMilliseconds)
	require.EqualValues(t, 1000, namespaceTiming.AverageDurationMilliseconds)

	remoteTiming, exists := data.OperationDurations["remote"]
	require.True(t, exists)
	require.Equal(t, 1, remoteTiming.Count)
	require.EqualValues(t, 250, remoteTiming.TotalDurationMilliseconds)
	require.EqualValues(t, 250, remoteTiming.AverageDurationMilliseconds)

	require.NotNil(t, data.EventCounts)

	serialized, err := json.Marshal(data)
	require.NoError(t, err)
	require.Contains(t, string(serialized), "\"total_repositories\":1")
}

func TestStructuredReporterSummaryDataIncludesStageDurations(t *testing.T) {
	reporter := NewStructuredReporter(&bytes.Buffer{}, &bytes.Buffer{}, WithRepositoryHeaders(false))

	reporter.RecordStageDuration("stage-1", 2*time.Second)
	reporter.RecordStageDuration("stage-1", 1*time.Second)
	reporter.RecordStageDuration("stage-2", 500*time.Millisecond)

	data := reporter.SummaryData()
	firstStage, exists := data.StageDurations["stage-1"]
	require.True(t, exists)
	require.Equal(t, 2, firstStage.Count)
	require.EqualValues(t, 3000, firstStage.TotalDurationMilliseconds)
	require.EqualValues(t, 1500, firstStage.AverageDurationMilliseconds)

	secondStage, exists := data.StageDurations["stage-2"]
	require.True(t, exists)
	require.Equal(t, 1, secondStage.Count)
	require.EqualValues(t, 500, secondStage.TotalDurationMilliseconds)
	require.EqualValues(t, 500, secondStage.AverageDurationMilliseconds)
}

func TestStructuredReporterHonorsDiscardWriters(t *testing.T) {
	originalStdout := os.Stdout
	reader, writer, pipeErr := os.Pipe()
	require.NoError(t, pipeErr)
	os.Stdout = writer
	t.Cleanup(func() {
		writer.Close()
		reader.Close()
		os.Stdout = originalStdout
	})

	reporter := NewStructuredReporter(io.Discard, io.Discard, WithRepositoryHeaders(false))
	reporter.Report(Event{
		Code:           "TASK_PLAN",
		RepositoryPath: "/tmp/repos/sample",
		Message:        "noop",
	})

	require.NoError(t, writer.Close())
	output, readErr := io.ReadAll(reader)
	require.NoError(t, readErr)
	require.Len(t, output, 0)
}

func TestStructuredReporterConsoleOutputOmitsMachinePart(t *testing.T) {
	buffer := &bytes.Buffer{}
	reporter := NewStructuredReporter(buffer, buffer, WithRepositoryHeaders(true))

	reporter.Report(Event{
		Code:                 EventCodeRepoSwitched,
		RepositoryIdentifier: "tyemirov/gix",
		RepositoryPath:       "/tmp/repos/gix",
		Message:              "→ master",
		Details:              map[string]string{"branch": "master"},
	})

	output := buffer.String()
	require.Contains(t, output, "-- repo: tyemirov/gix")
	require.Contains(t, output, EventCodeRepoSwitched)
	require.Contains(t, output, "→ master")
	require.NotContains(t, output, "event=")
	require.NotContains(t, output, "branch=master")
}

func TestStructuredReporterConsoleSuppressesInternalTaskEvents(t *testing.T) {
	buffer := &bytes.Buffer{}
	reporter := NewStructuredReporter(buffer, buffer, WithRepositoryHeaders(true))

	reporter.Report(Event{
		Code:                 EventCodeTaskPlan,
		RepositoryIdentifier: "tyemirov/gix",
		RepositoryPath:       "/tmp/repos/gix",
		Message:              "task plan ready",
	})
	reporter.Report(Event{
		Code:                 EventCodeRepoSwitched,
		RepositoryIdentifier: "tyemirov/gix",
		RepositoryPath:       "/tmp/repos/gix",
		Message:              "→ master",
	})

	output := buffer.String()
	require.Contains(t, output, "-- repo: tyemirov/gix")
	require.Contains(t, output, EventCodeRepoSwitched)
	require.NotContains(t, output, EventCodeTaskPlan)
	require.NotContains(t, output, "task plan ready")
}
