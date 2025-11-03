package refresh

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
	branchNameRequiredMessageConstant           = "branch name must be provided"
	gitExecutorMissingMessageConstant           = "git executor not configured"
	repositoryManagerMissingMessageConstant     = "repository manager not configured"
	cleanVerificationErrorTemplateConstant      = "failed to verify clean worktree: %w"
	worktreeNotCleanMessageConstant             = "repository worktree is not clean"
	stashFailureTemplateConstant                = "failed to stash local changes: %w"
	stageFailureTemplateConstant                = "failed to stage local changes: %w"
	commitFailureTemplateConstant               = "failed to commit local changes: %w"
	commitMessageTemplateConstant               = "chore: checkpoint before refreshing %s"
	gitFetchFailureTemplateConstant             = "failed to fetch updates: %w"
	gitCheckoutFailureTemplateConstant          = "failed to checkout branch %q: %w"
	gitPullFailureTemplateConstant              = "failed to pull latest changes: %w"
	gitFetchSubcommandConstant                  = "fetch"
	gitFetchPruneFlagConstant                   = "--prune"
	gitCheckoutSubcommandConstant               = "checkout"
	gitPullSubcommandConstant                   = "pull"
	gitPullFastForwardFlagConstant              = "--ff-only"
	gitPullRebaseFlagConstant                   = "--rebase"
	gitAddSubcommandConstant                    = "add"
	gitAddAllFlagConstant                       = "--all"
	gitCommitSubcommandConstant                 = "commit"
	gitCommitMessageFlagConstant                = "-m"
	gitStashSubcommandConstant                  = "stash"
	gitStashPushSubcommandConstant              = "push"
	gitStashIncludeUntrackedFlagConstant        = "--include-untracked"
	gitTerminalPromptEnvironmentNameConstant    = "GIT_TERMINAL_PROMPT"
	gitTerminalPromptEnvironmentDisableConstant = "0"
)

// ErrRepositoryPathRequired indicates the repository path option was empty.
var ErrRepositoryPathRequired = errors.New(repositoryPathRequiredMessageConstant)

// ErrBranchNameRequired indicates the branch name option was empty.
var ErrBranchNameRequired = errors.New(branchNameRequiredMessageConstant)

// ErrGitExecutorNotConfigured indicates the git executor dependency was missing.
var ErrGitExecutorNotConfigured = errors.New(gitExecutorMissingMessageConstant)

// ErrRepositoryManagerNotConfigured indicates the repository manager dependency was missing.
var ErrRepositoryManagerNotConfigured = errors.New(repositoryManagerMissingMessageConstant)

// ErrWorktreeNotClean indicates the repository contains uncommitted changes.
var ErrWorktreeNotClean = errors.New(worktreeNotCleanMessageConstant)

// Dependencies enumerates external collaborators required for refresh operations.
type Dependencies struct {
	GitExecutor       shared.GitExecutor
	RepositoryManager shared.GitRepositoryManager
}

// Options configures a branch refresh operation.
type Options struct {
	RepositoryPath string
	BranchName     string
	RequireClean   bool
	StashChanges   bool
	CommitChanges  bool
}

// Result captures the observable outcomes of a refresh.
type Result struct {
	RepositoryPath string
	BranchName     string
}

// Service coordinates branch refresh operations through git.
type Service struct {
	executor          shared.GitExecutor
	repositoryManager shared.GitRepositoryManager
}

// NewService constructs a Service from the provided dependencies.
func NewService(dependencies Dependencies) (*Service, error) {
	if dependencies.GitExecutor == nil {
		return nil, ErrGitExecutorNotConfigured
	}
	if dependencies.RepositoryManager == nil {
		return nil, ErrRepositoryManagerNotConfigured
	}
	return &Service{executor: dependencies.GitExecutor, repositoryManager: dependencies.RepositoryManager}, nil
}

