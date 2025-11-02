package namespace_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/gitrepo"
	"github.com/temirov/gix/internal/repos/filesystem"
	"github.com/temirov/gix/internal/repos/namespace"
	"github.com/temirov/gix/internal/repos/shared"
)

func TestRewriteDryRunReportsChanges(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))

	goModContent := "module github.com/old/account/app\n\ngo 1.22\nrequire github.com/old/account/dep v1.0.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goModContent), 0o644))

	source := `package main
import "github.com/old/account/dep"
func main() { dep.Do() }
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(source), 0o644))

	oldPrefix, err := namespace.NewModulePrefix("github.com/old/account")
	require.NoError(t, err)
	newPrefix, err := namespace.NewModulePrefix("github.com/new/account")
	require.NoError(t, err)

	executor := &noopGitExecutor{}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	service, err := namespace.NewService(namespace.Dependencies{
		FileSystem:        filesystem.OSFileSystem{},
		GitExecutor:       executor,
		RepositoryManager: manager,
		Clock:             fixedClock{instant: time.Date(2024, 11, 24, 12, 0, 0, 0, time.UTC)},
	})
	require.NoError(t, err)

	repositoryPath, err := shared.NewRepositoryPath(tempDir)
	require.NoError(t, err)

	result, rewriteErr := service.Rewrite(context.Background(), namespace.Options{
		RepositoryPath: repositoryPath,
		OldPrefix:      oldPrefix,
		NewPrefix:      newPrefix,
		DryRun:         true,
		Push:           true,
	})
	require.NoError(t, rewriteErr)

	require.False(t, result.Skipped)
	require.True(t, result.GoModChanged)
	require.Contains(t, result.ChangedFiles, "main.go")
	require.NotEmpty(t, result.BranchName)
	require.Contains(t, result.BranchName, "namespace-rewrite")
}

func TestRewriteAppliesChanges(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))

	goModContent := "module github.com/old/account/app\n\ngo 1.22\nrequire github.com/old/account/dep v1.0.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goModContent), 0o644))

	source := `package main
import "github.com/old/account/dep"
func main() { dep.Do() }
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(source), 0o644))

	oldPrefix, err := namespace.NewModulePrefix("github.com/old/account")
	require.NoError(t, err)
	newPrefix, err := namespace.NewModulePrefix("github.com/new/account")
	require.NoError(t, err)

	executor := &recordingGitExecutor{
		configValues: map[string]string{
			"user.name":  "Test User",
			"user.email": "test@example.com",
		},
	}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	service, err := namespace.NewService(namespace.Dependencies{
		FileSystem:        filesystem.OSFileSystem{},
		GitExecutor:       executor,
		RepositoryManager: manager,
		Clock:             fixedClock{instant: time.Date(2024, 11, 24, 12, 0, 0, 0, time.UTC)},
	})
	require.NoError(t, err)

	repositoryPath, err := shared.NewRepositoryPath(tempDir)
	require.NoError(t, err)

	result, rewriteErr := service.Rewrite(context.Background(), namespace.Options{
		RepositoryPath: repositoryPath,
		OldPrefix:      oldPrefix,
		NewPrefix:      newPrefix,
		BranchPrefix:   "ns-update",
		CommitMessage:  "chore: rewrite namespace",
		Push:           true,
		PushRemote:     "origin",
	})
	require.NoError(t, rewriteErr)

	require.False(t, result.Skipped)
	require.Equal(t, "ns-update/20241124-120000Z", result.BranchName)
	require.True(t, result.CommitCreated)
	require.True(t, result.PushPerformed)

	updatedGoMod, readModErr := os.ReadFile(filepath.Join(tempDir, "go.mod"))
	require.NoError(t, readModErr)
	require.NotContains(t, string(updatedGoMod), "github.com/old/account")
	require.Contains(t, string(updatedGoMod), "github.com/new/account")

	updatedSource, readSourceErr := os.ReadFile(filepath.Join(tempDir, "main.go"))
	require.NoError(t, readSourceErr)
	require.False(t, strings.Contains(string(updatedSource), "github.com/old/account"))
	require.True(t, strings.Contains(string(updatedSource), "github.com/new/account"))

	require.Contains(t, executor.recordedCommands(), "checkout -b ns-update/20241124-120000Z")
	require.Contains(t, executor.recordedCommands(), "add go.mod")
	require.Contains(t, executor.recordedCommands(), "add main.go")
	require.Contains(t, executor.recordedCommands(), "commit -m chore: rewrite namespace")
	require.Contains(t, executor.recordedCommands(), "push --set-upstream origin ns-update/20241124-120000Z")
}

