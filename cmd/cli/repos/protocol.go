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
	protocolUseConstant         = "repo-protocol-convert"
	protocolShortDescription    = "Convert repository origin URLs between git/ssh/https"
	protocolLongDescription     = "repo-protocol-convert converts origin URLs to a desired protocol."
	protocolFromFlagName        = "from"
	protocolFromFlagDescription = "Current protocol to convert from (git, ssh, https)"
	protocolToFlagName          = "to"
	protocolToFlagDescription   = "Target protocol to convert to (git, ssh, https)"
	protocolErrorMissingPair    = "specify both --from and --to"
	protocolErrorSamePair       = "--from and --to must differ"
	protocolErrorInvalidValue   = "invalid protocol value: %s"
	protocolPresetName          = "remote-update-protocol"
	protocolCommandKey          = "remote update-protocol"
	protocolPresetMissingError  = "remote-update-protocol preset not found"
	protocolPresetLoadError     = "unable to load remote-update-protocol preset: %w"
	protocolBuildWorkflowError  = "unable to build remote-update-protocol workflow: %w"
)

// ProtocolCommandBuilder assembles the repo-protocol-convert command.
type ProtocolCommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	GitHubResolver               shared.GitHubMetadataResolver
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() ProtocolConfiguration
	PresetCatalogFactory         func() workflowcmd.PresetCatalog
	WorkflowExecutorFactory      WorkflowExecutorFactory
}

// Build constructs the repo-protocol-convert command.
func (builder *ProtocolCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   protocolUseConstant,
		Short: protocolShortDescription,
		Long:  protocolLongDescription,
		Args:  cobra.NoArgs,
		RunE:  builder.run,
	}

	command.Flags().String(protocolFromFlagName, "", protocolFromFlagDescription)
	command.Flags().String(protocolToFlagName, "", protocolToFlagDescription)

	return command, nil
}

func (builder *ProtocolCommandBuilder) run(command *cobra.Command, arguments []string) error {
	configuration := builder.resolveConfiguration()
	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

	assumeYes := configuration.AssumeYes
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

	fromValue := configuration.FromProtocol
	if command != nil && command.Flags().Changed(protocolFromFlagName) {
		fromValue, _ = command.Flags().GetString(protocolFromFlagName)
	}

	toValue := configuration.ToProtocol
	if command != nil && command.Flags().Changed(protocolToFlagName) {
		toValue, _ = command.Flags().GetString(protocolToFlagName)
	}

	if len(strings.TrimSpace(fromValue)) == 0 || len(strings.TrimSpace(toValue)) == 0 {
		if helpError := displayCommandHelp(command); helpError != nil {
			return helpError
		}
		return errors.New(protocolErrorMissingPair)
	}

	fromProtocol, fromError := parseProtocolValue(fromValue)
	if fromError != nil {
		return fromError
	}

	toProtocol, toError := parseProtocolValue(toValue)
	if toError != nil {
		return toError
	}

	if fromProtocol == toProtocol {
		return errors.New(protocolErrorSamePair)
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
	presetConfiguration, presetFound, presetError := presetCatalog.Load(protocolPresetName)
	if presetError != nil {
		return fmt.Errorf(protocolPresetLoadError, presetError)
	}
	if !presetFound {
		return errors.New(protocolPresetMissingError)
	}

	for index := range presetConfiguration.Steps {
		if workflow.CommandPathKey(presetConfiguration.Steps[index].Command) != protocolCommandKey {
			continue
		}
		if presetConfiguration.Steps[index].Options == nil {
			presetConfiguration.Steps[index].Options = make(map[string]any)
		}
		presetConfiguration.Steps[index].Options["from"] = string(fromProtocol)
		presetConfiguration.Steps[index].Options["to"] = string(toProtocol)
	}

	nodes, operationsError := workflow.BuildOperations(presetConfiguration)
	if operationsError != nil {
		return fmt.Errorf(protocolBuildWorkflowError, operationsError)
	}

	runtimeOptions := workflow.RuntimeOptions{AssumeYes: assumeYes}
	executor := resolveWorkflowExecutor(builder.WorkflowExecutorFactory, nodes, workflowDependencies)
	_, runErr := executor.Execute(command.Context(), roots, runtimeOptions)
	return runErr
}

func (builder *ProtocolCommandBuilder) resolveConfiguration() ProtocolConfiguration {
	if builder.ConfigurationProvider == nil {
		defaults := DefaultToolsConfiguration()
		return defaults.Protocol
	}

	provided := builder.ConfigurationProvider()
	return provided.sanitize()
}

func (builder *ProtocolCommandBuilder) resolvePresetCatalog() workflowcmd.PresetCatalog {
	if builder.PresetCatalogFactory != nil {
		if catalog := builder.PresetCatalogFactory(); catalog != nil {
			return catalog
		}
	}
	return workflowcmd.NewEmbeddedPresetCatalog()
}

func parseProtocolValue(value string) (shared.RemoteProtocol, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	switch trimmed {
	case string(shared.RemoteProtocolGit):
		return shared.RemoteProtocolGit, nil
	case string(shared.RemoteProtocolSSH):
		return shared.RemoteProtocolSSH, nil
	case string(shared.RemoteProtocolHTTPS):
		return shared.RemoteProtocolHTTPS, nil
	default:
		return "", fmt.Errorf(protocolErrorInvalidValue, value)
	}
}
