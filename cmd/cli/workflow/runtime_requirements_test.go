package workflow

import (
	"testing"

	"github.com/stretchr/testify/require"

	workflowpkg "github.com/tyemirov/gix/internal/workflow"
)

func TestDeriveRuntimeRequirementsForRenameOperation(t *testing.T) {
	nodes := []*workflowpkg.OperationNode{
		{
			Name: "rename",
			Operation: &workflowpkg.RenameOperation{
				RequireCleanWorktree: true,
			},
		},
	}

	requirements := deriveRuntimeRequirements(nodes)

	require.True(t, requirements.includeNestedRepositories)
	require.True(t, requirements.processRepositoriesByDescendingDepth)
	require.True(t, requirements.captureInitialWorktreeStatus)
}

func TestDeriveRuntimeRequirementsIgnoresNonRenameOperations(t *testing.T) {
	nodes := []*workflowpkg.OperationNode{
		{
			Name:      "audit",
			Operation: &workflowpkg.AuditReportOperation{},
		},
		nil,
	}

	requirements := deriveRuntimeRequirements(nodes)

	require.False(t, requirements.includeNestedRepositories)
	require.False(t, requirements.processRepositoriesByDescendingDepth)
	require.False(t, requirements.captureInitialWorktreeStatus)
}
