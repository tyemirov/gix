package cd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	repositoryPathRequiredMessageConstant    = "repository path must be provided"
	branchNameRequiredMessageConstant        = "branch name must be provided"
	gitExecutorMissingMessageConstant        = "git executor not configured"
	gitFetchFailureTemplateConstant          = "failed to fetch updates: %w"
	gitRemoteListFailureTemplateConstant     = "failed to list remotes: %w"
	gitSwitchFailureTemplateConstant         = "failed to switch to branch %q: %s: %w"
	gitCreateBranchFromRemoteFailureTemplate = "failed to create branch %q from %s: %s: %w"
	gitCreateBranchLocalFailureTemplate      = "failed to create branch %q: %s: %w"
	gitPullFailureTemplateConstant           = "failed to pull latest changes: %w"
	defaultRemoteNameConstant                = shared.OriginRemoteNameConstant
	logFieldRemoteNameConstant               = "remote"
	logFieldRepositoryPathConstant           = "repository_path"
	fetchWarningLogMessageConstant           = "Fetch skipped due to error"
	pullWarningLogMessageConstant            = "Pull skipped due to error"
	fetchWarningTemplateConstant             = "FETCH-SKIP: %s (%s)"
	pullWarningTemplateConstant              = "PULL-SKIP: %s"
	missingRemoteWarningTemplateConstant     = "WARNING: no remote counterpart for %s"
	gitFetchSubcommandConstant               = "fetch"
	gitFetchAllFlagConstant                  = "--all"
	gitFetchPruneFlagConstant                = "--prune"
	gitRemoteSubcommandConstant              = "remote"
	gitSwitchSubcommandConstant              = "switch"
	gitCreateBranchFlagConstant              = "-c"
	gitTrackFlagConstant                     = "--track"
	gitPullSubcommandConstant                = "pull"
	gitPullRebaseFlagConstant                = "--rebase"
	gitTerminalPromptEnvironmentNameConstant = "GIT_TERMINAL_PROMPT"
	gitTerminalPromptEnvironmentDisableValue = "0"
)

// ErrRepositoryPathRequired indicates the repository path option was empty.
var ErrRepositoryPathRequired = errors.New(repositoryPathRequiredMessageConstant)

// ErrBranchNameRequired indicates the branch name option was empty.
var ErrBranchNameRequired = errors.New(branchNameRequiredMessageConstant)

// ErrGitExecutorNotConfigured indicates the git executor dependency was missing.
var ErrGitExecutorNotConfigured = errors.New(gitExecutorMissingMessageConstant)

// ServiceDependencies enumerates collaborators required by the service.
type ServiceDependencies struct {
	GitExecutor shared.GitExecutor
	Logger      *zap.Logger
}

// Options configure a branch change operation.
type Options struct {
	RepositoryPath  string
	BranchName      string
	RemoteName      string
	CreateIfMissing bool
	DryRun          bool
}

// Result captures the outcome of a branch change.
type Result struct {
	RepositoryPath string
	BranchName     string
	BranchCreated  bool
	Warnings       []string
}

// Service coordinates branch switching across repositories.
type Service struct {
	executor shared.GitExecutor
	logger   *zap.Logger
}

// NewService constructs a Service from the provided dependencies.
func NewService(dependencies ServiceDependencies) (*Service, error) {
	if dependencies.GitExecutor == nil {
		return nil, ErrGitExecutorNotConfigured
	}
	logger := dependencies.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Service{executor: dependencies.GitExecutor, logger: logger}, nil
}

