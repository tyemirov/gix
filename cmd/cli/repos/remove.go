package repos

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/repos/shared"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	rootutils "github.com/temirov/gix/internal/utils/roots"
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
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
	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

	assumeYes := configuration.AssumeYes
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

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

	roots, rootsError := rootutils.Resolve(command, nil, configuration.RepositoryRoots)
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
		taskrunner.DependenciesOptions{Command: command},
	)
	if dependencyError != nil {
		return dependencyError
	}

	workflowDependencies := dependencyResult.Workflow
	if command != nil {
		workflowDependencies.Output = command.OutOrStdout()
		workflowDependencies.Errors = command.ErrOrStderr()
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

	presetCatalog := builder.resolvePresetCatalog()
	presetConfiguration, presetFound, presetError := presetCatalog.Load(historyRemovePresetName)
	if presetError != nil {
		return fmt.Errorf(historyPresetLoadErrorTemplate, presetError)
	}
	if !presetFound {
		return errors.New(historyPresetMissingMessage)
	}

	for index := range presetConfiguration.Steps {
		if workflow.CommandPathKey(presetConfiguration.Steps[index].Command) != historyRemoveCommandKey {
			continue
		}
		updateHistoryPresetOptions(
			presetConfiguration.Steps[index].Options,
			normalizedPaths,
			strings.TrimSpace(remoteName),
			pushEnabled,
			restoreEnabled,
			pushMissing,
		)
	}

	nodes, operationsError := workflow.BuildOperations(presetConfiguration)
	if operationsError != nil {
		return fmt.Errorf(historyBuildWorkflowError, operationsError)
	}

	runtimeOptions := workflow.RuntimeOptions{AssumeYes: assumeYes}

	executor := workflowcmd.ResolveOperationExecutor(builder.WorkflowExecutorFactory, nodes, workflowDependencies)
	_, runErr := executor.Execute(command.Context(), roots, runtimeOptions)
	return runErr
}

func (builder *RemoveCommandBuilder) resolveConfiguration() RemoveConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultToolsConfiguration().Remove
	}

	return builder.ConfigurationProvider().sanitize()
}

func (builder *RemoveCommandBuilder) resolvePresetCatalog() workflowcmd.PresetCatalog {
	if builder.PresetCatalogFactory != nil {
		if catalog := builder.PresetCatalogFactory(); catalog != nil {
			return catalog
		}
	}
	return workflowcmd.NewEmbeddedPresetCatalog()
}

func updateHistoryPresetOptions(options map[string]any, paths []string, remote string, push bool, restore bool, pushMissing bool) {
	if options == nil {
		return
	}
	tasksValue, ok := options["tasks"].([]any)
	if !ok || len(tasksValue) == 0 {
		return
	}
	taskEntry, ok := tasksValue[0].(map[string]any)
	if !ok {
		return
	}
	actionsValue, ok := taskEntry["actions"].([]any)
	if !ok || len(actionsValue) == 0 {
		return
	}
	actionEntry, ok := actionsValue[0].(map[string]any)
	if !ok {
		return
	}
	actionOptions, _ := actionEntry["options"].(map[string]any)
	if actionOptions == nil {
		actionOptions = make(map[string]any)
	}
	actionOptions["paths"] = append([]string{}, paths...)
	if len(remote) > 0 {
		actionOptions["remote"] = remote
	}
	actionOptions["push"] = push
	actionOptions["restore"] = restore
	actionOptions["push_missing"] = pushMissing
	actionEntry["options"] = actionOptions
	actionsValue[0] = actionEntry
	taskEntry["actions"] = actionsValue
	tasksValue[0] = taskEntry
	options["tasks"] = tasksValue
}
