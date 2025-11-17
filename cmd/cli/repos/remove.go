package repos

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/repos/shared"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
)

const (
	removeUseConstant              = "repo-history-remove"
	removeShortDescription         = "Rewrite repository history to remove selected paths"
	removeLongDescription          = "repo-history-remove purges the specified paths from repository history using git-filter-repo, optionally force-pushing updates and restoring upstream tracking."
	removeRemoteFlagName           = "remote"
	removeRemoteFlagDescription    = "Remote to use when pushing after purge (auto-detected when omitted)"
	removePushFlagName             = "push"
	removePushFlagDescription      = "Force push rewritten history to the configured remote"
	removeRestoreFlagName          = "restore"
	removeRestoreFlagDescription   = "Restore upstream tracking for local branches after purge"
	removePushMissingFlagName      = "push-missing"
	removePushMissingDescription   = "Create missing remote branches when restoring upstreams"
	removeMissingPathsErrorMessage = "history purge requires at least one path argument"
	historyRemovePresetName        = "history-remove"
	historyRemoveCommandKey        = "tasks apply"
	historyPresetMissingMessage    = "history-remove preset not found"
	historyPresetLoadErrorTemplate = "unable to load history-remove preset: %w"
	historyBuildWorkflowError      = "unable to build history-remove workflow: %w"
)

// RemoveCommandBuilder assembles the repo-history-remove command.
type RemoveCommandBuilder struct {
	LoggerProvider               workflowcmd.LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() RemoveConfiguration
	PresetCatalogFactory         func() workflowcmd.PresetCatalog
	WorkflowExecutorFactory      workflowcmd.OperationExecutorFactory
}

// Build constructs the repo-history-remove command.
func (builder *RemoveCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   removeUseConstant,
		Short: removeShortDescription,
		Long:  removeLongDescription,
		RunE:  builder.run,
	}

	command.Flags().String(removeRemoteFlagName, "", removeRemoteFlagDescription)
	flagutils.AddToggleFlag(command.Flags(), nil, removePushFlagName, "", true, removePushFlagDescription)
	flagutils.AddToggleFlag(command.Flags(), nil, removeRestoreFlagName, "", true, removeRestoreFlagDescription)
	flagutils.AddToggleFlag(command.Flags(), nil, removePushMissingFlagName, "", false, removePushMissingDescription)

	return command, nil
}

func (builder *RemoveCommandBuilder) run(command *cobra.Command, arguments []string) error {
	if len(arguments) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return errors.New(removeMissingPathsErrorMessage)
	}

	configuration := builder.resolveConfiguration()

	remoteName := configuration.Remote
	if command != nil && command.Flags().Changed(removeRemoteFlagName) {
		flagValue, flagError := command.Flags().GetString(removeRemoteFlagName)
		if flagError != nil {
			return flagError
		}
		remoteName = strings.TrimSpace(flagValue)
	}

	pushEnabled := configuration.Push
	if command != nil {
		flagValue, flagChanged, flagError := flagutils.BoolFlag(command, removePushFlagName)
		if flagError != nil && !errors.Is(flagError, flagutils.ErrFlagNotDefined) {
			return flagError
		}
		if flagChanged {
			pushEnabled = flagValue
		}
	}

	restoreEnabled := configuration.Restore
	if command != nil {
		flagValue, flagChanged, flagError := flagutils.BoolFlag(command, removeRestoreFlagName)
		if flagError != nil && !errors.Is(flagError, flagutils.ErrFlagNotDefined) {
			return flagError
		}
		if flagChanged {
			restoreEnabled = flagValue
		}
	}

	pushMissing := configuration.PushMissing
	if command != nil {
		flagValue, flagChanged, flagError := flagutils.BoolFlag(command, removePushMissingFlagName)
		if flagError != nil && !errors.Is(flagError, flagutils.ErrFlagNotDefined) {
			return flagError
		}
		if flagChanged {
			pushMissing = flagValue
		}
	}

	normalizedPaths := make([]string, 0, len(arguments))
	for _, pathArgument := range arguments {
		trimmed := strings.TrimSpace(pathArgument)
		if len(trimmed) == 0 {
			continue
		}
		normalized := strings.TrimPrefix(trimmed, "./")
		if len(normalized) == 0 {
			continue
		}
		normalizedPaths = append(normalizedPaths, normalized)
	}
	if len(normalizedPaths) == 0 {
		return errors.New(removeMissingPathsErrorMessage)
	}

	request := workflowcmd.PresetCommandRequest{
		Command:                 command,
		Arguments:               arguments,
		RootArguments:           []string{},
		ConfiguredAssumeYes:     configuration.AssumeYes,
		ConfiguredRoots:         configuration.RepositoryRoots,
		PresetName:              historyRemovePresetName,
		PresetMissingMessage:    historyPresetMissingMessage,
		PresetLoadErrorTemplate: historyPresetLoadErrorTemplate,
		BuildErrorTemplate:      historyBuildWorkflowError,
		Configure: func(ctx workflowcmd.PresetCommandContext) (workflowcmd.PresetCommandResult, error) {
			taskDefinition := workflow.TaskDefinition{
				Name:        "Remove repository history paths",
				EnsureClean: true,
				Actions: []workflow.TaskActionDefinition{
					{
						Type: "repo.history.purge",
						Options: map[string]any{
							"paths":        append([]string{}, normalizedPaths...),
							"remote":       strings.TrimSpace(remoteName),
							"push":         pushEnabled,
							"restore":      restoreEnabled,
							"push_missing": pushMissing,
						},
					},
				},
			}

			for index := range ctx.Configuration.Steps {
				if workflow.CommandPathKey(ctx.Configuration.Steps[index].Command) != historyRemoveCommandKey {
					continue
				}
				ctx.Configuration.Steps[index].Options = workflow.TasksApplyDefinition{
					Tasks: []workflow.TaskDefinition{taskDefinition},
				}.Options()
			}

			return workflowcmd.PresetCommandResult{
				Configuration:  ctx.Configuration,
				RuntimeOptions: ctx.RuntimeOptions(),
			}, nil
		},
	}

	return builder.presetCommand().Execute(request)
}

func (builder *RemoveCommandBuilder) resolveConfiguration() RemoveConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultToolsConfiguration().Remove
	}

	return builder.ConfigurationProvider().sanitize()
}

func (builder *RemoveCommandBuilder) presetCommand() workflowcmd.PresetCommand {
	return newPresetCommand(presetCommandDependencies{
		LoggerProvider:               builder.LoggerProvider,
		HumanReadableLoggingProvider: builder.HumanReadableLoggingProvider,
		Discoverer:                   builder.Discoverer,
		GitExecutor:                  builder.GitExecutor,
		GitManager:                   builder.GitManager,
		FileSystem:                   builder.FileSystem,
		PresetCatalogFactory:         builder.PresetCatalogFactory,
		WorkflowExecutorFactory:      builder.WorkflowExecutorFactory,
	})
}
