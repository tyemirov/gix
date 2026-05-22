package cd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tyemirov/gix/internal/commitmsg"
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/workflow"
	"github.com/tyemirov/utils/llm"
)

const (
	gitWorktreeSubcommandConstant             = "worktree"
	gitWorktreeListSubcommandConstant         = "list"
	gitWorktreeRemoveSubcommandConstant       = "remove"
	gitWorktreePruneSubcommandConstant        = "prune"
	gitPorcelainFlagConstant                  = "--porcelain"
	gitPorcelainBranchFlagConstant            = "--branch"
	gitStatusSubcommand                       = "status"
	gitAddSubcommand                          = "add"
	gitCommitSubcommand                       = "commit"
	gitPushSubcommand                         = "push"
	gitRevListSubcommand                      = "rev-list"
	gitAddAllFlag                             = "--all"
	gitCommitMessageFlag                      = "-m"
	gitPushSetUpstreamFlag                    = "--set-upstream"
	gitRevListCountFlag                       = "--count"
	gitBranchReferencePrefix                  = "refs/heads/"
	gitStatusBranchHeaderPrefix               = "## "
	gitStatusAheadMarker                      = "ahead "
	gitBranchAlreadyUsedByWorktreeIndicator   = "already used by worktree"
	worktreeListFailureTemplate               = "failed to list worktrees before switching to %q: %w"
	worktreeStatusFailureTemplate             = "failed to inspect worktree %s: %w"
	worktreeLockedFailureTemplate             = "target branch %q is checked out in locked worktree %s"
	worktreeStageFailureTemplate              = "failed to stage sibling worktree changes at %s: %w"
	worktreeCommitFailureTemplate             = "failed to commit sibling worktree changes at %s: %w"
	worktreePushFailureTemplate               = "failed to push adopted worktree branch %q from %s: %w"
	worktreePushInspectionFailureTemplate     = "failed to inspect push requirement for branch %q against %q from %s: %w"
	worktreeRemoteInspectionFailureTemplate   = "failed to inspect remote branch %q from %s: %w"
	worktreeRemoveFailureTemplate             = "failed to remove sibling worktree %s: %w"
	worktreePruneFailureTemplate              = "failed to prune worktrees after removing %s: %w"
	worktreeMessageClientConfigurationFailure = "commit message generation requires model and api key configuration"
	worktreeMessageAPIKeyFailureTemplate      = "environment variable %s must be set to generate a commit message"
	worktreeMessageClientFailureTemplate      = "failed to initialize commit message client: %w"
	worktreeMessageGenerationFailureTemplate  = "failed to generate commit message for sibling worktree %s: %w"
	worktreeAdoptDetectedMessage              = "adopting sibling worktree"
	worktreeAdoptCommitMessage                = "committed sibling worktree changes"
	worktreeAdoptPushMessage                  = "pushed sibling worktree branch"
	worktreeAdoptRemoveMessage                = "removed sibling worktree"
	worktreeAdoptPruneMessage                 = "pruned worktree metadata"
)

type worktreeAdoptionOptions struct {
	BranchName     string
	RemoteName     string
	CommitMessages worktreeAdoptionCommitMessageOptions
}

func isBranchAlreadyUsedByWorktreeError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), gitBranchAlreadyUsedByWorktreeIndicator)
}

type worktreeAdoptionCommitMessageOptions struct {
	APIKeyEnv      string
	BaseURL        string
	Model          string
	MaxTokens      int
	Temperature    float64
	TimeoutSeconds int
	Client         llm.ChatClient
}

type listedWorktree struct {
	Path       string
	BranchName string
	Locked     bool
}

type worktreeStatus struct {
	Dirty bool
	Ahead bool
}

func worktreeAdoptionCommitMessageOptionsFromConfiguration(configuration CommitMessageConfiguration) worktreeAdoptionCommitMessageOptions {
	sanitized := configuration.Sanitize()
	return worktreeAdoptionCommitMessageOptions{
		APIKeyEnv:      sanitized.APIKeyEnv,
		BaseURL:        sanitized.BaseURL,
		Model:          sanitized.Model,
		MaxTokens:      sanitized.MaxTokens,
		Temperature:    sanitized.Temperature,
		TimeoutSeconds: sanitized.TimeoutSeconds,
	}
}

