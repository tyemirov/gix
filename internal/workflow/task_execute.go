package workflow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/repos/shared"
)

type taskExecutor struct {
	environment *Environment
	repository  *RepositoryState
	plan        taskPlan
}

func newTaskExecutor(environment *Environment, repository *RepositoryState, plan taskPlan) taskExecutor {
	return taskExecutor{environment: environment, repository: repository, plan: plan}
}

func (executor taskExecutor) Execute(executionContext context.Context) error {
	if executor.environment == nil {
		return nil
	}

	executor.plan.reportPlan(executor.environment)

	requireClean := executor.resolveEnsureClean()
	if executor.plan.skipped {
		executor.report(shared.EventCodeTaskSkip, shared.EventLevelInfo, "task has no changes", nil)
		return nil
	}

	hasFileChanges := hasApplicableChanges(executor.plan.fileChanges)
	hasActions := len(executor.plan.actions) > 0

	if requireClean {
		clean, cleanError := executor.environment.RepositoryManager.CheckCleanWorktree(executionContext, executor.repository.Path)
		if cleanError != nil {
			return cleanError
		}
		if !clean {
			fields := map[string]string{}
			statusEntries, statusError := executor.environment.RepositoryManager.WorktreeStatus(executionContext, executor.repository.Path)
			if statusError != nil {
				fields["status_error"] = statusError.Error()
			} else if len(statusEntries) > 0 {
				fields["status"] = strings.Join(statusEntries, ", ")
			}
			if len(fields) == 0 {
				fields = nil
			}
			executor.report(shared.EventCodeTaskSkip, shared.EventLevelWarn, "repository dirty", fields)
			return nil
		}
	}

	if hasFileChanges {
		startPoint := strings.TrimSpace(executor.plan.startPoint)
		if len(startPoint) > 0 {
			exists, existsError := executor.branchExists(executionContext, startPoint)
			if existsError != nil {
				return existsError
			}
			if !exists {
				executor.report(shared.EventCodeTaskSkip, shared.EventLevelWarn, "start point missing", map[string]string{"start_point": startPoint})
				executor.plan.startPoint = ""
			}
		}

		if branchExists, existsError := executor.branchExists(executionContext, executor.plan.branchName); existsError != nil {
			return existsError
		} else if branchExists {
			executor.report(shared.EventCodeTaskSkip, shared.EventLevelWarn, "branch exists", map[string]string{"branch": executor.plan.branchName})
			return nil
		}

		var branchError error
		originalBranch, branchError := executor.environment.RepositoryManager.GetCurrentBranch(executionContext, executor.repository.Path)
		if branchError != nil {
			return branchError
		}

		if branchToRestore := strings.TrimSpace(originalBranch); len(branchToRestore) > 0 {
			defer func(branch string) {
				_ = executor.checkoutBranch(executionContext, branch)
			}(branchToRestore)
		}

		if err := executor.checkoutBranch(executionContext, executor.plan.branchName); err != nil {
			startPoint := strings.TrimSpace(executor.plan.startPoint)
			if len(startPoint) > 0 {
				executor.report(shared.EventCodeTaskSkip, shared.EventLevelWarn, "start point missing", map[string]string{"start_point": startPoint})
				executor.plan.startPoint = ""
				if retryErr := executor.checkoutBranch(executionContext, executor.plan.branchName); retryErr != nil {
					return retryErr
				}
			} else {
				return err
			}
		}
	}

	if hasFileChanges {
		if err := executor.applyFileChanges(); err != nil {
			return err
		}
		if err := executor.stageChanges(executionContext); err != nil {
			return err
		}
		if err := executor.commitChanges(executionContext); err != nil {
			return err
		}
	}

	if hasFileChanges {
		if err := executor.pushBranch(executionContext); err != nil {
			return err
		}
	}

	if hasActions {
		if err := executor.executeActions(executionContext); err != nil {
			return err
		}
	}

	if executor.plan.pullRequest != nil {
		if err := executor.createPullRequest(executionContext); err != nil {
			return err
		}
	}

	fields := map[string]string{}
	if hasFileChanges {
		fields["branch"] = executor.plan.branchName
	}
	if hasActions {
		fields["actions"] = fmt.Sprintf("%d", len(executor.plan.actions))
	}
	if len(fields) == 0 {
		fields = nil
	}
	executor.report(shared.EventCodeTaskApply, shared.EventLevelInfo, "task applied", fields)
	return nil
}

func (executor taskExecutor) resolveEnsureClean() bool {
	defaultValue := executor.plan.task.EnsureClean
	variableName := strings.TrimSpace(executor.plan.task.EnsureCleanVariable)
	if len(variableName) == 0 || len(executor.plan.variables) == 0 {
		return defaultValue
	}

	rawValue, exists := executor.plan.variables[variableName]
	if !exists {
		return defaultValue
	}

	if parsedValue, parsed := parseEnsureCleanValue(rawValue); parsed {
		return parsedValue
	}

	return defaultValue
}

func parseEnsureCleanValue(raw string) (bool, bool) {
	normalized := strings.TrimSpace(strings.ToLower(raw))
	switch normalized {
	case "true", "1", "yes":
		return true, true
	case "false", "0", "no":
		return false, true
	default:
		return false, false
	}
}

