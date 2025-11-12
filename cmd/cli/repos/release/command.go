package release

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	repocli "github.com/temirov/gix/cmd/cli/repos"
	"github.com/temirov/gix/internal/repos/shared"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	rootutils "github.com/temirov/gix/internal/utils/roots"
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
)

const (
	commandUseName          = "release"
	commandUsageTemplate    = commandUseName + " <tag>"
	commandExampleTemplate  = "gix release v1.2.3 --roots ~/Development"
	commandShortDescription = "Create and push an annotated release tag"
	commandLongDescription  = "release annotates the provided tag (default message 'Release <tag>') and pushes it to the configured remote for each repository root. Provide the tag as the first argument before any optional repository roots or flags."
	messageFlagName         = "message"
	messageFlagUsage        = "Override the tag message"
	missingTagErrorMessage  = "tag name is required"
)

// CommandBuilder assembles the release command.
type CommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() CommandConfiguration
	TaskRunnerFactory            func(workflow.Dependencies) repocli.TaskRunnerExecutor
}

// Build constructs the repo release command.
func (builder *CommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:     commandUsageTemplate,
		Short:   commandShortDescription,
		Long:    commandLongDescription,
		Example: commandExampleTemplate,
		Args:    cobra.ArbitraryArgs,
		RunE:    builder.run,
	}

	command.Flags().String(messageFlagName, "", messageFlagUsage)

	return command, nil
}

func (builder *CommandBuilder) run(command *cobra.Command, arguments []string) error {
	configuration := builder.resolveConfiguration()

	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

	if len(arguments) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return errors.New(missingTagErrorMessage)
	}

	tagName := strings.TrimSpace(arguments[0])
	if len(tagName) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return errors.New(missingTagErrorMessage)
	}
	additionalArgs := arguments[1:]

	messageValue := configuration.Message
	if command != nil {
		if flagValue, flagError := command.Flags().GetString(messageFlagName); flagError == nil && command.Flags().Changed(messageFlagName) {
			messageValue = strings.TrimSpace(flagValue)
		}
	}

	remoteName := configuration.RemoteName
	if executionFlagsAvailable && executionFlags.RemoteSet {
		override := strings.TrimSpace(executionFlags.Remote)
		if len(override) > 0 {
			remoteName = override
		}
	}

	repositoryRoots, rootsError := rootutils.Resolve(command, additionalArgs, configuration.RepositoryRoots)
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
		taskrunner.DependenciesOptions{
			Command:         command,
			DisablePrompter: true,
		},
	)
	if dependencyError != nil {
		return dependencyError
	}

	taskRunner := repocli.ResolveTaskRunner(builder.TaskRunnerFactory, dependencyResult.Workflow)

	taskName := "Create release tag"
	if len(tagName) > 0 {
		taskName = "Create release tag " + tagName
	}

	actionOptions := map[string]any{
		"tag": tagName,
	}
	if len(messageValue) > 0 {
		actionOptions["message"] = messageValue
	}
	if len(remoteName) > 0 {
		actionOptions["remote"] = remoteName
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        taskName,
		EnsureClean: false,
		Actions: []workflow.TaskActionDefinition{
			{Type: "repo.release.tag", Options: actionOptions},
		},
		Commit: workflow.TaskCommitDefinition{},
	}

	assumeYes := false
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

	runtimeOptions := workflow.RuntimeOptions{AssumeYes: assumeYes}

	_, runErr := taskRunner.Run(command.Context(), repositoryRoots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
	return runErr
}

func (builder *CommandBuilder) resolveConfiguration() CommandConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultCommandConfiguration()
	}
	return builder.ConfigurationProvider().Sanitize()
}
