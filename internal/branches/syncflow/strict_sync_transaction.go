package syncflow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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
	gitAbbrevRefFlagConstant         = "--abbrev-ref"
	gitHeadReferenceConstant         = "HEAD"

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
	strictSyncReferenceOwnershipTemplate      = "local branch %s changed outside the strict-sync transaction: expected %s, found %s"
	strictSyncPushStatusMissingMessage        = "successful strict-sync push did not return a porcelain ref status"
	strictSyncPushStatusInvalidTemplate       = "successful strict-sync push returned unknown porcelain status %q"
)

type strictSyncTransaction struct {
	environment       *workflow.Environment
	repository        *workflow.RepositoryState
	startingWorktree  listedWorktree
	adoptedWorktrees  []listedWorktree
	touchedWorktrees  []strictSyncWorktreeSnapshot
	ownedStashes      []strictSyncStash
	localBranches     map[string]string
	branchMutations   map[string]strictSyncBranchMutation
	worktreeMutations map[string]struct{}
	targetBranch      string
	published         bool
	restoring         bool
}

type strictSyncTransactionContextKey struct{}
type strictSyncJournalSuppressionContextKey struct{}

type strictSyncBranchMutation struct {
	InitialCommit  string
	InitialExists  bool
	ExpectedCommit string
	ExpectedExists bool
}

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
		environment:       environment,
		repository:        repository,
		startingWorktree:  plan.startingWorktree,
		branchMutations:   make(map[string]strictSyncBranchMutation),
		worktreeMutations: make(map[string]struct{}),
		targetBranch:      plan.targetBranch,
	}
	localBranches, localBranchesErr := captureStrictSyncLocalBranches(ctx, environment.GitExecutor, repository.Path)
	if localBranchesErr != nil {
		return nil, localBranchesErr
	}
	transaction.localBranches = localBranches

	for worktreeIndex := range plan.touchedWorktrees {
		worktree := plan.touchedWorktrees[worktreeIndex]
		snapshot, snapshotErr := transaction.captureWorktreeSnapshot(ctx, worktree)
		if snapshotErr != nil {
			cleanupContext, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), mergeConflictResolutionRollbackTimeout)
			var cleanupErr error
			if snapshot.Backup != nil {
				transaction.touchedWorktrees = append(transaction.touchedWorktrees, snapshot)
				transaction.markWorktreeMutated(worktree.Path)
				_, cleanupErr = transaction.rollback(cleanupContext)
			} else {
				cleanupErr = transaction.finalizeSnapshots(cleanupContext)
			}
			cancelCleanup()
			return nil, errors.Join(snapshotErr, cleanupErr)
		}
		transaction.touchedWorktrees = append(transaction.touchedWorktrees, snapshot)
	}
	return transaction, nil
}

