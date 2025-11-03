package migrate

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubauth"
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/utils"
)

const (
	repositoryPathFieldNameConstant                 = "repository_path"
	remoteNameFieldNameConstant                     = "remote_name"
	repositoryIdentifierFieldNameConstant           = "repository_identifier"
	workflowsDirectoryFieldNameConstant             = "workflows_directory"
	sourceBranchFieldNameConstant                   = "source_branch"
	targetBranchFieldNameConstant                   = "target_branch"
	gitAddCommandNameConstant                       = "add"
	gitAllFlagConstant                              = "-A"
	gitCommitCommandNameConstant                    = "commit"
	gitMessageFlagConstant                          = "-m"
	gitPushCommandNameConstant                      = "push"
	gitBranchCommandNameConstant                    = "branch"
	gitDeleteForceFlagConstant                      = "-D"
	gitPushDeleteFlagConstant                       = "--delete"
	workflowCommitMessageTemplateConstant           = "CI: switch workflow branch filters to %s"
	cleanWorktreeRequiredMessageConstant            = "repository worktree must be clean before migration"
	repositoryManagerMissingMessageConstant         = "repository manager not configured"
	githubClientMissingMessageConstant              = "GitHub client not configured"
	gitExecutorMissingMessageConstant               = "git executor not configured"
	workflowRewriteErrorTemplateConstant            = "workflow rewrite failed: %w"
	workflowStageErrorTemplateConstant              = "unable to stage workflow updates: %w"
	workflowCommitErrorTemplateConstant             = "unable to commit workflow updates: %w"
	workflowPushErrorTemplateConstant               = "unable to push workflow updates: %w"
	pagesUpdateErrorTemplateConstant                = "GitHub Pages update failed: %w"
	pagesUpdateWarningMessageConstant               = "GitHub Pages update skipped"
	pagesUpdateWarningTemplateConstant              = "PAGES-SKIP: %s (%s)"
	defaultBranchUpdateErrorMessageTemplateConstant = "DEFAULT-BRANCH-UPDATE repository=%s path=%s source=%s target=%s"
	pullRequestListErrorTemplateConstant            = "unable to list pull requests: %w"
	pullRequestListWarningTemplateConstant          = "PR-LIST-SKIP: %s (%s)"
	pullRequestRetargetErrorTemplateConstant        = "unable to retarget pull request #%d: %w"
	pullRequestRetargetWarningTemplateConstant      = "PR-RETARGET-SKIP: #%d (%s)"
	branchProtectionCheckErrorTemplateConstant      = "unable to determine branch protection: %w"
	branchProtectionWarningTemplateConstant         = "PROTECTION-SKIP: %s"
	localBranchDeleteErrorTemplateConstant          = "unable to delete local source branch: %w"
	remoteBranchDeleteErrorTemplateConstant         = "unable to delete remote source branch: %w"
	branchDeletionWarningTemplateConstant           = "DELETE-SKIP: %s"
	branchDeletionSkippedMessageConstant            = "Skipping source branch deletion because safety gates blocked deletion"
)

// InvalidInputError describes migration option validation failures.
type InvalidInputError struct {
	FieldName string
	Message   string
}

// Error describes the invalid input.
func (inputError InvalidInputError) Error() string {
	return fmt.Sprintf("%s: %s", inputError.FieldName, inputError.Message)
}

// ServiceDependencies describes required collaborators for migration.
type ServiceDependencies struct {
	Logger            *zap.Logger
	RepositoryManager *gitrepo.RepositoryManager
	GitHubClient      GitHubOperations
	GitExecutor       CommandExecutor
}

// MigrationOptions configures the migrate workflow.
type MigrationOptions struct {
	RepositoryPath       string
	RepositoryRemoteName string
	RepositoryIdentifier string
	WorkflowsDirectory   string
	SourceBranch         BranchName
	TargetBranch         BranchName
	PushUpdates          bool
	EnableDebugLogging   bool
	DeleteSourceBranch   bool
}

// WorkflowOutcome captures workflow rewrite results.
type WorkflowOutcome struct {
	UpdatedFiles            []string
	RemainingMainReferences bool
}

// MigrationResult captures the observable outcomes.
type MigrationResult struct {
	WorkflowOutcome           WorkflowOutcome
	PagesConfigurationUpdated bool
	DefaultBranchUpdated      bool
	RetargetedPullRequests    []int
	SafetyStatus              SafetyStatus
	Warnings                  []string
}

