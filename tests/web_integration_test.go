package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/web"
)

const (
	webIntegrationListenHostConstant           = "127.0.0.1"
	webIntegrationStartupTimeoutConstant       = 10 * time.Second
	webIntegrationPollIntervalConstant         = 50 * time.Millisecond
	webIntegrationRequestTimeoutConstant       = 2 * time.Second
	webIntegrationExternalCDNHostConstant      = "cdn.jsdelivr.net"
	webIntegrationIndexAssetPathConstant       = "/"
	webIntegrationApplicationAssetPathConstant = "/assets/app.js"
	webIntegrationStylesAssetPathConstant      = "/assets/styles.css"
	webIntegrationRepositoriesAPIPathConstant  = "/api/repos"
	webIntegrationBindArgumentTemplateConstant = "--bind=%s"
	webIntegrationPortArgumentTemplateConstant = "--port=%d"
	webIntegrationWildcardHostConstant         = "0.0.0.0"
)

func TestWebBinaryEmbedsFirstPartyAssetsAndUsesCDNDependencies(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	workingDirectory := testInstance.TempDir()
	listenPort := reserveFreeLoopbackPort(testInstance)
	baseURL := fmt.Sprintf("http://%s:%d", webIntegrationListenHostConstant, listenPort)

	command := exec.Command(binaryPath, "--web", fmt.Sprintf(webIntegrationPortArgumentTemplateConstant, listenPort))
	command.Dir = workingDirectory
	command.Env = buildCommandEnvironment(integrationCommandOptions{})

	standardOutput, standardError := startLongRunningCommand(testInstance, command)
	defer stopLongRunningCommand(testInstance, command)

	waitForWebServerReady(testInstance, baseURL+webIntegrationIndexAssetPathConstant)

	indexDocument := readHTTPBody(testInstance, baseURL+webIntegrationIndexAssetPathConstant)
	require.Contains(testInstance, indexDocument, "https://cdn.jsdelivr.net/npm/wunderbaum@0/dist/wunderbaum.min.css")
	require.Contains(testInstance, indexDocument, "/assets/styles.css")
	require.Contains(testInstance, indexDocument, "/assets/app.js")
	require.Contains(testInstance, indexDocument, webIntegrationExternalCDNHostConstant)

	applicationScript := readHTTPBody(testInstance, baseURL+webIntegrationApplicationAssetPathConstant)
	require.Contains(testInstance, applicationScript, "from \"https://cdn.jsdelivr.net/npm/wunderbaum@0/+esm\"")

	stylesSheet := readHTTPBody(testInstance, baseURL+webIntegrationStylesAssetPathConstant)
	require.Contains(testInstance, stylesSheet, ".repo-tree")

	require.Contains(testInstance, standardOutput.String(), baseURL)
	require.Empty(testInstance, strings.TrimSpace(standardError.String()))
}

func TestWebBinaryHonorsExplicitBindAndPortFlags(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	workingDirectory := testInstance.TempDir()
	listenPort := reserveFreeLoopbackPort(testInstance)
	baseURL := fmt.Sprintf("http://%s:%d", webIntegrationListenHostConstant, listenPort)

	command := exec.Command(
		binaryPath,
		"--web",
		fmt.Sprintf(webIntegrationBindArgumentTemplateConstant, webIntegrationWildcardHostConstant),
		fmt.Sprintf(webIntegrationPortArgumentTemplateConstant, listenPort),
	)
	command.Dir = workingDirectory
	command.Env = buildCommandEnvironment(integrationCommandOptions{})

	standardOutput, standardError := startLongRunningCommand(testInstance, command)
	defer stopLongRunningCommand(testInstance, command)

	waitForWebServerReady(testInstance, baseURL+webIntegrationIndexAssetPathConstant)

	indexDocument := readHTTPBody(testInstance, baseURL+webIntegrationIndexAssetPathConstant)
	require.Contains(testInstance, indexDocument, "<title>gix Control Surface</title>")
	require.Contains(testInstance, standardOutput.String(), fmt.Sprintf("http://%s:%d", webIntegrationWildcardHostConstant, listenPort))
	require.Empty(testInstance, strings.TrimSpace(standardError.String()))
}

