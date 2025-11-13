package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/repos/shared"
)

type branchPrepareAction struct {
	branchName string
	startPoint string
}

func (action branchPrepareAction) Name() string {
	return "git.branch.prepare"
}

func (action branchPrepareAction) Guards() []actionGuard {
	return []actionGuard{newCleanWorktreeGuard(), newBranchAbsenceGuard(action.branchName)}
}

func (action branchPrepareAction) Execute(ctx context.Context, execCtx *ExecutionContext) error {
	planBranch := strings.TrimSpace(action.branchName)
	if len(planBranch) == 0 || execCtx == nil || execCtx.Environment == nil || execCtx.Environment.GitExecutor == nil {
		return errors.New("git executor not configured for branch preparation")
	}

	exists, existsErr := branchExists(ctx, execCtx.Environment.GitExecutor, execCtx.Repository.Path, planBranch)
	if existsErr != nil {
		return existsErr
	}
	if exists {
		return newActionSkipError("branch exists", map[string]string{"branch": planBranch})
	}

	arguments := []string{"checkout", "-B", planBranch}
	startPoint := strings.TrimSpace(action.startPoint)
	if len(startPoint) > 0 {
		exists, err := branchExists(ctx, execCtx.Environment.GitExecutor, execCtx.Repository.Path, startPoint)
		if err != nil {
			return err
		}
		if exists {
			arguments = append(arguments, startPoint)
		} else {
			execCtx.Environment.ReportRepositoryEvent(
				execCtx.Repository,
				shared.EventLevelWarn,
				shared.EventCodeTaskSkip,
				"start point missing",
				map[string]string{"start_point": startPoint},
			)
		}
	}

	_, err := execCtx.Environment.GitExecutor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        arguments,
		WorkingDirectory: execCtx.Repository.Path,
	})
	if err != nil {
		return err
	}
	execCtx.markBranchPrepared()
	return nil
}

type filesApplyAction struct {
	changes []taskFileChange
}

func (action filesApplyAction) Name() string {
	return "files.apply"
}

func (action filesApplyAction) Guards() []actionGuard {
	return []actionGuard{newCleanWorktreeGuard()}
}

func (action filesApplyAction) Execute(ctx context.Context, execCtx *ExecutionContext) error {
	if execCtx == nil || execCtx.Environment == nil || execCtx.Environment.FileSystem == nil {
		return errors.New("filesystem not configured for file application")
	}

	for _, change := range action.changes {
		if !change.apply {
			continue
		}
		if change.mode == taskFileModeAppendIfMissing {
			if err := applyAppendIfMissingChange(execCtx.Environment.FileSystem, change); err != nil {
				return err
			}
			continue
		}
		directory := filepath.Dir(change.absolutePath)
		if err := execCtx.Environment.FileSystem.MkdirAll(directory, 0o755); err != nil {
			return err
		}
		if err := execCtx.Environment.FileSystem.WriteFile(change.absolutePath, change.content, change.permissions); err != nil {
			return err
		}
	}

	execCtx.recordFilesApplied()
	return nil
}

type gitStageAction struct {
	changes []taskFileChange
}

func (action gitStageAction) Name() string {
	return "git.stage"
}

func (action gitStageAction) Guards() []actionGuard {
	return []actionGuard{newCleanWorktreeGuard()}
}

func (action gitStageAction) Execute(ctx context.Context, execCtx *ExecutionContext) error {
	if execCtx == nil || execCtx.Environment == nil || execCtx.Environment.GitExecutor == nil {
		return errors.New("git executor not configured for staging")
	}

	for _, change := range action.changes {
		if !change.apply {
			continue
		}
		_, err := execCtx.Environment.GitExecutor.ExecuteGit(ctx, execshell.CommandDetails{
			Arguments:        []string{"add", change.relativePath},
			WorkingDirectory: execCtx.Repository.Path,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

type gitCommitAction struct {
	message    string
	allowEmpty bool
}

func (action gitCommitAction) Name() string {
	return "git.commit"
}

func (action gitCommitAction) Guards() []actionGuard {
	return []actionGuard{newCleanWorktreeGuard()}
}

func (action gitCommitAction) Execute(ctx context.Context, execCtx *ExecutionContext) error {
	if execCtx == nil || execCtx.Environment == nil || execCtx.Environment.GitExecutor == nil {
		return errors.New("git executor not configured for commit")
	}
	if len(strings.TrimSpace(action.message)) == 0 {
		return errors.New("commit message not provided")
	}
	arguments := []string{"commit", "-m", action.message}
	if action.allowEmpty {
		arguments = append(arguments, "--allow-empty")
	}
	_, err := execCtx.Environment.GitExecutor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        arguments,
		WorkingDirectory: execCtx.Repository.Path,
	})
	return err
}

type gitPushAction struct {
	branch string
	remote string
}

func (action gitPushAction) Name() string {
	return "git.push"
}

type gitStageCommitAction struct {
	changes    []taskFileChange
	message    string
	allowEmpty bool
}

func (action gitStageCommitAction) Name() string {
	return "git.stage-commit"
}

func (action gitStageCommitAction) Guards() []actionGuard {
	return []actionGuard{newCleanWorktreeGuard()}
}

func (action gitStageCommitAction) Execute(ctx context.Context, execCtx *ExecutionContext) error {
	stageAction := gitStageAction{changes: action.changes}
	if err := stageAction.Execute(ctx, execCtx); err != nil {
		return err
	}
	commitAction := gitCommitAction{
		message:    action.message,
		allowEmpty: action.allowEmpty,
	}
	return commitAction.Execute(ctx, execCtx)
}

func (action gitPushAction) Guards() []actionGuard {
	return []actionGuard{newRemoteConfiguredGuard(action.remote)}
}

func (action gitPushAction) Execute(ctx context.Context, execCtx *ExecutionContext) error {
	if execCtx == nil || execCtx.Environment == nil || execCtx.Environment.GitExecutor == nil {
		return errors.New("git executor not configured for push")
	}
	arguments := []string{"push", "--set-upstream", strings.TrimSpace(action.remote), strings.TrimSpace(action.branch)}
	_, err := execCtx.Environment.GitExecutor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        arguments,
		WorkingDirectory: execCtx.Repository.Path,
	})
	return err
}

