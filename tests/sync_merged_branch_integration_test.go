package tests

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	syncMergedBranchIntegrationTimeout = 20 * time.Second
	syncMergedBranchOwnerRepository    = "owner/project"
	syncMergedBranchRemoteURL          = "https://github.com/" + syncMergedBranchOwnerRepository + ".git"
	syncMergedBranchGitHubLogVariable  = "GIX_SYNC_TEST_GH_LOG"
	syncMergedBranchNameVariable       = "GIX_SYNC_TEST_BRANCH"
	syncMergedBranchMergedVariable     = "GIX_SYNC_TEST_MERGED"
)

type syncMergedBranchFixture struct {
	RemotePath     string
	RootPath       string
	RepositoryPath string
	BranchName     string
}

func TestSyncCurrentMergedBranchPromptsAndSyncsMasterBeforeCreatingPullRequest(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	branchName := "feature/squashed-review"
	fixture := createSyncMergedBranchFixture(testInstance, branchName)
	configurationPath := writeSyncMergedBranchConfiguration(testInstance)
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncMergedBranchGitHubLogVariable: githubLogPath,
				syncMergedBranchNameVariable:      branchName,
			},
		},
		syncMergedBranchIntegrationTimeout,
		"y\n",
		[]string{"run", ".", "--config", configurationPath, "--log-level", "error", "sync", "--roots", fixture.RepositoryPath},
	)
	require.NoError(testInstance, runError, output)

	require.Contains(testInstance, output, `Pull request for branch "feature/squashed-review" into master is already merged. Sync master instead?`)
	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (master)", fixture.RepositoryPath))
	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, fixture.RepositoryPath, "branch", "--show-current")))
	require.Equal(testInstance, "squashed review\n", readTextFile(testInstance, filepath.Join(fixture.RepositoryPath, "feature.txt")))

	githubLog := readTextFile(testInstance, githubLogPath)
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state open --head feature/squashed-review")
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state merged --base master --head feature/squashed-review")
	require.NotContains(testInstance, githubLog, "pr create")
}

func TestSyncExplicitMasterReleasesMainWorktreeAndSwitchesLinkedWorktree(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	mainRepositoryPath := filepath.Join(workspacePath, "main", "project")
	linkedRootPath := filepath.Join(workspacePath, "linked")
	linkedWorktreePath := filepath.Join(linkedRootPath, "project-linked")
	branchName := "feature/linked-sync"
	createSyncGitHubBackedRepository(testInstance, remotePath, mainRepositoryPath)

	require.NoError(testInstance, os.WriteFile(filepath.Join(mainRepositoryPath, "README.md"), []byte("initial\n"), 0o644))
	runGit(testInstance, mainRepositoryPath, "add", "README.md")
	runGit(testInstance, mainRepositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, mainRepositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, mainRepositoryPath, "switch", "-c", branchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(mainRepositoryPath, "feature.txt"), []byte("feature work\n"), 0o644))
	runGit(testInstance, mainRepositoryPath, "add", "feature.txt")
	runGit(testInstance, mainRepositoryPath, "commit", "-m", "feature work")
	runGit(testInstance, mainRepositoryPath, "push", "-u", "origin", branchName)
	runGit(testInstance, mainRepositoryPath, "switch", "master")
	require.NoError(testInstance, os.MkdirAll(linkedRootPath, 0o755))
	runGit(testInstance, mainRepositoryPath, "worktree", "add", linkedWorktreePath, branchName)

	configurationPath := writeSyncMergedBranchConfiguration(testInstance)
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncMergedBranchGitHubLogVariable: githubLogPath,
				syncMergedBranchNameVariable:      branchName,
			},
		},
		syncMergedBranchIntegrationTimeout,
		"",
		[]string{"run", ".", "--config", configurationPath, "--log-level", "error", "sync", "master", "--roots", linkedWorktreePath},
	)
	require.NoError(testInstance, runError, output)

	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (master)", linkedWorktreePath))
	require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, linkedWorktreePath, "branch", "--show-current")))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, mainRepositoryPath, "branch", "--show-current")))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, mainRepositoryPath, "status", "--porcelain")))
	require.NotContains(testInstance, output, "main working tree")
}

func TestSyncExistingRemoteBranchWithoutPullRequestCreatesPullRequest(testInstance *testing.T) {
	repositoryRoot := integrationRepositoryRoot(testInstance)
	branchName := "feature/unreviewed-remote"
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", branchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "feature.txt"), []byte("needs review\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "feature.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "feature needs review")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", branchName)
	runGit(testInstance, repositoryPath, "switch", "master")

	configurationPath := writeSyncMergedBranchConfiguration(testInstance)
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	pathVariable := buildSyncMergedBranchExecutablePath(testInstance)
	pullRequestBody := "Publish the existing remote branch for review."

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: pathVariable,
			EnvironmentOverrides: map[string]string{
				syncMergedBranchGitHubLogVariable: githubLogPath,
				syncMergedBranchNameVariable:      branchName,
				syncMergedBranchMergedVariable:    "false",
			},
		},
		syncMergedBranchIntegrationTimeout,
		"",
		[]string{"run", ".", "--config", configurationPath, "--log-level", "error", "sync", branchName, "--body", pullRequestBody, "--roots", repositoryPath},
	)
	require.NoError(testInstance, runError, output)

	require.Contains(testInstance, output, fmt.Sprintf("SYNCED: %s (%s)", repositoryPath, branchName))
	require.Equal(testInstance, branchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
	githubLog := readTextFile(testInstance, githubLogPath)
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state open --head feature/unreviewed-remote")
	require.Contains(testInstance, githubLog, "pr list --repo owner/project --state merged --base master --head feature/unreviewed-remote")
	require.Contains(testInstance, githubLog, "pr create --repo owner/project --base master --head feature/unreviewed-remote --title feature/unreviewed-remote --body Publish the existing remote branch for review.")
}

