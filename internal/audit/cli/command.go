package cli

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/temirov/gix/internal/audit"
	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/gitrepo"
	"github.com/temirov/gix/internal/repos/dependencies"
	"github.com/temirov/gix/internal/utils"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
)

const (
	commandUseConstant               = "audit"
	commandShortDescriptionConstant  = "Audit and reconcile local GitHub repositories"
	commandLongDescriptionConstant   = "Scans git repositories for GitHub remotes and produces audit reports or applies reconciliation actions."
	flagRootNameConstant             = "roots"
	flagRootDescriptionConstant      = "Repository roots to scan (repeatable; nested paths ignored)"
	flagIncludeAllNameConstant       = "all"
	flagIncludeAllDescription        = "Include directories without Git repositories in the audit output"
	taskNameGenerateAuditReport      = "Generate audit report"
	missingRootsErrorMessageConstant = "no repository roots provided; specify --roots or configure defaults"
)

type commandOptions struct {
	debugOutput       bool
	includeAllFolders bool
	repositoryRoots   []string
}

// LoggerProvider yields a zap logger for command execution.
type LoggerProvider func() *zap.Logger

// CommandBuilder assembles the audit CLI command backed by workflow tasks.
type CommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   audit.RepositoryDiscoverer
	GitExecutor                  audit.GitExecutor
	GitManager                   audit.GitRepositoryManager
	GitHubResolver               audit.GitHubMetadataResolver
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() audit.CommandConfiguration
	TaskRunnerFactory            func(workflow.Dependencies) TaskRunnerExecutor
}

// Build constructs the audit command.
func (builder *CommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   commandUseConstant,
		Short: commandShortDescriptionConstant,
		Long:  commandLongDescriptionConstant,
		Args:  builder.noArgumentValidator(),
		RunE:  builder.run,
	}

	command.Flags().StringSlice(flagRootNameConstant, nil, flagRootDescriptionConstant)
	command.Flags().Bool(flagIncludeAllNameConstant, false, flagIncludeAllDescription)

	return command, nil
}

func (builder *CommandBuilder) run(command *cobra.Command, arguments []string) error {
	options, optionsError := builder.parseOptions(command)
	if optionsError != nil {
		return optionsError
	}

	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)
	assumeYes := false
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

	logger := builder.resolveLogger()
	humanReadable := false
	if builder.HumanReadableLoggingProvider != nil {
		humanReadable = builder.HumanReadableLoggingProvider()
	}

	gitExecutor, executorError := dependencies.ResolveGitExecutor(builder.GitExecutor, logger, humanReadable)
	if executorError != nil {
		return executorError
	}

	gitManager, managerError := dependencies.ResolveGitRepositoryManager(builder.GitManager, gitExecutor)
	if managerError != nil {
		return managerError
	}

	repositoryManager, managerCastError := gitManager.(*gitrepo.RepositoryManager)
	if !managerCastError {
		constructedManager, constructedManagerError := gitrepo.NewRepositoryManager(gitExecutor)
		if constructedManagerError != nil {
			return constructedManagerError
		}
		repositoryManager = constructedManager
	}

	gitHubClient, githubError := dependencies.ResolveGitHubResolver(builder.GitHubResolver, gitExecutor)
	if githubError != nil {
		return githubError
	}

	client, ok := gitHubClient.(*githubcli.Client)
	if !ok {
		constructedClient, constructedClientError := githubcli.NewClient(gitExecutor)
		if constructedClientError != nil {
			return constructedClientError
		}
		client = constructedClient
	}

	repositoryDiscoverer := dependencies.ResolveRepositoryDiscoverer(builder.Discoverer)

	taskDependencies := workflow.Dependencies{
		Logger:               logger,
		RepositoryDiscoverer: repositoryDiscoverer,
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         client,
		FileSystem:           dependencies.ResolveFileSystem(nil),
		Prompter:             nil,
		Output:               command.OutOrStdout(),
		Errors:               command.ErrOrStderr(),
	}

	taskRunner := resolveTaskRunner(builder.TaskRunnerFactory, taskDependencies)

	actionOptions := map[string]any{
		"include_all": options.includeAllFolders,
		"debug":       options.debugOutput,
		"depth":       string(audit.InspectionDepthFull),
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        taskNameGenerateAuditReport,
		EnsureClean: false,
		Actions: []workflow.TaskActionDefinition{
			{Type: "audit.report", Options: actionOptions},
		},
	}

	runtimeOptions := workflow.RuntimeOptions{AssumeYes: assumeYes}

	return taskRunner.Run(command.Context(), options.repositoryRoots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
}

func (builder *CommandBuilder) parseOptions(command *cobra.Command) (commandOptions, error) {
	configuration := builder.resolveConfiguration()

	debugMode := configuration.Debug
	if command != nil {
		contextAccessor := utils.NewCommandContextAccessor()
		if logLevel, available := contextAccessor.LogLevel(command.Context()); available {
			if strings.EqualFold(logLevel, string(utils.LogLevelDebug)) {
				debugMode = true
			}
		}
	}

	repositoryRoots := append([]string{}, configuration.Roots...)
	if command != nil && command.Flags().Changed(flagRootNameConstant) {
		flagRoots, _ := command.Flags().GetStringSlice(flagRootNameConstant)
		repositoryRoots = audit.CommandConfiguration{Roots: flagRoots}.Sanitize().Roots
	}

	includeAll := configuration.IncludeAll
	if command != nil {
		includeAllValue, includeAllChanged, includeAllError := flagutils.BoolFlag(command, flagIncludeAllNameConstant)
		if includeAllError != nil && !errors.Is(includeAllError, flagutils.ErrFlagNotDefined) {
			return commandOptions{}, includeAllError
		}
		if includeAllChanged {
			includeAll = includeAllValue
		}
	}

	if len(repositoryRoots) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return commandOptions{}, errors.New(missingRootsErrorMessageConstant)
	}

	return commandOptions{
		repositoryRoots:   repositoryRoots,
		includeAllFolders: includeAll,
		debugOutput:       debugMode,
	}, nil
}

func (builder *CommandBuilder) resolveLogger() *zap.Logger {
	if builder.LoggerProvider == nil {
		return zap.NewNop()
	}
	logger := builder.LoggerProvider()
	if logger == nil {
		return zap.NewNop()
	}
	return logger
}

func (builder *CommandBuilder) resolveConfiguration() audit.CommandConfiguration {
	if builder.ConfigurationProvider == nil {
		return audit.DefaultCommandConfiguration()
	}
	return builder.ConfigurationProvider().Sanitize()
}

func (builder *CommandBuilder) noArgumentValidator() cobra.PositionalArgs {
	return func(command *cobra.Command, arguments []string) error {
		if len(arguments) == 0 {
			return nil
		}
		_ = command.Help()
		return cobra.NoArgs(command, arguments)
	}
}
