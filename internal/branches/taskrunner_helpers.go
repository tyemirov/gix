package branches

import (
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
)

// TaskRunnerExecutor coordinates workflow task execution.
type TaskRunnerExecutor = taskrunner.Executor

func resolveTaskRunner(factory taskrunner.Factory, dependencies workflow.Dependencies) TaskRunnerExecutor {
	return taskrunner.Resolve(factory, dependencies)
}
