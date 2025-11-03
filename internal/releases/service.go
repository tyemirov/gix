package releases

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	repositoryPathRequiredMessageConstant       = "repository path must be provided"
	tagNameRequiredMessageConstant              = "tag name must be provided"
	gitExecutorMissingMessageConstant           = "git executor not configured"
	annotateTagFailureTemplateConstant          = "failed to create tag %q: %w"
	pushTagFailureTemplateConstant              = "failed to push tag %q to %s: %w"
	defaultRemoteNameConstant                   = "origin"
	gitTagSubcommandConstant                    = "tag"
	gitTagAnnotatedFlagConstant                 = "-a"
	gitTagMessageFlagConstant                   = "-m"
	gitPushSubcommandConstant                   = "push"
	gitTerminalPromptEnvironmentNameConstant    = "GIT_TERMINAL_PROMPT"
	gitTerminalPromptEnvironmentDisableConstant = "0"
)

// ErrRepositoryPathRequired indicates the repository path option was empty.
var ErrRepositoryPathRequired = errors.New(repositoryPathRequiredMessageConstant)

// ErrTagNameRequired indicates the tag name option was empty.
var ErrTagNameRequired = errors.New(tagNameRequiredMessageConstant)

// ErrGitExecutorNotConfigured indicates the git executor dependency was missing.
var ErrGitExecutorNotConfigured = errors.New(gitExecutorMissingMessageConstant)

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
	DryRun         bool
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

	if options.DryRun {
		return Result{RepositoryPath: repositoryPath, TagName: tagName}, nil
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

	if _, err := service.executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:            []string{gitTagSubcommandConstant, gitTagAnnotatedFlagConstant, tagName, gitTagMessageFlagConstant, message},
		WorkingDirectory:     repositoryPath,
		EnvironmentVariables: environment,
	}); err != nil {
		return Result{}, fmt.Errorf(annotateTagFailureTemplateConstant, tagName, err)
	}

	if _, err := service.executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:            []string{gitPushSubcommandConstant, remoteName, tagName},
		WorkingDirectory:     repositoryPath,
		EnvironmentVariables: environment,
	}); err != nil {
		return Result{}, fmt.Errorf(pushTagFailureTemplateConstant, tagName, remoteName, err)
	}

	return Result{RepositoryPath: repositoryPath, TagName: tagName}, nil
}
