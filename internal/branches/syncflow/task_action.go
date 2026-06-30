package syncflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/branches/refresh"
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/repos/worktree"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	taskTypeBranchSync                = "branch.sync"
	taskOptionBranchName              = "branch"
	taskOptionBranchRemote            = "remote"
	taskOptionBranchCreate            = "create_if_missing"
	taskOptionConfiguredDefaultBranch = "default_branch"
	taskOptionRefreshEnabled          = "refresh"
	taskOptionRequireClean            = "require_clean"
	taskOptionStashChanges            = "stash"
	taskOptionCommitChanges           = "commit"
	taskOptionWorktreeCommitMessage   = "worktree_commit_message"
	taskOptionRequirePullRequest      = "require_pull_request"
	taskOptionBaseBranch              = "base_branch"
	taskOptionPullRequestTitle        = "title"
	taskOptionPullRequestBody         = "body"

	branchResolutionSourceExplicit      = "explicit"
	branchResolutionSourceRemoteDefault = "remote_default"
	branchResolutionSourceConfigured    = "configured_default"

	defaultSyncBaseBranch                        = "master"
	syncCurrentBranchSource                      = "current"
	branchRefreshMessageTemplate                 = "REFRESHED: %s (%s)\n"
	branchStrictSyncMessageTemplate              = "SYNCED: %s (%s)\n"
	refreshMissingRepositoryManagerMessage       = "branch refresh requires repository manager"
	gitStashSubcommandConstant                   = "stash"
	gitStashPushSubcommandConstant               = "push"
	gitStashIncludeUntrackedFlagConstant         = "--include-untracked"
	gitStashPopSubcommandConstant                = "pop"
	gitMergeSubcommandConstant                   = "merge"
	gitMergeNoEditFlagConstant                   = "--no-edit"
	gitMergeFastForwardOnlyFlagConstant          = "--ff-only"
	gitResetSubcommandConstant                   = "reset"
	gitResetHardFlagConstant                     = "--hard"
	gitRestoreSubcommandConstant                 = "restore"
	gitPushSubcommandConstant                    = "push"
	gitPushSetUpstreamFlagConstant               = "-u"
	gitRevListSubcommandConstant                 = "rev-list"
	gitRevListCountFlagConstant                  = "--count"
	gitAddSubcommandConstant                     = "add"
	gitAddAllFlagConstant                        = "--all"
	gitCommitSubcommandConstant                  = "commit"
	gitCommitMessageFlagConstant                 = "-m"
	gitSwitchTrackFlagConstant                   = "--track"
	stashTrackedChangesFailureTemplateConstant   = "failed to stash tracked changes before switching: %w"
	restoreStashedChangesFailureTemplateConstant = "failed to restore stashed changes after switching: %w"
	stashExecutorMissingMessageConstant          = "git executor required to manage stash operations"
	strictSyncMissingGitHubClientMessage         = "strict sync requires GitHub CLI access to verify pull requests"
	strictSyncMissingRepositoryMessage           = "strict sync requires a GitHub repository remote"
	strictSyncDirtyWorktreeTemplate              = "worktree is dirty; remove --require-clean or use --stash before syncing"
	strictSyncLocalOnlyCommitTemplate            = "local branch %q has commits not on %s/%s"
	strictSyncMissingPullRequestTemplate         = "branch %q does not have an open pull request"
	strictSyncMissingPullRequestBaseTemplate     = "open pull request for branch %q did not report a base branch"
	strictSyncMergedPullRequestPromptTemplate    = "Pull request for branch %q into %s is already merged. Sync %s instead? [a/N/y] "
	strictSyncConflictTemplate                   = "merge from %s/%s into %s stopped with conflicts; resolve them before pushing"
	strictSyncFastForwardTemplate                = "fast-forward from %s/%s into %s stopped; commit, stash, or clean local changes before syncing"
)

func init() {
	workflow.RegisterTaskAction(taskTypeBranchSync, handleBranchSyncAction)
}

