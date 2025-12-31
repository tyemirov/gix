package branches

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/tyemirov/gix/internal/branches/refresh"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	taskActionNameBranchCleanup  = "repo.branches.cleanup"
	taskActionNameBranchRefresh  = "branch.refresh"
	defaultBranchCleanupLimit    = 100
	branchCleanupRemoteError     = "branch cleanup action requires 'remote'"
	branchCleanupLimitParseError = "branch cleanup action requires numeric 'limit': %w"
	branchRefreshBranchError     = "branch refresh action requires 'branch'"
	branchRefreshMessageTemplate = "REFRESHED: %s (%s)\n"
	branchCleanupSummaryTemplate = "PR cleanup: %s closed=%d deleted=%d missing=%d declined=%d failed=%d\n"
)

func init() {
	workflow.RegisterTaskAction(taskActionNameBranchCleanup, handleBranchCleanupAction)
	workflow.RegisterTaskAction(taskActionNameBranchRefresh, handleBranchRefreshAction)
}

func handleBranchCleanupAction(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, parameters map[string]any) error {
	if environment == nil || environment.GitExecutor == nil || repository == nil {
		return nil
	}

	remoteValue, remoteExists := parameters["remote"]
	remoteString := strings.TrimSpace(stringify(remoteValue))
	if !remoteExists || len(remoteString) == 0 {
		return errors.New(branchCleanupRemoteError)
	}

	limitValue := parameters["limit"]
	cleanupLimit := defaultBranchCleanupLimit
	if trimmedLimit := strings.TrimSpace(stringify(limitValue)); len(trimmedLimit) > 0 {
		parsedLimit, parseError := strconv.Atoi(trimmedLimit)
		if parseError != nil {
			return fmt.Errorf(branchCleanupLimitParseError, parseError)
		}
		cleanupLimit = parsedLimit
	}

	service, serviceError := NewService(environment.Logger, environment.GitExecutor, environment.Prompter)
	if serviceError != nil {
		return serviceError
	}

	assumeYes := false
	if environment.PromptState != nil && environment.PromptState.IsAssumeYesEnabled() {
		assumeYes = true
	}

	options := CleanupOptions{
		RemoteName:       remoteString,
		PullRequestLimit: cleanupLimit,
		WorkingDirectory: repository.Path,
		AssumeYes:        assumeYes,
	}

	cleanupSummary, cleanupError := service.Cleanup(ctx, options)
	if cleanupError != nil {
		return cleanupError
	}

	if environment.Output != nil {
		fmt.Fprintf(
			environment.Output,
			branchCleanupSummaryTemplate,
			repository.Path,
			cleanupSummary.ClosedBranches,
			cleanupSummary.DeletedBranches,
			cleanupSummary.MissingBranches,
			cleanupSummary.DeclinedBranches,
			cleanupSummary.FailedBranches,
		)
	}

	return nil
}

func handleBranchRefreshAction(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, parameters map[string]any) error {
	if environment == nil || repository == nil || environment.GitExecutor == nil || environment.RepositoryManager == nil {
		return nil
	}

	branchName := strings.TrimSpace(stringify(parameters["branch"]))
	if len(branchName) == 0 {
		return errors.New(branchRefreshBranchError)
	}

	stashChanges, stashError := boolValue(parameters["stash"])
	if stashError != nil {
		return stashError
	}
	commitChanges, commitError := boolValue(parameters["commit"])
	if commitError != nil {
		return commitError
	}
	requireClean, requireCleanError := boolValueDefault(parameters["require_clean"], true)
	if requireCleanError != nil {
		return requireCleanError
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
		BranchName:     branchName,
		RequireClean:   requireClean,
		StashChanges:   stashChanges,
		CommitChanges:  commitChanges,
	})
	if refreshError != nil {
		return refreshError
	}

	if environment.Output != nil {
		fmt.Fprintf(environment.Output, branchRefreshMessageTemplate, repository.Path, branchName)
	}

	return nil
}

func stringify(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func boolValue(value any) (bool, error) {
	return boolValueDefault(value, false)
}

func boolValueDefault(value any, defaultValue bool) (bool, error) {
	if value == nil {
		return defaultValue, nil
	}

	switch typed := value.(type) {
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
		return false, fmt.Errorf("option must be boolean, received %v", value)
	}

	return false, fmt.Errorf("option must be boolean, received %v", value)
}
