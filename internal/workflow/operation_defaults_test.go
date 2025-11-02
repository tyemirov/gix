package workflow

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestApplyDefaultsSetsRequireCleanWhenMissing(t *testing.T) {
	operation := &RenameOperation{}
	nodes := []*OperationNode{
		{Name: "rename-1", Operation: operation},
	}

	ApplyDefaults(nodes, OperationDefaults{RequireClean: true})

	require.True(t, operation.RequireCleanWorktree)
}

func TestApplyDefaultsRespectsExplicitRequireClean(t *testing.T) {
	operation := &RenameOperation{RequireCleanWorktree: false, requireCleanExplicit: true}
	nodes := []*OperationNode{
		{Name: "rename-1", Operation: operation},
	}

	ApplyDefaults(nodes, OperationDefaults{RequireClean: true})

	require.False(t, operation.RequireCleanWorktree)
}