// DefaultBranchUpdateError describes default-branch update failures with context.
type DefaultBranchUpdateError struct {
	RepositoryPath       string
	RepositoryIdentifier string
	SourceBranch         BranchName
	TargetBranch         BranchName
	Cause                error
}

// Error describes the contextual failure.
func (updateError DefaultBranchUpdateError) Error() string {
	context := fmt.Sprintf(
		defaultBranchUpdateErrorMessageTemplateConstant,
		updateError.RepositoryIdentifier,
		updateError.RepositoryPath,
		string(updateError.SourceBranch),
		string(updateError.TargetBranch),
	)
	if updateError.Cause == nil {
		return context
	}
	summary := summarizeCommandError(updateError.Cause)
	if len(summary) == 0 {
		summary = updateError.Cause.Error()
	}
	return fmt.Sprintf("%s: %s", context, summary)
}

// Unwrap exposes the underlying cause.
func (updateError DefaultBranchUpdateError) Unwrap() error {
	return updateError.Cause
}

// Service orchestrates the branch migration workflow.
type Service struct {
	logger            *zap.Logger
	repositoryManager *gitrepo.RepositoryManager
	gitHubClient      GitHubOperations
	gitExecutor       CommandExecutor
	workflowRewriter  *WorkflowRewriter
	pagesManager      *PagesManager
	safetyEvaluator   SafetyEvaluator
	warnings          []string
}

var _ MigrationExecutor = (*Service)(nil)

var (
	errRepositoryManagerMissing = errors.New(repositoryManagerMissingMessageConstant)
	errGitHubClientMissing      = errors.New(githubClientMissingMessageConstant)
	errGitExecutorMissing       = errors.New(gitExecutorMissingMessageConstant)
	errCleanWorktreeRequired    = errors.New(cleanWorktreeRequiredMessageConstant)
)

// NewService constructs a Service with the provided dependencies.
func NewService(dependencies ServiceDependencies) (*Service, error) {
	if dependencies.RepositoryManager == nil {
		return nil, errRepositoryManagerMissing
	}
	if dependencies.GitHubClient == nil {
		return nil, errGitHubClientMissing
	}
	if dependencies.GitExecutor == nil {
		return nil, errGitExecutorMissing
	}

	logger := dependencies.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	workflowRewriter := NewWorkflowRewriter(logger)
	pagesManager := NewPagesManager(logger, dependencies.GitHubClient)

	service := &Service{
		logger:            logger,
		repositoryManager: dependencies.RepositoryManager,
		gitHubClient:      dependencies.GitHubClient,
		gitExecutor:       dependencies.GitExecutor,
		workflowRewriter:  workflowRewriter,
		pagesManager:      pagesManager,
		safetyEvaluator:   SafetyEvaluator{},
	}

	return service, nil
}

func (service *Service) ensureGitHubTokenAvailable(options MigrationOptions) error {
	if len(strings.TrimSpace(options.RepositoryIdentifier)) == 0 {
		return nil
	}
	if _, available := githubauth.ResolveToken(nil); available {
		return nil
	}
	return DefaultBranchUpdateError{
		RepositoryPath:       options.RepositoryPath,
		RepositoryIdentifier: options.RepositoryIdentifier,
		SourceBranch:         options.SourceBranch,
		TargetBranch:         options.TargetBranch,
		Cause:                githubauth.NewMissingTokenError("default-branch", true),
	}
}

