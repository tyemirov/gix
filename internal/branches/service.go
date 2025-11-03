package branches

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	defaultRemoteNameConstant                    = "origin"
	defaultPullRequestLimitConstant              = 100
	lsRemoteSubcommandConstant                   = "ls-remote"
	headsFlagConstant                            = "--heads"
	pushSubcommandConstant                       = "push"
	deleteFlagConstant                           = "--delete"
	branchSubcommandConstant                     = "branch"
	forceDeleteFlagConstant                      = "-D"
	pullRequestSubcommandConstant                = "pr"
	listSubcommandConstant                       = "list"
	stateFlagConstant                            = "--state"
	closedStateConstant                          = "closed"
	jsonFlagConstant                             = "--json"
	headRefFieldConstant                         = "headRefName"
	limitFlagConstant                            = "--limit"
	branchReferencePrefixConstant                = "refs/heads/"
	logMessageListingRemoteBranchesConstant      = "Listing remote branches"
	logMessageListingPullRequestsConstant        = "Listing closed pull request branches"
	logMessageDeletingRemoteBranchConstant       = "Deleting remote branch"
	logMessageSkippingRemoteBranchDryRunConstant = "Skipping remote branch deletion (dry run)"
	logMessageSkippingMissingBranchConstant      = "Skipping branch (already gone)"
	logMessageDeletingLocalBranchConstant        = "Deleting local branch"
	logMessageSkippingLocalBranchDryRunConstant  = "Skipping local branch deletion (dry run)"
	logMessageRemoteDeletionFailedConstant       = "Remote branch deletion failed"
	logMessageLocalDeletionFailedConstant        = "Local branch deletion failed"
	logMessageDeletionSkippedByUserConstant      = "Skipping branch deletion (user declined)"
	logMessageDeletionPromptFailedConstant       = "Branch deletion confirmation failed"
	logFieldBranchNameConstant                   = "branch"
	logFieldRemoteNameConstant                   = "remote"
	logFieldDryRunConstant                       = "dry_run"
	logFieldWorkingDirectoryConstant             = "working_directory"
	logFieldErrorConstant                        = "error"
	logFieldPullRequestLimitConstant             = "pull_request_limit"
	remoteBranchesListErrorTemplateConstant      = "unable to list remote branches: %w"
	pullRequestListErrorTemplateConstant         = "unable to list closed pull requests: %w"
	remoteBranchParsingErrorTemplateConstant     = "unable to parse remote branch list: %w"
	pullRequestDecodingErrorTemplateConstant     = "unable to decode pull request response: %w"
	remoteNameRequiredMessageConstant            = "remote name must be provided"
	limitPositiveRequirementMessageConstant      = "pull request limit must be greater than zero"
	executorNotConfiguredMessageConstant         = "command executor not configured"
	branchDeletionPromptTemplateConstant         = "Delete pull request branch '%s' from remote '%s' and the local repository? [y/N] "
)

// CommandExecutor coordinates git and GitHub CLI invocations required for cleanup.
type CommandExecutor interface {
	ExecuteGit(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error)
	ExecuteGitHubCLI(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error)
}

// CleanupOptions describe the behavior of the branch cleanup routine.
type CleanupOptions struct {
	RemoteName       string
	PullRequestLimit int
	DryRun           bool
	WorkingDirectory string
	AssumeYes        bool
}

// Service orchestrates removal of remote and local branches tied to closed pull requests.
type Service struct {
	logger   *zap.Logger
	executor CommandExecutor
	prompter shared.ConfirmationPrompter
}

var (
	errRemoteNameRequired    = errors.New(remoteNameRequiredMessageConstant)
	errLimitMustBePositive   = errors.New(limitPositiveRequirementMessageConstant)
	errExecutorNotConfigured = errors.New(executorNotConfiguredMessageConstant)
)

