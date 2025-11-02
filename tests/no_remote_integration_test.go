package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	noRemoteIntegrationTimeout     = 10 * time.Second
	noRemoteIntegrationRunCommand  = "run"
	noRemoteIntegrationModulePath  = "."
	noRemoteIntegrationLogLevelArg = "--log-level"
	noRemoteIntegrationErrorLevel  = "error"
)

func TestBranchCommandsHandleRepositoriesWithoutRemotes(testInstance *testing.T) {
	testInstance.Helper()

	repositoryPath := filepath.Join(testInstance.TempDir(), "no-remote-branch")
	initializeRepositoryWithoutRemote(testInstance, repositoryPath)

	repositoryRoot := integrationRepositoryRoot(testInstance)
	commandArguments := []string{
		noRemoteIntegrationRunCommand,
		noRemoteIntegrationModulePath,
		noRemoteIntegrationLogLevelArg,
		noRemoteIntegrationErrorLevel,
		"branch",
		"cd",
		"master",
		"--roots",
		repositoryPath,
	}

	output := runIntegrationCommand(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{},
		noRemoteIntegrationTimeout,
		commandArguments,
	)
	testInstance.Logf("branch cd output:\n%s", output)

	require.Contains(testInstance, output, "SWITCHED:")
	require.Contains(testInstance, output, "-> master")
	require.NotContains(testInstance, strings.ToLower(output), "failed")
}

func TestWorkflowDefaultBranchHandlesRepositoriesWithoutRemotes(testInstance *testing.T) {
	testInstance.Helper()

	repositoryPath := filepath.Join(testInstance.TempDir(), "no-remote-workflow")
	initializeRepositoryWithoutRemote(testInstance, repositoryPath)

	configPath := filepath.Join(testInstance.TempDir(), "workflow-no-remote.yaml")
	configContents := `workflow:
  - step:
      name: promote-master
      operation: default-branch
      with:
        targets:
          - remote_name: origin
            target_branch: master
            push_to_remote: false
            delete_source_branch: false
`
	require.NoError(testInstance, os.WriteFile(configPath, []byte(configContents), 0o644))

	repositoryRoot := integrationRepositoryRoot(testInstance)
	commandArguments := []string{
		noRemoteIntegrationRunCommand,
		noRemoteIntegrationModulePath,
		noRemoteIntegrationLogLevelArg,
		noRemoteIntegrationErrorLevel,
		"workflow",
		configPath,
		"--roots",
		repositoryPath,
		"--yes",
	}

	output := runIntegrationCommand(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{},
		noRemoteIntegrationTimeout,
		commandArguments,
	)
	testInstance.Logf("workflow output:\n%s", output)

	require.Contains(testInstance, output, "WORKFLOW-DEFAULT:")
	require.Contains(testInstance, output, "(main → master)")
	require.NotContains(testInstance, strings.ToLower(output), "failed")
}

func initializeRepositoryWithoutRemote(testInstance *testing.T, repositoryPath string) {
	testInstance.Helper()

	initCommand := exec.Command("git", "init", "-b", "main", repositoryPath)
	initCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, initCommand.Run())

	configNameCommand := exec.Command("git", "-C", repositoryPath, "config", "user.name", "No Remote Tester")
	configNameCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, configNameCommand.Run())

	configEmailCommand := exec.Command("git", "-C", repositoryPath, "config", "user.email", "noremote@example.com")
	configEmailCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, configEmailCommand.Run())

	readmePath := filepath.Join(repositoryPath, "README.md")
	require.NoError(testInstance, os.WriteFile(readmePath, []byte("no remote\n"), 0o644))

	addCommand := exec.Command("git", "-C", repositoryPath, "add", "README.md")
	addCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, addCommand.Run())

	commitCommand := exec.Command("git", "-C", repositoryPath, "commit", "-m", "initial commit")
	commitCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, commitCommand.Run())
}

func integrationRepositoryRoot(testInstance *testing.T) string {
	testInstance.Helper()
	workingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	return filepath.Dir(workingDirectory)
}
