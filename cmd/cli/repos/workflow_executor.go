package repos

import (
	"context"

	"github.com/temirov/gix/internal/workflow"
)

// WorkflowExecutor executes workflow operation nodes across repositories.
type WorkflowExecutor interface {
	Execute(ctx context.Context, roots []string, options workflow.RuntimeOptions) error
}
