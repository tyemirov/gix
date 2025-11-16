package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	integrationUnexpectedSuccessMessageConstant = "command succeeded unexpectedly"
	integrationUnexpectedSuccessFormatConstant  = "%s\n%s"
	integrationCommandFailureFormatConstant     = "command failed: %v\n%s"
	pathEnvironmentVariableNameConstant         = "PATH"
	gitConfigSystemEnvironmentNameConstant      = "GIT_CONFIG_SYSTEM"
	gitConfigGlobalEnvironmentNameConstant      = "GIT_CONFIG_GLOBAL"
	gitConfigNoSystemEnvironmentNameConstant    = "GIT_CONFIG_NOSYSTEM"
	gitTerminalPromptEnvironmentNameConstant    = "GIT_TERMINAL_PROMPT"
	gitTerminalPromptDisableValueConstant       = "0"
	environmentAssignmentSeparatorConstant      = "="
	integrationBinaryFileNameConstant           = "gix-integration"
)

type integrationCommandOptions struct {
	PathVariable         string
	EnvironmentOverrides map[string]string
}

func runIntegrationCommand(testInstance *testing.T, repositoryRoot string, options integrationCommandOptions, timeout time.Duration, arguments []string) string {
	testInstance.Helper()
	outputText, commandError := executeIntegrationCommand(testInstance, repositoryRoot, options, timeout, arguments)
	requireNoError(testInstance, commandError, outputText)
	return outputText
}

func runFailingIntegrationCommand(testInstance *testing.T, repositoryRoot string, options integrationCommandOptions, timeout time.Duration, arguments []string) (string, error) {
	testInstance.Helper()
	outputText, commandError := executeIntegrationCommand(testInstance, repositoryRoot, options, timeout, arguments)
	if commandError == nil {
		testInstance.Fatalf(integrationUnexpectedSuccessFormatConstant, integrationUnexpectedSuccessMessageConstant, outputText)
	}
	return outputText, commandError
}

func executeIntegrationCommand(testInstance *testing.T, repositoryRoot string, options integrationCommandOptions, timeout time.Duration, arguments []string) (string, error) {
	testInstance.Helper()
	executionContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	command := exec.CommandContext(executionContext, "go", arguments...)
	command.Dir = repositoryRoot
	command.Env = buildCommandEnvironment(options)

	outputBytes, runError := command.CombinedOutput()
	outputText := string(outputBytes)
	return outputText, runError
}

func buildCommandEnvironment(options integrationCommandOptions) []string {
	environmentAssignments := append([]string{}, os.Environ()...)
	environmentValues := make(map[string]string, len(environmentAssignments))
	for _, assignment := range environmentAssignments {
		separatorIndex := strings.Index(assignment, environmentAssignmentSeparatorConstant)
		if separatorIndex <= 0 {
			continue
		}
		name := assignment[:separatorIndex]
		value := assignment[separatorIndex+len(environmentAssignmentSeparatorConstant):]
		environmentValues[name] = value
	}

	if len(options.PathVariable) > 0 {
		environmentValues[pathEnvironmentVariableNameConstant] = options.PathVariable
	}

	environmentValues[gitConfigSystemEnvironmentNameConstant] = "/dev/null"
	environmentValues[gitConfigGlobalEnvironmentNameConstant] = "/dev/null"
	environmentValues[gitConfigNoSystemEnvironmentNameConstant] = "1"

	for variableName, variableValue := range options.EnvironmentOverrides {
		environmentValues[variableName] = variableValue
	}

	if _, exists := environmentValues[gitTerminalPromptEnvironmentNameConstant]; !exists {
		environmentValues[gitTerminalPromptEnvironmentNameConstant] = gitTerminalPromptDisableValueConstant
	}

	if len(environmentValues) == 0 {
		return []string{}
	}

	environmentNames := make([]string, 0, len(environmentValues))
	for variableName := range environmentValues {
		environmentNames = append(environmentNames, variableName)
	}
	sort.Strings(environmentNames)

	mergedEnvironment := make([]string, 0, len(environmentNames))
	for _, variableName := range environmentNames {
		mergedEnvironment = append(mergedEnvironment, variableName+environmentAssignmentSeparatorConstant+environmentValues[variableName])
	}

	return mergedEnvironment
}

func buildGitCommandEnvironment(overrides map[string]string) []string {
	mergedOverrides := map[string]string{
		gitTerminalPromptEnvironmentNameConstant: gitTerminalPromptDisableValueConstant,
	}
	for key, value := range overrides {
		mergedOverrides[key] = value
	}
	return buildCommandEnvironment(integrationCommandOptions{EnvironmentOverrides: mergedOverrides})
}

func filterStructuredOutput(rawOutput string) string {
	lines := strings.Split(rawOutput, "\n")
	var filtered []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		if strings.HasPrefix(trimmed, "{") {
			continue
		}
		if strings.HasPrefix(trimmed, "Summary:") {
			continue
		}
		filtered = append(filtered, line)
	}
	if len(filtered) == 0 {
		return ""
	}
	return strings.Join(filtered, "\n") + "\n"
}

