package workflow

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/gitrepo"
	migrate "github.com/temirov/gix/internal/migrate"
	"github.com/temirov/gix/internal/repos/shared"
)

const (
	defaultMigrationRemoteNameConstant                 = "origin"
	defaultMigrationTargetBranchConstant               = "master"
	defaultMigrationWorkflowsDirectoryConstant         = ".github/workflows"
	migrationDryRunMessageTemplateConstant             = "WORKFLOW-PLAN: default %s (%s → %s)\n"
	migrationSuccessMessageTemplateConstant            = "WORKFLOW-DEFAULT: %s (%s → %s) safe_to_delete=%t\n"
	migrationIdentifierMissingMessageConstant          = "repository identifier unavailable for default-branch target"
	migrationExecutionErrorTemplateConstant            = "default branch update failed: %w"
	migrationRefreshErrorTemplateConstant              = "failed to refresh repository after default branch update: %w"
	migrationDependenciesMissingMessageConstant        = "default branch update requires repository manager, git executor, and GitHub client"
	migrationMultipleTargetsUnsupportedMessageConstant = "default branch update requires exactly one target configuration"
	migrationMetadataResolutionErrorTemplateConstant   = "default branch metadata resolution failed: %w"
	migrationMetadataMissingMessageConstant            = "repository metadata missing default branch for update"
	migrationSkipMessageTemplateConstant               = "WORKFLOW-DEFAULT-SKIP: %s already defaults to %s\n"
	localBranchVerificationErrorTemplateConstant       = "default branch local verification failed: %w"
	localBranchCreationErrorTemplateConstant           = "default branch local creation failed: %w"
	localCheckoutErrorTemplateConstant                 = "default branch checkout failed: %w"
	remotePresenceResolutionErrorTemplateConstant      = "default branch remote resolution failed: %w"
	remoteBranchVerificationErrorTemplateConstant      = "default branch remote verification failed: %w"
	remoteBranchCreationErrorTemplateConstant          = "default branch remote creation failed: %w"
	localDefaultResolutionErrorTemplateConstant        = "default branch local detection failed: %w"
	sourceBranchMissingMessageConstant                 = "default branch source not detected for promotion"
	gitExecutorMissingMessageConstant                  = "git executor not configured"
	gitRemoteSubcommandConstant                        = "remote"
	gitShowRefSubcommandConstant                       = "show-ref"
	gitShowRefVerifyFlagConstant                       = "--verify"
	gitShowRefQuietFlagConstant                        = "--quiet"
	gitHeadsReferencePrefixConstant                    = "refs/heads/"
	gitPushSubcommandConstant                          = "push"
	gitLSRemoteSubcommandConstant                      = "ls-remote"
	gitHeadsFlagConstant                               = "--heads"
	gitTerminalPromptEnvironmentNameConstant           = "GIT_TERMINAL_PROMPT"
	gitTerminalPromptDisableValueConstant              = "0"
)

// BranchMigrationTarget describes branch migration behavior for discovered repositories.
type BranchMigrationTarget struct {
	RemoteName         string
	SourceBranch       string
	TargetBranch       string
	PushToRemote       bool
	DeleteSourceBranch bool
}

// BranchMigrationOperation performs default-branch migrations for configured targets.
type BranchMigrationOperation struct {
	Targets []BranchMigrationTarget
}

// Name identifies the operation type.
func (operation *BranchMigrationOperation) Name() string {
	return string(OperationTypeBranchDefault)
}

