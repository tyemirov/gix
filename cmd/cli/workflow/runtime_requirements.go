package workflow

import workflowpkg "github.com/tyemirov/gix/internal/workflow"

type runtimeRequirements struct {
	includeNestedRepositories            bool
	processRepositoriesByDescendingDepth bool
	captureInitialWorktreeStatus         bool
}

func deriveRuntimeRequirements(nodes []*workflowpkg.OperationNode) runtimeRequirements {
	requirements := runtimeRequirements{}
	for _, node := range nodes {
		if node == nil || node.Operation == nil {
			continue
		}
		renameOperation, isRename := node.Operation.(*workflowpkg.RenameOperation)
		if !isRename {
			continue
		}

		requirements.includeNestedRepositories = true
		requirements.processRepositoriesByDescendingDepth = true
		if renameOperation.RequireCleanWorktree {
			requirements.captureInitialWorktreeStatus = true
		}
	}
	return requirements
}