// Change switches the repository to the requested branch, creating it from the remote if needed.
func (service *Service) Change(executionContext context.Context, options Options) (Result, error) {
	trimmedRepositoryPath := strings.TrimSpace(options.RepositoryPath)
	if len(trimmedRepositoryPath) == 0 {
		return Result{}, ErrRepositoryPathRequired
	}

	trimmedBranchName := strings.TrimSpace(options.BranchName)
	if len(trimmedBranchName) == 0 {
		return Result{}, ErrBranchNameRequired
	}

	remoteName := strings.TrimSpace(options.RemoteName)
	remoteExplicitlyProvided := len(remoteName) > 0
	if !remoteExplicitlyProvided {
		remoteName = defaultRemoteNameConstant
	}

	if options.DryRun {
		return Result{RepositoryPath: trimmedRepositoryPath, BranchName: trimmedBranchName}, nil
	}

	environment := map[string]string{gitTerminalPromptEnvironmentNameConstant: gitTerminalPromptEnvironmentDisableValue}

	remoteEnumeration, remoteLookupErr := service.enumerateRemotes(executionContext, trimmedRepositoryPath, remoteName, environment)
	if remoteLookupErr != nil {
		return Result{}, fmt.Errorf(gitFetchFailureTemplateConstant, fmt.Errorf(gitRemoteListFailureTemplateConstant, remoteLookupErr))
	}

	useAllRemotes := !remoteExplicitlyProvided && remoteEnumeration.hasRemotes && !remoteEnumeration.requestedExists
	shouldFetch := remoteEnumeration.hasRemotes && (!remoteExplicitlyProvided || remoteEnumeration.requestedExists)
	shouldPull := shouldFetch
	shouldTrackRemote := remoteEnumeration.requestedExists && shouldFetch
	warnings := make([]string, 0)

	if shouldFetch {
		fetchArguments := []string{gitFetchSubcommandConstant}
		if useAllRemotes {
			fetchArguments = append(fetchArguments, gitFetchAllFlagConstant, gitFetchPruneFlagConstant)
		} else {
			fetchArguments = append(fetchArguments, gitFetchPruneFlagConstant, remoteName)
		}

		if _, err := service.executor.ExecuteGit(executionContext, execshell.CommandDetails{
			Arguments:            fetchArguments,
			WorkingDirectory:     trimmedRepositoryPath,
			EnvironmentVariables: environment,
		}); err != nil {
			summary := summarizeCommandError(err)
			var warningMessage string
			if shouldReportMissingRemote(summary) {
				warningMessage = fmt.Sprintf(missingRemoteWarningTemplateConstant, formatMissingRemoteRepositoryName(trimmedRepositoryPath))
			} else {
				warningMessage = fmt.Sprintf(fetchWarningTemplateConstant, remoteName, summary)
			}
			service.logger.Warn(
				fetchWarningLogMessageConstant,
				zap.String(logFieldRepositoryPathConstant, trimmedRepositoryPath),
				zap.String(logFieldRemoteNameConstant, remoteName),
				zap.Error(err),
			)
			shouldPull = false
			warnings = append(warnings, warningMessage)
		}
	}

	branchCreated := false
	switchResultErr := service.trySwitch(executionContext, trimmedRepositoryPath, trimmedBranchName, environment)
	branchMissing := isBranchMissingError(switchResultErr)
	switchSummary := summarizeCommandError(switchResultErr)

	if switchResultErr != nil {
		if !options.CreateIfMissing || !branchMissing {
			return Result{}, fmt.Errorf(gitSwitchFailureTemplateConstant, trimmedBranchName, switchSummary, switchResultErr)
		}
		switchArguments := []string{gitSwitchSubcommandConstant, gitCreateBranchFlagConstant, trimmedBranchName}
		if shouldTrackRemote {
			trackReference := fmt.Sprintf("%s/%s", remoteName, trimmedBranchName)
			switchArguments = append(switchArguments, gitTrackFlagConstant, trackReference)
		}
		if _, err := service.executor.ExecuteGit(executionContext, execshell.CommandDetails{
			Arguments:            switchArguments,
			WorkingDirectory:     trimmedRepositoryPath,
			EnvironmentVariables: environment,
		}); err != nil {
			createSummary := summarizeCommandError(err)
			if shouldTrackRemote {
				return Result{}, fmt.Errorf(gitCreateBranchFromRemoteFailureTemplate, trimmedBranchName, remoteName, createSummary, err)
			}
			return Result{}, fmt.Errorf(gitCreateBranchLocalFailureTemplate, trimmedBranchName, createSummary, err)
		}
		branchCreated = true
	}

	if shouldPull {
		if _, err := service.executor.ExecuteGit(executionContext, execshell.CommandDetails{
			Arguments:            []string{gitPullSubcommandConstant, gitPullRebaseFlagConstant},
			WorkingDirectory:     trimmedRepositoryPath,
			EnvironmentVariables: environment,
		}); err != nil {
			warningMessage := fmt.Sprintf(pullWarningTemplateConstant, summarizeCommandError(err))
			service.logger.Warn(
				pullWarningLogMessageConstant,
				zap.String(logFieldRepositoryPathConstant, trimmedRepositoryPath),
				zap.Error(err),
			)
			warnings = append(warnings, warningMessage)
		}
	}

	return Result{
		RepositoryPath: trimmedRepositoryPath,
		BranchName:     trimmedBranchName,
		BranchCreated:  branchCreated,
		Warnings:       warnings,
	}, nil
}

