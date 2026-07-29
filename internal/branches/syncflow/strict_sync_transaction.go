package syncflow

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	gitWorktreeAddSubcommandConstant = "add"
	gitCleanSubcommandConstant       = "clean"
	gitCleanDirectoriesFlagConstant  = "-d"
	gitCleanForceFlagConstant        = "-f"
	gitForEachRefSubcommandConstant  = "for-each-ref"
	gitUpdateRefSubcommandConstant   = "update-ref"
	gitDeleteRefFlagConstant         = "-d"
	gitLocalHeadsPrefixConstant      = "refs/heads/"
	gitLocalHeadsFormatConstant      = "--format=%(refname) %(objectname)"

	strictSyncStartingWorktreeMissingTemplate = "strict sync starting worktree %s is missing from Git worktree metadata"
	strictSyncSnapshotRefsFailureTemplate     = "capture strict sync local branches at %s: %w"
	strictSyncSnapshotStatusFailureTemplate   = "inspect strict sync transaction snapshot at %s: %w"
	strictSyncSnapshotFailureTemplate         = "capture strict sync transaction snapshot at %s: %w"
	strictSyncFinalizeFailureTemplate         = "finalize strict sync transaction snapshot at %s: %w"
	strictSyncRestoreCheckoutFailureTemplate  = "failed to restore starting checkout %s: %w"
	strictSyncRestoreWorktreeFailureTemplate  = "failed to restore adopted worktree %s: %w"
	strictSyncRestoreTopologyChangedTemplate  = "cannot restore adopted worktree %s because it now holds branch %q"
	strictSyncDetachedCommitMissingMessage    = "detached worktree commit is missing"
	strictSyncRollbackMessage                 = "failed sync restored the starting checkout, local state, and adopted worktree topology; gix did not leave the target branch active"
	strictSyncHandoffMessage                  = "failed sync could not restore the starting checkout, local state, and adopted worktree topology"
	strictSyncPublishedHandoffMessage         = "strict sync published remote changes but finalization failed; gix preserved the invocation-owned recovery state and did not report SYNCED"
)

type strictSyncTransaction struct {
	environment      *workflow.Environment
	repository       *workflow.RepositoryState
	startingWorktree listedWorktree
	worktrees        []listedWorktree
	touchedWorktrees []strictSyncWorktreeSnapshot
	ownedStashes     []strictSyncStash
	localBranches    map[string]string
	targetBranch     string
	published        bool
}

type strictSyncTransactionContextKey struct{}

type strictSyncWorktreeSnapshot struct {
	Worktree      listedWorktree
	Backup        *strictSyncStash
	BackupDropped bool
}

type strictSyncRollbackResult struct {
	CheckoutRestored  bool
	WorktreesRestored int
}

func beginStrictSyncTransaction(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, plan strictSyncPlan) (*strictSyncTransaction, error) {
	transaction := &strictSyncTransaction{
		environment:      environment,
		repository:       repository,
		startingWorktree: plan.startingWorktree,
		worktrees:        append([]listedWorktree(nil), plan.worktrees...),
		targetBranch:     plan.targetBranch,
	}
	localBranches, localBranchesErr := captureStrictSyncLocalBranches(ctx, environment.GitExecutor, repository.Path)
	if localBranchesErr != nil {
		return nil, localBranchesErr
	}
	transaction.localBranches = localBranches

	for worktreeIndex := range plan.touchedWorktrees {
		worktree := plan.touchedWorktrees[worktreeIndex]
		snapshot, snapshotErr := transaction.captureWorktreeSnapshot(ctx, worktree)
		transaction.touchedWorktrees = append(transaction.touchedWorktrees, snapshot)
		if snapshotErr != nil {
			cleanupContext, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), mergeConflictResolutionRollbackTimeout)
			_, rollbackErr := transaction.rollback(cleanupContext)
			cancelCleanup()
			return nil, errors.Join(snapshotErr, rollbackErr)
		}
	}
	return transaction, nil
}