func handleBranchSyncAction(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, parameters map[string]any) (err error) {
	if environment == nil || repository == nil {
		return nil
	}

	stashRestorationEnabled := false
	stashPushCount := 0
	defer func() {
		if !stashRestorationEnabled {
			return
		}
		if environment == nil || environment.GitExecutor == nil {
			err = errors.Join(err, errors.New(stashExecutorMissingMessageConstant))
			return
		}
		if restoreErr := restoreStashedChanges(ctx, environment.GitExecutor, repository.Path, stashPushCount); restoreErr != nil {
			err = errors.Join(err, restoreErr)
		}
	}()

	captureSpec, captureErr := workflow.ParseBranchCaptureSpec(parameters)
	if captureErr != nil {
		return captureErr
	}
	restoreSpec, restoreErr := workflow.ParseBranchRestoreSpec(parameters)
	if restoreErr != nil {
		return restoreErr
	}
	if captureSpec != nil && restoreSpec != nil {
		return errors.New("branch.sync cannot capture and restore simultaneously")
	}

	if restoreSpec != nil {
		return performBranchRestore(ctx, environment, repository, restoreSpec)
	}

	branchName, branchErr := stringOption(parameters, taskOptionBranchName)
	if branchErr != nil {
		return branchErr
	}
	configuredFallbackBranch, fallbackErr := optionalStringOption(parameters, taskOptionConfiguredDefaultBranch)
	if fallbackErr != nil {
		return fallbackErr
	}

	remoteName, remoteErr := optionalStringOption(parameters, taskOptionBranchRemote)
	if remoteErr != nil {
		return remoteErr
	}

	refreshRequested, refreshErr := boolOptionDefault(parameters, taskOptionRefreshEnabled, false)
	if refreshErr != nil {
		return refreshErr
	}
	stashChanges, stashErr := boolOption(parameters, taskOptionStashChanges)
	if stashErr != nil {
		return stashErr
	}
	commitChanges, commitErr := boolOption(parameters, taskOptionCommitChanges)
	if commitErr != nil {
		return commitErr
	}
	requireClean, requireCleanErr := boolOptionDefault(parameters, taskOptionRequireClean, false)
	if requireCleanErr != nil {
		return requireCleanErr
	}
	requirePullRequest, requirePullRequestErr := boolOptionDefault(parameters, taskOptionRequirePullRequest, false)
	if requirePullRequestErr != nil {
		return requirePullRequestErr
	}
	baseBranch, baseBranchErr := optionalStringOption(parameters, taskOptionBaseBranch)
	if baseBranchErr != nil {
		return baseBranchErr
	}
	if strings.TrimSpace(baseBranch) == "" {
		baseBranch = defaultSyncBaseBranch
	}
	if stashChanges && commitChanges {
		return errors.New(conflictingRecoveryFlagsMessageConstant)
	}
	if stashChanges || commitChanges {
		refreshRequested = true
	}
	if stashChanges {
		if environment.GitExecutor == nil {
			return errors.New(stashExecutorMissingMessageConstant)
		}
		stashRestorationEnabled = true
	}

	createIfMissing := false
	if createValue, exists := parameters[taskOptionBranchCreate]; exists {
		if typed, ok := createValue.(bool); ok {
			createIfMissing = typed
		}
	}

	resolvedBranchName := strings.TrimSpace(branchName)
	resolutionSource := branchResolutionSourceExplicit

	if len(resolvedBranchName) == 0 && requirePullRequest {
		currentBranch := strings.TrimSpace(repository.Inspection.LocalBranch)
		if len(currentBranch) > 0 {
			resolvedBranchName = currentBranch
			resolutionSource = syncCurrentBranchSource
		}
	}

	if len(resolvedBranchName) == 0 && !requirePullRequest {
		remoteDefault := strings.TrimSpace(repository.Inspection.RemoteDefaultBranch)
		if len(remoteDefault) > 0 {
			resolvedBranchName = remoteDefault
			resolutionSource = branchResolutionSourceRemoteDefault
		}
	}

	if len(resolvedBranchName) == 0 {
		configuredDefault := strings.TrimSpace(configuredFallbackBranch)
		if len(configuredDefault) > 0 {
			resolvedBranchName = configuredDefault
			resolutionSource = branchResolutionSourceConfigured
		}
	}

	if len(resolvedBranchName) == 0 {
		return errors.New(missingBranchMessageConstant)
	}

	if requirePullRequest {
		commitMessageOptions, commitMessageErr := worktreeAdoptionCommitMessageOptionsFromParameters(parameters)
		if commitMessageErr != nil {
			return commitMessageErr
		}
		pullRequestMetadata, pullRequestMetadataErr := strictSyncPullRequestMetadataFromParameters(parameters)
		if pullRequestMetadataErr != nil {
			return pullRequestMetadataErr
		}
		return handleStrictSyncAction(ctx, environment, repository, strictSyncOptions{
			BranchName:       resolvedBranchName,
			RemoteName:       remoteName,
			BaseBranch:       baseBranch,
			RequireClean:     requireClean,
			StashChanges:     stashChanges,
			CommitChanges:    commitChanges,
			CommitMessages:   commitMessageOptions,
			PullRequest:      pullRequestMetadata,
			ResolutionSource: resolutionSource,
		})
	}

	var trackedStatus []string
	var untrackedStatus []string
	if environment.RepositoryManager != nil {
		statusEntries, statusErr := environment.RepositoryManager.WorktreeStatus(ctx, repository.Path)
		if statusErr != nil {
			return statusErr
		}
		trackedStatus, untrackedStatus = worktree.SplitStatusEntries(statusEntries, nil)
	}
	refreshSkippedDetails := map[string]string{}
	refreshSkipped := false
	if refreshRequested && requireClean && !stashChanges && !commitChanges {
		if environment.RepositoryManager == nil {
			return errors.New(refreshMissingRepositoryManagerMessage)
		}
		if len(trackedStatus) > 0 {
			refreshRequested = false
			refreshSkipped = true
			refreshSkippedDetails["branch"] = resolvedBranchName
			refreshSkippedDetails["status"] = strings.Join(trackedStatus, ", ")
		}
	}

	if refreshRequested && len(untrackedStatus) > 0 {
		untrackedSummary := summarizeStatusEntries(untrackedStatus)
		message := "untracked files present; refresh will continue"
		if len(untrackedSummary) > 0 {
			message = fmt.Sprintf("%s [%s]", message, untrackedSummary)
		}
		details := map[string]string{"status": strings.Join(untrackedStatus, ", ")}
		if len(untrackedSummary) > 0 {
			details["paths"] = untrackedSummary
		}
		environment.ReportRepositoryEvent(
			repository,
			shared.EventLevelWarn,
			shared.EventCodeRepoDirty,
			message,
			details,
		)
	}

	if refreshSkipped && len(untrackedStatus) > 0 {
		refreshSkippedDetails["untracked"] = strings.Join(untrackedStatus, ", ")
	}

	if stashChanges && len(trackedStatus) > 0 {
		if err := stashTrackedChanges(ctx, environment.GitExecutor, repository.Path); err != nil {
			return err
		}
		stashPushCount++
	}

	if captureSpec != nil {
		if err := captureBranchState(ctx, environment, repository, captureSpec); err != nil {
			return err
		}
	}

	service, serviceError := NewService(ServiceDependencies{
		GitExecutor: environment.GitExecutor,
		Logger:      environment.Logger,
	})
	if serviceError != nil {
		return serviceError
	}

	changeOptions := Options{
		RepositoryPath:  repository.Path,
		BranchName:      resolvedBranchName,
		RemoteName:      remoteName,
		CreateIfMissing: createIfMissing,
		pullMode:        pullModeForRefreshState(refreshSkipped),
	}
	commitMessageOptions, commitMessageErr := worktreeAdoptionCommitMessageOptionsFromParameters(parameters)
	if commitMessageErr != nil {
		return commitMessageErr
	}
	var result Result
	changeError := newWorktreeAdoptionService(environment, repository).Change(ctx, worktreeAdoptionChangeOptions{
		BranchName:     resolvedBranchName,
		RemoteName:     remoteName,
		CommitMessages: commitMessageOptions,
		Change: func() error {
			var serviceChangeErr error
			result, serviceChangeErr = service.Change(ctx, changeOptions)
			return serviceChangeErr
		},
	})
	if changeError != nil {
		return changeError
	}

	if refreshRequested {
		hasTracking, trackingErr := branchHasTrackingRemote(ctx, environment.GitExecutor, repository.Path, result.BranchName)
		if trackingErr != nil {
			return trackingErr
		}
		if !hasTracking {
			remoteNameCandidate := strings.TrimSpace(result.TrackingRemoteName)
			if len(remoteNameCandidate) > 0 {
				configured, configureErr := ensureTrackingRemote(ctx, environment.GitExecutor, repository.Path, remoteNameCandidate, result.BranchName)
				if configureErr != nil {
					return configureErr
				}
				if configured {
					hasTracking = true
				}
			}
			if !hasTracking {
				refreshRequested = false
				messageDetails := map[string]string{"branch": result.BranchName}
				if len(remoteNameCandidate) > 0 {
					messageDetails["remote_candidate"] = fmt.Sprintf("%s/%s", remoteNameCandidate, result.BranchName)
				}
				environment.ReportRepositoryEvent(
					repository,
					shared.EventLevelWarn,
					shared.EventCodeTaskSkip,
					"refresh skipped (no tracking remote)",
					messageDetails,
				)
			}
		}
	}

	if refreshSkipped {
		refreshSkippedDetails["branch"] = result.BranchName
		environment.ReportRepositoryEvent(
			repository,
			shared.EventLevelWarn,
			shared.EventCodeTaskSkip,
			"refresh skipped (dirty worktree)",
			refreshSkippedDetails,
		)
	}

	for _, warning := range result.Warnings {
		environment.ReportRepositoryEvent(
			repository,
			shared.EventLevelWarn,
			shared.EventCodeTaskSkip,
			warning,
			map[string]string{"warning": strings.ReplaceAll(strings.TrimSpace(warning), " ", "_")},
		)
	}

	message := fmt.Sprintf("→ %s", result.BranchName)
	details := map[string]string{
		"branch": result.BranchName,
		"source": resolutionSource,
	}
	details["refresh"] = fmt.Sprintf("%t", refreshRequested)
	if refreshRequested {
		details["require_clean"] = fmt.Sprintf("%t", requireClean)
		if stashChanges {
			details["stash"] = "true"
		}
		if commitChanges {
			details["commit"] = "true"
		}
	}
	created := result.BranchCreated
	if created {
		message += syncCreatedSuffixConstant
	}
	details["created"] = fmt.Sprintf("%t", created)

	environment.ReportRepositoryEvent(
		repository,
		shared.EventLevelInfo,
		shared.EventCodeRepoSwitched,
		message,
		details,
	)

	if refreshRequested {
		if environment.RepositoryManager == nil || environment.GitExecutor == nil {
			return errors.New(refreshMissingRepositoryManagerMessage)
		}
		service, serviceError := refresh.NewService(refresh.Dependencies{
			GitExecutor:       environment.GitExecutor,
			RepositoryManager: environment.RepositoryManager,
		})
		if serviceError != nil {
			return serviceError
		}

		_, refreshError := service.Refresh(ctx, refresh.Options{
			RepositoryPath: repository.Path,
			BranchName:     result.BranchName,
			RequireClean:   requireClean,
			StashChanges:   stashChanges,
			CommitChanges:  commitChanges,
		})
		if refreshError != nil {
			return refreshError
		}

		if environment.Output != nil {
			fmt.Fprintf(environment.Output, branchRefreshMessageTemplate, repository.Path, result.BranchName)
		}
	}

	return nil
}

