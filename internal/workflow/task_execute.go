package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/temirov/gix/internal/execshell"
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

	if executor.plan.skipped {
		executor.report(shared.EventCodeTaskSkip, shared.EventLevelInfo, "task has no changes", nil)
		return nil
	}

	execCtx := &ExecutionContext{
		Environment:  executor.environment,
		Repository:   executor.repository,
		Plan:         &executor.plan,
		requireClean: executor.resolveEnsureClean(),
	}

	hasFileChanges := hasApplicableChanges(executor.plan.fileChanges)
	if hasFileChanges && executor.environment.RepositoryManager != nil {
		originalBranch, branchError := executor.environment.RepositoryManager.GetCurrentBranch(executionContext, executor.repository.Path)
		if branchError != nil {
			return branchError
		}
		execCtx.setOriginalBranch(originalBranch)
		defer executor.restoreBranch(executionContext, execCtx)
	}

	for _, action := range executor.plan.workflowSteps {
		for _, guard := range action.Guards() {
			skipped, guardError := execCtx.handleActionError(action, guard.Check(executionContext, execCtx))
			if guardError != nil {
				return guardError
			}
			if skipped {
				return nil
			}
		}

		skipped, actionError := execCtx.handleActionError(action, action.Execute(executionContext, execCtx))
		if actionError != nil {
			return actionError
		}
		if skipped {
			return nil
		}
	}

	fields := map[string]string{}
	if execCtx.filesApplied {
		fields["branch"] = executor.plan.branchName
	}
	if execCtx.customActionsApplied > 0 {
		fields["actions"] = fmt.Sprintf("%d", execCtx.customActionsApplied)
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

func (executor taskExecutor) restoreBranch(executionContext context.Context, execCtx *ExecutionContext) {
	if execCtx == nil || !execCtx.branchWasPrepared() {
		return
	}
	branch := strings.TrimSpace(execCtx.originalBranch)
	if len(branch) == 0 || executor.environment == nil || executor.environment.GitExecutor == nil {
		return
	}
	_, _ = executor.environment.GitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{"checkout", branch},
		WorkingDirectory: executor.repository.Path,
	})
}

func (executor taskExecutor) report(eventCode string, level shared.EventLevel, message string, fields map[string]string) {
	if executor.environment == nil {
		return
	}
	executor.environment.ReportRepositoryEvent(executor.repository, level, eventCode, message, fields)
}
