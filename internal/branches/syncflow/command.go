package syncflow

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	rootutils "github.com/tyemirov/gix/internal/utils/roots"
	"github.com/tyemirov/gix/internal/workflow"
	"github.com/tyemirov/gix/pkg/taskrunner"
)

const (
	commandUseNameConstant                  = "sync"
	commandUsageTemplateConstant            = commandUseNameConstant + " [remote-url|branch]"
	commandExampleTemplateConstant          = "gix sync\ngix sync master\ngix sync feature/new-branch"
	commandShortDescriptionConstant         = "Synchronize the current workspace through the Gix PR workflow"
	commandLongDescriptionConstant          = "sync keeps a workspace aligned with explicitly targeted branches and PR-backed work branches. An explicit branch target is binding: dirty work is committed to that named branch. Explicit gix sync master commits to master, merges origin/master, and pushes master directly. With no branch argument, sync updates the current branch; dirty current master keeps the generated PR rescue flow. Existing PR branches sync against their current PR base branch. A missing explicit branch with dirty work is created on top of the current branch. If the current branch is not master, sync first ensures that its committed HEAD is remote-backed and has an open pull request, then opens the child pull request against that branch. The selected parent base is retained for retries after child push or pull-request failure. A clean or stashed missing branch is rejected because it would have no child pull request delta. Dirty work is clustered and described with the configured LLM transport. Dirty auto-commit is rejected on a known-merged branch; use --stash to preserve that work through the merged handoff before creating a new review branch. Sync never rebases or force-pushes. When sync creates a pull request, the body is generated from the branch diff unless --body or sync.pull_request.body supplies explicit text; title defaults to the branch unless --title or sync.pull_request.title supplies it."
	missingBranchMessageConstant            = "unable to determine branch; provide a branch argument or configure a default branch"
	syncCreatedSuffixConstant               = " (created)"
	stashFlagNameConstant                   = "stash"
	stashFlagDescriptionConstant            = "Stash local changes before syncing"
	commitFlagNameConstant                  = "commit"
	commitFlagDescriptionConstant           = "Commit local changes before syncing (default dirty-sync behavior)"
	requireCleanFlagNameConstant            = "require-clean"
	requireCleanFlagDescriptionConstant     = "Require a clean worktree instead of auto-committing dirty work"
	pullRequestTitleFlagNameConstant        = taskOptionPullRequestTitle
	pullRequestTitleFlagDescriptionConstant = "Set the title for a sync-created pull request"
	pullRequestBodyFlagNameConstant         = taskOptionPullRequestBody
	pullRequestBodyFlagDescriptionConstant  = "Set the body for a sync-created pull request"
	conflictingRecoveryFlagsMessageConstant = "use at most one of --stash or --commit"
	remoteTargetExtraArgsMessage            = "remote sync target does not accept repository root arguments"
	remoteTargetDirtyDirectoryMessage       = "remote sync target requires an empty directory when cloning"
	remoteTargetMismatchTemplate            = "workspace origin %q does not match requested remote %q"
	remoteTargetClonedTemplate              = "CLONED: %s\n"
	remoteTargetAttachedTemplate            = "ATTACHED: %s\n"
)

// LoggerProvider yields a zap logger instance.
type LoggerProvider func() *zap.Logger

// CommandBuilder assembles the sync command.
type CommandBuilder struct {
	LoggerProvider               LoggerProvider
	GitExecutor                  shared.GitExecutor
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() CommandConfiguration
	Discoverer                   shared.RepositoryDiscoverer
	GitManager                   shared.GitRepositoryManager
	GitHubResolver               shared.GitHubMetadataResolver
	FileSystem                   shared.FileSystem
	PrompterFactory              func(*cobra.Command) shared.ConfirmationPrompter
	TaskRunnerFactory            func(workflow.Dependencies) TaskRunnerExecutor
}

