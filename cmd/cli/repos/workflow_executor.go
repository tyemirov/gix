package repos

import (
	"context"

	"github.com/temirov/gix/internal/workflow"
)

// WorkflowExecutor runs compiled workflow operation graphs.
type WorkflowExecutor interface {
	Execute(ctx context.Context, roots []string, options workflow.RuntimeOptions) error
}