func worktreeAdoptionCommitMessageOptionsFromParameters(parameters map[string]any) (worktreeAdoptionCommitMessageOptions, error) {
	rawOptions, exists := parameters[taskOptionWorktreeCommitMessage]
	if !exists || rawOptions == nil {
		return worktreeAdoptionCommitMessageOptions{}, nil
	}
	typedOptions, ok := rawOptions.(worktreeAdoptionCommitMessageOptions)
	if !ok {
		return worktreeAdoptionCommitMessageOptions{}, fmt.Errorf("%s must be commit message options", taskOptionWorktreeCommitMessage)
	}
	return typedOptions, nil
}

func adoptExistingBranchWorktree(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, options worktreeAdoptionOptions) error {
	if environment == nil || repository == nil {
		return nil
	}
	if environment.GitExecutor == nil {
		return ErrGitExecutorNotConfigured
	}

	branchName := strings.TrimSpace(options.BranchName)
	if branchName == "" {
		return nil
	}

	worktrees, listErr := listRepositoryWorktrees(ctx, environment.GitExecutor, repository.Path, branchName)
	if listErr != nil {
		return listErr
	}

	for worktreeIndex := range worktrees {
		candidate := worktrees[worktreeIndex]
		if candidate.BranchName != branchName {
			continue
		}
		if sameFilesystemPath(candidate.Path, repository.Path) {
			continue
		}
		if candidate.Locked {
			return fmt.Errorf(worktreeLockedFailureTemplate, branchName, candidate.Path)
		}
		return adoptSiblingWorktree(ctx, environment, repository, candidate, options)
	}

	return nil
}

func listRepositoryWorktrees(ctx context.Context, executor shared.GitExecutor, repositoryPath string, branchName string) ([]listedWorktree, error) {
	result, listErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitWorktreeSubcommandConstant, gitWorktreeListSubcommandConstant, gitPorcelainFlagConstant},
		WorkingDirectory: repositoryPath,
	})
	if listErr != nil {
		return nil, fmt.Errorf(worktreeListFailureTemplate, branchName, listErr)
	}
	return parseListedWorktrees(result.StandardOutput), nil
}

func parseListedWorktrees(output string) []listedWorktree {
	var worktrees []listedWorktree
	current := listedWorktree{}
	flushCurrent := func() {
		if strings.TrimSpace(current.Path) == "" {
			current = listedWorktree{}
			return
		}
		worktrees = append(worktrees, current)
		current = listedWorktree{}
	}

	lines := strings.Split(output, "\n")
	for lineIndex := range lines {
		line := strings.TrimSpace(lines[lineIndex])
		if line == "" {
			flushCurrent()
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "worktree":
			flushCurrent()
			current.Path = strings.TrimSpace(strings.TrimPrefix(line, "worktree"))
		case "branch":
			referenceName := strings.TrimSpace(strings.TrimPrefix(line, "branch"))
			current.BranchName = strings.TrimPrefix(referenceName, gitBranchReferencePrefix)
		case "locked":
			current.Locked = true
		}
	}
	flushCurrent()
	return worktrees
}

