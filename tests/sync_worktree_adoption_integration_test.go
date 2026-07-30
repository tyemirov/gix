package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func TestSyncRepairsStaleLinkedWorktreeAfterPrimaryRepositoryMove(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	workspacePath := testInstance.TempDir()
	remotePath := filepath.Join(workspacePath, "remote.git")
	originalRepositoryPath := filepath.Join(workspacePath, "story-generator")
	relocatedRepositoryPath := filepath.Join(workspacePath, "TellTale")
	siblingPath := filepath.Join(workspacePath, "story-generator-b007")
	siblingBranch := "feature/sibling"

	runGitWithDir(testInstance, "", "init", "--bare", remotePath)
	runGitWithDir(testInstance, "", "init", "--initial-branch=master", originalRepositoryPath)
	configureGitIdentity(testInstance, originalRepositoryPath)
	runGit(testInstance, originalRepositoryPath, "remote", "add", "origin", remotePath)
	require.NoError(testInstance, os.WriteFile(filepath.Join(originalRepositoryPath, "README.md"), []byte("initial\n"), 0o644))
	runGit(testInstance, originalRepositoryPath, "add", "README.md")
	runGit(testInstance, originalRepositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, originalRepositoryPath, "push", "-u", "origin", "master")
	runGit(testInstance, originalRepositoryPath, "branch", siblingBranch)
	runGit(testInstance, originalRepositoryPath, "worktree", "add", siblingPath, siblingBranch)

	siblingReadmePath := filepath.Join(siblingPath, "README.md")
	require.NoError(testInstance, os.WriteFile(siblingReadmePath, []byte("initial\nstaged sibling change\n"), 0o644))
	runGit(testInstance, siblingPath, "add", "README.md")
	require.NoError(testInstance, os.WriteFile(siblingReadmePath, []byte("initial\nstaged sibling change\nunstaged sibling change\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(siblingPath, "untracked-sibling.txt"), []byte("untracked sibling change\n"), 0o644))

	expectedSiblingBranch := strings.TrimSpace(runGit(testInstance, siblingPath, "branch", "--show-current"))
	expectedSiblingHead := strings.TrimSpace(runGit(testInstance, siblingPath, "rev-parse", "HEAD"))
	expectedSiblingIndex := runGit(testInstance, siblingPath, "ls-files", "--stage", "-z")
	expectedSiblingStatus := runGit(testInstance, siblingPath, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	expectedSiblingFiles := snapshotSyncFiles(testInstance, siblingPath)
	canonicalOriginalRepositoryPath, canonicalOriginalErr := filepath.EvalSymlinks(originalRepositoryPath)
	require.NoError(testInstance, canonicalOriginalErr)
	canonicalSiblingPath, canonicalSiblingErr := filepath.EvalSymlinks(siblingPath)
	require.NoError(testInstance, canonicalSiblingErr)

	require.NoError(testInstance, os.Rename(originalRepositoryPath, relocatedRepositoryPath))
	canonicalRelocatedRepositoryPath, canonicalRelocatedErr := filepath.EvalSymlinks(relocatedRepositoryPath)
	require.NoError(testInstance, canonicalRelocatedErr)

	staleGitFileContents, staleGitFileErr := os.ReadFile(filepath.Join(siblingPath, ".git"))
	require.NoError(testInstance, staleGitFileErr)
	require.Contains(testInstance, string(staleGitFileContents), filepath.Join(canonicalOriginalRepositoryPath, ".git", "worktrees"))

	worktreeList := runGit(testInstance, relocatedRepositoryPath, "worktree", "list", "--porcelain")
	require.Contains(testInstance, worktreeList, "worktree "+canonicalSiblingPath)
	require.NotContains(testInstance, worktreeList, "prunable")

	brokenProbe := exec.Command("git", "-C", siblingPath, "rev-parse", "--git-path", "MERGE_HEAD")
	brokenProbe.Env = buildGitCommandEnvironment(nil)
	brokenProbeOutput, brokenProbeErr := brokenProbe.CombinedOutput()
	require.Error(testInstance, brokenProbeErr, string(brokenProbeOutput))

	configurationPath := writeSyncWorktreeAdoptionConfiguration(testInstance, "")
	output, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		relocatedRepositoryPath,
		nil,
		syncWorktreeAdoptionTimeout,
		[]string{"--config", configurationPath, "sync", "master", "--roots", relocatedRepositoryPath},
	)
	require.NoError(testInstance, runError, output)
	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (master)", relocatedRepositoryPath))

	repairedGitFileContents, repairedGitFileErr := os.ReadFile(filepath.Join(siblingPath, ".git"))
	require.NoError(testInstance, repairedGitFileErr)
	require.Contains(testInstance, string(repairedGitFileContents), filepath.Join(canonicalRelocatedRepositoryPath, ".git", "worktrees"))
	require.Equal(testInstance, expectedSiblingBranch, strings.TrimSpace(runGit(testInstance, siblingPath, "branch", "--show-current")))
	require.Equal(testInstance, expectedSiblingHead, strings.TrimSpace(runGit(testInstance, siblingPath, "rev-parse", "HEAD")))
	require.Equal(testInstance, expectedSiblingIndex, runGit(testInstance, siblingPath, "ls-files", "--stage", "-z"))
	require.Equal(testInstance, expectedSiblingStatus, runGit(testInstance, siblingPath, "status", "--porcelain=v1", "-z", "--untracked-files=all"))
	require.Equal(testInstance, expectedSiblingFiles, snapshotSyncFiles(testInstance, siblingPath))
}

func TestSyncRejectsCopiedPrimaryWithoutTakingOverLiveLinkedWorktree(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	workspacePath := testInstance.TempDir()
	remotePath := filepath.Join(workspacePath, "remote.git")
	originalRepositoryPath := filepath.Join(workspacePath, "original")
	copiedRepositoryPath := filepath.Join(workspacePath, "copied")
	siblingPath := filepath.Join(workspacePath, "linked")
	siblingBranch := "feature/live-owner"

	runGitWithDir(testInstance, "", "init", "--bare", remotePath)
	runGitWithDir(testInstance, "", "init", "--initial-branch=master", originalRepositoryPath)
	configureGitIdentity(testInstance, originalRepositoryPath)
	runGit(testInstance, originalRepositoryPath, "remote", "add", "origin", remotePath)
	require.NoError(testInstance, os.WriteFile(filepath.Join(originalRepositoryPath, "README.md"), []byte("initial\n"), 0o644))
	runGit(testInstance, originalRepositoryPath, "add", "README.md")
	runGit(testInstance, originalRepositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, originalRepositoryPath, "push", "-u", "origin", "master")
	runGit(testInstance, originalRepositoryPath, "branch", siblingBranch)
	runGit(testInstance, originalRepositoryPath, "worktree", "add", siblingPath, siblingBranch)

	siblingReadmePath := filepath.Join(siblingPath, "README.md")
	require.NoError(testInstance, os.WriteFile(siblingReadmePath, []byte("initial\nlive sibling change\n"), 0o644))
	runGit(testInstance, siblingPath, "add", "README.md")
	require.NoError(testInstance, os.WriteFile(filepath.Join(siblingPath, "untracked-sibling.txt"), []byte("live untracked change\n"), 0o644))

	expectedSiblingBranch := strings.TrimSpace(runGit(testInstance, siblingPath, "branch", "--show-current"))
	expectedSiblingHead := strings.TrimSpace(runGit(testInstance, siblingPath, "rev-parse", "HEAD"))
	expectedSiblingIndex := runGit(testInstance, siblingPath, "ls-files", "--stage", "-z")
	expectedSiblingStatus := runGit(testInstance, siblingPath, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	expectedSiblingFiles := snapshotSyncFiles(testInstance, siblingPath)
	expectedSiblingGitFile, expectedSiblingGitFileErr := os.ReadFile(filepath.Join(siblingPath, ".git"))
	require.NoError(testInstance, expectedSiblingGitFileErr)
	expectedOriginalWorktrees := runGit(testInstance, originalRepositoryPath, "worktree", "list", "--porcelain")

	require.NoError(testInstance, os.CopyFS(copiedRepositoryPath, os.DirFS(originalRepositoryPath)))
	expectedCopiedWorktrees := runGit(testInstance, copiedRepositoryPath, "worktree", "list", "--porcelain")
	canonicalOriginalRepositoryPath, canonicalOriginalErr := filepath.EvalSymlinks(originalRepositoryPath)
	require.NoError(testInstance, canonicalOriginalErr)
	canonicalSiblingPath, canonicalSiblingErr := filepath.EvalSymlinks(siblingPath)
	require.NoError(testInstance, canonicalSiblingErr)

	configurationPath := writeSyncWorktreeAdoptionConfiguration(testInstance, "")
	output, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		copiedRepositoryPath,
		nil,
		syncWorktreeAdoptionTimeout,
		[]string{"--config", configurationPath, "sync", "master", "--roots", copiedRepositoryPath},
	)

	require.Error(testInstance, runError, output)
	require.Contains(testInstance, output, "belongs to live common repository")
	require.Contains(testInstance, output, canonicalSiblingPath)
	require.Contains(testInstance, output, filepath.Join(canonicalOriginalRepositoryPath, ".git"))
	require.Contains(testInstance, output, filepath.Join(copiedRepositoryPath, ".git"))
	require.Contains(testInstance, output, "remove the conflicting registration explicitly")
	require.NotContains(testInstance, output, "SYNCED:")

	actualSiblingGitFile, actualSiblingGitFileErr := os.ReadFile(filepath.Join(siblingPath, ".git"))
	require.NoError(testInstance, actualSiblingGitFileErr)
	require.Equal(testInstance, expectedSiblingGitFile, actualSiblingGitFile)
	require.Equal(testInstance, expectedOriginalWorktrees, runGit(testInstance, originalRepositoryPath, "worktree", "list", "--porcelain"))
	require.Equal(testInstance, expectedCopiedWorktrees, runGit(testInstance, copiedRepositoryPath, "worktree", "list", "--porcelain"))
	require.Equal(testInstance, expectedSiblingBranch, strings.TrimSpace(runGit(testInstance, siblingPath, "branch", "--show-current")))
	require.Equal(testInstance, expectedSiblingHead, strings.TrimSpace(runGit(testInstance, siblingPath, "rev-parse", "HEAD")))
	require.Equal(testInstance, expectedSiblingIndex, runGit(testInstance, siblingPath, "ls-files", "--stage", "-z"))
	require.Equal(testInstance, expectedSiblingStatus, runGit(testInstance, siblingPath, "status", "--porcelain=v1", "-z", "--untracked-files=all"))
	require.Equal(testInstance, expectedSiblingFiles, snapshotSyncFiles(testInstance, siblingPath))
}

func TestSyncAdoptsSiblingWorktreeWithReadOnlyIgnoredCache(testInstance *testing.T) {
	if runtime.GOOS == "windows" {
		testInstance.Skip("read-only directory removal requires Unix permission semantics")
	}

	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	siblingPath := filepath.Join(workspacePath, "project-read-only-cache")
	branchName := "feature/read-only-cache"
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, ".gitignore"), []byte(".cache/\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md", ".gitignore")
	runGit(testInstance, repositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", branchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "feature.txt"), []byte("feature work\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "feature.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "feature work")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", branchName)
	runGit(testInstance, repositoryPath, "switch", "master")
	runGit(testInstance, repositoryPath, "worktree", "add", siblingPath, branchName)

	cacheDirectory := filepath.Join(siblingPath, ".cache", "go", "pkg", "mod", "read-only-module")
	require.NoError(testInstance, os.MkdirAll(cacheDirectory, 0o755))
	require.NoError(testInstance, os.WriteFile(filepath.Join(cacheDirectory, "cached-file"), []byte("cache\n"), 0o444))
	testInstance.Cleanup(func() {
		_ = os.Chmod(cacheDirectory, 0o755)
	})
	require.NoError(testInstance, os.Chmod(cacheDirectory, 0o555))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, siblingPath, "status", "--porcelain")))

	configurationPath := writeSyncMergedBranchConfiguration(testInstance)
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)
	output := runIntegrationCommand(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncMergedBranchGitHubLogVariable: githubLogPath,
				syncMergedBranchMergedVariable:    "false",
				syncMergedBranchNameVariable:      branchName,
			},
		},
		syncWorktreeAdoptionTimeout,
		[]string{"run", ".", "--config", configurationPath, "--log-level", "error", "sync", branchName, "--body", "Adopt the read-only cache worktree.", "--roots", repositoryPath},
	)
	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (%s)", repositoryPath, branchName))
	require.NoDirExists(testInstance, siblingPath)
	require.NotContains(testInstance, runGit(testInstance, repositoryPath, "worktree", "list", "--porcelain"), "worktree "+siblingPath)
	require.Equal(testInstance, branchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
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

	configurationPath := filepath.Join(testInstance.TempDir(), "config.yml")
	llmConfiguration := `llm:
  openai:
    priority: 1
    model: gpt-4.1
    base_url: https://api.openai.com/v1
    credential: integration-openai-key
  llm_proxy:
    priority: 2
    provider: meta
    model: muse-spark-1.1
    base_url: "https://llm-proxy.example"
    credential: integration-proxy-key
`
	messageConfiguration := ""
	if strings.TrimSpace(baseURL) != "" {
		llmConfiguration = fmt.Sprintf(`llm:
  openai:
    priority: 1
    model: mock-model
    base_url: %q
    credential: test-key
  llm_proxy:
    priority: 2
    provider: meta
    model: muse-spark-1.1
    base_url: "https://llm-proxy.example"
    credential: test-proxy-key
  max_completion_tokens: 64
  temperature: 0
  timeout_seconds: 5
`, baseURL)
		messageConfiguration = `
  - command: ["message", "commit"]
    with:
      diff_source: staged
      max_completion_tokens: 64
      temperature: 0
      timeout_seconds: 5
`
	}
	configurationContent := fmt.Sprintf(`common:
  log_level: error
  log_format: console
github:
  credential: test-github-key
%soperations:
  - command: ["sync"]
    with:
      remote: origin
      require_clean: true
%s`, llmConfiguration, messageConfiguration)
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