func createSyncMergedBranchFixture(testInstance *testing.T, branchName string) syncMergedBranchFixture {
	testInstance.Helper()

	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryRootPath := filepath.Join(workspacePath, "roots")
	repositoryPath := filepath.Join(repositoryRootPath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "initial commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", branchName)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "feature.txt"), []byte("squashed review\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "feature.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "feature branch commit")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", branchName)

	runGit(testInstance, repositoryPath, "switch", "master")
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "feature.txt"), []byte("squashed review\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "feature.txt")
	runGit(testInstance, repositoryPath, "commit", "-m", "squash merged feature")
	runGit(testInstance, repositoryPath, "push", "origin", "master")
	runGit(testInstance, repositoryPath, "switch", branchName)

	return syncMergedBranchFixture{
		RemotePath:     remotePath,
		RootPath:       repositoryRootPath,
		RepositoryPath: repositoryPath,
		BranchName:     branchName,
	}
}

func createSyncGitHubBackedRepository(testInstance *testing.T, remotePath string, repositoryPath string) {
	testInstance.Helper()

	require.NoError(testInstance, os.MkdirAll(filepath.Dir(remotePath), 0o755))
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(repositoryPath), 0o755))
	runGitWithDir(testInstance, "", "init", "--bare", remotePath)
	runGitWithDir(testInstance, "", "init", "--initial-branch=master", repositoryPath)
	configureGitIdentity(testInstance, repositoryPath)
	runGit(testInstance, repositoryPath, "remote", "add", "origin", localFileURL(remotePath))
}

func writeSyncMergedBranchConfiguration(testInstance *testing.T) string {
	testInstance.Helper()

	configurationPath := filepath.Join(testInstance.TempDir(), "config.yaml")
	configurationContent := `common:
  log_level: error
  log_format: console
operations:
  - command: ["sync"]
    with:
      remote: origin
      require_clean: true
`
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))
	return configurationPath
}

func syncMergedBranchGitHubStubScript() string {
	return `#!/bin/sh
printf '%s\n' "$*" >>"$GIX_SYNC_TEST_GH_LOG"

if [ "$1" = "repo" ] && [ "$2" = "view" ]; then
  printf '%s\n' '{"nameWithOwner":"owner/project","description":"","defaultBranchRef":{"name":"master"},"isInOrganization":false}'
  exit 0
fi

if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
  state=""
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --state)
        state="$2"
        shift
        ;;
    esac
    shift
  done
  if [ "$state" = "open" ]; then
    printf '%s\n' '[]'
    exit 0
  fi
  if [ "$state" = "merged" ]; then
    if [ "$GIX_SYNC_TEST_MERGED" = "false" ]; then
      printf '%s\n' '[]'
      exit 0
    fi
    printf '[{"number":9,"title":"Merged","headRefName":"%s","baseRefName":"master"}]\n' "$GIX_SYNC_TEST_BRANCH"
    exit 0
  fi
fi

if [ "$1" = "pr" ] && [ "$2" = "create" ]; then
  printf '%s\n' 'https://github.com/owner/project/pull/10'
  exit 0
fi

printf 'unexpected gh invocation: %s\n' "$*" >&2
exit 1
`
}

func buildSyncMergedBranchExecutablePath(testInstance *testing.T) string {
	testInstance.Helper()

	stubDirectory := testInstance.TempDir()
	realGitPath, lookupError := exec.LookPath("git")
	require.NoError(testInstance, lookupError)

	gitStubScript := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "remote" ] && [ "$2" = "get-url" ] && [ "$3" = "origin" ]; then
  printf '%%s\n' %q
  exit 0
fi
exec %q "$@"
`, syncMergedBranchRemoteURL, realGitPath)
	require.NoError(testInstance, os.WriteFile(filepath.Join(stubDirectory, "git"), []byte(gitStubScript), 0o755))
	require.NoError(testInstance, os.WriteFile(filepath.Join(stubDirectory, "gh"), []byte(syncMergedBranchGitHubStubScript()), 0o755))

	currentPath := os.Getenv(pathEnvironmentVariableNameConstant)
	if currentPath == "" {
		return stubDirectory
	}
	return stubDirectory + string(os.PathListSeparator) + currentPath
}

func runIntegrationCommandWithInput(testInstance *testing.T, repositoryRoot string, options integrationCommandOptions, timeout time.Duration, input string, arguments []string) (string, error) {
	testInstance.Helper()

	executionContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	command := exec.CommandContext(executionContext, "go", arguments...)
	command.Dir = repositoryRoot
	command.Env = buildCommandEnvironment(options)
	command.Stdin = strings.NewReader(input)

	outputBytes, runError := command.CombinedOutput()
	return string(outputBytes), runError
}

func localFileURL(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func syncHomeWorkspace(testInstance *testing.T) string {
	testInstance.Helper()
	homeDirectory, homeError := os.UserHomeDir()
	require.NoError(testInstance, homeError)
	workspacePath, workspaceError := os.MkdirTemp(homeDirectory, "gix-sync-merged-branch-")
	require.NoError(testInstance, workspaceError)
	testInstance.Cleanup(func() {
		_ = os.RemoveAll(workspacePath)
	})
	canonicalPath, canonicalError := filepath.EvalSymlinks(workspacePath)
	require.NoError(testInstance, canonicalError)
	return canonicalPath
}

func readTextFile(testInstance *testing.T, path string) string {
	testInstance.Helper()
	contents, readError := os.ReadFile(path)
	require.NoError(testInstance, readError)
	return string(contents)
}
