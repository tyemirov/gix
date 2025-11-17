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
	LoggerProvider               workflowcmd.LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() ReplaceConfiguration
	PresetCatalogFactory         func() workflowcmd.PresetCatalog
	WorkflowExecutorFactory      workflowcmd.OperationExecutorFactory
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

	request := workflowcmd.PresetCommandRequest{
		Command:                 command,
		Arguments:               nil,
		RootArguments:           []string{},
		ConfiguredAssumeYes:     configuration.AssumeYes,
		ConfiguredRoots:         configuration.RepositoryRoots,
		PresetName:              replacePresetName,
		PresetMissingMessage:    replacePresetMissingMessage,
		PresetLoadErrorTemplate: replacePresetLoadError,
		BuildErrorTemplate:      replaceBuildWorkflowError,
		Configure: func(ctx workflowcmd.PresetCommandContext) (workflowcmd.PresetCommandResult, error) {
			params := filesReplacePresetOptions{
				Patterns:     patterns,
				Find:         findValue,
				Replace:      replaceValue,
				Command:      commandArguments,
				RequireClean: requireClean,
				Branch:       branchValue,
				RequirePaths: requiredPaths,
			}

			for index := range ctx.Configuration.Steps {
				if workflow.CommandPathKey(ctx.Configuration.Steps[index].Command) != replacePresetCommandKey {
					continue
				}
				updateFilesReplacePresetOptions(ctx.Configuration.Steps[index].Options, params)
			}

			return workflowcmd.PresetCommandResult{
				Configuration:  ctx.Configuration,
				RuntimeOptions: ctx.RuntimeOptions(),
			}, nil
		},
	}

	return builder.presetCommand().Execute(request)
}

func (builder *ReplaceCommandBuilder) resolveConfiguration() ReplaceConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultToolsConfiguration().Replace
	}
	return builder.ConfigurationProvider().Sanitize()
}

func (builder *ReplaceCommandBuilder) presetCommand() workflowcmd.PresetCommand {
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

func (builder *ReplaceCommandBuilder) presetCommand() workflowcmd.PresetCommand {
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

	safeguardOptions := buildFilesReplaceSafeguards(params)
	if len(safeguardOptions) > 0 {
		actionOptions["safeguards"] = safeguardOptions
	} else {
		delete(actionOptions, "safeguards")
	}

	actionEntry["options"] = actionOptions
	actionsValue[0] = actionEntry
	taskEntry["actions"] = actionsValue
	tasksValue[0] = taskEntry
	options["tasks"] = tasksValue
}

func buildFilesReplaceSafeguards(params filesReplacePresetOptions) map[string]any {
	hardSafeguards := map[string]any{}
	if params.RequireClean {
		hardSafeguards["require_clean"] = true
	}

	softSafeguards := map[string]any{}
	if len(strings.TrimSpace(params.Branch)) > 0 {
		softSafeguards["branch"] = params.Branch
	}
	if len(params.RequirePaths) > 0 {
		softSafeguards["paths"] = append([]string{}, params.RequirePaths...)
	}

	if len(hardSafeguards) == 0 && len(softSafeguards) == 0 {
		return nil
	}

	safeguardOptions := make(map[string]any)
	if len(hardSafeguards) > 0 {
		safeguardOptions["hard_stop"] = hardSafeguards
	}
	if len(softSafeguards) > 0 {
		safeguardOptions["soft_skip"] = softSafeguards
	}
	return safeguardOptions
}
