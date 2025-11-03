package cd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/workflow"
)

const (
	taskTypeBranchChange   = "branch.change"
	taskOptionBranchName   = "branch"
	taskOptionBranchRemote = "remote"
	taskOptionBranchCreate = "create_if_missing"
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
	if len(branchName) == 0 {
		return errors.New("branch change action requires branch name")
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

	service, serviceError := NewService(ServiceDependencies{
		GitExecutor: environment.GitExecutor,
		Logger:      environment.Logger,
	})
	if serviceError != nil {
		return serviceError
	}

	result, changeError := service.Change(ctx, Options{
		RepositoryPath:  repository.Path,
		BranchName:      branchName,
		RemoteName:      remoteName,
		CreateIfMissing: createIfMissing,
		DryRun:          environment.DryRun,
	})
	if changeError != nil {
		return changeError
	}

	if environment.Output != nil {
		for _, warning := range result.Warnings {
			fmt.Fprintln(environment.Output, warning)
		}
	}

	if environment.Output != nil {
		message := fmt.Sprintf(changeSuccessMessageTemplateConstant, result.RepositoryPath, result.BranchName)
		if result.BranchCreated && !environment.DryRun {
			message += changeCreatedSuffixConstant
		}
		fmt.Fprintln(environment.Output, message)
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
