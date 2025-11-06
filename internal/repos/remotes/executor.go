package remotes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	repoerrors "github.com/temirov/gix/internal/repos/errors"
	"github.com/temirov/gix/internal/repos/shared"
)

const (
	promptTemplate                   = "Update 'origin' in '%s' to canonical (%s → %s)? [a/N/y] "
	ownerRepoNotDetectedErrorMessage = "owner repository not detected"
	unknownProtocolErrorTemplate     = "unknown protocol %s"
	gitProtocolURLTemplate           = "git@github.com:%s.git"
	sshProtocolURLTemplate           = "ssh://git@github.com/%s.git"
	httpsProtocolURLTemplate         = "https://github.com/%s.git"
	declinedReason                   = "user declined"
)

// Options configures the remote update workflow.
type Options struct {
	RepositoryPath           shared.RepositoryPath
	CurrentOriginURL         *shared.RemoteURL
	OriginOwnerRepository    *shared.OwnerRepository
	CanonicalOwnerRepository *shared.OwnerRepository
	RemoteProtocol           shared.RemoteProtocol
	ConfirmationPolicy       shared.ConfirmationPolicy
	OwnerConstraint          *shared.OwnerSlug
}

// Dependencies captures collaborators required to update remotes.
type Dependencies struct {
	GitManager shared.GitRepositoryManager
	Prompter   shared.ConfirmationPrompter
	Reporter   shared.Reporter
}

// Executor orchestrates canonical remote updates.
type Executor struct {
	dependencies Dependencies
}

// NewExecutor constructs an Executor from the provided dependencies.
func NewExecutor(dependencies Dependencies) *Executor {
	return &Executor{dependencies: dependencies}
}

// Execute performs the remote update according to the provided options.
func (executor *Executor) Execute(executionContext context.Context, options Options) error {
	repositoryPath := options.RepositoryPath.String()

	if options.OriginOwnerRepository == nil {
		executor.report(shared.EventLevelWarn, shared.EventCodeRemoteSkip, repositoryPath, nil, "missing origin owner", map[string]string{"reason": "origin_owner_missing"})
		return repoerrors.WrapMessage(
			repoerrors.OperationCanonicalRemote,
			repositoryPath,
			repoerrors.ErrOriginOwnerMissing,
			"missing origin owner repository",
		)
	}

	if options.CanonicalOwnerRepository == nil {
		executor.report(shared.EventLevelWarn, shared.EventCodeRemoteSkip, repositoryPath, options.OriginOwnerRepository, "canonical redirect unavailable", map[string]string{"reason": "canonical_missing"})
		return repoerrors.WrapMessage(
			repoerrors.OperationCanonicalRemote,
			repositoryPath,
			repoerrors.ErrCanonicalOwnerMissing,
			"canonical owner repository not provided",
		)
	}

	originOwner := options.OriginOwnerRepository.String()
	canonicalOwner := options.CanonicalOwnerRepository.String()

	if strings.EqualFold(originOwner, canonicalOwner) {
		executor.report(shared.EventLevelInfo, shared.EventCodeRemoteSkip, repositoryPath, options.OriginOwnerRepository, "already canonical", map[string]string{"reason": "already_canonical"})
		return nil
	}

	targetURL, targetError := BuildRemoteURL(options.RemoteProtocol, canonicalOwner)
	if targetError != nil {
		executor.report(shared.EventLevelWarn, shared.EventCodeRemoteSkip, repositoryPath, options.OriginOwnerRepository, "failed to build target URL", map[string]string{"reason": "target_url_unavailable", "protocol": string(options.RemoteProtocol)})
		return repoerrors.WrapMessage(
			repoerrors.OperationCanonicalRemote,
			repositoryPath,
			repoerrors.ErrRemoteURLBuildFailed,
			"failed to build target URL",
		)
	}

	if options.ConfirmationPolicy.ShouldPrompt() && executor.dependencies.Prompter != nil {
		prompt := fmt.Sprintf(promptTemplate, repositoryPath, originOwner, canonicalOwner)
		confirmationResult, promptError := executor.dependencies.Prompter.Confirm(prompt)
		if promptError != nil {
			executor.report(shared.EventLevelWarn, shared.EventCodeRemoteSkip, repositoryPath, options.CanonicalOwnerRepository, "confirmation failed", map[string]string{
				"reason":  "confirmation_error",
				"current": originOwner,
				"target":  canonicalOwner,
			})
			return repoerrors.WrapMessage(
				repoerrors.OperationCanonicalRemote,
				repositoryPath,
				repoerrors.ErrUserConfirmationFailed,
				"confirmation failed",
			)
		}
		if !confirmationResult.Confirmed {
			executor.report(shared.EventLevelWarn, shared.EventCodeRemoteDeclined, repositoryPath, options.CanonicalOwnerRepository, declinedReason, map[string]string{
				"reason":  "user_declined",
				"current": originOwner,
				"target":  canonicalOwner,
			})
			return nil
		}
	}

	if executor.dependencies.GitManager == nil {
		executor.report(shared.EventLevelWarn, shared.EventCodeRemoteSkip, repositoryPath, options.CanonicalOwnerRepository, "git manager unavailable", map[string]string{"reason": "git_manager_unavailable"})
		return repoerrors.WrapMessage(
			repoerrors.OperationCanonicalRemote,
			repositoryPath,
			repoerrors.ErrGitManagerUnavailable,
			"git manager unavailable",
		)
	}

	updateError := executor.dependencies.GitManager.SetRemoteURL(executionContext, repositoryPath, shared.OriginRemoteNameConstant, targetURL)
	if updateError != nil {
		executor.report(shared.EventLevelWarn, shared.EventCodeRemoteSkip, repositoryPath, options.CanonicalOwnerRepository, "failed to update origin", map[string]string{
			"reason":     "update_failed",
			"target_url": FormatRemoteURLForDisplay(targetURL),
		})
		return repoerrors.WrapMessage(
			repoerrors.OperationCanonicalRemote,
			repositoryPath,
			repoerrors.ErrRemoteUpdateFailed,
			"failed to update origin URL",
		)
	}

	executor.report(shared.EventLevelInfo, shared.EventCodeRemoteUpdate, repositoryPath, options.CanonicalOwnerRepository, fmt.Sprintf("origin now %s", FormatRemoteURLForDisplay(targetURL)), map[string]string{
		"target_url": FormatRemoteURLForDisplay(targetURL),
		"protocol":   string(options.RemoteProtocol),
	})
	return nil
}

