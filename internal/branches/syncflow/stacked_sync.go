package syncflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	strictSyncStackedBranchNoChangesTemplate = "cannot create stacked branch %q from %q: no changes would remain for its pull request"
	strictSyncStackedDetachedHeadTemplate    = "cannot create stacked branch %q from a detached HEAD"
	strictSyncStackedParentMergedTemplate    = "cannot create stacked branch %q from %q: the parent branch pull request is already merged"
	strictSyncStackedParentBehindTemplate    = "cannot create stacked branch %q from %q: the parent branch is behind %s and must be synced first"
	strictSyncStackedMergedDirtyTemplate     = "cannot commit uncommitted changes on merged branch %q; rerun with --stash to preserve them through the merged handoff before creating a new review branch"
	strictSyncStackedReviewBaseCycleTemplate = "cannot resolve stacked pull-request chain: review base cycle at branch %q"
	strictSyncStackReviewBaseKeyTemplate     = "branch.%s.gix-review-base"
	strictSyncStackReviewBaseReadTemplate    = "read stacked review base for %q: %w"
	strictSyncStackReviewBaseWriteTemplate   = "record stacked review base for %q: %w"
	strictSyncStackReviewBaseDeleteTemplate  = "remove stacked review base for %q: %w"
	strictSyncGitConfigSubcommand            = "config"
	strictSyncGitConfigGetFlag               = "--get"
	strictSyncGitConfigUnsetFlag             = "--unset"
)

type strictSyncStackPlan struct {
	ChildBranch            string
	ParentBranch           string
	RecordReviewBase       bool
	ChildPullRequestMerged bool
}

type strictSyncStackPlanningOptions struct {
	RemoteName       string
	ChildBranch      string
	ResolutionSource string
	Dirty            bool
	StashChanges     bool
}

type strictSyncStackParentOptions struct {
	RemoteName           string
	ConfiguredBaseBranch string
	Plan                 strictSyncStackPlan
	CommitMessages       worktreeAdoptionCommitMessageOptions
}

func planStrictSyncStack(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, options strictSyncStackPlanningOptions) (*strictSyncStackPlan, error) {
	remoteReference := fmt.Sprintf("%s/%s", options.RemoteName, options.ChildBranch)
	remoteExists, remoteExistsErr := remoteReferenceExists(ctx, environment.GitExecutor, repository.Path, remoteReference)
	if remoteExistsErr != nil {
		return nil, remoteExistsErr
	}
	localExists, localExistsErr := localBranchExists(ctx, environment.GitExecutor, repository.Path, options.ChildBranch)
	if localExistsErr != nil {
		return nil, localExistsErr
	}
	storedParentBranch, storedParentBranchErr := strictSyncStackReviewBase(ctx, environment.GitExecutor, repository.Path, options.ChildBranch)
	if storedParentBranchErr != nil {
		return nil, storedParentBranchErr
	}
	if storedParentBranch != "" {
		if storedParentBranch == options.ChildBranch {
			return nil, fmt.Errorf(strictSyncStackedReviewBaseCycleTemplate, options.ChildBranch)
		}
		if remoteExists || localExists {
			repositoryIdentifier := strictSyncRepositoryIdentifier(repository)
			if repositoryIdentifier != "" && environment.GitHubClient != nil {
				openPullRequest, openPullRequestErr := openPullRequestForBranch(ctx, environment, repositoryIdentifier, options.ChildBranch)
				if openPullRequestErr != nil {
					return nil, openPullRequestErr
				}
				if openPullRequest != nil {
					return nil, nil
				}
				mergedPullRequest, mergedPullRequestErr := pullRequestForBranchWithState(ctx, environment, repositoryIdentifier, "", options.ChildBranch, githubcli.PullRequestStateMerged)
				if mergedPullRequestErr != nil {
					return nil, mergedPullRequestErr
				}
				if mergedPullRequest != nil {
					mergedBaseBranch, mergedBaseBranchErr := openPullRequestBaseBranch(*mergedPullRequest, options.ChildBranch)
					if mergedBaseBranchErr != nil {
						return nil, mergedBaseBranchErr
					}
					if mergedBaseBranch == options.ChildBranch {
						return nil, fmt.Errorf(strictSyncStackedReviewBaseCycleTemplate, options.ChildBranch)
					}
					return &strictSyncStackPlan{
						ChildBranch:            options.ChildBranch,
						ParentBranch:           mergedBaseBranch,
						ChildPullRequestMerged: true,
					}, nil
				}
			}
		} else if !options.Dirty || options.StashChanges {
			return nil, fmt.Errorf(strictSyncStackedBranchNoChangesTemplate, options.ChildBranch, storedParentBranch)
		}
		return &strictSyncStackPlan{
			ChildBranch:  options.ChildBranch,
			ParentBranch: storedParentBranch,
		}, nil
	}
	if strings.TrimSpace(options.ResolutionSource) != branchResolutionSourceExplicit {
		return nil, nil
	}
	if remoteExists || localExists {
		return nil, nil
	}

	parentBranch := strings.TrimSpace(repository.Inspection.LocalBranch)
	if parentBranch == "" {
		return nil, fmt.Errorf(strictSyncStackedDetachedHeadTemplate, options.ChildBranch)
	}
	if !options.Dirty || options.StashChanges {
		return nil, fmt.Errorf(strictSyncStackedBranchNoChangesTemplate, options.ChildBranch, parentBranch)
	}
	return &strictSyncStackPlan{
		ChildBranch:      options.ChildBranch,
		ParentBranch:     parentBranch,
		RecordReviewBase: true,
	}, nil
}

