package taskrunner

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/workflow"
)

// Executor runs workflow task definitions across repositories.
type Executor interface {
	Run(ctx context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error)
}

// Factory constructs an Executor given workflow dependencies.
type Factory func(workflow.Dependencies) Executor

type workflowRunner interface {
	Run(ctx context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error)
}

type taskRunnerAdapter struct {
	runner workflowRunner
}

func (adapter taskRunnerAdapter) Run(ctx context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	return adapter.runner.Run(ctx, roots, definitions, options)
}

// Resolve returns either the provided factory result or a default workflow task runner.
func Resolve(factory Factory, dependencies workflow.Dependencies) Executor {
	var base Executor
	if factory != nil {
		base = factory(dependencies)
	}
	if base == nil {
		base = taskRunnerAdapter{runner: workflow.NewTaskRunner(dependencies)}
	}
	return summaryExecutor{
		delegate:     base,
		dependencies: dependencies,
	}
}

type summaryExecutor struct {
	delegate     Executor
	dependencies workflow.Dependencies
}

func (executor summaryExecutor) Run(ctx context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	outcome, err := executor.delegate.Run(ctx, roots, definitions, options)
	executor.printSummary(outcome, roots)
	return outcome, err
}

func (executor summaryExecutor) printSummary(outcome workflow.ExecutionOutcome, roots []string) {
	writer := executor.summaryWriter()
	if writer == nil {
		return
	}

	summary := renderSummaryLine(outcome.ReporterSummaryData, roots)
	if len(strings.TrimSpace(summary)) == 0 {
		return
	}
	fmt.Fprintln(writer, summary)
}

func (executor summaryExecutor) summaryWriter() io.Writer {
	if executor.dependencies.Errors != nil {
		return executor.dependencies.Errors
	}
	if executor.dependencies.Output != nil {
		return executor.dependencies.Output
	}
	return nil
}

func renderSummaryLine(data shared.SummaryData, roots []string) string {
	repositoryCount := data.TotalRepositories
	if repositoryCount == 0 {
		repositoryCount = deduplicateRoots(roots)
	}
	if repositoryCount <= 1 {
		return ""
	}

	parts := []string{fmt.Sprintf("Summary: total.repos=%d", repositoryCount)}

	if len(data.EventCounts) > 0 {
		keys := make([]string, 0, len(data.EventCounts))
		for key := range data.EventCounts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s=%d", key, data.EventCounts[key]))
		}
	}

	warnCount := data.LevelCounts[shared.EventLevelWarn]
	errorCount := data.LevelCounts[shared.EventLevelError]

	parts = append(parts, fmt.Sprintf("%s=%d", shared.EventLevelWarn, warnCount))
	parts = append(parts, fmt.Sprintf("%s=%d", shared.EventLevelError, errorCount))

	durationHuman := strings.TrimSpace(data.DurationHuman)
	if durationHuman == "" {
		durationHuman = "0s"
	}

	parts = append(parts, fmt.Sprintf("duration_human=%s", durationHuman))
	parts = append(parts, fmt.Sprintf("duration_ms=%d", data.DurationMilliseconds))

	return strings.Join(parts, " ")
}

func deduplicateRoots(roots []string) int {
	if len(roots) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		trimmed := strings.TrimSpace(root)
		if trimmed == "" {
			continue
		}
		cleaned := filepath.Clean(trimmed)
		if _, exists := seen[cleaned]; exists {
			continue
		}
		seen[cleaned] = struct{}{}
	}
	return len(seen)
}
