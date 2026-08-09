package taskrunner

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

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
	if executor.dependencies.DisableWorkflowLogging {
		return
	}
	writer := executor.summaryWriter()
	if writer == nil {
		return
	}

	summary := RenderSummaryLine(outcome.ReporterSummaryData, roots)
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
