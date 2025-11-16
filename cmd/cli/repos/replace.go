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
	replaceUseConstant          = "repo-files-replace"
	replaceShortDescription     = "Perform string replacements across repository files"
	replaceLongDescription      = "repo-files-replace applies string substitutions to files matched by glob patterns, optionally enforcing safeguards and running a follow-up command."
	replacePatternFlagName      = "pattern"
	replaceFindFlagName         = "find"
	replaceReplaceFlagName      = "replace"
	replaceCommandFlagName      = "command"
	replaceRequireCleanFlagName = "require-clean"
	replaceBranchFlagName       = "branch"
	replaceRequirePathFlagName  = "require-path"
	replaceMissingFindError     = "replacement requires --find"
	replacePresetName           = "files-replace"
	replacePresetCommandKey     = "tasks apply"
	replacePresetMissingMessage = "files-replace preset not found"
	replacePresetLoadError      = "unable to load files-replace preset: %w"
	replaceBuildWorkflowError   = "unable to build files-replace workflow: %w"
)

// ReplaceCommandBuilder assembles the repo-files-replace command.
type ReplaceCommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() ReplaceConfiguration
	PresetCatalogFactory         func() workflowcmd.PresetCatalog
	WorkflowExecutorFactory      WorkflowExecutorFactory
}

// Build constructs the repo-files-replace command.
func (builder *ReplaceCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   replaceUseConstant,
		Short: replaceShortDescription,
		Long:  replaceLongDescription,
		RunE:  builder.run,
	}

	command.Flags().StringSlice(replacePatternFlagName, nil, "Glob pattern (repeatable) selecting files to update")
	command.Flags().String(replaceFindFlagName, "", "String to search for within matched files (required)")
	command.Flags().String(replaceReplaceFlagName, "", "String to substitute in place of the search string")
	command.Flags().String(replaceCommandFlagName, "", "Optional command to run after replacements (quoted, e.g. \"go fmt ./...\")")
	flagutils.AddToggleFlag(command.Flags(), nil, replaceRequireCleanFlagName, "", false, "Require a clean working tree before applying replacements")
	command.Flags().String(replaceBranchFlagName, "", "Require the repository to be on the specified branch before applying replacements")
	command.Flags().StringSlice(replaceRequirePathFlagName, nil, "Require the specified relative path to exist before applying replacements (repeatable)")

	return command, nil
}