// Execute performs branch migration workflows for configured targets.
func (operation *BranchMigrationOperation) Execute(executionContext context.Context, environment *Environment, state *State) error {
	if environment == nil || state == nil {
		return nil
	}

	if environment.RepositoryManager == nil || environment.GitExecutor == nil || environment.GitHubClient == nil {
		return errors.New(migrationDependenciesMissingMessageConstant)
	}

	serviceDependencies := migrate.ServiceDependencies{
		Logger:            environment.Logger,
		RepositoryManager: environment.RepositoryManager,
		GitHubClient:      environment.GitHubClient,
		GitExecutor:       environment.GitExecutor,
	}

	migrationService, serviceError := migrate.NewService(serviceDependencies)
	if serviceError != nil {
		return fmt.Errorf(migrationExecutionErrorTemplateConstant, serviceError)
	}

	if len(operation.Targets) == 0 {
		return nil
	}
	if len(operation.Targets) > 1 {
		return errors.New(migrationMultipleTargetsUnsupportedMessageConstant)
	}

	target := operation.Targets[0]

	repositories := state.CloneRepositories()

	for repositoryIndex := range repositories {
		repositoryState := repositories[repositoryIndex]
		if repositoryState == nil {
			continue
		}

		repositoryPath := strings.TrimSpace(repositoryState.Path)
		if len(repositoryPath) == 0 {
			continue
		}

		remoteName := strings.TrimSpace(target.RemoteName)
		if len(remoteName) == 0 {
			remoteName = defaultMigrationRemoteNameConstant
		}

		localDefaultBranch, localDefaultError := resolveLocalDefaultBranch(executionContext, environment.RepositoryManager, repositoryState)
		if localDefaultError != nil {
			return fmt.Errorf(localDefaultResolutionErrorTemplateConstant, localDefaultError)
		}

		remoteAvailable, remoteResolutionError := remoteExists(executionContext, environment.GitExecutor, repositoryPath, remoteName)
		if remoteResolutionError != nil {
			return fmt.Errorf(remotePresenceResolutionErrorTemplateConstant, remoteResolutionError)
		}

		remoteDefaultBranch := ""
		if remoteAvailable {
			remoteDefaultBranch = strings.TrimSpace(repositoryState.Inspection.RemoteDefaultBranch)
		}

		repositoryIdentifier := ""
		identifierResolved := false
		if remoteAvailable {
			identifier, identifierError := resolveRepositoryIdentifier(repositoryState)
			if identifierError == nil && len(strings.TrimSpace(identifier)) > 0 {
				repositoryIdentifier = identifier
				identifierResolved = true
			} else {
				inferredIdentifier, inferred := inferRepositoryIdentifier(executionContext, environment.RepositoryManager, repositoryPath, remoteName)
				if inferred {
					repositoryIdentifier = inferredIdentifier
					identifierResolved = true
				} else {
					remoteAvailable = false
					remoteDefaultBranch = ""
				}
			}
		}

		repositoryMetadata := githubcli.RepositoryMetadata{}
		metadataResolved := false
		if remoteAvailable && identifierResolved {
			metadata, metadataError := environment.GitHubClient.ResolveRepoMetadata(executionContext, repositoryIdentifier)
			if metadataError != nil {
				return fmt.Errorf(migrationMetadataResolutionErrorTemplateConstant, metadataError)
			}
			repositoryMetadata = metadata
			metadataResolved = true
			canonicalIdentifier := strings.TrimSpace(metadata.NameWithOwner)
			if len(canonicalIdentifier) > 0 {
				repositoryIdentifier = canonicalIdentifier
			}
			if len(remoteDefaultBranch) == 0 {
				remoteDefaultBranch = strings.TrimSpace(metadata.DefaultBranch)
			}
		}

		sourceBranchValue := strings.TrimSpace(target.SourceBranch)
		if len(sourceBranchValue) == 0 {
			if remoteAvailable && len(remoteDefaultBranch) > 0 {
				sourceBranchValue = remoteDefaultBranch
			} else if remoteAvailable && metadataResolved {
				sourceBranchValue = strings.TrimSpace(repositoryMetadata.DefaultBranch)
			} else if len(localDefaultBranch) > 0 {
				sourceBranchValue = localDefaultBranch
			}
		}

		if remoteAvailable && len(sourceBranchValue) == 0 {
			metadata, metadataError := environment.GitHubClient.ResolveRepoMetadata(executionContext, repositoryIdentifier)
			if metadataError != nil {
				return fmt.Errorf(migrationMetadataResolutionErrorTemplateConstant, metadataError)
			}
			repositoryMetadata = metadata
			metadataResolved = true
			remoteDefaultBranch = strings.TrimSpace(metadata.DefaultBranch)
			if len(remoteDefaultBranch) == 0 {
				return errors.New(migrationMetadataMissingMessageConstant)
			}
			sourceBranchValue = remoteDefaultBranch
		}

		if len(sourceBranchValue) == 0 {
			return errors.New(sourceBranchMissingMessageConstant)
		}

		targetBranchValue := strings.TrimSpace(target.TargetBranch)
		if len(targetBranchValue) == 0 {
			targetBranchValue = defaultMigrationTargetBranchConstant
		}

		sourceBranch := migrate.BranchName(sourceBranchValue)
		targetBranch := migrate.BranchName(targetBranchValue)

		skipMigration := false
		if remoteAvailable && len(remoteDefaultBranch) > 0 && len(localDefaultBranch) > 0 {
			skipMigration = strings.EqualFold(targetBranchValue, remoteDefaultBranch) && strings.EqualFold(targetBranchValue, localDefaultBranch)
		} else if !remoteAvailable && len(localDefaultBranch) > 0 {
			skipMigration = strings.EqualFold(targetBranchValue, localDefaultBranch)
		}

		if skipMigration {
			if environment.Output != nil {
				fmt.Fprintf(environment.Output, migrationSkipMessageTemplateConstant, repositoryState.Path, targetBranchValue)
			}
			continue
		}

		options := migrate.MigrationOptions{
			RepositoryPath:       repositoryPath,
			RepositoryRemoteName: remoteName,
			RepositoryIdentifier: repositoryIdentifier,
			WorkflowsDirectory:   defaultMigrationWorkflowsDirectoryConstant,
			SourceBranch:         sourceBranch,
			TargetBranch:         targetBranch,
			PushUpdates:          target.PushToRemote && remoteAvailable,
			DeleteSourceBranch:   target.DeleteSourceBranch && remoteAvailable,
		}

		if environment.DryRun {
			if environment.Output != nil {
				fmt.Fprintf(environment.Output, migrationDryRunMessageTemplateConstant, repositoryState.Path, sourceBranchValue, targetBranchValue)
			}
			continue
		}

		if ensureLocalError := ensureLocalBranch(executionContext, environment.RepositoryManager, environment.GitExecutor, repositoryPath, targetBranchValue, sourceBranchValue); ensureLocalError != nil {
			return ensureLocalError
		}

		if checkoutError := environment.RepositoryManager.CheckoutBranch(executionContext, repositoryPath, targetBranchValue); checkoutError != nil {
			return fmt.Errorf(localCheckoutErrorTemplateConstant, checkoutError)
		}

		if !remoteAvailable {
			if environment.Output != nil {
				fmt.Fprintf(environment.Output, migrationSuccessMessageTemplateConstant, repositoryState.Path, sourceBranchValue, targetBranchValue, false)
			}
			continue
		}

		if target.PushToRemote {
			if ensureRemoteError := ensureRemoteBranch(executionContext, environment.GitExecutor, repositoryPath, remoteName, targetBranchValue); ensureRemoteError != nil {
				return ensureRemoteError
			}
		}

		result, executionError := migrationService.Execute(executionContext, options)
		if executionError != nil {
			var updateError migrate.DefaultBranchUpdateError
			if errors.As(executionError, &updateError) {
				return executionError
			}
			return fmt.Errorf(migrationExecutionErrorTemplateConstant, executionError)
		}

		if environment.Output != nil {
			fmt.Fprintf(environment.Output, migrationSuccessMessageTemplateConstant, repositoryState.Path, sourceBranchValue, targetBranchValue, result.SafetyStatus.SafeToDelete)
			for _, warning := range result.Warnings {
				fmt.Fprintln(environment.Output, warning)
			}
		}

		if environment.AuditService != nil {
			if refreshError := repositoryState.Refresh(executionContext, environment.AuditService); refreshError != nil {
				return fmt.Errorf(migrationRefreshErrorTemplateConstant, refreshError)
			}
		}
	}

	return nil
}