type strictSyncOptions struct {
	BranchName       string
	RemoteName       string
	BaseBranch       string
	RequireClean     bool
	StashChanges     bool
	CommitChanges    bool
	CommitMessages   worktreeAdoptionCommitMessageOptions
	PullRequest      strictSyncPullRequestMetadata
	ResolutionSource string
}

func handleStrictSyncAction(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, options strictSyncOptions) (err error) {
	if environment == nil || repository == nil {
		return nil
	}
	if environment.GitExecutor == nil {
		return ErrGitExecutorNotConfigured
	}
	if environment.RepositoryManager == nil {
		return errors.New(refreshMissingRepositoryManagerMessage)
	}

	branchName := strings.TrimSpace(options.BranchName)
	baseBranch := strings.TrimSpace(options.BaseBranch)
	if baseBranch == "" {
		baseBranch = defaultSyncBaseBranch
	}
	remoteName := strings.TrimSpace(options.RemoteName)
	if remoteName == "" {
		remoteName = defaultRemoteNameConstant
	}

	statusEntries, statusErr := environment.RepositoryManager.WorktreeStatus(ctx, repository.Path)
	if statusErr != nil {
		return statusErr
	}
	filteredStatus, filteredStatusErr := filterIgnoredSyncStatusEntries(ctx, environment.GitExecutor, repository.Path, statusEntries)
	if filteredStatusErr != nil {
		return filteredStatusErr
	}
	statusEntries = filteredStatus.StageableEntries
	trackedStatus, untrackedStatus := worktree.SplitStatusEntries(statusEntries, nil)
	dirty := len(trackedStatus) > 0 || len(untrackedStatus) > 0

	if dirty && options.RequireClean && !options.CommitChanges && !options.StashChanges {
		return errors.New(strictSyncDirtyWorktreeTemplate)
	}

	if fetchErr := executeGit(ctx, environment.GitExecutor, repository.Path, []string{gitFetchSubcommandConstant, gitFetchPruneFlagConstant, remoteName}); fetchErr != nil {
		return fmt.Errorf(gitFetchFailureTemplateConstant, fetchErr)
	}
	if restoreErr := restoreIgnoredSyncStatusEntries(ctx, environment.GitExecutor, repository.Path, filteredStatus.IgnoredTrackedEntries); restoreErr != nil {
		return restoreErr
	}

	stashPushed := false
	if dirty && options.StashChanges {
		if stashErr := stashAllChanges(ctx, environment.GitExecutor, repository.Path); stashErr != nil {
			return stashErr
		}
		stashPushed = true
		defer func() {
			if restoreErr := restoreStashedChanges(ctx, environment.GitExecutor, repository.Path, 1); restoreErr != nil {
				err = errors.Join(err, restoreErr)
			}
		}()
		dirty = false
	}

	if dirty && options.RequireClean && !options.CommitChanges {
		return errors.New(strictSyncDirtyWorktreeTemplate)
	}

	if dirty {
		if syncStatusEntriesHaveConflicts(statusEntries) {
			return errors.New(strictSyncConflictWorktreeMessage)
		}
		commitBranchName := branchName
		if commitBranchName == baseBranch {
			generatedBranchName, generatedBranchErr := selectGeneratedSyncBranchName(ctx, environment, repository, remoteName, baseBranch, options.CommitMessages)
			if generatedBranchErr != nil {
				return generatedBranchErr
			}
			commitBranchName = generatedBranchName
		}
		if prepareErr := prepareStrictSyncBranchForDirtyWork(ctx, environment, repository, remoteName, baseBranch, commitBranchName, options.CommitMessages); prepareErr != nil {
			return prepareErr
		}
		_, commitErr := saveDirtyWorkClusters(ctx, environment.GitExecutor, repository.Path, statusEntries, options.CommitMessages)
		if commitErr != nil {
			return fmt.Errorf(strictSyncDirtyCommitFailureTemplate, commitErr)
		}
		branchName = commitBranchName
		dirty = false
	}

	if branchName == baseBranch {
		if syncErr := syncBaseBranch(ctx, environment, repository, remoteName, baseBranch, options.CommitMessages); syncErr != nil {
			return syncErr
		}
		reportStrictSync(repository, environment, branchName, options.ResolutionSource, false, stashPushed)
		return nil
	}

	pullRequestSyncResult, syncErr := syncPullRequestBranch(ctx, environment, repository, strictPullRequestBranchOptions{
		BranchName:     branchName,
		RemoteName:     remoteName,
		BaseBranch:     baseBranch,
		DirtyWorktree:  dirty,
		CommitMessages: options.CommitMessages,
		PullRequest:    options.PullRequest,
	})
	if syncErr != nil {
		return syncErr
	}
	reportBranchName := branchName
	if strings.TrimSpace(pullRequestSyncResult.SyncedBranch) != "" {
		reportBranchName = pullRequestSyncResult.SyncedBranch
	}
	reportStrictSync(repository, environment, reportBranchName, options.ResolutionSource, pullRequestSyncResult.Created, stashPushed)
	return nil
}

