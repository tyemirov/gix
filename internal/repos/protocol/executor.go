package protocol

import (
	"context"
	"fmt"
	"strings"

	repoerrors "github.com/tyemirov/gix/internal/repos/errors"
	"github.com/tyemirov/gix/internal/repos/remotes"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	ownerRepoErrorMessage = "cannot derive owner/repo for protocol conversion in %s"
	targetErrorMessage    = "cannot build target URL for protocol '%s' in %s"
	promptTemplate        = "Convert 'origin' in '%s' (%s → %s)? [a/N/y] "
	failureMessage        = "failed to set origin to %s in %s"
	declinedReason        = "user declined"
)

// Options configures the protocol conversion workflow.
type Options struct {
	RepositoryPath           shared.RepositoryPath
	OriginOwnerRepository    *shared.OwnerRepository
	CanonicalOwnerRepository *shared.OwnerRepository
	CurrentProtocol          shared.RemoteProtocol
	TargetProtocol           shared.RemoteProtocol
	ConfirmationPolicy       shared.ConfirmationPolicy
}

// Dependencies supplies collaborators required for protocol conversion.
type Dependencies struct {
	GitManager shared.GitRepositoryManager
	Prompter   shared.ConfirmationPrompter
	Reporter   shared.Reporter
}

// Executor orchestrates protocol conversions for repository remotes.
type Executor struct {
	dependencies Dependencies
}

// NewExecutor constructs an Executor with the provided dependencies.
func NewExecutor(dependencies Dependencies) *Executor {
	return &Executor{dependencies: dependencies}
}

