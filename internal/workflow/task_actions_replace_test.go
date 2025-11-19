package workflow

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/filesystem"
)

type recordingShellExecutor struct {
	commands []execshell.ShellCommand
	clean    bool
	branch   string
}

func (executor *recordingShellExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	if len(details.Arguments) == 0 {
		return execshell.ExecutionResult{}, nil
	}

	switch details.Arguments[0] {
	case "status":
		if executor.clean {
			return execshell.ExecutionResult{StandardOutput: ""}, nil
		}
		return execshell.ExecutionResult{StandardOutput: "M file.txt\n"}, nil
	case "rev-parse":
		if len(details.Arguments) > 1 && details.Arguments[1] == "--abbrev-ref" {
			branch := executor.branch
			if len(branch) == 0 {
				branch = "main"
			}
			return execshell.ExecutionResult{StandardOutput: branch + "\n"}, nil
		}
	}

	return execshell.ExecutionResult{}, nil
}

func (executor *recordingShellExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func (executor *recordingShellExecutor) Execute(_ context.Context, command execshell.ShellCommand) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, command)
	return execshell.ExecutionResult{}, nil
}

func TestHandleFileReplaceActionAppliesChanges(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "example.txt")
	require.NoError(t, filesystem.OSFileSystem{}.WriteFile(targetPath, []byte("alpha BETA gamma BETA"), 0o644))

	executor := &recordingShellExecutor{clean: true, branch: "master"}
	manager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerError)

	output := &bytes.Buffer{}
	environment := &Environment{
		FileSystem:        filesystem.OSFileSystem{},
		RepositoryManager: manager,
		GitExecutor:       executor,
		Output:            output,
	}

	repository := &RepositoryState{Path: tempDir}
	parameters := map[string]any{
		"pattern": "*.txt",
		"find":    "BETA",
		"replace": "DELTA",
		"command": []string{"git", "status"},
	}

	err := handleFileReplaceAction(context.Background(), environment, repository, parameters)
	require.NoError(t, err)

	updatedContent, readErr := filesystem.OSFileSystem{}.ReadFile(targetPath)
	require.NoError(t, readErr)
	require.Equal(t, "alpha DELTA gamma DELTA", string(updatedContent))

	require.Contains(t, output.String(), "REPLACE-APPLY")
	require.Contains(t, output.String(), "REPLACE-COMMAND")
	require.Len(t, executor.commands, 1)
	require.Equal(t, execshell.CommandName("git"), executor.commands[0].Name)
	require.Equal(t, []string{"status"}, executor.commands[0].Details.Arguments)
	require.Equal(t, tempDir, executor.commands[0].Details.WorkingDirectory)
}

func TestHandleFileReplaceActionSafeguardSkips(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "example.txt")
	require.NoError(t, filesystem.OSFileSystem{}.WriteFile(targetPath, []byte("token value"), 0o644))

	executor := &recordingShellExecutor{clean: false, branch: "master"}
	manager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerError)

	output := &bytes.Buffer{}
	environment := &Environment{
		FileSystem:        filesystem.OSFileSystem{},
		RepositoryManager: manager,
		GitExecutor:       executor,
		Output:            output,
	}

	repository := &RepositoryState{Path: tempDir}
	parameters := map[string]any{
		"pattern": "*.txt",
		"find":    "token",
		"replace": "value",
		"safeguards": map[string]any{
			"require_clean": true,
		},
	}

	err := handleFileReplaceAction(context.Background(), environment, repository, parameters)
	require.NoError(t, err)

	currentContent, readErr := filesystem.OSFileSystem{}.ReadFile(targetPath)
	require.NoError(t, readErr)
	require.Equal(t, "token value", string(currentContent))

	require.Contains(t, output.String(), "REPLACE-SKIP")
	require.Empty(t, executor.commands)
}

func TestHandleFileReplaceActionHardStopSafeguard(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	targetPath := filepath.Join(tempDir, "example.txt")
	require.NoError(t, filesystem.OSFileSystem{}.WriteFile(targetPath, []byte("token value"), 0o644))

	executor := &recordingShellExecutor{clean: false, branch: "master"}
	manager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerError)

	environment := &Environment{
		FileSystem:        filesystem.OSFileSystem{},
		RepositoryManager: manager,
		GitExecutor:       executor,
	}

	repository := &RepositoryState{Path: tempDir}
	parameters := map[string]any{
		"pattern": "*.txt",
		"find":    "token",
		"replace": "value",
		"safeguards": map[string]any{
			"hard_stop": map[string]any{"require_clean": true},
		},
	}

	err := handleFileReplaceAction(context.Background(), environment, repository, parameters)
	require.ErrorIs(t, err, errRepositorySkipped)
}
