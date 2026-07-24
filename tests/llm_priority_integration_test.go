package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMessageCommitUsesLLMConnectionPriorityAndFailover(testInstance *testing.T) {
	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRootDirectory := filepath.Dir(currentWorkingDirectory)
	binaryPath := buildIntegrationBinary(testInstance, repositoryRootDirectory)

	testCases := []struct {
		name                   string
		openAIPriority         int
		llmProxyPriority       int
		expectedOpenAIAttempts bool
	}{
		{
			name:                   "openai_failure_uses_llm_proxy",
			openAIPriority:         1,
			llmProxyPriority:       2,
			expectedOpenAIAttempts: true,
		},
		{
			name:                   "llm_proxy_success_stops_before_openai",
			openAIPriority:         2,
			llmProxyPriority:       1,
			expectedOpenAIAttempts: false,
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			var openAIAttempts atomic.Int32
			openAIServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				openAIAttempts.Add(1)
				http.Error(responseWriter, "openai unavailable", http.StatusServiceUnavailable)
			}))
			subtest.Cleanup(openAIServer.Close)

			var llmProxyAttempts atomic.Int32
			var capturedProxyProvider string
			var capturedProxyProviderMutex sync.Mutex
			llmProxyServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/v2" {
					http.NotFound(responseWriter, request)
					return
				}
				llmProxyAttempts.Add(1)
				capturedProxyProviderMutex.Lock()
				capturedProxyProvider = request.URL.Query().Get("provider")
				capturedProxyProviderMutex.Unlock()
				_, _ = responseWriter.Write([]byte("docs: use prioritized llm"))
			}))
			subtest.Cleanup(llmProxyServer.Close)

			repositoryPath := createGitRepository(subtest, gitRepositoryOptions{
				DirectoryName: "priority-fixture",
				InitialBranch: "main",
			})
			require.NoError(subtest, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("priority\n"), 0o644))
			addCommand := exec.Command("git", "-C", repositoryPath, "add", "README.md")
			addCommand.Env = buildGitCommandEnvironment(nil)
			addOutput, addError := addCommand.CombinedOutput()
			require.NoError(subtest, addError, string(addOutput))

			configurationPath := filepath.Join(subtest.TempDir(), "config.yml")
			configurationContent := fmt.Sprintf(`common:
  log_level: error
  log_format: console
llm:
  openai:
    priority: %d
    model: gpt-4.1
    base_url: %q
    credential: openai-secret
  llm_proxy:
    priority: %d
    provider: meta
    model: muse-spark-1.1
    base_url: %q
    credential: proxy-secret
  max_completion_tokens: 64
  temperature: 0
  timeout_seconds: 2
operations:
  - command: ["message", "commit"]
    with:
      roots:
        - %q
      diff_source: staged
`, testCase.openAIPriority, openAIServer.URL, testCase.llmProxyPriority, llmProxyServer.URL, repositoryPath)
			require.NoError(subtest, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))

			outputText, runError := runBinaryIntegrationCommand(
				subtest,
				binaryPath,
				repositoryPath,
				map[string]string{},
				10*time.Second,
				[]string{"--config", configurationPath, "message", "commit"},
			)

			require.NoError(subtest, runError, outputText)
			require.Equal(subtest, int32(1), llmProxyAttempts.Load())
			if testCase.expectedOpenAIAttempts {
				require.Positive(subtest, openAIAttempts.Load())
			} else {
				require.Zero(subtest, openAIAttempts.Load())
			}
			capturedProxyProviderMutex.Lock()
			require.Equal(subtest, "meta", capturedProxyProvider)
			capturedProxyProviderMutex.Unlock()
		})
	}
}
