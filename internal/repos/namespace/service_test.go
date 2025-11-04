package namespace_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/gitrepo"
	repoerrors "github.com/tyemirov/gix/internal/repos/errors"
	"github.com/tyemirov/gix/internal/repos/filesystem"
	"github.com/tyemirov/gix/internal/repos/namespace"
	"github.com/tyemirov/gix/internal/repos/shared"
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

	testSource := `package main
import (
	"testing"
	"github.com/old/account/dep"
)

func TestDo(t *testing.T) {
	dep.Do()
}
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main_test.go"), []byte(testSource), 0o644))

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
	require.Contains(t, result.ChangedFiles, "main_test.go")
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

	testSource := `package main
import (
	"testing"
	"github.com/old/account/dep"
)

func TestDo(t *testing.T) {
	dep.Do()
}
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main_test.go"), []byte(testSource), 0o644))

	oldPrefix, err := namespace.NewModulePrefix("github.com/old/account")
	require.NoError(t, err)
	newPrefix, err := namespace.NewModulePrefix("github.com/new/account")
	require.NoError(t, err)

	executor := &recordingGitExecutor{
		configValues: map[string]string{
			"user.name":         "Test User",
			"user.email":        "test@example.com",
			"remote.origin.url": "git@example.com:old/account.git",
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
	require.Contains(t, result.ChangedFiles, "main.go")
	require.Contains(t, result.ChangedFiles, "main_test.go")

	updatedGoMod, readModErr := os.ReadFile(filepath.Join(tempDir, "go.mod"))
	require.NoError(t, readModErr)
	require.NotContains(t, string(updatedGoMod), "github.com/old/account")
	require.Contains(t, string(updatedGoMod), "github.com/new/account")

	updatedSource, readSourceErr := os.ReadFile(filepath.Join(tempDir, "main.go"))
	require.NoError(t, readSourceErr)
	require.False(t, strings.Contains(string(updatedSource), "github.com/old/account"))
	require.True(t, strings.Contains(string(updatedSource), "github.com/new/account"))

	updatedTestSource, readTestErr := os.ReadFile(filepath.Join(tempDir, "main_test.go"))
	require.NoError(t, readTestErr)
	require.False(t, strings.Contains(string(updatedTestSource), "github.com/old/account"))
	require.True(t, strings.Contains(string(updatedTestSource), "github.com/new/account"))

	require.Contains(t, executor.recordedCommands(), "checkout -b ns-update/20241124-120000Z")
	require.Contains(t, executor.recordedCommands(), "add go.mod")
	require.Contains(t, executor.recordedCommands(), "add main.go")
	require.Contains(t, executor.recordedCommands(), "add main_test.go")
	require.Contains(t, executor.recordedCommands(), "commit -m chore: rewrite namespace")
	require.Contains(t, executor.recordedCommands(), "push --set-upstream origin ns-update/20241124-120000Z")
}

func TestRewriteSkipsGitIgnoredPaths(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("ignored/\n"), 0o644))

	goModContent := "module github.com/old/account/app\n\ngo 1.22\nrequire github.com/old/account/dep v1.0.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goModContent), 0o644))

	mainSource := `package main
import "github.com/old/account/dep"
func main() { dep.Do() }
`
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(mainSource), 0o644))

	ignoredDir := filepath.Join(tempDir, "ignored")
	require.NoError(t, os.MkdirAll(ignoredDir, 0o755))
	ignoredSource := `package ignored
import "github.com/old/account/dep"
func Do() { dep.Do() }
`
	require.NoError(t, os.WriteFile(filepath.Join(ignoredDir, "ignored.go"), []byte(ignoredSource), 0o644))

	oldPrefix, err := namespace.NewModulePrefix("github.com/old/account")
	require.NoError(t, err)
	newPrefix, err := namespace.NewModulePrefix("github.com/new/account")
	require.NoError(t, err)

	executor := &recordingGitExecutor{
		configValues: map[string]string{
			"user.name":  "Test User",
			"user.email": "test@example.com",
		},
		ignoredPaths: map[string]struct{}{
			"ignored/ignored.go": {},
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
	require.True(t, result.CommitCreated)
	commands := executor.recordedCommands()
	for _, command := range commands {
		require.NotEqual(t, "add ignored/ignored.go", command)
	}

	updatedMain, readErr := os.ReadFile(filepath.Join(tempDir, "main.go"))
	require.NoError(t, readErr)
	require.Contains(t, string(updatedMain), "github.com/new/account")
	require.NotContains(t, string(updatedMain), "github.com/old/account")

	ignoredContent, ignoredReadErr := os.ReadFile(filepath.Join(ignoredDir, "ignored.go"))
	require.NoError(t, ignoredReadErr)
	require.Contains(t, string(ignoredContent), "github.com/old/account")

	require.Contains(t, commands, "check-ignore --stdin")
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

func TestRewriteHandlesPushFailure(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))

	goModContent := "module github.com/old/account\n\ngo 1.22\n"
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goModContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n"), 0o644))

	oldPrefix, err := namespace.NewModulePrefix("github.com/old/account")
	require.NoError(t, err)
	newPrefix, err := namespace.NewModulePrefix("github.com/new/account")
	require.NoError(t, err)

	executionError := execshell.CommandFailedError{
		Command: execshell.ShellCommand{Name: execshell.CommandGit},
		Result:  execshell.ExecutionResult{ExitCode: 1, StandardError: "permission denied"},
	}
	executor := &recordingGitExecutor{
		configValues: map[string]string{
			"user.name":         "Test User",
			"user.email":        "test@example.com",
			"remote.origin.url": "git@example.com:old/account.git",
		},
		pushError: executionError,
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
		Push:           true,
		PushRemote:     "origin",
	})
	require.Error(t, rewriteErr)
	var operationError repoerrors.OperationError
	require.True(t, errors.As(rewriteErr, &operationError))
	require.Equal(t, string(repoerrors.ErrNamespacePushFailed), operationError.Code())
	require.True(t, result.CommitCreated)
	require.False(t, result.PushPerformed)
	require.Contains(t, result.PushSkippedReason, "push failed")

	require.Contains(t, executor.recordedCommands(), "config --get remote.origin.url")
}