func (service *Service) trySwitch(executionContext context.Context, repositoryPath string, branchName string, environment map[string]string) error {
	_, err := service.executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:            []string{gitSwitchSubcommandConstant, branchName},
		WorkingDirectory:     repositoryPath,
		EnvironmentVariables: environment,
	})
	return err
}

type remoteEnumeration struct {
	hasRemotes      bool
	requestedExists bool
}

func (service *Service) enumerateRemotes(executionContext context.Context, repositoryPath string, remoteName string, environment map[string]string) (remoteEnumeration, error) {
	result, err := service.executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:            []string{gitRemoteSubcommandConstant},
		WorkingDirectory:     repositoryPath,
		EnvironmentVariables: environment,
	})
	if err != nil {
		return remoteEnumeration{}, err
	}

	enumeration := remoteEnumeration{}
	for _, candidate := range strings.Split(result.StandardOutput, "\n") {
		trimmedCandidate := strings.TrimSpace(candidate)
		if len(trimmedCandidate) == 0 {
			continue
		}
		enumeration.hasRemotes = true
		if trimmedCandidate == remoteName {
			enumeration.requestedExists = true
		}
	}
	return enumeration, nil
}

var missingBranchIndicators = []string{
	"did not match any file(s) known to git",
	"unknown revision or path not in the working tree",
	"not a valid reference",
	"invalid reference",
	"no such ref was found",
	"matches none of the refs",
}

var missingRemoteErrorIndicators = []string{
	"repository not found",
	"could not read from remote repository",
	"no such remote",
}

func isBranchMissingError(err error) bool {
	if err == nil {
		return false
	}
	summary := strings.ToLower(summarizeCommandError(err))
	if len(summary) == 0 {
		return false
	}
	for _, indicator := range missingBranchIndicators {
		if strings.Contains(summary, indicator) {
			return true
		}
	}
	return false
}

func summarizeCommandError(err error) string {
	if err == nil {
		return ""
	}
	var commandFailure execshell.CommandFailedError
	if errors.As(err, &commandFailure) {
		trimmedStandardError := strings.TrimSpace(commandFailure.Result.StandardError)
		if len(trimmedStandardError) > 0 {
			return firstLine(trimmedStandardError)
		}
		return firstLine(commandFailure.Error())
	}
	return firstLine(strings.TrimSpace(err.Error()))
}

func firstLine(message string) string {
	if len(message) == 0 {
		return ""
	}
	for _, line := range strings.Split(message, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 0 {
			return trimmed
		}
	}
	return ""
}

func shouldReportMissingRemote(summary string) bool {
	if len(summary) == 0 {
		return false
	}
	normalized := strings.ToLower(summary)
	for _, indicator := range missingRemoteErrorIndicators {
		if strings.Contains(normalized, indicator) {
			return true
		}
	}
	return false
}

func formatMissingRemoteRepositoryName(repositoryPath string) string {
	base := strings.TrimSpace(filepath.Base(repositoryPath))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "this repository"
	}
	lower := strings.ToLower(base)
	if strings.HasSuffix(lower, "repo") || strings.HasSuffix(lower, "repository") {
		return base
	}
	return fmt.Sprintf("%s repo", base)
}