type strictPullRequestBranchOptions struct {
	BranchName     string
	RemoteName     string
	BaseBranch     string
	DirtyWorktree  bool
	CommitMessages worktreeAdoptionCommitMessageOptions
	PullRequest    strictSyncPullRequestMetadata
}

type strictPullRequestBranchResult struct {
	Created      bool
	SyncedBranch string
}

type strictPullRequestCreateOptions struct {
	RepositoryIdentifier string
	BaseBranch           string
	BranchName           string
	Title                string
	Body                 string
}

func syncBaseBranch(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, remoteName string, baseBranch string, commitMessages worktreeAdoptionCommitMessageOptions) error {
	remoteReference := fmt.Sprintf("%s/%s", remoteName, baseBranch)
	remoteExists, remoteExistsErr := remoteReferenceExists(ctx, environment.GitExecutor, repository.Path, remoteReference)
	if remoteExistsErr != nil {
		return remoteExistsErr
	}
	if !remoteExists {
		return fmt.Errorf("remote base branch %q does not exist", remoteReference)
	}

	localExists, localExistsErr := localBranchExists(ctx, environment.GitExecutor, repository.Path, baseBranch)
	if localExistsErr != nil {
		return localExistsErr
	}
	if !localExists {
		return executeGit(ctx, environment.GitExecutor, repository.Path, []string{gitSwitchSubcommandConstant, gitCreateBranchFlagConstant, baseBranch, gitSwitchTrackFlagConstant, remoteReference})
	}

	if switchErr := switchToLocalOrRemoteBranchWithAdoption(ctx, environment, repository, remoteName, baseBranch, commitMessages); switchErr != nil {
		return switchErr
	}
	aheadCount, aheadErr := commitCount(ctx, environment.GitExecutor, repository.Path, fmt.Sprintf("%s..%s", remoteReference, baseBranch))
	if aheadErr != nil {
		return aheadErr
	}
	if aheadCount > 0 {
		return fmt.Errorf(strictSyncLocalOnlyCommitTemplate, baseBranch, remoteName, baseBranch)
	}
	return executeGit(ctx, environment.GitExecutor, repository.Path, []string{gitResetSubcommandConstant, gitResetHardFlagConstant, remoteReference})
}

