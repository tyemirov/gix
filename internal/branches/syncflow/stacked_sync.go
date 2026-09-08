package syncflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	strictSyncStackedDetachedHeadTemplate    = "cannot create stacked branch %q from a detached HEAD"
	strictSyncStackedParentMergedTemplate    = "cannot create stacked branch %q from %q: the parent branch pull request is already merged"
	strictSyncStackedMergedDirtyTemplate     = "cannot commit uncommitted changes on merged branch %q; rerun with --stash to preserve them through the merged handoff before creating a new review branch"
	strictSyncStackedReviewBaseCycleTemplate = "cannot resolve stacked pull-request chain: review base cycle at branch %q"
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
	DefaultBranch    string
	ResolutionSource string
	Dirty            bool
	StashChanges     bool
}

type strictSyncStackParentOptions struct {
	RemoteName     string
	DefaultBranch  string
	Plan           strictSyncStackPlan
	CommitMessages worktreeAdoptionCommitMessageOptions
}

func planStrictSyncStack(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, options strictSyncStackPlanningOptions) (*strictSyncStackPlan, error) {
	if strings.TrimSpace(options.ChildBranch) == strings.TrimSpace(options.DefaultBranch) {
		return nil, nil
	}
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
	}
	inspectMergedStateBeforeCommit := options.Dirty && options.ChildBranch != options.DefaultBranch
	if (remoteExists || localExists) && (storedParentBranch != "" || inspectMergedStateBeforeCommit) {
		if environment.GitHubClient != nil {
			repositoryIdentifier, identifierErr := strictSyncRepositoryIdentifier(ctx, environment, repository, options.RemoteName)
			if identifierErr != nil {
				return nil, identifierErr
			}
			openPullRequest, openPullRequestErr := openPullRequestForBranch(ctx, environment, repositoryIdentifier, options.ChildBranch)
			if openPullRequestErr != nil {
				return nil, openPullRequestErr
			}
			if openPullRequest != nil {
				return nil, nil
			}
			mergedPullRequest, mergedPullRequestErr := mergedPullRequestForCurrentBranchTip(ctx, environment, repository, repositoryIdentifier, options.RemoteName, options.ChildBranch)
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
	}
	if storedParentBranch != "" {
		parent, parentErr := reconcileStrictSyncReviewBase(ctx, environment, repository, options.RemoteName, options.DefaultBranch, options.ChildBranch, storedParentBranch)
		if parentErr != nil {
			return nil, parentErr
		}
		return &strictSyncStackPlan{
			ChildBranch:      options.ChildBranch,
			ParentBranch:     parent,
			RecordReviewBase: parent != storedParentBranch,
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
	parentBranch, parentErr := reconcileStrictSyncReviewBase(ctx, environment, repository, options.RemoteName, options.DefaultBranch, options.ChildBranch, parentBranch)
	if parentErr != nil {
		return nil, parentErr
	}
	return &strictSyncStackPlan{
		ChildBranch:      options.ChildBranch,
		ParentBranch:     parentBranch,
		RecordReviewBase: true,
	}, nil
}

func strictSyncStackReviewBase(ctx context.Context, executor shared.GitExecutor, repositoryPath string, childBranch string) (string, error) {
	reviewBase, _, reviewBaseErr := gitrepo.BranchReviewBase(ctx, executor, repositoryPath, childBranch)
	return reviewBase, reviewBaseErr
}

func reconcileStrictSyncReviewBase(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, remoteName, defaultBranch, childBranch, parentBranch string) (string, error) {
	if parentBranch == defaultBranch {
		return parentBranch, nil
	}
	identifier, identifierErr := strictSyncRepositoryIdentifier(ctx, environment, repository, remoteName)
	if identifierErr != nil {
		return "", identifierErr
	}
	open, openErr := openPullRequestForBranch(ctx, environment, identifier, parentBranch)
	if openErr != nil || open != nil {
		return parentBranch, openErr
	}
	merged, mergedErr := mergedPullRequestForCurrentBranchTip(ctx, environment, repository, identifier, remoteName, parentBranch)
	if mergedErr != nil || merged == nil {
		return parentBranch, mergedErr
	}
	base, baseErr := openPullRequestBaseBranch(*merged, parentBranch)
	if baseErr != nil {
		return "", baseErr
	}
	return resolveMergedPullRequestBaseTarget(ctx, environment, repository, identifier, remoteName, base, defaultBranch, map[string]struct{}{childBranch: {}, parentBranch: {}})
}

func recordStrictSyncStackReviewBase(ctx context.Context, executor shared.GitExecutor, repositoryPath string, plan strictSyncStackPlan) error {
	return gitrepo.RecordBranchReviewBase(ctx, executor, repositoryPath, plan.ChildBranch, plan.ParentBranch)
}

func removeStrictSyncStackReviewBaseWhenChildMissing(ctx context.Context, executor shared.GitExecutor, repositoryPath string, plan strictSyncStackPlan) error {
	localExists, localExistsErr := localBranchExists(ctx, executor, repositoryPath, plan.ChildBranch)
	if localExistsErr != nil {
		return localExistsErr
	}
	if localExists {
		return nil
	}
	return gitrepo.RemoveBranchReviewBase(ctx, executor, repositoryPath, plan.ChildBranch)
}

func strictSyncStackReviewBaseKey(childBranch string) string {
	return gitrepo.BranchReviewBaseKey(childBranch)
}

func ensureStrictSyncStackParent(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, options strictSyncStackParentOptions) (bool, error) {
	return ensureStrictSyncStackParentChain(ctx, environment, repository, options, map[string]struct{}{})
}

func ensureStrictSyncStackParentChain(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, options strictSyncStackParentOptions, visitedBranches map[string]struct{}) (bool, error) {
	if options.Plan.ParentBranch == options.DefaultBranch {
		return true, nil
	}
	if _, visited := visitedBranches[options.Plan.ParentBranch]; visited {
		return false, fmt.Errorf(strictSyncStackedReviewBaseCycleTemplate, options.Plan.ParentBranch)
	}
	visitedBranches[options.Plan.ParentBranch] = struct{}{}

	repositoryIdentifier, identifierErr := strictSyncRepositoryIdentifier(ctx, environment, repository, options.RemoteName)
	if identifierErr != nil {
		return false, identifierErr
	}

	openPullRequest, openPullRequestErr := openPullRequestForBranch(ctx, environment, repositoryIdentifier, options.Plan.ParentBranch)
	if openPullRequestErr != nil {
		return false, openPullRequestErr
	}
	if openPullRequest != nil {
		baseBranch, baseBranchErr := openPullRequestBaseBranch(*openPullRequest, options.Plan.ParentBranch)
		if baseBranchErr != nil {
			return false, baseBranchErr
		}
		localParentExists, localParentExistsErr := localBranchExists(ctx, environment.GitExecutor, repository.Path, options.Plan.ParentBranch)
		if localParentExistsErr != nil {
			return false, localParentExistsErr
		}
		if !localParentExists {
			return true, nil
		}
		if syncErr := synchronizeStrictSyncStackParent(ctx, environment, repository, options, baseBranch); syncErr != nil {
			return false, syncErr
		}
		return publishStrictSyncBranch(ctx, environment, repository, options.RemoteName, options.Plan.ParentBranch, []string{gitPushSubcommandConstant, gitPushSetUpstreamFlagConstant, options.RemoteName, options.Plan.ParentBranch})
	}

	mergedPullRequest, mergedPullRequestErr := mergedPullRequestForCurrentBranchTip(ctx, environment, repository, repositoryIdentifier, options.RemoteName, options.Plan.ParentBranch)
	if mergedPullRequestErr != nil {
		return false, mergedPullRequestErr
	}
	if mergedPullRequest != nil {
		return false, fmt.Errorf(strictSyncStackedParentMergedTemplate, options.Plan.ChildBranch, options.Plan.ParentBranch)
	}

	parentReviewBase := options.DefaultBranch
	storedParentReviewBase, storedParentReviewBaseErr := strictSyncStackReviewBase(ctx, environment.GitExecutor, repository.Path, options.Plan.ParentBranch)
	if storedParentReviewBaseErr != nil {
		return false, storedParentReviewBaseErr
	}
	if storedParentReviewBase != "" {
		if storedParentReviewBase == options.Plan.ParentBranch {
			return false, fmt.Errorf(strictSyncStackedReviewBaseCycleTemplate, options.Plan.ParentBranch)
		}
		parentReviewBase = storedParentReviewBase
	}

	resolvedBase, baseErr := reconcileStrictSyncReviewBase(ctx, environment, repository, options.RemoteName, options.DefaultBranch, options.Plan.ParentBranch, parentReviewBase)
	if baseErr != nil {
		return false, baseErr
	}
	if resolvedBase != parentReviewBase {
		if recordErr := gitrepo.RecordBranchReviewBase(ctx, environment.GitExecutor, repository.Path, options.Plan.ParentBranch, resolvedBase); recordErr != nil {
			return false, recordErr
		}
		parentReviewBase = resolvedBase
	}
	if parentReviewBase != options.DefaultBranch {
		if published, parentErr := ensureStrictSyncStackParentChain(ctx, environment, repository, strictSyncStackParentOptions{
			RemoteName:    options.RemoteName,
			DefaultBranch: options.DefaultBranch,
			Plan: strictSyncStackPlan{
				ChildBranch:  options.Plan.ParentBranch,
				ParentBranch: parentReviewBase,
			},
			CommitMessages: options.CommitMessages,
		}, visitedBranches); parentErr != nil || !published {
			return published, parentErr
		}
	}
	if syncErr := synchronizeStrictSyncStackParent(ctx, environment, repository, options, parentReviewBase); syncErr != nil {
		return false, syncErr
	}
	return pushAndCreatePullRequest(ctx, environment, repository, repositoryIdentifier, strictPullRequestBranchOptions{
		BranchName:     options.Plan.ParentBranch,
		RemoteName:     options.RemoteName,
		BaseBranch:     parentReviewBase,
		CommitMessages: options.CommitMessages,
	})
}

// synchronizeStrictSyncStackParent runs with pending child work held outside the checkout.
func synchronizeStrictSyncStackParent(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, options strictSyncStackParentOptions, baseBranch string) error {
	parentBranch := options.Plan.ParentBranch
	if switchErr := switchToLocalOrRemoteBranchWithAdoption(ctx, environment, repository, options.RemoteName, parentBranch, options.CommitMessages); switchErr != nil {
		return switchErr
	}
	remoteExists, remoteErr := remoteReferenceExists(ctx, environment.GitExecutor, repository.Path, fmt.Sprintf("%s/%s", options.RemoteName, parentBranch))
	if remoteErr != nil {
		return remoteErr
	}
	if remoteExists {
		if mergeErr := mergeRemoteBranchIntoLocal(ctx, environment, repository, environment.GitExecutor, repository.Path, options.RemoteName, parentBranch, options.CommitMessages); mergeErr != nil {
			return mergeErr
		}
	}
	return mergeBaseIntoBranch(ctx, environment, repository, environment.GitExecutor, repository.Path, options.RemoteName, baseBranch, parentBranch, options.CommitMessages)
}
