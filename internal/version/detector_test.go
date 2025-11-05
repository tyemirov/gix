package version_test

import (
	"context"
	"errors"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/version"
)

type stubBuildInfoProvider struct {
	info      *debug.BuildInfo
	available bool
}

func (provider stubBuildInfoProvider) Read() (*debug.BuildInfo, bool) {
	if !provider.available {
		return nil, false
	}
	return provider.info, true
}

type stubGitExecutor struct {
	testInstance *testing.T
	commands     []stubGitCommand
}

type stubGitCommand struct {
	expectedArguments []string
	output            string
	executionError    error
}

func (executor *stubGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.testInstance.Helper()
	require.Greater(executor.testInstance, len(executor.commands), 0)

	executedArguments := append([]string{}, details.Arguments...)
	command := executor.commands[0]
	executor.commands = executor.commands[1:]

	require.Equal(executor.testInstance, command.expectedArguments, executedArguments)
	return execshell.ExecutionResult{StandardOutput: command.output}, command.executionError
}

func (executor *stubGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.testInstance.Helper()
	require.Fail(executor.testInstance, "ExecuteGitHubCLI should not be invoked")
	return execshell.ExecutionResult{}, errors.New("unexpected invocation")
}

func TestVersionUsesBuildInfoWhenAvailable(t *testing.T) {
	provider := stubBuildInfoProvider{info: &debug.BuildInfo{Main: debug.Module{Version: "v1.2.3"}}, available: true}
	detector, creationError := version.NewDetector(version.Dependencies{BuildInfoProvider: provider})
	require.NoError(t, creationError)

	versionString := detector.Version(context.Background())
	require.Equal(t, "v1.2.3", versionString)
}

func TestVersionFallsBackToExactDescribe(t *testing.T) {
	executor := &stubGitExecutor{
		testInstance: t,
		commands: []stubGitCommand{
			{expectedArguments: []string{"rev-parse", "--show-toplevel"}, output: "/workspace"},
			{expectedArguments: []string{"describe", "--tags", "--exact-match"}, output: "v0.9.0"},
		},
	}
	detector, creationError := version.NewDetector(version.Dependencies{
		BuildInfoProvider: stubBuildInfoProvider{info: &debug.BuildInfo{Main: debug.Module{Version: "devel"}}, available: true},
		GitExecutor:       executor,
	})
	require.NoError(t, creationError)

	versionString := detector.Version(context.Background())
	require.Equal(t, "v0.9.0", versionString)
	require.Len(t, executor.commands, 0)
}

func TestVersionUsesLongDescribeWhenExactMissing(t *testing.T) {
	executor := &stubGitExecutor{
		testInstance: t,
		commands: []stubGitCommand{
			{expectedArguments: []string{"rev-parse", "--show-toplevel"}, output: "/workspace"},
			{expectedArguments: []string{"describe", "--tags", "--exact-match"}, executionError: errors.New("not tagged")},
			{expectedArguments: []string{"describe", "--tags", "--long", "--dirty"}, output: "v0.9.0-1-gabcdef"},
		},
	}
	detector, creationError := version.NewDetector(version.Dependencies{
		BuildInfoProvider: stubBuildInfoProvider{info: &debug.BuildInfo{Main: debug.Module{Version: "devel"}}, available: true},
		GitExecutor:       executor,
	})
	require.NoError(t, creationError)

	versionString := detector.Version(context.Background())
	require.Equal(t, "v0.9.0-1-gabcdef", versionString)
}

func TestVersionReturnsUnknownWhenAllSourcesFail(t *testing.T) {
	executor := &stubGitExecutor{
		testInstance: t,
		commands: []stubGitCommand{
			{expectedArguments: []string{"rev-parse", "--show-toplevel"}, executionError: errors.New("failure")},
			{expectedArguments: []string{"describe", "--tags", "--exact-match"}, executionError: errors.New("failure")},
			{expectedArguments: []string{"describe", "--tags", "--long", "--dirty"}, executionError: errors.New("failure")},
		},
	}
	detector, creationError := version.NewDetector(version.Dependencies{
		BuildInfoProvider: stubBuildInfoProvider{info: &debug.BuildInfo{Main: debug.Module{Version: "devel"}}, available: true},
		GitExecutor:       executor,
	})
	require.NoError(t, creationError)

	versionString := detector.Version(context.Background())
	require.Equal(t, "unknown", versionString)
}

var _ shared.GitExecutor = (*stubGitExecutor)(nil)
