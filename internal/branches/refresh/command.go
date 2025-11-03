package refresh

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/dependencies"
	"github.com/tyemirov/gix/internal/repos/shared"
	rootutils "github.com/tyemirov/gix/internal/utils/roots"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	commandUseConstant                      = "branch-refresh"
	commandShortDescriptionConstant         = "Fetch, checkout, and pull a branch"
	commandLongDescriptionConstant          = "branch-refresh synchronizes a repository branch by fetching updates, checking out the branch, and pulling the latest changes."
	stashFlagNameConstant                   = "stash"
	stashFlagDescriptionConstant            = "Stash local changes before refreshing the branch"
	commitFlagNameConstant                  = "commit"
	commitFlagDescriptionConstant           = "Commit local changes before refreshing the branch"
	missingBranchNameMessageConstant        = "branch name is required; supply --branch"
	conflictingRecoveryFlagsMessageConstant = "use at most one of --stash or --commit"
	branchFlagNameConstant                  = "branch"
	branchFlagDescriptionConstant           = "Branch name to refresh"
	refreshSuccessMessageTemplateConstant   = "REFRESHED: %s (%s)\n"
	taskActionBranchRefreshType             = "branch.refresh"
)

// LoggerProvider yields a zap logger for command execution.
type LoggerProvider func() *zap.Logger

// CommandBuilder assembles the branch-refresh command.
type CommandBuilder struct {
	LoggerProvider               LoggerProvider
	GitExecutor                  shared.GitExecutor
	GitRepositoryManager         shared.GitRepositoryManager
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() CommandConfiguration
	Discoverer                   shared.RepositoryDiscoverer
	FileSystem                   shared.FileSystem
	TaskRunnerFactory            func(workflow.Dependencies) TaskRunnerExecutor
}

// Build constructs the branch-refresh command.
func (builder *CommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   commandUseConstant,
		Short: commandShortDescriptionConstant,
		Long:  commandLongDescriptionConstant,
		Args:  cobra.NoArgs,
		RunE:  builder.run,
	}

	command.Flags().Bool(stashFlagNameConstant, false, stashFlagDescriptionConstant)
	command.Flags().Bool(commitFlagNameConstant, false, commitFlagDescriptionConstant)
	command.Flags().String(branchFlagNameConstant, "", branchFlagDescriptionConstant)

	return command, nil
}

func (builder *CommandBuilder) run(command *cobra.Command, arguments []string) error {
	configuration := builder.resolveConfiguration()

	branchName := strings.TrimSpace(configuration.BranchName)
	if command != nil {
		if branchFlagValue, flagError := command.Flags().GetString(branchFlagNameConstant); flagError == nil && command.Flags().Changed(branchFlagNameConstant) {
			branchName = strings.TrimSpace(branchFlagValue)
		} else if flagError != nil {
			return flagError
		}
	}
	if len(branchName) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return errors.New(missingBranchNameMessageConstant)
	}

	stashRequested, stashFlagError := command.Flags().GetBool(stashFlagNameConstant)
	if stashFlagError != nil {
		return stashFlagError
	}
	commitRequested, commitFlagError := command.Flags().GetBool(commitFlagNameConstant)
	if commitFlagError != nil {
		return commitFlagError
	}
	if stashRequested && commitRequested {
		return errors.New(conflictingRecoveryFlagsMessageConstant)
	}

	repositoryRoots, rootsError := rootutils.Resolve(command, arguments, configuration.RepositoryRoots)
	if rootsError != nil {
		return rootsError
	}

	logger := builder.resolveLogger()
	humanReadableLogging := false
	if builder.HumanReadableLoggingProvider != nil {
		humanReadableLogging = builder.HumanReadableLoggingProvider()
	}
	gitExecutor, executorError := dependencies.ResolveGitExecutor(builder.GitExecutor, logger, humanReadableLogging)
	if executorError != nil {
		return executorError
	}

	repositoryManager, managerError := dependencies.ResolveGitRepositoryManager(builder.GitRepositoryManager, gitExecutor)
	if managerError != nil {
		return managerError
	}

	var concreteManager *gitrepo.RepositoryManager
	if typedManager, ok := repositoryManager.(*gitrepo.RepositoryManager); ok {
		concreteManager = typedManager
	} else {
		constructedManager, constructedManagerError := gitrepo.NewRepositoryManager(gitExecutor)
		if constructedManagerError != nil {
			return constructedManagerError
		}
		concreteManager = constructedManager
	}

	repositoryDiscoverer := dependencies.ResolveRepositoryDiscoverer(builder.Discoverer)
	fileSystem := dependencies.ResolveFileSystem(builder.FileSystem)

	gitHubClient, clientError := githubcli.NewClient(gitExecutor)
	if clientError != nil {
		return clientError
	}

	taskDependencies := workflow.Dependencies{
		Logger:               logger,
		RepositoryDiscoverer: repositoryDiscoverer,
		GitExecutor:          gitExecutor,
		RepositoryManager:    concreteManager,
		GitHubClient:         gitHubClient,
		FileSystem:           fileSystem,
		Prompter:             nil,
		Output:               command.OutOrStdout(),
		Errors:               command.ErrOrStderr(),
	}

	taskRunner := resolveTaskRunner(builder.TaskRunnerFactory, taskDependencies)

	actionOptions := map[string]any{
		"branch":        branchName,
		"stash":         stashRequested,
		"commit":        commitRequested,
		"require_clean": true,
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        fmt.Sprintf("Refresh branch %s", branchName),
		EnsureClean: false,
		Actions: []workflow.TaskActionDefinition{
			{Type: taskActionBranchRefreshType, Options: actionOptions},
		},
	}

	runtimeOptions := workflow.RuntimeOptions{DryRun: false, AssumeYes: false}

	return taskRunner.Run(command.Context(), repositoryRoots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
}

func (builder *CommandBuilder) resolveConfiguration() CommandConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultCommandConfiguration()
	}
	return builder.ConfigurationProvider().Sanitize()
}

func (builder *CommandBuilder) resolveLogger() *zap.Logger {
	if builder.LoggerProvider == nil {
		return zap.NewNop()
	}
	logger := builder.LoggerProvider()
	if logger == nil {
		return zap.NewNop()
	}
	return logger
}
