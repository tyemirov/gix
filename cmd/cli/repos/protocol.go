package repos

import (
	"errors"
	"fmt"
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
)

// ProtocolCommandBuilder assembles the repo-protocol-convert command.
type ProtocolCommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	GitHubResolver               shared.GitHubMetadataResolver
	PrompterFactory              PrompterFactory
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() ProtocolConfiguration
	TaskRunnerFactory            func(workflow.Dependencies) TaskRunnerExecutor
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
	taskDependencies.Output = command.OutOrStdout()
	taskDependencies.Errors = command.ErrOrStderr()

	taskRunner := ResolveTaskRunner(builder.TaskRunnerFactory, taskDependencies)

	taskDefinition := workflow.TaskDefinition{
		Name: "Convert remote protocol",
		Actions: []workflow.TaskActionDefinition{
			{
				Type: "repo.remote.convert-protocol",
				Options: map[string]any{
					"from": string(fromProtocol),
					"to":   string(toProtocol),
				},
			},
		},
		Commit: workflow.TaskCommitDefinition{},
	}

	runtimeOptions := workflow.RuntimeOptions{AssumeYes: trackingPrompter.AssumeYes()}

	return taskRunner.Run(command.Context(), roots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
}

func (builder *ProtocolCommandBuilder) resolveConfiguration() ProtocolConfiguration {
	if builder.ConfigurationProvider == nil {
		defaults := DefaultToolsConfiguration()
		return defaults.Protocol
	}

	provided := builder.ConfigurationProvider()
	return provided.sanitize()
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
