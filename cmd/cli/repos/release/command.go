package release

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/repos/shared"
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
)

const (
	commandUseName          = "release"
	commandUsageTemplate    = commandUseName + " <tag>"
	commandExampleTemplate  = "gix release v1.2.3 --roots ~/Development"
	commandShortDescription = "Create and push an annotated release tag"
	commandLongDescription  = "release annotates the provided tag (default message 'Release <tag>') and pushes it to the configured remote for each repository root. Provide the tag as the first argument before any optional repository roots or flags."
	messageFlagName         = "message"
	messageFlagUsage        = "Override the tag message"
	missingTagErrorMessage  = "tag name is required"
	releasePresetName       = "release-tag"
	releasePresetCommandKey = "tasks apply"
	releasePresetMissingMsg = "release-tag preset not found"
	releasePresetLoadError  = "unable to load release-tag preset: %w"
	releaseBuildWorkflowErr = "unable to build release-tag workflow: %w"
)

// CommandBuilder assembles the release command.
type CommandBuilder struct {
	LoggerProvider               workflowcmd.LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() CommandConfiguration
	PresetCatalogFactory         func() workflowcmd.PresetCatalog
	WorkflowExecutorFactory      workflowcmd.OperationExecutorFactory
}

// Build constructs the repo release command.
func (builder *CommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:     commandUsageTemplate,
		Short:   commandShortDescription,
		Long:    commandLongDescription,
		Example: commandExampleTemplate,
		Args:    cobra.ArbitraryArgs,
		RunE:    builder.run,
	}

	command.Flags().String(messageFlagName, "", messageFlagUsage)

	return command, nil
}

func (builder *CommandBuilder) run(command *cobra.Command, arguments []string) error {
	configuration := builder.resolveConfiguration()

	if len(arguments) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return errors.New(missingTagErrorMessage)
	}

	tagName := strings.TrimSpace(arguments[0])
	if len(tagName) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return errors.New(missingTagErrorMessage)
	}
	additionalArgs := arguments[1:]

	messageValue := configuration.Message
	if command != nil {
		if flagValue, flagError := command.Flags().GetString(messageFlagName); flagError == nil && command.Flags().Changed(messageFlagName) {
			messageValue = strings.TrimSpace(flagValue)
		}
	}

	request := workflowcmd.PresetCommandRequest{
		Command:                 command,
		Arguments:               arguments,
		RootArguments:           additionalArgs,
		ConfiguredAssumeYes:     false,
		ConfiguredRoots:         configuration.RepositoryRoots,
		PresetName:              releasePresetName,
		PresetMissingMessage:    releasePresetMissingMsg,
		PresetLoadErrorTemplate: releasePresetLoadError,
		BuildErrorTemplate:      releaseBuildWorkflowErr,
		DependenciesOptions: taskrunner.DependenciesOptions{
			DisablePrompter: true,
		},
		Configure: func(ctx workflowcmd.PresetCommandContext) (workflowcmd.PresetCommandResult, error) {
			resolvedRemote := configuration.RemoteName
			if ctx.ExecutionFlagsAvailable && ctx.ExecutionFlags.RemoteSet {
				override := strings.TrimSpace(ctx.ExecutionFlags.Remote)
				if len(override) > 0 {
					resolvedRemote = override
				}
			}

			for index := range ctx.Configuration.Steps {
				if workflow.CommandPathKey(ctx.Configuration.Steps[index].Command) != releasePresetCommandKey {
					continue
				}
				updateReleasePresetOptions(
					ctx.Configuration.Steps[index].Options,
					tagName,
					messageValue,
					resolvedRemote,
				)
			}

			return workflowcmd.PresetCommandResult{
				Configuration:  ctx.Configuration,
				RuntimeOptions: ctx.RuntimeOptions(),
			}, nil
		},
	}

	return builder.presetCommand().Execute(request)
}

func (builder *CommandBuilder) resolveConfiguration() CommandConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultCommandConfiguration()
	}
	return builder.ConfigurationProvider().Sanitize()
}

func (builder *CommandBuilder) presetCommand() workflowcmd.PresetCommand {
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

func (builder *CommandBuilder) presetCommand() workflowcmd.PresetCommand {
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

func updateReleasePresetOptions(options map[string]any, tagName string, message string, remote string) {
	if options == nil {
		return
	}
	taskEntries, ok := options["tasks"].([]any)
	if !ok || len(taskEntries) == 0 {
		return
	}
	taskEntry, ok := taskEntries[0].(map[string]any)
	if !ok {
		return
	}

	displayName := "Create release tag"
	trimmedTag := strings.TrimSpace(tagName)
	if len(trimmedTag) > 0 {
		displayName = fmt.Sprintf("Create release tag %s", trimmedTag)
	}
	taskEntry["name"] = displayName
	taskEntry["ensure_clean"] = false

	actionEntries, ok := taskEntry["actions"].([]any)
	if !ok || len(actionEntries) == 0 {
		return
	}
	actionEntry, ok := actionEntries[0].(map[string]any)
	if !ok {
		return
	}
	actionOptions, _ := actionEntry["options"].(map[string]any)
	if actionOptions == nil {
		actionOptions = make(map[string]any)
	}
	actionOptions["tag"] = trimmedTag
	if len(strings.TrimSpace(message)) > 0 {
		actionOptions["message"] = strings.TrimSpace(message)
	} else {
		delete(actionOptions, "message")
	}
	if len(strings.TrimSpace(remote)) > 0 {
		actionOptions["remote"] = strings.TrimSpace(remote)
	} else {
		delete(actionOptions, "remote")
	}
	actionEntry["options"] = actionOptions
	actionEntries[0] = actionEntry
	taskEntry["actions"] = actionEntries
	taskEntries[0] = taskEntry
	options["tasks"] = taskEntries
}
