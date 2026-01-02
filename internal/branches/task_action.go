package branches

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/tyemirov/gix/internal/branches/refresh"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	taskActionNameBranchCleanup        = "repo.branches.cleanup"
	taskActionNameBranchRefresh        = "branch.refresh"
	defaultBranchCleanupLimit          = 100
	branchCleanupRemoteError           = "branch cleanup action requires 'remote'"
	branchCleanupLimitParseError       = "branch cleanup action requires numeric 'limit': %w"
	branchRefreshBranchError           = "branch refresh action requires 'branch'"
	branchRefreshMessageTemplate       = "REFRESHED: %s (%s)\n"
	branchCleanupSummaryTemplate       = "PR cleanup: %s closed=%d deleted=%d missing=%d declined=%d failed=%d\n"
	branchCleanupFailureHeaderTemplate = "PR cleanup failures: %s failed=%d showing=%d total=%d\n"
	branchCleanupFailureLineTemplate   = "PR cleanup failure: %s branch=%s %s\n"
	branchCleanupFailureRemainder      = "PR cleanup failures: %s remaining=%d\n"
	branchCleanupFailureFallback       = "PR cleanup failures: %s failed=%d\n"
	branchCleanupFailureDetailNone     = "detail=missing"
	branchCleanupFailureDetailTemplate = "%s=%s"
	branchCleanupFailureDetailPrompt   = "prompt"
	branchCleanupFailureDetailRemote   = "remote"
	branchCleanupFailureDetailLocal    = "local"
	branchCleanupFailureMaxLines       = 5
)

var branchCleanupHTTPSCredentialPattern = regexp.MustCompile(`(?i)(https?://)[^/\s]+@`)

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
	writeBranchCleanupFailureDetails(environment, repository.Path, cleanupSummary)

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

func writeBranchCleanupFailureDetails(environment *workflow.Environment, repositoryPath string, summary CleanupSummary) {
	if environment == nil || summary.FailedBranches == 0 {
		return
	}

	errorWriter := resolveBranchCleanupFailureWriter(environment)
	if errorWriter == nil {
		return
	}

	failures := summary.Failures
	if len(failures) == 0 {
		fmt.Fprintf(errorWriter, branchCleanupFailureFallback, repositoryPath, summary.FailedBranches)
		return
	}

	linesToPrint := branchCleanupFailureMaxLines
	if len(failures) < linesToPrint {
		linesToPrint = len(failures)
	}

	fmt.Fprintf(errorWriter, branchCleanupFailureHeaderTemplate, repositoryPath, summary.FailedBranches, linesToPrint, len(failures))
	for failureIndex := 0; failureIndex < linesToPrint; failureIndex++ {
		failure := failures[failureIndex]
		formatted := formatBranchCleanupFailure(failure)
		fmt.Fprintf(errorWriter, branchCleanupFailureLineTemplate, repositoryPath, failure.BranchName, formatted)
	}

	if remaining := len(failures) - linesToPrint; remaining > 0 {
		fmt.Fprintf(errorWriter, branchCleanupFailureRemainder, repositoryPath, remaining)
	}
}

func resolveBranchCleanupFailureWriter(environment *workflow.Environment) io.Writer {
	if environment == nil {
		return nil
	}
	if environment.Errors != nil {
		return environment.Errors
	}
	return environment.Output
}

func formatBranchCleanupFailure(failure CleanupFailure) string {
	details := make([]string, 0, 3)

	promptDetail := strings.TrimSpace(failure.PromptError)
	if len(promptDetail) > 0 {
		details = append(details, fmt.Sprintf(branchCleanupFailureDetailTemplate, branchCleanupFailureDetailPrompt, sanitizeBranchCleanupFailureDetail(promptDetail)))
	}

	remoteDetail := strings.TrimSpace(failure.RemoteDeletionError)
	if len(remoteDetail) > 0 {
		details = append(details, fmt.Sprintf(branchCleanupFailureDetailTemplate, branchCleanupFailureDetailRemote, sanitizeBranchCleanupFailureDetail(remoteDetail)))
	}

	localDetail := strings.TrimSpace(failure.LocalDeletionError)
	if len(localDetail) > 0 {
		details = append(details, fmt.Sprintf(branchCleanupFailureDetailTemplate, branchCleanupFailureDetailLocal, sanitizeBranchCleanupFailureDetail(localDetail)))
	}

	if len(details) == 0 {
		return branchCleanupFailureDetailNone
	}
	return strings.Join(details, " ")
}

func sanitizeBranchCleanupFailureDetail(detail string) string {
	if len(detail) == 0 {
		return detail
	}
	redacted := branchCleanupHTTPSCredentialPattern.ReplaceAllString(detail, "${1}***@")
	redacted = strings.ReplaceAll(redacted, "\n", " | ")
	return strings.TrimSpace(redacted)
}
