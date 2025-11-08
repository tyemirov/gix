package repos

import (
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/temirov/gix/internal/repos/shared"
	rootutils "github.com/temirov/gix/internal/utils/roots"
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
)

// LoggerProvider yields a zap logger for command execution.
type LoggerProvider func() *zap.Logger

// PrompterFactory creates confirmation prompters scoped to a Cobra command.
type PrompterFactory func(*cobra.Command) shared.ConfirmationPrompter

// TaskRunnerExecutor represents a workflow task runner.
type TaskRunnerExecutor = taskrunner.Executor

// TaskRunnerFactory constructs workflow executors.
type TaskRunnerFactory = taskrunner.Factory

type dependencyInputs struct {
	LoggerProvider               LoggerProvider
	HumanReadableLoggingProvider func() bool
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	GitHubResolver               shared.GitHubMetadataResolver
	FileSystem                   shared.FileSystem
	PrompterFactory              PrompterFactory
}

func buildDependencies(
	command *cobra.Command,
	inputs dependencyInputs,
	options taskrunner.DependenciesOptions,
) (taskrunner.DependenciesResult, error) {
	if options.Command == nil {
		options.Command = command
	}
	config := taskrunner.DependenciesConfig{
		LoggerProvider:               inputs.LoggerProvider,
		HumanReadableLoggingProvider: inputs.HumanReadableLoggingProvider,
		RepositoryDiscoverer:         inputs.Discoverer,
		GitExecutor:                  inputs.GitExecutor,
		GitRepositoryManager:         inputs.GitManager,
		GitHubResolver:               inputs.GitHubResolver,
		FileSystem:                   inputs.FileSystem,
		PrompterFactory:              inputs.PrompterFactory,
	}
	return taskrunner.BuildDependencies(config, options)
}

// ResolveTaskRunner returns either the provided executor or a default workflow runner.
func ResolveTaskRunner(factory TaskRunnerFactory, dependencies workflow.Dependencies) TaskRunnerExecutor {
	return taskrunner.Resolve(factory, dependencies)
}

func requireRepositoryRoots(command *cobra.Command, arguments []string, configuredRoots []string) ([]string, error) {
	roots, resolveError := rootutils.Resolve(command, arguments, configuredRoots)
	if resolveError != nil {
		return nil, resolveError
	}
	return roots, nil
}

func resolveLogger(provider LoggerProvider) *zap.Logger {
	if provider == nil {
		return zap.NewNop()
	}
	logger := provider()
	if logger == nil {
		return zap.NewNop()
	}
	return logger
}

// cascadingConfirmationPrompter forwards confirmations while tracking apply-to-all decisions.
type cascadingConfirmationPrompter struct {
	basePrompter shared.ConfirmationPrompter
	assumeYes    bool
}

func newCascadingConfirmationPrompter(base shared.ConfirmationPrompter, initialAssumeYes bool) *cascadingConfirmationPrompter {
	return &cascadingConfirmationPrompter{basePrompter: base, assumeYes: initialAssumeYes}
}

func (prompter *cascadingConfirmationPrompter) Confirm(prompt string) (shared.ConfirmationResult, error) {
	if prompter.basePrompter == nil {
		return shared.ConfirmationResult{}, nil
	}
	result, err := prompter.basePrompter.Confirm(prompt)
	if err != nil {
		return shared.ConfirmationResult{}, err
	}
	if result.ApplyToAll {
		prompter.assumeYes = true
	}
	return result, nil
}

func (prompter *cascadingConfirmationPrompter) AssumeYes() bool {
	return prompter.assumeYes
}

func displayCommandHelp(command *cobra.Command) error {
	if command == nil {
		return nil
	}
	return command.Help()
}
