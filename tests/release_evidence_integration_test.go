package tests

import (
	"encoding/json"
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

func TestReleaseNextAnalyzesCompleteLargeDiff(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	var mutex sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, readError := io.ReadAll(request.Body)
		require.NoError(testInstance, readError)
		mutex.Lock()
		requests = append(requests, string(body))
		mutex.Unlock()
		if len(body) > 96*1024 {
			writer.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		if strings.Contains(string(body), "LATE_CONTRACT_REMOVAL") {
			_, _ = io.WriteString(writer, `{"impact":"incompatible","public_contract":"supported text command","reason":"The release removes a supported command."}`)
			return
		}
		_, _ = io.WriteString(writer, `{"impact":"compatible","public_contract":"","reason":"This packet contains compatible maintenance."}`)
	}))
	defer server.Close()
	repository := releaseEvidenceRepository(testInstance)
	noise := strings.Repeat("Routine maintenance preserves the supported public behavior.\n", 24000)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "a-maintenance.txt"), []byte(noise), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "z-command.txt"), []byte("LATE_CONTRACT_REMOVAL: reject the previously supported text command.\n"), 0o644))
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "fix: update behavior and maintenance")
	configuration := releaseEvidenceConfiguration(testInstance, server.URL)
	output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
		[]string{"--config", configuration, "release", "next", "semver", "--format", "json"})
	require.NoError(testInstance, runError, output)
	require.Contains(testInstance, output, `"next_version":"v2.0.0"`)
	require.Contains(testInstance, output, `"bump":"major"`)
	mutex.Lock()
	captured := append([]string(nil), requests...)
	mutex.Unlock()
	require.Greater(testInstance, len(captured), 2)
	for _, request := range captured {
		require.LessOrEqual(testInstance, len(request), 96*1024)
	}
	record := releaseEvidenceRecord(testInstance, repository)
	require.Contains(testInstance, string(record), "LATE_CONTRACT_REMOVAL")
	require.Contains(testInstance, string(record), "v2.0.0")
	require.NotContains(testInstance, string(record), "proxy-secret")
}

func releaseEvidenceRepository(testInstance *testing.T) string {
	testInstance.Helper()
	repository := createGitRepository(testInstance, gitRepositoryOptions{DirectoryName: "release-evidence-fixture", InitialBranch: "master"})
	configureGitIdentity(testInstance, repository)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "z-command.txt"), []byte("The text command is supported.\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "behavior.txt"), []byte("OLD_SUPPORTED_BEHAVIOR: existing consumers may use the text command.\n"), 0o644))
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "fixture baseline")
	runGit(testInstance, repository, "tag", "v1.2.3")
	return repository
}

func TestReleaseNextRetrievesCommittedContext(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, readError := io.ReadAll(request.Body)
		require.NoError(testInstance, readError)
		if !strings.Contains(string(body), "OLD_SUPPORTED_BEHAVIOR") {
			_, _ = io.WriteString(writer, `{"impact":"unresolved","public_contract":"text command","reason":"The previous supported behavior needs source context.","context_requests":[{"path":"behavior.txt","source":"old","start_line":1,"line_count":1}]}`)
			return
		}
		_, _ = io.WriteString(writer, `{"impact":"incompatible","public_contract":"text command","reason":"The release removes the command used by existing consumers."}`)
	}))
	defer server.Close()
	repository := releaseEvidenceRepository(testInstance)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "z-command.txt"), []byte("REQUEST_CONTEXT_INSPECTION: the text command is removed.\n"), 0o644))
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "change command behavior")
	// A working copy edit must never become evidence for the committed release.
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "behavior.txt"), []byte("UNCOMMITTED_BEHAVIOR_MUST_NOT_APPEAR\n"), 0o644))
	configuration := releaseEvidenceConfiguration(testInstance, server.URL)
	output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
		[]string{"--config", configuration, "release", "next", "semver", "--format", "json"})
	require.NoError(testInstance, runError, output)
	require.Contains(testInstance, output, `"next_version":"v2.0.0"`)
	record := string(releaseEvidenceRecord(testInstance, repository))
	require.Contains(testInstance, record, "OLD_SUPPORTED_BEHAVIOR")
	require.NotContains(testInstance, record, "UNCOMMITTED_BEHAVIOR_MUST_NOT_APPEAR")
}

