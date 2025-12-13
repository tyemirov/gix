package workflow

import "strings"

// LogPhase enumerates the human-readable workflow logging phases.
type LogPhase string

// Supported logging phases.
const (
	LogPhaseUnknown      LogPhase = ""
	LogPhaseSteps        LogPhase = "steps"
	LogPhaseRemoteFolder LogPhase = "remote_folder"
	LogPhaseBranch       LogPhase = "branch"
	LogPhaseFiles        LogPhase = "files"
	LogPhaseGit          LogPhase = "git"
	LogPhasePullRequest  LogPhase = "pull_request"
)

func (plan taskPlan) loggingPhase() LogPhase {
	if phase := detectPhaseFromActions(plan.actions); phase != LogPhaseUnknown {
		return phase
	}
	if phase := detectPhaseFromWorkflowSteps(plan.workflowSteps); phase != LogPhaseUnknown {
		return phase
	}
	if len(plan.fileChanges) > 0 {
		return LogPhaseFiles
	}
	return LogPhaseFiles
}

func detectPhaseFromActions(actions []taskAction) LogPhase {
	for _, action := range actions {
		if phase := phaseForActionType(action.actionType); phase != LogPhaseUnknown {
			return phase
		}
	}
	return LogPhaseUnknown
}

func detectPhaseFromWorkflowSteps(steps []workflowAction) LogPhase {
	hasFiles := false
	hasGit := false
	hasPullRequest := false
	hasBranch := false

	for _, step := range steps {
		switch typed := step.(type) {
		case filesApplyAction:
			hasFiles = true
		case gitStageAction, gitCommitAction, gitStageCommitAction, gitPushAction:
			hasGit = true
		case pullRequestAction, pullRequestOpenAction:
			hasPullRequest = true
		case branchPrepareAction:
			hasBranch = true
		case customTaskAction:
			if phase := phaseForActionType(typed.task.actionType); phase != LogPhaseUnknown {
				return phase
			}
		}
	}

	switch {
	case hasFiles:
		return LogPhaseFiles
	case hasGit:
		return LogPhaseGit
	case hasPullRequest:
		return LogPhasePullRequest
	case hasBranch:
		return LogPhaseBranch
	default:
		return LogPhaseUnknown
	}
}

func phaseForActionType(actionType string) LogPhase {
	normalized := strings.ToLower(strings.TrimSpace(actionType))
	switch normalized {
	case "branch.change", "branch.default":
		return LogPhaseBranch
	case taskActionCanonicalRemote, taskActionProtocolConversion, taskActionRenameDirectories:
		return LogPhaseRemoteFolder
	case taskActionNamespaceRewrite, taskActionFileReplace, taskActionAuditReport:
		return LogPhaseFiles
	case taskActionHistoryPurge, taskActionReleaseTag, taskActionReleaseRetag:
		return LogPhaseGit
	default:
		return LogPhaseUnknown
	}
}
