package cd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/repos/shared"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	rootutils "github.com/tyemirov/gix/internal/utils/roots"
	"github.com/tyemirov/gix/internal/workflow"
	"github.com/tyemirov/gix/pkg/taskrunner"
)

const (
	commandUseNameConstant                  = "cd"
	commandUsageTemplateConstant            = commandUseNameConstant + " [branch]"
	commandExampleTemplateConstant          = "gix cd feature/new-branch --roots ~/Development"
	commandShortDescriptionConstant         = "Switch repositories to the selected branch"
	commandLongDescriptionConstant          = "cd fetches updates, switches to the requested branch, creates it if missing, and rebases onto the remote for each repository root. Provide the branch name as the first argument before any optional repository roots or flags, or configure a default branch in the application settings."
	missingBranchMessageConstant            = "unable to determine branch; provide a branch argument or configure a default branch"
	changeSuccessMessageTemplateConstant    = "SWITCHED: %s -> %s"
	changeCreatedSuffixConstant             = " (created)"
	legacyAliasNameConstant                 = "branch-cd"
	legacyAliasDeprecationMessage           = "DEPRECATED: use `gix cd` instead of `gix branch-cd`."
	stashFlagNameConstant                   = "stash"
	stashFlagDescriptionConstant            = "Stash local changes before refreshing"
	commitFlagNameConstant                  = "commit"
	commitFlagDescriptionConstant           = "Commit local changes before refreshing"
	requireCleanFlagNameConstant            = "require-clean"
	requireCleanFlagDescriptionConstant     = "Require a clean worktree when refreshing"
	conflictingRecoveryFlagsMessageConstant = "use at most one of --stash or --commit"
)

// LoggerProvider yields a zap logger instance.
type LoggerProvider func() *zap.Logger

// CommandBuilder assembles the cd command.
type CommandBuilder struct {
	LoggerProvider               LoggerProvider
	GitExecutor                  shared.GitExecutor
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() CommandConfiguration
	Discoverer                   shared.RepositoryDiscoverer
	GitManager                   shared.GitRepositoryManager
	GitHubResolver               shared.GitHubMetadataResolver
	FileSystem                   shared.FileSystem
	TaskRunnerFactory            func(workflow.Dependencies) TaskRunnerExecutor
}

// Build constructs the cd command.
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
	flagutils.AddToggleFlag(command.Flags(), nil, requireCleanFlagNameConstant, "", true, requireCleanFlagDescriptionConstant)

	return command, nil
}

func (builder *CommandBuilder) run(command *cobra.Command, arguments []string) error {
	configuration := builder.resolveConfiguration()

	if command != nil && strings.EqualFold(command.CalledAs(), legacyAliasNameConstant) {
		_, _ = fmt.Fprintln(command.ErrOrStderr(), legacyAliasDeprecationMessage)
	}

	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

	explicitBranch, configuredFallbackBranch, remainingArgs := builder.resolveBranchName(command, arguments, configuration)

	refreshRequested := configuration.RequireClean
	stashRequested := configuration.StashChanges
	commitRequested := configuration.CommitChanges
	requireClean := configuration.RequireClean

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

	repositoryRoots, rootsError := rootutils.Resolve(command, remainingArgs, configuration.RepositoryRoots)
	if rootsError != nil {
		return rootsError
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
		},
		taskrunner.DependenciesOptions{
			Command:         command,
			Output:          command.OutOrStdout(),
			Errors:          command.ErrOrStderr(),
			DisablePrompter: true,
		},
	)
	if dependencyError != nil {
		return dependencyError
	}

	taskRunner := resolveTaskRunner(builder.TaskRunnerFactory, dependencyResult.Workflow)

	actionOptions := map[string]any{
		taskOptionBranchRemote: remoteName,
		taskOptionBranchCreate: configuration.CreateIfMissing,
	}
	if len(explicitBranch) > 0 {
		actionOptions[taskOptionBranchName] = explicitBranch
	}
	if len(configuredFallbackBranch) > 0 {
		actionOptions[taskOptionConfiguredDefaultBranch] = configuredFallbackBranch
	}
	if refreshRequested {
		actionOptions[taskOptionRefreshEnabled] = true
		actionOptions[taskOptionRequireClean] = requireClean
	}
	if stashRequested {
		actionOptions[taskOptionStashChanges] = true
	}
	if commitRequested {
		actionOptions[taskOptionCommitChanges] = true
	}

	taskBranchLabel := strings.TrimSpace(explicitBranch)
	if len(taskBranchLabel) == 0 {
		taskBranchLabel = strings.TrimSpace(configuredFallbackBranch)
	}
	taskName := fmt.Sprintf("Switch branch to %s", taskBranchLabel)
	if len(taskBranchLabel) == 0 {
		taskName = "Switch branch to default branch"
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        taskName,
		EnsureClean: false,
		Actions: []workflow.TaskActionDefinition{
			{Type: taskTypeBranchChange, Options: actionOptions},
		},
	}

	runtimeOptions := workflow.RuntimeOptions{
		AssumeYes: false,
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
