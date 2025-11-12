package workflow

import (
	"context"
	"time"
)

// TaskRunner orchestrates task execution for imperative callers.
type TaskRunner struct {
	dependencies Dependencies
}

// NewTaskRunner constructs a TaskRunner with the provided dependencies.
func NewTaskRunner(dependencies Dependencies) TaskRunner {
	return TaskRunner{dependencies: dependencies}
}

// Run executes the supplied task definitions across the provided repository roots.
func (runner TaskRunner) Run(ctx context.Context, roots []string, definitions []TaskDefinition, options RuntimeOptions) (ExecutionOutcome, error) {
	if len(definitions) == 0 {
		now := time.Now()
		return ExecutionOutcome{StartTime: now, EndTime: now}, nil
	}

	tasks := make([]TaskDefinition, len(definitions))
	copy(tasks, definitions)

	operation := &TaskOperation{tasks: tasks}
	executor := NewExecutor([]Operation{operation}, runner.dependencies)
	return executor.Execute(ctx, roots, options)
}
