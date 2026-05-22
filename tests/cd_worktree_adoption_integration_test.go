package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	cdWorktreeAdoptionTimeout        = 20 * time.Second
	cdWorktreeAdoptionBranchName     = "feature/adopt-worktree"
	cdWorktreeAdoptionCommitSubject  = "chore: save sibling changes"
	cdWorktreeAdoptionAPIKeyVariable = "TEST_GIX_LLM_KEY"
)

type cdWorktreeAdoptionFixture struct {
	RemotePath     string
	RepositoryPath string
	SiblingPath    string
	BranchName     string
}

func TestCdAdoptsDirtySiblingWorktree(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	fixture := createCdWorktreeAdoptionFixture(testInstance)
	serverRequestCount := 0
	mockLLMServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		serverRequestCount++
		require.Equal(testInstance, "/chat/completions", request.URL.Path)
		require.Equal(testInstance, "Bearer test-api-key", request.Header.Get("Authorization"))
		responseWriter.Header().Set("Content-Type", "application/json")
		_, writeError := responseWriter.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"` + cdWorktreeAdoptionCommitSubject + `"},"finish_reason":"stop"}]}`))
		require.NoError(testInstance, writeError)
	}))
	defer mockLLMServer.Close()

	dirtyPath := filepath.Join(fixture.SiblingPath, "feature.txt")
	require.NoError(testInstance, os.WriteFile(dirtyPath, []byte("dirty sibling change\n"), 0o644))

	configurationPath := writeCdWorktreeAdoptionConfiguration(testInstance, mockLLMServer.URL)
	output := runIntegrationCommand(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{EnvironmentOverrides: map[string]string{cdWorktreeAdoptionAPIKeyVariable: "test-api-key"}},
		cdWorktreeAdoptionTimeout,
		[]string{"run", ".", "--config", configurationPath, "cd", fixture.BranchName, "--roots", fixture.RepositoryPath},
	)

	require.Contains(testInstance, output, "WORKTREE_ADOPT")
	require.Equal(testInstance, 1, serverRequestCount)
	require.NoDirExists(testInstance, fixture.SiblingPath)
	require.Equal(testInstance, fixture.BranchName, strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, "dirty sibling change\n", readFile(testInstance, filepath.Join(fixture.RepositoryPath, "feature.txt")))
	require.Equal(testInstance, cdWorktreeAdoptionCommitSubject, strings.TrimSpace(runGitWithDir(testInstance, "", "--git-dir", fixture.RemotePath, "log", "-1", "--pretty=%s", "refs/heads/"+fixture.BranchName)))
	require.Equal(testInstance, "0", strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "rev-list", "--count", "origin/"+fixture.BranchName+".."+fixture.BranchName)))
}

func TestCdAdoptsCleanAheadSiblingWorktree(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	fixture := createCdWorktreeAdoptionFixture(testInstance)

	aheadPath := filepath.Join(fixture.SiblingPath, "ahead.txt")
	require.NoError(testInstance, os.WriteFile(aheadPath, []byte("already committed locally\n"), 0o644))
	runGit(testInstance, fixture.SiblingPath, "add", "ahead.txt")
	runGit(testInstance, fixture.SiblingPath, "commit", "-m", "chore: local sibling commit")
	runGit(testInstance, fixture.SiblingPath, "branch", "--unset-upstream")
	require.NotContains(testInstance, runGit(testInstance, fixture.SiblingPath, "status", "--porcelain", "--branch"), "ahead")

	configurationPath := writeCdWorktreeAdoptionConfiguration(testInstance, "")
	output := runIntegrationCommand(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{},
		cdWorktreeAdoptionTimeout,
		[]string{"run", ".", "--config", configurationPath, "cd", fixture.BranchName, "--roots", fixture.RepositoryPath},
	)

	require.Contains(testInstance, output, "WORKTREE_ADOPT")
	require.NoDirExists(testInstance, fixture.SiblingPath)
	require.Equal(testInstance, fixture.BranchName, strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, "chore: local sibling commit", strings.TrimSpace(runGitWithDir(testInstance, "", "--git-dir", fixture.RemotePath, "log", "-1", "--pretty=%s", "refs/heads/"+fixture.BranchName)))
	require.Equal(testInstance, "0", strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "rev-list", "--count", "origin/"+fixture.BranchName+".."+fixture.BranchName)))
}

func createCdWorktreeAdoptionFixture(testInstance *testing.T) cdWorktreeAdoptionFixture {
	testInstance.Helper()

	workspacePath := testInstance.TempDir()
	remotePath := filepath.Join(workspacePath, "remote.git")
	runGitWithDir(testInstance, "", "init", "--bare", remotePath)

	repositoryPath := filepath.Join(workspacePath, "repository")
	runGitWithDir(testInstance, "", "init", "--initial-branch=master", repositoryPath)
	configureGitIdentity(testInstance, repositoryPath)
	runGit(testInstance, repositoryPath, "remote", "add", "origin", remotePath)

	readmePath := filepath.Join(repositoryPath, "README.md")
	require.NoError(testInstance, os.WriteFile(readmePath, []byte("initial\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", cdWorktreeAdoptionBranchName)
	featurePath := filepath.Join(repositoryPath, "feature.txt")
	require.NoError(testInstance, os.WriteFile(featurePath, []byte("feature base\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "feature.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "feature base")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", cdWorktreeAdoptionBranchName)
	runGit(testInstance, repositoryPath, "switch", "master")

	siblingPath := filepath.Join(workspacePath, "repository-feature")
	runGit(testInstance, repositoryPath, "worktree", "add", siblingPath, cdWorktreeAdoptionBranchName)

	return cdWorktreeAdoptionFixture{
		RemotePath:     remotePath,
		RepositoryPath: repositoryPath,
		SiblingPath:    siblingPath,
		BranchName:     cdWorktreeAdoptionBranchName,
	}
}

func writeCdWorktreeAdoptionConfiguration(testInstance *testing.T, baseURL string) string {
	testInstance.Helper()

	configurationPath := filepath.Join(testInstance.TempDir(), "config.yaml")
	messageConfiguration := ""
	if strings.TrimSpace(baseURL) != "" {
		messageConfiguration = fmt.Sprintf(`
  - command: ["message", "commit"]
    with:
      api_key_env: %s
      base_url: %q
      model: mock-model
      diff_source: staged
      max_completion_tokens: 64
      temperature: 0
      timeout_seconds: 5
