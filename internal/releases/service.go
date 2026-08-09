package releases

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	repoerrors "github.com/tyemirov/gix/internal/repos/errors"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	repositoryPathRequiredMessageConstant       = "repository path must be provided"
	tagNameRequiredMessageConstant              = "tag name must be provided"
	gitExecutorMissingMessageConstant           = "git executor not configured"
	retagMappingsRequiredMessageConstant        = "retag requires at least one mapping"
	defaultRemoteNameConstant                   = "origin"
	gitTagSubcommandConstant                    = "tag"
	gitTagAnnotatedFlagConstant                 = "-a"
	gitTagMessageFlagConstant                   = "-m"
	gitTagDeleteFlagConstant                    = "-d"
	gitPushForceFlagConstant                    = "--force"
	gitPushSubcommandConstant                   = "push"
	gitRevParseSubcommandConstant               = "rev-parse"
	gitTerminalPromptEnvironmentNameConstant    = "GIT_TERMINAL_PROMPT"
	gitTerminalPromptEnvironmentDisableConstant = "0"
)

// ErrRepositoryPathRequired indicates the repository path option was empty.
var ErrRepositoryPathRequired = errors.New(repositoryPathRequiredMessageConstant)

// ErrTagNameRequired indicates the tag name option was empty.
var ErrTagNameRequired = errors.New(tagNameRequiredMessageConstant)

// ErrGitExecutorNotConfigured indicates the git executor dependency was missing.
var ErrGitExecutorNotConfigured = errors.New(gitExecutorMissingMessageConstant)

// ErrRetagMappingsRequired indicates no retag mappings were provided.
var ErrRetagMappingsRequired = errors.New(retagMappingsRequiredMessageConstant)

// ServiceDependencies enumerates collaborators required by the release service.
type ServiceDependencies struct {
	GitExecutor shared.GitExecutor
}

// Options configure a release operation.
type Options struct {
	RepositoryPath string
	TagName        string
	Message        string
	RemoteName     string
}

// Result captures the outcome of a release.
type Result struct {
	RepositoryPath string
	TagName        string
}

// Service orchestrates tag creation and pushing.
type Service struct {
	executor shared.GitExecutor
}

// RetagMapping describes one tag reassignment.
type RetagMapping struct {
	TagName         string
	TargetReference string
	Message         string
}

// RetagOptions configure a retag operation across a repository.
type RetagOptions struct {
	RepositoryPath string
	RemoteName     string
	Mappings       []RetagMapping
}

// RetagResult captures the outcome for a single tag.
type RetagResult struct {
	TagName         string
	TargetReference string
}

// NewService constructs a Service from dependencies.
func NewService(dependencies ServiceDependencies) (*Service, error) {
	if dependencies.GitExecutor == nil {
		return nil, ErrGitExecutorNotConfigured
	}
	return &Service{executor: dependencies.GitExecutor}, nil
}

// Release annotates a tag and pushes it to the selected remote.
func (service *Service) Release(executionContext context.Context, options Options) (Result, error) {
	repositoryPath := strings.TrimSpace(options.RepositoryPath)
	if len(repositoryPath) == 0 {
		return Result{}, ErrRepositoryPathRequired
	}

	tagName := strings.TrimSpace(options.TagName)
	if len(tagName) == 0 {
		return Result{}, ErrTagNameRequired
	}

	message := strings.TrimSpace(options.Message)
	if len(message) == 0 {
		message = fmt.Sprintf("Release %s", tagName)
	}

	remoteName := strings.TrimSpace(options.RemoteName)
	if len(remoteName) == 0 {
		remoteName = defaultRemoteNameConstant
	}

	environment := map[string]string{gitTerminalPromptEnvironmentNameConstant: gitTerminalPromptEnvironmentDisableConstant}

	annotateCommand := execshell.CommandDetails{
		Arguments:            []string{gitTagSubcommandConstant, gitTagAnnotatedFlagConstant, tagName, gitTagMessageFlagConstant, message},
		WorkingDirectory:     repositoryPath,
		EnvironmentVariables: environment,
	}

	if _, err := service.executor.ExecuteGit(executionContext, annotateCommand); err != nil {
		messageText := formatReleaseTagCreateFailure(tagName, annotateCommand, err)
		detail := fmt.Errorf("%s: %w", messageText, err)
		return Result{}, repoerrors.WrapWithMessage(
			repoerrors.OperationReleaseTag,
			repositoryPath,
			repoerrors.ErrReleaseTagCreateFailed,
			detail,
			messageText,
		)
	}

	pushCommand := execshell.CommandDetails{
		Arguments:            []string{gitPushSubcommandConstant, remoteName, tagName},
		WorkingDirectory:     repositoryPath,
		EnvironmentVariables: environment,
	}

	if _, err := service.executor.ExecuteGit(executionContext, pushCommand); err != nil {
		messageText := formatReleaseTagPushFailure(tagName, remoteName, pushCommand, err)
		detail := fmt.Errorf("%s: %w", messageText, err)
		return Result{}, repoerrors.WrapWithMessage(
			repoerrors.OperationReleaseTag,
			repositoryPath,
			repoerrors.ErrReleaseTagPushFailed,
			detail,
			messageText,
		)
	}

	return Result{RepositoryPath: repositoryPath, TagName: tagName}, nil
}

