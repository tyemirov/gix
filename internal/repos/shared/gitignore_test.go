package shared

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
)

type stubGitExecutor struct {
	result      execshell.ExecutionResult
	err         error
	lastCommand execshell.CommandDetails
	commands    []execshell.CommandDetails
	results     map[string]execshell.ExecutionResult
	errors      map[string]error
}

func (executor *stubGitExecutor) ExecuteGit(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.lastCommand = details
	executor.commands = append(executor.commands, details)
	commandKey := strings.Join(details.Arguments, " ")
	if executor.errors != nil {
		if commandErr, exists := executor.errors[commandKey]; exists {
			return execshell.ExecutionResult{}, commandErr
		}
	}
	if executor.results != nil {
		if result, exists := executor.results[commandKey]; exists {
			return result, nil
		}
	}
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
		errors: map[string]error{
			"check-ignore --stdin": execshell.CommandFailedError{
				Result: execshell.ExecutionResult{ExitCode: 1},
			},
		},
	}

	result, err := CheckIgnoredPaths(context.Background(), executor, "/tmp/worktree", []string{"tools/licenser"})
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestCheckIgnoredPathsMapsOriginalNames(t *testing.T) {
	t.Parallel()

	executor := &stubGitExecutor{
		results: map[string]execshell.ExecutionResult{
			"check-ignore --stdin": {StandardOutput: "tools/licenser\n"},
		},
	}

	result, err := CheckIgnoredPaths(context.Background(), executor, "/tmp/worktree", []string{"tools/licenser"})
	require.NoError(t, err)
	require.Contains(t, result, "tools/licenser")
	require.Equal(t, "/tmp/worktree", executor.commands[0].WorkingDirectory)
	require.Equal(t, "tools/licenser", string(executor.commands[0].StandardInput))
}

func TestCheckIgnoredPathsHandlesWindowsSeparators(t *testing.T) {
	t.Parallel()

	executor := &stubGitExecutor{
		results: map[string]execshell.ExecutionResult{
			"check-ignore --stdin": {StandardOutput: "tools\\licenser\n"},
		},
	}

	result, err := CheckIgnoredPaths(context.Background(), executor, "/tmp/worktree", []string{"tools/licenser"})
	require.NoError(t, err)
	require.Contains(t, result, "tools/licenser")
}

func TestCheckIgnoredPathsIncludesTrackedIgnoredPaths(t *testing.T) {
	t.Parallel()

	executor := &stubGitExecutor{
		errors: map[string]error{
			"check-ignore --stdin": execshell.CommandFailedError{
				Result: execshell.ExecutionResult{ExitCode: 1},
			},
		},
		results: map[string]execshell.ExecutionResult{
			"ls-files --cached --ignored --exclude-standard -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc scripts/deploy.sh": {
				StandardOutput: "python/llm_proxy_client/__pycache__/client.cpython-313.pyc\n",
			},
		},
	}

	result, err := CheckIgnoredPaths(context.Background(), executor, "/tmp/worktree", []string{
		"python/llm_proxy_client/__pycache__/client.cpython-313.pyc",
		"scripts/deploy.sh",
	})
	require.NoError(t, err)
	require.Contains(t, result, "python/llm_proxy_client/__pycache__/client.cpython-313.pyc")
	require.NotContains(t, result, "scripts/deploy.sh")
	require.Len(t, executor.commands, 2)
	require.Equal(t, "check-ignore --stdin", strings.Join(executor.commands[0].Arguments, " "))
	require.Equal(t, "ls-files --cached --ignored --exclude-standard -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc scripts/deploy.sh", strings.Join(executor.commands[1].Arguments, " "))
}

func TestCheckIgnoredPathsDoesNotTreatCachedIgnoredChildrenAsIgnoredDirectories(t *testing.T) {
	t.Parallel()

	executor := &stubGitExecutor{
		errors: map[string]error{
			"check-ignore --stdin": execshell.CommandFailedError{
				Result: execshell.ExecutionResult{ExitCode: 1},
			},
		},
		results: map[string]execshell.ExecutionResult{
			"ls-files --cached --ignored --exclude-standard -- python/llm_proxy_client scripts/deploy.sh": {
				StandardOutput: "python/llm_proxy_client/__pycache__/client.cpython-313.pyc\n",
			},
		},
	}

	result, err := CheckIgnoredPaths(context.Background(), executor, "/tmp/worktree", []string{
		"python/llm_proxy_client",
		"scripts/deploy.sh",
	})
	require.NoError(t, err)
	require.NotContains(t, result, "python/llm_proxy_client")
	require.NotContains(t, result, "scripts/deploy.sh")
}

