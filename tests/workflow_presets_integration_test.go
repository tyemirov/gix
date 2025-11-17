package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	workflowPresetLogLevelFlag = "--log-level"
	workflowPresetErrorLevel   = "error"
)

func TestWorkflowPresetRemoteUpdateProtocolIntegration(testInstance *testing.T) {
	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRoot := filepath.Dir(currentWorkingDirectory)

	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	repositoryPath := createGitRepository(testInstance, gitRepositoryOptions{
		RemoteURL: "https://github.com/canonical/example.git",
	})

	commandArguments := []string{
		"workflow",
		"remote-update-protocol",
		workflowPresetLogLevelFlag,
		workflowPresetErrorLevel,
		"--var",
		"from=https",
		"--var",
		"to=ssh",
		"--roots",
		repositoryPath,
		"--yes",
	}

	outputText, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		repositoryRoot,
		nil,
		integrationCommandTimeout,
		commandArguments,
	)
	require.NoError(testInstance, runError, outputText)

	remoteCommand := exec.Command("git", "-C", repositoryPath, "remote", "get-url", "origin")
	remoteCommand.Env = buildGitCommandEnvironment(nil)
	remoteOutput, remoteError := remoteCommand.CombinedOutput()
	require.NoError(testInstance, remoteError, string(remoteOutput))
	require.Equal(testInstance, "ssh://git@github.com/canonical/example.git", strings.TrimSpace(string(remoteOutput)))
}

func TestWorkflowPresetHistoryRemoveIntegration(testInstance *testing.T) {
	currentWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRoot := filepath.Dir(currentWorkingDirectory)

	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	remotePath := filepath.Join(testInstance.TempDir(), "history-remote.git")
	initializeRemote := exec.Command("git", "init", "--bare", remotePath)
	initializeRemote.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, initializeRemote.Run())

	repositoryPath := createGitRepository(testInstance, gitRepositoryOptions{RemoteURL: remotePath})

	configureGit := exec.Command("git", "-C", repositoryPath, "config", "user.name", "History Preset")
	configureGit.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, configureGit.Run())

	configureEmail := exec.Command("git", "-C", repositoryPath, "config", "user.email", "history@example.com")
	configureEmail.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, configureEmail.Run())

	createFile := exec.Command("git", "-C", repositoryPath, "commit", "--allow-empty", "-m", "initial")
	createFile.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, createFile.Run())

	commandArguments := []string{
		"workflow",
		"history-remove",
		workflowPresetLogLevelFlag,
		workflowPresetErrorLevel,
		"--var",
		"paths=missing.txt",
		"--var",
		"push=false",
		"--var",
		"restore=false",
		"--var",
		"remote=origin",
		"--roots",
		repositoryPath,
		"--yes",
	}

	outputText, runError := runBinaryIntegrationCommand(
		testInstance,
		binaryPath,
		repositoryRoot,
		nil,
		integrationCommandTimeout,
		commandArguments,
	)
	require.NoError(testInstance, runError, outputText)
	require.Contains(testInstance, outputText, "HISTORY-SKIP")
}
