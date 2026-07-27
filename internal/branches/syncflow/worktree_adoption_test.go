package syncflow

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrepareSiblingWorktreeForRemovalOnlyChangesDirectoriesInsideWorktree(testInstance *testing.T) {
	if runtime.GOOS == "windows" {
		testInstance.Skip("read-only directory permissions require Unix semantics")
	}

	worktreePath := testInstance.TempDir()
	cacheDirectory := filepath.Join(worktreePath, ".cache", "go", "pkg", "mod", "read-only-module")
	require.NoError(testInstance, os.MkdirAll(cacheDirectory, 0o755))
	cacheFile := filepath.Join(cacheDirectory, "cached-file")
	require.NoError(testInstance, os.WriteFile(cacheFile, []byte("cache\n"), 0o644))
	require.NoError(testInstance, os.Chmod(cacheFile, 0o444))
	require.NoError(testInstance, os.Chmod(cacheDirectory, 0o555))
	testInstance.Cleanup(func() {
		_ = os.Chmod(cacheDirectory, 0o755)
	})

	externalDirectory := testInstance.TempDir()
	require.NoError(testInstance, os.Chmod(externalDirectory, 0o555))
	testInstance.Cleanup(func() {
		_ = os.Chmod(externalDirectory, 0o755)
	})
	require.NoError(testInstance, os.Symlink(externalDirectory, filepath.Join(worktreePath, "external-cache")))

	require.NoError(testInstance, prepareSiblingWorktreeForRemoval(worktreePath))

	cacheDirectoryInfo, cacheDirectoryInfoErr := os.Stat(cacheDirectory)
	require.NoError(testInstance, cacheDirectoryInfoErr)
	require.Equal(testInstance, os.FileMode(0o300), cacheDirectoryInfo.Mode().Perm()&0o300)
	cacheFileInfo, cacheFileInfoErr := os.Stat(cacheFile)
	require.NoError(testInstance, cacheFileInfoErr)
	require.Equal(testInstance, os.FileMode(0o444), cacheFileInfo.Mode().Perm())
	externalDirectoryInfo, externalDirectoryInfoErr := os.Stat(externalDirectory)
	require.NoError(testInstance, externalDirectoryInfoErr)
	require.Equal(testInstance, os.FileMode(0o555), externalDirectoryInfo.Mode().Perm())
}