func TestRewriteUpdatesGoModDependencyBlocks(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))

	goModContent := "module github.com/old/account/app\n\ngo 1.22\n\nrequire (\n\tgithub.com/old/account/dep v1.0.0\n\tgithub.com/another/module v0.2.0\n)\n\nreplace (\n\tgithub.com/old/account/dep => github.com/old/account/dep v1.0.1\n\tgithub.com/old/account/tool => github.com/old/account/tool v1.2.3\n)\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goModContent), 0o644))

	source := `package main
import "github.com/old/account/dep"
func main() { dep.Do() }
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(source), 0o644))

	oldPrefix, err := namespace.NewModulePrefix("github.com/old/account")
	require.NoError(t, err)
	newPrefix, err := namespace.NewModulePrefix("github.com/new/account")
	require.NoError(t, err)

	executor := &recordingGitExecutor{
		configValues: map[string]string{
			"user.name":  "Test User",
			"user.email": "test@example.com",
		},
	}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	service, err := namespace.NewService(namespace.Dependencies{
		FileSystem:        filesystem.OSFileSystem{},
		GitExecutor:       executor,
		RepositoryManager: manager,
		Clock:             fixedClock{instant: time.Date(2024, 11, 24, 12, 0, 0, 0, time.UTC)},
	})
	require.NoError(t, err)

	repositoryPath, err := shared.NewRepositoryPath(tempDir)
	require.NoError(t, err)

	_, rewriteErr := service.Rewrite(context.Background(), namespace.Options{
		RepositoryPath: repositoryPath,
		OldPrefix:      oldPrefix,
		NewPrefix:      newPrefix,
		CommitMessage:  "chore: rewrite namespace",
		Push:           false,
	})
	require.NoError(t, rewriteErr)

	updatedGoMod, readErr := os.ReadFile(filepath.Join(tempDir, "go.mod"))
	require.NoError(t, readErr)
	goModString := string(updatedGoMod)

	require.NotContains(t, goModString, "github.com/old/account/dep v1.0.0")
	require.Contains(t, goModString, "github.com/new/account/dep v1.0.0")
	require.Contains(t, goModString, "github.com/new/account/dep => github.com/new/account/dep v1.0.1")
	require.Contains(t, goModString, "github.com/new/account/tool => github.com/new/account/tool v1.2.3")
}

func TestRewriteDryRunDetectsRootModulePrefix(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))

	goModContent := "module github.com/old/account\n\ngo 1.22\n\nrequire (\n\tgithub.com/old/account v1.0.0\n\tgithub.com/another/module v0.2.0\n)\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goModContent), 0o644))

	oldPrefix, err := namespace.NewModulePrefix("github.com/old/account")
	require.NoError(t, err)
	newPrefix, err := namespace.NewModulePrefix("github.com/new/account")
	require.NoError(t, err)

	executor := &noopGitExecutor{}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	service, err := namespace.NewService(namespace.Dependencies{
		FileSystem:        filesystem.OSFileSystem{},
		GitExecutor:       executor,
		RepositoryManager: manager,
		Clock:             fixedClock{instant: time.Date(2024, 11, 24, 12, 0, 0, 0, time.UTC)},
	})
	require.NoError(t, err)

	repositoryPath, err := shared.NewRepositoryPath(tempDir)
	require.NoError(t, err)

	result, rewriteErr := service.Rewrite(context.Background(), namespace.Options{
		RepositoryPath: repositoryPath,
		OldPrefix:      oldPrefix,
		NewPrefix:      newPrefix,
		DryRun:         true,
	})
	require.NoError(t, rewriteErr)

	require.False(t, result.Skipped)
	require.True(t, result.GoModChanged)
	require.Empty(t, result.ChangedFiles)
}

func TestRewriteUpdatesGoModRootModuleEntries(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))

	goModContent := "module github.com/old/account\n\ngo 1.22\n\nrequire (\n\tgithub.com/old/account v1.0.0\n\tgithub.com/old/account/dep v1.2.3\n)\n\nreplace (\n\tgithub.com/old/account => github.com/old/account v1.3.0\n\tgithub.com/old/account/dep => github.com/old/account/dep v1.2.4\n)\n\nexclude (\n\tgithub.com/old/account v1.4.0\n\tgithub.com/old/account/dep v1.4.1\n)\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goModContent), 0o644))

	oldPrefix, err := namespace.NewModulePrefix("github.com/old/account")
	require.NoError(t, err)
	newPrefix, err := namespace.NewModulePrefix("github.com/new/account")
	require.NoError(t, err)

	executor := &recordingGitExecutor{
		configValues: map[string]string{
			"user.name":  "Test User",
			"user.email": "test@example.com",
		},
	}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	service, err := namespace.NewService(namespace.Dependencies{
		FileSystem:        filesystem.OSFileSystem{},
		GitExecutor:       executor,
		RepositoryManager: manager,
		Clock:             fixedClock{instant: time.Date(2024, 11, 24, 12, 0, 0, 0, time.UTC)},
	})
	require.NoError(t, err)

	repositoryPath, err := shared.NewRepositoryPath(tempDir)
	require.NoError(t, err)

	result, rewriteErr := service.Rewrite(context.Background(), namespace.Options{
		RepositoryPath: repositoryPath,
		OldPrefix:      oldPrefix,
		NewPrefix:      newPrefix,
		CommitMessage:  "chore: rewrite namespace",
	})
	require.NoError(t, rewriteErr)

	require.False(t, result.Skipped)
	require.True(t, result.GoModChanged)
	require.Contains(t, executor.recordedCommands(), "add go.mod")

	updatedGoMod, readErr := os.ReadFile(filepath.Join(tempDir, "go.mod"))
	require.NoError(t, readErr)
	goModString := string(updatedGoMod)

	require.NotContains(t, goModString, "github.com/old/account")
	require.Contains(t, goModString, "module github.com/new/account\n")
	require.Contains(t, goModString, "github.com/new/account v1.0.0")
	require.Contains(t, goModString, "github.com/new/account/dep v1.2.3")
	require.Contains(t, goModString, "github.com/new/account => github.com/new/account v1.3.0")
	require.Contains(t, goModString, "github.com/new/account/dep => github.com/new/account/dep v1.2.4")
	require.Contains(t, goModString, "github.com/new/account v1.4.0")
	require.Contains(t, goModString, "github.com/new/account/dep v1.4.1")
}

type noopGitExecutor struct{}

func (noopGitExecutor) ExecuteGit(_ context.Context, _ execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func (noopGitExecutor) ExecuteGitHubCLI(_ context.Context, _ execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type recordingGitExecutor struct {
	commands     []execshell.CommandDetails
	staged       map[string]struct{}
	configValues map[string]string
}

func (executor *recordingGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	if executor.staged == nil {
		executor.staged = map[string]struct{}{}
	}
	executor.commands = append(executor.commands, details)

	if len(details.Arguments) == 0 {
		return execshell.ExecutionResult{}, nil
	}

	switch details.Arguments[0] {
	case "status":
		return execshell.ExecutionResult{StandardOutput: ""}, nil
	case "checkout":
		return execshell.ExecutionResult{}, nil
	case "add":
		if len(details.Arguments) > 1 {
			executor.staged[details.Arguments[1]] = struct{}{}
		}
		return execshell.ExecutionResult{}, nil
	case "diff":
		if len(executor.staged) == 0 {
			return execshell.ExecutionResult{}, nil
		}
		return execshell.ExecutionResult{}, execshell.CommandFailedError{
			Command: execshell.ShellCommand{Name: execshell.CommandGit, Details: details},
			Result:  execshell.ExecutionResult{ExitCode: 1},
		}
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

func (executor *recordingGitExecutor) ExecuteGitHubCLI(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	return execshell.ExecutionResult{}, nil
}

func (executor *recordingGitExecutor) handleConfig(details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	args := details.Arguments
	if len(args) < 2 {
		return execshell.ExecutionResult{}, nil
	}
	if args[1] == "--bool" && len(args) >= 3 {
		return execshell.ExecutionResult{StandardOutput: "false\n"}, nil
	}
	if args[1] == "--local" && len(args) >= 4 && args[2] == "--get" {
		value := executor.configValues[args[3]]
		if value == "" {
			return execshell.ExecutionResult{}, execshell.CommandFailedError{
				Command: execshell.ShellCommand{Name: execshell.CommandGit, Details: details},
				Result:  execshell.ExecutionResult{ExitCode: 1},
			}
		}
		return execshell.ExecutionResult{StandardOutput: value + "\n"}, nil
	}
	return execshell.ExecutionResult{}, nil
}

func (executor *recordingGitExecutor) recordedCommands() []string {
	commands := make([]string, 0, len(executor.commands))
	for _, details := range executor.commands {
		commands = append(commands, strings.Join(details.Arguments, " "))
	}
	return commands
}

type fixedClock struct {
	instant time.Time
}

func (clock fixedClock) Now() time.Time {
	return clock.instant
}
