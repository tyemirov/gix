package repos

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	workflowcmd "github.com/tyemirov/gix/cmd/cli/workflow"
	"github.com/tyemirov/gix/internal/repos/shared"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	remotesUseConstant                  = "repo-remote-update"
	remotesShortDescription             = "Update origin URLs to match canonical GitHub repositories"
	remotesLongDescription              = "repo-remote-update adjusts origin remotes to point to canonical GitHub repositories."
	remotesOwnerFlagName                = "owner"
	remotesOwnerFlagDescription         = "Require canonical owner to match this value"
	remoteCanonicalPresetName           = "remote-update-to-canonical"
	remoteCanonicalCommandKey           = "remote update-to-canonical"
	remotesPresetLoadErrorTemplate      = "unable to load remote-update-to-canonical preset: %w"
	remotesPresetMissingMessage         = "remote-update-to-canonical preset not found"
	remotesBuildOperationsErrorTemplate = "unable to build remote-update-to-canonical workflow: %w"
)

// RemotesCommandBuilder assembles the repo-remote-update command.
type RemotesCommandBuilder struct {
	LoggerProvider               workflowcmd.LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	GitHubResolver               shared.GitHubMetadataResolver
	PrompterFactory              workflowcmd.PrompterFactory
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() RemotesConfiguration
	PresetCatalogFactory         func() workflowcmd.PresetCatalog
	WorkflowExecutorFactory      workflowcmd.OperationExecutorFactory
}

// Build constructs the repo-remote-update command.
func (builder *RemotesCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   remotesUseConstant,
		Short: remotesShortDescription,
		Long:  remotesLongDescription,
		Args:  cobra.NoArgs,
		RunE:  builder.run,
	}

	command.Flags().String(remotesOwnerFlagName, "", remotesOwnerFlagDescription)

	return command, nil
}

func (builder *RemotesCommandBuilder) run(command *cobra.Command, arguments []string) error {
	configuration := builder.resolveConfiguration()

	ownerConstraint := configuration.Owner
	if command != nil {
		ownerValue, ownerChanged, ownerError := flagutils.StringFlag(command, remotesOwnerFlagName)
		if ownerError != nil && !errors.Is(ownerError, flagutils.ErrFlagNotDefined) {
			return ownerError
		}
		if ownerChanged {
			ownerConstraint = strings.TrimSpace(ownerValue)
		}
	}

	request := workflowcmd.PresetCommandRequest{
		Command:                 command,
		Arguments:               arguments,
		RootArguments:           arguments,
		ConfiguredAssumeYes:     configuration.AssumeYes,
		ConfiguredRoots:         configuration.RepositoryRoots,
		PresetName:              remoteCanonicalPresetName,
		PresetMissingMessage:    remotesPresetMissingMessage,
		PresetLoadErrorTemplate: remotesPresetLoadErrorTemplate,
		BuildErrorTemplate:      remotesBuildOperationsErrorTemplate,
		Configure: func(ctx workflowcmd.PresetCommandContext) (workflowcmd.PresetCommandResult, error) {
			trimmedOwner := strings.TrimSpace(ownerConstraint)
			for index := range ctx.Configuration.Steps {
				if workflow.CommandPathKey(ctx.Configuration.Steps[index].Command) != remoteCanonicalCommandKey {
					continue
				}
				if ctx.Configuration.Steps[index].Options == nil {
					ctx.Configuration.Steps[index].Options = make(map[string]any)
				}
				ctx.Configuration.Steps[index].Options["owner"] = trimmedOwner
			}

			return workflowcmd.PresetCommandResult{
				Configuration:  ctx.Configuration,
				RuntimeOptions: ctx.RuntimeOptions(),
			}, nil
		},
	}

	return builder.presetCommand().Execute(request)
}

func (builder *RemotesCommandBuilder) resolveConfiguration() RemotesConfiguration {
	if builder.ConfigurationProvider == nil {
		defaults := DefaultToolsConfiguration()
		return defaults.Remotes
	}

	provided := builder.ConfigurationProvider()
	return provided.sanitize()
}

func (builder *RemotesCommandBuilder) presetCommand() workflowcmd.PresetCommand {
	return newPresetCommand(presetCommandDependencies{
		LoggerProvider:               builder.LoggerProvider,
		HumanReadableLoggingProvider: builder.HumanReadableLoggingProvider,
		Discoverer:                   builder.Discoverer,
		GitExecutor:                  builder.GitExecutor,
		GitManager:                   builder.GitManager,
		GitHubResolver:               builder.GitHubResolver,
		PrompterFactory:              builder.PrompterFactory,
		PresetCatalogFactory:         builder.PresetCatalogFactory,
		WorkflowExecutorFactory:      builder.WorkflowExecutorFactory,
	})
}
