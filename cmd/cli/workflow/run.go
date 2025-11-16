package workflow

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/temirov/gix/internal/repos/shared"
	"github.com/temirov/gix/internal/utils"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	rootutils "github.com/temirov/gix/internal/utils/roots"
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
)

const (
	commandUseConstant                       = "workflow <configuration|preset>"
	commandShortDescriptionConstant          = "Run a workflow configuration file or embedded preset"
	commandLongDescriptionConstant           = "workflow executes operations defined in a YAML/JSON configuration or runs embedded presets (see --list-presets) across discovered repositories."
	commandExampleConstant                   = "gix workflow ./workflow.yaml --roots ~/Development\n  gix workflow license --roots ~/Development --yes"
	requireCleanFlagNameConstant             = "require-clean"
	requireCleanFlagDescriptionConstant      = "Require clean worktrees for rename operations"
	variableFlagNameConstant                 = "var"
	variableFlagDescriptionConstant          = "Set workflow variable (key=value). Repeatable."
	variableFileFlagNameConstant             = "var-file"
	variableFileFlagDescriptionConstant      = "Load workflow variables from a YAML/JSON file. Repeatable."
	workflowWorkersFlagNameConstant          = "workflow-workers"
	workflowWorkersFlagDescriptionConstant   = "Maximum number of repositories to process concurrently (default 1)"
	listPresetsFlagNameConstant              = "list-presets"
	listPresetsFlagDescriptionConstant       = "List embedded workflow presets and exit"
	configurationPathRequiredMessageConstant = "workflow configuration path or preset name required; provide a positional argument or --config flag"
	loadConfigurationErrorTemplateConstant   = "unable to load workflow configuration: %w"
	loadPresetErrorTemplateConstant          = "unable to load embedded workflow %q: %w"
	buildOperationsErrorTemplateConstant     = "unable to build workflow operations: %w"
)

// CommandBuilder assembles the workflow command.
type CommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	FileSystem                   shared.FileSystem
	PrompterFactory              PrompterFactory
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() CommandConfiguration
	OperationExecutorFactory     OperationExecutorFactory
	PresetCatalogFactory         func() PresetCatalog
}

// Build constructs the workflow command.
func (builder *CommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:     commandUseConstant,
		Short:   commandShortDescriptionConstant,
		Long:    commandLongDescriptionConstant,
		Example: commandExampleConstant,
		RunE:    builder.run,
	}

	flagutils.AddToggleFlag(command.Flags(), nil, requireCleanFlagNameConstant, "", false, requireCleanFlagDescriptionConstant)
	flagutils.AddToggleFlag(command.Flags(), nil, listPresetsFlagNameConstant, "", false, listPresetsFlagDescriptionConstant)
	command.Flags().StringArray(variableFlagNameConstant, nil, variableFlagDescriptionConstant)
	command.Flags().StringArray(variableFileFlagNameConstant, nil, variableFileFlagDescriptionConstant)
	command.Flags().Int(workflowWorkersFlagNameConstant, 1, workflowWorkersFlagDescriptionConstant)

	return command, nil
}

