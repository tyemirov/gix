package workflow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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

	hardSafeguards, softSafeguards := splitSafeguardSets(safeguards, safeguardDefaultSoftSkip)
	if len(hardSafeguards) > 0 {
		pass, reason, evalErr := EvaluateSafeguards(ctx, environment, repository, hardSafeguards)
		if evalErr != nil {
			return evalErr
		}
		if !pass {
			logNamespaceReason(environment, repository, shared.EventCodeNamespaceSkip, shared.EventLevelWarn, reason)
			return repositorySkipError{reason: reason}
		}
	}
	if len(softSafeguards) > 0 {
		pass, reason, evalErr := EvaluateSafeguards(ctx, environment, repository, softSafeguards)
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
	}

	if environment.PromptState != nil && !environment.PromptState.IsAssumeYesEnabled() {
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

	if result.Skipped {
		logNamespaceReason(environment, repository, shared.EventCodeNamespaceNoop, shared.EventLevelInfo, result.SkipReason)
		return nil
	}

	logNamespaceApply(environment, repository, result.BranchName, filesChanged, result.PushPerformed)
	recordNamespaceMutations(environment, repository, result)

	if environment != nil && environment.Variables != nil && result.PushPerformed {
		if variableName, ok := namespaceBranchVariableName(repository); ok && len(strings.TrimSpace(result.BranchName)) > 0 {
			environment.Variables.Set(variableName, strings.TrimSpace(result.BranchName))
		}
	}

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

func logNamespaceApply(environment *Environment, repository *RepositoryState, branch string, files int, pushed bool) {
	message := fmt.Sprintf("files=%d", files)
	if pushed {
		message = fmt.Sprintf("files=%d, pushed", files)
	}

	environment.ReportRepositoryEvent(
		repository,
		shared.EventLevelInfo,
		shared.EventCodeNamespaceApply,
		message,
		map[string]string{
			"branch":        branch,
			"files_changed": fmt.Sprintf("%d", files),
			"push":          fmt.Sprintf("%t", pushed),
		},
	)
}

func recordNamespaceMutations(environment *Environment, repository *RepositoryState, result namespace.Result) {
	if environment == nil || repository == nil {
		return
	}
	if !result.GoModChanged && len(result.ChangedFiles) == 0 {
		return
	}
	if result.GoModChanged {
		environment.RecordMutatedFile(repository, "go.mod")
	}
	for _, relativePath := range result.ChangedFiles {
		if len(strings.TrimSpace(relativePath)) == 0 {
			continue
		}
		environment.RecordMutatedFile(repository, strings.TrimSpace(relativePath))
	}
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

func namespaceBranchVariableName(repository *RepositoryState) (VariableName, bool) {
	if repository == nil {
		return "", false
	}
	owner, name := splitOwnerAndName(repository.Inspection.FinalOwnerRepo)
	if len(owner) == 0 && len(name) == 0 {
		owner, name = splitOwnerAndName(repository.Inspection.CanonicalOwnerRepo)
	}
	if len(owner) == 0 {
		owner = "repository"
	}
	if len(name) == 0 {
		name = strings.TrimSpace(repository.Inspection.DesiredFolderName)
	}
	if len(name) == 0 {
		name = strings.TrimSpace(repository.Inspection.FolderName)
	}
	if len(name) == 0 {
		name = filepath.Base(strings.TrimSpace(repository.Path))
	}
	if len(name) == 0 {
		name = "repository"
	}

	identifier := fmt.Sprintf(
		"namespace_branch_%s_%s",
		sanitizeVariableToken(owner),
		sanitizeVariableToken(name),
	)

	variableName, err := NewVariableName(identifier)
	if err != nil {
		return "", false
	}
	return variableName, true
}

func sanitizeVariableToken(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) == 0 {
		return "repository"
	}

	var builder strings.Builder
	for _, symbol := range trimmed {
		switch {
		case symbol >= 'a' && symbol <= 'z':
			builder.WriteRune(symbol)
		case symbol >= 'A' && symbol <= 'Z':
			builder.WriteRune(symbol)
		case symbol >= '0' && symbol <= '9':
			builder.WriteRune(symbol)
		case symbol == '-', symbol == '_', symbol == '.':
			builder.WriteRune(symbol)
		default:
			builder.WriteRune('_')
		}
	}

	result := strings.Trim(builder.String(), "_")
	if len(result) == 0 {
		return "repository"
	}
	return result
}
