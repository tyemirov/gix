package repos

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	workflowcmd "github.com/tyemirov/gix/cmd/cli/workflow"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	protocolUseConstant         = "repo-protocol-convert"
	protocolShortDescription    = "Convert repository origin URLs between git/ssh/https"
	protocolLongDescription     = "repo-protocol-convert converts origin URLs to a desired protocol."
	protocolFromFlagName        = "from"
	protocolFromFlagDescription = "Current protocol to convert from (git, ssh, https)"
	protocolToFlagName          = "to"
	protocolToFlagDescription   = "Target protocol to convert to (git, ssh, https)"
	protocolErrorMissingPair    = "specify both --from and --to"
	protocolErrorSamePair       = "--from and --to must differ"
	protocolErrorInvalidValue   = "invalid protocol value: %s"
	protocolPresetName          = "remote-update-protocol"
	protocolCommandKey          = "remote update-protocol"
	protocolPresetMissingError  = "remote-update-protocol preset not found"
	protocolPresetLoadError     = "unable to load remote-update-protocol preset: %w"
	protocolBuildWorkflowError  = "unable to build remote-update-protocol workflow: %w"
)

// ProtocolCommandBuilder assembles the repo-protocol-convert command.
type ProtocolCommandBuilder struct {
	LoggerProvider               workflowcmd.LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	GitHubResolver               shared.GitHubMetadataResolver
	PrompterFactory              workflowcmd.PrompterFactory
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() ProtocolConfiguration
	PresetCatalogFactory         func() workflowcmd.PresetCatalog
	WorkflowExecutorFactory      workflowcmd.OperationExecutorFactory
}

// Build constructs the repo-protocol-convert command.
func (builder *ProtocolCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   protocolUseConstant,
		Short: protocolShortDescription,
		Long:  protocolLongDescription,
		Args:  cobra.NoArgs,
		RunE:  builder.run,
	}

	command.Flags().String(protocolFromFlagName, "", protocolFromFlagDescription)
	command.Flags().String(protocolToFlagName, "", protocolToFlagDescription)

	return command, nil
}

func (builder *ProtocolCommandBuilder) run(command *cobra.Command, arguments []string) error {
	configuration := builder.resolveConfiguration()

	fromValue := configuration.FromProtocol
	if command != nil && command.Flags().Changed(protocolFromFlagName) {
		fromValue, _ = command.Flags().GetString(protocolFromFlagName)
	}

	toValue := configuration.ToProtocol
	if command != nil && command.Flags().Changed(protocolToFlagName) {
		toValue, _ = command.Flags().GetString(protocolToFlagName)
	}

	if len(strings.TrimSpace(fromValue)) == 0 || len(strings.TrimSpace(toValue)) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return errors.New(protocolErrorMissingPair)
	}

	fromProtocol, fromError := parseProtocolValue(fromValue)
	if fromError != nil {
		return fromError
	}

	toProtocol, toError := parseProtocolValue(toValue)
	if toError != nil {
		return toError
	}

	if fromProtocol == toProtocol {
		return errors.New(protocolErrorSamePair)
	}

	request := workflowcmd.PresetCommandRequest{
		Command:                 command,
		Arguments:               arguments,
		RootArguments:           arguments,
		ConfiguredAssumeYes:     configuration.AssumeYes,
		ConfiguredRoots:         configuration.RepositoryRoots,
		PresetName:              protocolPresetName,
		PresetMissingMessage:    protocolPresetMissingError,
		PresetLoadErrorTemplate: protocolPresetLoadError,
		BuildErrorTemplate:      protocolBuildWorkflowError,
		Configure: func(ctx workflowcmd.PresetCommandContext) (workflowcmd.PresetCommandResult, error) {
			for index := range ctx.Configuration.Steps {
				if workflow.CommandPathKey(ctx.Configuration.Steps[index].Command) != protocolCommandKey {
					continue
				}
				if ctx.Configuration.Steps[index].Options == nil {
					ctx.Configuration.Steps[index].Options = make(map[string]any)
				}
				ctx.Configuration.Steps[index].Options["from"] = string(fromProtocol)
				ctx.Configuration.Steps[index].Options["to"] = string(toProtocol)
			}

			return workflowcmd.PresetCommandResult{
				Configuration:  ctx.Configuration,
				RuntimeOptions: ctx.RuntimeOptions(),
			}, nil
		},
	}

	return builder.presetCommand().Execute(request)
}

func (builder *ProtocolCommandBuilder) resolveConfiguration() ProtocolConfiguration {
	if builder.ConfigurationProvider == nil {
		defaults := DefaultToolsConfiguration()
		return defaults.Protocol
	}

	provided := builder.ConfigurationProvider()
	return provided.sanitize()
}

func (builder *ProtocolCommandBuilder) presetCommand() workflowcmd.PresetCommand {
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

func parseProtocolValue(value string) (shared.RemoteProtocol, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	switch trimmed {
	case string(shared.RemoteProtocolGit):
		return shared.RemoteProtocolGit, nil
	case string(shared.RemoteProtocolSSH):
		return shared.RemoteProtocolSSH, nil
	case string(shared.RemoteProtocolHTTPS):
		return shared.RemoteProtocolHTTPS, nil
	default:
		return "", fmt.Errorf(protocolErrorInvalidValue, value)
	}
}
