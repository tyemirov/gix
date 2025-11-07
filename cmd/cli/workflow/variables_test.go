package workflow

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseVariableAssignments(t *testing.T) {
	vars, err := parseVariableAssignments([]string{"foo=bar", "Baz = qux"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"foo": "bar", "Baz": " qux"}, vars)
}

func TestParseVariableAssignmentsRejectsInvalidInput(t *testing.T) {
	_, err := parseVariableAssignments([]string{"missing"})
	require.Error(t, err)
}

func TestLoadVariablesFromFiles(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "vars.yaml")
	require.NoError(t, os.WriteFile(filePath, []byte("foo: bar\nbaz: 123\n"), 0o644))

	vars, err := loadVariablesFromFiles([]string{filePath})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"foo": "bar", "baz": "123"}, vars)
}