// Build constructs the sync command.
func (builder *CommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:     commandUsageTemplateConstant,
		Short:   commandShortDescriptionConstant,
		Long:    commandLongDescriptionConstant,
		RunE:    builder.run,
		Args:    cobra.ArbitraryArgs,
		Example: commandExampleTemplateConstant,
	}

	flagutils.AddToggleFlag(command.Flags(), nil, stashFlagNameConstant, "", false, stashFlagDescriptionConstant)
	flagutils.AddToggleFlag(command.Flags(), nil, commitFlagNameConstant, "", false, commitFlagDescriptionConstant)
	flagutils.AddToggleFlag(command.Flags(), nil, requireCleanFlagNameConstant, "", false, requireCleanFlagDescriptionConstant)
	command.Flags().String(pullRequestTitleFlagNameConstant, "", pullRequestTitleFlagDescriptionConstant)
	command.Flags().String(pullRequestBodyFlagNameConstant, "", pullRequestBodyFlagDescriptionConstant)

	return command, nil
}

func (builder *CommandBuilder) run(command *cobra.Command, arguments []string) error {
	configuration := builder.resolveConfiguration()

	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

	explicitBranch, configuredFallbackBranch, remainingArgs := builder.resolveBranchName(command, arguments, configuration)

	refreshRequested := configuration.RequireClean
	stashRequested := configuration.StashChanges
	commitRequested := configuration.CommitChanges
	requireClean := configuration.RequireClean
	pullRequestTitle := strings.TrimSpace(configuration.PullRequest.Title)
	pullRequestBody := strings.TrimSpace(configuration.PullRequest.Body)

	if command != nil {
		if flagValue, err := command.Flags().GetBool(stashFlagNameConstant); err == nil && command.Flags().Changed(stashFlagNameConstant) {
			stashRequested = flagValue
		}
		if flagValue, err := command.Flags().GetBool(commitFlagNameConstant); err == nil && command.Flags().Changed(commitFlagNameConstant) {
			commitRequested = flagValue
		}
		if flagValue, err := command.Flags().GetBool(requireCleanFlagNameConstant); err == nil && command.Flags().Changed(requireCleanFlagNameConstant) {
			requireClean = flagValue
		}
		if flagValue, err := command.Flags().GetString(pullRequestTitleFlagNameConstant); err == nil && command.Flags().Changed(pullRequestTitleFlagNameConstant) {
			pullRequestTitle = strings.TrimSpace(flagValue)
		}
		if flagValue, err := command.Flags().GetString(pullRequestBodyFlagNameConstant); err == nil && command.Flags().Changed(pullRequestBodyFlagNameConstant) {
			pullRequestBody = strings.TrimSpace(flagValue)
		}
	}

	if stashRequested && commitRequested {
		return errors.New(conflictingRecoveryFlagsMessageConstant)
	}
	refreshRequested = refreshRequested || stashRequested || commitRequested

	remoteName := strings.TrimSpace(configuration.RemoteName)
	if executionFlagsAvailable && executionFlags.RemoteSet {
		overridden := strings.TrimSpace(executionFlags.Remote)
		if len(overridden) > 0 {
			remoteName = overridden
		}
	}

	dependencyResult, dependencyError := taskrunner.BuildDependencies(
		taskrunner.DependenciesConfig{
			LoggerProvider:               builder.LoggerProvider,
			HumanReadableLoggingProvider: builder.HumanReadableLoggingProvider,
			RepositoryDiscoverer:         builder.Discoverer,
			GitExecutor:                  builder.GitExecutor,
			GitRepositoryManager:         builder.GitManager,
			GitHubResolver:               builder.GitHubResolver,
			FileSystem:                   builder.FileSystem,
			PrompterFactory:              builder.PrompterFactory,
		},
		taskrunner.DependenciesOptions{
			Command: command,
			Output:  command.OutOrStdout(),
			Errors:  command.ErrOrStderr(),
		},
	)
	if dependencyError != nil {
		return dependencyError
	}
	dependencyResult.Workflow.SuppressOperationFailureOutput = true

	if remoteURL, remoteTarget := resolveRemoteTarget(command, dependencyResult.GitExecutor, explicitBranch); remoteTarget {
		if len(remainingArgs) > 0 {
			return errors.New(remoteTargetExtraArgsMessage)
		}
		return syncRemoteTarget(command, dependencyResult.GitExecutor, remoteURL)
	}

	repositoryRoots, rootsError := rootutils.Resolve(command, remainingArgs, configuration.RepositoryRoots)
	if rootsError != nil {
		return rootsError
	}

	taskRunner := resolveTaskRunner(builder.TaskRunnerFactory, dependencyResult.Workflow)

	actionOptions := map[string]any{
		taskOptionBranchRemote:       remoteName,
		taskOptionBranchCreate:       configuration.CreateIfMissing,
		taskOptionRequirePullRequest: true,
		taskOptionBaseBranch:         "master",
		taskOptionRequireClean:       requireClean,
	}
	if len(explicitBranch) > 0 {
		actionOptions[taskOptionBranchName] = explicitBranch
	}
	if len(configuredFallbackBranch) > 0 {
		actionOptions[taskOptionConfiguredDefaultBranch] = configuredFallbackBranch
	}
	if refreshRequested {
		actionOptions[taskOptionRefreshEnabled] = true
	}
	if stashRequested {
		actionOptions[taskOptionStashChanges] = true
	}
	if commitRequested {
		actionOptions[taskOptionCommitChanges] = true
	}
	if len(pullRequestTitle) > 0 {
		actionOptions[taskOptionPullRequestTitle] = pullRequestTitle
	}
	if len(pullRequestBody) > 0 {
		actionOptions[taskOptionPullRequestBody] = pullRequestBody
	}
	actionOptions[taskOptionWorktreeCommitMessage] = worktreeAdoptionCommitMessageOptionsFromConfiguration(configuration.CommitMessage)

	taskBranchLabel := strings.TrimSpace(explicitBranch)
	if len(taskBranchLabel) == 0 {
		taskBranchLabel = strings.TrimSpace(configuredFallbackBranch)
	}
	taskName := fmt.Sprintf("Sync %s", taskBranchLabel)
	if len(taskBranchLabel) == 0 {
		taskName = "Sync current branch"
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        taskName,
		EnsureClean: false,
		Actions: []workflow.TaskActionDefinition{
			{Type: taskTypeBranchSync, Options: actionOptions},
		},
	}

	assumeYes := false
	if executionFlagsAvailable {
		assumeYes = executionFlags.AssumeYes
	}
	runtimeOptions := workflow.RuntimeOptions{
		AssumeYes: assumeYes,
	}

	_, runErr := taskRunner.Run(
		command.Context(),
		repositoryRoots,
		[]workflow.TaskDefinition{taskDefinition},
		runtimeOptions,
	)
	return runErr
}

