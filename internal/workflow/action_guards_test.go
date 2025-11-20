package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/worktree"
)

func TestCleanWorktreeGuardIgnoresConfiguredPatterns(t *testing.T) {
	guard := newCleanWorktreeGuard()

	manager, err := gitrepo.NewRepositoryManager(stubStatusExecutor{output: "?? .DS_Store\n"})
	require.NoError(t, err)

	execCtx := &ExecutionContext{
		Environment:  &Environment{RepositoryManager: manager},
		Repository:   &RepositoryState{Path: "/tmp/repo"},
		requireClean: true,
		ignoredDirtyPatterns: worktree.BuildIgnorePatterns([]string{
			".DS_Store",
		}),
	}

	require.NoError(t, guard.Check(context.Background(), execCtx))
}

func TestCleanWorktreeGuardRejectsNonIgnoredChanges(t *testing.T) {
	guard := newCleanWorktreeGuard()

	manager, err := gitrepo.NewRepositoryManager(stubStatusExecutor{output: " M README.md\n"})
	require.NoError(t, err)

	execCtx := &ExecutionContext{
		Environment:  &Environment{RepositoryManager: manager},
		Repository:   &RepositoryState{Path: "/tmp/repo"},
		requireClean: true,
		ignoredDirtyPatterns: worktree.BuildIgnorePatterns([]string{
			".DS_Store",
		}),
	}

	err = guard.Check(context.Background(), execCtx)
	require.Error(t, err)

	var skipErr actionSkipError
	require.True(t, errors.As(err, &skipErr))
	require.Equal(t, "repository dirty", skipErr.reason)
	require.Contains(t, skipErr.fields["status"], "README.md")
}

func TestCleanWorktreeGuardIgnoresUntrackedWithoutPatterns(t *testing.T) {
	guard := newCleanWorktreeGuard()

	manager, err := gitrepo.NewRepositoryManager(stubStatusExecutor{output: "?? scratch.txt\n"})
	require.NoError(t, err)

	execCtx := &ExecutionContext{
		Environment:  &Environment{RepositoryManager: manager},
		Repository:   &RepositoryState{Path: "/tmp/repo"},
		requireClean: true,
	}

	require.NoError(t, guard.Check(context.Background(), execCtx))
}

type stubStatusExecutor struct {
	output string
}

func (executor stubStatusExecutor) ExecuteGit(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{StandardOutput: executor.output}, nil
}
