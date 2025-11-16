package release

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	repocli "github.com/temirov/gix/cmd/cli/repos"
	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/repos/shared"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	rootutils "github.com/temirov/gix/internal/utils/roots"
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
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() CommandConfiguration
	PresetCatalogFactory         func() workflowcmd.PresetCatalog
	WorkflowExecutorFactory      repocli.WorkflowExecutorFactory
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

	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

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

	repositoryRoots, rootsError := rootutils.Resolve(command, nil, configuration.RepositoryRoots)
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
			FileSystem:                   builder.FileSystem,
		},
		taskrunner.DependenciesOptions{
			Command:         command,
			DisablePrompter: true,
		},
	)
	if dependencyError != nil {
		return dependencyError
	}

	workflowDependencies := dependencyResult.Workflow
	if command != nil {
		workflowDependencies.Output = command.OutOrStdout()
		workflowDependencies.Errors = command.ErrOrStderr()
	}

	presetCatalog := builder.resolvePresetCatalog()
	presetConfiguration, presetFound, presetError := presetCatalog.Load(retagPresetName)
	if presetError != nil {
		return fmt.Errorf(retagPresetLoadError, presetError)
	}
	if !presetFound {
		return errors.New(retagPresetMissingMessage)
	}

	parsedMappings, parsedMappingsError := buildRetagMappings(mappingValues, messageTemplate)
	if parsedMappingsError != nil {
		return parsedMappingsError
	}

	params := releaseRetagPresetOptions{
		Mappings: parsedMappings,
		Remote:   configuration.RemoteName,
	}

	if executionFlagsAvailable && executionFlags.RemoteSet {
		override := strings.TrimSpace(executionFlags.Remote)
		if len(override) > 0 {
			params.Remote = override
		}
	}

	for index := range presetConfiguration.Steps {
		if workflow.CommandPathKey(presetConfiguration.Steps[index].Command) != retagPresetCommandKey {
			continue
		}
		updateReleaseRetagPresetOptions(presetConfiguration.Steps[index].Options, params)
	}

	nodes, operationsError := workflow.BuildOperations(presetConfiguration)
	if operationsError != nil {
		return fmt.Errorf(retagBuildWorkflowError, operationsError)
	}

	assumeYes := false
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

	runtimeOptions := workflow.RuntimeOptions{AssumeYes: assumeYes}

	executor := repocli.ResolveWorkflowExecutor(builder.WorkflowExecutorFactory, nodes, workflowDependencies)
	_, runErr := executor.Execute(command.Context(), repositoryRoots, runtimeOptions)
	return runErr
}

func (builder *RetagCommandBuilder) resolveConfiguration() CommandConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultCommandConfiguration()
	}
	return builder.ConfigurationProvider().Sanitize()
}

func (builder *RetagCommandBuilder) resolvePresetCatalog() workflowcmd.PresetCatalog {
	if builder.PresetCatalogFactory != nil {
		if catalog := builder.PresetCatalogFactory(); catalog != nil {
			return catalog
		}
	}
	return workflowcmd.NewEmbeddedPresetCatalog()
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
