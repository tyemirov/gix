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
	retagCommandUseTemplate      = "retag --map <tag=target> [--map <tag=target>...]"
	retagCommandShortDescription = "Retag existing releases to new commits"
	retagCommandLongDescription  = "retag deletes and recreates existing annotated tags so they point to the provided commits, then force-pushes the updated tags to the configured remote."
	retagCommandAlias            = "fix"
	retagMappingFlagName         = "map"
	retagMappingFlagUsage        = "Mapping of tag=target (repeatable)"
	retagMessageTemplateFlagName = "message-template"
	retagMessageTemplateUsage    = "Optional template for retag messages (placeholders: {{tag}}, {{target}})"
	retagPresetName              = "release-retag"
	retagPresetCommandKey        = "tasks apply"
	retagPresetMissingMessage    = "release-retag preset not found"
	retagPresetLoadError         = "unable to load release-retag preset: %w"
	retagBuildWorkflowError      = "unable to build release-retag workflow: %w"
)

// RetagCommandBuilder assembles the repo release retag command.
type RetagCommandBuilder struct {
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

// Build constructs the retag Cobra command.
func (builder *RetagCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   retagCommandUseTemplate,
		Short: retagCommandShortDescription,
		Long:  retagCommandLongDescription,
		Args:  cobra.NoArgs,
		RunE:  builder.run,
	}

	command.Flags().StringSlice(retagMappingFlagName, nil, retagMappingFlagUsage)
	command.Flags().String(retagMessageTemplateFlagName, "", retagMessageTemplateUsage)

	return command, nil
}

func (builder *RetagCommandBuilder) run(command *cobra.Command, _ []string) error {
	configuration := builder.resolveConfiguration()

	messageTemplate := configuration.Message
	if command != nil && command.Flags().Changed(retagMessageTemplateFlagName) {
		templateValue, templateError := command.Flags().GetString(retagMessageTemplateFlagName)
		if templateError != nil {
			return templateError
		}
		messageTemplate = strings.TrimSpace(templateValue)
	}

	mappingValues, mappingError := command.Flags().GetStringSlice(retagMappingFlagName)
	if mappingError != nil {
		return mappingError
	}
	if len(mappingValues) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return errors.New("retag requires at least one --map <tag=target> entry")
	}

	parsedMappings, parsedMappingsError := buildRetagMappings(mappingValues, messageTemplate)
	if parsedMappingsError != nil {
		return parsedMappingsError
	}

	request := workflowcmd.PresetCommandRequest{
		Command:                 command,
		Arguments:               nil,
		RootArguments:           []string{},
		ConfiguredAssumeYes:     false,
		ConfiguredRoots:         configuration.RepositoryRoots,
		PresetName:              retagPresetName,
		PresetMissingMessage:    retagPresetMissingMessage,
		PresetLoadErrorTemplate: retagPresetLoadError,
		BuildErrorTemplate:      retagBuildWorkflowError,
		DependenciesOptions: taskrunner.DependenciesOptions{
			DisablePrompter: true,
		},
		Configure: func(ctx workflowcmd.PresetCommandContext) (workflowcmd.PresetCommandResult, error) {
			params := releaseRetagPresetOptions{
				Mappings: parsedMappings,
				Remote:   configuration.RemoteName,
			}

			if ctx.ExecutionFlagsAvailable && ctx.ExecutionFlags.RemoteSet {
				override := strings.TrimSpace(ctx.ExecutionFlags.Remote)
				if len(override) > 0 {
					params.Remote = override
				}
			}

			for index := range ctx.Configuration.Steps {
				if workflow.CommandPathKey(ctx.Configuration.Steps[index].Command) != retagPresetCommandKey {
					continue
				}
				updateReleaseRetagPresetOptions(ctx.Configuration.Steps[index].Options, params)
			}

			return workflowcmd.PresetCommandResult{
				Configuration:  ctx.Configuration,
				RuntimeOptions: ctx.RuntimeOptions(),
			}, nil
		},
	}

	return builder.presetCommand().Execute(request)
}

func (builder *RetagCommandBuilder) resolveConfiguration() CommandConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultCommandConfiguration()
	}
	return builder.ConfigurationProvider().Sanitize()
}

func (builder *RetagCommandBuilder) presetCommand() workflowcmd.PresetCommand {
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

func (builder *RetagCommandBuilder) presetCommand() workflowcmd.PresetCommand {
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

type releaseRetagPresetOptions struct {
	Mappings []any
	Remote   string
}

func updateReleaseRetagPresetOptions(options map[string]any, params releaseRetagPresetOptions) {
	if options == nil {
		return
	}
	tasks, ok := options["tasks"].([]any)
	if !ok || len(tasks) == 0 {
		return
	}
	taskEntry, ok := tasks[0].(map[string]any)
	if !ok {
		return
	}
	taskEntry["name"] = "Retag release tags"
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
	actionOptions["mappings"] = params.Mappings
	if len(strings.TrimSpace(params.Remote)) > 0 {
		actionOptions["remote"] = strings.TrimSpace(params.Remote)
	} else {
		delete(actionOptions, "remote")
	}
	actionEntry["options"] = actionOptions
	actionEntries[0] = actionEntry
	taskEntry["actions"] = actionEntries
	tasks[0] = taskEntry
	options["tasks"] = tasks
}

func buildRetagMappings(rawMappings []string, messageTemplate string) ([]any, error) {
	mappings := make([]any, 0, len(rawMappings))
	templateValue := strings.TrimSpace(messageTemplate)
	for _, raw := range rawMappings {
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --map value %q: expected <tag=target>", raw)
		}
		tag := strings.TrimSpace(parts[0])
		target := strings.TrimSpace(parts[1])
		if len(tag) == 0 || len(target) == 0 {
			return nil, fmt.Errorf("invalid --map value %q: tag and target are required", raw)
		}
		entry := map[string]any{
			"tag":    tag,
			"target": target,
		}
		if len(templateValue) > 0 {
			message := strings.ReplaceAll(templateValue, "{{tag}}", tag)
			message = strings.ReplaceAll(message, "{{target}}", target)
			entry["message"] = message
		}
		mappings = append(mappings, entry)
	}
	return mappings, nil
}
