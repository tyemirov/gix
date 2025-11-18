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
