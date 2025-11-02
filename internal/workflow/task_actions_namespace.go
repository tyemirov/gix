package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/temirov/gix/internal/repos/namespace"
	"github.com/temirov/gix/internal/repos/shared"
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
	namespacePlanMessageTemplate    = "NAMESPACE-PLAN: %s branch=%s files=%d push=%t\\n"
	namespaceApplyMessageTemplate   = "NAMESPACE-APPLY: %s branch=%s files=%d push=%t\\n"
	namespaceNoopMessageTemplate    = "NAMESPACE-NOOP: %s reason=%s\\n"
	namespaceSkipMessageTemplate    = "NAMESPACE-SKIP: %s reason=%s\\n"
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
			writeNamespaceReason(environment, namespaceSkipMessageTemplate, repository.Path, reason)
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
				writeNamespaceReason(environment, namespaceSkipMessageTemplate, repository.Path, "user declined")
				return nil
			}
		}
	}

	result, rewriteErr := service.Rewrite(ctx, options)
	if rewriteErr != nil {
		return rewriteErr
	}

	filesChanged := len(result.ChangedFiles)
	if result.GoModChanged {
		filesChanged++
	}

	if environment.DryRun {
		writeNamespaceApply(environment, namespacePlanMessageTemplate, repository.Path, result.BranchName, filesChanged, options.Push)
		return nil
	}

	if result.Skipped {
		writeNamespaceReason(environment, namespaceNoopMessageTemplate, repository.Path, result.SkipReason)
		return nil
	}

	writeNamespaceApply(environment, namespaceApplyMessageTemplate, repository.Path, result.BranchName, filesChanged, result.PushPerformed)

	if environment.AuditService != nil {
		if refreshErr := repository.Refresh(ctx, environment.AuditService); refreshErr != nil {
			return refreshErr
		}
	}

	return nil
}

func writeNamespaceApply(environment *Environment, template string, repositoryPath string, branch string, files int, pushed bool) {
	if environment == nil || environment.Output == nil {
		return
	}
	fmt.Fprintf(environment.Output, template, repositoryPath, branch, files, pushed)
}

func writeNamespaceReason(environment *Environment, template string, repositoryPath string, reason string) {
	if environment == nil || environment.Output == nil {
		return
	}
	fmt.Fprintf(environment.Output, template, repositoryPath, reason)
}