// Refresh synchronizes the specified branch with its remote counterpart.
func (service *Service) Refresh(executionContext context.Context, options Options) (Result, error) {
	trimmedRepositoryPath := strings.TrimSpace(options.RepositoryPath)
	if len(trimmedRepositoryPath) == 0 {
		return Result{}, ErrRepositoryPathRequired
	}

	trimmedBranchName := strings.TrimSpace(options.BranchName)
	if len(trimmedBranchName) == 0 {
		return Result{}, ErrBranchNameRequired
	}

	requireClean := options.RequireClean
	checkpointCommitCreated := false
	if requireClean {
		clean, cleanError := service.repositoryManager.CheckCleanWorktree(executionContext, trimmedRepositoryPath)
		if cleanError != nil {
			return Result{}, fmt.Errorf(cleanVerificationErrorTemplateConstant, cleanError)
		}
		if !clean {
			if options.StashChanges {
				if stashError := service.stashLocalChanges(executionContext, trimmedRepositoryPath); stashError != nil {
					return Result{}, stashError
				}
			} else if options.CommitChanges {
				if commitError := service.commitLocalChanges(executionContext, trimmedRepositoryPath, trimmedBranchName); commitError != nil {
					return Result{}, commitError
				}
				checkpointCommitCreated = true
			} else {
				return Result{}, ErrWorktreeNotClean
			}

			clean, cleanError = service.repositoryManager.CheckCleanWorktree(executionContext, trimmedRepositoryPath)
			if cleanError != nil {
				return Result{}, fmt.Errorf(cleanVerificationErrorTemplateConstant, cleanError)
			}
			if !clean {
				return Result{}, ErrWorktreeNotClean
			}
		}
	}

	if fetchError := service.executeGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitFetchSubcommandConstant, gitFetchPruneFlagConstant},
		WorkingDirectory: trimmedRepositoryPath,
	}); fetchError != nil {
		return Result{}, fmt.Errorf(gitFetchFailureTemplateConstant, fetchError)
	}

	if checkoutError := service.executeGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitCheckoutSubcommandConstant, trimmedBranchName},
		WorkingDirectory: trimmedRepositoryPath,
	}); checkoutError != nil {
		return Result{}, fmt.Errorf(gitCheckoutFailureTemplateConstant, trimmedBranchName, checkoutError)
	}

	pullArguments := []string{gitPullSubcommandConstant, gitPullFastForwardFlagConstant}
	if checkpointCommitCreated {
		pullArguments = []string{gitPullSubcommandConstant, gitPullRebaseFlagConstant}
	}
	if pullError := service.executeGit(executionContext, execshell.CommandDetails{
		Arguments:        pullArguments,
		WorkingDirectory: trimmedRepositoryPath,
	}); pullError != nil {
		return Result{}, fmt.Errorf(gitPullFailureTemplateConstant, pullError)
	}

	return Result{RepositoryPath: trimmedRepositoryPath, BranchName: trimmedBranchName}, nil
}

func (service *Service) executeGit(executionContext context.Context, details execshell.CommandDetails) error {
	if details.EnvironmentVariables == nil {
		details.EnvironmentVariables = map[string]string{}
	}
	details.EnvironmentVariables[gitTerminalPromptEnvironmentNameConstant] = gitTerminalPromptEnvironmentDisableConstant
	_, executionError := service.executor.ExecuteGit(executionContext, details)
	return executionError
}

func (service *Service) stashLocalChanges(executionContext context.Context, repositoryPath string) error {
	if stashError := service.executeGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitStashSubcommandConstant, gitStashPushSubcommandConstant, gitStashIncludeUntrackedFlagConstant},
		WorkingDirectory: repositoryPath,
	}); stashError != nil {
		return fmt.Errorf(stashFailureTemplateConstant, stashError)
	}
	return nil
}

func (service *Service) commitLocalChanges(executionContext context.Context, repositoryPath string, branchName string) error {
	if stageError := service.executeGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitAddSubcommandConstant, gitAddAllFlagConstant},
		WorkingDirectory: repositoryPath,
	}); stageError != nil {
		return fmt.Errorf(stageFailureTemplateConstant, stageError)
	}

	commitMessage := fmt.Sprintf(commitMessageTemplateConstant, branchName)
	if commitError := service.executeGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitCommitSubcommandConstant, gitCommitMessageFlagConstant, commitMessage},
		WorkingDirectory: repositoryPath,
	}); commitError != nil {
		return fmt.Errorf(commitFailureTemplateConstant, commitError)
	}

	return nil
}