func adoptSiblingWorktree(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, worktree listedWorktree, options worktreeAdoptionOptions) error {
	branchName := strings.TrimSpace(options.BranchName)
	remoteName := strings.TrimSpace(options.RemoteName)
	if remoteName == "" {
		remoteName = defaultRemoteNameConstant
	}

	environment.ReportRepositoryEvent(
		repository,
		shared.EventLevelInfo,
		shared.EventCodeWorktreeAdopt,
		worktreeAdoptDetectedMessage,
		map[string]string{"branch": branchName, "worktree": worktree.Path},
	)

	status, statusErr := inspectSiblingWorktreeStatus(ctx, environment.GitExecutor, worktree.Path)
	if statusErr != nil {
		return statusErr
	}

	pushed := false
	if status.Dirty {
		commitMessage, commitMessageErr := generateSiblingCommitMessage(ctx, environment.GitExecutor, worktree.Path, options.CommitMessages)
		if commitMessageErr != nil {
			return commitMessageErr
		}
		if commitErr := executeGit(ctx, environment.GitExecutor, worktree.Path, []string{gitCommitSubcommand, gitCommitMessageFlag, commitMessage}); commitErr != nil {
			return fmt.Errorf(worktreeCommitFailureTemplate, worktree.Path, commitErr)
		}
		environment.ReportRepositoryEvent(
			repository,
			shared.EventLevelInfo,
			shared.EventCodeWorktreeAdopt,
			worktreeAdoptCommitMessage,
			map[string]string{"branch": branchName, "worktree": worktree.Path},
		)
		if pushErr := pushSiblingBranch(ctx, environment.GitExecutor, worktree.Path, remoteName, branchName); pushErr != nil {
			return pushErr
		}
		pushed = true
	}

	if !status.Dirty {
		needsPush, needsPushErr := cleanSiblingBranchNeedsPush(ctx, environment.GitExecutor, worktree.Path, remoteName, branchName, status)
		if needsPushErr != nil {
			return needsPushErr
		}
		if needsPush {
			if pushErr := pushSiblingBranch(ctx, environment.GitExecutor, worktree.Path, remoteName, branchName); pushErr != nil {
				return pushErr
			}
			pushed = true
		}
	}

	if pushed {
		environment.ReportRepositoryEvent(
			repository,
			shared.EventLevelInfo,
			shared.EventCodeWorktreeAdopt,
			worktreeAdoptPushMessage,
			map[string]string{"branch": branchName, "remote": remoteName, "worktree": worktree.Path},
		)
	}

	if removeErr := executeGit(ctx, environment.GitExecutor, repository.Path, []string{gitWorktreeSubcommandConstant, gitWorktreeRemoveSubcommandConstant, worktree.Path}); removeErr != nil {
		return fmt.Errorf(worktreeRemoveFailureTemplate, worktree.Path, removeErr)
	}
	environment.ReportRepositoryEvent(
		repository,
		shared.EventLevelInfo,
		shared.EventCodeWorktreeAdopt,
		worktreeAdoptRemoveMessage,
		map[string]string{"branch": branchName, "worktree": worktree.Path},
	)

	if pruneErr := executeGit(ctx, environment.GitExecutor, repository.Path, []string{gitWorktreeSubcommandConstant, gitWorktreePruneSubcommandConstant}); pruneErr != nil {
		return fmt.Errorf(worktreePruneFailureTemplate, worktree.Path, pruneErr)
	}
	environment.ReportRepositoryEvent(
		repository,
		shared.EventLevelInfo,
		shared.EventCodeWorktreeAdopt,
		worktreeAdoptPruneMessage,
		map[string]string{"branch": branchName, "worktree": worktree.Path},
	)

	return nil
}

func inspectSiblingWorktreeStatus(ctx context.Context, executor shared.GitExecutor, worktreePath string) (worktreeStatus, error) {
	result, statusErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitStatusSubcommand, gitPorcelainFlagConstant, gitPorcelainBranchFlagConstant},
		WorkingDirectory: worktreePath,
	})
	if statusErr != nil {
		return worktreeStatus{}, fmt.Errorf(worktreeStatusFailureTemplate, worktreePath, statusErr)
	}

	status := worktreeStatus{}
	lines := strings.Split(result.StandardOutput, "\n")
	for lineIndex := range lines {
		line := strings.TrimSpace(lines[lineIndex])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, gitStatusBranchHeaderPrefix) {
			status.Ahead = strings.Contains(line, gitStatusAheadMarker)
			continue
		}
		status.Dirty = true
	}
	return status, nil
}

func cleanSiblingBranchNeedsPush(ctx context.Context, executor shared.GitExecutor, worktreePath string, remoteName string, branchName string, status worktreeStatus) (bool, error) {
	if status.Ahead {
		return true, nil
	}

	remoteReference := fmt.Sprintf("%s/%s", remoteName, branchName)
	remoteExists, remoteErr := remoteBranchReferenceExists(ctx, executor, worktreePath, remoteReference)
	if remoteErr != nil {
		return false, remoteErr
	}
	if !remoteExists {
		return true, nil
	}

	comparisonRange := fmt.Sprintf("%s..%s", remoteReference, branchName)
	result, countErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitRevListSubcommand, gitRevListCountFlag, comparisonRange},
		WorkingDirectory: worktreePath,
	})
	if countErr != nil {
		return false, fmt.Errorf(worktreePushInspectionFailureTemplate, branchName, remoteReference, worktreePath, countErr)
	}

	return strings.TrimSpace(result.StandardOutput) != "0", nil
}