func (builder *CommandBuilder) resolveConfiguration() CommandConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultCommandConfiguration()
	}
	return builder.ConfigurationProvider().Sanitize()
}

func (builder *CommandBuilder) resolveBranchName(command *cobra.Command, arguments []string, configuration CommandConfiguration) (string, string, []string) {
	remaining := arguments
	if len(remaining) > 0 {
		branch := strings.TrimSpace(remaining[0])
		return branch, strings.TrimSpace(configuration.DefaultBranch), remaining[1:]
	}

	return "", strings.TrimSpace(configuration.DefaultBranch), remaining
}

func resolveRemoteTarget(command *cobra.Command, executor shared.GitExecutor, target string) (string, bool) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "", false
	}
	if remoteURL, ok := remoteURLFromExplicitTarget(trimmed); ok {
		return remoteURL, true
	}
	if ownerRepositoryURL, ok := remoteURLFromOwnerRepository(trimmed); ok && !commandDirectoryIsGitWorktree(command, executor) {
		return ownerRepositoryURL, true
	}
	return "", false
}

func remoteURLFromExplicitTarget(target string) (string, bool) {
	lowered := strings.ToLower(strings.TrimSpace(target))
	switch {
	case strings.HasPrefix(lowered, "https://"):
		return target, true
	case strings.HasPrefix(lowered, "http://"):
		return target, true
	case strings.HasPrefix(lowered, "ssh://"):
		return target, true
	case strings.HasPrefix(lowered, "git@"):
		return target, true
	case strings.HasSuffix(lowered, ".git") && strings.Contains(lowered, "/"):
		return target, true
	default:
		return "", false
	}
}

