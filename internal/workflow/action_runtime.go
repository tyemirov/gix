package workflow

import (
	"context"
	"fmt"

	"github.com/temirov/gix/internal/repos/shared"
)

type workflowAction interface {
	Name() string
	Guards() []actionGuard
	Execute(ctx context.Context, execCtx *ExecutionContext) error
}

type actionGuard interface {
	Check(ctx context.Context, execCtx *ExecutionContext) error
}

type guardFunc struct {
	run func(ctx context.Context, execCtx *ExecutionContext) error
}

func (guard guardFunc) Check(ctx context.Context, execCtx *ExecutionContext) error {
	if guard.run == nil {
		return nil
	}
	return guard.run(ctx, execCtx)
}

func newGuard(run func(ctx context.Context, execCtx *ExecutionContext) error) actionGuard {
	return guardFunc{run: run}
}

type actionSkipError struct {
	reason string
	fields map[string]string
}

func (skipErr actionSkipError) Error() string {
	return skipErr.reason
}

type ExecutionContext struct {
	Environment *Environment
	Repository  *RepositoryState
	Plan        *taskPlan

	requireClean         bool
	worktreeChecked      bool
	worktreeClean        bool
	worktreeStatus       []string
	filesApplied         bool
	customActionsApplied int
	originalBranch       string
	branchPrepared       bool
}

func (execCtx *ExecutionContext) recordFilesApplied() {
	if execCtx != nil {
		execCtx.filesApplied = true
	}
}

func (execCtx *ExecutionContext) recordCustomAction() {
	if execCtx != nil {
		execCtx.customActionsApplied++
	}
}

func (execCtx *ExecutionContext) reportSkip(message string, fields map[string]string) {
	if execCtx == nil || execCtx.Environment == nil {
		return
	}
	execCtx.Environment.ReportRepositoryEvent(execCtx.Repository, shared.EventLevelWarn, shared.EventCodeTaskSkip, message, fields)
}

func (execCtx *ExecutionContext) setOriginalBranch(name string) {
	if execCtx == nil {
		return
	}
	execCtx.originalBranch = name
}

func (execCtx *ExecutionContext) markBranchPrepared() {
	if execCtx == nil {
		return
	}
	execCtx.branchPrepared = true
}

func (execCtx *ExecutionContext) branchWasPrepared() bool {
	return execCtx != nil && execCtx.branchPrepared
}

func (execCtx *ExecutionContext) requireCleanWorktree() bool {
	return execCtx != nil && execCtx.requireClean
}

func (execCtx *ExecutionContext) storeWorktreeCheck(clean bool, status []string) {
	if execCtx == nil {
		return
	}
	execCtx.worktreeChecked = true
	execCtx.worktreeClean = clean
	execCtx.worktreeStatus = status
}

func (execCtx *ExecutionContext) knownWorktreeState() (bool, []string, bool) {
	if execCtx == nil || !execCtx.worktreeChecked {
		return false, nil, false
	}
	return execCtx.worktreeClean, execCtx.worktreeStatus, true
}

func newActionSkipError(reason string, fields map[string]string) error {
	return actionSkipError{reason: reason, fields: fields}
}

func wrapFields(fields map[string]string) map[string]string {
	if len(fields) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(fields))
	for key, value := range fields {
		cloned[key] = value
	}
	return cloned
}

func (execCtx *ExecutionContext) handleActionError(action workflowAction, err error) (bool, error) {
	if err == nil {
		return false, nil
	}

	if skipErr, ok := err.(actionSkipError); ok {
		execCtx.reportSkip(skipErr.reason, wrapFields(skipErr.fields))
		return true, nil
	}

	return false, fmt.Errorf("%s action failed: %w", action.Name(), err)
}