func TestWebBinaryScopesInitialRepositoryCatalogToExplicitRoots(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	workingDirectory := testInstance.TempDir()
	launchRoot := filepath.Join(testInstance.TempDir(), "fleet")
	firstRepositoryPath := createGitRepository(testInstance, gitRepositoryOptions{Path: filepath.Join(launchRoot, "alpha")})
	secondRepositoryPath := createGitRepository(testInstance, gitRepositoryOptions{Path: filepath.Join(launchRoot, "nested", "beta")})
	createGitRepository(testInstance, gitRepositoryOptions{Path: filepath.Join(testInstance.TempDir(), "ignored", "skip")})
	listenPort := reserveFreeLoopbackPort(testInstance)
	baseURL := fmt.Sprintf("http://%s:%d", webIntegrationListenHostConstant, listenPort)

	command := exec.Command(
		binaryPath,
		"--web",
		fmt.Sprintf(webIntegrationPortArgumentTemplateConstant, listenPort),
		"--roots", launchRoot,
	)
	command.Dir = workingDirectory
	command.Env = buildCommandEnvironment(integrationCommandOptions{})

	standardOutput, standardError := startLongRunningCommand(testInstance, command)
	defer stopLongRunningCommand(testInstance, command)

	waitForWebServerReady(testInstance, baseURL+webIntegrationIndexAssetPathConstant)

	repositoryCatalog := readRepositoryCatalog(testInstance, baseURL+webIntegrationRepositoriesAPIPathConstant)
	require.Equal(testInstance, "configured_roots", repositoryCatalog.LaunchMode)
	require.Equal(testInstance, canonicalFilesystemPath(testInstance, launchRoot), canonicalFilesystemPath(testInstance, repositoryCatalog.LaunchPath))
	require.Len(testInstance, repositoryCatalog.LaunchRoots, 1)
	require.Equal(testInstance, canonicalFilesystemPath(testInstance, launchRoot), canonicalFilesystemPath(testInstance, repositoryCatalog.LaunchRoots[0]))
	require.Equal(testInstance, canonicalFilesystemPath(testInstance, launchRoot), canonicalFilesystemPath(testInstance, repositoryCatalog.ExplorerRoot))
	require.Len(testInstance, repositoryCatalog.Repositories, 2)
	require.Equal(testInstance, canonicalFilesystemPath(testInstance, firstRepositoryPath), canonicalFilesystemPath(testInstance, repositoryCatalog.Repositories[0].Path))
	require.Equal(testInstance, canonicalFilesystemPath(testInstance, secondRepositoryPath), canonicalFilesystemPath(testInstance, repositoryCatalog.Repositories[1].Path))
	require.Contains(testInstance, standardOutput.String(), baseURL)
	require.Empty(testInstance, strings.TrimSpace(standardError.String()))
}

func TestWebBinaryResolvesRelativeExplicitRootsWithoutUsingCurrentRepositoryContext(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	launchRoot := filepath.Join(testInstance.TempDir(), "tyemirov")
	currentRepositoryPath := createGitRepository(testInstance, gitRepositoryOptions{Path: filepath.Join(launchRoot, "gix")})
	siblingRepositoryPath := createGitRepository(testInstance, gitRepositoryOptions{Path: filepath.Join(launchRoot, "alpha")})
	workingDirectory := filepath.Join(currentRepositoryPath, "internal")
	require.NoError(testInstance, os.MkdirAll(workingDirectory, 0o755))
	listenPort := reserveFreeLoopbackPort(testInstance)
	baseURL := fmt.Sprintf("http://%s:%d", webIntegrationListenHostConstant, listenPort)

	command := exec.Command(
		binaryPath,
		"--web",
		fmt.Sprintf(webIntegrationPortArgumentTemplateConstant, listenPort),
		"--roots", "../..",
	)
	command.Dir = workingDirectory
	command.Env = buildCommandEnvironment(integrationCommandOptions{})

	standardOutput, standardError := startLongRunningCommand(testInstance, command)
	defer stopLongRunningCommand(testInstance, command)

	waitForWebServerReady(testInstance, baseURL+webIntegrationIndexAssetPathConstant)

	repositoryCatalog := readRepositoryCatalog(testInstance, baseURL+webIntegrationRepositoriesAPIPathConstant)
	require.Equal(testInstance, "configured_roots", repositoryCatalog.LaunchMode)
	require.Equal(testInstance, canonicalFilesystemPath(testInstance, launchRoot), canonicalFilesystemPath(testInstance, repositoryCatalog.LaunchPath))
	require.Len(testInstance, repositoryCatalog.LaunchRoots, 1)
	require.Equal(testInstance, canonicalFilesystemPath(testInstance, launchRoot), canonicalFilesystemPath(testInstance, repositoryCatalog.LaunchRoots[0]))
	require.Len(testInstance, repositoryCatalog.Repositories, 2)
	require.Equal(testInstance, canonicalFilesystemPath(testInstance, siblingRepositoryPath), canonicalFilesystemPath(testInstance, repositoryCatalog.Repositories[0].Path))
	require.Equal(testInstance, canonicalFilesystemPath(testInstance, currentRepositoryPath), canonicalFilesystemPath(testInstance, repositoryCatalog.Repositories[1].Path))
	require.False(testInstance, repositoryCatalog.Repositories[0].ContextCurrent)
	require.False(testInstance, repositoryCatalog.Repositories[1].ContextCurrent)
	require.Empty(testInstance, repositoryCatalog.SelectedRepositoryID)
	require.Contains(testInstance, standardOutput.String(), baseURL)
	require.Empty(testInstance, strings.TrimSpace(standardError.String()))
}