func (transaction *strictSyncTransaction) captureWorktreeSnapshot(ctx context.Context, worktree listedWorktree) (strictSyncWorktreeSnapshot, error) {
	snapshot := strictSyncWorktreeSnapshot{Worktree: worktree}
	snapshotContext := suppressStrictSyncMutationJournal(ctx)
	if unmergedErr := ensureWorktreeHasNoOperatorOwnedUnmergedIndex(snapshotContext, transaction.environment.GitExecutor, worktree.Path); unmergedErr != nil {
		return snapshot, unmergedErr
	}
	result, statusErr := transaction.environment.GitExecutor.ExecuteGit(snapshotContext, execshell.CommandDetails{
		Arguments:        []string{gitStatusSubcommand, gitPorcelainFlagConstant, "--untracked-files=all"},
		WorkingDirectory: worktree.Path,
	})
	if statusErr != nil {
		return snapshot, fmt.Errorf(strictSyncSnapshotStatusFailureTemplate, worktree.Path, statusErr)
	}
	if strings.TrimSpace(result.StandardOutput) == "" {
		return snapshot, nil
	}

	backup, backupErr := pushStrictSyncStash(snapshotContext, transaction.environment.GitExecutor, worktree.Path, strictSyncTransactionStashMessage)
	if backupErr != nil {
		return snapshot, fmt.Errorf(strictSyncSnapshotFailureTemplate, worktree.Path, backupErr)
	}
	snapshot.Backup = &backup
	if applyErr := applyStrictSyncStash(snapshotContext, transaction.environment.GitExecutor, backup); applyErr != nil {
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

func suppressStrictSyncMutationJournal(ctx context.Context) context.Context {
	return context.WithValue(ctx, strictSyncJournalSuppressionContextKey{}, true)
}

func strictSyncTransactionFromContext(ctx context.Context) (*strictSyncTransaction, bool) {
	transaction, ok := ctx.Value(strictSyncTransactionContextKey{}).(*strictSyncTransaction)
	return transaction, ok && transaction != nil
}

func markStrictSyncPublished(ctx context.Context) {
	transaction, ok := strictSyncTransactionFromContext(ctx)
	if !ok {
		return
	}
	transaction.published = true
}

func protectStrictSyncWorktree(ctx context.Context, worktree listedWorktree) error {
	transaction, ok := strictSyncTransactionFromContext(ctx)
	if !ok {
		return nil
	}
	if operationErr := ensureWorktreeHasNoOperatorOwnedGitOperation(ctx, transaction.environment.GitExecutor, worktree.Path); operationErr != nil {
		return operationErr
	}
	if unmergedErr := ensureWorktreeHasNoOperatorOwnedUnmergedIndex(ctx, transaction.environment.GitExecutor, worktree.Path); unmergedErr != nil {
		return unmergedErr
	}
	for snapshotIndex := range transaction.touchedWorktrees {
		if sameFilesystemPath(transaction.touchedWorktrees[snapshotIndex].Worktree.Path, worktree.Path) {
			transaction.ownAdoptedWorktree(worktree)
			return nil
		}
	}
	snapshot, snapshotErr := transaction.captureWorktreeSnapshot(ctx, worktree)
	if snapshotErr != nil {
		if snapshot.Backup != nil {
			transaction.touchedWorktrees = append(transaction.touchedWorktrees, snapshot)
			transaction.markWorktreeMutated(worktree.Path)
		}
		return snapshotErr
	}
	transaction.touchedWorktrees = append(transaction.touchedWorktrees, snapshot)
	transaction.ownAdoptedWorktree(worktree)
	return nil
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

func (transaction *strictSyncTransaction) ownAdoptedWorktree(worktree listedWorktree) {
	for worktreeIndex := range transaction.adoptedWorktrees {
		if sameFilesystemPath(transaction.adoptedWorktrees[worktreeIndex].Path, worktree.Path) {
			transaction.markWorktreeMutated(worktree.Path)
			return
		}
	}
	transaction.adoptedWorktrees = append(transaction.adoptedWorktrees, worktree)
	transaction.markWorktreeMutated(worktree.Path)
}

func (transaction *strictSyncTransaction) markWorktreeMutated(worktreePath string) {
	if transaction.worktreeMutations == nil {
		transaction.worktreeMutations = make(map[string]struct{})
	}
	transaction.worktreeMutations[filepath.Clean(strings.TrimSpace(worktreePath))] = struct{}{}
}

func (transaction *strictSyncTransaction) worktreeMutated(worktreePath string) bool {
	_, mutated := transaction.worktreeMutations[filepath.Clean(strings.TrimSpace(worktreePath))]
	return mutated
}

func (transaction *strictSyncTransaction) rollback(ctx context.Context) (strictSyncRollbackResult, error) {
	transaction.restoring = true
	defer func() {
		transaction.restoring = false
	}()

	currentWorktrees, listErr := listRepositoryWorktrees(ctx, transaction.environment.GitExecutor, transaction.repository.Path, transaction.targetBranch)
	if listErr != nil {
		return strictSyncRollbackResult{}, listErr
	}
	currentWorktree, exists := findListedWorktreeByPath(currentWorktrees, transaction.repository.Path)
	if !exists {
		return strictSyncRollbackResult{}, fmt.Errorf(strictSyncStartingWorktreeMissingTemplate, transaction.repository.Path)
	}
	if ownershipErr := transaction.validateRollbackReferenceOwnership(ctx); ownershipErr != nil {
		return strictSyncRollbackResult{}, ownershipErr
	}

	result := strictSyncRollbackResult{}
	callerMutated := transaction.worktreeMutated(transaction.repository.Path) || !sameListedWorktreeCheckout(currentWorktree, transaction.startingWorktree)
	if callerMutated {
		if clearErr := resetAndCleanWorktree(ctx, transaction.environment.GitExecutor, transaction.repository.Path, currentWorktree.Commit); clearErr != nil {
			return result, fmt.Errorf(strictSyncRestoreCheckoutFailureTemplate, transaction.repository.Path, clearErr)
		}
	}
	if callerMutated && !sameListedWorktreeCheckout(currentWorktree, transaction.startingWorktree) {
		if restoreErr := restoreListedWorktreeCheckout(ctx, transaction.environment.GitExecutor, transaction.repository.Path, transaction.startingWorktree); restoreErr != nil {
			return result, fmt.Errorf(strictSyncRestoreCheckoutFailureTemplate, transaction.repository.Path, restoreErr)
		}
		result.CheckoutRestored = true
	}
	if restoreErr := transaction.restoreLocalBranches(ctx); restoreErr != nil {
		return result, restoreErr
	}

	refreshedWorktrees, refreshedListErr := listRepositoryWorktrees(ctx, transaction.environment.GitExecutor, transaction.repository.Path, transaction.targetBranch)
	if refreshedListErr != nil {
		return result, refreshedListErr
	}

	for worktreeIndex := range transaction.adoptedWorktrees {
		startingSibling := transaction.adoptedWorktrees[worktreeIndex]
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
			result.WorktreesRestored++
			continue
		}

		if restoreErr := addListedWorktree(ctx, transaction.environment.GitExecutor, transaction.repository.Path, startingSibling); restoreErr != nil {
			return result, fmt.Errorf(strictSyncRestoreWorktreeFailureTemplate, startingSibling.Path, restoreErr)
		}
		result.WorktreesRestored++
	}

	for snapshotIndex := range transaction.touchedWorktrees {
		snapshot := transaction.touchedWorktrees[snapshotIndex]
		if !transaction.worktreeMutated(snapshot.Worktree.Path) {
			continue
		}
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

type strictSyncGitMutationJournal struct {
	transaction *strictSyncTransaction
	references  []string
}

func prepareStrictSyncGitMutation(ctx context.Context, executor shared.GitExecutor, workingDirectory string, arguments []string) (strictSyncGitMutationJournal, error) {
	transaction, ok := strictSyncTransactionFromContext(ctx)
	if !ok || transaction.restoring || ctx.Value(strictSyncJournalSuppressionContextKey{}) == true {
		return strictSyncGitMutationJournal{}, nil
	}
	if strictSyncCommandMutatesWorktree(arguments) {
		transaction.markWorktreeMutated(workingDirectory)
	}
	references, referenceErr := strictSyncMutatedBranchReferences(ctx, executor, workingDirectory, arguments)
	if referenceErr != nil {
		return strictSyncGitMutationJournal{}, referenceErr
	}
	if len(references) == 0 {
		return strictSyncGitMutationJournal{transaction: transaction}, nil
	}

	currentBranches, currentBranchesErr := captureStrictSyncLocalBranches(ctx, executor, transaction.repository.Path)
	if currentBranchesErr != nil {
		return strictSyncGitMutationJournal{}, currentBranchesErr
	}
	for referenceIndex := range references {
		referenceName := references[referenceIndex]
		currentCommit, currentExists := currentBranches[referenceName]
		mutation, alreadyOwned := transaction.branchMutations[referenceName]
		if !alreadyOwned {
			initialCommit, initialExists := transaction.localBranches[referenceName]
			mutation = strictSyncBranchMutation{
				InitialCommit:  initialCommit,
				InitialExists:  initialExists,
				ExpectedCommit: initialCommit,
				ExpectedExists: initialExists,
			}
			transaction.branchMutations[referenceName] = mutation
		}
		if currentExists != mutation.ExpectedExists || currentCommit != mutation.ExpectedCommit {
			return strictSyncGitMutationJournal{}, strictSyncReferenceOwnershipError(referenceName, mutation.ExpectedCommit, mutation.ExpectedExists, currentCommit, currentExists)
		}
	}
	return strictSyncGitMutationJournal{transaction: transaction, references: references}, nil
}

func completeStrictSyncGitMutation(ctx context.Context, executor shared.GitExecutor, journal strictSyncGitMutationJournal) error {
	if journal.transaction == nil || len(journal.references) == 0 {
		return nil
	}
	currentBranches, currentBranchesErr := captureStrictSyncLocalBranches(ctx, executor, journal.transaction.repository.Path)
	if currentBranchesErr != nil {
		return currentBranchesErr
	}
	for referenceIndex := range journal.references {
		referenceName := journal.references[referenceIndex]
		mutation := journal.transaction.branchMutations[referenceName]
		mutation.ExpectedCommit, mutation.ExpectedExists = currentBranches[referenceName]
		journal.transaction.branchMutations[referenceName] = mutation
	}
	return nil
}

func strictSyncCommandMutatesWorktree(arguments []string) bool {
	if len(arguments) == 0 {
		return false
	}
	switch arguments[0] {
	case gitAddSubcommandConstant,
		gitCheckoutSubcommandConstant,
		gitCleanSubcommandConstant,
		gitCommitSubcommandConstant,
		gitMergeSubcommandConstant,
		gitResetSubcommandConstant,
		gitRmSubcommandConstant,
		gitSwitchSubcommandConstant:
		return true
	case gitStashSubcommandConstant:
		if len(arguments) < 2 {
			return false
		}
		return arguments[1] == gitStashPushSubcommandConstant || arguments[1] == gitStashApplySubcommandConstant || arguments[1] == gitStashPopSubcommandConstant
	default:
		return false
	}
}

func strictSyncMutatedBranchReferences(ctx context.Context, executor shared.GitExecutor, workingDirectory string, arguments []string) ([]string, error) {
	if len(arguments) == 0 {
		return nil, nil
	}
	if arguments[0] == gitSwitchSubcommandConstant {
		for argumentIndex := 1; argumentIndex+1 < len(arguments); argumentIndex++ {
			if arguments[argumentIndex] == gitCreateBranchFlagConstant {
				return []string{gitLocalHeadsPrefixConstant + strings.TrimSpace(arguments[argumentIndex+1])}, nil
			}
		}
		return nil, nil
	}
	mutatesCurrentBranch := arguments[0] == gitCommitSubcommandConstant || arguments[0] == gitMergeSubcommandConstant
	if arguments[0] == gitResetSubcommandConstant {
		mutatesCurrentBranch = strictSyncArgumentsContain(arguments, gitResetHardFlagConstant)
	}
	if !mutatesCurrentBranch {
		return nil, nil
	}
	result, branchErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitRevParseSubcommandConstant, gitAbbrevRefFlagConstant, gitHeadReferenceConstant},
		WorkingDirectory: workingDirectory,
	})
	if branchErr != nil {
		return nil, branchErr
	}
	branchName := strings.TrimSpace(result.StandardOutput)
	if branchName == "" || branchName == gitHeadReferenceConstant {
		return nil, nil
	}
	return []string{gitLocalHeadsPrefixConstant + branchName}, nil
}

func strictSyncArgumentsContain(arguments []string, target string) bool {
	for argumentIndex := range arguments {
		if arguments[argumentIndex] == target {
			return true
		}
	}
	return false
}

func strictSyncPushUpdatedRemote(output string) (bool, error) {
	statusObserved := false
	updated := false
	for _, line := range strings.Split(output, "\n") {
		if len(line) < 2 || line[1] != '\t' {
			continue
		}
		statusObserved = true
		switch line[0] {
		case ' ', '+', '-', '*':
			updated = true
		case '=', '!':
		default:
			return false, fmt.Errorf(strictSyncPushStatusInvalidTemplate, string(line[0]))
		}
	}
	if !statusObserved {
		return false, errors.New(strictSyncPushStatusMissingMessage)
	}
	return updated, nil
}

func (transaction *strictSyncTransaction) restoreLocalBranches(ctx context.Context) error {
	currentBranches, currentBranchesErr := captureStrictSyncLocalBranches(ctx, transaction.environment.GitExecutor, transaction.repository.Path)
	if currentBranchesErr != nil {
		return currentBranchesErr
	}
	ownedReferences := sortedStrictSyncMutationReferenceNames(transaction.branchMutations)
	for referenceIndex := range ownedReferences {
		referenceName := ownedReferences[referenceIndex]
		mutation := transaction.branchMutations[referenceName]
		currentCommit, currentExists := currentBranches[referenceName]
		if currentExists != mutation.ExpectedExists || currentCommit != mutation.ExpectedCommit {
			return strictSyncReferenceOwnershipError(referenceName, mutation.ExpectedCommit, mutation.ExpectedExists, currentCommit, currentExists)
		}
		if mutation.InitialExists {
			if currentCommit == mutation.InitialCommit {
				continue
			}
			if updateErr := executeGit(ctx, transaction.environment.GitExecutor, transaction.repository.Path, []string{
				gitUpdateRefSubcommandConstant,
				referenceName,
				mutation.InitialCommit,
				mutation.ExpectedCommit,
			}); updateErr != nil {
				return updateErr
			}
			continue
		}
		if !currentExists {
			continue
		}
		if deleteErr := executeGit(ctx, transaction.environment.GitExecutor, transaction.repository.Path, []string{
			gitUpdateRefSubcommandConstant,
			gitDeleteRefFlagConstant,
			referenceName,
			mutation.ExpectedCommit,
		}); deleteErr != nil {
			return deleteErr
		}
	}
	return nil
}

func (transaction *strictSyncTransaction) validateRollbackReferenceOwnership(ctx context.Context) error {
	currentBranches, currentBranchesErr := captureStrictSyncLocalBranches(ctx, transaction.environment.GitExecutor, transaction.repository.Path)
	if currentBranchesErr != nil {
		return currentBranchesErr
	}
	ownedReferences := sortedStrictSyncMutationReferenceNames(transaction.branchMutations)
	for referenceIndex := range ownedReferences {
		referenceName := ownedReferences[referenceIndex]
		mutation := transaction.branchMutations[referenceName]
		currentCommit, currentExists := currentBranches[referenceName]
		if currentExists != mutation.ExpectedExists || currentCommit != mutation.ExpectedCommit {
			return strictSyncReferenceOwnershipError(referenceName, mutation.ExpectedCommit, mutation.ExpectedExists, currentCommit, currentExists)
		}
	}
	for snapshotIndex := range transaction.touchedWorktrees {
		snapshot := transaction.touchedWorktrees[snapshotIndex]
		if !transaction.worktreeMutated(snapshot.Worktree.Path) || snapshot.Worktree.BranchName == "" {
			continue
		}
		referenceName := gitLocalHeadsPrefixConstant + snapshot.Worktree.BranchName
		if _, owned := transaction.branchMutations[referenceName]; owned {
			continue
		}
		initialCommit, initialExists := transaction.localBranches[referenceName]
		currentCommit, currentExists := currentBranches[referenceName]
		if currentExists != initialExists || currentCommit != initialCommit {
			return strictSyncReferenceOwnershipError(referenceName, initialCommit, initialExists, currentCommit, currentExists)
		}
	}
	return nil
}

func strictSyncReferenceOwnershipError(referenceName string, expectedCommit string, expectedExists bool, currentCommit string, currentExists bool) error {
	return fmt.Errorf(
		strictSyncReferenceOwnershipTemplate,
		referenceName,
		strictSyncReferenceState(expectedCommit, expectedExists),
		strictSyncReferenceState(currentCommit, currentExists),
	)
}

func strictSyncReferenceState(commitID string, exists bool) string {
	if !exists {
		return "missing"
	}
	return commitID
}

func sortedStrictSyncMutationReferenceNames(references map[string]strictSyncBranchMutation) []string {
	names := make([]string, 0, len(references))
	for referenceName := range references {
		names = append(names, referenceName)
	}
	sort.Strings(names)
	return names
}