`, cdWorktreeAdoptionAPIKeyVariable, baseURL)
	}
	configurationContent := fmt.Sprintf(`common:
  log_level: error
  log_format: console
operations:
  - command: ["cd"]
    with:
      remote: origin
      require_clean: true
%s`, messageConfiguration)
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))
	return configurationPath
}

func configureGitIdentity(testInstance *testing.T, repositoryPath string) {
	testInstance.Helper()
	runGit(testInstance, repositoryPath, "config", "user.name", "Cd Worktree")
	runGit(testInstance, repositoryPath, "config", "user.email", "cd-worktree@example.com")
}

func runGit(testInstance *testing.T, repositoryPath string, arguments ...string) string {
	testInstance.Helper()
	return runGitWithDir(testInstance, repositoryPath, append([]string{"-C", repositoryPath}, arguments...)...)
}

func runGitWithDir(testInstance *testing.T, workingDirectory string, arguments ...string) string {
	testInstance.Helper()
	command := exec.Command("git", arguments...)
	if strings.TrimSpace(workingDirectory) != "" {
		command.Dir = workingDirectory
	}
	command.Env = buildGitCommandEnvironment(nil)
	outputBytes, commandError := command.CombinedOutput()
	require.NoError(testInstance, commandError, string(outputBytes))
	return string(outputBytes)
}

func readFile(testInstance *testing.T, path string) string {
	testInstance.Helper()
	content, readError := os.ReadFile(path)
	require.NoError(testInstance, readError)
	return string(content)
}