func (transaction *strictSyncTransaction) captureWorktreeSnapshot(ctx context.Context, worktree listedWorktree) (strictSyncWorktreeSnapshot, error) {
	snapshot := strictSyncWorktreeSnapshot{Worktree: worktree}
	result, statusErr := transaction.environment.GitExecutor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitStatusSubcommand, gitPorcelainFlagConstant, "--untracked-files=all"},
		WorkingDirectory: worktree.Path,
	})
	if statusErr != nil {
		return snapshot, fmt.Errorf(strictSyncSnapshotStatusFailureTemplate, worktree.Path, statusErr)
	}
	if strings.TrimSpace(result.StandardOutput) == "" {
		return snapshot, nil
	}

	backup, backupErr := pushStrictSyncStash(ctx, transaction.environment.GitExecutor, worktree.Path, strictSyncTransactionStashMessage)
	if backupErr != nil {
		return snapshot, fmt.Errorf(strictSyncSnapshotFailureTemplate, worktree.Path, backupErr)
	}
	snapshot.Backup = &backup
	if applyErr := applyStrictSyncStash(ctx, transaction.environment.GitExecutor, backup); applyErr != nil {
		return snapshot, fmt.Errorf(strictSyncSnapshotFailureTemplate, worktree.Path, applyErr)
	}
	return snapshot, nil
}

func (transaction *strictSyncTransaction) finalize(ctx context.Context) error {
	if len(transaction.ownedStashes) > 0 {
		return fmt.Errorf(strictSyncFinalizeFailureTemplate, transaction.repository.Path, errors.New("strict sync invocation stash was not restored"))
	}
	return transaction.finalizeSnapshots(ctx)
}

func (transaction *strictSyncTransaction) finalizeSnapshots(ctx context.Context) error {
	for snapshotIndex := len(transaction.touchedWorktrees) - 1; snapshotIndex >= 0; snapshotIndex-- {
		snapshot := &transaction.touchedWorktrees[snapshotIndex]
		if snapshot.Backup == nil || snapshot.BackupDropped {
			continue
		}
		if dropErr := dropStrictSyncStash(ctx, transaction.environment.GitExecutor, *snapshot.Backup); dropErr != nil {
			return fmt.Errorf(strictSyncFinalizeFailureTemplate, snapshot.Worktree.Path, dropErr)
		}
		snapshot.BackupDropped = true
	}
	return nil
}

func withStrictSyncTransaction(ctx context.Context, transaction *strictSyncTransaction) context.Context {
	return context.WithValue(ctx, strictSyncTransactionContextKey{}, transaction)
}

func markStrictSyncPublished(ctx context.Context) {
	transaction, ok := ctx.Value(strictSyncTransactionContextKey{}).(*strictSyncTransaction)
	if !ok || transaction == nil {
		return
	}
	transaction.published = true
}

func protectStrictSyncWorktree(ctx context.Context, worktree listedWorktree) error {
	transaction, ok := ctx.Value(strictSyncTransactionContextKey{}).(*strictSyncTransaction)
	if !ok || transaction == nil {
		return nil
	}
	for snapshotIndex := range transaction.touchedWorktrees {
		if sameFilesystemPath(transaction.touchedWorktrees[snapshotIndex].Worktree.Path, worktree.Path) {
			return nil
		}
	}
	if operationErr := ensureWorktreeHasNoOperatorOwnedGitOperation(ctx, transaction.environment.GitExecutor, worktree.Path); operationErr != nil {
		return operationErr
	}
	snapshot, snapshotErr := transaction.captureWorktreeSnapshot(ctx, worktree)
	transaction.touchedWorktrees = append(transaction.touchedWorktrees, snapshot)
	return snapshotErr
}

func (transaction *strictSyncTransaction) ownStash(stash strictSyncStash) {
	transaction.ownedStashes = append(transaction.ownedStashes, stash)
}

func (transaction *strictSyncTransaction) releaseStash(stash strictSyncStash) {
	for stashIndex := range transaction.ownedStashes {
		if transaction.ownedStashes[stashIndex].CommitID != stash.CommitID {
			continue
		}
		transaction.ownedStashes = append(transaction.ownedStashes[:stashIndex], transaction.ownedStashes[stashIndex+1:]...)
		return
	}
}