// Execute performs the migration workflow.
func (service *Service) Execute(executionContext context.Context, options MigrationOptions) (MigrationResult, error) {
	if validationError := service.validateOptions(options); validationError != nil {
		return MigrationResult{}, validationError
	}

	requireClean := true
	contextAccessor := utils.NewCommandContextAccessor()
	if branchContext, exists := contextAccessor.BranchContext(executionContext); exists {
		requireClean = branchContext.RequireClean
	}

	if requireClean {
		cleanWorktree, cleanError := service.repositoryManager.CheckCleanWorktree(executionContext, options.RepositoryPath)
		if cleanError != nil {
			return MigrationResult{}, cleanError
		}
		if !cleanWorktree {
			return MigrationResult{}, errCleanWorktreeRequired
		}
	}

	if tokenError := service.ensureGitHubTokenAvailable(options); tokenError != nil {
		return MigrationResult{}, tokenError
	}

	workflowOutcome, rewriteError := service.workflowRewriter.Rewrite(executionContext, WorkflowRewriteConfig{
		RepositoryPath:     options.RepositoryPath,
		WorkflowsDirectory: options.WorkflowsDirectory,
		SourceBranch:       options.SourceBranch,
		TargetBranch:       options.TargetBranch,
	})
	if rewriteError != nil {
		return MigrationResult{}, fmt.Errorf(workflowRewriteErrorTemplateConstant, rewriteError)
	}

	workflowCommitted, workflowCommitError := service.commitWorkflowChanges(executionContext, options, workflowOutcome)
	if workflowCommitError != nil {
		return MigrationResult{}, workflowCommitError
	}
	service.warnings = service.warnings[:0]

	if workflowCommitted && options.PushUpdates {
		if pushError := service.pushWorkflowChanges(executionContext, options); pushError != nil {
			return MigrationResult{}, pushError
		}
	}

	remoteOperationsEnabled := len(strings.TrimSpace(options.RepositoryIdentifier)) > 0
	pagesUpdated := false
	if remoteOperationsEnabled {
		var pagesError error
		pagesUpdated, pagesError = service.pagesManager.EnsureLegacyBranch(executionContext, PagesUpdateConfig{
			RepositoryIdentifier: options.RepositoryIdentifier,
			SourceBranch:         options.SourceBranch,
			TargetBranch:         options.TargetBranch,
		})
		if pagesError != nil {
			if isNonCriticalPagesError(pagesError) {
				service.logger.Warn(
					pagesUpdateWarningMessageConstant,
					zap.String(repositoryPathFieldNameConstant, options.RepositoryPath),
					zap.String(repositoryIdentifierFieldNameConstant, options.RepositoryIdentifier),
					zap.Error(pagesError),
				)
				warning := fmt.Sprintf(pagesUpdateWarningTemplateConstant, options.RepositoryIdentifier, summarizeCommandError(pagesError))
				service.warnings = append(service.warnings, warning)
				pagesUpdated = false
			} else {
				return MigrationResult{}, fmt.Errorf(pagesUpdateErrorTemplateConstant, pagesError)
			}
		}
	}

	defaultBranchUpdated := false
	if remoteOperationsEnabled {
		if err := service.gitHubClient.SetDefaultBranch(executionContext, options.RepositoryIdentifier, string(options.TargetBranch)); err != nil {
			if isMissingRemoteRepositoryError(err) {
				remoteOperationsEnabled = false
			} else {
				return MigrationResult{}, DefaultBranchUpdateError{
					RepositoryPath:       options.RepositoryPath,
					RepositoryIdentifier: options.RepositoryIdentifier,
					SourceBranch:         options.SourceBranch,
					TargetBranch:         options.TargetBranch,
					Cause:                err,
				}
			}
		} else {
			defaultBranchUpdated = true
		}
	}

	pullRequests := []githubcli.PullRequest{}
	if remoteOperationsEnabled {
		var listError error
		pullRequests, listError = service.gitHubClient.ListPullRequests(executionContext, options.RepositoryIdentifier, githubcli.PullRequestListOptions{
			State:       githubcli.PullRequestStateOpen,
			BaseBranch:  string(options.SourceBranch),
			ResultLimit: defaultPullRequestQueryLimit,
		})
		if listError != nil {
			service.logger.Warn(
				"Pull request listing failed",
				zap.String(repositoryIdentifierFieldNameConstant, options.RepositoryIdentifier),
				zap.Error(listError),
			)
			warning := fmt.Sprintf(pullRequestListWarningTemplateConstant, options.RepositoryIdentifier, summarizeCommandError(listError))
			service.warnings = append(service.warnings, warning)
			pullRequests = []githubcli.PullRequest{}
			if isMissingRemoteRepositoryError(listError) {
				remoteOperationsEnabled = false
			}
		}
	}

	retargeted := make([]int, 0)
	if remoteOperationsEnabled {
		var retargetWarnings []string
		retargeted, retargetWarnings = service.retargetPullRequests(executionContext, options, pullRequests)
		service.warnings = append(service.warnings, retargetWarnings...)
	}

	branchProtected := true
	if remoteOperationsEnabled {
		var protectionError error
		branchProtected, protectionError = service.gitHubClient.CheckBranchProtection(executionContext, options.RepositoryIdentifier, string(options.SourceBranch))
		if protectionError != nil {
			service.logger.Warn(
				"Branch protection check failed",
				zap.String(repositoryIdentifierFieldNameConstant, options.RepositoryIdentifier),
				zap.Error(protectionError),
			)
			warning := fmt.Sprintf(branchProtectionWarningTemplateConstant, summarizeCommandError(protectionError))
			service.warnings = append(service.warnings, warning)
			branchProtected = true
		}
	}

	safetyStatus := service.safetyEvaluator.Evaluate(SafetyInputs{
		OpenPullRequestCount: len(pullRequests),
		BranchProtected:      branchProtected,
		WorkflowMentions:     workflowOutcome.RemainingMainReferences,
	})

	result := MigrationResult{
		WorkflowOutcome:           workflowOutcome,
		PagesConfigurationUpdated: pagesUpdated,
		DefaultBranchUpdated:      defaultBranchUpdated,
		RetargetedPullRequests:    retargeted,
		SafetyStatus:              safetyStatus,
		Warnings:                  append([]string(nil), service.warnings...),
	}

	if options.DeleteSourceBranch {
		if !result.SafetyStatus.SafeToDelete {
			service.logger.Warn(
				branchDeletionSkippedMessageConstant,
				zap.String(repositoryPathFieldNameConstant, options.RepositoryPath),
				zap.String(sourceBranchFieldNameConstant, string(options.SourceBranch)),
			)
		} else {
			if deletionError := service.deleteSourceBranch(executionContext, options); deletionError != nil {
				service.logger.Warn(
					"Source branch deletion failed",
					zap.String(repositoryPathFieldNameConstant, options.RepositoryPath),
					zap.Error(deletionError),
				)
				warning := fmt.Sprintf(branchDeletionWarningTemplateConstant, summarizeCommandError(deletionError))
				result.Warnings = append(result.Warnings, warning)
			}
		}
	}

	return result, nil
}