func TestCheckIgnoredPathsIncludesTrackedIgnoredPathsFromGit(t *testing.T) {
	t.Parallel()

	worktreeRoot := t.TempDir()
	runGitTestCommand(t, worktreeRoot, "init", "-q")
	runGitTestCommand(t, worktreeRoot, "config", "user.name", "Ignored Path Test")
	runGitTestCommand(t, worktreeRoot, "config", "user.email", "ignored-path-test@example.com")

	require.NoError(t, os.WriteFile(filepath.Join(worktreeRoot, ".gitignore"), []byte("__pycache__/\n"), 0o644))
	modifiedPath := filepath.Join("python", "llm_proxy_client", "__pycache__", "client.cpython-313.pyc")
	deletedPath := filepath.Join("python", "tests", "__pycache__", "test_client.cpython-313-pytest-9.0.3.pyc")
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(worktreeRoot, modifiedPath)), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(worktreeRoot, deletedPath)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(worktreeRoot, modifiedPath), []byte("modified before commit\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(worktreeRoot, deletedPath), []byte("deleted before commit\n"), 0o644))

	runGitTestCommand(t, worktreeRoot, "add", ".gitignore")
	runGitTestCommand(t, worktreeRoot, "add", "-f", modifiedPath, deletedPath)
	runGitTestCommand(t, worktreeRoot, "commit", "-q", "-m", "initial")

	require.NoError(t, os.WriteFile(filepath.Join(worktreeRoot, modifiedPath), []byte("modified after commit\n"), 0o644))
	require.NoError(t, os.Remove(filepath.Join(worktreeRoot, deletedPath)))

	executor, executorErr := execshell.NewShellExecutor(zap.NewNop(), execshell.NewOSCommandRunner(), false)
	require.NoError(t, executorErr)
	ignored, ignoredErr := CheckIgnoredPaths(context.Background(), executor, worktreeRoot, []string{
		filepath.ToSlash(modifiedPath),
		filepath.ToSlash(deletedPath),
		"scripts/deploy.sh",
	})
	require.NoError(t, ignoredErr)
	require.Contains(t, ignored, filepath.ToSlash(modifiedPath))
	require.Contains(t, ignored, filepath.ToSlash(deletedPath))
	require.NotContains(t, ignored, "scripts/deploy.sh")
}

func runGitTestCommand(t *testing.T, workingDirectory string, arguments ...string) string {
	t.Helper()

	command := exec.Command("git", arguments...)
	command.Dir = workingDirectory
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	return string(output)
}

func TestCheckIgnoredPathsIgnoresNonRepositories(t *testing.T) {
	t.Parallel()

	executor := &stubGitExecutor{
		err: execshell.CommandFailedError{
			Result: execshell.ExecutionResult{
				ExitCode:      128,
				StandardError: "fatal: not a git repository (or any of the parent directories): .git",
			},
		},
	}

	result, err := CheckIgnoredPaths(context.Background(), executor, "/tmp/worktree", []string{"tools/licenser"})
	require.NoError(t, err)
	require.Empty(t, result)
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
		results: map[string]execshell.ExecutionResult{
			"check-ignore --stdin": {StandardOutput: "tools/licenser\n"},
		},
	}

	repositories := []string{
		"/tmp/repo",
		"/tmp/repo/tools/licenser",
		"/tmp/another",
	}

	filtered, err := FilterIgnoredRepositories(context.Background(), executor, repositories)
	require.NoError(t, err)
	require.Equal(t, []string{"/tmp/repo", "/tmp/another"}, filtered)
	require.Equal(t, "/tmp/repo", executor.commands[0].WorkingDirectory)
	require.Equal(t, "tools/licenser", string(executor.commands[0].StandardInput))
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
		errors: map[string]error{
			"check-ignore --stdin": execshell.CommandFailedError{
				Result: execshell.ExecutionResult{ExitCode: 1},
			},
		},
	}

	repositories := []string{"/tmp/repo", "/tmp/repo/tools/licenser"}

	filtered, err := FilterIgnoredRepositories(context.Background(), executor, repositories)
	require.NoError(t, err)
	require.Equal(t, repositories, filtered)
}