// NewService constructs a Service instance.
func NewService(logger *zap.Logger, executor CommandExecutor, prompter shared.ConfirmationPrompter) (*Service, error) {
	if executor == nil {
		return nil, errExecutorNotConfigured
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	return &Service{logger: logger, executor: executor, prompter: prompter}, nil
}

// Cleanup removes stale branches based on closed pull requests.
func (service *Service) Cleanup(executionContext context.Context, options CleanupOptions) error {
	trimmedRemoteName := strings.TrimSpace(options.RemoteName)
	if len(trimmedRemoteName) == 0 {
		return errRemoteNameRequired
	}

	if options.PullRequestLimit <= 0 {
		return errLimitMustBePositive
	}

	remoteBranches, remoteBranchesError := service.fetchRemoteBranches(executionContext, trimmedRemoteName, options.WorkingDirectory)
	if remoteBranchesError != nil {
		return fmt.Errorf(remoteBranchesListErrorTemplateConstant, remoteBranchesError)
	}

	closedBranches, pullRequestsError := service.fetchClosedPullRequestBranches(executionContext, options.PullRequestLimit, options.WorkingDirectory)
	if pullRequestsError != nil {
		return fmt.Errorf(pullRequestListErrorTemplateConstant, pullRequestsError)
	}

	confirmation := newBranchDeletionConfirmation(service.prompter, options.AssumeYes)
	service.processBranches(executionContext, trimmedRemoteName, remoteBranches, closedBranches, confirmation, options)

	return nil
}

func (service *Service) fetchRemoteBranches(executionContext context.Context, remoteName string, workingDirectory string) (map[string]struct{}, error) {
	service.logger.Info(logMessageListingRemoteBranchesConstant,
		zap.String(logFieldRemoteNameConstant, remoteName),
		zap.String(logFieldWorkingDirectoryConstant, workingDirectory),
	)

	commandDetails := execshell.CommandDetails{
		Arguments:        []string{lsRemoteSubcommandConstant, headsFlagConstant, remoteName},
		WorkingDirectory: workingDirectory,
	}

	executionResult, executionError := service.executor.ExecuteGit(executionContext, commandDetails)
	if executionError != nil {
		return nil, executionError
	}

	branchSet, parsingError := parseRemoteBranches(executionResult.StandardOutput)
	if parsingError != nil {
		return nil, parsingError
	}

	return branchSet, nil
}

func (service *Service) fetchClosedPullRequestBranches(executionContext context.Context, limit int, workingDirectory string) ([]string, error) {
	service.logger.Info(logMessageListingPullRequestsConstant,
		zap.Int(logFieldPullRequestLimitConstant, limit),
		zap.String(logFieldWorkingDirectoryConstant, workingDirectory),
	)

	limitArgument := strconv.Itoa(limit)

	commandDetails := execshell.CommandDetails{
		Arguments: []string{
			pullRequestSubcommandConstant,
			listSubcommandConstant,
			stateFlagConstant,
			closedStateConstant,
			jsonFlagConstant,
			headRefFieldConstant,
			limitFlagConstant,
			limitArgument,
		},
		WorkingDirectory: workingDirectory,
	}

	executionResult, executionError := service.executor.ExecuteGitHubCLI(executionContext, commandDetails)
	if executionError != nil {
		return nil, executionError
	}

	pullRequestBranches, decodingError := decodePullRequestBranches(executionResult.StandardOutput)
	if decodingError != nil {
		return nil, decodingError
	}

	return pullRequestBranches, nil
}

func (service *Service) processBranches(executionContext context.Context, remoteName string, remoteBranches map[string]struct{}, pullRequestBranches []string, confirmation *branchDeletionConfirmation, options CleanupOptions) {
	processedBranches := make(map[string]struct{})
	for branchIndex := range pullRequestBranches {
		branchName := strings.TrimSpace(pullRequestBranches[branchIndex])
		if len(branchName) == 0 {
			continue
		}

		if _, alreadyProcessed := processedBranches[branchName]; alreadyProcessed {
			continue
		}
		processedBranches[branchName] = struct{}{}

		if _, existsInRemote := remoteBranches[branchName]; existsInRemote {
			service.deleteRemoteAndLocalBranch(executionContext, remoteName, branchName, confirmation, options)
			continue
		}

		service.logger.Info(logMessageSkippingMissingBranchConstant,
			zap.String(logFieldBranchNameConstant, branchName),
			zap.String(logFieldRemoteNameConstant, remoteName),
			zap.String(logFieldWorkingDirectoryConstant, options.WorkingDirectory),
		)
	}
}

func (service *Service) deleteRemoteAndLocalBranch(executionContext context.Context, remoteName string, branchName string, confirmation *branchDeletionConfirmation, options CleanupOptions) {
	baseFields := []zap.Field{
		zap.String(logFieldBranchNameConstant, branchName),
		zap.String(logFieldRemoteNameConstant, remoteName),
		zap.String(logFieldWorkingDirectoryConstant, options.WorkingDirectory),
	}

	if options.DryRun {
		service.logger.Info(logMessageSkippingRemoteBranchDryRunConstant,
			append(baseFields, zap.Bool(logFieldDryRunConstant, true))...,
		)
		service.logger.Info(logMessageSkippingLocalBranchDryRunConstant,
			append(baseFields, zap.Bool(logFieldDryRunConstant, true))...,
		)
		return
	}

	if confirmation != nil {
		allowed, confirmationError := confirmation.Confirm(branchName, remoteName)
		if confirmationError != nil {
			service.logger.Warn(logMessageDeletionPromptFailedConstant,
				append(baseFields, zap.Error(confirmationError))...,
			)
			return
		}
		if !allowed {
			service.logger.Info(logMessageDeletionSkippedByUserConstant, baseFields...)
			return
		}
	}

	service.logger.Info(logMessageDeletingRemoteBranchConstant, baseFields...)
	pushCommandDetails := execshell.CommandDetails{
		Arguments: []string{
			pushSubcommandConstant,
			remoteName,
			deleteFlagConstant,
			branchName,
		},
		WorkingDirectory: options.WorkingDirectory,
	}

	if _, pushError := service.executor.ExecuteGit(executionContext, pushCommandDetails); pushError != nil {
		service.logger.Warn(logMessageRemoteDeletionFailedConstant,
			append(baseFields, zap.Error(pushError))...,
		)
	}

	service.logger.Info(logMessageDeletingLocalBranchConstant, baseFields...)
	deleteLocalCommand := execshell.CommandDetails{
		Arguments: []string{
			branchSubcommandConstant,
			forceDeleteFlagConstant,
			branchName,
		},
		WorkingDirectory: options.WorkingDirectory,
	}

	if _, deleteError := service.executor.ExecuteGit(executionContext, deleteLocalCommand); deleteError != nil {
		service.logger.Warn(logMessageLocalDeletionFailedConstant,
			append(baseFields, zap.Error(deleteError))...,
		)
	}
}

func parseRemoteBranches(commandOutput string) (map[string]struct{}, error) {
	branchSet := make(map[string]struct{})
	scanner := bufio.NewScanner(strings.NewReader(commandOutput))
	for scanner.Scan() {
		lineText := scanner.Text()
		lineParts := strings.Fields(lineText)
		if len(lineParts) < 2 {
			continue
		}
		referenceName := lineParts[1]
		branchName := strings.TrimPrefix(referenceName, branchReferencePrefixConstant)
		branchName = strings.TrimSpace(branchName)
		if len(branchName) == 0 {
			continue
		}
		branchSet[branchName] = struct{}{}
	}

	if scanError := scanner.Err(); scanError != nil {
		return nil, fmt.Errorf(remoteBranchParsingErrorTemplateConstant, scanError)
	}

	return branchSet, nil
}

func decodePullRequestBranches(standardOutput string) ([]string, error) {
	type pullRequestPayload struct {
		HeadRefName string `json:"headRefName"`
	}

	trimmedOutput := strings.TrimSpace(standardOutput)
	if len(trimmedOutput) == 0 {
		return []string{}, nil
	}

	var payload []pullRequestPayload
	if decodeError := json.Unmarshal([]byte(trimmedOutput), &payload); decodeError != nil {
		return nil, fmt.Errorf(pullRequestDecodingErrorTemplateConstant, decodeError)
	}

	branches := make([]string, 0, len(payload))
	for payloadIndex := range payload {
		branches = append(branches, payload[payloadIndex].HeadRefName)
	}
	return branches, nil
}

type branchDeletionConfirmation struct {
	prompter   shared.ConfirmationPrompter
	assumeYes  bool
	confirmAll bool
}

func newBranchDeletionConfirmation(prompter shared.ConfirmationPrompter, assumeYes bool) *branchDeletionConfirmation {
	return &branchDeletionConfirmation{prompter: prompter, assumeYes: assumeYes}
}

func (confirmation *branchDeletionConfirmation) Confirm(branchName string, remoteName string) (bool, error) {
	if confirmation == nil || confirmation.assumeYes || confirmation.confirmAll || confirmation.prompter == nil {
		return true, nil
	}

	prompt := fmt.Sprintf(branchDeletionPromptTemplateConstant, branchName, remoteName)
	result, promptError := confirmation.prompter.Confirm(prompt)
	if promptError != nil {
		return false, promptError
	}
	if result.ApplyToAll {
		confirmation.confirmAll = true
	}
	return result.Confirmed, nil
}
