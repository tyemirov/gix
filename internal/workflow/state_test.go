package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/audit"
)

func TestRepositoryStateRefreshSkipsWhenAuditServiceUnavailable(testInstance *testing.T) {
	repositoryState := &RepositoryState{Path: "/tmp/workflow/repository", Inspection: audit.RepositoryInspection{Path: "/tmp/workflow/repository"}}

	require.NoError(testInstance, repositoryState.Refresh(context.Background(), nil))
}
