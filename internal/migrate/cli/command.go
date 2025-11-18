package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/temirov/gix/internal/migrate"
	"github.com/temirov/gix/internal/repos/shared"
	"github.com/temirov/gix/internal/utils"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	rootutils "github.com/temirov/gix/internal/utils/roots"
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
)

const (
	commandUseConstant                  = "default"
	commandUseTemplateConstant          = commandUseConstant + " <target-branch>"
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
	GitHubResolver               shared.GitHubMetadataResolver
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

	dependencyResult, dependencyError := taskrunner.BuildDependencies(
		taskrunner.DependenciesConfig{
			LoggerProvider: func() *zap.Logger {
				return logger
			},
			HumanReadableLoggingProvider: builder.HumanReadableLoggingProvider,
			RepositoryDiscoverer:         builder.Discoverer,
			GitExecutor:                  builder.GitExecutor,
			GitRepositoryManager:         builder.GitRepositoryManager,
			GitHubResolver:               builder.GitHubResolver,
			FileSystem:                   builder.FileSystem,
			PrompterFactory:              builder.PrompterFactory,
		},
		taskrunner.DependenciesOptions{
			Command: command,
			Output:  command.OutOrStdout(),
			Errors:  command.ErrOrStderr(),
		},
	)
	if dependencyError != nil {
		return dependencyError
	}

	taskRunner := resolveTaskRunner(builder.TaskRunnerFactory, dependencyResult.Workflow)

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

	_, runErr := taskRunner.Run(command.Context(), options.repositoryRoots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
	return runErr
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
