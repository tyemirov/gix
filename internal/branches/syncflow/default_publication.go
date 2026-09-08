package syncflow

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/workflow"
)

type syncBranchPublication uint8

const (
	syncBranchPublicationDirect syncBranchPublication = iota + 1
	syncBranchPublicationReview
	syncBranchPublicationFailureTemplate = "resolve publication policy for branch %q: %w"
	syncDefaultReviewInitialLimit        = 100
	syncDefaultReviewBranchTemplate      = strictSyncGeneratedBranchPrefix + "/sync-%s"
)

func resolveSyncBranchPublication(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, remoteName string, branchName string) (syncBranchPublication, error) {
	if environment.GitHubClient == nil {
		return 0, fmt.Errorf(syncBranchPublicationFailureTemplate, branchName, errors.New(strictSyncMissingGitHubClientMessage))
	}
	repositoryIdentifier, identifierErr := strictSyncRepositoryIdentifier(ctx, environment, repository, remoteName)
	if identifierErr != nil {
		return 0, fmt.Errorf(syncBranchPublicationFailureTemplate, branchName, identifierErr)
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
	reviewBranch, branchErr := selectDefaultSnapshotReviewBranch(ctx, environment, repository, remoteName, defaultBranch, fmt.Sprintf(syncDefaultReviewBranchTemplate, commitID))
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

func selectDefaultSnapshotReviewBranch(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, remoteName string, defaultBranch string, initialBranch string) (string, error) {
	review, reviewErr := defaultSnapshotReview(ctx, environment, repository, remoteName, defaultBranch, githubcli.PullRequestStateOpen)
	if reviewErr != nil {
		return "", reviewErr
	}
	if review != nil {
		refspec := fmt.Sprintf(gitFetchRemoteBranchRefspecTemplateConstant, review.HeadRefName, remoteName, review.HeadRefName)
		if fetchErr := executeGit(ctx, environment.GitExecutor, repository.Path, []string{gitFetchSubcommandConstant, gitFetchNoTagsFlagConstant, remoteName, refspec}); fetchErr != nil {
			return "", fmt.Errorf("fetch review branch %q from %q: %w", review.HeadRefName, remoteName, fetchErr)
		}
		return review.HeadRefName, nil
	}
	return selectSyncBranchName(ctx, environment, repository, remoteName, defaultBranch, initialBranch)
}

func defaultSnapshotReview(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, remoteName string, defaultBranch string, state githubcli.PullRequestState) (*githubcli.PullRequest, error) {
	localTip, localExists, localErr := strictSyncReferenceCommit(ctx, environment.GitExecutor, repository.Path, gitBranchReferencePrefix+defaultBranch)
	if localErr != nil || !localExists {
		return nil, localErr
	}
	remoteReference := fmt.Sprintf("%s/%s", remoteName, defaultBranch)
	ahead, aheadErr := commitCount(ctx, environment.GitExecutor, repository.Path, fmt.Sprintf("%s..%s", remoteReference, localTip))
	if aheadErr != nil || ahead == 0 {
		return nil, aheadErr
	}
	repositoryIdentifier, identifierErr := strictSyncRepositoryIdentifier(ctx, environment, repository, remoteName)
	if identifierErr != nil {
		return nil, identifierErr
	}
	if environment.GitHubClient == nil {
		return nil, errors.New(strictSyncMissingGitHubClientMessage)
	}
	var reviews []githubcli.PullRequest
	for limit := syncDefaultReviewInitialLimit; ; limit *= 2 {
		var reviewErr error
		reviews, reviewErr = environment.GitHubClient.ListPullRequests(ctx, repositoryIdentifier, githubcli.PullRequestListOptions{
			State: state, BaseBranch: defaultBranch, ResultLimit: limit,
		})
		if reviewErr != nil {
			return nil, fmt.Errorf("find %s reviews for default branch %q: %w", state, defaultBranch, reviewErr)
		}
		if len(reviews) < limit {
			break
		}
	}
	sort.Slice(reviews, func(left, right int) bool { return reviews[left].Number < reviews[right].Number })
	for _, review := range reviews {
		if review.BaseRefName != defaultBranch || state == githubcli.PullRequestStateOpen && !strings.EqualFold(review.HeadRepositoryNameWithOwner, repositoryIdentifier) {
			continue
		}
		if strings.TrimSpace(review.HeadRefName) == "" || strings.TrimSpace(review.HeadRefOID) == "" {
			return nil, fmt.Errorf("review %d for default branch %q has no head branch or commit", review.Number, defaultBranch)
		}
		headCommit, headErr := ensureStrictSyncPullRequestHeadCommit(ctx, environment.GitExecutor, repository.Path, remoteName, review)
		if headErr != nil {
			return nil, headErr
		}
		contains, ancestryErr := strictSyncCommitIsAncestor(ctx, environment.GitExecutor, repository.Path, localTip, headCommit)
		if ancestryErr != nil {
			return nil, ancestryErr
		}
		if contains {
			return &review, nil
		}
	}
	return nil, nil
}
