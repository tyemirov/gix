package cli

import (
	"github.com/tyemirov/gix/internal/workflow"
	"github.com/tyemirov/gix/pkg/taskrunner"
)

// TaskRunnerExecutor coordinates workflow task execution.
type TaskRunnerExecutor = taskrunner.Executor

func resolveTaskRunner(factory taskrunner.Factory, dependencies workflow.Dependencies) TaskRunnerExecutor {
	return taskrunner.Resolve(factory, dependencies)
}