func (executor taskExecutor) branchExists(executionContext context.Context, branchName string) (bool, error) {
	branchName = strings.TrimSpace(branchName)
	if len(branchName) == 0 {
		return false, nil
	}

	arguments := []string{"rev-parse", "--verify", branchName}
	_, err := executor.environment.GitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{Arguments: arguments, WorkingDirectory: executor.repository.Path})
	if err == nil {
		return true, nil
	}

	var commandErr execshell.CommandFailedError
	if errors.As(err, &commandErr) {
		return false, nil
	}

	if strings.Contains(err.Error(), "unknown revision") || strings.Contains(err.Error(), "Needed a single revision") {
		return false, nil
	}

	return false, err
}

func (executor taskExecutor) checkoutBranch(executionContext context.Context, branchName string) error {
	trimmedName := strings.TrimSpace(branchName)
	if len(trimmedName) == 0 {
		return nil
	}

	planBranch := strings.TrimSpace(executor.plan.branchName)
	startPoint := strings.TrimSpace(executor.plan.startPoint)

	arguments := []string{"checkout", trimmedName}
	if strings.EqualFold(trimmedName, planBranch) {
		arguments = []string{"checkout", "-B", planBranch}
		if len(startPoint) > 0 {
			arguments = append(arguments, startPoint)
		}
	}

	_, err := executor.environment.GitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{Arguments: arguments, WorkingDirectory: executor.repository.Path})
	return err
}

func (executor taskExecutor) applyFileChanges() error {
	for _, change := range executor.plan.fileChanges {
		if !change.apply {
			continue
		}

		directory := filepath.Dir(change.absolutePath)
		if err := executor.environment.FileSystem.MkdirAll(directory, 0o755); err != nil {
			return err
		}
		if err := executor.environment.FileSystem.WriteFile(change.absolutePath, change.content, change.permissions); err != nil {
			return err
		}
	}
	return nil
}

func (executor taskExecutor) stageChanges(executionContext context.Context) error {
	for _, change := range executor.plan.fileChanges {
		if !change.apply {
			continue
		}
		_, err := executor.environment.GitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{Arguments: []string{"add", change.relativePath}, WorkingDirectory: executor.repository.Path})
		if err != nil {
			return err
		}
	}
	return nil
}

func (executor taskExecutor) commitChanges(executionContext context.Context) error {
	arguments := []string{"commit", "-m", executor.plan.commitMessage}
	_, err := executor.environment.GitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{Arguments: arguments, WorkingDirectory: executor.repository.Path})
	return err
}

func (executor taskExecutor) pushBranch(executionContext context.Context) error {
	remote := strings.TrimSpace(executor.plan.task.Branch.PushRemote)
	if len(remote) == 0 {
		executor.report(shared.EventCodeTaskSkip, shared.EventLevelWarn, "push remote not configured (set task.branch.push_remote)", nil)
		return nil
	}

	if executor.environment != nil && executor.environment.RepositoryManager != nil {
		remoteURL, remoteError := executor.environment.RepositoryManager.GetRemoteURL(executionContext, executor.repository.Path, remote)
		if remoteError != nil {
			executor.report(
				shared.EventCodeTaskSkip,
				shared.EventLevelWarn,
				"remote lookup failed",
				map[string]string{"remote": remote, "error": remoteError.Error()},
			)
			return nil
		}
		if len(strings.TrimSpace(remoteURL)) == 0 {
			executor.report(
				shared.EventCodeTaskSkip,
				shared.EventLevelWarn,
				"remote missing",
				map[string]string{"remote": remote},
			)
			return nil
		}
	}

	arguments := []string{"push", "--set-upstream", remote, executor.plan.branchName}
	_, err := executor.environment.GitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{Arguments: arguments, WorkingDirectory: executor.repository.Path})
	return err
}

func (executor taskExecutor) createPullRequest(executionContext context.Context) error {
	pr := executor.plan.pullRequest
	if pr == nil {
		return nil
	}

	repository := executor.repository.Inspection.FinalOwnerRepo
	if len(strings.TrimSpace(repository)) == 0 {
		return errors.New("unable to determine repository owner/name for pull request")
	}

	options := githubPullRequestOptions{
		Repository: repository,
		Title:      pr.title,
		Body:       pr.body,
		Base:       pr.base,
		Head:       executor.plan.branchName,
		Draft:      pr.draft,
	}
	return createPullRequest(executionContext, executor.environment, options)
}

func (executor taskExecutor) executeActions(executionContext context.Context) error {
	actionExecutor := newTaskActionExecutor(executor.environment)
	for _, action := range executor.plan.actions {
		if err := actionExecutor.execute(executionContext, executor.repository, action); err != nil {
			return err
		}
	}
	return nil
}

func (executor taskExecutor) report(eventCode string, level shared.EventLevel, message string, fields map[string]string) {
	if executor.environment == nil {
		return
	}
	executor.environment.ReportRepositoryEvent(executor.repository, level, eventCode, message, fields)
}

type githubPullRequestOptions struct {
	Repository string
	Title      string
	Body       string
	Base       string
	Head       string
	Draft      bool
}

func createPullRequest(executionContext context.Context, environment *Environment, options githubPullRequestOptions) error {
	if environment == nil || environment.GitHubClient == nil {
		return errors.New("GitHub client not configured for task execution")
	}

	return environment.GitHubClient.CreatePullRequest(executionContext, githubcli.PullRequestCreateOptions{
		Repository: options.Repository,
		Title:      options.Title,
		Body:       options.Body,
		Base:       options.Base,
		Head:       options.Head,
		Draft:      options.Draft,
	})
}