func reserveFreeLoopbackPort(testInstance *testing.T) int {
	testInstance.Helper()

	listener, listenError := net.Listen("tcp", net.JoinHostPort(webIntegrationListenHostConstant, "0"))
	require.NoError(testInstance, listenError)
	defer listener.Close()

	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	require.True(testInstance, ok)
	return tcpAddress.Port
}

func startLongRunningCommand(testInstance *testing.T, command *exec.Cmd) (*strings.Builder, *strings.Builder) {
	testInstance.Helper()

	standardOutput := &strings.Builder{}
	standardError := &strings.Builder{}
	command.Stdout = standardOutput
	command.Stderr = standardError

	startError := command.Start()
	require.NoError(testInstance, startError)
	return standardOutput, standardError
}

func stopLongRunningCommand(testInstance *testing.T, command *exec.Cmd) {
	testInstance.Helper()

	if command.Process == nil {
		return
	}

	_ = command.Process.Kill()
	_ = command.Wait()
}

func waitForWebServerReady(testInstance *testing.T, endpoint string) {
	testInstance.Helper()

	deadline := time.Now().Add(webIntegrationStartupTimeoutConstant)
	client := &http.Client{Timeout: webIntegrationRequestTimeoutConstant}

	for time.Now().Before(deadline) {
		response, requestError := client.Get(endpoint)
		if requestError == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(webIntegrationPollIntervalConstant)
	}

	testInstance.Fatalf("web server did not become ready at %s", endpoint)
}

func readHTTPBody(testInstance *testing.T, endpoint string) string {
	testInstance.Helper()

	client := &http.Client{Timeout: webIntegrationRequestTimeoutConstant}
	response, requestError := client.Get(endpoint)
	require.NoError(testInstance, requestError)
	defer response.Body.Close()
	require.Equal(testInstance, http.StatusOK, response.StatusCode)

	bodyBytes, readError := io.ReadAll(response.Body)
	require.NoError(testInstance, readError)
	return string(bodyBytes)
}

func readRepositoryCatalog(testInstance *testing.T, endpoint string) web.RepositoryCatalog {
	testInstance.Helper()

	client := &http.Client{Timeout: webIntegrationRequestTimeoutConstant}
	response, requestError := client.Get(endpoint)
	require.NoError(testInstance, requestError)
	defer response.Body.Close()
	require.Equal(testInstance, http.StatusOK, response.StatusCode)

	var repositoryCatalog web.RepositoryCatalog
	require.NoError(testInstance, json.NewDecoder(response.Body).Decode(&repositoryCatalog))
	return repositoryCatalog
}

func canonicalFilesystemPath(testInstance *testing.T, path string) string {
	testInstance.Helper()

	resolvedPath, resolveError := filepath.EvalSymlinks(path)
	if resolveError == nil {
		return resolvedPath
	}
	if !os.IsNotExist(resolveError) {
		require.NoError(testInstance, resolveError)
	}

	absolutePath, absolutePathError := filepath.Abs(path)
	require.NoError(testInstance, absolutePathError)
	return absolutePath
}
