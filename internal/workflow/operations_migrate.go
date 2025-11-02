package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/gitrepo"
	migrate "github.com/temirov/gix/internal/migrate"
	"github.com/temirov/gix/internal/repos/identity"
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

const (
	defaultInfoPrefix    = "WORKFLOW-DEFAULT"
	defaultWarningPrefix = "WORKFLOW-DEFAULT-WARNING"
	defaultErrorPrefix   = "WORKFLOW-DEFAULT-ERROR"
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
		reportedOwnerRepo := strings.TrimSpace(repositoryState.Inspection.FinalOwnerRepo)

		remoteName := strings.TrimSpace(target.RemoteName)
		if len(remoteName) == 0 {
			remoteName = defaultMigrationRemoteNameConstant
		}

		localDefaultBranch, localDefaultError := resolveLocalDefaultBranch(executionContext, environment.RepositoryManager, repositoryState)
		if localDefaultError != nil {
			return newDefaultError(reportedOwnerRepo, repositoryPath, "unable to determine local default branch", localDefaultError)
		}

	remoteResolution, remoteResolutionError := identity.ResolveRemoteIdentity(
		executionContext,
		identity.RemoteResolutionDependencies{
			RepositoryManager: environment.RepositoryManager,
			GitExecutor:       environment.GitExecutor,
			MetadataResolver:  environment.GitHubClient,
		},
		identity.RemoteResolutionOptions{
			RepositoryPath:            repositoryPath,
			RemoteName:                remoteName,
			ReportedOwnerRepository:   repositoryState.Inspection.FinalOwnerRepo,
			ReportedDefaultBranchName: repositoryState.Inspection.RemoteDefaultBranch,
		},
	)
	if remoteResolutionError != nil {
		if errors.Is(remoteResolutionError, identity.ErrRemoteMetadataUnavailable) {
			return newDefaultError(reportedOwnerRepo, repositoryPath, "remote metadata unavailable", remoteResolutionError)
		}
		return newDefaultError(reportedOwnerRepo, repositoryPath, "unable to resolve remote", remoteResolutionError)
	}

		remoteAvailable := remoteResolution.RemoteDetected && remoteResolution.OwnerRepository != nil
		repositoryIdentifier := ""
		if remoteResolution.OwnerRepository != nil {
			repositoryIdentifier = remoteResolution.OwnerRepository.String()
		}

		remoteDefaultBranch := ""
		if remoteResolution.DefaultBranch != nil {
			remoteDefaultBranch = remoteResolution.DefaultBranch.String()
		}
		if !remoteAvailable {
			remoteDefaultBranch = ""
		}

		effectiveOwner := repositoryIdentifier
		if len(strings.TrimSpace(effectiveOwner)) == 0 {
			effectiveOwner = reportedOwnerRepo
		}

		sourceBranchValue := strings.TrimSpace(target.SourceBranch)
		if len(sourceBranchValue) == 0 {
			if remoteAvailable && len(remoteDefaultBranch) > 0 {
				sourceBranchValue = remoteDefaultBranch
			} else if len(localDefaultBranch) > 0 {
				sourceBranchValue = localDefaultBranch
			}
		}

		if remoteAvailable && len(sourceBranchValue) == 0 {
			if remoteResolution.DefaultBranch == nil {
				return newDefaultError(effectiveOwner, repositoryPath, "default branch source not detected", nil)
			}
			sourceBranchValue = remoteResolution.DefaultBranch.String()
		}

		if len(sourceBranchValue) == 0 {
			return newDefaultError(effectiveOwner, repositoryPath, "default branch source not detected", nil)
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
				fmt.Fprint(environment.Output, formatDefaultWarning(effectiveOwner, repositoryState.Path, fmt.Sprintf("already defaults to %s", targetBranchValue)))
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
			return newDefaultError(effectiveOwner, repositoryPath, fmt.Sprintf("unable to ensure local branch %s", targetBranchValue), ensureLocalError)
		}

		if checkoutError := environment.RepositoryManager.CheckoutBranch(executionContext, repositoryPath, targetBranchValue); checkoutError != nil {
			return newDefaultError(effectiveOwner, repositoryPath, fmt.Sprintf("failed to switch worktree to %s", targetBranchValue), checkoutError)
		}

		if !remoteAvailable {
			if environment.Output != nil {
				fmt.Fprint(environment.Output, formatDefaultWarning(effectiveOwner, repositoryState.Path, "remote unavailable; skipping remote promotion"))
				fmt.Fprint(environment.Output, formatDefaultInfo(effectiveOwner, repositoryState.Path, "%s → %s safe_to_delete=%t", sourceBranchValue, targetBranchValue, false))
			}
			continue
		}

		if target.PushToRemote {
			if ensureRemoteError := ensureRemoteBranch(executionContext, environment.GitExecutor, repositoryPath, remoteName, targetBranchValue); ensureRemoteError != nil {
				return newDefaultError(effectiveOwner, repositoryPath, fmt.Sprintf("unable to ensure remote branch %s on %s", targetBranchValue, remoteName), ensureRemoteError)
			}
		}

		result, executionError := migrationService.Execute(executionContext, options)
		if executionError != nil {
			return newDefaultError(effectiveOwner, repositoryPath, "default branch promotion failed", executionError)
		}

		if environment.Output != nil {
			fmt.Fprint(environment.Output, formatDefaultInfo(effectiveOwner, repositoryState.Path, "%s → %s safe_to_delete=%t", sourceBranchValue, targetBranchValue, result.SafetyStatus.SafeToDelete))
			for _, warning := range result.Warnings {
				fmt.Fprintln(environment.Output, warning)
			}
		}

		if environment.AuditService != nil {
			if refreshError := repositoryState.Refresh(executionContext, environment.AuditService); refreshError != nil {
				return newDefaultError(effectiveOwner, repositoryPath, "failed to refresh repository metadata", refreshError)
			}
		}
	}

	return nil
}