func syncPullRequestBranch(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, options strictPullRequestBranchOptions) (strictPullRequestBranchResult, error) {
	remoteReference := fmt.Sprintf("%s/%s", options.RemoteName, options.BranchName)
	remoteExists, remoteExistsErr := remoteReferenceExists(ctx, environment.GitExecutor, repository.Path, remoteReference)
	if remoteExistsErr != nil {
		return strictPullRequestBranchResult{}, remoteExistsErr
	}

	repositoryIdentifier := strictSyncRepositoryIdentifier(repository)
	if repositoryIdentifier == "" {
		return strictPullRequestBranchResult{}, errors.New(strictSyncMissingRepositoryMessage)
	}

	if remoteExists {
		openPullRequest, pullRequestErr := openPullRequestForBranch(ctx, environment, repositoryIdentifier, options.BranchName)
		if pullRequestErr != nil {
			return strictPullRequestBranchResult{}, pullRequestErr
		}
		if openPullRequest == nil {
			syncedBaseBranch, syncBaseBranchErr := syncBaseBranchAfterMergedPullRequest(ctx, environment, repository, repositoryIdentifier, options)
			if syncBaseBranchErr != nil {
				return strictPullRequestBranchResult{}, syncBaseBranchErr
			}
			if syncedBaseBranch {
				return strictPullRequestBranchResult{SyncedBranch: options.BaseBranch}, nil
			}
			return strictPullRequestBranchResult{}, fmt.Errorf(strictSyncMissingPullRequestTemplate, options.BranchName)
		}
		pullRequestBaseBranch, pullRequestBaseBranchErr := openPullRequestBaseBranch(*openPullRequest, options.BranchName)
		if pullRequestBaseBranchErr != nil {
			return strictPullRequestBranchResult{}, pullRequestBaseBranchErr
		}
		if switchErr := switchToLocalOrRemoteBranchWithAdoption(ctx, environment, repository, options.RemoteName, options.BranchName, options.CommitMessages); switchErr != nil {
			return strictPullRequestBranchResult{}, switchErr
		}
		aheadCount, aheadErr := commitCount(ctx, environment.GitExecutor, repository.Path, fmt.Sprintf("%s..%s", remoteReference, options.BranchName))
		if aheadErr != nil {
			return strictPullRequestBranchResult{}, aheadErr
		}
		if aheadCount > 0 {
			if mergeErr := mergeRemoteBranchIntoLocal(ctx, environment.GitExecutor, repository.Path, options.RemoteName, options.BranchName, options.CommitMessages); mergeErr != nil {
				return strictPullRequestBranchResult{}, mergeErr
			}
		} else if options.DirtyWorktree {
			if fastForwardErr := fastForwardRemoteBranchIntoLocal(ctx, environment.GitExecutor, repository.Path, options.RemoteName, options.BranchName); fastForwardErr != nil {
				return strictPullRequestBranchResult{}, fastForwardErr
			}
		} else if resetErr := executeGit(ctx, environment.GitExecutor, repository.Path, []string{gitResetSubcommandConstant, gitResetHardFlagConstant, remoteReference}); resetErr != nil {
			return strictPullRequestBranchResult{}, resetErr
		}
		if mergeErr := mergeBaseIntoBranch(ctx, environment.GitExecutor, repository.Path, options.RemoteName, pullRequestBaseBranch, options.BranchName, options.CommitMessages); mergeErr != nil {
			return strictPullRequestBranchResult{}, mergeErr
		}
		return strictPullRequestBranchResult{}, executeGit(ctx, environment.GitExecutor, repository.Path, []string{gitPushSubcommandConstant, options.RemoteName, options.BranchName})
	}

	localExists, localExistsErr := localBranchExists(ctx, environment.GitExecutor, repository.Path, options.BranchName)
	if localExistsErr != nil {
		return strictPullRequestBranchResult{}, localExistsErr
	}
	if localExists {
		localAheadOfBase, localAheadOfBaseErr := branchHasCommitsBeyondBase(ctx, environment.GitExecutor, repository.Path, options.RemoteName, options.BaseBranch, options.BranchName)
		if localAheadOfBaseErr != nil {
			return strictPullRequestBranchResult{}, localAheadOfBaseErr
		}
		if !localAheadOfBase {
			syncedBaseBranch, syncBaseBranchErr := syncBaseBranchAfterMergedPullRequest(ctx, environment, repository, repositoryIdentifier, options)
			if syncBaseBranchErr != nil {
				return strictPullRequestBranchResult{}, syncBaseBranchErr
			}
			if syncedBaseBranch {
				return strictPullRequestBranchResult{SyncedBranch: options.BaseBranch}, nil
			}
		}
		if switchErr := switchToLocalOrRemoteBranchWithAdoption(ctx, environment, repository, options.RemoteName, options.BranchName, options.CommitMessages); switchErr != nil {
			return strictPullRequestBranchResult{}, switchErr
		}
		if mergeErr := mergeBaseIntoBranch(ctx, environment.GitExecutor, repository.Path, options.RemoteName, options.BaseBranch, options.BranchName, options.CommitMessages); mergeErr != nil {
			return strictPullRequestBranchResult{}, mergeErr
		}
		if pullRequestErr := pushAndCreatePullRequest(ctx, environment, repository, repositoryIdentifier, options); pullRequestErr != nil {
			return strictPullRequestBranchResult{}, pullRequestErr
		}
		return strictPullRequestBranchResult{Created: true}, nil
	}

	baseReference := fmt.Sprintf("%s/%s", options.RemoteName, options.BaseBranch)
	baseExists, baseExistsErr := remoteReferenceExists(ctx, environment.GitExecutor, repository.Path, baseReference)
	if baseExistsErr != nil {
		return strictPullRequestBranchResult{}, baseExistsErr
	}
	if !baseExists {
		return strictPullRequestBranchResult{}, fmt.Errorf("remote base branch %q does not exist", baseReference)
	}
	if createErr := executeGit(ctx, environment.GitExecutor, repository.Path, []string{gitSwitchSubcommandConstant, gitCreateBranchFlagConstant, options.BranchName, baseReference}); createErr != nil {
		return strictPullRequestBranchResult{}, createErr
	}
	if pullRequestErr := pushAndCreatePullRequest(ctx, environment, repository, repositoryIdentifier, options); pullRequestErr != nil {
		return strictPullRequestBranchResult{}, pullRequestErr
	}
	return strictPullRequestBranchResult{Created: true}, nil
}

func branchHasCommitsBeyondBase(ctx context.Context, executor shared.GitExecutor, repositoryPath string, remoteName string, baseBranch string, branchName string) (bool, error) {
	baseReference := fmt.Sprintf("%s/%s", remoteName, baseBranch)
	aheadCount, aheadErr := commitCount(ctx, executor, repositoryPath, fmt.Sprintf("%s..%s", baseReference, branchName))
	if aheadErr != nil {
		return false, aheadErr
	}
	return aheadCount > 0, nil
}

func syncBaseBranchAfterMergedPullRequest(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, repositoryIdentifier string, options strictPullRequestBranchOptions) (bool, error) {
	mergedPullRequest, mergedPullRequestErr := branchHasMergedPullRequest(ctx, environment, repositoryIdentifier, options.BaseBranch, options.BranchName)
	if mergedPullRequestErr != nil {
		return false, mergedPullRequestErr
	}
	if !mergedPullRequest {
		return false, nil
	}
	syncBaseBranchConfirmed, confirmErr := confirmSyncBaseAfterMergedPullRequest(environment, options.BranchName, options.BaseBranch)
	if confirmErr != nil {
		return false, confirmErr
	}
	if !syncBaseBranchConfirmed {
		return false, nil
	}
	if syncErr := syncBaseBranch(ctx, environment, repository, options.RemoteName, options.BaseBranch, options.CommitMessages); syncErr != nil {
		return false, syncErr
	}
	return true, nil
}

