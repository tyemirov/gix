package shared

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
)

type stubGitExecutor struct {
	result      execshell.ExecutionResult
	err         error
	lastCommand execshell.CommandDetails
}

func (executor *stubGitExecutor) ExecuteGit(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.lastCommand = details
	if executor.err != nil {
		return execshell.ExecutionResult{}, executor.err
	}
	return executor.result, nil
}

func (executor *stubGitExecutor) ExecuteGitHubCLI(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func TestCheckIgnoredPathsNilExecutor(t *testing.T) {
	t.Parallel()

	result, err := CheckIgnoredPaths(context.Background(), nil, "/tmp/worktree", []string{"tools/licenser"})
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestCheckIgnoredPathsExitCodeOne(t *testing.T) {
	t.Parallel()

	executor := &stubGitExecutor{
		err: execshell.CommandFailedError{
			Result: execshell.ExecutionResult{ExitCode: 1},
		},
	}

	result, err := CheckIgnoredPaths(context.Background(), executor, "/tmp/worktree", []string{"tools/licenser"})
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestCheckIgnoredPathsMapsOriginalNames(t *testing.T) {
	t.Parallel()

	executor := &stubGitExecutor{
		result: execshell.ExecutionResult{StandardOutput: "tools/licenser\n"},
	}

	result, err := CheckIgnoredPaths(context.Background(), executor, "/tmp/worktree", []string{"tools/licenser"})
	require.NoError(t, err)
	require.Contains(t, result, "tools/licenser")
	require.Equal(t, "/tmp/worktree", executor.lastCommand.WorkingDirectory)
	require.Equal(t, "tools/licenser", string(executor.lastCommand.StandardInput))
}

func TestCheckIgnoredPathsHandlesWindowsSeparators(t *testing.T) {
	t.Parallel()

	executor := &stubGitExecutor{
		result: execshell.ExecutionResult{StandardOutput: "tools\\licenser\n"},
	}

	result, err := CheckIgnoredPaths(context.Background(), executor, "/tmp/worktree", []string{"tools/licenser"})
	require.NoError(t, err)
	require.Contains(t, result, "tools/licenser")
}

func TestFilterIgnoredRepositoriesNilExecutor(t *testing.T) {
	t.Parallel()

	repositories := []string{"/tmp/repo", "/tmp/repo/tools/licenser"}
	filtered, err := FilterIgnoredRepositories(context.Background(), nil, repositories)
	require.NoError(t, err)
	require.Equal(t, repositories, filtered)
}

func TestFilterIgnoredRepositoriesSkipsIgnoredChildren(t *testing.T) {
	t.Parallel()

	executor := &stubGitExecutor{
		result: execshell.ExecutionResult{StandardOutput: "tools/licenser\n"},
	}

	repositories := []string{
		"/tmp/repo",
		"/tmp/repo/tools/licenser",
		"/tmp/another",
	}

	filtered, err := FilterIgnoredRepositories(context.Background(), executor, repositories)
	require.NoError(t, err)
	require.Equal(t, []string{"/tmp/repo", "/tmp/another"}, filtered)
	require.Equal(t, "/tmp/repo", executor.lastCommand.WorkingDirectory)
	require.Equal(t, "tools/licenser", string(executor.lastCommand.StandardInput))
}

func TestFilterIgnoredRepositoriesPropagatesErrors(t *testing.T) {
	t.Parallel()

	executor := &stubGitExecutor{
		err: errors.New("git failure"),
	}

	_, err := FilterIgnoredRepositories(
		context.Background(),
		executor,
		[]string{"/tmp/repo", "/tmp/repo/tools/licenser"},
	)
	require.Error(t, err)
	require.EqualError(t, err, "git failure")
}

func TestFilterIgnoredRepositoriesExitCodeOne(t *testing.T) {
	t.Parallel()

	executor := &stubGitExecutor{
		err: execshell.CommandFailedError{
			Result: execshell.ExecutionResult{ExitCode: 1},
		},
	}

	repositories := []string{"/tmp/repo", "/tmp/repo/tools/licenser"}

	filtered, err := FilterIgnoredRepositories(context.Background(), executor, repositories)
	require.NoError(t, err)
	require.Equal(t, repositories, filtered)
}
