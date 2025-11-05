package branches

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/dependencies"
	"github.com/tyemirov/gix/internal/repos/prompt"
	"github.com/tyemirov/gix/internal/repos/shared"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	rootutils "github.com/tyemirov/gix/internal/utils/roots"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	commandUseConstant                          = "repo-prs-purge"
	commandShortDescriptionConstant             = "Remove remote and local branches for closed pull requests"
	commandLongDescriptionConstant              = "repo-prs-purge removes remote and local Git branches whose pull requests are already closed."
	flagRemoteDescriptionConstant               = "Name of the remote containing pull request branches"
	flagLimitNameConstant                       = "limit"
	flagLimitDescriptionConstant                = "Maximum number of closed pull requests to examine"
	invalidRemoteNameErrorMessageConstant       = "remote name must not be empty or whitespace"
	invalidPullRequestLimitErrorMessageConstant = "limit must be greater than zero"
)

// RepositoryDiscoverer locates Git repositories beneath the provided roots.
type RepositoryDiscoverer interface {
	DiscoverRepositories(roots []string) ([]string, error)
}

// LoggerProvider supplies a zap logger instance.
type LoggerProvider func() *zap.Logger

// CommandBuilder assembles the repo-prs-purge Cobra command.
type CommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() CommandConfiguration
	PrompterFactory              func(*cobra.Command) shared.ConfirmationPrompter
	TaskRunnerFactory            func(workflow.Dependencies) TaskRunnerExecutor
}

// TaskRunnerExecutor coordinates workflow task execution.
type TaskRunnerExecutor interface {
	Run(ctx context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) error
}

type taskRunnerAdapter struct {
	runner workflow.TaskRunner
}

func (adapter taskRunnerAdapter) Run(ctx context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) error {
	return adapter.runner.Run(ctx, roots, definitions, options)
}

// Build constructs the repo-prs-purge command.
func (builder *CommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   commandUseConstant,
		Short: commandShortDescriptionConstant,
		Long:  commandLongDescriptionConstant,
		RunE:  builder.run,
	}

	command.Flags().Int(flagLimitNameConstant, defaultPullRequestLimitConstant, flagLimitDescriptionConstant)
	flagutils.EnsureRemoteFlag(command, defaultRemoteNameConstant, flagRemoteDescriptionConstant)

	return command, nil
}

func (builder *CommandBuilder) run(command *cobra.Command, arguments []string) error {
	options, optionsError := builder.parseOptions(command, arguments)
	if optionsError != nil {
		return optionsError
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

	repositoryManager, ok := gitManager.(*gitrepo.RepositoryManager)
	if !ok {
		constructedManager, constructedManagerError := gitrepo.NewRepositoryManager(gitExecutor)
		if constructedManagerError != nil {
			return constructedManagerError
		}
		repositoryManager = constructedManager
	}

	repositoryDiscoverer := dependencies.ResolveRepositoryDiscoverer(builder.Discoverer)
	fileSystem := dependencies.ResolveFileSystem(builder.FileSystem)
	prompter := builder.resolvePrompter(command)

	taskDependencies := workflow.Dependencies{
		Logger:               logger,
		RepositoryDiscoverer: repositoryDiscoverer,
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         nil,
		FileSystem:           fileSystem,
		Prompter:             prompter,
		Output:               command.OutOrStdout(),
		Errors:               command.ErrOrStderr(),
	}

	taskRunner := builder.resolveTaskRunner(taskDependencies)

	actionOptions := map[string]any{
		"remote": options.CleanupOptions.RemoteName,
		"limit":  strconv.Itoa(options.CleanupOptions.PullRequestLimit),
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        "Cleanup pull request branches",
		EnsureClean: false,
		Actions: []workflow.TaskActionDefinition{
			{Type: "repo.branches.cleanup", Options: actionOptions},
		},
		Commit: workflow.TaskCommitDefinition{},
	}

	runtimeOptions := workflow.RuntimeOptions{
		DryRun:                 options.CleanupOptions.DryRun,
		AssumeYes:              options.CleanupOptions.AssumeYes,
		SkipRepositoryMetadata: true,
	}
	return taskRunner.Run(command.Context(), options.RepositoryRoots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
}

type commandOptions struct {
	CleanupOptions  CleanupOptions
	RepositoryRoots []string
}

func (builder *CommandBuilder) parseOptions(command *cobra.Command, arguments []string) (commandOptions, error) {
	configuration := builder.resolveConfiguration()
	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

	trimmedRemoteName := strings.TrimSpace(configuration.RemoteName)
	if executionFlagsAvailable && executionFlags.RemoteSet {
		overrideRemote := strings.TrimSpace(executionFlags.Remote)
		if len(overrideRemote) == 0 {
			if command != nil {
				_ = command.Help()
			}
			return commandOptions{}, errors.New(invalidRemoteNameErrorMessageConstant)
		}
		trimmedRemoteName = overrideRemote
	}
	if len(trimmedRemoteName) == 0 && builder.ConfigurationProvider == nil {
		trimmedRemoteName = defaultRemoteNameConstant
	}
	if len(trimmedRemoteName) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return commandOptions{}, errors.New(invalidRemoteNameErrorMessageConstant)
	}

	limitValue := configuration.PullRequestLimit
	if command != nil {
		flagLimitValue, _ := command.Flags().GetInt(flagLimitNameConstant)
		if command.Flags().Changed(flagLimitNameConstant) {
			limitValue = flagLimitValue
		} else if limitValue == 0 && builder.ConfigurationProvider == nil {
			limitValue = flagLimitValue
		}
	}
	if limitValue <= 0 {
		if command != nil {
			_ = command.Help()
		}
		return commandOptions{}, errors.New(invalidPullRequestLimitErrorMessageConstant)
	}

	dryRunValue := configuration.DryRun
	if executionFlagsAvailable && executionFlags.DryRunSet {
		dryRunValue = executionFlags.DryRun
	}

	assumeYesValue := configuration.AssumeYes
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYesValue = executionFlags.AssumeYes
	}

	cleanupOptions := CleanupOptions{
		RemoteName:       trimmedRemoteName,
		PullRequestLimit: limitValue,
		DryRun:           dryRunValue,
		AssumeYes:        assumeYesValue,
	}

	repositoryRoots, rootsError := rootutils.Resolve(command, arguments, configuration.RepositoryRoots)
	if rootsError != nil {
		return commandOptions{}, rootsError
	}

	return commandOptions{CleanupOptions: cleanupOptions, RepositoryRoots: repositoryRoots}, nil
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

func (builder *CommandBuilder) resolvePrompter(command *cobra.Command) shared.ConfirmationPrompter {
	if builder.PrompterFactory != nil {
		if prompter := builder.PrompterFactory(command); prompter != nil {
			return prompter
		}
	}

	if command == nil {
		return nil
	}

	return prompt.NewIOConfirmationPrompter(command.InOrStdin(), command.OutOrStdout())
}

func (builder *CommandBuilder) resolveConfiguration() CommandConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultCommandConfiguration()
	}

	provided := builder.ConfigurationProvider()
	return provided.Sanitize()
}

func (builder *CommandBuilder) resolveTaskRunner(dependencies workflow.Dependencies) TaskRunnerExecutor {
	if builder.TaskRunnerFactory != nil {
		return builder.TaskRunnerFactory(dependencies)
	}
	return taskRunnerAdapter{runner: workflow.NewTaskRunner(dependencies)}
}