func (builder *CommandBuilder) run(command *cobra.Command, arguments []string) error {
	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)
	contextAccessor := utils.NewCommandContextAccessor()
	presetCatalog := builder.resolvePresetCatalog()
	listPresets := false
	if command != nil {
		listFlagValue, _, listFlagError := flagutils.BoolFlag(command, listPresetsFlagNameConstant)
		if listFlagError != nil && !errors.Is(listFlagError, flagutils.ErrFlagNotDefined) {
			return listFlagError
		}
		listPresets = listFlagValue
	}

	configurationPathCandidate := ""
	remainingArguments := []string{}
	if len(arguments) > 0 {
		configurationPathCandidate = strings.TrimSpace(arguments[0])
		if len(arguments) > 1 {
			remainingArguments = append(remainingArguments, arguments[1:]...)
		}
	} else {
		configurationPathFromContext, configurationPathAvailable := contextAccessor.ConfigurationFilePath(command.Context())
		if configurationPathAvailable {
			configurationPathCandidate = strings.TrimSpace(configurationPathFromContext)
		}
	}

	if listPresets && len(configurationPathCandidate) == 0 {
		builder.printPresetList(command, presetCatalog)
		return nil
	}

	if len(configurationPathCandidate) == 0 {
		if helpError := displayCommandHelp(command); helpError != nil {
			return helpError
		}
		return errors.New(configurationPathRequiredMessageConstant)
	}

	if listPresets {
		builder.printPresetList(command, presetCatalog)
		return nil
	}

	configurationPath := configurationPathCandidate
	var workflowConfiguration workflow.Configuration
	loadedFromPreset := false
	if presetCatalog != nil {
		presetConfiguration, presetFound, presetError := presetCatalog.Load(configurationPath)
		if presetError != nil {
			return fmt.Errorf(loadPresetErrorTemplateConstant, configurationPath, presetError)
		}
		if presetFound {
			workflowConfiguration = presetConfiguration
			loadedFromPreset = true
		}
	}

	if !loadedFromPreset {
		loadedConfiguration, configurationError := workflow.LoadConfiguration(configurationPath)
		if configurationError != nil {
			return fmt.Errorf(loadConfigurationErrorTemplateConstant, configurationError)
		}
		workflowConfiguration = loadedConfiguration
	}

	nodes, operationsError := workflow.BuildOperations(workflowConfiguration)
	if operationsError != nil {
		return fmt.Errorf(buildOperationsErrorTemplateConstant, operationsError)
	}

	commandConfiguration := builder.resolveConfiguration()
	variableAssignments, variableError := builder.resolveVariables(command, commandConfiguration)
	if variableError != nil {
		return variableError
	}

	requireCleanDefault := commandConfiguration.RequireClean
	if command != nil {
		requireCleanFlagValue, requireCleanFlagChanged, requireCleanFlagError := flagutils.BoolFlag(command, requireCleanFlagNameConstant)
		if requireCleanFlagError != nil && !errors.Is(requireCleanFlagError, flagutils.ErrFlagNotDefined) {
			return requireCleanFlagError
		}
		if requireCleanFlagChanged {
			requireCleanDefault = requireCleanFlagValue
		}
	}

	workflow.ApplyDefaults(nodes, workflow.OperationDefaults{RequireClean: requireCleanDefault})
	runtimeRequirements := deriveRuntimeRequirements(nodes)

	dependencyOptions := taskrunner.DependenciesOptions{Command: command}
	if command != nil {
		dependencyOptions.Output = utils.NewFlushingWriter(command.OutOrStdout())
		dependencyOptions.Errors = utils.NewFlushingWriter(command.ErrOrStderr())
	}

	dependencyResult, dependencyError := taskrunner.BuildDependencies(
		taskrunner.DependenciesConfig{
			LoggerProvider:               builder.LoggerProvider,
			HumanReadableLoggingProvider: builder.HumanReadableLoggingProvider,
			RepositoryDiscoverer:         builder.Discoverer,
			GitExecutor:                  builder.GitExecutor,
			FileSystem:                   builder.FileSystem,
			PrompterFactory:              builder.PrompterFactory,
		},
		dependencyOptions,
	)
	if dependencyError != nil {
		return dependencyError
	}

	workflowDependencies := dependencyResult.Workflow
	if workflowDependencies.HumanReadableLogging && !workflowDependencies.DisableWorkflowLogging {
		workflowDependencies.ReporterOptions = append(
			workflowDependencies.ReporterOptions,
			shared.WithEventFormatter(newWorkflowHumanFormatter()),
		)
	}

	roots, rootsError := rootutils.Resolve(command, remainingArguments, commandConfiguration.Roots)
	if rootsError != nil {
		return rootsError
	}

	assumeYes := commandConfiguration.AssumeYes
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

	workflowWorkers := commandConfiguration.WorkflowWorkers
	if command != nil {
		workerValue, workerErr := command.Flags().GetInt(workflowWorkersFlagNameConstant)
		if workerErr != nil {
			return workerErr
		}
		if command.Flags().Changed(workflowWorkersFlagNameConstant) {
			workflowWorkers = workerValue
		}
	}

	runtimeOptions := workflow.RuntimeOptions{
		AssumeYes:                            assumeYes,
		IncludeNestedRepositories:            runtimeRequirements.includeNestedRepositories,
		ProcessRepositoriesByDescendingDepth: runtimeRequirements.processRepositoriesByDescendingDepth,
		CaptureInitialWorktreeStatus:         runtimeRequirements.captureInitialWorktreeStatus,
		WorkflowParallelism:                  workflowWorkers,
		Variables:                            variableAssignments,
	}

	executor := resolveOperationExecutor(builder.OperationExecutorFactory, nodes, workflowDependencies)
	_, runErr := executor.Execute(command.Context(), roots, runtimeOptions)
	return runErr
}

func (builder *CommandBuilder) resolveConfiguration() CommandConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultCommandConfiguration()
	}

	provided := builder.ConfigurationProvider()
	return provided.Sanitize()
}

func (builder *CommandBuilder) resolveVariables(command *cobra.Command, configuration CommandConfiguration) (map[string]string, error) {
	variableAssignments := make(map[string]string)
	if len(configuration.Variables) > 0 {
		for key, value := range configuration.Variables {
			normalizedKey, normalizeError := normalizeVariableName(key)
			if normalizeError != nil {
				return nil, fmt.Errorf("invalid workflow variable %q in configuration: %w", key, normalizeError)
			}
			variableAssignments[normalizedKey] = value
		}
	}

	if command != nil {
		varFiles, varFileError := command.Flags().GetStringArray(variableFileFlagNameConstant)
		if varFileError != nil {
			return nil, varFileError
		}
		fileVariables, loadError := loadVariablesFromFiles(varFiles)
		if loadError != nil {
			return nil, loadError
		}
		for key, value := range fileVariables {
			variableAssignments[key] = value
		}

		varAssignments, varError := command.Flags().GetStringArray(variableFlagNameConstant)
		if varError != nil {
			return nil, varError
		}
		parsedAssignments, parseError := parseVariableAssignments(varAssignments)
		if parseError != nil {
			return nil, parseError
		}
		for key, value := range parsedAssignments {
			variableAssignments[key] = value
		}
	}

	if len(variableAssignments) == 0 {
		return nil, nil
	}
	return variableAssignments, nil
}

func (builder *CommandBuilder) resolvePresetCatalog() PresetCatalog {
	if builder.PresetCatalogFactory != nil {
		if catalog := builder.PresetCatalogFactory(); catalog != nil {
			return catalog
		}
	}
	return NewEmbeddedPresetCatalog()
}

func (builder *CommandBuilder) printPresetList(command *cobra.Command, catalog PresetCatalog) {
	output := utils.NewFlushingWriter(command.OutOrStdout())
	if catalog == nil {
		fmt.Fprintln(output, "No embedded workflows available.")
		return
	}

	presets := catalog.List()
	if len(presets) == 0 {
		fmt.Fprintln(output, "No embedded workflows available.")
		return
	}

	fmt.Fprintln(output, "Embedded workflows:")
	for _, preset := range presets {
		description := strings.TrimSpace(preset.Description)
		if len(description) == 0 {
			fmt.Fprintf(output, "  - %s\n", preset.Name)
			continue
		}
		fmt.Fprintf(output, "  - %s: %s\n", preset.Name, description)
	}
}
