package workflow

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/gitrepo"
	"github.com/temirov/gix/internal/repos/filesystem"
)

const namespaceTestCommitMessage = "chore: rewrite namespace"

func TestHandleNamespaceRewriteActionDryRun(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))

	goMod := "module github.com/old/org/app\n\ngo 1.22\nrequire github.com/old/org/dep v1.0.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0o644))

	source := `package main
import "github.com/old/org/dep"
func main() { dep.Do() }
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(source), 0o644))

	executor := &namespaceTestGitExecutor{}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	output := &bytes.Buffer{}
	environment := &Environment{
		FileSystem:        filesystem.OSFileSystem{},
		GitExecutor:       executor,
		RepositoryManager: manager,
		Output:            output,
		DryRun:            true,
	}

	repository := &RepositoryState{Path: tempDir}
	parameters := map[string]any{
		"old": "github.com/old/org",
		"new": "github.com/new/org",
	}

	err = handleNamespaceRewriteAction(context.Background(), environment, repository, parameters)
	require.NoError(t, err)
	require.Contains(t, output.String(), "NAMESPACE-PLAN")

	content, readErr := os.ReadFile(filepath.Join(tempDir, "main.go"))
	require.NoError(t, readErr)
	require.Contains(t, string(content), "github.com/old/org/dep")
}

func TestHandleNamespaceRewriteActionApply(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))

	goMod := "module github.com/old/org/app\n\ngo 1.22\nrequire github.com/old/org/dep v1.0.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0o644))

	source := `package main
import "github.com/old/org/dep"
func main() { dep.Do() }
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(source), 0o644))

	executor := &namespaceTestGitExecutor{
		configValues: map[string]string{
			"user.name":  "Test",
			"user.email": "test@example.com",
		},
	}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	output := &bytes.Buffer{}
	environment := &Environment{
		FileSystem:        filesystem.OSFileSystem{},
		GitExecutor:       executor,
		RepositoryManager: manager,
		Output:            output,
		PromptState:       NewPromptState(true),
	}

	repository := &RepositoryState{Path: tempDir}
	parameters := map[string]any{
		"old":                          "github.com/old/org",
		"new":                          "github.com/new/org",
		"branch_prefix":                "rewrite",
		"remote":                       "origin",
		namespaceCommitMessageFlagName: namespaceTestCommitMessage,
	}

	err = handleNamespaceRewriteAction(context.Background(), environment, repository, parameters)
	require.NoError(t, err)
	require.Contains(t, output.String(), "NAMESPACE-APPLY")

	updatedSource, readErr := os.ReadFile(filepath.Join(tempDir, "main.go"))
	require.NoError(t, readErr)
	require.False(t, strings.Contains(string(updatedSource), "github.com/old/org"))
	require.True(t, strings.Contains(string(updatedSource), "github.com/new/org"))

	joinedCommands := strings.Join(executor.recorded(), "\n")
	require.Contains(t, joinedCommands, "checkout -b rewrite/")
	require.Contains(t, joinedCommands, "push --set-upstream origin")

	commitCommandFound := false
	for _, details := range executor.commands {
		if len(details.Arguments) == 0 {
			continue
		}
		if details.Arguments[0] != "commit" {
			continue
		}
		commitCommandFound = true
		require.Contains(t, details.Arguments, "-m")
		require.Contains(t, details.Arguments, namespaceTestCommitMessage)
	}
	require.True(t, commitCommandFound)
}

func TestHandleNamespaceRewriteActionRespectsSafeguards(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))

	goMod := "module github.com/old/org/app\n\ngo 1.22\nrequire github.com/old/org/dep v1.0.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0o644))

	source := `package main
import "github.com/old/org/dep"
func main() { dep.Do() }
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(source), 0o644))

	executor := &namespaceTestGitExecutor{statusOutput: " M main.go"}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)
	output := &bytes.Buffer{}
	environment := &Environment{
		FileSystem:        filesystem.OSFileSystem{},
		GitExecutor:       executor,
		RepositoryManager: manager,
		Output:            output,
	}

	repository := &RepositoryState{Path: tempDir}
	parameters := map[string]any{
		"old":        "github.com/old/org",
		"new":        "github.com/new/org",
		"safeguards": map[string]any{"require_clean": true},
	}

	err = handleNamespaceRewriteAction(context.Background(), environment, repository, parameters)
	require.NoError(t, err)

	updatedSource, readErr := os.ReadFile(filepath.Join(tempDir, "main.go"))
	require.NoError(t, readErr)
	require.Contains(t, string(updatedSource), "github.com/old/org/dep")

	require.Contains(t, output.String(), "NAMESPACE-SKIP")
	recorded := executor.recorded()
	require.GreaterOrEqual(t, len(recorded), 1)
	require.Equal(t, "status --porcelain", recorded[0])
}

type namespaceTestGitExecutor struct {
	commands     []execshell.CommandDetails
	staged       map[string]struct{}
	configValues map[string]string
	statusOutput string
}

func (executor *namespaceTestGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	if executor.staged == nil {
		executor.staged = map[string]struct{}{}
	}
	executor.commands = append(executor.commands, details)

	args := details.Arguments
	if len(args) == 0 {
		return execshell.ExecutionResult{}, nil
	}

	switch args[0] {
	case "status":
		return execshell.ExecutionResult{StandardOutput: executor.statusOutput}, nil
	case "checkout":
		return execshell.ExecutionResult{}, nil
	case "add":
		if len(args) > 1 {
			executor.staged[args[1]] = struct{}{}
		}
		return execshell.ExecutionResult{}, nil
	case "diff":
		if len(executor.staged) == 0 {
			return execshell.ExecutionResult{}, nil
		}
		return execshell.ExecutionResult{}, execshell.CommandFailedError{Result: execshell.ExecutionResult{ExitCode: 1}}
	case "config":
		return executor.handleConfig(details)
	case "commit":
		executor.staged = map[string]struct{}{}
		return execshell.ExecutionResult{}, nil
	case "push":
		return execshell.ExecutionResult{}, nil
	default:
		return execshell.ExecutionResult{}, nil
	}
}

func (executor *namespaceTestGitExecutor) ExecuteGitHubCLI(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	return execshell.ExecutionResult{}, nil
}

func (executor *namespaceTestGitExecutor) handleConfig(details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	args := details.Arguments
	if len(args) >= 3 && args[1] == "--bool" {
		return execshell.ExecutionResult{StandardOutput: "false\n"}, nil
	}
	if len(args) >= 4 && args[1] == "--local" && args[2] == "--get" {
		if value := executor.configValues[args[3]]; value != "" {
			return execshell.ExecutionResult{StandardOutput: value + "\n"}, nil
		}
		return execshell.ExecutionResult{}, execshell.CommandFailedError{Result: execshell.ExecutionResult{ExitCode: 1}}
	}
	return execshell.ExecutionResult{}, nil
}

func (executor *namespaceTestGitExecutor) recorded() []string {
	results := make([]string, 0, len(executor.commands)*3)
	for _, details := range executor.commands {
		args := details.Arguments
		if len(args) == 0 {
			continue
		}

		results = append(results, strings.Join(args, " "))

		if len(args) >= 2 {
			results = append(results, strings.Join(args[:2], " "))
		}

		if len(args) >= 3 {
			prefix := args[2]
			if slash := strings.Index(prefix, "/"); slash >= 0 {
				prefix = prefix[:slash+1]
			}
			results = append(results, args[0]+" "+args[1]+" "+prefix)
		}
	}
	return results
}
