package repos

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/repos/shared"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
)

const (
	remotesUseConstant                  = "repo-remote-update"
	remotesShortDescription             = "Update origin URLs to match canonical GitHub repositories"
	remotesLongDescription              = "repo-remote-update adjusts origin remotes to point to canonical GitHub repositories."
	remotesOwnerFlagName                = "owner"
	remotesOwnerFlagDescription         = "Require canonical owner to match this value"
	remoteCanonicalPresetName           = "remote-update-to-canonical"
	remoteCanonicalCommandKey           = "remote update-to-canonical"
	remotesPresetLoadErrorTemplate      = "unable to load remote-update-to-canonical preset: %w"
	remotesPresetMissingMessage         = "remote-update-to-canonical preset not found"
	remotesBuildOperationsErrorTemplate = "unable to build remote-update-to-canonical workflow: %w"
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
	PresetCatalogFactory         func() workflowcmd.PresetCatalog
	WorkflowExecutorFactory      WorkflowExecutorFactory
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
	configuration := builder.resolveConfiguration()
	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

	assumeYes := configuration.AssumeYes
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

	ownerConstraint := configuration.Owner
	if command != nil {
		ownerValue, ownerChanged, ownerError := flagutils.StringFlag(command, remotesOwnerFlagName)
		if ownerError != nil && !errors.Is(ownerError, flagutils.ErrFlagNotDefined) {
			return ownerError
		}
		if ownerChanged {
			ownerConstraint = strings.TrimSpace(ownerValue)
		}
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

	workflowDependencies := dependencyResult.Workflow
	if command != nil {
		workflowDependencies.Output = command.OutOrStdout()
		workflowDependencies.Errors = command.ErrOrStderr()
	}

	presetCatalog := builder.resolvePresetCatalog()
	presetConfiguration, presetFound, presetError := presetCatalog.Load(remoteCanonicalPresetName)
	if presetError != nil {
		return fmt.Errorf(remotesPresetLoadErrorTemplate, presetError)
	}
	if !presetFound {
		return errors.New(remotesPresetMissingMessage)
	}

	trimmedOwner := strings.TrimSpace(ownerConstraint)
	for index := range presetConfiguration.Steps {
		if workflow.CommandPathKey(presetConfiguration.Steps[index].Command) != remoteCanonicalCommandKey {
			continue
		}
		if presetConfiguration.Steps[index].Options == nil {
			presetConfiguration.Steps[index].Options = make(map[string]any)
		}
		presetConfiguration.Steps[index].Options["owner"] = trimmedOwner
	}

	nodes, operationsError := workflow.BuildOperations(presetConfiguration)
	if operationsError != nil {
		return fmt.Errorf(remotesBuildOperationsErrorTemplate, operationsError)
	}

	runtimeOptions := workflow.RuntimeOptions{AssumeYes: assumeYes}
	executor := resolveWorkflowExecutor(builder.WorkflowExecutorFactory, nodes, workflowDependencies)
	_, runErr := executor.Execute(command.Context(), roots, runtimeOptions)
	return runErr
}

func (builder *RemotesCommandBuilder) resolveConfiguration() RemotesConfiguration {
	if builder.ConfigurationProvider == nil {
		defaults := DefaultToolsConfiguration()
		return defaults.Remotes
	}

	provided := builder.ConfigurationProvider()
	return provided.sanitize()
}

func (builder *RemotesCommandBuilder) resolvePresetCatalog() workflowcmd.PresetCatalog {
	if builder.PresetCatalogFactory != nil {
		if catalog := builder.PresetCatalogFactory(); catalog != nil {
			return catalog
		}
	}
	return workflowcmd.NewEmbeddedPresetCatalog()
}
