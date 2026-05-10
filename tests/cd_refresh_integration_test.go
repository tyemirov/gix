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

func TestCdFastForwardPullsWhenWorktreeHasUnrelatedTrackedChanges(testInstance *testing.T) {
	testInstance.Helper()

	repositoryRoot := integrationRepositoryRoot(testInstance)
	realGitPath, lookupError := exec.LookPath("git")
	require.NoError(testInstance, lookupError)

	gitInvocationLog := filepath.Join(testInstance.TempDir(), cdRefreshIntegrationGitInvocationLog)
	gitStubScript := []byte(strings.Join([]string{
		"#!/bin/sh",
		"echo \"$@\" >> " + gitInvocationLog,
		"if [ \"$1\" = \"pull\" ] && [ \"$2\" = \"--rebase\" ]; then exit 42; fi",
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

	upstreamPath := filepath.Join(testInstance.TempDir(), "upstream")
	cloneCommand := exec.Command("git", "clone", remotePath, upstreamPath)
	cloneCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, cloneCommand.Run())

	upstreamNameCommand := exec.Command("git", "-C", upstreamPath, "config", "user.name", "Cd Refresh")
	upstreamNameCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, upstreamNameCommand.Run())

	upstreamEmailCommand := exec.Command("git", "-C", upstreamPath, "config", "user.email", "cd-refresh@example.com")
	upstreamEmailCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, upstreamEmailCommand.Run())

	upstreamFilePath := filepath.Join(upstreamPath, "UPSTREAM.md")
	require.NoError(testInstance, os.WriteFile(upstreamFilePath, []byte("remote update\n"), 0o644))

	upstreamAddCommand := exec.Command("git", "-C", upstreamPath, "add", "UPSTREAM.md")
	upstreamAddCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, upstreamAddCommand.Run())

	upstreamCommitCommand := exec.Command("git", "-C", upstreamPath, "commit", "-m", "remote update")
	upstreamCommitCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, upstreamCommitCommand.Run())

	upstreamPushCommand := exec.Command("git", "-C", upstreamPath, "push", "origin", "master")
	upstreamPushCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, upstreamPushCommand.Run())

	require.NoError(testInstance, os.WriteFile(readmePath, []byte("modified locally\n"), 0o644))

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
	require.Contains(testInstance, string(invocationLogContents), "pull --ff-only")
	require.NotContains(testInstance, string(invocationLogContents), "pull --rebase")

	remoteFileContents, remoteReadError := os.ReadFile(filepath.Join(repositoryPath, "UPSTREAM.md"))
	require.NoError(testInstance, remoteReadError)
	require.Equal(testInstance, "remote update\n", string(remoteFileContents))

	localFileContents, localReadError := os.ReadFile(readmePath)
	require.NoError(testInstance, localReadError)
	require.Equal(testInstance, "modified locally\n", string(localFileContents))
}