func TestRewriteSkipsPushWhenRemoteMissing(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module github.com/old/account\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n"), 0o644))

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
		Push:           true,
		PushRemote:     "origin",
	})
	require.Error(t, rewriteErr)
	var opErr repoerrors.OperationError
	require.True(t, errors.As(rewriteErr, &opErr))
	require.Equal(t, string(repoerrors.ErrRemoteMissing), opErr.Code())
	require.Contains(t, result.PushSkippedReason, "remote origin not configured")
	require.True(t, result.CommitCreated)
	require.False(t, result.PushPerformed)
}

func TestRewriteSkipsPushWhenRemoteUpToDate(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module github.com/old/account\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main\n"), 0o644))

	oldPrefix, err := namespace.NewModulePrefix("github.com/old/account")
	require.NoError(t, err)
	newPrefix, err := namespace.NewModulePrefix("github.com/new/account")
	require.NoError(t, err)

	branchPrefix := "rewrite"
	timestamp := "20241124-120000Z"
	branchName := fmt.Sprintf("%s/%s", branchPrefix, timestamp)

	executor := &recordingGitExecutor{
		configValues: map[string]string{
			"user.name":         "Test User",
			"user.email":        "test@example.com",
			"remote.origin.url": "git@example.com:old/account.git",
		},
		headHash:   "abcdef1234567890",
		remoteRefs: map[string]string{"origin:" + branchName: "abcdef1234567890"},
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
		Push:           true,
		PushRemote:     "origin",
		BranchPrefix:   branchPrefix,
	})
	require.NoError(t, rewriteErr)
	require.False(t, result.PushPerformed)
	require.Contains(t, result.PushSkippedReason, "already up to date")

	commands := executor.recordedCommands()
	require.Contains(t, commands, "ls-remote --heads origin "+branchName)
}

