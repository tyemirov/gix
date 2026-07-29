package syncflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	gitWorktreeAddSubcommandConstant = "add"

	strictSyncStartingWorktreeMissingTemplate = "strict sync starting worktree %s is missing from Git worktree metadata"
	strictSyncRestoreCheckoutFailureTemplate  = "failed to restore starting checkout %s: %w"
	strictSyncRestoreWorktreeFailureTemplate  = "failed to restore adopted worktree %s: %w"
	strictSyncRestoreTopologyChangedTemplate  = "cannot restore adopted worktree %s because it now holds branch %q"
	strictSyncDetachedCommitMissingMessage    = "detached worktree commit is missing"
	strictSyncRollbackMessage                 = "failed sync restored the starting checkout and adopted worktree topology; gix did not leave the target branch active"
	strictSyncHandoffMessage                  = "failed sync could not restore the starting checkout and adopted worktree topology"
)

type strictSyncTransaction struct {
	environment      *workflow.Environment
	repository       *workflow.RepositoryState
	startingWorktree listedWorktree
	worktrees        []listedWorktree
	targetBranch     string
}

type strictSyncRollbackResult struct {
	CheckoutRestored  bool
	WorktreesRestored int
}

func beginStrictSyncTransaction(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, targetBranch string) (strictSyncTransaction, error) {
	worktrees, listErr := listRepositoryWorktrees(ctx, environment.GitExecutor, repository.Path, targetBranch)
	if listErr != nil {
		return strictSyncTransaction{}, listErr
	}

	startingWorktree, exists := findListedWorktreeByPath(worktrees, repository.Path)
	if !exists {
		return strictSyncTransaction{}, fmt.Errorf(strictSyncStartingWorktreeMissingTemplate, repository.Path)
	}

	return strictSyncTransaction{
		environment:      environment,
		repository:       repository,
		startingWorktree: startingWorktree,
		worktrees:        worktrees,
		targetBranch:     strings.TrimSpace(targetBranch),
	}, nil
}

func (transaction strictSyncTransaction) rollback(ctx context.Context) (strictSyncRollbackResult, error) {
	currentWorktrees, listErr := listRepositoryWorktrees(ctx, transaction.environment.GitExecutor, transaction.repository.Path, transaction.targetBranch)
	if listErr != nil {
		return strictSyncRollbackResult{}, listErr
	}
	currentWorktree, exists := findListedWorktreeByPath(currentWorktrees, transaction.repository.Path)
	if !exists {
		return strictSyncRollbackResult{}, fmt.Errorf(strictSyncStartingWorktreeMissingTemplate, transaction.repository.Path)
	}

	result := strictSyncRollbackResult{}
	if !sameListedWorktreeCheckout(currentWorktree, transaction.startingWorktree) {
		if restoreErr := restoreListedWorktreeCheckout(ctx, transaction.environment.GitExecutor, transaction.repository.Path, transaction.startingWorktree); restoreErr != nil {
			return result, fmt.Errorf(strictSyncRestoreCheckoutFailureTemplate, transaction.repository.Path, restoreErr)
		}
		result.CheckoutRestored = true
	}

	refreshedWorktrees, refreshedListErr := listRepositoryWorktrees(ctx, transaction.environment.GitExecutor, transaction.repository.Path, transaction.targetBranch)
	if refreshedListErr != nil {
		return result, refreshedListErr
	}

	for worktreeIndex := range transaction.worktrees {
		startingSibling := transaction.worktrees[worktreeIndex]
		if startingSibling.Prunable || sameFilesystemPath(startingSibling.Path, transaction.repository.Path) {
			continue
		}
		currentSibling, siblingExists := findListedWorktreeByPath(refreshedWorktrees, startingSibling.Path)
		if siblingExists {
			if sameListedWorktreeCheckout(currentSibling, startingSibling) {
				continue
			}
			if currentSibling.BranchName != "" {
				return result, fmt.Errorf(strictSyncRestoreTopologyChangedTemplate, startingSibling.Path, currentSibling.BranchName)
			}
			if restoreErr := restoreListedWorktreeCheckout(ctx, transaction.environment.GitExecutor, startingSibling.Path, startingSibling); restoreErr != nil {
				return result, fmt.Errorf(strictSyncRestoreWorktreeFailureTemplate, startingSibling.Path, restoreErr)
			}
			result.WorktreesRestored++
			continue
		}

		if restoreErr := addListedWorktree(ctx, transaction.environment.GitExecutor, transaction.repository.Path, startingSibling); restoreErr != nil {
			return result, fmt.Errorf(strictSyncRestoreWorktreeFailureTemplate, startingSibling.Path, restoreErr)
		}
		result.WorktreesRestored++
	}

	if result.CheckoutRestored || result.WorktreesRestored > 0 {
		transaction.reportRollback(result)
	}
	return result, nil
}