func resolveLocalDefaultBranch(executionContext context.Context, manager *gitrepo.RepositoryManager, repositoryState *RepositoryState) (string, error) {
	if manager == nil {
		return "", errors.New(migrationDependenciesMissingMessageConstant)
	}
	if repositoryState == nil {
		return "", nil
	}
	if value := strings.TrimSpace(repositoryState.Inspection.LocalBranch); len(value) > 0 {
		return value, nil
	}
	if len(strings.TrimSpace(repositoryState.Path)) == 0 {
		return "", nil
	}
	branchName, branchError := manager.GetCurrentBranch(executionContext, repositoryState.Path)
	if branchError != nil {
		return "", branchError
	}
	return strings.TrimSpace(branchName), nil
}

func remoteExists(executionContext context.Context, executor shared.GitExecutor, repositoryPath string, remoteName string) (bool, error) {
	if executor == nil {
		return false, errors.New(gitExecutorMissingMessageConstant)
	}
	result, executionError := executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitRemoteSubcommandConstant},
		WorkingDirectory: repositoryPath,
	})
	if executionError != nil {
		return false, executionError
	}
	for _, candidate := range strings.Split(result.StandardOutput, "\n") {
		if strings.TrimSpace(candidate) == remoteName {
			return true, nil
		}
	}
	return false, nil
}

