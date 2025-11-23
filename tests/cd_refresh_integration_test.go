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
	cdRefreshIntegrationTimeout          = 10 * time.Second
	cdRefreshIntegrationRunCommand       = "run"
	cdRefreshIntegrationModulePath       = "."
	cdRefreshIntegrationLogLevelFlag     = "--log-level"
	cdRefreshIntegrationErrorLogLevel    = "error"
	cdRefreshIntegrationGitInvocationLog = "git-invocations.log"
)

func TestCdSkipsPullWhenWorktreeIsDirty(testInstance *testing.T) {
	testInstance.Helper()

	repositoryRoot := integrationRepositoryRoot(testInstance)
	realGitPath, lookupError := exec.LookPath("git")
	require.NoError(testInstance, lookupError)

	gitInvocationLog := filepath.Join(testInstance.TempDir(), cdRefreshIntegrationGitInvocationLog)
	gitStubScript := []byte(strings.Join([]string{
		"#!/bin/sh",
		"echo \"$@\" >> " + gitInvocationLog,
		"if [ \"$1\" = \"pull\" ]; then exit 42; fi",
		"exec " + realGitPath + " \"$@\"",
	}, "\n") + "\n")
	pathVariable := buildStubbedExecutablePath(testInstance, "git", string(gitStubScript))

	remotePath := filepath.Join(testInstance.TempDir(), "remote.git")
	remoteInitCommand := exec.Command("git", "init", "--bare", remotePath)
	remoteInitCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, remoteInitCommand.Run())

	repositoryPath := filepath.Join(testInstance.TempDir(), "worktree")
	initCommand := exec.Command("git", "init", "--initial-branch=master", repositoryPath)
	initCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, initCommand.Run())

	configNameCommand := exec.Command("git", "-C", repositoryPath, "config", "user.name", "Cd Refresh")
	configNameCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, configNameCommand.Run())

	configEmailCommand := exec.Command("git", "-C", repositoryPath, "config", "user.email", "cd-refresh@example.com")
	configEmailCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, configEmailCommand.Run())

	remoteAddCommand := exec.Command("git", "-C", repositoryPath, "remote", "add", "origin", remotePath)
	remoteAddCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, remoteAddCommand.Run())

	readmePath := filepath.Join(repositoryPath, "README.md")
	require.NoError(testInstance, os.WriteFile(readmePath, []byte("initial\n"), 0o644))

	addCommand := exec.Command("git", "-C", repositoryPath, "add", "README.md")
	addCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, addCommand.Run())

	commitCommand := exec.Command("git", "-C", repositoryPath, "commit", "-m", "initial commit")
	commitCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, commitCommand.Run())

	pushCommand := exec.Command("git", "-C", repositoryPath, "push", "-u", "origin", "master")
	pushCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, pushCommand.Run())

	require.NoError(testInstance, os.WriteFile(readmePath, []byte("modified\n"), 0o644))

	commandArguments := []string{
		cdRefreshIntegrationRunCommand,
		cdRefreshIntegrationModulePath,
		cdRefreshIntegrationLogLevelFlag,
		cdRefreshIntegrationErrorLogLevel,
		"cd",
		"master",
		"--roots",
		repositoryPath,
	}

	output := runIntegrationCommand(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{PathVariable: pathVariable},
		cdRefreshIntegrationTimeout,
		commandArguments,
	)
	testInstance.Logf("cd output:\n%s", output)

	invocationLogContents, readError := os.ReadFile(gitInvocationLog)
	require.NoError(testInstance, readError)
	require.NotContains(testInstance, string(invocationLogContents), "pull")
}