// Execute performs the remote update workflow using transient executor state.
func Execute(executionContext context.Context, dependencies Dependencies, options Options) error {
	return NewExecutor(dependencies).Execute(executionContext, options)
}

func (executor *Executor) report(level shared.EventLevel, code string, repositoryPath string, ownerRepository *shared.OwnerRepository, message string, details map[string]string) {
	if executor.dependencies.Reporter == nil {
		return
	}
	repositoryIdentifier := ""
	if ownerRepository != nil {
		repositoryIdentifier = ownerRepository.String()
	}

	metadata := make(map[string]string, len(details))
	for key, value := range details {
		metadata[key] = value
	}

	if _, exists := metadata["reason"]; !exists && code == shared.EventCodeRemoteSkip {
		metadata["reason"] = "skip"
	}

	if code == shared.EventCodeRemotePlan {
		if _, exists := metadata["reason"]; !exists {
			metadata["reason"] = "plan"
		}
	}

	executor.dependencies.Reporter.Report(shared.Event{
		Level:                level,
		Code:                 code,
		RepositoryIdentifier: repositoryIdentifier,
		RepositoryPath:       repositoryPath,
		Message:              message,
		Details:              metadata,
	})
}

// BuildRemoteURL formats the canonical remote URL for the provided protocol and owner/repository tuple.
func BuildRemoteURL(protocol shared.RemoteProtocol, ownerRepo string) (string, error) {
	trimmedOwnerRepo := strings.TrimSpace(ownerRepo)
	if len(trimmedOwnerRepo) == 0 {
		return "", errors.New(ownerRepoNotDetectedErrorMessage)
	}

	switch protocol {
	case shared.RemoteProtocolGit:
		return fmt.Sprintf(gitProtocolURLTemplate, trimmedOwnerRepo), nil
	case shared.RemoteProtocolSSH:
		return fmt.Sprintf(sshProtocolURLTemplate, trimmedOwnerRepo), nil
	case shared.RemoteProtocolHTTPS:
		return fmt.Sprintf(httpsProtocolURLTemplate, trimmedOwnerRepo), nil
	default:
		return "", fmt.Errorf(unknownProtocolErrorTemplate, protocol)
	}
}

// FormatRemoteURLForDisplay normalizes remote URLs to the form presented by `git remote -v`.
func FormatRemoteURLForDisplay(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) == 0 {
		return trimmed
	}
	if strings.HasPrefix(trimmed, shared.SSHProtocolURLPrefixConstant) {
		suffix := strings.TrimPrefix(trimmed, shared.SSHProtocolURLPrefixConstant)
		return shared.GitProtocolURLPrefixConstant + suffix
	}
	return trimmed
}
