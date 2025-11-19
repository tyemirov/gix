package rename_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/repos/rename"
	"github.com/tyemirov/gix/internal/repos/shared"
)

func TestNewOptionsRejectsEmptyName(t *testing.T) {
	t.Parallel()

	path, err := shared.NewRepositoryPath("/tmp/repo")
	require.NoError(t, err)

	_, buildError := rename.NewOptions(rename.OptionsDefinition{
		RepositoryPath:    path,
		DesiredFolderName: "  ",
	})
	require.Error(t, buildError)
}

func TestNewOptionsTrimsName(t *testing.T) {
	t.Parallel()

	path, err := shared.NewRepositoryPath("/tmp/repo")
	require.NoError(t, err)

	options, buildError := rename.NewOptions(rename.OptionsDefinition{
		RepositoryPath:    path,
		DesiredFolderName: " example ",
	})
	require.NoError(t, buildError)

	require.Equal(t, "example", options.DesiredFolderName())
}
