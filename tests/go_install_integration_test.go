package tests

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

const (
	goInstallModulePath       = "github.com/tyemirov/gix"
	goInstallTransportVersion = "v1.1.26"
)

func TestGoInstallLatestReportsCurrentProductVersion(testInstance *testing.T) {
	repositoryRoot := releaseRepositoryRoot(testInstance)
	moduleSource := copyTrackedModuleSource(testInstance, repositoryRoot)
	proxyRoot := testInstance.TempDir()
	writeGoModuleProxyVersion(testInstance, proxyRoot, moduleSource, goInstallTransportVersion)
	writeReleaseFixtureFile(
		testInstance,
		filepath.Join(proxyRoot, "github.com", "tyemirov", "gix", "@v", "list"),
		"v1.1.25\n"+goInstallTransportVersion+"\n",
	)

	installRoot := testInstance.TempDir()
	moduleCache := filepath.Join(installRoot, "modcache")
	testInstance.Cleanup(func() {
		walkError := filepath.Walk(moduleCache, func(path string, fileInformation os.FileInfo, walkError error) error {
			if walkError != nil {
				return walkError
			}
			permissions := fileInformation.Mode().Perm() | 0o600
			if fileInformation.IsDir() {
				permissions |= 0o100
			}
			return os.Chmod(path, permissions)
		})
		require.NoError(testInstance, walkError)
	})
	executionContext, cancelFunction := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancelFunction()
	installCommand := exec.CommandContext(executionContext, "go", "install", goInstallModulePath+"@latest")
	installCommand.Env = append(os.Environ(),
		"GOBIN="+filepath.Join(installRoot, "bin"),
		"GOMODCACHE="+moduleCache,
		"GONOSUMDB="+goInstallModulePath,
		"GOPROXY=file://"+proxyRoot+",https://proxy.golang.org,direct",
	)
	installOutput, installError := installCommand.CombinedOutput()
	require.NoError(testInstance, installError, string(installOutput))

	configurationPath := writeCanonicalIntegrationConfiguration(testInstance)
	versionCommand := exec.CommandContext(
		executionContext,
		filepath.Join(installRoot, "bin", "gix"),
		"--config",
		configurationPath,
		"version",
	)
	versionOutput, versionError := versionCommand.CombinedOutput()
	require.NoError(testInstance, versionError, string(versionOutput))
	productVersion := strings.TrimSpace(readTextFile(testInstance, filepath.Join(repositoryRoot, "internal", "version", "product-version.txt")))
	require.Equal(testInstance, "gix version: "+productVersion+"\n", string(versionOutput))
}

func copyTrackedModuleSource(testInstance *testing.T, repositoryRoot string) string {
	testInstance.Helper()
	moduleSource := testInstance.TempDir()
	listCommand := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	listCommand.Dir = repositoryRoot
	listOutput, listError := listCommand.Output()
	require.NoError(testInstance, listError)
	for _, relativePath := range strings.Split(strings.TrimSuffix(string(listOutput), "\x00"), "\x00") {
		sourcePath := filepath.Join(repositoryRoot, relativePath)
		fileInformation, statError := os.Stat(sourcePath)
		require.NoError(testInstance, statError)
		if !fileInformation.Mode().IsRegular() {
			continue
		}
		contents, readError := os.ReadFile(sourcePath)
		require.NoError(testInstance, readError)
		destinationPath := filepath.Join(moduleSource, relativePath)
		require.NoError(testInstance, os.MkdirAll(filepath.Dir(destinationPath), 0o755))
		require.NoError(testInstance, os.WriteFile(destinationPath, contents, fileInformation.Mode().Perm()))
	}
	return moduleSource
}

func writeGoModuleProxyVersion(testInstance *testing.T, proxyRoot string, moduleSource string, version string) {
	testInstance.Helper()
	versionRoot := filepath.Join(proxyRoot, "github.com", "tyemirov", "gix", "@v")
	require.NoError(testInstance, os.MkdirAll(versionRoot, 0o755))
	moduleData, moduleReadError := os.ReadFile(filepath.Join(moduleSource, "go.mod"))
	require.NoError(testInstance, moduleReadError)
	require.NoError(testInstance, os.WriteFile(filepath.Join(versionRoot, version+".mod"), moduleData, 0o644))
	information, informationError := json.Marshal(map[string]string{
		"Version": version,
		"Time":    "2026-08-09T18:30:00Z",
	})
	require.NoError(testInstance, informationError)
	require.NoError(testInstance, os.WriteFile(filepath.Join(versionRoot, version+".info"), append(information, '\n'), 0o644))
	archive, archiveError := os.Create(filepath.Join(versionRoot, version+".zip"))
	require.NoError(testInstance, archiveError)
	require.NoError(testInstance, modzip.CreateFromDir(archive, module.Version{Path: goInstallModulePath, Version: version}, moduleSource))
	require.NoError(testInstance, archive.Close())
}