func pushAndCreatePullRequest(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, repositoryIdentifier string, options strictPullRequestBranchOptions) error {
	pullRequestMetadata, pullRequestMetadataErr := resolveStrictSyncPullRequestMetadata(ctx, environment.GitExecutor, strictSyncPullRequestMetadataOptions{
		RepositoryPath: repository.Path,
		RemoteName:     options.RemoteName,
		BaseBranch:     options.BaseBranch,
		BranchName:     options.BranchName,
		PullRequest:    options.PullRequest,
		CommitMessages: options.CommitMessages,
	})
	if pullRequestMetadataErr != nil {
		return pullRequestMetadataErr
	}
	if pushErr := executeGit(ctx, environment.GitExecutor, repository.Path, []string{gitPushSubcommandConstant, gitPushSetUpstreamFlagConstant, options.RemoteName, options.BranchName}); pushErr != nil {
		return pushErr
	}
	return createPullRequest(ctx, environment, strictPullRequestCreateOptions{
		RepositoryIdentifier: repositoryIdentifier,
		BaseBranch:           options.BaseBranch,
		BranchName:           options.BranchName,
		Title:                pullRequestMetadata.Title,
		Body:                 pullRequestMetadata.Body,
	})
}

func switchToLocalOrRemoteBranch(ctx context.Context, executor shared.GitExecutor, repositoryPath string, remoteName string, branchName string) error {
	if switchErr := executeGit(ctx, executor, repositoryPath, []string{gitSwitchSubcommandConstant, branchName}); switchErr == nil {
		return nil
	} else if !isBranchMissingError(switchErr) {
		return switchErr
	}
	remoteReference := fmt.Sprintf("%s/%s", remoteName, branchName)
	return executeGit(ctx, executor, repositoryPath, []string{gitSwitchSubcommandConstant, gitCreateBranchFlagConstant, branchName, gitSwitchTrackFlagConstant, remoteReference})
}

func switchToLocalOrRemoteBranchWithAdoption(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, remoteName string, branchName string, commitMessages worktreeAdoptionCommitMessageOptions) error {
	return newWorktreeAdoptionService(environment, repository).Change(ctx, worktreeAdoptionChangeOptions{
		BranchName:                 branchName,
		RemoteName:                 remoteName,
		CommitMessages:             commitMessages,
		RefetchRemoteAfterAdoption: true,
		Change: func() error {
			return switchToLocalOrRemoteBranch(ctx, environment.GitExecutor, repository.Path, remoteName, branchName)
		},
	})
}

func mergeBaseIntoBranch(ctx context.Context, executor shared.GitExecutor, repositoryPath string, remoteName string, baseBranch string, branchName string, commitMessages worktreeAdoptionCommitMessageOptions) error {
	baseReference := fmt.Sprintf("%s/%s", remoteName, baseBranch)
	if mergeErr := executeGit(ctx, executor, repositoryPath, []string{gitMergeSubcommandConstant, gitMergeNoEditFlagConstant, baseReference}); mergeErr != nil {
		return resolveMergeConflictOrError(ctx, executor, repositoryPath, baseReference, branchName, fmt.Sprintf(strictSyncConflictTemplate, remoteName, baseBranch, branchName), commitMessages, mergeErr)
	}
	return nil
}

func mergeRemoteBranchIntoLocal(ctx context.Context, executor shared.GitExecutor, repositoryPath string, remoteName string, branchName string, commitMessages worktreeAdoptionCommitMessageOptions) error {
	remoteReference := fmt.Sprintf("%s/%s", remoteName, branchName)
	if mergeErr := executeGit(ctx, executor, repositoryPath, []string{gitMergeSubcommandConstant, gitMergeNoEditFlagConstant, remoteReference}); mergeErr != nil {
		return resolveMergeConflictOrError(ctx, executor, repositoryPath, remoteReference, branchName, fmt.Sprintf(strictSyncConflictTemplate, remoteName, branchName, branchName), commitMessages, mergeErr)
	}
	return nil
}

func fastForwardRemoteBranchIntoLocal(ctx context.Context, executor shared.GitExecutor, repositoryPath string, remoteName string, branchName string) error {
	remoteReference := fmt.Sprintf("%s/%s", remoteName, branchName)
	if mergeErr := executeGit(ctx, executor, repositoryPath, []string{gitMergeSubcommandConstant, gitMergeFastForwardOnlyFlagConstant, remoteReference}); mergeErr != nil {
		return fmt.Errorf("%s: %w", fmt.Sprintf(strictSyncFastForwardTemplate, remoteName, branchName, branchName), mergeErr)
	}
	return nil
}

func openPullRequestForBranch(ctx context.Context, environment *workflow.Environment, repositoryIdentifier string, branchName string) (*githubcli.PullRequest, error) {
	return pullRequestForBranchWithState(ctx, environment, repositoryIdentifier, "", branchName, githubcli.PullRequestStateOpen)
}

func branchHasMergedPullRequest(ctx context.Context, environment *workflow.Environment, repositoryIdentifier string, baseBranch string, branchName string) (bool, error) {
	pullRequest, pullRequestErr := pullRequestForBranchWithState(ctx, environment, repositoryIdentifier, baseBranch, branchName, githubcli.PullRequestStateMerged)
	if pullRequestErr != nil {
		return false, pullRequestErr
	}
	return pullRequest != nil, nil
}

func pullRequestForBranchWithState(ctx context.Context, environment *workflow.Environment, repositoryIdentifier string, baseBranch string, branchName string, state githubcli.PullRequestState) (*githubcli.PullRequest, error) {
	if environment.GitHubClient == nil {
		return nil, errors.New(strictSyncMissingGitHubClientMessage)
	}
	pullRequests, pullRequestErr := environment.GitHubClient.ListPullRequests(ctx, repositoryIdentifier, githubcli.PullRequestListOptions{
		State:       state,
		BaseBranch:  baseBranch,
		HeadBranch:  branchName,
		ResultLimit: 100,
	})
	if pullRequestErr != nil {
		return nil, pullRequestErr
	}
	for _, pullRequest := range pullRequests {
		if strings.TrimSpace(pullRequest.HeadRefName) == branchName {
			matchedPullRequest := pullRequest
			return &matchedPullRequest, nil
		}
	}
	return nil, nil
}

func openPullRequestBaseBranch(pullRequest githubcli.PullRequest, branchName string) (string, error) {
	baseBranch := strings.TrimSpace(pullRequest.BaseRefName)
	if baseBranch != "" {
		return baseBranch, nil
	}
	return "", fmt.Errorf(strictSyncMissingPullRequestBaseTemplate, branchName)
}