func strictSyncStackReviewBase(ctx context.Context, executor shared.GitExecutor, repositoryPath string, childBranch string) (string, error) {
	result, configErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{strictSyncGitConfigSubcommand, strictSyncGitConfigGetFlag, strictSyncStackReviewBaseKey(childBranch)},
		WorkingDirectory: repositoryPath,
	})
	if configErr == nil {
		return strings.TrimSpace(result.StandardOutput), nil
	}
	var commandFailure execshell.CommandFailedError
	if errors.As(configErr, &commandFailure) && commandFailure.Result.ExitCode == 1 {
		return "", nil
	}
	return "", fmt.Errorf(strictSyncStackReviewBaseReadTemplate, childBranch, configErr)
}

func recordStrictSyncStackReviewBase(ctx context.Context, executor shared.GitExecutor, repositoryPath string, plan strictSyncStackPlan) error {
	if configErr := executeGit(ctx, executor, repositoryPath, []string{strictSyncGitConfigSubcommand, strictSyncStackReviewBaseKey(plan.ChildBranch), plan.ParentBranch}); configErr != nil {
		return fmt.Errorf(strictSyncStackReviewBaseWriteTemplate, plan.ChildBranch, configErr)
	}
	return nil
}

func removeStrictSyncStackReviewBaseWhenChildMissing(ctx context.Context, executor shared.GitExecutor, repositoryPath string, plan strictSyncStackPlan) error {
	localExists, localExistsErr := localBranchExists(ctx, executor, repositoryPath, plan.ChildBranch)
	if localExistsErr != nil {
		return localExistsErr
	}
	if localExists {
		return nil
	}
	if configErr := executeGit(ctx, executor, repositoryPath, []string{strictSyncGitConfigSubcommand, strictSyncGitConfigUnsetFlag, strictSyncStackReviewBaseKey(plan.ChildBranch)}); configErr != nil {
		return fmt.Errorf(strictSyncStackReviewBaseDeleteTemplate, plan.ChildBranch, configErr)
	}
	return nil
}

func strictSyncStackReviewBaseKey(childBranch string) string {
	return fmt.Sprintf(strictSyncStackReviewBaseKeyTemplate, childBranch)
}

func ensureStrictSyncStackParent(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, options strictSyncStackParentOptions) error {
	return ensureStrictSyncStackParentChain(ctx, environment, repository, options, map[string]struct{}{})
}

