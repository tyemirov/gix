package tests

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	webIntegrationListenHostConstant             = "127.0.0.1"
	webIntegrationStartupTimeoutConstant         = 10 * time.Second
	webIntegrationPollIntervalConstant           = 50 * time.Millisecond
	webIntegrationRequestTimeoutConstant         = 2 * time.Second
	webIntegrationExternalCDNHostConstant        = "cdn.jsdelivr.net"
	webIntegrationIndexAssetPathConstant         = "/"
	webIntegrationApplicationAssetPathConstant   = "/assets/app.js"
	webIntegrationStylesAssetPathConstant        = "/assets/styles.css"
	webIntegrationLaunchArgumentTemplateConstant = "--web=%d"
)

func TestWebBinaryEmbedsFirstPartyAssetsAndUsesCDNDependencies(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	workingDirectory := testInstance.TempDir()
	listenPort := reserveFreeLoopbackPort(testInstance)
	baseURL := fmt.Sprintf("http://%s:%d", webIntegrationListenHostConstant, listenPort)

	command := exec.Command(binaryPath, fmt.Sprintf(webIntegrationLaunchArgumentTemplateConstant, listenPort))
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