func confirmSyncBaseAfterMergedPullRequest(environment *workflow.Environment, branchName string, baseBranch string) (bool, error) {
	if environment == nil {
		return false, nil
	}
	if environment.PromptState != nil && environment.PromptState.IsAssumeYesEnabled() {
		return true, nil
	}
	if environment.Prompter == nil {
		return false, nil
	}
	confirmation, confirmErr := environment.Prompter.Confirm(fmt.Sprintf(strictSyncMergedPullRequestPromptTemplate, branchName, baseBranch, baseBranch))
	if confirmErr != nil {
		return false, confirmErr
	}
	return confirmation.Confirmed, nil
}

func createPullRequest(ctx context.Context, environment *workflow.Environment, options strictPullRequestCreateOptions) error {
	if environment.GitHubClient == nil {
		return errors.New(strictSyncMissingGitHubClientMessage)
	}
	return environment.GitHubClient.CreatePullRequest(ctx, githubcli.PullRequestCreateOptions{
		Repository: options.RepositoryIdentifier,
		Title:      options.Title,
		Body:       options.Body,
		Base:       options.BaseBranch,
		Head:       options.BranchName,
	})
}

func strictSyncRepositoryIdentifier(repository *workflow.RepositoryState) string {
	if repository == nil {
		return ""
	}
	for _, candidate := range []string{
		repository.Inspection.FinalOwnerRepo,
		repository.Inspection.CanonicalOwnerRepo,
		repository.Inspection.OriginOwnerRepo,
	} {
		trimmed := strings.TrimSpace(candidate)
		if trimmed != "" && trimmed != "n/a" {
			return trimmed
		}
	}
	return ""
}

func localBranchExists(ctx context.Context, executor shared.GitExecutor, repositoryPath string, branchName string) (bool, error) {
	localReference := fmt.Sprintf("refs/heads/%s", strings.TrimSpace(branchName))
	_, err := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitRevParseSubcommandConstant, gitVerifyFlagConstant, localReference},
		WorkingDirectory: repositoryPath,
	})
	if err == nil {
		return true, nil
	}
	if isBranchMissingError(err) {
		return false, nil
	}
	return false, err
}

func remoteReferenceExists(ctx context.Context, executor shared.GitExecutor, repositoryPath string, reference string) (bool, error) {
	_, err := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitRevParseSubcommandConstant, gitVerifyFlagConstant, reference},
		WorkingDirectory: repositoryPath,
	})
	if err == nil {
		return true, nil
	}
	if isBranchMissingError(err) {
		return false, nil
	}
	return false, err
}

func commitCount(ctx context.Context, executor shared.GitExecutor, repositoryPath string, revisionRange string) (int, error) {
	result, err := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitRevListSubcommandConstant, gitRevListCountFlagConstant, revisionRange},
		WorkingDirectory: repositoryPath,
	})
	if err != nil {
		return 0, err
	}
	trimmed := strings.TrimSpace(result.StandardOutput)
	if trimmed == "" {
		return 0, nil
	}
	var count int
	if _, scanErr := fmt.Sscanf(trimmed, "%d", &count); scanErr != nil {
		return 0, scanErr
	}
	return count, nil
}

func stashAllChanges(ctx context.Context, executor shared.GitExecutor, repositoryPath string) error {
	if err := executeGit(ctx, executor, repositoryPath, []string{gitStashSubcommandConstant, gitStashPushSubcommandConstant, gitStashIncludeUntrackedFlagConstant}); err != nil {
		return fmt.Errorf(stashTrackedChangesFailureTemplateConstant, err)
	}
	return nil
}

func reportStrictSync(repository *workflow.RepositoryState, environment *workflow.Environment, branchName string, source string, created bool, stashed bool) {
	details := map[string]string{
		"branch":  branchName,
		"source":  strings.TrimSpace(source),
		"created": fmt.Sprintf("%t", created),
	}
	if stashed {
		details["stash"] = "true"
	}
	message := fmt.Sprintf("→ %s", branchName)
	if created {
		message += syncCreatedSuffixConstant
	}
	environment.ReportRepositoryEvent(repository, shared.EventLevelInfo, shared.EventCodeRepoSwitched, message, details)
	if environment.Output != nil {
		fmt.Fprintf(environment.Output, branchStrictSyncMessageTemplate, repository.Path, branchName)
	}
}

func pullModeForRefreshState(refreshSkipped bool) pullMode {
	return pullModeFastForwardOnly
}

func branchHasTrackingRemote(ctx context.Context, executor shared.GitExecutor, repositoryPath string, branchName string) (bool, error) {
	if executor == nil {
		return false, errors.New("git executor required to inspect tracking state")
	}
	configKey := fmt.Sprintf("branch.%s.remote", branchName)
	_, err := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{"config", "--get", configKey},
		WorkingDirectory: repositoryPath,
	})
	if err == nil {
		return true, nil
	}
	var commandFailure execshell.CommandFailedError
	if errors.As(err, &commandFailure) && commandFailure.Result.ExitCode == 1 {
		return false, nil
	}
	return false, err
}

func ensureTrackingRemote(ctx context.Context, executor shared.GitExecutor, repositoryPath string, remoteName string, branchName string) (bool, error) {
	trimmedRemote := strings.TrimSpace(remoteName)
	trimmedBranch := strings.TrimSpace(branchName)
	if len(trimmedRemote) == 0 || len(trimmedBranch) == 0 {
		return false, nil
	}
	if executor == nil {
		return false, errors.New("git executor required to configure tracking remote")
	}
	environment := map[string]string{gitTerminalPromptEnvironmentNameConstant: gitTerminalPromptEnvironmentDisableValue}
	remoteReference := fmt.Sprintf("%s/%s", trimmedRemote, trimmedBranch)
	_, err := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:            []string{gitRevParseSubcommandConstant, gitVerifyFlagConstant, remoteReference},
		WorkingDirectory:     repositoryPath,
		EnvironmentVariables: environment,
	})
	if err != nil {
		if isBranchMissingError(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to verify remote branch %q: %w", remoteReference, err)
	}
	setUpstreamFlag := fmt.Sprintf("%s=%s", gitSetUpstreamToFlagConstant, remoteReference)
	_, err = executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:            []string{gitBranchSubcommandConstant, setUpstreamFlag, trimmedBranch},
		WorkingDirectory:     repositoryPath,
		EnvironmentVariables: environment,
	})
	if err != nil {
		return false, fmt.Errorf("failed to configure tracking remote for %s: %w", trimmedBranch, err)
	}
	return true, nil
}

