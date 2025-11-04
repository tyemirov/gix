package repos

import (
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/dependencies"
	"github.com/tyemirov/gix/internal/repos/shared"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/workflow"
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

	dryRun := configuration.DryRun
	if executionFlagsAvailable && executionFlags.DryRunSet {
		dryRun = executionFlags.DryRun
	}

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

	logger := resolveLogger(builder.LoggerProvider)
	humanReadableLogging := false
	if builder.HumanReadableLoggingProvider != nil {
		humanReadableLogging = builder.HumanReadableLoggingProvider()
	}
	gitExecutor, executorError := dependencies.ResolveGitExecutor(builder.GitExecutor, logger, humanReadableLogging)
	if executorError != nil {
		return executorError
	}

	gitManager, managerError := dependencies.ResolveGitRepositoryManager(builder.GitManager, gitExecutor)
	if managerError != nil {
		return managerError
	}

	repositoryDiscoverer := dependencies.ResolveRepositoryDiscoverer(builder.Discoverer)
	prompter := resolvePrompter(builder.PrompterFactory, command)
	trackingPrompter := newCascadingConfirmationPrompter(prompter, assumeYes)

	resolvedManager, repositoryManager := gitManager, (*gitrepo.RepositoryManager)(nil)
	if concreteManager, ok := resolvedManager.(*gitrepo.RepositoryManager); ok {
		repositoryManager = concreteManager
	} else {
		constructedManager, constructedManagerError := gitrepo.NewRepositoryManager(gitExecutor)
		if constructedManagerError != nil {
			return constructedManagerError
		}
		repositoryManager = constructedManager
	}

	githubClient, githubClientError := githubcli.NewClient(gitExecutor)
	if githubClientError != nil {
		return githubClientError
	}

	outputWriter := command.OutOrStdout()
	if outputWriter == nil || outputWriter == io.Discard {
		outputWriter = os.Stdout
	}

	errorWriter := command.ErrOrStderr()
	if errorWriter == nil || errorWriter == io.Discard {
		errorWriter = os.Stderr
	}

	taskDependencies := workflow.Dependencies{
		Logger:               logger,
		RepositoryDiscoverer: repositoryDiscoverer,
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         githubClient,
		FileSystem:           dependencies.ResolveFileSystem(nil),
		Prompter:             trackingPrompter,
		Output:               outputWriter,
		Errors:               errorWriter,
	}

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

	runtimeOptions := workflow.RuntimeOptions{DryRun: dryRun, AssumeYes: trackingPrompter.AssumeYes()}

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
