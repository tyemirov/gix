package tests

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func releaseRepositoryRoot(testInstance *testing.T) string {
	testInstance.Helper()
	workingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	return filepath.Dir(workingDirectory)
}

func writeReleaseFixtureFile(testInstance *testing.T, path string, contents string) {
	testInstance.Helper()
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(testInstance, os.WriteFile(path, []byte(contents), 0o644))
}