func (transaction *strictSyncTransaction) rollback(ctx context.Context) (strictSyncRollbackResult, error) {
	currentWorktrees, listErr := listRepositoryWorktrees(ctx, transaction.environment.GitExecutor, transaction.repository.Path, transaction.targetBranch)
	if listErr != nil {
		return strictSyncRollbackResult{}, listErr
	}
	currentWorktree, exists := findListedWorktreeByPath(currentWorktrees, transaction.repository.Path)
	if !exists {
		return strictSyncRollbackResult{}, fmt.Errorf(strictSyncStartingWorktreeMissingTemplate, transaction.repository.Path)
	}

	result := strictSyncRollbackResult{}
	if clearErr := resetAndCleanWorktree(ctx, transaction.environment.GitExecutor, transaction.repository.Path, currentWorktree.Commit); clearErr != nil {
		return result, fmt.Errorf(strictSyncRestoreCheckoutFailureTemplate, transaction.repository.Path, clearErr)
	}
	if !sameListedWorktreeCheckout(currentWorktree, transaction.startingWorktree) {
		if restoreErr := restoreListedWorktreeCheckout(ctx, transaction.environment.GitExecutor, transaction.repository.Path, transaction.startingWorktree); restoreErr != nil {
			return result, fmt.Errorf(strictSyncRestoreCheckoutFailureTemplate, transaction.repository.Path, restoreErr)
		}
		result.CheckoutRestored = true
	}
	if restoreErr := resetAndCleanWorktree(ctx, transaction.environment.GitExecutor, transaction.repository.Path, transaction.startingWorktree.Commit); restoreErr != nil {
		return result, fmt.Errorf(strictSyncRestoreCheckoutFailureTemplate, transaction.repository.Path, restoreErr)
	}
	if restoreErr := transaction.restoreLocalBranches(ctx); restoreErr != nil {
		return result, restoreErr
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
			if currentSibling.BranchName != "" && currentSibling.BranchName != startingSibling.BranchName {
				return result, fmt.Errorf(strictSyncRestoreTopologyChangedTemplate, startingSibling.Path, currentSibling.BranchName)
			}
			if restoreErr := restoreListedWorktreeCheckout(ctx, transaction.environment.GitExecutor, startingSibling.Path, startingSibling); restoreErr != nil {
				return result, fmt.Errorf(strictSyncRestoreWorktreeFailureTemplate, startingSibling.Path, restoreErr)
			}
			if restoreErr := resetAndCleanWorktree(ctx, transaction.environment.GitExecutor, startingSibling.Path, startingSibling.Commit); restoreErr != nil {
				return result, fmt.Errorf(strictSyncRestoreWorktreeFailureTemplate, startingSibling.Path, restoreErr)
			}
			result.WorktreesRestored++
			continue
		}

		if restoreErr := addListedWorktree(ctx, transaction.environment.GitExecutor, transaction.repository.Path, startingSibling); restoreErr != nil {
			return result, fmt.Errorf(strictSyncRestoreWorktreeFailureTemplate, startingSibling.Path, restoreErr)
		}
		if restoreErr := resetAndCleanWorktree(ctx, transaction.environment.GitExecutor, startingSibling.Path, startingSibling.Commit); restoreErr != nil {
			return result, fmt.Errorf(strictSyncRestoreWorktreeFailureTemplate, startingSibling.Path, restoreErr)
		}
		result.WorktreesRestored++
	}

	for snapshotIndex := range transaction.touchedWorktrees {
		snapshot := transaction.touchedWorktrees[snapshotIndex]
		if restoreErr := resetAndCleanWorktree(ctx, transaction.environment.GitExecutor, snapshot.Worktree.Path, snapshot.Worktree.Commit); restoreErr != nil {
			return result, fmt.Errorf(strictSyncRestoreWorktreeFailureTemplate, snapshot.Worktree.Path, restoreErr)
		}
		if snapshot.Backup == nil {
			continue
		}
		if restoreErr := applyStrictSyncStash(ctx, transaction.environment.GitExecutor, *snapshot.Backup); restoreErr != nil {
			return result, fmt.Errorf(strictSyncRestoreWorktreeFailureTemplate, snapshot.Worktree.Path, restoreErr)
		}
	}
	for stashIndex := len(transaction.ownedStashes) - 1; stashIndex >= 0; stashIndex-- {
		if dropErr := dropStrictSyncStash(ctx, transaction.environment.GitExecutor, transaction.ownedStashes[stashIndex]); dropErr != nil {
			return result, dropErr
		}
	}
	transaction.ownedStashes = nil
	if finalizeErr := transaction.finalize(ctx); finalizeErr != nil {
		return result, finalizeErr
	}

	if result.CheckoutRestored || result.WorktreesRestored > 0 || len(transaction.touchedWorktrees) > 0 {
		transaction.reportRollback(result)
	}
	return result, nil
}

