package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

		clean, err := execCtx.Environment.RepositoryManager.CheckCleanWorktree(ctx, execCtx.Repository.Path)
		if err != nil {
			return err
		}

		var statusEntries []string
		if !clean {
			statusEntries, err = execCtx.Environment.RepositoryManager.WorktreeStatus(ctx, execCtx.Repository.Path)
			if err != nil {
				statusEntries = []string{fmt.Sprintf("status_error:%s", err.Error())}
			} else {
				statusEntries = filterIgnoredStatusEntries(statusEntries, execCtx.ignoredDirtyPatterns)
				if len(statusEntries) == 0 {
					clean = true
				}
			}
		}

		if clean {
			execCtx.storeWorktreeCheck(true, nil)
			return nil
		}

		execCtx.storeWorktreeCheck(false, statusEntries)

		fields := map[string]string{}
		if len(statusEntries) > 0 {
			fields["status"] = strings.Join(statusEntries, ", ")
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