type pullRequestAction struct {
	title string
	body  string
	base  string
}

func (action pullRequestAction) Name() string {
	return "pull-request.create"
}

func (action pullRequestAction) Guards() []actionGuard {
	return nil
}

func (action pullRequestAction) Execute(ctx context.Context, execCtx *ExecutionContext) error {
	if execCtx == nil || execCtx.Environment == nil || execCtx.Environment.GitHubClient == nil {
		return errors.New("GitHub client not configured for pull request action")
	}

	repository := execCtx.Repository.Inspection.FinalOwnerRepo
	if len(strings.TrimSpace(repository)) == 0 {
		return errors.New("unable to determine repository owner/name for pull request")
	}

	options := githubcli.PullRequestCreateOptions{
		Repository: repository,
		Title:      action.title,
		Body:       action.body,
		Base:       action.base,
		Head:       execCtx.Plan.branchName,
		Draft:      execCtx.Plan.pullRequest.draft,
	}
	return execCtx.Environment.GitHubClient.CreatePullRequest(ctx, options)
}

type pullRequestOpenAction struct {
	branch string
	remote string
	title  string
	body   string
	base   string
	draft  bool
}

func (action pullRequestOpenAction) Name() string {
	return "pull-request.open"
}

func (action pullRequestOpenAction) Guards() []actionGuard {
	return []actionGuard{newRemoteConfiguredGuard(action.remote)}
}

func (action pullRequestOpenAction) Execute(ctx context.Context, execCtx *ExecutionContext) error {
	push := gitPushAction{branch: action.branch, remote: action.remote}
	if err := push.Execute(ctx, execCtx); err != nil {
		if skipErr, ok := err.(actionSkipError); ok {
			execCtx.reportSkip(skipErr.reason, skipErr.fields)
			return nil
		}
		return err
	}

	pr := pullRequestAction{
		title: action.title,
		body:  action.body,
		base:  action.base,
	}
	return pr.Execute(ctx, execCtx)
}

type customTaskAction struct {
	task taskAction
}

func (action customTaskAction) Name() string {
	return fmt.Sprintf("task.action.%s", action.task.actionType)
}

func (action customTaskAction) Guards() []actionGuard {
	return []actionGuard{newCleanWorktreeGuard()}
}

func (action customTaskAction) Execute(ctx context.Context, execCtx *ExecutionContext) error {
	if execCtx == nil {
		return nil
	}

	executor := newTaskActionExecutor(execCtx.Environment)
	if err := executor.execute(ctx, execCtx.Repository, action.task); err != nil {
		return err
	}
	execCtx.recordCustomAction()
	return nil
}

func applyAppendIfMissingChange(fileSystem shared.FileSystem, change taskFileChange) error {
	if fileSystem == nil {
		return errors.New("filesystem not configured for append-if-missing change")
	}

	directory := filepath.Dir(change.absolutePath)
	if err := fileSystem.MkdirAll(directory, 0o755); err != nil {
		return err
	}

	existingContent, readError := fileSystem.ReadFile(change.absolutePath)
	if readError != nil && !errors.Is(readError, fs.ErrNotExist) {
		return readError
	}

	desiredLines := parseEnsureLines(change.content)
	if len(desiredLines) == 0 {
		return nil
	}

	existingSet := buildEnsureLineSet(existingContent)
	buffer := bytes.NewBuffer(nil)
	if len(existingContent) > 0 {
		buffer.Write(existingContent)
	}

	appendNewline := func() {
		if buffer.Len() == 0 {
			return
		}
		if buffer.Bytes()[buffer.Len()-1] != '\n' {
			buffer.WriteByte('\n')
		}
	}

	added := false
	for _, line := range desiredLines {
		if _, exists := existingSet[line]; exists {
			continue
		}
		appendNewline()
		buffer.WriteString(line)
		buffer.WriteByte('\n')
		existingSet[line] = struct{}{}
		added = true
	}

	if !added && readError == nil {
		return nil
	}

	return fileSystem.WriteFile(change.absolutePath, buffer.Bytes(), change.permissions)
}