func (builder *ReplaceCommandBuilder) run(command *cobra.Command, _ []string) error {
	configuration := builder.resolveConfiguration()
	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

	assumeYes := configuration.AssumeYes
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

	patterns := append([]string{}, configuration.Patterns...)
	if command != nil && command.Flags().Changed(replacePatternFlagName) {
		flagPatterns, flagError := command.Flags().GetStringSlice(replacePatternFlagName)
		if flagError != nil {
			return flagError
		}
		patterns = sanitizeReplacementPatterns(flagPatterns)
	}

	findValue := strings.TrimSpace(configuration.Find)
	if command != nil && command.Flags().Changed(replaceFindFlagName) {
		flagValue, flagError := command.Flags().GetString(replaceFindFlagName)
		if flagError != nil {
			return flagError
		}
		findValue = strings.TrimSpace(flagValue)
	}
	if len(findValue) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return errors.New(replaceMissingFindError)
	}

	replaceValue := configuration.Replace
	if command != nil && command.Flags().Changed(replaceReplaceFlagName) {
		flagValue, flagError := command.Flags().GetString(replaceReplaceFlagName)
		if flagError != nil {
			return flagError
		}
		replaceValue = flagValue
	}

	commandValue := strings.TrimSpace(configuration.Command)
	if command != nil && command.Flags().Changed(replaceCommandFlagName) {
		flagValue, flagError := command.Flags().GetString(replaceCommandFlagName)
		if flagError != nil {
			return flagError
		}
		commandValue = strings.TrimSpace(flagValue)
	}
	commandArguments := sanitizeCommandArguments(strings.Fields(commandValue))

	requireClean := configuration.RequireClean
	if command != nil {
		flagValue, flagChanged, flagError := flagutils.BoolFlag(command, replaceRequireCleanFlagName)
		if flagError != nil && !errors.Is(flagError, flagutils.ErrFlagNotDefined) {
			return flagError
		}
		if flagChanged {
			requireClean = flagValue
		}
	}

	branchValue := strings.TrimSpace(configuration.Branch)
	if command != nil && command.Flags().Changed(replaceBranchFlagName) {
		flagValue, flagError := command.Flags().GetString(replaceBranchFlagName)
		if flagError != nil {
			return flagError
		}
		branchValue = strings.TrimSpace(flagValue)
	}

	requiredPaths := append([]string{}, configuration.RequirePaths...)
	if command != nil && command.Flags().Changed(replaceRequirePathFlagName) {
		flagPaths, flagError := command.Flags().GetStringSlice(replaceRequirePathFlagName)
		if flagError != nil {
			return flagError
		}
		requiredPaths = sanitizeReplacementPaths(flagPaths)
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

	presetCatalog := builder.resolvePresetCatalog()
	presetConfiguration, presetFound, presetError := presetCatalog.Load(replacePresetName)
	if presetError != nil {
		return fmt.Errorf(replacePresetLoadError, presetError)
	}
	if !presetFound {
		return errors.New(replacePresetMissingMessage)
	}

	params := filesReplacePresetOptions{
		Patterns:     patterns,
		Find:         findValue,
		Replace:      replaceValue,
		Command:      commandArguments,
		RequireClean: requireClean,
		Branch:       branchValue,
		RequirePaths: requiredPaths,
	}

	for index := range presetConfiguration.Steps {
		if workflow.CommandPathKey(presetConfiguration.Steps[index].Command) != replacePresetCommandKey {
			continue
		}
		updateFilesReplacePresetOptions(presetConfiguration.Steps[index].Options, params)
	}

	nodes, operationsError := workflow.BuildOperations(presetConfiguration)
	if operationsError != nil {
		return fmt.Errorf(replaceBuildWorkflowError, operationsError)
	}

	runtimeOptions := workflow.RuntimeOptions{AssumeYes: assumeYes}

	executor := ResolveWorkflowExecutor(builder.WorkflowExecutorFactory, nodes, workflowDependencies)
	_, runErr := executor.Execute(command.Context(), roots, runtimeOptions)
	return runErr
}

func (builder *ReplaceCommandBuilder) resolveConfiguration() ReplaceConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultToolsConfiguration().Replace
	}
	return builder.ConfigurationProvider().Sanitize()
}

func (builder *ReplaceCommandBuilder) resolvePresetCatalog() workflowcmd.PresetCatalog {
	if builder.PresetCatalogFactory != nil {
		if catalog := builder.PresetCatalogFactory(); catalog != nil {
			return catalog
		}
	}
	return workflowcmd.NewEmbeddedPresetCatalog()
}

func sanitizeCommandArguments(arguments []string) []string {
	sanitized := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		trimmed := strings.TrimSpace(argument)
		if len(trimmed) == 0 {
			continue
		}
		sanitized = append(sanitized, trimmed)
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}

type filesReplacePresetOptions struct {
	Patterns     []string
	Find         string
	Replace      string
	Command      []string
	RequireClean bool
	Branch       string
	RequirePaths []string
}

func updateFilesReplacePresetOptions(options map[string]any, params filesReplacePresetOptions) {
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
	actionOptions["find"] = params.Find
	actionOptions["replace"] = params.Replace

	delete(actionOptions, "pattern")
	delete(actionOptions, "patterns")
	if len(params.Patterns) == 1 {
		actionOptions["pattern"] = params.Patterns[0]
	} else if len(params.Patterns) > 1 {
		actionOptions["patterns"] = append([]string{}, params.Patterns...)
	}

	if len(params.Command) > 0 {
		actionOptions["command"] = append([]string{}, params.Command...)
	} else {
		delete(actionOptions, "command")
	}

	safeguards := map[string]any{}
	if params.RequireClean {
		safeguards["require_clean"] = true
	}
	if len(params.Branch) > 0 {
		safeguards["branch"] = params.Branch
	}
	if len(params.RequirePaths) > 0 {
		safeguards["paths"] = append([]string{}, params.RequirePaths...)
	}
	if len(safeguards) > 0 {
		actionOptions["safeguards"] = safeguards
	} else {
		delete(actionOptions, "safeguards")
	}

	actionEntry["options"] = actionOptions
	actionsValue[0] = actionEntry
	taskEntry["actions"] = actionsValue
	tasksValue[0] = taskEntry
	options["tasks"] = tasksValue
}
