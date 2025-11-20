package workflow

import (
	"context"
	"errors"
	"strings"

	"github.com/tyemirov/gix/internal/repos/worktree"
)

func newCleanWorktreeGuard() actionGuard {
	return newGuard(func(ctx context.Context, execCtx *ExecutionContext) error {
		if !execCtx.requireCleanWorktree() {
			return nil
		}
		if execCtx == nil || execCtx.Environment == nil || execCtx.Environment.RepositoryManager == nil {
			return errors.New("repository manager not configured")
		}

		if clean, status, known := execCtx.knownWorktreeState(); known {
			if clean {
				return nil
			}
			if len(status) == 0 {
				return newActionSkipError("repository dirty", nil)
			}
			return newActionSkipError("repository dirty", map[string]string{"status": strings.Join(status, ", ")})
		}

		statusResult, statusError := worktree.CheckStatus(ctx, execCtx.Environment.RepositoryManager, execCtx.Repository.Path, execCtx.ignoredDirtyPatterns)
		if statusError != nil {
			return statusError
		}

		if statusResult.Clean() {
			execCtx.storeWorktreeCheck(true, nil)
			return nil
		}

		execCtx.storeWorktreeCheck(false, statusResult.Entries)

		fields := map[string]string{}
		if len(statusResult.Entries) > 0 {
			fields["status"] = strings.Join(statusResult.Entries, ", ")
		}
		return newActionSkipError("repository dirty", fields)
	})
}

func newBranchAbsenceGuard(branchName string) actionGuard {
	return newGuard(func(ctx context.Context, execCtx *ExecutionContext) error {
		if execCtx == nil || execCtx.Environment == nil {
			return errors.New("environment not configured")
		}
		sanitized := strings.TrimSpace(branchName)
		if len(sanitized) == 0 {
			return nil
		}

		exists, err := branchExists(ctx, execCtx.Environment.GitExecutor, execCtx.Repository.Path, sanitized)
		if err != nil {
			return err
		}
		if exists {
			return newActionSkipError("branch exists", map[string]string{"branch": sanitized})
		}
		return nil
	})
}

func newRemoteConfiguredGuard(remoteName string) actionGuard {
	return newGuard(func(ctx context.Context, execCtx *ExecutionContext) error {
		if execCtx == nil || execCtx.Environment == nil || execCtx.Environment.RepositoryManager == nil {
			return errors.New("repository manager not configured")
		}
		remote := strings.TrimSpace(remoteName)
		if len(remote) == 0 {
			return newActionSkipError("push remote not configured (set task.branch.push_remote)", nil)
		}

		remoteURL, remoteErr := execCtx.Environment.RepositoryManager.GetRemoteURL(ctx, execCtx.Repository.Path, remote)
		if remoteErr != nil {
			return newActionSkipError("remote lookup failed", map[string]string{"remote": remote, "error": remoteErr.Error()})
		}
		if len(strings.TrimSpace(remoteURL)) == 0 {
			return newActionSkipError("remote missing", map[string]string{"remote": remote})
		}
		return nil
	})
}
