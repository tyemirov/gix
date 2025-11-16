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

	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

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

	remoteName := configuration.RemoteName
	if executionFlagsAvailable && executionFlags.RemoteSet {
		override := strings.TrimSpace(executionFlags.Remote)
		if len(override) > 0 {
			remoteName = override
		}
	}

	repositoryRoots, rootsError := rootutils.Resolve(command, additionalArgs, configuration.RepositoryRoots)
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
	presetConfiguration, presetFound, presetError := presetCatalog.Load(releasePresetName)
	if presetError != nil {
		return fmt.Errorf(releasePresetLoadError, presetError)
	}
	if !presetFound {
		return errors.New(releasePresetMissingMsg)
	}

	for index := range presetConfiguration.Steps {
		if workflow.CommandPathKey(presetConfiguration.Steps[index].Command) != releasePresetCommandKey {
			continue
		}
		updateReleasePresetOptions(
			presetConfiguration.Steps[index].Options,
			tagName,
			messageValue,
			remoteName,
		)
	}

	nodes, operationsError := workflow.BuildOperations(presetConfiguration)
	if operationsError != nil {
		return fmt.Errorf(releaseBuildWorkflowErr, operationsError)
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

func (builder *CommandBuilder) resolveConfiguration() CommandConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultCommandConfiguration()
	}
	return builder.ConfigurationProvider().Sanitize()
}

func (builder *CommandBuilder) resolvePresetCatalog() workflowcmd.PresetCatalog {
	if builder.PresetCatalogFactory != nil {
		if catalog := builder.PresetCatalogFactory(); catalog != nil {
			return catalog
		}
	}
	return workflowcmd.NewEmbeddedPresetCatalog()
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
