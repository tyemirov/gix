package repos

import (
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
)

// TaskRunnerExecutor represents a workflow task runner.
type TaskRunnerExecutor = taskrunner.Executor

// TaskRunnerFactory constructs workflow executors.
type TaskRunnerFactory = taskrunner.Factory

// ResolveTaskRunner returns either the provided executor or a default workflow runner.
func ResolveTaskRunner(factory TaskRunnerFactory, dependencies workflow.Dependencies) TaskRunnerExecutor {
	return taskrunner.Resolve(factory, dependencies)
}
