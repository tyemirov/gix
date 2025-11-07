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

	"github.com/temirov/gix/internal/githubauth"
)

const (
	branchDefaultIntegrationTimeout             = 20 * time.Second
	branchDefaultGitExecutable                  = "git"
	branchDefaultInitialBranch                  = "main"
	branchDefaultTargetBranch                   = "master"
	branchDefaultInitialCommitMessage           = "initial commit"
	branchDefaultWorkflowRelativePath           = ".github/workflows/ci.yml"
	branchDefaultWorkflowTemplate               = "name: CI\non:\n  push:\n    branches:\n      - %s\n"
	branchDefaultStubExecutableName             = "gh"
	branchDefaultGitWrapperExecutableName       = "git"
	branchDefaultParentRemoteRepository         = "example/parent"
	branchDefaultChildRemoteRepository          = "example/remote-child"
	branchDefaultParentRemoteURL                = "https://github.com/" + branchDefaultParentRemoteRepository + ".git"
	branchDefaultChildRemoteURL                 = "https://github.com/" + branchDefaultChildRemoteRepository + ".git"
	branchDefaultGitIgnoreContents              = "tools/\n"
	branchDefaultStubStateDirectoryEnvironment  = "BRANCH_DEFAULT_STATE_DIR"
	branchDefaultStubDefaultBranchPlaceholder   = "main"
	branchDefaultUserName                       = "Branch Default Tester"
	branchDefaultUserEmail                      = "branch-default@example.com"
	branchDefaultWorkflowCommitMessageTemplate  = "add workflow for %s"
	branchDefaultGitWrapperRealBinaryEnv        = "BRANCH_DEFAULT_REAL_GIT"
	branchDefaultWorkflowRewriteCommitSubstring = "CI: switch workflow branch filters to"
)

func TestBranchDefaultHandlesNestedRepositoriesWithMixedRemotes(testInstance *testing.T) {
	testInstance.Helper()

	workspaceDirectory := testInstance.TempDir()
	parentRepositoryPath := filepath.Join(workspaceDirectory, "parent-remote")
	remoteChildPath := filepath.Join(parentRepositoryPath, "tools", "remote-child")
	localChildPath := filepath.Join(parentRepositoryPath, "tools", "local-child")

	initializeRepositoryWithFiles(
		testInstance,
		parentRepositoryPath,
		branchDefaultParentRemoteURL,
		map[string]string{
			"README.md":                       "parent repository\n",
			".gitignore":                      branchDefaultGitIgnoreContents,
			branchDefaultWorkflowRelativePath: fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch),
		},
	)

	initializeRepositoryWithFiles(
		testInstance,
		remoteChildPath,
		branchDefaultChildRemoteURL,
		map[string]string{
			"README.md":                       filepath.Base(remoteChildPath) + "\n",
			branchDefaultWorkflowRelativePath: fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch),
		},
	)

	initializeRepositoryWithFiles(
		testInstance,
		localChildPath,
		"",
		map[string]string{
			"README.md":                       filepath.Base(localChildPath) + "\n",
			branchDefaultWorkflowRelativePath: fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch),
		},
	)

	stateDirectory := testInstance.TempDir()
	initializeStubStateFile(testInstance, stateDirectory, branchDefaultParentRemoteRepository, branchDefaultInitialBranch)
	initializeStubStateFile(testInstance, stateDirectory, branchDefaultChildRemoteRepository, branchDefaultInitialBranch)
	testInstance.Logf("state directory: %s", stateDirectory)

	stubDirectory := filepath.Join(testInstance.TempDir(), "bin")
	require.NoError(testInstance, os.MkdirAll(stubDirectory, 0o755))
	testInstance.Logf("stub directory: %s", stubDirectory)

	githubStubPath := filepath.Join(stubDirectory, branchDefaultStubExecutableName)
	require.NoError(testInstance, os.WriteFile(githubStubPath, []byte(buildBranchDefaultStubScript(stateDirectory)), 0o755))

	realGitBinary, lookupError := exec.LookPath(branchDefaultGitExecutable)
	require.NoError(testInstance, lookupError)

	gitWrapperPath := filepath.Join(stubDirectory, branchDefaultGitWrapperExecutableName)
	require.NoError(testInstance, os.WriteFile(gitWrapperPath, []byte(buildBranchDefaultGitWrapper(realGitBinary, stateDirectory)), 0o755))

	repositoryRoot := integrationRepositoryRoot(testInstance)
	commandArguments := []string{
		"run",
		".",
		"--log-level",
		"error",
		"default",
		branchDefaultTargetBranch,
		"--roots",
		parentRepositoryPath,
		"--yes",
	}

	extendedPath := stubDirectory + string(os.PathListSeparator) + os.Getenv(pathEnvironmentVariableNameConstant)
	commandOptions := integrationCommandOptions{
		PathVariable: extendedPath,
		EnvironmentOverrides: map[string]string{
			branchDefaultStubStateDirectoryEnvironment: stateDirectory,
			githubauth.EnvGitHubToken:                  "test-token",
			githubauth.EnvGitHubCLIToken:               "test-token",
			githubauth.EnvGitHubAPIToken:               "test-token",
		},
	}

	output := runIntegrationCommand(
		testInstance,
		repositoryRoot,
		commandOptions,
		branchDefaultIntegrationTimeout,
		commandArguments,
	)
	testInstance.Logf("default output:\n%s", output)

	require.Contains(testInstance, output, fmt.Sprintf("WORKFLOW-DEFAULT: %s (main → master)", parentRepositoryPath))
	require.NotContains(testInstance, output, remoteChildPath)
	require.NotContains(testInstance, output, localChildPath)
	require.NotContains(testInstance, strings.ToLower(output), "default branch update failed")

	assertRepositoryHead(testInstance, parentRepositoryPath, branchDefaultTargetBranch)
	assertRepositoryHead(testInstance, remoteChildPath, branchDefaultInitialBranch)
	assertRepositoryHead(testInstance, localChildPath, branchDefaultInitialBranch)

	assertStateFileBranch(testInstance, stateDirectory, branchDefaultParentRemoteRepository, branchDefaultTargetBranch)
	assertStateFileBranch(testInstance, stateDirectory, branchDefaultChildRemoteRepository, branchDefaultInitialBranch)
}