// Retag deletes and recreates existing annotated tags, pushing the updates to the configured remote.
func (service *Service) Retag(executionContext context.Context, options RetagOptions) ([]RetagResult, error) {
	repositoryPath := strings.TrimSpace(options.RepositoryPath)
	if len(repositoryPath) == 0 {
		return nil, ErrRepositoryPathRequired
	}
	if len(options.Mappings) == 0 {
		return nil, ErrRetagMappingsRequired
	}

	remoteName := strings.TrimSpace(options.RemoteName)
	if len(remoteName) == 0 {
		remoteName = defaultRemoteNameConstant
	}

	results := make([]RetagResult, 0, len(options.Mappings))
	environment := map[string]string{gitTerminalPromptEnvironmentNameConstant: gitTerminalPromptEnvironmentDisableConstant}

	for _, mapping := range options.Mappings {
		tagName := strings.TrimSpace(mapping.TagName)
		if len(tagName) == 0 {
			return nil, ErrTagNameRequired
		}

		targetReference := strings.TrimSpace(mapping.TargetReference)
		if len(targetReference) == 0 {
			return nil, fmt.Errorf("target reference required for tag %s", tagName)
		}

		resolveCommand := execshell.CommandDetails{
			Arguments:            []string{gitRevParseSubcommandConstant, "--verify", targetReference},
			WorkingDirectory:     repositoryPath,
			EnvironmentVariables: environment,
		}
		if _, err := service.executor.ExecuteGit(executionContext, resolveCommand); err != nil {
			messageText := formatReleaseFailureMessage(fmt.Sprintf("failed to resolve %q for tag %q", targetReference, tagName), resolveCommand, err)
			return nil, repoerrors.WrapWithMessage(
				repoerrors.OperationReleaseTag,
				repositoryPath,
				repoerrors.ErrReleaseTagResolveFailed,
				err,
				messageText,
			)
		}

		results = append(results, RetagResult{TagName: tagName, TargetReference: targetReference})

		tagExists := false
		inspectCommand := execshell.CommandDetails{
			Arguments:            []string{gitRevParseSubcommandConstant, "--verify", fmt.Sprintf("refs/tags/%s", tagName)},
			WorkingDirectory:     repositoryPath,
			EnvironmentVariables: environment,
		}
		if _, err := service.executor.ExecuteGit(executionContext, inspectCommand); err == nil {
			tagExists = true
		}

		if tagExists {
			deleteCommand := execshell.CommandDetails{
				Arguments:            []string{gitTagSubcommandConstant, gitTagDeleteFlagConstant, tagName},
				WorkingDirectory:     repositoryPath,
				EnvironmentVariables: environment,
			}
			if _, err := service.executor.ExecuteGit(executionContext, deleteCommand); err != nil {
				messageText := formatReleaseFailureMessage(fmt.Sprintf("failed to delete tag %q", tagName), deleteCommand, err)
				return nil, repoerrors.WrapWithMessage(
					repoerrors.OperationReleaseTag,
					repositoryPath,
					repoerrors.ErrReleaseTagDeleteFailed,
					err,
					messageText,
				)
			}
		}

		message := strings.TrimSpace(mapping.Message)
		if len(message) == 0 {
			message = fmt.Sprintf("Retag %s to %s", tagName, targetReference)
		}

		createCommand := execshell.CommandDetails{
			Arguments:            []string{gitTagSubcommandConstant, gitTagAnnotatedFlagConstant, tagName, targetReference, gitTagMessageFlagConstant, message},
			WorkingDirectory:     repositoryPath,
			EnvironmentVariables: environment,
		}
		if _, err := service.executor.ExecuteGit(executionContext, createCommand); err != nil {
			messageText := formatReleaseFailureMessage(fmt.Sprintf("failed to create tag %q", tagName), createCommand, err)
			return nil, repoerrors.WrapWithMessage(
				repoerrors.OperationReleaseTag,
				repositoryPath,
				repoerrors.ErrReleaseTagCreateFailed,
				err,
				messageText,
			)
		}

		pushCommand := execshell.CommandDetails{
			Arguments:            []string{gitPushSubcommandConstant, gitPushForceFlagConstant, remoteName, tagName},
			WorkingDirectory:     repositoryPath,
			EnvironmentVariables: environment,
		}
		if _, err := service.executor.ExecuteGit(executionContext, pushCommand); err != nil {
			messageText := formatReleaseFailureMessage(fmt.Sprintf("failed to push tag %q to %s", tagName, remoteName), pushCommand, err)
			return nil, repoerrors.WrapWithMessage(
				repoerrors.OperationReleaseTag,
				repositoryPath,
				repoerrors.ErrReleaseTagPushFailed,
				err,
				messageText,
			)
		}
	}

	return results, nil
}

