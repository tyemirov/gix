package repos

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/temirov/gix/internal/repos/shared"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
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

	dependencyResult, dependencyError := buildDependencies(
		command,
		dependencyInputs{
			LoggerProvider:               builder.LoggerProvider,
			HumanReadableLoggingProvider: builder.HumanReadableLoggingProvider,
			Discoverer:                   builder.Discoverer,
			GitExecutor:                  builder.GitExecutor,
			GitManager:                   builder.GitManager,
			GitHubResolver:               builder.GitHubResolver,
			FileSystem:                   builder.FileSystem,
			PrompterFactory:              builder.PrompterFactory,
		},
		taskrunner.DependenciesOptions{},
	)
	if dependencyError != nil {
		return dependencyError
	}

	taskDependencies := dependencyResult.Workflow
	trackingPrompter := newCascadingConfirmationPrompter(taskDependencies.Prompter, assumeYes)
	taskDependencies.Prompter = trackingPrompter
	taskDependencies.Output = command.OutOrStdout()
	taskDependencies.Errors = command.ErrOrStderr()

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
