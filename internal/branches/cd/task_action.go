package cd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/branches/refresh"
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/repos/worktree"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	taskTypeBranchChange              = "branch.change"
	taskOptionBranchName              = "branch"
	taskOptionBranchRemote            = "remote"
	taskOptionBranchCreate            = "create_if_missing"
	taskOptionConfiguredDefaultBranch = "default_branch"
	taskOptionRefreshEnabled          = "refresh"
	taskOptionRequireClean            = "require_clean"
	taskOptionStashChanges            = "stash"
	taskOptionCommitChanges           = "commit"

	branchResolutionSourceExplicit      = "explicit"
	branchResolutionSourceRemoteDefault = "remote_default"
	branchResolutionSourceConfigured    = "configured_default"

	branchRefreshMessageTemplate                 = "REFRESHED: %s (%s)\n"
	refreshMissingRepositoryManagerMessage       = "branch refresh requires repository manager"
	gitStashSubcommandConstant                   = "stash"
	gitStashPushSubcommandConstant               = "push"
	gitStashPopSubcommandConstant                = "pop"
	stashTrackedChangesFailureTemplateConstant   = "failed to stash tracked changes before switching: %w"
	restoreStashedChangesFailureTemplateConstant = "failed to restore stashed changes after switching: %w"
	stashExecutorMissingMessageConstant          = "git executor required to manage stash operations"
)

func init() {
	workflow.RegisterTaskAction(taskTypeBranchChange, handleBranchChangeAction)
}

func handleBranchChangeAction(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, parameters map[string]any) (err error) {
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
		return errors.New("branch.change cannot capture and restore simultaneously")
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
	requireClean, requireCleanErr := boolOptionDefault(parameters, taskOptionRequireClean, true)
	if requireCleanErr != nil {
		return requireCleanErr
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

	if len(resolvedBranchName) == 0 {
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
		environment.ReportRepositoryEvent(
			repository,
			shared.EventLevelWarn,
			shared.EventCodeRepoDirty,
			"untracked files present; refresh will continue",
			map[string]string{"status": strings.Join(untrackedStatus, ", ")},
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

	service, serviceError := NewService(ServiceDependencies{
		GitExecutor: environment.GitExecutor,
		Logger:      environment.Logger,
	})
	if serviceError != nil {
		return serviceError
	}

	result, changeError := service.Change(ctx, Options{
		RepositoryPath:  repository.Path,
		BranchName:      resolvedBranchName,
		RemoteName:      remoteName,
		CreateIfMissing: createIfMissing,
		SkipPull:        refreshSkipped,
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
		if refreshRequested && stashChanges && requireClean && len(untrackedStatus) > 0 {
			stashPushCount++
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

	if captureSpec != nil {
		if err := captureBranchState(ctx, environment, repository, captureSpec); err != nil {
			return err
		}
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
		message += changeCreatedSuffixConstant
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