func formatReleaseTagCreateFailure(tagName string, command execshell.CommandDetails, err error) string {
	return formatReleaseFailureMessage(fmt.Sprintf("failed to create tag %q", tagName), command, err)
}

func formatReleaseTagPushFailure(tagName string, remoteName string, command execshell.CommandDetails, err error) string {
	return formatReleaseFailureMessage(fmt.Sprintf("failed to push tag %q to %s", tagName, remoteName), command, err)
}

func formatReleaseFailureMessage(prefix string, command execshell.CommandDetails, err error) string {
	commandSummary := formatGitCommand(command)
	details := describeGitFailure(err)
	if len(details) == 0 {
		return fmt.Sprintf("%s: %s", prefix, commandSummary)
	}
	return fmt.Sprintf("%s: %s (%s)", prefix, commandSummary, details)
}

func formatGitCommand(command execshell.CommandDetails) string {
	if len(command.Arguments) == 0 {
		return "git"
	}

	formatted := make([]string, 0, len(command.Arguments)+1)
	formatted = append(formatted, "git")
	for _, argument := range command.Arguments {
		formatted = append(formatted, quoteArgument(argument))
	}

	return strings.Join(formatted, " ")
}

func quoteArgument(argument string) string {
	if len(argument) == 0 {
		return `""`
	}
	if strings.ContainsAny(argument, " \t\"'") {
		return strconv.Quote(argument)
	}
	return argument
}

func describeGitFailure(err error) string {
	if err == nil {
		return ""
	}

	var commandFailed execshell.CommandFailedError
	if errors.As(err, &commandFailed) {
		parts := make([]string, 0, 2)
		if commandFailed.Result.ExitCode != 0 {
			parts = append(parts, fmt.Sprintf("exit code %d", commandFailed.Result.ExitCode))
		}
		if stderr := summarizeStandardError(commandFailed.Result.StandardError); len(stderr) > 0 {
			parts = append(parts, fmt.Sprintf("stderr: %s", stderr))
		}
		return strings.Join(parts, ", ")
	}

	return strings.TrimSpace(err.Error())
}

func summarizeStandardError(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) == 0 {
		return ""
	}

	collapsed := strings.Join(strings.Fields(trimmed), " ")
	const maxLength = 200
	if len(collapsed) > maxLength {
		return collapsed[:maxLength]
	}
	return collapsed
}