func requireNoError(testInstance *testing.T, err error, output string) {
	testInstance.Helper()
	if err != nil {
		testInstance.Fatalf(integrationCommandFailureFormatConstant, err, output)
	}
}

func buildIntegrationBinary(testInstance *testing.T, repositoryRoot string) string {
	testInstance.Helper()
	binaryDirectory := testInstance.TempDir()
	binaryPath := filepath.Join(binaryDirectory, integrationBinaryFileNameConstant)

	command := exec.Command("go", "build", "-o", binaryPath, ".")
	command.Dir = repositoryRoot
	command.Env = os.Environ()

	outputBytes, runError := command.CombinedOutput()
	if runError != nil {
		testInstance.Fatalf(integrationCommandFailureFormatConstant, runError, string(outputBytes))
	}

	return binaryPath
}

func runBinaryIntegrationCommand(
	testInstance *testing.T,
	binaryPath string,
	workingDirectory string,
	environmentOverrides map[string]string,
	timeout time.Duration,
	arguments []string,
) (string, error) {
	testInstance.Helper()

	executionContext, cancelFunction := context.WithTimeout(context.Background(), timeout)
	defer cancelFunction()

	command := exec.CommandContext(executionContext, binaryPath, arguments...)
	command.Dir = workingDirectory
	command.Env = buildCommandEnvironment(integrationCommandOptions{EnvironmentOverrides: environmentOverrides})

	outputBytes, runError := command.CombinedOutput()
	outputText := string(outputBytes)
	return outputText, runError
}

type gitRepositoryOptions struct {
	Path          string
	DirectoryName string
	RemoteURL     string
	InitialBranch string
}

func createGitRepository(testInstance *testing.T, options gitRepositoryOptions) string {
	testInstance.Helper()

	targetPath := strings.TrimSpace(options.Path)
	directoryName := strings.TrimSpace(options.DirectoryName)
	if len(directoryName) == 0 {
		directoryName = "repository"
	}

	initialBranch := strings.TrimSpace(options.InitialBranch)
	if len(initialBranch) == 0 {
		initialBranch = "main"
	}

	var repositoryPath string
	if len(targetPath) > 0 {
		repositoryPath = filepath.Clean(targetPath)
		require.NoError(testInstance, os.MkdirAll(filepath.Dir(repositoryPath), 0o755))
	} else {
		repositoryParent := testInstance.TempDir()
		repositoryPath = filepath.Join(repositoryParent, directoryName)
	}

	initCommand := exec.Command("git", "init", "--initial-branch="+initialBranch, repositoryPath)
	initCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, initCommand.Run())

	remoteURL := strings.TrimSpace(options.RemoteURL)
	if len(remoteURL) == 0 {
		return repositoryPath
	}

	remoteCommand := exec.Command("git", "-C", repositoryPath, "remote", "add", "origin", remoteURL)
	remoteCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, remoteCommand.Run())

	return repositoryPath
}

func buildStubbedExecutablePath(testInstance *testing.T, executableName string, scriptContents string) string {
	testInstance.Helper()

	stubDirectory := testInstance.TempDir()
	stubPath := filepath.Join(stubDirectory, executableName)
	require.NoError(testInstance, os.WriteFile(stubPath, []byte(scriptContents), 0o755))

	currentPath := os.Getenv(pathEnvironmentVariableNameConstant)
	if len(currentPath) == 0 {
		return stubDirectory
	}
	return stubDirectory + string(os.PathListSeparator) + currentPath
}

func TestCreateGitRepositoryInitializesRemote(t *testing.T) {
	repositoryPath := createGitRepository(t, gitRepositoryOptions{
		DirectoryName: "fixture",
		RemoteURL:     "https://example.com/foo.git",
		InitialBranch: "main",
	})

	require.DirExists(t, repositoryPath)

	remoteCommand := exec.Command("git", "-C", repositoryPath, "remote", "get-url", "origin")
	remoteCommand.Env = buildGitCommandEnvironment(nil)

	outputBytes, commandError := remoteCommand.CombinedOutput()
	require.NoError(t, commandError, string(outputBytes))
	require.Equal(t, "https://example.com/foo.git\n", string(outputBytes))
}

func TestBuildStubbedExecutablePathCreatesBinary(t *testing.T) {
	stubScript := "#!/bin/sh\necho stub\n"
	pathValue := buildStubbedExecutablePath(t, "gh", stubScript)

	require.NotEmpty(t, pathValue)

	pathEntries := strings.Split(pathValue, string(os.PathListSeparator))
	require.NotEmpty(t, pathEntries)

	stubDirectory := pathEntries[0]
	stubPath := filepath.Join(stubDirectory, "gh")
	require.FileExists(t, stubPath)

	t.Setenv(pathEnvironmentVariableNameConstant, pathValue)

	resolvedPath, lookupError := exec.LookPath("gh")
	require.NoError(t, lookupError)
	require.Equal(t, stubPath, resolvedPath)

	command := exec.Command("gh")
	outputBytes, commandError := command.CombinedOutput()
	require.NoError(t, commandError, string(outputBytes))
	require.Equal(t, "stub\n", string(outputBytes))
}
