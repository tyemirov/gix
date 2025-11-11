package workflow

import (
	"time"

	"github.com/temirov/gix/internal/repos/shared"
)

// ExecutionOutcome captures aggregated workflow execution metrics.
type ExecutionOutcome struct {
	StartTime           time.Time
	EndTime             time.Time
	Duration            time.Duration
	RepositoryCount     int
	StageOutcomes       []StageOutcome
	OperationOutcomes   []OperationOutcome
	Failures            []OperationFailure
	ReporterSummaryData shared.SummaryData
}

// StageOutcome describes the operations executed within a particular stage.
type StageOutcome struct {
	Index      int
	Operations []string
	Duration   time.Duration
}

// OperationOutcome reports the execution status for a single operation.
type OperationOutcome struct {
	Name     string
	Duration time.Duration
	Failed   bool
	Error    error
}

// OperationFailure captures a formatted failure for user-facing reporting.
type OperationFailure struct {
	Name    string
	Message string
	Error   error
}