func initializeRepositoryWithFiles(testInstance *testing.T, repositoryPath string, remoteURL string, files map[string]string) {
	testInstance.Helper()

	require.NoError(testInstance, os.MkdirAll(repositoryPath, 0o755))

	initCommand := exec.Command(branchDefaultGitExecutable, "init", "-b", branchDefaultInitialBranch, repositoryPath)
	initCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, initCommand.Run())

	configNameCommand := exec.Command(branchDefaultGitExecutable, "-C", repositoryPath, "config", "user.name", branchDefaultUserName)
	configNameCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, configNameCommand.Run())

	configEmailCommand := exec.Command(branchDefaultGitExecutable, "-C", repositoryPath, "config", "user.email", branchDefaultUserEmail)
	configEmailCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, configEmailCommand.Run())

	for relativePath, contents := range files {
		fullPath := filepath.Join(repositoryPath, relativePath)
		parentDirectory := filepath.Dir(fullPath)
		require.NoError(testInstance, os.MkdirAll(parentDirectory, 0o755))
		require.NoError(testInstance, os.WriteFile(fullPath, []byte(contents), 0o644))
	}

	addCommand := exec.Command(branchDefaultGitExecutable, "-C", repositoryPath, "add", ".")
	addCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, addCommand.Run())

	commitMessage := branchDefaultInitialCommitMessage
	if remoteURL == "" {
		commitMessage = fmt.Sprintf(branchDefaultWorkflowCommitMessageTemplate, filepath.Base(repositoryPath))
	}

	commitCommand := exec.Command(branchDefaultGitExecutable, "-C", repositoryPath, "commit", "-m", commitMessage)
	commitCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, commitCommand.Run())

	if len(strings.TrimSpace(remoteURL)) == 0 {
		return
	}

	remoteAddCommand := exec.Command(branchDefaultGitExecutable, "-C", repositoryPath, "remote", "add", "origin", remoteURL)
	remoteAddCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, remoteAddCommand.Run())
}

