package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/gitrepo"
	"github.com/temirov/gix/internal/migrate"
	"github.com/temirov/gix/internal/repos/dependencies"
	"github.com/temirov/gix/internal/repos/shared"
	"github.com/temirov/gix/internal/utils"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	rootutils "github.com/temirov/gix/internal/utils/roots"
	"github.com/temirov/gix/internal/workflow"
)

const (
	commandUseConstant                  = "default"
	commandUseTemplateConstant          = commandUseConstant + " <target-branch>"
	commandLegacyAliasConstant          = "branch-default"
	commandShortDescriptionConstant     = "Set the repository default branch"
	commandLongDescriptionConstant      = "default retargets workflows, updates GitHub configuration, and evaluates safety gates before promoting the requested branch, automatically detecting the current default branch."
	taskNameTemplateConstant            = "Promote default branch to %s"
	taskActionBranchDefaultTypeConstant = "branch.default"
	taskOptionTargetBranchKeyConstant   = "target"
)

type commandOptions struct {
	debugLoggingEnabled bool
	repositoryRoots     []string
	targetBranch        migrate.BranchName
}

// LoggerProvider supplies a zap logger instance.
type LoggerProvider func() *zap.Logger

// CommandBuilder assembles the default Cobra command backed by workflow tasks.
type CommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitRepositoryManager         shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
	PrompterFactory              func(*cobra.Command) shared.ConfirmationPrompter
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() migrate.CommandConfiguration
	TaskRunnerFactory            func(workflow.Dependencies) TaskRunnerExecutor
}

// Build constructs the default command.
func (builder *CommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:           commandUseTemplateConstant,
		Short:         commandShortDescriptionConstant,
		Long:          commandLongDescriptionConstant,
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.MaximumNArgs(1),
		RunE:          builder.runDefault,
	}
	command.Aliases = append(command.Aliases, commandLegacyAliasConstant)

	return command, nil
}

func (builder *CommandBuilder) runDefault(command *cobra.Command, arguments []string) error {
	options, optionsError := builder.parseOptions(command, arguments)
	if optionsError != nil {
		return optionsError
	}

	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

	assumeYes := false
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

	logger := builder.resolveLogger(options.debugLoggingEnabled)
	humanReadable := false
	if builder.HumanReadableLoggingProvider != nil {
		humanReadable = builder.HumanReadableLoggingProvider()
	}

	gitExecutor, executorError := dependencies.ResolveGitExecutor(builder.GitExecutor, logger, humanReadable)
	if executorError != nil {
		return executorError
	}

	resolvedManager, managerError := dependencies.ResolveGitRepositoryManager(builder.GitRepositoryManager, gitExecutor)
	if managerError != nil {
		return managerError
	}

	var repositoryManager *gitrepo.RepositoryManager
	if concrete, ok := resolvedManager.(*gitrepo.RepositoryManager); ok {
		repositoryManager = concrete
	} else {
		constructedManager, constructedManagerError := gitrepo.NewRepositoryManager(gitExecutor)
		if constructedManagerError != nil {
			return constructedManagerError
		}
		repositoryManager = constructedManager
	}

	githubClient, clientError := githubcli.NewClient(gitExecutor)
	if clientError != nil {
		return clientError
	}

	repositoryDiscoverer := dependencies.ResolveRepositoryDiscoverer(builder.Discoverer)
	fileSystem := dependencies.ResolveFileSystem(builder.FileSystem)
	prompter := resolvePrompter(builder.PrompterFactory, command)

	taskDependencies := workflow.Dependencies{
		Logger:               logger,
		RepositoryDiscoverer: repositoryDiscoverer,
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         githubClient,
		FileSystem:           fileSystem,
		Prompter:             prompter,
		Output:               command.OutOrStdout(),
		Errors:               command.ErrOrStderr(),
	}

	taskRunner := resolveTaskRunner(builder.TaskRunnerFactory, taskDependencies)

	actionOptions := map[string]any{
		taskOptionTargetBranchKeyConstant: string(options.targetBranch),
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        fmt.Sprintf(taskNameTemplateConstant, string(options.targetBranch)),
		EnsureClean: false,
		Actions: []workflow.TaskActionDefinition{
			{Type: taskActionBranchDefaultTypeConstant, Options: actionOptions},
		},
	}

	runtimeOptions := workflow.RuntimeOptions{
		AssumeYes: assumeYes,
	}

	return taskRunner.Run(command.Context(), options.repositoryRoots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
}

func (builder *CommandBuilder) parseOptions(command *cobra.Command, arguments []string) (commandOptions, error) {
	configuration := builder.resolveConfiguration()

	debugEnabled := configuration.EnableDebugLogging
	if command != nil {
		contextAccessor := utils.NewCommandContextAccessor()
		if logLevel, available := contextAccessor.LogLevel(command.Context()); available {
			if strings.EqualFold(logLevel, string(utils.LogLevelDebug)) {
				debugEnabled = true
			}
		}
	}

	repositoryRoots, resolveRootsError := rootutils.Resolve(command, nil, configuration.RepositoryRoots)
	if resolveRootsError != nil {
		return commandOptions{}, resolveRootsError
	}

	targetBranchName := strings.TrimSpace(configuration.TargetBranch)
	if len(arguments) > 0 {
		targetBranchName = strings.TrimSpace(arguments[0])
	}

	if len(targetBranchName) == 0 {
		targetBranchName = string(migrate.BranchMaster)
	}

	targetBranch := migrate.BranchName(targetBranchName)

	return commandOptions{
		debugLoggingEnabled: debugEnabled,
		repositoryRoots:     repositoryRoots,
		targetBranch:        targetBranch,
	}, nil
}

func (builder *CommandBuilder) resolveLogger(enableDebug bool) *zap.Logger {
	var logger *zap.Logger
	if builder.LoggerProvider != nil {
		logger = builder.LoggerProvider()
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	if enableDebug {
		logger = logger.WithOptions(zap.IncreaseLevel(zapcore.DebugLevel))
	}
	return logger
}

func (builder *CommandBuilder) resolveConfiguration() migrate.CommandConfiguration {
	if builder.ConfigurationProvider == nil {
		return migrate.DefaultCommandConfiguration()
	}

	provided := builder.ConfigurationProvider()
	return provided.Sanitize()
}

func resolvePrompter(factory func(*cobra.Command) shared.ConfirmationPrompter, command *cobra.Command) shared.ConfirmationPrompter {
	if factory == nil {
		return nil
	}
	return factory(command)
}
