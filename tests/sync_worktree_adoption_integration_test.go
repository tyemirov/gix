package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	syncWorktreeAdoptionTimeout        = 20 * time.Second
	syncWorktreeAdoptionBranchName     = "feature/adopt-worktree"
	syncWorktreeAdoptionAPIKeyVariable = "TEST_GIX_LLM_KEY"
	syncWorktreeAdoptionMissingGitHub  = "strict sync requires a GitHub repository remote"
)

type syncWorktreeAdoptionFixture struct {
	RemotePath     string
	RepositoryPath string
	SiblingPath    string
	BranchName     string
}

func TestSyncRejectsDirtySiblingWorktreeWithoutGitHubPullRequest(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	fixture := createSyncWorktreeAdoptionFixture(testInstance)

	dirtyPath := filepath.Join(fixture.SiblingPath, "feature.txt")
	require.NoError(testInstance, os.WriteFile(dirtyPath, []byte("dirty sibling change\n"), 0o644))

	configurationPath := writeSyncWorktreeAdoptionConfiguration(testInstance, "")
	output, runError := runFailingIntegrationCommand(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{},
		syncWorktreeAdoptionTimeout,
		[]string{"run", ".", "--config", configurationPath, "sync", fixture.BranchName, "--roots", fixture.RepositoryPath},
	)
	require.Error(testInstance, runError)

	require.Contains(testInstance, output, syncWorktreeAdoptionMissingGitHub)
	require.DirExists(testInstance, fixture.SiblingPath)
	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "branch", "--show-current")))
	require.NoFileExists(testInstance, filepath.Join(fixture.RepositoryPath, "feature.txt"))
}

func TestSyncRejectsCleanAheadSiblingWorktreeWithoutGitHubPullRequest(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	fixture := createSyncWorktreeAdoptionFixture(testInstance)

	aheadPath := filepath.Join(fixture.SiblingPath, "ahead.txt")
	require.NoError(testInstance, os.WriteFile(aheadPath, []byte("already committed locally\n"), 0o644))
	runGit(testInstance, fixture.SiblingPath, "add", "ahead.txt")
	runGit(testInstance, fixture.SiblingPath, "commit", "-m", "chore: local sibling commit")
	runGit(testInstance, fixture.SiblingPath, "branch", "--unset-upstream")
	require.NotContains(testInstance, runGit(testInstance, fixture.SiblingPath, "status", "--porcelain", "--branch"), "ahead")

	configurationPath := writeSyncWorktreeAdoptionConfiguration(testInstance, "")
	output, runError := runFailingIntegrationCommand(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{},
		syncWorktreeAdoptionTimeout,
		[]string{"run", ".", "--config", configurationPath, "sync", fixture.BranchName, "--roots", fixture.RepositoryPath},
	)
	require.Error(testInstance, runError)

	require.Contains(testInstance, output, syncWorktreeAdoptionMissingGitHub)
	require.DirExists(testInstance, fixture.SiblingPath)
	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "branch", "--show-current")))
	require.NoFileExists(testInstance, filepath.Join(fixture.RepositoryPath, "ahead.txt"))
}

func TestSyncExplicitMasterPrunesStaleLinkedWorktreeBeforeSwitch(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := testInstance.TempDir()
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "repository")
	siblingPath := filepath.Join(workspacePath, "stale-master")

	runGitWithDir(testInstance, "", "init", "--bare", remotePath)
	runGitWithDir(testInstance, "", "init", "--initial-branch=master", repositoryPath)
	configureGitIdentity(testInstance, repositoryPath)
	runGit(testInstance, repositoryPath, "remote", "add", "origin", remotePath)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", "feature/current-work")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "feature/current-work")
	runGit(testInstance, repositoryPath, "worktree", "add", siblingPath, "master")
	canonicalSiblingPath, canonicalSiblingPathErr := filepath.EvalSymlinks(siblingPath)
	require.NoError(testInstance, canonicalSiblingPathErr)
	require.NoError(testInstance, os.RemoveAll(siblingPath))

	staleWorktreeList := runGit(testInstance, repositoryPath, "worktree", "list", "--porcelain")
	require.Contains(testInstance, staleWorktreeList, "worktree "+canonicalSiblingPath)
	require.Contains(testInstance, staleWorktreeList, "prunable")

	configurationPath := writeSyncWorktreeAdoptionConfiguration(testInstance, "")
	output := runIntegrationCommand(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{},
		syncWorktreeAdoptionTimeout,
		[]string{"run", ".", "--config", configurationPath, "sync", "master", "--roots", repositoryPath},
	)

	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (master)", repositoryPath))
	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	require.NotContains(testInstance, runGit(testInstance, repositoryPath, "worktree", "list", "--porcelain"), "worktree "+canonicalSiblingPath)
}

func createSyncWorktreeAdoptionFixture(testInstance *testing.T) syncWorktreeAdoptionFixture {
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

	runGit(testInstance, repositoryPath, "switch", "-c", syncWorktreeAdoptionBranchName)
	featurePath := filepath.Join(repositoryPath, "feature.txt")
	require.NoError(testInstance, os.WriteFile(featurePath, []byte("feature base\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "feature.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "feature base")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", syncWorktreeAdoptionBranchName)
	runGit(testInstance, repositoryPath, "switch", "master")

	siblingPath := filepath.Join(workspacePath, "repository-feature")
	runGit(testInstance, repositoryPath, "worktree", "add", siblingPath, syncWorktreeAdoptionBranchName)

	return syncWorktreeAdoptionFixture{
		RemotePath:     remotePath,
		RepositoryPath: repositoryPath,
		SiblingPath:    siblingPath,
		BranchName:     syncWorktreeAdoptionBranchName,
	}
}

func writeSyncWorktreeAdoptionConfiguration(testInstance *testing.T, baseURL string) string {
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
`, syncWorktreeAdoptionAPIKeyVariable, baseURL)
	}
	configurationContent := fmt.Sprintf(`common:
  log_level: error
  log_format: console
operations:
  - command: ["sync"]
    with:
      remote: origin
      require_clean: true
%s`, messageConfiguration)
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))
	return configurationPath
}

func configureGitIdentity(testInstance *testing.T, repositoryPath string) {
	testInstance.Helper()
	runGit(testInstance, repositoryPath, "config", "user.name", "Sync Worktree")
	runGit(testInstance, repositoryPath, "config", "user.email", "sync-worktree@example.com")
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
