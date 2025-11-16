package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	flagutils "github.com/temirov/gix/internal/utils/flags"
)

const (
	filesAddIntegrationTimeout        = 10 * time.Second
	filesAddIntegrationLogLevelFlag   = "--log-level"
	filesAddIntegrationLogLevelValue  = "error"
	filesAddIntegrationRunCommand     = "run"
	filesAddIntegrationModulePath     = "."
	filesAddIntegrationAssumeYesFlag  = "--" + flagutils.AssumeYesFlagName
	filesAddIntegrationRootFlag       = "--" + flagutils.DefaultRootFlagName
	filesAddIntegrationPushFlag       = "--push"
	filesAddIntegrationBranchFlag     = "--branch"
	filesAddIntegrationStartPointFlag = "--start-point"
	filesAddIntegrationPathFlag       = "--path"
	filesAddIntegrationContentFlag    = "--content"
	filesAddIntegrationCommitFlag     = "--commit-message"
	filesAddIntegrationPushNo         = "no"
	filesAddIntegrationBranchName     = "automation-docs"
	filesAddIntegrationStartPoint     = "main"
	filesAddIntegrationCommitMessage  = "docs: add policy"
	filesAddIntegrationFilePath       = "docs/POLICY.md"
	filesAddIntegrationUserName       = "Files Add Integration"
	filesAddIntegrationUserEmail      = "files-add@example.com"
)

func TestFilesAddDoesNotPushWhenPushDisabled(testInstance *testing.T) {
	testInstance.Parallel()

	workingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRoot := filepath.Dir(workingDirectory)

	remoteDirectory := filepath.Join(testInstance.TempDir(), "remote.git")
	remoteInit := exec.Command("git", "init", "--bare", "--initial-branch=main", remoteDirectory)
	remoteInit.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, remoteInit.Run())

	repositoryPath := createGitRepository(testInstance, gitRepositoryOptions{RemoteURL: remoteDirectory})

	nameConfig := exec.Command("git", "-C", repositoryPath, "config", "user.name", filesAddIntegrationUserName)
	nameConfig.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, nameConfig.Run())

	emailConfig := exec.Command("git", "-C", repositoryPath, "config", "user.email", filesAddIntegrationUserEmail)
	emailConfig.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, emailConfig.Run())

	initialFilePath := filepath.Join(repositoryPath, "README.md")
	require.NoError(testInstance, os.WriteFile(initialFilePath, []byte("initial"), 0o644))

	addInitial := exec.Command("git", "-C", repositoryPath, "add", "README.md")
	addInitial.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, addInitial.Run())

	commitInitial := exec.Command("git", "-C", repositoryPath, "commit", "-m", "chore: initial")
	commitInitial.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, commitInitial.Run())

	commandArguments := []string{
		filesAddIntegrationRunCommand,
		filesAddIntegrationModulePath,
		filesAddIntegrationLogLevelFlag,
		filesAddIntegrationLogLevelValue,
		"files",
		"add",
		filesAddIntegrationAssumeYesFlag,
		filesAddIntegrationPathFlag,
		filesAddIntegrationFilePath,
		filesAddIntegrationContentFlag,
		"Seed content",
		filesAddIntegrationBranchFlag,
		filesAddIntegrationBranchName,
		filesAddIntegrationStartPointFlag,
		filesAddIntegrationStartPoint,
		filesAddIntegrationPushFlag,
		filesAddIntegrationPushNo,
		filesAddIntegrationCommitFlag,
		filesAddIntegrationCommitMessage,
		filesAddIntegrationRootFlag,
		repositoryPath,
	}

	runIntegrationCommand(testInstance, repositoryRoot, integrationCommandOptions{}, filesAddIntegrationTimeout, commandArguments)

	showFile := exec.Command(
		"git",
		"-C",
		repositoryPath,
		"show",
		fmt.Sprintf("%s:%s", filesAddIntegrationBranchName, filesAddIntegrationFilePath),
	)
	showFile.Env = buildGitCommandEnvironment(nil)
	fileOutput, fileError := showFile.CombinedOutput()
	require.NoError(testInstance, fileError, string(fileOutput))
	require.Equal(testInstance, "Seed content", string(fileOutput))

	branchCheck := exec.Command("git", "-C", repositoryPath, "rev-parse", filesAddIntegrationBranchName)
	branchCheck.Env = buildGitCommandEnvironment(nil)
	branchOutput, branchError := branchCheck.CombinedOutput()
	require.NoError(testInstance, branchError, string(branchOutput))

	remoteBranchCheck := exec.Command("git", "--git-dir", remoteDirectory, "show-ref", "refs/heads/"+filesAddIntegrationBranchName)
	remoteBranchCheck.Env = buildGitCommandEnvironment(nil)
	_, remoteBranchError := remoteBranchCheck.CombinedOutput()
	require.Error(testInstance, remoteBranchError)
}