func ensureStrictSyncStackParentChain(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, options strictSyncStackParentOptions, visitedBranches map[string]struct{}) error {
	if options.Plan.ParentBranch == options.ConfiguredBaseBranch {
		return nil
	}
	if _, visited := visitedBranches[options.Plan.ParentBranch]; visited {
		return fmt.Errorf(strictSyncStackedReviewBaseCycleTemplate, options.Plan.ParentBranch)
	}
	visitedBranches[options.Plan.ParentBranch] = struct{}{}

	repositoryIdentifier := strictSyncRepositoryIdentifier(repository)
	if repositoryIdentifier == "" {
		return errors.New(strictSyncMissingRepositoryMessage)
	}

	openPullRequest, openPullRequestErr := openPullRequestForBranch(ctx, environment, repositoryIdentifier, options.Plan.ParentBranch)
	if openPullRequestErr != nil {
		return openPullRequestErr
	}
	if openPullRequest != nil {
		if _, baseBranchErr := openPullRequestBaseBranch(*openPullRequest, options.Plan.ParentBranch); baseBranchErr != nil {
			return baseBranchErr
		}
		localParentExists, localParentExistsErr := localBranchExists(ctx, environment.GitExecutor, repository.Path, options.Plan.ParentBranch)
		if localParentExistsErr != nil {
			return localParentExistsErr
		}
		if !localParentExists {
			return nil
		}
		if remoteStateErr := validateStrictSyncStackParentRemoteState(ctx, environment.GitExecutor, repository.Path, options.RemoteName, options.Plan); remoteStateErr != nil {
			return remoteStateErr
		}
		return executeGit(ctx, environment.GitExecutor, repository.Path, []string{gitPushSubcommandConstant, gitPushSetUpstreamFlagConstant, options.RemoteName, options.Plan.ParentBranch})
	}

	mergedPullRequest, mergedPullRequestErr := pullRequestForBranchWithState(ctx, environment, repositoryIdentifier, "", options.Plan.ParentBranch, githubcli.PullRequestStateMerged)
	if mergedPullRequestErr != nil {
		return mergedPullRequestErr
	}
	if mergedPullRequest != nil {
		return fmt.Errorf(strictSyncStackedParentMergedTemplate, options.Plan.ChildBranch, options.Plan.ParentBranch)
	}

	parentReviewBase := options.ConfiguredBaseBranch
	storedParentReviewBase, storedParentReviewBaseErr := strictSyncStackReviewBase(ctx, environment.GitExecutor, repository.Path, options.Plan.ParentBranch)
	if storedParentReviewBaseErr != nil {
		return storedParentReviewBaseErr
	}
	if storedParentReviewBase != "" {
		if storedParentReviewBase == options.Plan.ParentBranch {
			return fmt.Errorf(strictSyncStackedReviewBaseCycleTemplate, options.Plan.ParentBranch)
		}
		parentReviewBase = storedParentReviewBase
	}

	parentHasChanges, parentHasChangesErr := branchHasCommitsBeyondBase(ctx, environment.GitExecutor, repository.Path, options.RemoteName, parentReviewBase, options.Plan.ParentBranch)
	if parentHasChangesErr != nil {
		return parentHasChangesErr
	}
	if !parentHasChanges {
		return fmt.Errorf(strictSyncEmptyLocalBranchTemplate, options.Plan.ParentBranch, options.RemoteName, parentReviewBase)
	}
	if remoteStateErr := validateStrictSyncStackParentRemoteState(ctx, environment.GitExecutor, repository.Path, options.RemoteName, options.Plan); remoteStateErr != nil {
		return remoteStateErr
	}
	if parentReviewBase != options.ConfiguredBaseBranch {
		if parentErr := ensureStrictSyncStackParentChain(ctx, environment, repository, strictSyncStackParentOptions{
			RemoteName:           options.RemoteName,
			ConfiguredBaseBranch: options.ConfiguredBaseBranch,
			Plan: strictSyncStackPlan{
				ChildBranch:  options.Plan.ParentBranch,
				ParentBranch: parentReviewBase,
			},
			CommitMessages: options.CommitMessages,
		}, visitedBranches); parentErr != nil {
			return parentErr
		}
	}
	return pushAndCreatePullRequest(ctx, environment, repository, repositoryIdentifier, strictPullRequestBranchOptions{
		BranchName:     options.Plan.ParentBranch,
		RemoteName:     options.RemoteName,
		BaseBranch:     parentReviewBase,
		CommitMessages: options.CommitMessages,
	})
}

func validateStrictSyncStackParentRemoteState(ctx context.Context, executor shared.GitExecutor, repositoryPath string, remoteName string, plan strictSyncStackPlan) error {
	remoteReference := fmt.Sprintf("%s/%s", remoteName, plan.ParentBranch)
	remoteExists, remoteExistsErr := remoteReferenceExists(ctx, executor, repositoryPath, remoteReference)
	if remoteExistsErr != nil {
		return remoteExistsErr
	}
	if !remoteExists {
		return nil
	}
	remoteAheadCount, remoteAheadErr := commitCount(ctx, executor, repositoryPath, fmt.Sprintf("%s..%s", plan.ParentBranch, remoteReference))
	if remoteAheadErr != nil {
		return remoteAheadErr
	}
	if remoteAheadCount > 0 {
		return fmt.Errorf(strictSyncStackedParentBehindTemplate, plan.ChildBranch, plan.ParentBranch, remoteReference)
	}
	return nil
}