func remoteURLFromOwnerRepository(target string) (string, bool) {
	if branchLikeTarget(target) {
		return "", false
	}
	ownerRepository, err := shared.NewOwnerRepository(target)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("https://github.com/%s.git", ownerRepository.String()), true
}

func branchLikeTarget(target string) bool {
	segments := strings.Split(strings.TrimSpace(target), "/")
	if len(segments) < 2 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(segments[0])) {
	case "feature", "improvement", "bugfix", "maintenance", "blocked", "chore", "fix", "hotfix", "release", "dependabot", "renovate":
		return true
	default:
		return false
	}
}

func commandDirectoryIsGitWorktree(command *cobra.Command, executor shared.GitExecutor) bool {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return false
	}
	if executor == nil {
		return false
	}
	result, executionErr := executor.ExecuteGit(command.Context(), execshellDetails([]string{"rev-parse", "--is-inside-work-tree"}, workingDirectory))
	return executionErr == nil && strings.TrimSpace(result.StandardOutput) == "true"
}

func syncRemoteTarget(command *cobra.Command, executor shared.GitExecutor, remoteURL string) error {
	if executor == nil {
		return ErrGitExecutorNotConfigured
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}

	insideWorktree := false
	result, worktreeErr := executor.ExecuteGit(command.Context(), execshellDetails([]string{"rev-parse", "--is-inside-work-tree"}, workingDirectory))
	if worktreeErr == nil && strings.TrimSpace(result.StandardOutput) == "true" {
		insideWorktree = true
	}

	if insideWorktree {
		originResult, originErr := executor.ExecuteGit(command.Context(), execshellDetails([]string{"remote", "get-url", defaultRemoteNameConstant}, workingDirectory))
		if originErr != nil {
			return originErr
		}
		originURL := strings.TrimSpace(originResult.StandardOutput)
		if normalizeRemoteURL(originURL) != normalizeRemoteURL(remoteURL) {
			return fmt.Errorf(remoteTargetMismatchTemplate, originURL, remoteURL)
		}
		fmt.Fprintf(command.OutOrStdout(), remoteTargetAttachedTemplate, remoteURL)
		return nil
	}

	entries, readErr := os.ReadDir(workingDirectory)
	if readErr != nil {
		return readErr
	}
	if len(entries) > 0 {
		return errors.New(remoteTargetDirtyDirectoryMessage)
	}
	if _, cloneErr := executor.ExecuteGit(command.Context(), execshellDetails([]string{"clone", remoteURL, "."}, workingDirectory)); cloneErr != nil {
		return cloneErr
	}
	fmt.Fprintf(command.OutOrStdout(), remoteTargetClonedTemplate, remoteURL)
	return nil
}

func normalizeRemoteURL(raw string) string {
	trimmed := strings.TrimSuffix(strings.TrimSpace(raw), ".git")
	trimmed = strings.TrimPrefix(trimmed, "ssh://git@github.com/")
	trimmed = strings.TrimPrefix(trimmed, "git@github.com:")
	trimmed = strings.TrimPrefix(trimmed, "https://github.com/")
	trimmed = strings.TrimPrefix(trimmed, "http://github.com/")
	return strings.ToLower(strings.Trim(trimmed, "/"))
}

func execshellDetails(arguments []string, workingDirectory string) execshell.CommandDetails {
	return execshell.CommandDetails{
		Arguments:        arguments,
		WorkingDirectory: workingDirectory,
		EnvironmentVariables: map[string]string{
			gitTerminalPromptEnvironmentNameConstant: gitTerminalPromptEnvironmentDisableValue,
		},
	}
}
