package workflow

import (
	"context"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/temirov/gix/internal/repos/shared"
	workflowpkg "github.com/temirov/gix/internal/workflow"
)

// LoggerProvider yields a zap logger for command execution.
type LoggerProvider func() *zap.Logger

// PrompterFactory constructs confirmation prompters scoped to a command.
type PrompterFactory func(*cobra.Command) shared.ConfirmationPrompter

// OperationExecutor coordinates workflow execution.
type OperationExecutor interface {
	Execute(ctx context.Context, roots []string, options workflowpkg.RuntimeOptions) (workflowpkg.ExecutionOutcome, error)
}

// OperationExecutorFactory constructs workflow executors.
type OperationExecutorFactory func(nodes []*workflowpkg.OperationNode, dependencies workflowpkg.Dependencies) OperationExecutor

// ResolveOperationExecutor returns a custom executor when provided, otherwise it builds the default workflow executor.
func ResolveOperationExecutor(factory OperationExecutorFactory, nodes []*workflowpkg.OperationNode, dependencies workflowpkg.Dependencies) OperationExecutor {
	if factory != nil {
		if executor := factory(nodes, dependencies); executor != nil {
			return executor
		}
	}
	return operationExecutorAdapter{executor: workflowpkg.NewExecutorFromNodes(nodes, dependencies)}
}

type operationExecutorAdapter struct {
	executor *workflowpkg.Executor
}

func (adapter operationExecutorAdapter) Execute(ctx context.Context, roots []string, options workflowpkg.RuntimeOptions) (workflowpkg.ExecutionOutcome, error) {
	return adapter.executor.Execute(ctx, roots, options)
}

func displayCommandHelp(command *cobra.Command) error {
	if command == nil {
		return nil
	}
	return command.Help()
}