func (transaction strictSyncTransaction) reportRollback(result strictSyncRollbackResult) {
	startingReference := transaction.startingWorktree.BranchName
	if startingReference == "" {
		startingReference = transaction.startingWorktree.Commit
	}
	transaction.environment.ReportRepositoryEvent(
		transaction.repository,
		shared.EventLevelError,
		shared.EventCodeSyncSwitchRollback,
		strictSyncRollbackMessage,
		map[string]string{
			"starting_reference": startingReference,
			"target_branch":      transaction.targetBranch,
			"checkout_restored":  fmt.Sprintf("%t", result.CheckoutRestored),
			"worktrees_restored": fmt.Sprintf("%d", result.WorktreesRestored),
		},
	)
}

func (transaction strictSyncTransaction) reportHandoff(rollbackErr error) {
	transaction.environment.ReportRepositoryEvent(
		transaction.repository,
		shared.EventLevelError,
		shared.EventCodeSyncSwitchHandoff,
		fmt.Sprintf("%s: %s", strictSyncHandoffMessage, strings.TrimSpace(rollbackErr.Error())),
		map[string]string{
			"target_branch": transaction.targetBranch,
			"reason":        strings.TrimSpace(rollbackErr.Error()),
		},
	)
}

func findListedWorktreeByPath(worktrees []listedWorktree, worktreePath string) (listedWorktree, bool) {
	for worktreeIndex := range worktrees {
		if sameFilesystemPath(worktrees[worktreeIndex].Path, worktreePath) {
			return worktrees[worktreeIndex], true
		}
	}
	return listedWorktree{}, false
}

func sameListedWorktreeCheckout(first listedWorktree, second listedWorktree) bool {
	if first.BranchName != "" || second.BranchName != "" {
		return first.BranchName == second.BranchName
	}
	return first.Commit == second.Commit
}

func restoreListedWorktreeCheckout(ctx context.Context, executor shared.GitExecutor, worktreePath string, worktree listedWorktree) error {
	if worktree.BranchName != "" {
		return executeGit(ctx, executor, worktreePath, []string{gitSwitchSubcommandConstant, worktree.BranchName})
	}
	if worktree.Commit == "" {
		return errors.New(strictSyncDetachedCommitMissingMessage)
	}
	return executeGit(ctx, executor, worktreePath, []string{gitSwitchSubcommandConstant, gitSwitchDetachFlagConstant, worktree.Commit})
}

func addListedWorktree(ctx context.Context, executor shared.GitExecutor, repositoryPath string, worktree listedWorktree) error {
	arguments := []string{gitWorktreeSubcommandConstant, gitWorktreeAddSubcommandConstant}
	if worktree.BranchName != "" {
		arguments = append(arguments, worktree.Path, worktree.BranchName)
	} else {
		if worktree.Commit == "" {
			return errors.New(strictSyncDetachedCommitMissingMessage)
		}
		arguments = append(arguments, gitSwitchDetachFlagConstant, worktree.Path, worktree.Commit)
	}
	return executeGit(ctx, executor, repositoryPath, arguments)
}
