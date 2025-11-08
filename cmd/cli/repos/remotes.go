package repos

import (
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/temirov/gix/internal/repos/shared"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
)

const (
	remotesUseConstant          = "repo-remote-update"
	remotesShortDescription     = "Update origin URLs to match canonical GitHub repositories"
	remotesLongDescription      = "repo-remote-update adjusts origin remotes to point to canonical GitHub repositories."
	remotesOwnerFlagName        = "owner"
	remotesOwnerFlagDescription = "Require canonical owner to match this value"
)

// RemotesCommandBuilder assembles the repo-remote-update command.
type RemotesCommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	GitHubResolver               shared.GitHubMetadataResolver
	PrompterFactory              PrompterFactory
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() RemotesConfiguration
	TaskRunnerFactory            func(workflow.Dependencies) TaskRunnerExecutor
}

// Build constructs the repo-remote-update command.
func (builder *RemotesCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   remotesUseConstant,
		Short: remotesShortDescription,
		Long:  remotesLongDescription,
		Args:  cobra.NoArgs,
		RunE:  builder.run,
	}

	command.Flags().String(remotesOwnerFlagName, "", remotesOwnerFlagDescription)

	return command, nil
}

func (builder *RemotesCommandBuilder) run(command *cobra.Command, arguments []string) error {
	if command != nil {
		if command.OutOrStdout() == io.Discard {
			command.SetOut(os.Stdout)
		}
		if command.ErrOrStderr() == io.Discard {
			command.SetErr(os.Stderr)
		}
	}

	configuration := builder.resolveConfiguration()
	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

	assumeYes := configuration.AssumeYes
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

	ownerConstraint := configuration.Owner
	if command != nil && command.Flags().Changed(remotesOwnerFlagName) {
		ownerValue, _ := command.Flags().GetString(remotesOwnerFlagName)
		ownerConstraint = strings.TrimSpace(ownerValue)
	}

	roots, rootsError := requireRepositoryRoots(command, arguments, configuration.RepositoryRoots)
	if rootsError != nil {
		return rootsError
	}

	dependencyResult, dependencyError := buildDependencies(
		command,
		dependencyInputs{
			LoggerProvider:               builder.LoggerProvider,
			HumanReadableLoggingProvider: builder.HumanReadableLoggingProvider,
			Discoverer:                   builder.Discoverer,
			GitExecutor:                  builder.GitExecutor,
			GitManager:                   builder.GitManager,
			GitHubResolver:               builder.GitHubResolver,
			PrompterFactory:              builder.PrompterFactory,
		},
		taskrunner.DependenciesOptions{},
	)
	if dependencyError != nil {
		return dependencyError
	}

	taskDependencies := dependencyResult.Workflow
	trackingPrompter := newCascadingConfirmationPrompter(taskDependencies.Prompter, assumeYes)
	taskDependencies.Prompter = trackingPrompter

	taskRunner := ResolveTaskRunner(builder.TaskRunnerFactory, taskDependencies)

	actionOptions := map[string]any{}
	if len(strings.TrimSpace(ownerConstraint)) > 0 {
		actionOptions["owner"] = ownerConstraint
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        "Update canonical remote",
		EnsureClean: false,
		Actions: []workflow.TaskActionDefinition{
			{Type: "repo.remote.update", Options: actionOptions},
		},
		Commit: workflow.TaskCommitDefinition{},
	}

	runtimeOptions := workflow.RuntimeOptions{AssumeYes: trackingPrompter.AssumeYes()}

	return taskRunner.Run(command.Context(), roots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
}

func (builder *RemotesCommandBuilder) resolveConfiguration() RemotesConfiguration {
	if builder.ConfigurationProvider == nil {
		defaults := DefaultToolsConfiguration()
		return defaults.Remotes
	}

	provided := builder.ConfigurationProvider()
	return provided.sanitize()
}