func buildBranchDefaultStubScript(stateDirectory string) string {
	return fmt.Sprintf(`#!/bin/sh
STATE_DIR=%[1]q
DEFAULT_BRANCH=%[2]q

state_path() {
  repo="$1"
  key=$(echo "$repo" | sed 's#/#__#g')
  echo "$STATE_DIR/$key.txt"
}

log_command() {
  printf '%%s\n' "$*" >>"$STATE_DIR/gh.log"
}

log_command "$@"

ensure_state() {
  repo="$1"
  path=$(state_path "$repo")
  if [ ! -f "$path" ]; then
    printf '%%s\n' "$DEFAULT_BRANCH" >"$path"
  fi
}

read_state() {
  repo="$1"
  ensure_state "$repo"
  path=$(state_path "$repo")
  tr -d '\n' <"$path"
}

write_state() {
  repo="$1"
  branch="$2"
  path=$(state_path "$repo")
  printf '%%s\n' "$branch" >"$path"
}

if [ "$1" = "repo" ] && [ "$2" = "view" ]; then
  repo="$3"
  default_branch=$(read_state "$repo")
  printf '{"nameWithOwner":"%%s","defaultBranchRef":{"name":"%%s"},"description":""}\n' "$repo" "$default_branch"
  exit 0
fi

if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
  echo '[]'
  exit 0
fi

if [ "$1" = "pr" ] && [ "$2" = "edit" ]; then
  exit 0
fi

if [ "$1" = "api" ]; then
  endpoint="$2"
  method="$4"

  case "$endpoint" in
    repos/*/pages)
      if [ "$method" = "GET" ]; then
        echo '{"build_type":"legacy","source":{"branch":"main","path":"/"}}'
        exit 0
      fi
      if [ "$method" = "PUT" ]; then
        exit 0
      fi
      ;;
    repos/*/branches/*/protection)
      echo 'gh: Not Found (HTTP 404)' >&2
      exit 1
      ;;
    repos/*)
      repo=${endpoint#repos/}
      writeBranch="$method"
      if [ "$writeBranch" = "PATCH" ]; then
        for argument in "$@"; do
          case "$argument" in
            default_branch=*)
              write_state "$repo" "${argument#default_branch=}"
              ;;
          esac
        done
      fi
      exit 0
      ;;
  esac
fi

exit 0
`, stateDirectory, branchDefaultStubDefaultBranchPlaceholder)
}

func buildBranchDefaultGitWrapper(realGitPath string, stateDirectory string) string {
	return fmt.Sprintf(`#!/bin/sh
REAL_GIT=%q
STATE_DIR=%q
printf '%%s\n' "$@" >>"$STATE_DIR/git.log"
if [ "$1" = "ls-remote" ]; then
  exit 0
fi
if [ "$1" = "push" ]; then
  exit 0
fi
exec "$REAL_GIT" "$@"
`, realGitPath, stateDirectory)
}

func initializeStubStateFile(testInstance *testing.T, stateDirectory string, repository string, branch string) {
	testInstance.Helper()
	key := strings.ReplaceAll(repository, "/", "__")
	statePath := filepath.Join(stateDirectory, key+".txt")
	require.NoError(testInstance, os.WriteFile(statePath, []byte(branch+"\n"), 0o644))
}

func assertRepositoryHead(testInstance *testing.T, repositoryPath string, expectedBranch string) {
	testInstance.Helper()
	command := exec.Command(branchDefaultGitExecutable, "-C", repositoryPath, "rev-parse", "--abbrev-ref", "HEAD")
	command.Env = buildGitCommandEnvironment(nil)
	output, err := command.CombinedOutput()
	require.NoError(testInstance, err, string(output))
	require.Equal(testInstance, expectedBranch, strings.TrimSpace(string(output)))
}

func assertStateFileBranch(testInstance *testing.T, stateDirectory string, repository string, expectedBranch string) {
	testInstance.Helper()
	key := strings.ReplaceAll(repository, "/", "__")
	statePath := filepath.Join(stateDirectory, key+".txt")
	content, readError := os.ReadFile(statePath)
	require.NoError(testInstance, readError)
	require.Equal(testInstance, expectedBranch, strings.TrimSpace(string(content)))
}