type noopGitExecutor struct{}

func (noopGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	if len(details.Arguments) > 0 && details.Arguments[0] == "check-ignore" {
		return execshell.ExecutionResult{}, execshell.CommandFailedError{
			Command: execshell.ShellCommand{Name: execshell.CommandGit, Details: details},
			Result:  execshell.ExecutionResult{ExitCode: 1},
		}
	}
	return execshell.ExecutionResult{}, nil
}

func (noopGitExecutor) ExecuteGitHubCLI(_ context.Context, _ execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type recordingGitExecutor struct {
	commands     []execshell.CommandDetails
	staged       map[string]struct{}
	configValues map[string]string
	pushError    error
	headHash     string
	remoteRefs   map[string]string
	ignoredPaths map[string]struct{}
}

func (executor *recordingGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
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
		return execshell.ExecutionResult{StandardOutput: ""}, nil
	case "checkout":
		return execshell.ExecutionResult{}, nil
	case "check-ignore":
		if len(args) >= 2 && args[1] == "--stdin" {
			input := strings.TrimSpace(string(details.StandardInput))
			if len(input) == 0 {
				return execshell.ExecutionResult{}, execshell.CommandFailedError{
					Command: execshell.ShellCommand{Name: execshell.CommandGit, Details: details},
					Result:  execshell.ExecutionResult{ExitCode: 1},
				}
			}
			lines := strings.Split(input, "\n")
			matches := make([]string, 0, len(lines))
			for _, line := range lines {
				path := strings.TrimSpace(line)
				if len(path) == 0 {
					continue
				}
				if executor.ignoredPaths != nil {
					if _, exists := executor.ignoredPaths[path]; exists {
						matches = append(matches, path)
					}
				}
			}
			if len(matches) == 0 {
				return execshell.ExecutionResult{}, execshell.CommandFailedError{
					Command: execshell.ShellCommand{Name: execshell.CommandGit, Details: details},
					Result:  execshell.ExecutionResult{ExitCode: 1},
				}
			}
			return execshell.ExecutionResult{StandardOutput: strings.Join(matches, "\n") + "\n"}, nil
		}
		pathIndex := len(details.Arguments) - 1
		if pathIndex >= 1 {
			pathArg := details.Arguments[pathIndex]
			if executor.ignoredPaths != nil {
				if _, exists := executor.ignoredPaths[pathArg]; exists {
					return execshell.ExecutionResult{StandardOutput: pathArg + "\n"}, nil
				}
			}
		}
		return execshell.ExecutionResult{}, execshell.CommandFailedError{
			Command: execshell.ShellCommand{Name: execshell.CommandGit, Details: details},
			Result:  execshell.ExecutionResult{ExitCode: 1},
		}
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
		if executor.pushError != nil {
			return execshell.ExecutionResult{}, executor.pushError
		}
		return execshell.ExecutionResult{}, nil
	case "rev-parse":
		hash := executor.headHash
		if len(hash) == 0 {
			hash = "HEADHASH"
		}
		return execshell.ExecutionResult{StandardOutput: hash + "\n"}, nil
	case "ls-remote":
		if executor.remoteRefs != nil && len(details.Arguments) >= 4 {
			key := fmt.Sprintf("%s:%s", details.Arguments[2], details.Arguments[3])
			if hash, exists := executor.remoteRefs[key]; exists {
				output := fmt.Sprintf("%s\trefs/heads/%s\n", hash, details.Arguments[3])
				return execshell.ExecutionResult{StandardOutput: output}, nil
			}
		}
		return execshell.ExecutionResult{StandardOutput: ""}, nil
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
	if args[1] == "--get" && len(args) >= 3 {
		value := executor.configValues[args[2]]
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
