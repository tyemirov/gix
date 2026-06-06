package shared

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
)

type stubGitExecutor struct {
	result   execshell.ExecutionResult
	err      error
	commands []execshell.CommandDetails
	results  map[string]execshell.ExecutionResult
	errors   map[string]error
}

func (executor *stubGitExecutor) ExecuteGit(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
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

func TestCheckIgnoredPaths(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		executor         *stubGitExecutor
		paths            []string
		expectedIgnored  []string
		expectedError    string
		expectedCommands []string
		expectedInputs   []string
	}{
		{
			name:            "nil executor returns empty",
			paths:           []string{"tools/licenser"},
			expectedIgnored: []string{},
		},
		{
			name:             "empty paths skip git commands",
			executor:         &stubGitExecutor{},
			paths:            []string{},
			expectedIgnored:  []string{},
			expectedCommands: []string{},
			expectedInputs:   []string{},
		},
		{
			name: "check-ignore exit one means no untracked ignored paths",
			executor: &stubGitExecutor{
				errors: map[string]error{
					"check-ignore --stdin": commandFailedErrorWithExitCode(1, ""),
				},
			},
			paths:           []string{"tools/licenser"},
			expectedIgnored: []string{},
			expectedCommands: []string{
				"check-ignore --stdin",
				"ls-files --cached --ignored --exclude-standard -- tools/licenser",
			},
			expectedInputs: []string{"tools/licenser", ""},
		},
		{
			name: "maps exact check-ignore output",
			executor: &stubGitExecutor{
				results: map[string]execshell.ExecutionResult{
					"check-ignore --stdin": {StandardOutput: "tools/licenser\n"},
				},
			},
			paths:           []string{"tools/licenser"},
			expectedIgnored: []string{"tools/licenser"},
			expectedCommands: []string{
				"check-ignore --stdin",
				"ls-files --cached --ignored --exclude-standard -- tools/licenser",
			},
			expectedInputs: []string{"tools/licenser", ""},
		},
		{
			name: "maps check-ignore directory output to child path",
			executor: &stubGitExecutor{
				results: map[string]execshell.ExecutionResult{
					"check-ignore --stdin": {StandardOutput: "python/pkg/__pycache__\n"},
				},
			},
			paths: []string{
				"python/pkg/__pycache__/client.cpython-313.pyc",
				"scripts/deploy.sh",
			},
			expectedIgnored: []string{"python/pkg/__pycache__/client.cpython-313.pyc"},
			expectedCommands: []string{
				"check-ignore --stdin",
				"ls-files --cached --ignored --exclude-standard -- python/pkg/__pycache__/client.cpython-313.pyc scripts/deploy.sh",
			},
			expectedInputs: []string{"python/pkg/__pycache__/client.cpython-313.pyc\nscripts/deploy.sh", ""},
		},
		{
			name: "maps check-ignore windows output",
			executor: &stubGitExecutor{
				results: map[string]execshell.ExecutionResult{
					"check-ignore --stdin": {StandardOutput: "tools\\licenser\n"},
				},
			},
			paths:           []string{"tools/licenser"},
			expectedIgnored: []string{"tools/licenser"},
			expectedCommands: []string{
				"check-ignore --stdin",
				"ls-files --cached --ignored --exclude-standard -- tools/licenser",
			},
			expectedInputs: []string{"tools/licenser", ""},
		},
		{
			name: "preserves original path spelling for windows-style inputs",
			executor: &stubGitExecutor{
				results: map[string]execshell.ExecutionResult{
					"check-ignore --stdin": {StandardOutput: "tools/licenser\n"},
				},
			},
			paths:           []string{`tools\licenser`},
			expectedIgnored: []string{`tools\licenser`},
			expectedCommands: []string{
				"check-ignore --stdin",
				`ls-files --cached --ignored --exclude-standard -- tools\licenser`,
			},
			expectedInputs: []string{`tools\licenser`, ""},
		},
		{
			name: "includes exact cached ignored tracked paths",
			executor: &stubGitExecutor{
				errors: map[string]error{
					"check-ignore --stdin": commandFailedErrorWithExitCode(1, ""),
				},
				results: map[string]execshell.ExecutionResult{
					"ls-files --cached --ignored --exclude-standard -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc scripts/deploy.sh": {
						StandardOutput: "python/llm_proxy_client/__pycache__/client.cpython-313.pyc\n",
					},
				},
			},
			paths: []string{
				"python/llm_proxy_client/__pycache__/client.cpython-313.pyc",
				"scripts/deploy.sh",
			},
			expectedIgnored: []string{"python/llm_proxy_client/__pycache__/client.cpython-313.pyc"},
			expectedCommands: []string{
				"check-ignore --stdin",
				"ls-files --cached --ignored --exclude-standard -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc scripts/deploy.sh",
			},
			expectedInputs: []string{"python/llm_proxy_client/__pycache__/client.cpython-313.pyc\nscripts/deploy.sh", ""},
		},
		{
			name: "maps cached ignored windows output",
			executor: &stubGitExecutor{
				errors: map[string]error{
					"check-ignore --stdin": commandFailedErrorWithExitCode(1, ""),
				},
				results: map[string]execshell.ExecutionResult{
					"ls-files --cached --ignored --exclude-standard -- python/pkg/__pycache__/client.pyc": {
						StandardOutput: "python\\pkg\\__pycache__\\client.pyc\n",
					},
				},
			},
			paths:           []string{"python/pkg/__pycache__/client.pyc"},
			expectedIgnored: []string{"python/pkg/__pycache__/client.pyc"},
			expectedCommands: []string{
				"check-ignore --stdin",
				"ls-files --cached --ignored --exclude-standard -- python/pkg/__pycache__/client.pyc",
			},
			expectedInputs: []string{"python/pkg/__pycache__/client.pyc", ""},
		},
		{
			name: "does not treat cached ignored child output as ignored directory pathspec",
			executor: &stubGitExecutor{
				errors: map[string]error{
					"check-ignore --stdin": commandFailedErrorWithExitCode(1, ""),
				},
				results: map[string]execshell.ExecutionResult{
					"ls-files --cached --ignored --exclude-standard -- python/llm_proxy_client scripts/deploy.sh": {
						StandardOutput: "python/llm_proxy_client/__pycache__/client.cpython-313.pyc\n",
					},
				},
			},
			paths: []string{
				"python/llm_proxy_client",
				"scripts/deploy.sh",
			},
			expectedIgnored: []string{},
			expectedCommands: []string{
				"check-ignore --stdin",
				"ls-files --cached --ignored --exclude-standard -- python/llm_proxy_client scripts/deploy.sh",
			},
			expectedInputs: []string{"python/llm_proxy_client\nscripts/deploy.sh", ""},
		},
		{
			name: "combines check-ignore and cached ignored outputs",
			executor: &stubGitExecutor{
				results: map[string]execshell.ExecutionResult{
					"check-ignore --stdin": {StandardOutput: "python/pkg/__pycache__/new.pyc\n"},
					"ls-files --cached --ignored --exclude-standard -- python/pkg/__pycache__/new.pyc python/pkg/__pycache__/tracked.pyc scripts/deploy.sh": {
						StandardOutput: "python/pkg/__pycache__/tracked.pyc\n",
					},
				},
			},
			paths: []string{
				"python/pkg/__pycache__/new.pyc",
				"python/pkg/__pycache__/tracked.pyc",
				"scripts/deploy.sh",
			},
			expectedIgnored: []string{
				"python/pkg/__pycache__/new.pyc",
				"python/pkg/__pycache__/tracked.pyc",
			},
			expectedCommands: []string{
				"check-ignore --stdin",
				"ls-files --cached --ignored --exclude-standard -- python/pkg/__pycache__/new.pyc python/pkg/__pycache__/tracked.pyc scripts/deploy.sh",
			},
			expectedInputs: []string{"python/pkg/__pycache__/new.pyc\npython/pkg/__pycache__/tracked.pyc\nscripts/deploy.sh", ""},
		},
		{
			name: "ignores non repositories during check-ignore",
			executor: &stubGitExecutor{
				err: commandFailedErrorWithExitCode(128, "fatal: not a git repository (or any of the parent directories): .git"),
			},
			paths:            []string{"tools/licenser"},
			expectedIgnored:  []string{},
			expectedCommands: []string{"check-ignore --stdin"},
			expectedInputs:   []string{"tools/licenser"},
		},
		{
			name: "propagates check-ignore errors",
			executor: &stubGitExecutor{
				err: errors.New("git failure"),
			},
			paths:            []string{"tools/licenser"},
			expectedError:    "git failure",
			expectedCommands: []string{"check-ignore --stdin"},
			expectedInputs:   []string{"tools/licenser"},
		},
		{
			name: "ignores non repositories during cached ignored lookup",
			executor: &stubGitExecutor{
				errors: map[string]error{
					"check-ignore --stdin": commandFailedErrorWithExitCode(1, ""),
					"ls-files --cached --ignored --exclude-standard -- tools/licenser": commandFailedErrorWithExitCode(
						128,
						"fatal: not a git repository (or any of the parent directories): .git",
					),
				},
			},
			paths:           []string{"tools/licenser"},
			expectedIgnored: []string{},
			expectedCommands: []string{
				"check-ignore --stdin",
				"ls-files --cached --ignored --exclude-standard -- tools/licenser",
			},
			expectedInputs: []string{"tools/licenser", ""},
		},
		{
			name: "propagates cached ignored lookup errors",
			executor: &stubGitExecutor{
				errors: map[string]error{
					"check-ignore --stdin": commandFailedErrorWithExitCode(1, ""),
					"ls-files --cached --ignored --exclude-standard -- tools/licenser": errors.New("cached failure"),
				},
			},
			paths:           []string{"tools/licenser"},
			expectedError:   "cached failure",
			expectedIgnored: []string{},
			expectedCommands: []string{
				"check-ignore --stdin",
				"ls-files --cached --ignored --exclude-standard -- tools/licenser",
			},
			expectedInputs: []string{"tools/licenser", ""},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var executor GitExecutor
			if testCase.executor != nil {
				executor = testCase.executor
			}
			result, err := CheckIgnoredPaths(context.Background(), executor, "/tmp/worktree", testCase.paths)
			if testCase.expectedError != "" {
				require.EqualError(t, err, testCase.expectedError)
			} else {
				require.NoError(t, err)
			}
			require.ElementsMatch(t, testCase.expectedIgnored, ignoredPathKeys(result))
			if testCase.executor == nil {
				return
			}
			require.Equal(t, testCase.expectedCommands, recordedCommandStrings(testCase.executor.commands))
			require.Equal(t, testCase.expectedInputs, recordedCommandInputs(testCase.executor.commands))
			for _, command := range testCase.executor.commands {
				require.Equal(t, "/tmp/worktree", command.WorkingDirectory)
			}
		})
	}
}