func formatDefaultDescriptor(ownerRepo string, repositoryPath string) string {
	trimmedOwner := strings.TrimSpace(ownerRepo)
	trimmedPath := strings.TrimSpace(repositoryPath)
	switch {
	case len(trimmedOwner) > 0 && len(trimmedPath) > 0:
		return fmt.Sprintf("%s (%s)", trimmedOwner, trimmedPath)
	case len(trimmedOwner) > 0:
		return trimmedOwner
	case len(trimmedPath) > 0:
		return trimmedPath
	default:
		return "<unknown repository>"
	}
}

func formatDefaultWarning(ownerRepo string, repositoryPath string, message string) string {
	return fmt.Sprintf("%s: %s %s\n", defaultWarningPrefix, formatDefaultDescriptor(ownerRepo, repositoryPath), message)
}

func formatDefaultInfo(ownerRepo string, repositoryPath string, format string, arguments ...any) string {
	descriptor := formatDefaultDescriptor(ownerRepo, repositoryPath)
	return fmt.Sprintf("%s: %s %s\n", defaultInfoPrefix, descriptor, fmt.Sprintf(format, arguments...))
}

func newDefaultError(ownerRepo string, repositoryPath string, message string, cause error) error {
	descriptor := formatDefaultDescriptor(ownerRepo, repositoryPath)
	if cause == nil {
		return fmt.Errorf("%s: %s %s", defaultErrorPrefix, descriptor, message)
	}
	return fmt.Errorf("%s: %s %s: %w", defaultErrorPrefix, descriptor, message, cause)
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
		if shouldIgnoreRemotePushError(pushError) {
			return nil
		}
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

func shouldIgnoreDefaultBranchUpdateError(updateError migrate.DefaultBranchUpdateError) bool {
	if updateError.Cause == nil {
		return false
	}
	lowered := strings.ToLower(updateError.Cause.Error())
	if strings.Contains(lowered, "http 404") {
		return true
	}
	if strings.Contains(lowered, "repository not found") {
		return true
	}
	if strings.Contains(lowered, "could not resolve to a repository") {
		return true
	}
	if strings.Contains(lowered, "could not read from remote repository") {
		return true
	}
	return false
}
