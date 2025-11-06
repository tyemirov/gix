package cd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/temirov/gix/internal/branches/refresh"
	"github.com/temirov/gix/internal/repos/shared"
	"github.com/temirov/gix/internal/workflow"
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

	branchRefreshMessageTemplate           = "REFRESHED: %s (%s)\n"
	refreshMissingRepositoryManagerMessage = "branch refresh requires repository manager"
)

func init() {
	workflow.RegisterTaskAction(taskTypeBranchChange, handleBranchChangeAction)
}

func handleBranchChangeAction(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, parameters map[string]any) error {
	if environment == nil || repository == nil {
		return nil
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
	})
	if changeError != nil {
		return changeError
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
