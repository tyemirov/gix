package cd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/temirov/gix/internal/repos/shared"
	"github.com/temirov/gix/internal/workflow"
)

const (
	taskTypeBranchChange              = "branch.change"
	taskOptionBranchName              = "branch"
	taskOptionBranchRemote            = "remote"
	taskOptionBranchCreate            = "create_if_missing"
	taskOptionConfiguredDefaultBranch = "default_branch"

	branchResolutionSourceExplicit      = "explicit"
	branchResolutionSourceRemoteDefault = "remote_default"
	branchResolutionSourceConfigured    = "configured_default"
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
		DryRun:          environment.DryRun,
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
	created := result.BranchCreated && !environment.DryRun
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
