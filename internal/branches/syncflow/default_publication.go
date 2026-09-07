package syncflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/tyemirov/gix/internal/workflow"
)

type syncBranchPublication uint8

const (
	syncBranchPublicationDirect syncBranchPublication = iota + 1
	syncBranchPublicationReview
	syncBranchPublicationFailureTemplate = "resolve publication policy for branch %q: %w"
	syncDefaultReviewBranchTemplate      = strictSyncGeneratedBranchPrefix + "/sync-%s"
)

func resolveSyncBranchPublication(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, branchName string) (syncBranchPublication, error) {
	if environment.GitHubClient == nil {
		return 0, fmt.Errorf(syncBranchPublicationFailureTemplate, branchName, errors.New(strictSyncMissingGitHubClientMessage))
	}
	repositoryIdentifier := strictSyncRepositoryIdentifier(repository)
	if repositoryIdentifier == "" {
		return 0, fmt.Errorf(syncBranchPublicationFailureTemplate, branchName, errors.New(strictSyncMissingRepositoryMessage))
	}
	protected, protectionErr := environment.GitHubClient.CheckBranchProtection(ctx, repositoryIdentifier, branchName)
	if protectionErr != nil {
		return 0, fmt.Errorf(syncBranchPublicationFailureTemplate, branchName, protectionErr)
	}
	if protected {
		return syncBranchPublicationReview, nil
	}
	return syncBranchPublicationDirect, nil
}

func mergeLocalDefaultIntoReviewBranch(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, defaultBranch string, reviewBranch string, commitMessages worktreeAdoptionCommitMessageOptions) error {
	localExists, localErr := localBranchExists(ctx, environment.GitExecutor, repository.Path, defaultBranch)
	if localErr != nil || !localExists {
		return localErr
	}
	return mergeReferenceIntoBranch(ctx, environment, repository, environment.GitExecutor, repository.Path, gitBranchReferencePrefix+defaultBranch, reviewBranch, commitMessages)
}

func syncCommittedDefaultForReview(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, remoteName string, defaultBranch string, commitMessages worktreeAdoptionCommitMessageOptions, pullRequest strictSyncPullRequestMetadata) (strictPullRequestBranchResult, error) {
	commitID, _, commitErr := strictSyncReferenceCommit(ctx, environment.GitExecutor, repository.Path, gitHeadReferenceConstant)
	if commitErr != nil {
		return strictPullRequestBranchResult{}, commitErr
	}
	reviewBranch, branchErr := selectSyncBranchName(ctx, environment, repository, remoteName, defaultBranch, fmt.Sprintf(syncDefaultReviewBranchTemplate, commitID))
	if branchErr != nil {
		return strictPullRequestBranchResult{}, branchErr
	}
	if prepareErr := prepareStrictSyncBranchForDirtyWork(ctx, environment, repository, remoteName, defaultBranch, reviewBranch, strictSyncDirtyBranchStartCurrentCheckout, commitMessages); prepareErr != nil {
		return strictPullRequestBranchResult{}, prepareErr
	}
	if mergeErr := mergeLocalDefaultIntoReviewBranch(ctx, environment, repository, defaultBranch, reviewBranch, commitMessages); mergeErr != nil {
		return strictPullRequestBranchResult{}, mergeErr
	}
	result, syncErr := syncPullRequestBranch(ctx, environment, repository, strictPullRequestBranchOptions{
		BranchName:     reviewBranch,
		RemoteName:     remoteName,
		BaseBranch:     defaultBranch,
		DefaultBranch:  defaultBranch,
		CommitMessages: commitMessages,
		PullRequest:    pullRequest,
	})
	result.SyncedBranch = reviewBranch
	return result, syncErr
}
