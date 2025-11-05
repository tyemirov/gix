package shared

import (
	"bytes"
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
	require.Contains(t, summary, "duration_human=1.5s")
	require.Contains(t, summary, "duration_ms=1500")
}
