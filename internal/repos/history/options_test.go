package history_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/repos/history"
	"github.com/temirov/gix/internal/repos/shared"
)

func TestNewPaths(t *testing.T) {
	t.Parallel()

	segments, err := history.NewPaths([]string{" secrets.txt ", "./nested/creds.env", "nested/creds.env"})
	require.NoError(t, err)
	require.Len(t, segments, 2)
	require.Equal(t, "nested/creds.env", segments[0].String())
	require.Equal(t, "secrets.txt", segments[1].String())
}

func TestNewPathsRejectsInvalid(t *testing.T) {
	t.Parallel()

	_, err := history.NewPaths([]string{"../escape"})
	require.Error(t, err)
}

func TestNewOptionsRequiresPaths(t *testing.T) {
	t.Parallel()

	repoPath, err := shared.NewRepositoryPath("/tmp/repo")
	require.NoError(t, err)

	_, buildError := history.NewOptions(repoPath, []shared.RepositoryPathSegment{}, nil, false, false, false)
	require.Error(t, buildError)
}