func ensureLocalBranch(executionContext context.Context, manager *gitrepo.RepositoryManager, executor shared.GitExecutor, repositoryPath string, branchName string, sourceBranch string) error {
	exists, existsError := localBranchExists(executionContext, executor, repositoryPath, branchName)
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

func localBranchExists(executionContext context.Context, executor shared.GitExecutor, repositoryPath string, branchName string) (bool, error) {
	if executor == nil {
		return false, errors.New(gitExecutorMissingMessageConstant)
	}
	reference := fmt.Sprintf("%s%s", gitHeadsReferencePrefixConstant, strings.TrimSpace(branchName))
	_, executionError := executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitShowRefSubcommandConstant, gitShowRefVerifyFlagConstant, gitShowRefQuietFlagConstant, reference},
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

func ensureRemoteBranch(executionContext context.Context, executor shared.GitExecutor, repositoryPath string, remoteName string, branchName string) error {
	exists, existsError := remoteBranchExists(executionContext, executor, repositoryPath, remoteName, branchName)
	if existsError != nil {
		return fmt.Errorf(remoteBranchVerificationErrorTemplateConstant, existsError)
	}
	if exists {
		return nil
	}
	pushArguments := []string{gitPushSubcommandConstant, remoteName, fmt.Sprintf("%s:%s", branchName, branchName)}
	_, pushError := executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:            pushArguments,
		WorkingDirectory:     repositoryPath,
		EnvironmentVariables: map[string]string{gitTerminalPromptEnvironmentNameConstant: gitTerminalPromptDisableValueConstant},
	})
	if pushError != nil {
		return fmt.Errorf(remoteBranchCreationErrorTemplateConstant, pushError)
	}
	return nil
}

func remoteBranchExists(executionContext context.Context, executor shared.GitExecutor, repositoryPath string, remoteName string, branchName string) (bool, error) {
	if executor == nil {
		return false, errors.New(gitExecutorMissingMessageConstant)
	}
	result, executionError := executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitLSRemoteSubcommandConstant, gitHeadsFlagConstant, remoteName, strings.TrimSpace(branchName)},
		WorkingDirectory: repositoryPath,
	})
	if executionError != nil {
		return false, executionError
	}
	return strings.TrimSpace(result.StandardOutput) != "", nil
}

func inferRepositoryIdentifier(executionContext context.Context, manager *gitrepo.RepositoryManager, repositoryPath string, remoteName string) (string, bool) {
	if manager == nil {
		return "", false
	}
	remoteURL, remoteURLError := manager.GetRemoteURL(executionContext, repositoryPath, remoteName)
	if remoteURLError != nil {
		return "", false
	}
	identifier, ok := ownerRepoFromRemoteURL(remoteURL)
	if !ok {
		return "", false
	}
	return identifier, true
}

func ownerRepoFromRemoteURL(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "", false
	}
	trimmed = strings.TrimSuffix(trimmed, ".git")
	if strings.Contains(trimmed, "://") {
		parsed, parseError := url.Parse(trimmed)
		if parseError != nil {
			return "", false
		}
		path := strings.TrimPrefix(parsed.Path, "/")
		return ownerRepoFromPath(path)
	}
	colonIndex := strings.Index(trimmed, ":")
	if colonIndex >= 0 {
		trimmed = trimmed[colonIndex+1:]
	}
	trimmed = strings.TrimPrefix(trimmed, "/")
	return ownerRepoFromPath(trimmed)
}

func ownerRepoFromPath(path string) (string, bool) {
	segments := strings.Split(path, "/")
	if len(segments) < 2 {
		return "", false
	}
	owner := strings.TrimSpace(segments[len(segments)-2])
	repository := strings.TrimSpace(segments[len(segments)-1])
	if len(owner) == 0 || len(repository) == 0 {
		return "", false
	}
	return fmt.Sprintf("%s/%s", owner, repository), true
}

func resolveRepositoryIdentifier(repositoryState *RepositoryState) (string, error) {
	if repositoryState == nil {
		return "", errors.New(migrationIdentifierMissingMessageConstant)
	}

	identifierCandidates := []string{
		repositoryState.Inspection.CanonicalOwnerRepo,
		repositoryState.Inspection.FinalOwnerRepo,
		repositoryState.Inspection.OriginOwnerRepo,
	}

	for _, candidate := range identifierCandidates {
		trimmed := strings.TrimSpace(candidate)
		if len(trimmed) > 0 {
			return trimmed, nil
		}
	}

	return "", errors.New(migrationIdentifierMissingMessageConstant)
}