func stringOption(options map[string]any, key string) (string, error) {
	raw, exists := options[key]
	if !exists {
		return "", nil
	}
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed), nil
	default:
		return "", fmt.Errorf("%s must be a string", key)
	}
}

func performBranchRestore(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, spec *workflow.BranchRestoreSpec) error {
	if environment == nil || repository == nil || spec == nil {
		return nil
	}
	if environment.Variables == nil {
		return fmt.Errorf("capture %s is not defined", spec.Name)
	}

	value, exists := environment.Variables.Get(spec.Name)
	if !exists || len(strings.TrimSpace(value)) == 0 {
		return fmt.Errorf("capture %s is not defined", spec.Name)
	}

	restoreKind := spec.Kind
	if !spec.KindExplicit {
		if recordedKind, ok := environment.CaptureKindForVariable(spec.Name); ok {
			restoreKind = recordedKind
		}
	}
	if restoreKind == "" {
		restoreKind = workflow.CaptureKindBranch
	}

	switch restoreKind {
	case workflow.CaptureKindBranch:
		service, serviceError := NewService(ServiceDependencies{
			GitExecutor: environment.GitExecutor,
			Logger:      environment.Logger,
		})
		if serviceError != nil {
			return serviceError
		}
		_, changeErr := service.Change(ctx, Options{
			RepositoryPath: repository.Path,
			BranchName:     strings.TrimSpace(value),
		})
		return changeErr
	case workflow.CaptureKindCommit:
		if environment.GitExecutor == nil {
			return errors.New("git executor required to restore commit")
		}
		command := execshell.CommandDetails{
			Arguments:        []string{"checkout", strings.TrimSpace(value)},
			WorkingDirectory: repository.Path,
		}
		_, execErr := environment.GitExecutor.ExecuteGit(ctx, command)
		return execErr
	default:
		return fmt.Errorf("unsupported restore value %q", restoreKind)
	}
}

func captureBranchState(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, spec *workflow.BranchCaptureSpec) error {
	if environment == nil || repository == nil || spec == nil {
		return nil
	}
	if environment.RepositoryManager == nil {
		return errors.New("repository manager required to capture branch state")
	}
	if environment.Variables == nil {
		environment.Variables = workflow.NewVariableStore()
	}

	var capturedValue string
	switch spec.Kind {
	case workflow.CaptureKindBranch:
		currentBranch, branchErr := environment.RepositoryManager.GetCurrentBranch(ctx, repository.Path)
		if branchErr != nil {
			return branchErr
		}
		if len(strings.TrimSpace(currentBranch)) == 0 {
			return errors.New("cannot capture current branch: repository is not on a named branch")
		}
		capturedValue = strings.TrimSpace(currentBranch)
	case workflow.CaptureKindCommit:
		if environment.GitExecutor == nil {
			return errors.New("git executor required to capture commit")
		}
		command := execshell.CommandDetails{
			Arguments:        []string{"rev-parse", "HEAD"},
			WorkingDirectory: repository.Path,
		}
		result, execErr := environment.GitExecutor.ExecuteGit(ctx, command)
		if execErr != nil {
			return execErr
		}
		capturedValue = strings.TrimSpace(result.StandardOutput)
	default:
		return fmt.Errorf("unsupported capture value %q", spec.Kind)
	}

	environment.StoreCaptureValue(spec.Name, capturedValue, spec.Overwrite)
	environment.RecordCaptureKind(spec.Name, spec.Kind)
	return nil
}

func stashTrackedChanges(ctx context.Context, executor shared.GitExecutor, repositoryPath string) error {
	if executor == nil {
		return errors.New(stashExecutorMissingMessageConstant)
	}
	if _, err := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitStashSubcommandConstant, gitStashPushSubcommandConstant},
		WorkingDirectory: repositoryPath,
	}); err != nil {
		return fmt.Errorf(stashTrackedChangesFailureTemplateConstant, err)
	}
	return nil
}

func restoreStashedChanges(ctx context.Context, executor shared.GitExecutor, repositoryPath string, stashPushCount int) error {
	if executor == nil {
		return errors.New(stashExecutorMissingMessageConstant)
	}
	for i := 0; i < stashPushCount; i++ {
		if _, err := executor.ExecuteGit(ctx, execshell.CommandDetails{
			Arguments:        []string{gitStashSubcommandConstant, gitStashPopSubcommandConstant},
			WorkingDirectory: repositoryPath,
		}); err != nil {
			return fmt.Errorf(restoreStashedChangesFailureTemplateConstant, err)
		}
	}
	return nil
}

func optionalStringOption(options map[string]any, key string) (string, error) {
	value, err := stringOption(options, key)
	if err != nil {
		return "", err
	}
	return value, nil
}

func boolOption(options map[string]any, key string) (bool, error) {
	return boolOptionDefault(options, key, false)
}

func boolOptionDefault(options map[string]any, key string, defaultValue bool) (bool, error) {
	raw, exists := options[key]
	if !exists {
		return defaultValue, nil
	}
	switch typed := raw.(type) {
	case bool:
		return typed, nil
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(typed))
		if trimmed == "" {
			return defaultValue, nil
		}
		if trimmed == "true" {
			return true, nil
		}
		if trimmed == "false" {
			return false, nil
		}
	default:
		return false, fmt.Errorf("%s must be boolean", key)
	}
	return false, fmt.Errorf("%s must be boolean", key)
}

func summarizeStatusEntries(entries []string) string {
	if len(entries) == 0 {
		return ""
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		path := strings.TrimSpace(worktree.StatusEntryPath(entry))
		if len(path) == 0 {
			path = strings.TrimSpace(entry)
		}
		if len(path) == 0 {
			continue
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return ""
	}
	return strings.Join(paths, ", ")
}
