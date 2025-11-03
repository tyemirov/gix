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
	skipParseMessage                 = "UPDATE-REMOTE-SKIP: %s (error: could not parse origin owner/repo)\n"
	skipCanonicalMessage             = "UPDATE-REMOTE-SKIP: %s (no upstream: no canonical redirect found)\n"
	skipSameMessage                  = "UPDATE-REMOTE-SKIP: %s (already canonical)\n"
	skipTargetMessage                = "UPDATE-REMOTE-SKIP: %s (error: could not construct target URL)\n"
	planMessage                      = "PLAN-UPDATE-REMOTE: %s origin %s → %s\n"
	promptTemplate                   = "Update 'origin' in '%s' to canonical (%s → %s)? [a/N/y] "
	declinedMessage                  = "UPDATE-REMOTE-SKIP: user declined for %s\n"
	successMessage                   = "UPDATE-REMOTE-DONE: %s origin now %s\n"
	failureMessage                   = "UPDATE-REMOTE-SKIP: %s (error: failed to set origin URL)\n"
	ownerRepoNotDetectedErrorMessage = "owner repository not detected"
	unknownProtocolErrorTemplate     = "unknown protocol %s"
	gitProtocolURLTemplate           = "git@github.com:%s.git"
	sshProtocolURLTemplate           = "ssh://git@github.com/%s.git"
	httpsProtocolURLTemplate         = "https://github.com/%s.git"
)

// Options configures the remote update workflow.
type Options struct {
	RepositoryPath           shared.RepositoryPath
	CurrentOriginURL         *shared.RemoteURL
	OriginOwnerRepository    *shared.OwnerRepository
	CanonicalOwnerRepository *shared.OwnerRepository
	RemoteProtocol           shared.RemoteProtocol
	DryRun                   bool
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
		executor.printfOutput(skipParseMessage, repositoryPath)
		return repoerrors.WrapMessage(
			repoerrors.OperationCanonicalRemote,
			repositoryPath,
			repoerrors.ErrOriginOwnerMissing,
			fmt.Sprintf(skipParseMessage, repositoryPath),
		)
	}

	if options.CanonicalOwnerRepository == nil {
		executor.printfOutput(skipCanonicalMessage, repositoryPath)
		return repoerrors.WrapMessage(
			repoerrors.OperationCanonicalRemote,
			repositoryPath,
			repoerrors.ErrCanonicalOwnerMissing,
			fmt.Sprintf(skipCanonicalMessage, repositoryPath),
		)
	}

	originOwner := options.OriginOwnerRepository.String()
	canonicalOwner := options.CanonicalOwnerRepository.String()

	if strings.EqualFold(originOwner, canonicalOwner) {
		executor.printfOutput(skipSameMessage, repositoryPath)
		return nil
	}

	targetURL, targetError := BuildRemoteURL(options.RemoteProtocol, canonicalOwner)
	if targetError != nil {
		executor.printfOutput(skipTargetMessage, repositoryPath)
		return repoerrors.WrapMessage(
			repoerrors.OperationCanonicalRemote,
			repositoryPath,
			repoerrors.ErrRemoteURLBuildFailed,
			fmt.Sprintf(skipTargetMessage, repositoryPath),
		)
	}

	currentOriginURL := ""
	if options.CurrentOriginURL != nil {
		currentOriginURL = options.CurrentOriginURL.String()
	}

	if options.DryRun {
		displayCurrent := FormatRemoteURLForDisplay(currentOriginURL)
		displayTarget := FormatRemoteURLForDisplay(targetURL)
		executor.printfOutput(planMessage, repositoryPath, displayCurrent, displayTarget)
		return nil
	}

	if options.ConfirmationPolicy.ShouldPrompt() && executor.dependencies.Prompter != nil {
		prompt := fmt.Sprintf(promptTemplate, repositoryPath, originOwner, canonicalOwner)
		confirmationResult, promptError := executor.dependencies.Prompter.Confirm(prompt)
		if promptError != nil {
			executor.printfOutput(skipTargetMessage, repositoryPath)
			return repoerrors.WrapMessage(
				repoerrors.OperationCanonicalRemote,
				repositoryPath,
				repoerrors.ErrUserConfirmationFailed,
				fmt.Sprintf(skipTargetMessage, repositoryPath),
			)
		}
		if !confirmationResult.Confirmed {
			executor.printfOutput(declinedMessage, repositoryPath)
			return nil
		}
	}

	if executor.dependencies.GitManager == nil {
		executor.printfOutput(failureMessage, repositoryPath)
		return repoerrors.WrapMessage(
			repoerrors.OperationCanonicalRemote,
			repositoryPath,
			repoerrors.ErrGitManagerUnavailable,
			fmt.Sprintf(failureMessage, repositoryPath),
		)
	}

	updateError := executor.dependencies.GitManager.SetRemoteURL(executionContext, repositoryPath, shared.OriginRemoteNameConstant, targetURL)
	if updateError != nil {
		executor.printfOutput(failureMessage, repositoryPath)
		return repoerrors.WrapMessage(
			repoerrors.OperationCanonicalRemote,
			repositoryPath,
			repoerrors.ErrRemoteUpdateFailed,
			fmt.Sprintf(failureMessage, repositoryPath),
		)
	}

	executor.printfOutput(successMessage, repositoryPath, FormatRemoteURLForDisplay(targetURL))
	return nil
}

// Execute performs the remote update workflow using transient executor state.
func Execute(executionContext context.Context, dependencies Dependencies, options Options) error {
	return NewExecutor(dependencies).Execute(executionContext, options)
}

func (executor *Executor) printfOutput(format string, arguments ...any) {
	if executor.dependencies.Reporter == nil {
		return
	}
	executor.dependencies.Reporter.Printf(format, arguments...)
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
