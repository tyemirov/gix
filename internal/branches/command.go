package branches

import (
	"errors"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/repos/shared"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	rootutils "github.com/tyemirov/gix/internal/utils/roots"
	"github.com/tyemirov/gix/internal/workflow"
	"github.com/tyemirov/gix/pkg/taskrunner"
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

	dependencyResult, dependencyError := taskrunner.BuildDependencies(
		taskrunner.DependenciesConfig{
			LoggerProvider:               builder.LoggerProvider,
			HumanReadableLoggingProvider: builder.HumanReadableLoggingProvider,
			RepositoryDiscoverer:         builder.Discoverer,
			GitExecutor:                  builder.GitExecutor,
			GitRepositoryManager:         builder.GitManager,
			FileSystem:                   builder.FileSystem,
			PrompterFactory:              builder.PrompterFactory,
		},
		taskrunner.DependenciesOptions{
			Command:            command,
			Output:             command.OutOrStdout(),
			Errors:             command.ErrOrStderr(),
			SkipGitHubResolver: true,
		},
	)
	if dependencyError != nil {
		return dependencyError
	}

	taskRunner := resolveTaskRunner(builder.TaskRunnerFactory, dependencyResult.Workflow)

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
		AssumeYes:              options.CleanupOptions.AssumeYes,
		SkipRepositoryMetadata: true,
	}
	_, runErr := taskRunner.Run(command.Context(), options.RepositoryRoots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
	return runErr
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

	assumeYesValue := configuration.AssumeYes
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYesValue = executionFlags.AssumeYes
	}

	cleanupOptions := CleanupOptions{
		RemoteName:       trimmedRemoteName,
		PullRequestLimit: limitValue,
		AssumeYes:        assumeYesValue,
	}

	repositoryRoots, rootsError := rootutils.Resolve(command, arguments, configuration.RepositoryRoots)
	if rootsError != nil {
		return commandOptions{}, rootsError
	}

	return commandOptions{CleanupOptions: cleanupOptions, RepositoryRoots: repositoryRoots}, nil
}

func (builder *CommandBuilder) resolveConfiguration() CommandConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultCommandConfiguration()
	}

	provided := builder.ConfigurationProvider()
	return provided.Sanitize()
}
