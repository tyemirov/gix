package repos

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/repos/shared"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	rootutils "github.com/temirov/gix/internal/utils/roots"
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
	folderRenamePresetName        = "folder-rename"
	folderRenameCommandKey        = "folder rename"
	renamePresetLoadErrorTemplate = "unable to load folder-rename preset: %w"
	renamePresetMissingMessage    = "folder-rename preset not found"
	renameBuildOperationsError    = "unable to build folder-rename workflow: %w"
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
	PresetCatalogFactory         func() workflowcmd.PresetCatalog
	WorkflowExecutorFactory      WorkflowExecutorFactory
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

	roots, rootsError := rootutils.Resolve(command, arguments, configuration.RepositoryRoots)
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
			PrompterFactory:              builder.PrompterFactory,
		},
		taskrunner.DependenciesOptions{Command: command},
	)
	if dependencyError != nil {
		return dependencyError
	}

	workflowDependencies := dependencyResult.Workflow
	workflowDependencies.Output = command.OutOrStdout()
	workflowDependencies.Errors = command.ErrOrStderr()

	presetCatalog := builder.resolvePresetCatalog()
	presetConfiguration, presetFound, presetError := presetCatalog.Load(folderRenamePresetName)
	if presetError != nil {
		return fmt.Errorf(renamePresetLoadErrorTemplate, presetError)
	}
	if !presetFound {
		return errors.New(renamePresetMissingMessage)
	}

	for index := range presetConfiguration.Steps {
		if workflow.CommandPathKey(presetConfiguration.Steps[index].Command) != folderRenameCommandKey {
			continue
		}
		if presetConfiguration.Steps[index].Options == nil {
			presetConfiguration.Steps[index].Options = make(map[string]any)
		}
		presetConfiguration.Steps[index].Options["require_clean"] = requireClean
		presetConfiguration.Steps[index].Options["include_owner"] = includeOwner
	}

	nodes, operationsError := workflow.BuildOperations(presetConfiguration)
	if operationsError != nil {
		return fmt.Errorf(renameBuildOperationsError, operationsError)
	}
	workflow.ApplyDefaults(nodes, workflow.OperationDefaults{RequireClean: requireClean})

	runtimeOptions := workflow.RuntimeOptions{
		AssumeYes:                            assumeYes,
		IncludeNestedRepositories:            true,
		ProcessRepositoriesByDescendingDepth: true,
	}
	if requireClean {
		runtimeOptions.CaptureInitialWorktreeStatus = true
	}

	executor := ResolveWorkflowExecutor(builder.WorkflowExecutorFactory, nodes, workflowDependencies)
	_, runErr := executor.Execute(command.Context(), roots, runtimeOptions)
	return runErr
}

func (builder *RenameCommandBuilder) resolveConfiguration() RenameConfiguration {
	if builder.ConfigurationProvider == nil {
		defaults := DefaultToolsConfiguration()
		return defaults.Rename
	}

	provided := builder.ConfigurationProvider()
	return provided.sanitize()
}

func (builder *RenameCommandBuilder) resolvePresetCatalog() workflowcmd.PresetCatalog {
	if builder.PresetCatalogFactory != nil {
		if catalog := builder.PresetCatalogFactory(); catalog != nil {
			return catalog
		}
	}
	return workflowcmd.NewEmbeddedPresetCatalog()
}
