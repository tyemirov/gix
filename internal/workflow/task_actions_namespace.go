package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	repoerrors "github.com/tyemirov/gix/internal/repos/errors"
	"github.com/tyemirov/gix/internal/repos/namespace"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	namespaceActionType             = "repo.namespace.rewrite"
	namespaceOldPrefixOptionKey     = "old"
	namespaceNewPrefixOptionKey     = "new"
	namespaceBranchPrefixOptionKey  = "branch_prefix"
	namespaceBranchPrefixLegacyKey  = "branch-prefix"
	namespaceCommitMessageOptionKey = "commit_message"
	namespaceCommitMessageFlagName  = "commit-message"
	namespacePushOptionKey          = "push"
	namespaceRemoteOptionKey        = "remote"
	namespaceSafeguardsOptionKey    = "safeguards"
	namespacePromptTemplate         = "Rewrite namespace %s -> %s in %s? [a/N/y] "
)

func handleNamespaceRewriteAction(ctx context.Context, environment *Environment, repository *RepositoryState, parameters map[string]any) error {
	if environment == nil || repository == nil {
		return nil
	}

	if environment.FileSystem == nil || environment.GitExecutor == nil || environment.RepositoryManager == nil {
		return fmt.Errorf("namespace rewrite action requires filesystem, git executor, and repository manager")
	}

	reader := newOptionReader(parameters)

	oldValue, oldExists, oldErr := reader.stringValue(namespaceOldPrefixOptionKey)
	if oldErr != nil {
		return oldErr
	}
	oldValue = strings.TrimSpace(oldValue)
	if !oldExists || len(oldValue) == 0 {
		return fmt.Errorf("namespace rewrite action requires 'old'")
	}

	newValue, newExists, newErr := reader.stringValue(namespaceNewPrefixOptionKey)
	if newErr != nil {
		return newErr
	}
	newValue = strings.TrimSpace(newValue)
	if !newExists || len(newValue) == 0 {
		return fmt.Errorf("namespace rewrite action requires 'new'")
	}

	oldPrefix, prefixErr := namespace.NewModulePrefix(oldValue)
	if prefixErr != nil {
		return prefixErr
	}
	newPrefix, newPrefixErr := namespace.NewModulePrefix(newValue)
	if newPrefixErr != nil {
		return newPrefixErr
	}

	branchPrefix, branchExists, branchErr := reader.stringValue(namespaceBranchPrefixOptionKey)
	if branchErr != nil {
		return branchErr
	}
	if !branchExists {
		if value, exists, legacyErr := reader.stringValue(namespaceBranchPrefixLegacyKey); legacyErr != nil {
			return legacyErr
		} else if exists {
			branchPrefix = strings.TrimSpace(value)
		} else if raw, ok := parameters[namespaceBranchPrefixLegacyKey]; ok {
			if stringValue, ok := raw.(string); ok {
				branchPrefix = strings.TrimSpace(stringValue)
			}
		}
	}

	push := true
	if value, exists, pushErr := reader.boolValue(namespacePushOptionKey); pushErr != nil {
		return pushErr
	} else if exists {
		push = value
	}

	remote := ""
	if value, exists, remoteErr := reader.stringValue(namespaceRemoteOptionKey); remoteErr != nil {
		return remoteErr
	} else if exists {
		remote = strings.TrimSpace(value)
	}

	commitMessage, commitExists, commitErr := reader.stringValue(namespaceCommitMessageOptionKey)
	if commitErr != nil {
		return commitErr
	}
	if !commitExists {
		if value, exists, legacyErr := reader.stringValue(namespaceCommitMessageFlagName); legacyErr != nil {
			return legacyErr
		} else if exists {
			commitMessage = strings.TrimSpace(value)
		} else if raw, ok := parameters[namespaceCommitMessageFlagName]; ok {
			if stringValue, ok := raw.(string); ok {
				commitMessage = strings.TrimSpace(stringValue)
			}
		}
	}

	repositoryPath, repoPathErr := shared.NewRepositoryPath(repository.Path)
	if repoPathErr != nil {
		return repoPathErr
	}

	safeguards, _, safeguardsErr := reader.mapValue(namespaceSafeguardsOptionKey)
	if safeguardsErr != nil {
		return safeguardsErr
	}

	if len(safeguards) > 0 {
		pass, reason, evalErr := EvaluateSafeguards(ctx, environment, repository, safeguards)
		if evalErr != nil {
			return evalErr
		}
		if !pass {
			logNamespaceReason(environment, repository, shared.EventCodeNamespaceSkip, shared.EventLevelWarn, reason)
			return nil
		}
	}

	service, serviceErr := namespace.NewService(namespace.Dependencies{
		FileSystem:        environment.FileSystem,
		GitExecutor:       environment.GitExecutor,
		RepositoryManager: environment.RepositoryManager,
	})
	if serviceErr != nil {
		return serviceErr
	}

	options := namespace.Options{
		RepositoryPath: repositoryPath,
		OldPrefix:      oldPrefix,
		NewPrefix:      newPrefix,
		BranchPrefix:   branchPrefix,
		CommitMessage:  commitMessage,
		Push:           push,
		PushRemote:     remote,
		DryRun:         environment.DryRun,
	}

	if !environment.DryRun && environment.PromptState != nil && !environment.PromptState.IsAssumeYesEnabled() {
		if environment.Prompter != nil {
			prompt := fmt.Sprintf(namespacePromptTemplate, oldPrefix.String(), newPrefix.String(), repository.Path)
			confirmation, confirmErr := environment.Prompter.Confirm(prompt)
			if confirmErr != nil {
				return confirmErr
			}
			if !confirmation.Confirmed {
				logNamespaceReason(environment, repository, shared.EventCodeNamespaceSkip, shared.EventLevelWarn, "user declined")
				return nil
			}
		}
	}

	result, rewriteErr := service.Rewrite(ctx, options)
	if rewriteErr != nil {
		var operationError repoerrors.OperationError
		if errors.As(rewriteErr, &operationError) {
			logRepositoryOperationError(environment, rewriteErr)
			if operationError.Code() != string(repoerrors.ErrNamespacePushFailed) && operationError.Code() != string(repoerrors.ErrRemoteMissing) {
				return rewriteErr
			}
		} else {
			return rewriteErr
		}
	}

	filesChanged := len(result.ChangedFiles)
	if result.GoModChanged {
		filesChanged++
	}

	if environment.DryRun {
		logNamespaceApply(environment, repository, result.BranchName, filesChanged, options.Push, true)
		return nil
	}

	if result.Skipped {
		logNamespaceReason(environment, repository, shared.EventCodeNamespaceNoop, shared.EventLevelInfo, result.SkipReason)
		return nil
	}

	logNamespaceApply(environment, repository, result.BranchName, filesChanged, result.PushPerformed, false)

	if len(strings.TrimSpace(result.PushSkippedReason)) > 0 {
		logNamespaceReason(environment, repository, shared.EventCodeNamespaceSkip, shared.EventLevelWarn, result.PushSkippedReason)
	}

	if environment.AuditService != nil {
		if refreshErr := repository.Refresh(ctx, environment.AuditService); refreshErr != nil {
			return refreshErr
		}
	}

	return nil
}

func logNamespaceApply(environment *Environment, repository *RepositoryState, branch string, files int, pushed bool, plan bool) {
	message := fmt.Sprintf("files=%d", files)
	if pushed {
		if plan {
			message = fmt.Sprintf("files=%d, push scheduled", files)
		} else {
			message = fmt.Sprintf("files=%d, pushed", files)
		}
	}

	code := shared.EventCodeNamespaceApply
	if plan {
		code = shared.EventCodeNamespacePlan
	}

	environment.ReportRepositoryEvent(
		repository,
		shared.EventLevelInfo,
		code,
		message,
		map[string]string{
			"branch":        branch,
			"files_changed": fmt.Sprintf("%d", files),
			"push":          fmt.Sprintf("%t", pushed),
		},
	)
}

func logNamespaceReason(environment *Environment, repository *RepositoryState, code string, level shared.EventLevel, reason string) {
	environment.ReportRepositoryEvent(
		repository,
		level,
		code,
		strings.TrimSpace(reason),
		map[string]string{"reason": sanitizeReason(reason)},
	)
}

func sanitizeReason(reason string) string {
	return strings.ReplaceAll(strings.TrimSpace(reason), " ", "_")
}
