package migrate

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/gitrepo"
)

const (
	localBranchVerificationErrorTemplateConstant  = "default branch local verification failed: %w"
	localBranchCreationErrorTemplateConstant      = "default branch local creation failed: %w"
	localCheckoutErrorTemplateConstant            = "default branch checkout failed: %w"
	remoteBranchVerificationErrorTemplateConstant = "default branch remote verification failed: %w"
	remoteBranchCreationErrorTemplateConstant     = "default branch remote creation failed: %w"
	gitShowRefCommandNameConstant                 = "show-ref"
	gitShowRefVerifyFlagConstant                  = "--verify"
	gitShowRefQuietFlagConstant                   = "--quiet"
	gitHeadsReferencePrefixConstant               = "refs/heads/"
	gitLSRemoteCommandNameConstant                = "ls-remote"
	gitHeadsFlagConstant                          = "--heads"
	gitTerminalPromptEnvironmentNameConstant      = "GIT_TERMINAL_PROMPT"
	gitTerminalPromptDisableValueConstant         = "0"
)

func prepareTargetBranch(executionContext context.Context, manager *gitrepo.RepositoryManager, executor CommandExecutor, options MigrationOptions) error {
	targetBranch := strings.TrimSpace(string(options.TargetBranch))
	sourceBranch := strings.TrimSpace(string(options.SourceBranch))
	if ensureLocalError := ensureLocalTargetBranch(executionContext, manager, executor, options.RepositoryPath, targetBranch, sourceBranch); ensureLocalError != nil {
		return ensureLocalError
	}
	if checkoutError := manager.CheckoutBranch(executionContext, options.RepositoryPath, targetBranch); checkoutError != nil {
		return fmt.Errorf(localCheckoutErrorTemplateConstant, checkoutError)
	}
	if !options.PushUpdates {
		return nil
	}
	return ensureRemoteTargetBranch(executionContext, executor, options.RepositoryPath, options.RepositoryRemoteName, targetBranch)
}

func ensureLocalTargetBranch(executionContext context.Context, manager *gitrepo.RepositoryManager, executor CommandExecutor, repositoryPath string, branchName string, sourceBranch string) error {
	exists, existsError := localTargetBranchExists(executionContext, executor, repositoryPath, branchName)
	if existsError != nil {
		return fmt.Errorf(localBranchVerificationErrorTemplateConstant, existsError)
	}
	if exists {
		return nil
	}
	if creationError := manager.CreateBranch(executionContext, repositoryPath, branchName, sourceBranch); creationError != nil {
		return fmt.Errorf(localBranchCreationErrorTemplateConstant, creationError)
	}
	return nil
}

func localTargetBranchExists(executionContext context.Context, executor CommandExecutor, repositoryPath string, branchName string) (bool, error) {
	reference := fmt.Sprintf("%s%s", gitHeadsReferencePrefixConstant, strings.TrimSpace(branchName))
	_, executionError := executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitShowRefCommandNameConstant, gitShowRefVerifyFlagConstant, gitShowRefQuietFlagConstant, reference},
		WorkingDirectory: repositoryPath,
	})
	if executionError != nil {
		var commandFailure execshell.CommandFailedError
		if errors.As(executionError, &commandFailure) && commandFailure.Result.ExitCode == 1 {
			return false, nil
		}
		return false, executionError
	}
	return true, nil
}

func ensureRemoteTargetBranch(executionContext context.Context, executor CommandExecutor, repositoryPath string, remoteName string, branchName string) error {
	exists, existsError := remoteTargetBranchExists(executionContext, executor, repositoryPath, remoteName, branchName)
	if existsError != nil {
		return fmt.Errorf(remoteBranchVerificationErrorTemplateConstant, existsError)
	}
	if exists {
		return nil
	}
	pushArguments := []string{gitPushCommandNameConstant, remoteName, fmt.Sprintf("%s:%s", branchName, branchName)}
	_, pushError := executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:            pushArguments,
		WorkingDirectory:     repositoryPath,
		EnvironmentVariables: map[string]string{gitTerminalPromptEnvironmentNameConstant: gitTerminalPromptDisableValueConstant},
	})
	if pushError != nil {
		if shouldIgnoreRemotePushError(pushError) {
			return nil
		}
		return fmt.Errorf(remoteBranchCreationErrorTemplateConstant, pushError)
	}
	return nil
}

func remoteTargetBranchExists(executionContext context.Context, executor CommandExecutor, repositoryPath string, remoteName string, branchName string) (bool, error) {
	result, executionError := executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitLSRemoteCommandNameConstant, gitHeadsFlagConstant, remoteName, strings.TrimSpace(branchName)},
		WorkingDirectory: repositoryPath,
	})
	if executionError != nil {
		var commandFailure execshell.CommandFailedError
		if errors.As(executionError, &commandFailure) && shouldIgnoreRemoteBranchError(commandFailure) {
			return false, nil
		}
		return false, executionError
	}
	return strings.TrimSpace(result.StandardOutput) != "", nil
}

func shouldIgnoreRemoteBranchError(failure execshell.CommandFailedError) bool {
	if failure.Result.ExitCode == 1 || failure.Result.ExitCode == 128 {
		normalized := strings.ToLower(strings.TrimSpace(failure.Result.StandardError))
		if len(normalized) == 0 {
			normalized = strings.ToLower(strings.TrimSpace(failure.Error()))
		}
		if strings.Contains(normalized, "could not read from remote repository") {
			return true
		}
		if strings.Contains(normalized, "not a git repository") {
			return true
		}
		if failure.Result.ExitCode == 1 {
			return true
		}
	}
	return false
}

func shouldIgnoreRemotePushError(err error) bool {
	var commandFailure execshell.CommandFailedError
	if !errors.As(err, &commandFailure) {
		return false
	}
	if commandFailure.Result.ExitCode == 128 {
		normalized := strings.ToLower(strings.TrimSpace(commandFailure.Result.StandardError))
		if len(normalized) == 0 {
			normalized = strings.ToLower(strings.TrimSpace(commandFailure.Error()))
		}
		if strings.Contains(normalized, "could not read from remote repository") {
			return true
		}
		if strings.Contains(normalized, "not a git repository") {
			return true
		}
	}
	return false
}
