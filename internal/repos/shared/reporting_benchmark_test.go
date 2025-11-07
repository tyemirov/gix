package shared

import (
	"bytes"
	"testing"
	"time"
)

func BenchmarkStructuredReporterReport(b *testing.B) {
	reporter := NewStructuredReporter(&bytes.Buffer{}, &bytes.Buffer{}, WithRepositoryHeaders(false))
	event := Event{
		Code:                 "BENCH_EVENT",
		Level:                EventLevelInfo,
		RepositoryIdentifier: "temirov/demo",
		Message:              "completed",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		reporter.Report(event)
	}
}

func BenchmarkStructuredReporterRecordEvent(b *testing.B) {
	reporter := NewStructuredReporter(&bytes.Buffer{}, &bytes.Buffer{}, WithRepositoryHeaders(false))

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		reporter.RecordEvent("benchmark_event", EventLevelWarn)
	}
}

func BenchmarkStructuredReporterRecordOperationDuration(b *testing.B) {
	reporter := NewStructuredReporter(&bytes.Buffer{}, &bytes.Buffer{}, WithRepositoryHeaders(false))
	duration := 5 * time.Millisecond

	b.ReportAllocs()
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		reporter.RecordOperationDuration("benchmark-operation", duration)
	}
}
