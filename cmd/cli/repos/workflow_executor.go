package repos

import (
	"context"

	"github.com/temirov/gix/internal/workflow"
)

// WorkflowExecutor runs compiled workflow operation graphs.
type WorkflowExecutor interface {
	Execute(ctx context.Context, roots []string, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error)
}

// WorkflowExecutorFactory constructs workflow executors.
type WorkflowExecutorFactory func(nodes []*workflow.OperationNode, dependencies workflow.Dependencies) WorkflowExecutor

func resolveWorkflowExecutor(factory WorkflowExecutorFactory, nodes []*workflow.OperationNode, dependencies workflow.Dependencies) WorkflowExecutor {
	if factory != nil {
		if executor := factory(nodes, dependencies); executor != nil {
			return executor
		}
	}
	return workflowExecutorAdapter{executor: workflow.NewExecutorFromNodes(nodes, dependencies)}
}

type workflowExecutorAdapter struct {
	executor *workflow.Executor
}

func (adapter workflowExecutorAdapter) Execute(ctx context.Context, roots []string, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	return adapter.executor.Execute(ctx, roots, options)
}