func TestCheckIgnoredPathsFromGit(t *testing.T) {
	t.Parallel()

	worktreeRoot := t.TempDir()
	runGitTestCommand(t, worktreeRoot, "init", "-q")
	runGitTestCommand(t, worktreeRoot, "config", "user.name", "Ignored Path Test")
	runGitTestCommand(t, worktreeRoot, "config", "user.email", "ignored-path-test@example.com")

	require.NoError(t, os.WriteFile(filepath.Join(worktreeRoot, ".gitignore"), []byte("__pycache__/\n"), 0o644))
	modifiedPath := filepath.Join("python", "llm_proxy_client", "__pycache__", "client.cpython-313.pyc")
	deletedPath := filepath.Join("python", "tests", "__pycache__", "test_client.cpython-313-pytest-9.0.3.pyc")
	untrackedPath := filepath.Join("python", "scratch", "__pycache__", "scratch.cpython-313.pyc")
	normalPath := filepath.Join("scripts", "deploy.sh")
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(worktreeRoot, modifiedPath)), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(worktreeRoot, deletedPath)), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(worktreeRoot, untrackedPath)), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(worktreeRoot, normalPath)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(worktreeRoot, modifiedPath), []byte("modified before commit\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(worktreeRoot, deletedPath), []byte("deleted before commit\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(worktreeRoot, normalPath), []byte("normal before commit\n"), 0o644))

	runGitTestCommand(t, worktreeRoot, "add", ".gitignore")
	runGitTestCommand(t, worktreeRoot, "add", normalPath)
	runGitTestCommand(t, worktreeRoot, "add", "-f", modifiedPath, deletedPath)
	runGitTestCommand(t, worktreeRoot, "commit", "-q", "-m", "initial")

	require.NoError(t, os.WriteFile(filepath.Join(worktreeRoot, modifiedPath), []byte("modified after commit\n"), 0o644))
	require.NoError(t, os.Remove(filepath.Join(worktreeRoot, deletedPath)))
	require.NoError(t, os.WriteFile(filepath.Join(worktreeRoot, untrackedPath), []byte("untracked ignored\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(worktreeRoot, normalPath), []byte("normal after commit\n"), 0o644))

	executor, executorErr := execshell.NewShellExecutor(zap.NewNop(), execshell.NewOSCommandRunner(), false)
	require.NoError(t, executorErr)

	slashedModifiedPath := filepath.ToSlash(modifiedPath)
	slashedDeletedPath := filepath.ToSlash(deletedPath)
	slashedUntrackedPath := filepath.ToSlash(untrackedPath)
	slashedNormalPath := filepath.ToSlash(normalPath)
	testCases := []struct {
		name            string
		paths           []string
		expectedIgnored []string
	}{
		{
			name: "tracked ignored modified and deleted files",
			paths: []string{
				slashedModifiedPath,
				slashedDeletedPath,
				slashedNormalPath,
			},
			expectedIgnored: []string{
				slashedModifiedPath,
				slashedDeletedPath,
			},
		},
		{
			name:            "untracked ignored file",
			paths:           []string{slashedUntrackedPath, slashedNormalPath},
			expectedIgnored: []string{slashedUntrackedPath},
		},
		{
			name:            "directory containing cached ignored child remains stageable",
			paths:           []string{"python/llm_proxy_client", slashedNormalPath},
			expectedIgnored: []string{},
		},
		{
			name:            "normal tracked file is not ignored",
			paths:           []string{slashedNormalPath},
			expectedIgnored: []string{},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ignored, ignoredErr := CheckIgnoredPaths(context.Background(), executor, worktreeRoot, testCase.paths)
			require.NoError(t, ignoredErr)
			require.ElementsMatch(t, testCase.expectedIgnored, ignoredPathKeys(ignored))
		})
	}
}

func runGitTestCommand(t *testing.T, workingDirectory string, arguments ...string) string {
	t.Helper()

	command := exec.Command("git", arguments...)
	command.Dir = workingDirectory
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	return string(output)
}

func commandFailedErrorWithExitCode(exitCode int, standardError string) execshell.CommandFailedError {
	return execshell.CommandFailedError{
		Result: execshell.ExecutionResult{
			ExitCode:      exitCode,
			StandardError: standardError,
		},
	}
}

func ignoredPathKeys(paths map[string]struct{}) []string {
	keys := make([]string, 0, len(paths))
	for path := range paths {
		keys = append(keys, path)
	}
	sort.Strings(keys)
	return keys
}

func recordedCommandStrings(commands []execshell.CommandDetails) []string {
	recorded := make([]string, 0, len(commands))
	for _, command := range commands {
		recorded = append(recorded, strings.Join(command.Arguments, " "))
	}
	return recorded
}

func recordedCommandInputs(commands []execshell.CommandDetails) []string {
	recorded := make([]string, 0, len(commands))
	for _, command := range commands {
		recorded = append(recorded, string(command.StandardInput))
	}
	return recorded
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