func TestReleaseNextRetainsFailedEvidence(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	cases := []struct {
		name, response, expected string
		status                   int
	}{
		{name: "unresolved", response: `{"impact":"unresolved","public_contract":"text command","reason":"The supported behavior remains unresolved."}`, expected: "unresolved"},
		{name: "unsafe_context", response: `{"impact":"unresolved","public_contract":"text command","reason":"More context is needed.","context_requests":[{"path":"../outside.txt","source":"old","start_line":1,"line_count":1}]}`, expected: "context"},
		{name: "provider", status: http.StatusInternalServerError, response: "controlled provider failure", expected: "candidate.llm"},
	}
	for _, scenario := range cases {
		testInstance.Run(scenario.name, func(testInstance *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if scenario.status != 0 {
					writer.WriteHeader(scenario.status)
				}
				_, _ = io.WriteString(writer, scenario.response)
			}))
			defer server.Close()
			repository := releaseEvidenceRepository(testInstance)
			require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "z-command.txt"), []byte("The command behavior changes.\n"), 0o644))
			runGit(testInstance, repository, "add", ".")
			runGit(testInstance, repository, "commit", "-m", "change command behavior")
			configuration := releaseEvidenceConfiguration(testInstance, server.URL)
			output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
				[]string{"--config", configuration, "release", "next", "semver", "--format", "json"})
			require.Error(testInstance, runError, output)
			require.Contains(testInstance, output, scenario.expected)
			require.NotContains(testInstance, output, `"next_version"`)
			var record map[string]json.RawMessage
			require.NoError(testInstance, json.Unmarshal(releaseEvidenceRecord(testInstance, repository), &record))
			require.Contains(testInstance, string(record["failure"]), scenario.expected)
		})
	}
}

func TestReleaseNextStopsWhenEvidenceCannotBeSaved(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, `{"impact":"compatible","public_contract":"","reason":"The release preserves supported behavior."}`)
	}))
	defer server.Close()
	repository := releaseEvidenceRepository(testInstance)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "z-command.txt"), []byte("The supported command has a compatible repair.\n"), 0o644))
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "fix command behavior")
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, ".git", "gix"), []byte("An existing file prevents evidence directory creation.\n"), 0o644))
	configuration := releaseEvidenceConfiguration(testInstance, server.URL)
	output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
		[]string{"--config", configuration, "release", "next", "semver", "--format", "json"})
	require.Error(testInstance, runError, output)
	require.Contains(testInstance, output, "create release evidence directory")
	require.NotContains(testInstance, output, `"next_version"`)
}

func TestReleaseNextStopsWhenCompleteEvidenceExceedsBudget(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	var mutex sync.Mutex
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		calls++
		mutex.Unlock()
		_, _ = io.WriteString(writer, `{"impact":"compatible","public_contract":"","reason":"The release preserves supported behavior."}`)
	}))
	defer server.Close()
	repository := releaseEvidenceRepository(testInstance)
	noise := strings.Repeat("Routine maintenance preserves the supported public behavior.\n", 60000)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "a-maintenance.txt"), []byte(noise), 0o644))
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "update maintenance text")
	configuration := releaseEvidenceConfiguration(testInstance, server.URL)
	output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
		[]string{"--config", configuration, "release", "next", "semver", "--format", "json"})
	require.Error(testInstance, runError, output)
	require.Contains(testInstance, output, "provider call limit")
	require.NotContains(testInstance, output, `"next_version"`)
	mutex.Lock()
	actualCalls := calls
	mutex.Unlock()
	require.Zero(testInstance, actualCalls)
	require.Contains(testInstance, string(releaseEvidenceRecord(testInstance, repository)), "provider call limit")
}

func releaseEvidenceConfiguration(testInstance *testing.T, serverURL string) string {
	testInstance.Helper()
	configuration := filepath.Join(testInstance.TempDir(), "config.yml")
	content := fmt.Sprintf(`common:
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
`, serverURL)
	require.NoError(testInstance, os.WriteFile(configuration, []byte(content), 0o600))
	return configuration
}

func releaseEvidenceRecord(testInstance *testing.T, repository string) []byte {
	testInstance.Helper()
	records, err := filepath.Glob(filepath.Join(repository, ".git", "gix", "release-decisions", "*.json"))
	require.NoError(testInstance, err)
	require.Len(testInstance, records, 1)
	content, err := os.ReadFile(records[0])
	require.NoError(testInstance, err)
	require.True(testInstance, json.Valid(content))
	return content
}
