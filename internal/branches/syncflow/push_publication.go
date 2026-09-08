package syncflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/workflow"
)

const syncPushRejectedMessage = "local synchronization completed on %q; the remote rejected the push and did not receive the changes: %s. To publish the commits, remove branch protection or open a new pull request"
const syncAdditionalRefRejectedMessage = "the remote has the changes for branch %q; the push rejected another ref: %s. Resolve the rejected ref and push it again"

// publishStrictSyncBranch distinguishes a remote refusal from an execution failure.
// A refusal completes local sync, but must not trigger pull-request creation.
func publishStrictSyncBranch(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, remoteName, branchName string, arguments []string) (bool, error) {
	pushErr := executeGit(ctx, environment.GitExecutor, repository.Path, arguments)
	if pushErr == nil {
		return true, nil
	}
	var failure execshell.CommandFailedError
	if !errors.As(pushErr, &failure) || !strictSyncRemoteRejectedPush(failure.Result) {
		return false, pushErr
	}
	status := strictSyncRequestedBranchPushStatus(failure.Result, branchName)
	if status == strictSyncPushUnknown {
		return false, pushErr
	}
	if status == strictSyncPushAccepted {
		environment.ReportRepositoryEvent(repository, shared.EventLevelWarn, shared.EventCodeSyncPushRejected,
			fmt.Sprintf(syncAdditionalRefRejectedMessage, branchName, strings.TrimSpace(failure.Result.StandardError+"\n"+failure.Result.StandardOutput)),
			map[string]string{"branch": branchName, "remote": remoteName, "published": "true"})
		return true, nil
	}
	environment.ReportRepositoryEvent(repository, shared.EventLevelWarn, shared.EventCodeSyncPushRejected,
		fmt.Sprintf(syncPushRejectedMessage, branchName, strings.TrimSpace(failure.Result.StandardError+"\n"+failure.Result.StandardOutput)),
		map[string]string{"branch": branchName, "remote": remoteName, "published": "false"})
	return false, nil
}

type strictSyncPushStatus uint8

const (
	strictSyncPushUnknown strictSyncPushStatus = iota
	strictSyncPushAccepted
	strictSyncPushRejected
)

func strictSyncRequestedBranchPushStatus(result execshell.ExecutionResult, branchName string) strictSyncPushStatus {
	observedStatus := false
	for _, line := range strings.Split(result.StandardOutput, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 || len(fields[0]) != 1 {
			continue
		}
		observedStatus = true
		_, destination, _ := strings.Cut(fields[1], ":")
		if destination != gitBranchReferencePrefix+branchName {
			continue
		}
		switch fields[0] {
		case " ", "+", "*", "=":
			return strictSyncPushAccepted
		case "!":
			return strictSyncPushRejected
		}
	}
	// A server can reject the operation before Git reports any ref status.
	if !observedStatus && strictSyncRemoteRejectedPush(result) {
		return strictSyncPushRejected
	}
	return strictSyncPushUnknown
}

func strictSyncRemoteRejectedPush(result execshell.ExecutionResult) bool {
	for _, line := range strings.Split(result.StandardOutput, "\n") {
		if strings.HasPrefix(line, "!\t") && (strings.Contains(line, "[remote rejected]") || strings.Contains(line, "[rejected]")) {
			return true
		}
	}
	for _, line := range strings.Split(result.StandardError, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "remote:"))
		line = strings.TrimPrefix(line, "error: ")
		if strings.HasPrefix(line, "GH006:") || strings.HasPrefix(line, "GH013:") {
			return true
		}
	}
	return false
}

const syncPublicationDeferredMessage = "saved file work on %q; its push and pull request are deferred because parent %q could not be published; rerun sync to publish the parent and child"

func syncDeferredChildLocally(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, remoteName, branchName, parentBranch string, commitMessages worktreeAdoptionCommitMessageOptions) error {
	remoteExists, remoteErr := remoteReferenceExists(ctx, environment.GitExecutor, repository.Path, fmt.Sprintf("%s/%s", remoteName, branchName))
	if remoteErr != nil {
		return remoteErr
	}
	if remoteExists {
		if mergeErr := mergeRemoteBranchIntoLocal(ctx, environment, repository, environment.GitExecutor, repository.Path, remoteName, branchName, commitMessages); mergeErr != nil {
			return mergeErr
		}
	}
	if mergeErr := mergeReferenceIntoBranch(ctx, environment, repository, environment.GitExecutor, repository.Path, gitBranchReferencePrefix+parentBranch, branchName, commitMessages); mergeErr != nil {
		return mergeErr
	}
	environment.ReportRepositoryEvent(repository, shared.EventLevelWarn, shared.EventCodeSyncPublicationDeferred,
		fmt.Sprintf(syncPublicationDeferredMessage, branchName, parentBranch), map[string]string{"branch": branchName, "parent": parentBranch, "published": "false"})
	return nil
}
