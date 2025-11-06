package workflow

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/gitrepo"
	"github.com/temirov/gix/internal/repos/dependencies"
	"github.com/temirov/gix/internal/repos/shared"
	"github.com/temirov/gix/internal/utils"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	rootutils "github.com/temirov/gix/internal/utils/roots"
	"github.com/temirov/gix/internal/workflow"
)

const (
	commandUseConstant                        = "workflow <configuration>"
	commandShortDescriptionConstant           = "Run a workflow configuration file"
	commandLongDescriptionConstant            = "workflow executes operations defined in a YAML or JSON configuration file across discovered repositories. Provide the configuration path as the first argument or supply --config."
	commandExampleConstant                    = "gix workflow ./workflow.yaml --roots ~/Development --dry-run"
	requireCleanFlagNameConstant              = "require-clean"
	requireCleanFlagDescriptionConstant       = "Require clean worktrees for rename operations"
	configurationPathRequiredMessageConstant  = "workflow configuration path required; provide a positional argument or --config flag"
	loadConfigurationErrorTemplateConstant    = "unable to load workflow configuration: %w"
	buildOperationsErrorTemplateConstant      = "unable to build workflow operations: %w"
	buildTasksErrorTemplateConstant           = "unable to build workflow tasks: %w"
	gitRepositoryManagerErrorTemplateConstant = "unable to construct repository manager: %w"
	gitHubClientErrorTemplateConstant         = "unable to construct GitHub client: %w"
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
	TaskRunnerFactory            func(workflow.Dependencies) TaskRunnerExecutor
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

	return command, nil
}

func (builder *CommandBuilder) run(command *cobra.Command, arguments []string) error {
	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)
	contextAccessor := utils.NewCommandContextAccessor()

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

	if len(configurationPathCandidate) == 0 {
		if helpError := displayCommandHelp(command); helpError != nil {
			return helpError
		}
		return errors.New(configurationPathRequiredMessageConstant)
	}

	configurationPath := configurationPathCandidate
	workflowConfiguration, configurationError := workflow.LoadConfiguration(configurationPath)
	if configurationError != nil {
		return fmt.Errorf(loadConfigurationErrorTemplateConstant, configurationError)
	}

	nodes, operationsError := workflow.BuildOperations(workflowConfiguration)
	if operationsError != nil {
		return fmt.Errorf(buildOperationsErrorTemplateConstant, operationsError)
	}

	commandConfiguration := builder.resolveConfiguration()

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

	taskDefinitions, taskRuntimeOptions, taskBuildError := buildWorkflowTasks(nodes)
	if taskBuildError != nil {
		return fmt.Errorf(buildTasksErrorTemplateConstant, taskBuildError)
	}
	if len(taskDefinitions) == 0 {
		return nil
	}

	logger := resolveLogger(builder.LoggerProvider)
	humanReadableLogging := false
	if builder.HumanReadableLoggingProvider != nil {
		humanReadableLogging = builder.HumanReadableLoggingProvider()
	}
	gitExecutor, executorError := dependencies.ResolveGitExecutor(builder.GitExecutor, logger, humanReadableLogging)
	if executorError != nil {
		return executorError
	}

	repositoryManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	if managerError != nil {
		return fmt.Errorf(gitRepositoryManagerErrorTemplateConstant, managerError)
	}

	gitHubClient, clientError := githubcli.NewClient(gitExecutor)
	if clientError != nil {
		return fmt.Errorf(gitHubClientErrorTemplateConstant, clientError)
	}

	repositoryDiscoverer := dependencies.ResolveRepositoryDiscoverer(builder.Discoverer)
	fileSystem := dependencies.ResolveFileSystem(builder.FileSystem)
	prompter := resolvePrompter(builder.PrompterFactory, command)

	workflowDependencies := workflow.Dependencies{
		Logger:               logger,
		RepositoryDiscoverer: repositoryDiscoverer,
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         gitHubClient,
		FileSystem:           fileSystem,
		Prompter:             prompter,
		Output:               utils.NewFlushingWriter(command.OutOrStdout()),
		Errors:               utils.NewFlushingWriter(command.ErrOrStderr()),
	}

	taskRunner := resolveTaskRunner(builder.TaskRunnerFactory, workflowDependencies)

	roots, rootsError := rootutils.Resolve(command, remainingArguments, commandConfiguration.Roots)
	if rootsError != nil {
		return rootsError
	}

	assumeYes := commandConfiguration.AssumeYes
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

	runtimeOptions := workflow.RuntimeOptions{
		AssumeYes:                            assumeYes,
		IncludeNestedRepositories:            taskRuntimeOptions.IncludeNestedRepositories,
		ProcessRepositoriesByDescendingDepth: taskRuntimeOptions.ProcessRepositoriesByDescendingDepth,
		CaptureInitialWorktreeStatus:         taskRuntimeOptions.CaptureInitialWorktreeStatus,
	}

	return taskRunner.Run(command.Context(), roots, taskDefinitions, runtimeOptions)
}

func (builder *CommandBuilder) resolveConfiguration() CommandConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultCommandConfiguration()
	}

	provided := builder.ConfigurationProvider()
	return provided.Sanitize()
}
