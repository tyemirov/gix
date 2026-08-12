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
	runGit(testInstance, repositoryPath, "add", "CHANGELOG.md", "audit.go")
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

	missingPolicyOutput, missingPolicyError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		repositoryPath,
		map[string]string{},
		20*time.Second,
		[]string{"--config", configurationPath, "release", "next", "--format", "json"},
	)
	require.Error(testInstance, missingPolicyError, missingPolicyOutput)
	require.Contains(testInstance, missingPolicyOutput, "release policy must be semver or calver")

	outputText, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		repositoryPath,
		map[string]string{},
		20*time.Second,
		[]string{"--config", configurationPath, "release", "next", "semver", "--fixed-major", "1", "--format", "json"},
	)

	require.NoError(testInstance, runError, outputText)
	require.Contains(testInstance, outputText, `"previous_version":"v1.2.0"`)
	require.Contains(testInstance, outputText, `"next_version":"v1.2.1"`)
	require.Contains(testInstance, outputText, `"bump":"patch"`)
	require.Contains(testInstance, outputText, `"contract":"mprlab.version-decision/v2"`)
	require.Contains(testInstance, outputText, `"policy":{"scheme":"semver","fixed_major":1}`)
	calVerOutput, calVerError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		repositoryPath,
		map[string]string{},
		20*time.Second,
		[]string{"--config", configurationPath, "release", "next", "calver", "--release-timestamp", "2026-08-10T12:34:56Z", "--format", "json"},
	)
	require.NoError(testInstance, calVerError, calVerOutput)
	require.Contains(testInstance, calVerOutput, `"contract":"mprlab.version-decision/v2"`)
	require.Contains(testInstance, calVerOutput, `"policy":{"scheme":"calver"}`)
	require.Contains(testInstance, calVerOutput, `"next_version":"26.810.123456"`)
	requestsMutex.Lock()
	capturedRequests := strings.Join(requests, "\n")
	capturedRequestCount := len(requests)
	requestsMutex.Unlock()
	require.Equal(testInstance, 2, capturedRequestCount)
	require.Contains(testInstance, capturedRequests, "Preserved containing audit roots")
	require.NotContains(testInstance, capturedRequests, "Added historical public feature")
}

func TestReleaseNextKeepsRequiredRepairInputsOnPatchLine(testInstance *testing.T) {
	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRoot := filepath.Dir(currentWorkingDirectory)
	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)

	const repairClassificationRule = "Treat required inputs that only enable a compatible repair as part of the repair, not as optional new functionality."
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
		if strings.Contains(requestText, repairClassificationRule) {
			_, _ = responseWriter.Write([]byte(`{"impact":"compatible","public_contract":"gix release next artifact successor","reason":"The release repairs artifact successor selection."}`))
			return
		}
		_, _ = responseWriter.Write([]byte(`{"impact":"additive","public_contract":"gix release next output identity flags","reason":"The release adds optional output identity flags."}`))
	}))
	testInstance.Cleanup(decisionServer.Close)

	repositoryPath := createGitRepository(testInstance, gitRepositoryOptions{
		DirectoryName: "release-repair-input-fixture",
		InitialBranch: "master",
	})
	configureGitIdentity(testInstance, repositoryPath)
	require.NoError(testInstance, os.MkdirAll(filepath.Join(repositoryPath, ".mprlab"), 0o755))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "release.go"), []byte("package fixture\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("The release command selects the next SemVer version.\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, ".mprlab", "ISSUES.md"), []byte("# ISSUES\n\n## BugFixes\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", ".")
	runGit(testInstance, repositoryPath, "commit", "-m", "fixture release baseline")
	runGit(testInstance, repositoryPath, "tag", "v1.4.0")

	repairSource := strings.Join([]string{
		"package fixture",
		"",
		"var previousReleaseOutput string",
		"var candidateReleaseOutput string",
		"",
	}, "\n")
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "release.go"), []byte(repairSource), 0o644))
	repairDocumentation := strings.Join([]string{
		"The release command selects the next SemVer version.",
		"Artifact producers provide previous and candidate output identities for a same-commit patch successor.",
		"",
	}, "\n")
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte(repairDocumentation), 0o644))
	repairIssue := strings.Join([]string{
		"# ISSUES",
		"",
		"## BugFixes",
		"",
		"- [x] [B063] An artifact successor rejects an empty SemVer range.",
		"  Goal:",
		"  Select one patch successor when a release system changes sealed outputs for the same source commit.",
		"  Requirements:",
		"  - Accept exact previous and candidate output identities at the CLI boundary.",
		"  - Select a patch successor without an LLM request.",
		"",
	}, "\n")
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, ".mprlab", "ISSUES.md"), []byte(repairIssue), 0o644))
	runGit(testInstance, repositoryPath, "add", ".")
	runGit(testInstance, repositoryPath, "commit", "-m", "fix(release): select same-commit artifact successors")

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
		[]string{"--config", configurationPath, "release", "next", "semver", "--fixed-major", "1", "--format", "json"},
	)

	require.NoError(testInstance, runError, outputText)
	require.Contains(testInstance, outputText, `"previous_version":"v1.4.0"`)
	require.Contains(testInstance, outputText, `"next_version":"v1.4.1"`)
	require.Contains(testInstance, outputText, `"bump":"patch"`)
	requestsMutex.Lock()
	capturedRequests := strings.Join(requests, "\n")
	capturedRequestCount := len(requests)
	requestsMutex.Unlock()
	require.Equal(testInstance, 2, capturedRequestCount)
	require.Contains(testInstance, capturedRequests, repairClassificationRule)
	require.Contains(testInstance, capturedRequests, "Select one patch successor")
}