func isNonCriticalPagesError(err error) bool {
	var operationError githubcli.OperationError
	if errors.As(err, &operationError) {
		return true
	}
	var decodingError githubcli.ResponseDecodingError
	if errors.As(err, &decodingError) {
		return true
	}
	var missingToken githubauth.MissingTokenError
	return errors.As(err, &missingToken) && !missingToken.CriticalRequirement()
}

func (service *Service) validateOptions(options MigrationOptions) error {
	if len(strings.TrimSpace(options.RepositoryPath)) == 0 {
		return InvalidInputError{FieldName: repositoryPathFieldNameConstant, Message: requiredValueMessageConstant}
	}
	if len(strings.TrimSpace(options.RepositoryRemoteName)) == 0 {
		return InvalidInputError{FieldName: remoteNameFieldNameConstant, Message: requiredValueMessageConstant}
	}
	if len(strings.TrimSpace(options.WorkflowsDirectory)) == 0 {
		return InvalidInputError{FieldName: workflowsDirectoryFieldNameConstant, Message: requiredValueMessageConstant}
	}
	if len(strings.TrimSpace(string(options.SourceBranch))) == 0 {
		return InvalidInputError{FieldName: sourceBranchFieldNameConstant, Message: requiredValueMessageConstant}
	}
	if len(strings.TrimSpace(string(options.TargetBranch))) == 0 {
		return InvalidInputError{FieldName: targetBranchFieldNameConstant, Message: requiredValueMessageConstant}
	}
	return nil
}

func (service *Service) commitWorkflowChanges(executionContext context.Context, options MigrationOptions, outcome WorkflowOutcome) (bool, error) {
	if len(outcome.UpdatedFiles) == 0 {
		return false, nil
	}

	addArguments := []string{gitAddCommandNameConstant, gitAllFlagConstant, options.WorkflowsDirectory}
	if _, stageError := service.gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        addArguments,
		WorkingDirectory: options.RepositoryPath,
	}); stageError != nil {
		return false, fmt.Errorf(workflowStageErrorTemplateConstant, stageError)
	}

	commitMessage := fmt.Sprintf(workflowCommitMessageTemplateConstant, string(options.TargetBranch))
	_, commitError := service.gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitCommitCommandNameConstant, gitMessageFlagConstant, commitMessage},
		WorkingDirectory: options.RepositoryPath,
	})
	if commitError != nil {
		var commandFailure execshell.CommandFailedError
		if errors.As(commitError, &commandFailure) {
			service.logger.Info("No workflow changes to commit", zap.String(workflowsDirectoryFieldNameConstant, options.WorkflowsDirectory))
			return false, nil
		}
		return false, fmt.Errorf(workflowCommitErrorTemplateConstant, commitError)
	}

	return true, nil
}

