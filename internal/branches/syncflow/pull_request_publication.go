package syncflow

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	strictSyncPullRequestCreateAttempts = 2
	strictSyncPullRequestRetryMessage   = "retrying pull request creation for %q into %q after a GitHub internal error"
	strictSyncPullRequestFoundMessage   = "confirmed open pull request #%d for %q into %q"
	strictSyncPullRequestLookupError    = "verify open pull request in %s from %q into %q: %w"
)

func createPullRequest(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, options githubcli.PullRequestCreateOptions) (strictSyncReviewPublication, error) {
	client := environment.GitHubClient
	if client == nil {
		return strictSyncBranchPublished, errors.New(strictSyncMissingGitHubClientMessage)
	}

	var createErr error
	for attempt := range strictSyncPullRequestCreateAttempts {
		if ctx.Err() != nil {
			break
		}
		attemptErr := client.CreatePullRequest(ctx, options)
		if attemptErr == nil {
			markStrictSyncPublished(ctx)
			return strictSyncPullRequestCreated, nil
		}
		if attempt == 0 && !githubcli.IsPullRequestCreateInternalError(attemptErr) {
			return strictSyncBranchPublished, attemptErr
		}
		createErr = errors.Join(createErr, attemptErr)
		if ctx.Err() != nil {
			break
		}
		pullRequest, lookupErr := client.FindOpenPullRequest(ctx, options.Repository, options.Base, options.Head)
		if lookupErr != nil {
			return strictSyncBranchPublished, errors.Join(createErr, fmt.Errorf(strictSyncPullRequestLookupError, options.Repository, options.Head, options.Base, lookupErr))
		}
		if pullRequest != nil {
			markStrictSyncPublished(ctx)
			environment.ReportRepositoryEvent(repository, shared.EventLevelInfo, shared.EventCodeSyncPullRequestConfirmed,
				fmt.Sprintf(strictSyncPullRequestFoundMessage, pullRequest.Number, options.Head, options.Base),
				map[string]string{"head": options.Head, "base": options.Base, "pull_request": strconv.Itoa(pullRequest.Number)})
			return strictSyncPullRequestConfirmed, nil
		}
		if attempt == 0 && ctx.Err() == nil {
			environment.ReportRepositoryEvent(repository, shared.EventLevelWarn, shared.EventCodeSyncPullRequestRetry,
				fmt.Sprintf(strictSyncPullRequestRetryMessage, options.Head, options.Base),
				map[string]string{"head": options.Head, "base": options.Base})
		}
	}
	return strictSyncBranchPublished, errors.Join(createErr, context.Cause(ctx))
}
