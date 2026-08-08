package workflow

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTaskFileModeReplace(t *testing.T) {
	require.Equal(t, TaskFileModeReplace, parseTaskFileMode("replace"))
	require.Equal(t, TaskFileModeReplace, parseTaskFileMode("  REPLACE  "))
	require.Equal(t, TaskFileModeOverwrite, parseTaskFileMode("unknown"))
}

func TestBuildTaskFilesPreservesContentBoundary(t *testing.T) {
	files, buildError := buildTaskFiles(newOptionReader(map[string]any{
		"files": []any{
			map[string]any{
				"path":    "LICENSE",
				"content": "# Exact text\n",
			},
		},
	}))

	require.NoError(t, buildError)
	require.Equal(t, "# Exact text\n", files[0].ContentTemplate)
}
