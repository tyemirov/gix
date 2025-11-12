package taskrunner

import (
	"context"

	"github.com/temirov/gix/internal/workflow"
)

// Executor runs workflow task definitions across repositories.
type Executor interface {
	Run(ctx context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error)
}

// Factory constructs an Executor given workflow dependencies.
type Factory func(workflow.Dependencies) Executor

type taskRunnerAdapter struct {
	runner workflow.TaskRunner
}

func (adapter taskRunnerAdapter) Run(ctx context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	return adapter.runner.Run(ctx, roots, definitions, options)
}

// Resolve returns either the provided factory result or a default workflow task runner.
func Resolve(factory Factory, dependencies workflow.Dependencies) Executor {
	if factory != nil {
		if executor := factory(dependencies); executor != nil {
			return executor
		}
	}
	return taskRunnerAdapter{runner: workflow.NewTaskRunner(dependencies)}
}