func (transaction *strictSyncTransaction) reportRollback(result strictSyncRollbackResult) {
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

func (transaction *strictSyncTransaction) reportHandoff(rollbackErr error) {
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

func (transaction *strictSyncTransaction) reportPublishedHandoff(reason error) {
	transaction.environment.ReportRepositoryEvent(
		transaction.repository,
		shared.EventLevelError,
		shared.EventCodeSyncSwitchHandoff,
		strictSyncPublishedHandoffMessage,
		map[string]string{
			"target_branch": transaction.targetBranch,
			"reason":        strings.TrimSpace(reason.Error()),
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
	return first.BranchName == second.BranchName && first.Commit == second.Commit
}

func restoreListedWorktreeCheckout(ctx context.Context, executor shared.GitExecutor, worktreePath string, worktree listedWorktree) error {
	if worktree.BranchName != "" {
		if switchErr := executeGit(ctx, executor, worktreePath, []string{gitSwitchSubcommandConstant, worktree.BranchName}); switchErr != nil {
			return switchErr
		}
		return resetAndCleanWorktree(ctx, executor, worktreePath, worktree.Commit)
	}
	if worktree.Commit == "" {
		return errors.New(strictSyncDetachedCommitMissingMessage)
	}
	if switchErr := executeGit(ctx, executor, worktreePath, []string{gitSwitchSubcommandConstant, gitSwitchDetachFlagConstant, worktree.Commit}); switchErr != nil {
		return switchErr
	}
	return resetAndCleanWorktree(ctx, executor, worktreePath, worktree.Commit)
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

func resetAndCleanWorktree(ctx context.Context, executor shared.GitExecutor, worktreePath string, commitID string) error {
	if strings.TrimSpace(commitID) == "" {
		return errors.New(strictSyncDetachedCommitMissingMessage)
	}
	if resetErr := executeGit(ctx, executor, worktreePath, []string{gitResetSubcommandConstant, gitResetHardFlagConstant, commitID}); resetErr != nil {
		return resetErr
	}
	return executeGit(ctx, executor, worktreePath, []string{gitCleanSubcommandConstant, gitCleanDirectoriesFlagConstant, gitCleanForceFlagConstant})
}

func captureStrictSyncLocalBranches(ctx context.Context, executor shared.GitExecutor, repositoryPath string) (map[string]string, error) {
	result, listErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitForEachRefSubcommandConstant, gitLocalHeadsFormatConstant, gitLocalHeadsPrefixConstant},
		WorkingDirectory: repositoryPath,
	})
	if listErr != nil {
		return nil, fmt.Errorf(strictSyncSnapshotRefsFailureTemplate, repositoryPath, listErr)
	}
	branches := make(map[string]string)
	for _, line := range strings.Split(result.StandardOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 || !strings.HasPrefix(fields[0], gitLocalHeadsPrefixConstant) {
			return nil, fmt.Errorf(strictSyncSnapshotRefsFailureTemplate, repositoryPath, fmt.Errorf("invalid local branch record %q", line))
		}
		branches[fields[0]] = fields[1]
	}
	return branches, nil
}

func (transaction *strictSyncTransaction) restoreLocalBranches(ctx context.Context) error {
	currentBranches, currentBranchesErr := captureStrictSyncLocalBranches(ctx, transaction.environment.GitExecutor, transaction.repository.Path)
	if currentBranchesErr != nil {
		return currentBranchesErr
	}
	startingReferences := sortedStrictSyncReferenceNames(transaction.localBranches)
	for referenceIndex := range startingReferences {
		referenceName := startingReferences[referenceIndex]
		commitID := transaction.localBranches[referenceName]
		if updateErr := executeGit(ctx, transaction.environment.GitExecutor, transaction.repository.Path, []string{gitUpdateRefSubcommandConstant, referenceName, commitID}); updateErr != nil {
			return updateErr
		}
	}
	currentReferences := sortedStrictSyncReferenceNames(currentBranches)
	for referenceIndex := range currentReferences {
		referenceName := currentReferences[referenceIndex]
		if _, existed := transaction.localBranches[referenceName]; existed {
			continue
		}
		if deleteErr := executeGit(ctx, transaction.environment.GitExecutor, transaction.repository.Path, []string{gitUpdateRefSubcommandConstant, gitDeleteRefFlagConstant, referenceName}); deleteErr != nil {
			return deleteErr
		}
	}
	return nil
}

func sortedStrictSyncReferenceNames(references map[string]string) []string {
	names := make([]string, 0, len(references))
	for referenceName := range references {
		names = append(names, referenceName)
	}
	sort.Strings(names)
	return names
}
