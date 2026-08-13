package tests

import (
	"bytes"
	"errors"
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
	prCleanupCancellationRepositoryCount = 3
	prCleanupCancellationTimeout         = 10 * time.Second
)

func TestPullRequestCleanupInterruptStopsWithoutFailureOutput(testInstance *testing.T) {
	repositoryRoot := filepath.Dir(mustWorkingDirectory(testInstance))
	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	fleetRoot := testInstance.TempDir()

	for repositoryIndex := 0; repositoryIndex < prCleanupCancellationRepositoryCount; repositoryIndex++ {
		repositoryPath := createGitRepository(testInstance, gitRepositoryOptions{
			Path:          filepath.Join(fleetRoot, fmt.Sprintf("repository-%d", repositoryIndex)),
			RemoteURL:     fmt.Sprintf("https://github.com/octocat/repository-%d.git", repositoryIndex),
			InitialBranch: "master",
		})
		runGit(testInstance, repositoryPath, "config", "user.name", "Integration Tester")
		runGit(testInstance, repositoryPath, "config", "user.email", "tester@example.com")
		require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("fixture\n"), 0o644))
		runGit(testInstance, repositoryPath, "add", "README.md")
		runGit(testInstance, repositoryPath, "commit", "-m", "test: initialize cleanup fixture")
	}

	realGitPath, lookupError := exec.LookPath("git")
	require.NoError(testInstance, lookupError)
	startedMarkerPath := filepath.Join(testInstance.TempDir(), "ls-remote-started")
	invocationMarkerPath := filepath.Join(testInstance.TempDir(), "ls-remote-invocations")
	stubScript := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "ls-remote" ] && [ "$2" = "--heads" ]; then
  printf 'started\n' >> "$GIX_TEST_LS_REMOTE_INVOCATIONS"
  : > "$GIX_TEST_LS_REMOTE_STARTED"
  while :; do sleep 1; done
fi
exec %q "$@"
`, realGitPath)
	stubPath := buildStubbedExecutablePath(testInstance, "git", stubScript)

	environmentOverrides := map[string]string{
		"GIX_TEST_LS_REMOTE_INVOCATIONS": invocationMarkerPath,
		"GIX_TEST_LS_REMOTE_STARTED":     startedMarkerPath,
	}
	arguments := injectBinaryIntegrationConfiguration(testInstance, []string{
		"--log-format=console",
		"prs",
		"delete",
		"--roots",
		fleetRoot,
		"--limit",
		"100",
	}, environmentOverrides)

	command := exec.Command(binaryPath, arguments...)
	command.Dir = repositoryRoot
	command.Env = buildCommandEnvironment(integrationCommandOptions{
		PathVariable:         stubPath,
		EnvironmentOverrides: environmentOverrides,
	})
	var standardOutput bytes.Buffer
	var standardError bytes.Buffer
	command.Stdout = &standardOutput
	command.Stderr = &standardError
	require.NoError(testInstance, command.Start())

	waitForFile(testInstance, startedMarkerPath, prCleanupCancellationTimeout)
	require.NoError(testInstance, command.Process.Signal(os.Interrupt))

	waitChannel := make(chan error, 1)
	go func() {
		waitChannel <- command.Wait()
	}()

	var runError error
	select {
	case runError = <-waitChannel:
	case <-time.After(prCleanupCancellationTimeout):
		_ = command.Process.Kill()
		<-waitChannel
		testInstance.Fatal("interrupted cleanup did not exit")
	}

	exitError := &exec.ExitError{}
	require.ErrorAs(testInstance, runError, &exitError)
	require.Equal(testInstance, 130, exitError.ExitCode())

	combinedOutput := standardOutput.String() + standardError.String()
	require.NotContains(testInstance, combinedOutput, "context canceled")
	require.NotContains(testInstance, combinedOutput, "action failed")
	require.NotContains(testInstance, combinedOutput, "command execution failed")
	require.NotContains(testInstance, combinedOutput, "WORKFLOW_OPERATION_FAILURE")
	require.NotContains(testInstance, combinedOutput, "ERROR\t")

	invocationBytes, readError := os.ReadFile(invocationMarkerPath)
	require.NoError(testInstance, readError)
	require.Equal(testInstance, 1, strings.Count(string(invocationBytes), "started"))
}

func mustWorkingDirectory(testInstance *testing.T) string {
	testInstance.Helper()
	workingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	return workingDirectory
}

func waitForFile(testInstance *testing.T, path string, timeout time.Duration) {
	testInstance.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, statError := os.Stat(path)
		if statError == nil {
			return
		}
		if !errors.Is(statError, os.ErrNotExist) {
			require.NoError(testInstance, statError)
		}
		time.Sleep(10 * time.Millisecond)
	}
	testInstance.Fatalf("timed out waiting for %s", path)
}