// Execute performs the conversion using the executor's dependencies.
func (executor *Executor) Execute(executionContext context.Context, options Options) error {
	repositoryPath := options.RepositoryPath.String()

	if executor.dependencies.GitManager == nil {
		reportDetails := map[string]string{"reason": "git_manager_unavailable"}
		executor.report(shared.EventLevelError, shared.EventCodeProtocolSkip, repositoryPath, nil, "git manager unavailable", reportDetails)
		return repoerrors.WrapMessage(
			repoerrors.OperationProtocolConvert,
			repositoryPath,
			repoerrors.ErrGitManagerUnavailable,
			fmt.Sprintf(failureMessage, "", repositoryPath),
		)
	}

	currentURL, fetchError := executor.dependencies.GitManager.GetRemoteURL(executionContext, repositoryPath, shared.OriginRemoteNameConstant)
	if fetchError != nil {
		return repoerrors.Wrap(
			repoerrors.OperationProtocolConvert,
			repositoryPath,
			repoerrors.ErrRemoteEnumerationFailed,
			fetchError,
		)
	}

	currentProtocol := detectProtocol(currentURL)
	if currentProtocol != options.CurrentProtocol {
		executor.report(
			shared.EventLevelInfo,
			shared.EventCodeProtocolSkip,
			repositoryPath,
			nil,
			fmt.Sprintf("no-op: current protocol %s does not match from %s", currentProtocol, options.CurrentProtocol),
			map[string]string{
				"reason":           "protocol_mismatch",
				"current_protocol": string(currentProtocol),
				"from_protocol":    string(options.CurrentProtocol),
				"target_protocol":  string(options.TargetProtocol),
			},
		)
		return nil
	}

	var ownerRepository *shared.OwnerRepository
	if options.CanonicalOwnerRepository != nil {
		ownerRepository = options.CanonicalOwnerRepository
	} else if options.OriginOwnerRepository != nil {
		ownerRepository = options.OriginOwnerRepository
	}

	if ownerRepository == nil {
		executor.report(
			shared.EventLevelWarn,
			shared.EventCodeProtocolSkip,
			repositoryPath,
			nil,
			"owner metadata missing",
			map[string]string{"reason": "owner_missing"},
		)
		return repoerrors.WrapMessage(
			repoerrors.OperationProtocolConvert,
			repositoryPath,
			repoerrors.ErrOriginOwnerMissing,
			fmt.Sprintf(ownerRepoErrorMessage, repositoryPath),
		)
	}

	ownerRepoString := ownerRepository.String()

	targetURL, targetError := remotes.BuildRemoteURL(options.TargetProtocol, ownerRepoString)
	if targetError != nil {
		executor.report(
			shared.EventLevelWarn,
			shared.EventCodeProtocolSkip,
			repositoryPath,
			ownerRepository,
			"unknown protocol",
			map[string]string{
				"reason":           "unknown_protocol",
				"target_protocol":  string(options.TargetProtocol),
				"current_protocol": string(currentProtocol),
			},
		)
		return repoerrors.WrapMessage(
			repoerrors.OperationProtocolConvert,
			repositoryPath,
			repoerrors.ErrUnknownProtocol,
			fmt.Sprintf(targetErrorMessage, string(options.TargetProtocol), repositoryPath),
		)
	}

	if options.ConfirmationPolicy.ShouldPrompt() && executor.dependencies.Prompter != nil {
		prompt := fmt.Sprintf(promptTemplate, repositoryPath, currentProtocol, options.TargetProtocol)
		confirmationResult, promptError := executor.dependencies.Prompter.Confirm(prompt)
		if promptError != nil {
			executor.report(
				shared.EventLevelWarn,
				shared.EventCodeProtocolSkip,
				repositoryPath,
				ownerRepository,
				"confirmation failed",
				map[string]string{
					"reason":           "confirmation_error",
					"target_protocol":  string(options.TargetProtocol),
					"current_protocol": string(currentProtocol),
				},
			)
			return repoerrors.WrapMessage(
				repoerrors.OperationProtocolConvert,
				repositoryPath,
				repoerrors.ErrUserConfirmationFailed,
				fmt.Sprintf(failureMessage, targetURL, repositoryPath),
			)
		}
		if !confirmationResult.Confirmed {
			executor.report(
				shared.EventLevelWarn,
				shared.EventCodeProtocolDeclined,
				repositoryPath,
				ownerRepository,
				declinedReason,
				map[string]string{
					"reason":           "user_declined",
					"target_protocol":  string(options.TargetProtocol),
					"current_protocol": string(currentProtocol),
				},
			)
			return nil
		}
	}

	updateError := executor.dependencies.GitManager.SetRemoteURL(executionContext, repositoryPath, shared.OriginRemoteNameConstant, targetURL)
	if updateError != nil {
		executor.report(
			shared.EventLevelWarn,
			shared.EventCodeProtocolSkip,
			repositoryPath,
			ownerRepository,
			"failed to update remote",
			map[string]string{
				"reason":           "update_failed",
				"target_protocol":  string(options.TargetProtocol),
				"current_protocol": string(currentProtocol),
			},
		)
		return repoerrors.WrapMessage(
			repoerrors.OperationProtocolConvert,
			repositoryPath,
			repoerrors.ErrRemoteUpdateFailed,
			fmt.Sprintf(failureMessage, targetURL, repositoryPath),
		)
	}

	executor.report(
		shared.EventLevelInfo,
		shared.EventCodeProtocolUpdate,
		repositoryPath,
		ownerRepository,
		fmt.Sprintf("origin now %s", remotes.FormatRemoteURLForDisplay(targetURL)),
		map[string]string{
			"current_protocol": string(currentProtocol),
			"target_protocol":  string(options.TargetProtocol),
			"target_url":       remotes.FormatRemoteURLForDisplay(targetURL),
		},
	)
	return nil
}

// Execute performs the conversion using transient executor state.
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

	executor.dependencies.Reporter.Report(shared.Event{
		Level:                level,
		Code:                 code,
		RepositoryIdentifier: repositoryIdentifier,
		RepositoryPath:       repositoryPath,
		Message:              message,
		Details:              metadata,
	})
}

func detectProtocol(remoteURL string) shared.RemoteProtocol {
	switch {
	case strings.HasPrefix(remoteURL, shared.GitProtocolURLPrefixConstant):
		return shared.RemoteProtocolGit
	case strings.HasPrefix(remoteURL, shared.SSHProtocolURLPrefixConstant):
		return shared.RemoteProtocolSSH
	case strings.HasPrefix(remoteURL, shared.HTTPSProtocolURLPrefixConstant):
		return shared.RemoteProtocolHTTPS
	default:
		return shared.RemoteProtocolOther
	}
}