func TestReleaseNextSelectsSameCommitArtifactSuccessor(testInstance *testing.T) {
	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRoot := filepath.Dir(currentWorkingDirectory)
	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)

	repositoryPath := createGitRepository(testInstance, gitRepositoryOptions{
		DirectoryName: "release-artifact-successor-fixture",
		InitialBranch: "master",
	})
	configureGitIdentity(testInstance, repositoryPath)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "application.txt"), []byte("sealed source\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "application.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "fixture release source")
	runGit(testInstance, repositoryPath, "tag", "v1.2.3")

	configurationPath := filepath.Join(testInstance.TempDir(), "config.yml")
	configurationContent := `common:
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
    base_url: http://127.0.0.1:1
    credential: unused
  max_completion_tokens: 1200
  effort: high
  timeout_seconds: 5
operations: []
workflow: []
`
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))

	ordinaryOutput, ordinaryError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		repositoryPath,
		map[string]string{},
		20*time.Second,
		[]string{"--config", configurationPath, "release", "next", "semver", "--format", "json"},
	)
	require.Error(testInstance, ordinaryError, ordinaryOutput)
	require.Contains(testInstance, ordinaryOutput, "semver decision range contains no committed changes")

	previousOutput := "sha256:" + strings.Repeat("a", 64)
	candidateOutput := "sha256:" + strings.Repeat("b", 64)
	outputText, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		repositoryPath,
		map[string]string{},
		20*time.Second,
		[]string{
			"--config", configurationPath,
			"release", "next", "semver",
			"--format", "json",
			"--previous-release-output", previousOutput,
			"--candidate-release-output", candidateOutput,
		},
	)

	require.NoError(testInstance, runError, outputText)
	require.Contains(testInstance, outputText, `"previous_version":"v1.2.3"`)
	require.Contains(testInstance, outputText, `"next_version":"v1.2.4"`)
	require.Contains(testInstance, outputText, `"bump":"patch"`)
	require.Contains(testInstance, outputText, `"deterministic_floor":"patch"`)
	require.Contains(testInstance, outputText, "The sealed release output changed for the same source commit")

	runGit(testInstance, repositoryPath, "tag", "--delete", "v1.2.3")
	untaggedOutput, untaggedError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		repositoryPath,
		map[string]string{},
		20*time.Second,
		[]string{
			"--config", configurationPath,
			"release", "next", "semver",
			"--format", "json",
			"--previous-release-output", previousOutput,
			"--candidate-release-output", candidateOutput,
		},
	)
	require.Error(testInstance, untaggedError, untaggedOutput)
	require.Contains(testInstance, untaggedOutput, "release output transition requires the latest SemVer tag at the source commit")
}
