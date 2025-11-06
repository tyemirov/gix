package repos

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/gitrepo"
	"github.com/temirov/gix/internal/repos/dependencies"
	"github.com/temirov/gix/internal/repos/shared"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
)

const (
	renameUseConstant             = "repo-folders-rename"
	renameShortDescription        = "Rename repository directories to match canonical GitHub names"
	renameLongDescription         = "repo-folders-rename normalizes repository directory names to match canonical GitHub repositories."
	renameRequireCleanFlagName    = "require-clean"
	renameRequireCleanDescription = "Require clean worktrees before applying renames"
	renameIncludeOwnerFlagName    = "owner"
	renameIncludeOwnerDescription = "Include repository owner in the target directory path"
)

// RenameCommandBuilder assembles the repo-folders-rename command.
type RenameCommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	GitHubResolver               shared.GitHubMetadataResolver
	FileSystem                   shared.FileSystem
	PrompterFactory              PrompterFactory
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() RenameConfiguration
	TaskRunnerFactory            func(workflow.Dependencies) TaskRunnerExecutor
}

// Build constructs the repo-folders-rename command.
func (builder *RenameCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   renameUseConstant,
		Short: renameShortDescription,
		Long:  renameLongDescription,
		Args:  cobra.NoArgs,
		RunE:  builder.run,
	}

	flagutils.AddToggleFlag(command.Flags(), nil, renameRequireCleanFlagName, "", false, renameRequireCleanDescription)
	flagutils.AddToggleFlag(command.Flags(), nil, renameIncludeOwnerFlagName, "", false, renameIncludeOwnerDescription)

	return command, nil
}

func (builder *RenameCommandBuilder) run(command *cobra.Command, arguments []string) error {
	configuration := builder.resolveConfiguration()
	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

	assumeYes := configuration.AssumeYes
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

	requireClean := configuration.RequireCleanWorktree
	if command != nil {
		requireCleanFlagValue, requireCleanFlagChanged, requireCleanFlagError := flagutils.BoolFlag(command, renameRequireCleanFlagName)
		if requireCleanFlagError != nil && !errors.Is(requireCleanFlagError, flagutils.ErrFlagNotDefined) {
			return requireCleanFlagError
		}
		if requireCleanFlagChanged {
			requireClean = requireCleanFlagValue
		}
	}

	includeOwner := configuration.IncludeOwner
	if command != nil {
		includeOwnerFlagValue, includeOwnerFlagChanged, includeOwnerFlagError := flagutils.BoolFlag(command, renameIncludeOwnerFlagName)
		if includeOwnerFlagError != nil && !errors.Is(includeOwnerFlagError, flagutils.ErrFlagNotDefined) {
			return includeOwnerFlagError
		}
		if includeOwnerFlagChanged {
			includeOwner = includeOwnerFlagValue
		}
	}

	roots, rootsError := requireRepositoryRoots(command, arguments, configuration.RepositoryRoots)
	if rootsError != nil {
		return rootsError
	}

	logger := resolveLogger(builder.LoggerProvider)
	humanReadableLogging := false
	if builder.HumanReadableLoggingProvider != nil {
		humanReadableLogging = builder.HumanReadableLoggingProvider()
	}
	gitExecutor, executorError := dependencies.ResolveGitExecutor(builder.GitExecutor, logger, humanReadableLogging)
	if executorError != nil {
		return executorError
	}

	gitManager, managerError := dependencies.ResolveGitRepositoryManager(builder.GitManager, gitExecutor)
	if managerError != nil {
		return managerError
	}

	resolvedManager := gitManager
	repositoryManager := (*gitrepo.RepositoryManager)(nil)
	if concreteManager, ok := resolvedManager.(*gitrepo.RepositoryManager); ok {
		repositoryManager = concreteManager
	} else {
		constructedManager, constructedManagerError := gitrepo.NewRepositoryManager(gitExecutor)
		if constructedManagerError != nil {
			return constructedManagerError
		}
		repositoryManager = constructedManager
	}

	repositoryDiscoverer := dependencies.ResolveRepositoryDiscoverer(builder.Discoverer)
	fileSystem := dependencies.ResolveFileSystem(builder.FileSystem)

	prompter := resolvePrompter(builder.PrompterFactory, command)
	trackingPrompter := newCascadingConfirmationPrompter(prompter, assumeYes)

	githubClient, githubClientError := githubcli.NewClient(gitExecutor)
	if githubClientError != nil {
		return githubClientError
	}

	taskDependencies := workflow.Dependencies{
		Logger:               logger,
		RepositoryDiscoverer: repositoryDiscoverer,
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         githubClient,
		FileSystem:           fileSystem,
		Prompter:             trackingPrompter,
		Output:               command.OutOrStdout(),
		Errors:               command.ErrOrStderr(),
	}

	taskRunner := ResolveTaskRunner(builder.TaskRunnerFactory, taskDependencies)

	actionOptions := map[string]any{
		"require_clean": requireClean,
		"include_owner": includeOwner,
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        "Rename repository directories",
		EnsureClean: false,
		Actions: []workflow.TaskActionDefinition{
			{Type: "repo.folder.rename", Options: actionOptions},
		},
		Commit: workflow.TaskCommitDefinition{},
	}

	runtimeOptions := workflow.RuntimeOptions{
		AssumeYes:                            trackingPrompter.AssumeYes(),
		IncludeNestedRepositories:            true,
		ProcessRepositoriesByDescendingDepth: true,
		CaptureInitialWorktreeStatus:         requireClean,
	}

	return taskRunner.Run(command.Context(), roots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
}

func (builder *RenameCommandBuilder) resolveConfiguration() RenameConfiguration {
	if builder.ConfigurationProvider == nil {
		defaults := DefaultToolsConfiguration()
		return defaults.Rename
	}

	provided := builder.ConfigurationProvider()
	return provided.sanitize()
}
