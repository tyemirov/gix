package tests

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReleaseNextKeepsHistoricalUnreleasedFeatureOutOfBugfixDecision(testInstance *testing.T) {
	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRoot := filepath.Dir(currentWorkingDirectory)
	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)

	var requestsMutex sync.Mutex
	requests := make([]string, 0, 2)
	decisionServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v2" {
			http.NotFound(responseWriter, request)
			return
		}
		requestBody, _ := io.ReadAll(request.Body)
		requestText := string(requestBody)
		requestsMutex.Lock()
		requests = append(requests, requestText)
		requestsMutex.Unlock()
		if strings.Contains(requestText, "Added historical public feature") {
			_, _ = responseWriter.Write([]byte(`{"impact":"additive","public_contract":"historical feature","reason":"The evidence contains an additive feature."}`))
			return
		}
		_, _ = responseWriter.Write([]byte(`{"impact":"compatible","public_contract":"gix audit folder labels","reason":"The release repairs nested checkout labels."}`))
	}))
	testInstance.Cleanup(decisionServer.Close)

	repositoryPath := createGitRepository(testInstance, gitRepositoryOptions{
		DirectoryName: "release-patch-fixture",
		InitialBranch: "master",
	})
	configureGitIdentity(testInstance, repositoryPath)
	require.NoError(testInstance, os.MkdirAll(filepath.Join(repositoryPath, ".mprlab"), 0o755))
	require.NoError(testInstance, os.WriteFile(
		filepath.Join(repositoryPath, ".mprlab", "release.yml"),
		[]byte("schema_version: 1\nscheme: semver\nsemver:\n  fixed_major: 1\n"),
		0o644,
	))
	baselineChangelog := strings.Join([]string{
		"# Changelog",
		"",
		"## [Unreleased]",
		"",
		"### Features",
		"- Added historical public feature.",
		"",
		"### Documentation",
		"- Documented existing command one.",
		"- Documented existing command two.",
		"- Documented existing command three.",
		"- Documented existing command four.",
		"",
		"### Bug Fixes",
		"",
	}, "\n")
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "CHANGELOG.md"), []byte(baselineChangelog), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "audit.go"), []byte("package fixture\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", ".mprlab/release.yml", "CHANGELOG.md", "audit.go")
	runGit(testInstance, repositoryPath, "commit", "-m", "feat: add historical public feature")
	runGit(testInstance, repositoryPath, "tag", "v1.2.0")

	bugfixChangelog := baselineChangelog + "- Preserved containing audit roots for nested linked checkouts.\n"
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "CHANGELOG.md"), []byte(bugfixChangelog), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "audit.go"), []byte("package fixture\n\nconst preservesNestedCheckoutIdentity = true\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "CHANGELOG.md", "audit.go")
	runGit(testInstance, repositoryPath, "commit", "-m", "fix(audit): preserve nested checkout folder identity")

	configurationPath := filepath.Join(testInstance.TempDir(), "config.yml")
	configurationContent := fmt.Sprintf(`common:
  log_level: error
  log_format: console
github:
  credential: ""
llm:
  openai:
    priority: 2
    model: gpt-5.6-terra
    base_url: https://api.openai.com/v1
    credential: ""
  llm_proxy:
    priority: 1
    provider: meta
    model: muse-spark-1.1
    base_url: %q
    credential: proxy-secret
  max_completion_tokens: 1200
  effort: high
  timeout_seconds: 5
operations: []
workflow: []
`, decisionServer.URL)
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))

	outputText, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		repositoryPath,
		map[string]string{},
		20*time.Second,
		[]string{"--config", configurationPath, "release", "next", "--format", "json"},
	)

	require.NoError(testInstance, runError, outputText)
	require.Contains(testInstance, outputText, `"previous_version":"v1.2.0"`)
	require.Contains(testInstance, outputText, `"next_version":"v1.2.1"`)
	require.Contains(testInstance, outputText, `"bump":"patch"`)
	requestsMutex.Lock()
	capturedRequests := strings.Join(requests, "\n")
	capturedRequestCount := len(requests)
	requestsMutex.Unlock()
	require.Equal(testInstance, 2, capturedRequestCount)
	require.Contains(testInstance, capturedRequests, "Preserved containing audit roots")
	require.NotContains(testInstance, capturedRequests, "Added historical public feature")
}