func (service *Service) pushWorkflowChanges(executionContext context.Context, options MigrationOptions) error {
	pushArguments := []string{gitPushCommandNameConstant, options.RepositoryRemoteName, string(options.TargetBranch)}
	if _, pushError := service.gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        pushArguments,
		WorkingDirectory: options.RepositoryPath,
	}); pushError != nil {
		return fmt.Errorf(workflowPushErrorTemplateConstant, pushError)
	}
	return nil
}

func (service *Service) deleteSourceBranch(executionContext context.Context, options MigrationOptions) error {
	deleteLocalArguments := []string{gitBranchCommandNameConstant, gitDeleteForceFlagConstant, string(options.SourceBranch)}
	if _, deleteLocalError := service.gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        deleteLocalArguments,
		WorkingDirectory: options.RepositoryPath,
	}); deleteLocalError != nil {
		return fmt.Errorf(localBranchDeleteErrorTemplateConstant, deleteLocalError)
	}

	deleteRemoteArguments := []string{gitPushCommandNameConstant, options.RepositoryRemoteName, gitPushDeleteFlagConstant, string(options.SourceBranch)}
	if _, deleteRemoteError := service.gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        deleteRemoteArguments,
		WorkingDirectory: options.RepositoryPath,
	}); deleteRemoteError != nil {
		return fmt.Errorf(remoteBranchDeleteErrorTemplateConstant, deleteRemoteError)
	}

	return nil
}

func (service *Service) retargetPullRequests(executionContext context.Context, options MigrationOptions, pullRequests []githubcli.PullRequest) ([]int, []string) {
	retargeted := make([]int, 0, len(pullRequests))
	warnings := make([]string, 0)
	for _, pullRequest := range pullRequests {
		retargetError := service.gitHubClient.UpdatePullRequestBase(executionContext, options.RepositoryIdentifier, pullRequest.Number, string(options.TargetBranch))
		if retargetError != nil {
			warning := fmt.Sprintf(pullRequestRetargetWarningTemplateConstant, pullRequest.Number, summarizeCommandError(retargetError))
			warnings = append(warnings, warning)
			service.logger.Warn(
				"Pull request retarget failed",
				zap.Int("pull_request", pullRequest.Number),
				zap.String(repositoryIdentifierFieldNameConstant, options.RepositoryIdentifier),
				zap.Error(retargetError),
			)
			continue
		}
		retargeted = append(retargeted, pullRequest.Number)
	}
	return retargeted, warnings
}

func summarizeCommandError(err error) string {
	var missingToken githubauth.MissingTokenError
	if errors.As(err, &missingToken) {
		return missingToken.Error()
	}
	var commandFailure execshell.CommandFailedError
	if errors.As(err, &commandFailure) {
		trimmed := strings.TrimSpace(commandFailure.Result.StandardError)
		if len(trimmed) > 0 {
			return trimmed
		}
		return commandFailure.Error()
	}
	return strings.TrimSpace(err.Error())
}

func isMissingRemoteRepositoryError(err error) bool {
	var operationError githubcli.OperationError
	if errors.As(err, &operationError) && operationError.Cause != nil {
		err = operationError.Cause
	}
	var commandFailure execshell.CommandFailedError
	if !errors.As(err, &commandFailure) {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(commandFailure.Result.StandardError))
	if len(normalized) == 0 {
		normalized = strings.ToLower(strings.TrimSpace(commandFailure.Error()))
	}
	if len(normalized) == 0 {
		return false
	}
	if strings.Contains(normalized, "http 404") {
		return true
	}
	if strings.Contains(normalized, "repository") && strings.Contains(normalized, "not found") {
		return true
	}
	return false
}

const defaultPullRequestQueryLimit = 100

// WorkflowRewriteConfig describes the workflow rewrite inputs.
type WorkflowRewriteConfig struct {
	RepositoryPath     string
	WorkflowsDirectory string
	SourceBranch       BranchName
	TargetBranch       BranchName
}

// PagesUpdateConfig describes GitHub Pages update inputs.
type PagesUpdateConfig struct {
	RepositoryIdentifier string
	SourceBranch         BranchName
	TargetBranch         BranchName
}