func remoteBranchReferenceExists(ctx context.Context, executor shared.GitExecutor, worktreePath string, remoteReference string) (bool, error) {
	_, remoteErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitRevParseSubcommandConstant, gitVerifyFlagConstant, remoteReference},
		WorkingDirectory: worktreePath,
	})
	if remoteErr == nil {
		return true, nil
	}
	if isBranchMissingError(remoteErr) {
		return false, nil
	}
	return false, fmt.Errorf(worktreeRemoteInspectionFailureTemplate, remoteReference, worktreePath, remoteErr)
}

func generateSiblingCommitMessage(ctx context.Context, executor shared.GitExecutor, worktreePath string, options worktreeAdoptionCommitMessageOptions) (string, error) {
	if stageErr := executeGit(ctx, executor, worktreePath, []string{gitAddSubcommand, gitAddAllFlag}); stageErr != nil {
		return "", fmt.Errorf(worktreeStageFailureTemplate, worktreePath, stageErr)
	}

	client, clientErr := resolveCommitMessageClient(options)
	if clientErr != nil {
		return "", clientErr
	}

	var temperature *float64
	if options.Temperature != 0 {
		temperatureValue := options.Temperature
		temperature = &temperatureValue
	}

	generator := commitmsg.Generator{
		GitExecutor: executor,
		Client:      client,
	}
	result, generateErr := generator.Generate(ctx, commitmsg.Options{
		RepositoryPath: worktreePath,
		Source:         commitmsg.DiffSourceStaged,
		MaxTokens:      options.MaxTokens,
		Temperature:    temperature,
	})
	if generateErr != nil {
		return "", fmt.Errorf(worktreeMessageGenerationFailureTemplate, worktreePath, generateErr)
	}
	return result.Message, nil
}

func resolveCommitMessageClient(options worktreeAdoptionCommitMessageOptions) (llm.ChatClient, error) {
	if options.Client != nil {
		return options.Client, nil
	}
	apiKeyEnv := strings.TrimSpace(options.APIKeyEnv)
	model := strings.TrimSpace(options.Model)
	if apiKeyEnv == "" || model == "" {
		return nil, errors.New(worktreeMessageClientConfigurationFailure)
	}
	apiKey := strings.TrimSpace(os.Getenv(apiKeyEnv))
	if apiKey == "" {
		return nil, fmt.Errorf(worktreeMessageAPIKeyFailureTemplate, apiKeyEnv)
	}
	timeoutSeconds := options.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultCommitMessageTimeoutSeconds
	}
	client, clientErr := llm.NewFactory(llm.Config{
		BaseURL:             strings.TrimSpace(options.BaseURL),
		APIKey:              apiKey,
		Model:               model,
		MaxCompletionTokens: options.MaxTokens,
		Temperature:         options.Temperature,
		RequestTimeout:      time.Duration(timeoutSeconds) * time.Second,
	})
	if clientErr != nil {
		return nil, fmt.Errorf(worktreeMessageClientFailureTemplate, clientErr)
	}
	return client, nil
}

func pushSiblingBranch(ctx context.Context, executor shared.GitExecutor, worktreePath string, remoteName string, branchName string) error {
	if pushErr := executeGit(ctx, executor, worktreePath, []string{gitPushSubcommand, gitPushSetUpstreamFlag, remoteName, branchName}); pushErr != nil {
		return fmt.Errorf(worktreePushFailureTemplate, branchName, worktreePath, pushErr)
	}
	return nil
}

func executeGit(ctx context.Context, executor shared.GitExecutor, workingDirectory string, arguments []string) error {
	_, executionErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        arguments,
		WorkingDirectory: workingDirectory,
	})
	return executionErr
}

func sameFilesystemPath(firstPath string, secondPath string) bool {
	normalizedFirst := normalizeFilesystemPath(firstPath)
	normalizedSecond := normalizeFilesystemPath(secondPath)
	if normalizedFirst == "" || normalizedSecond == "" {
		return strings.TrimSpace(firstPath) == strings.TrimSpace(secondPath)
	}
	return normalizedFirst == normalizedSecond
}

func normalizeFilesystemPath(path string) string {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return ""
	}
	absolutePath, absoluteErr := filepath.Abs(trimmedPath)
	if absoluteErr != nil {
		return filepath.Clean(trimmedPath)
	}
	return filepath.Clean(absolutePath)
}
