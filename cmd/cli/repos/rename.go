package repos

import (
	"errors"

	"github.com/spf13/cobra"

	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
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
	folderRenamePresetName        = "folder-rename"
	folderRenameCommandKey        = "folder rename"
	renamePresetLoadErrorTemplate = "unable to load folder-rename preset: %w"
	renamePresetMissingMessage    = "folder-rename preset not found"
	renameBuildOperationsError    = "unable to build folder-rename workflow: %w"
)

// RenameCommandBuilder assembles the repo-folders-rename command.
type RenameCommandBuilder struct {
	LoggerProvider               workflowcmd.LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	GitHubResolver               shared.GitHubMetadataResolver
	FileSystem                   shared.FileSystem
	PrompterFactory              workflowcmd.PrompterFactory
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() RenameConfiguration
	PresetCatalogFactory         func() workflowcmd.PresetCatalog
	WorkflowExecutorFactory      workflowcmd.OperationExecutorFactory
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

	request := workflowcmd.PresetCommandRequest{
		Command:                 command,
		Arguments:               arguments,
		RootArguments:           arguments,
		ConfiguredAssumeYes:     configuration.AssumeYes,
		ConfiguredRoots:         configuration.RepositoryRoots,
		PresetName:              folderRenamePresetName,
		PresetMissingMessage:    renamePresetMissingMessage,
		PresetLoadErrorTemplate: renamePresetLoadErrorTemplate,
		BuildErrorTemplate:      renameBuildOperationsError,
		Configure: func(ctx workflowcmd.PresetCommandContext) (workflowcmd.PresetCommandResult, error) {
			for index := range ctx.Configuration.Steps {
				if workflow.CommandPathKey(ctx.Configuration.Steps[index].Command) != folderRenameCommandKey {
					continue
				}
				if ctx.Configuration.Steps[index].Options == nil {
					ctx.Configuration.Steps[index].Options = make(map[string]any)
				}
				ctx.Configuration.Steps[index].Options["require_clean"] = requireClean
				ctx.Configuration.Steps[index].Options["include_owner"] = includeOwner
			}

			runtimeOptions := ctx.RuntimeOptions()
			runtimeOptions.IncludeNestedRepositories = true
			runtimeOptions.ProcessRepositoriesByDescendingDepth = true
			if requireClean {
				runtimeOptions.CaptureInitialWorktreeStatus = true
			}

			prepare := func(nodes []*workflow.OperationNode) error {
				workflow.ApplyDefaults(nodes, workflow.OperationDefaults{RequireClean: requireClean})
				return nil
			}

			return workflowcmd.PresetCommandResult{
				Configuration:     ctx.Configuration,
				RuntimeOptions:    runtimeOptions,
				PrepareOperations: prepare,
			}, nil
		},
	}

	return builder.presetCommand().Execute(request)
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

func (builder *RenameCommandBuilder) presetCommand() workflowcmd.PresetCommand {
	return newPresetCommand(presetCommandDependencies{
		LoggerProvider:               builder.LoggerProvider,
		HumanReadableLoggingProvider: builder.HumanReadableLoggingProvider,
		Discoverer:                   builder.Discoverer,
		GitExecutor:                  builder.GitExecutor,
		GitManager:                   builder.GitManager,
		GitHubResolver:               builder.GitHubResolver,
		FileSystem:                   builder.FileSystem,
		PrompterFactory:              builder.PrompterFactory,
		PresetCatalogFactory:         builder.PresetCatalogFactory,
		WorkflowExecutorFactory:      builder.WorkflowExecutorFactory,
	})
}
