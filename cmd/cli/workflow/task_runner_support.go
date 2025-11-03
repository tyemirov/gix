package workflow

import (
	"context"

	"github.com/tyemirov/gix/internal/workflow"
)

// TaskRunnerExecutor represents an executor capable of running workflow tasks.
type TaskRunnerExecutor interface {
	Run(ctx context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) error
}

type taskRunnerAdapter struct {
	runner workflow.TaskRunner
}

func (adapter taskRunnerAdapter) Run(ctx context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) error {
	return adapter.runner.Run(ctx, roots, definitions, options)
}

func resolveTaskRunner(factory func(workflow.Dependencies) TaskRunnerExecutor, dependencies workflow.Dependencies) TaskRunnerExecutor {
	if factory != nil {
		return factory(dependencies)
	}
	return taskRunnerAdapter{runner: workflow.NewTaskRunner(dependencies)}
}
