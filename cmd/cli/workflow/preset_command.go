package workflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/temirov/gix/internal/repos/shared"
	"github.com/temirov/gix/internal/utils"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	rootutils "github.com/temirov/gix/internal/utils/roots"
	workflowpkg "github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
)

// PresetCommand centralizes shared preset-backed command behavior.
type PresetCommand struct {
	LoggerProvider               LoggerProvider
	HumanReadableLoggingProvider func() bool
	RepositoryDiscoverer         shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitRepositoryManager         shared.GitRepositoryManager
	GitHubResolver               shared.GitHubMetadataResolver
	FileSystem                   shared.FileSystem
	PrompterFactory              PrompterFactory
	PresetCatalogFactory         func() PresetCatalog
	WorkflowExecutorFactory      OperationExecutorFactory
}

// PresetCommandRequest describes a preset-backed command execution.
type PresetCommandRequest struct {
	Command                 *cobra.Command
	Arguments               []string
	RootArguments           []string
	ConfiguredAssumeYes     bool
	ConfiguredRoots         []string
	PresetName              string
	PresetMissingMessage    string
	PresetLoadErrorTemplate string
	BuildErrorTemplate      string
	Configure               func(PresetCommandContext) (PresetCommandResult, error)
	DependenciesOptions     taskrunner.DependenciesOptions
}

// PresetCommandContext exposes execution context to command-specific callbacks.
type PresetCommandContext struct {
	Command                 *cobra.Command
	Arguments               []string
	ExecutionFlags          utils.ExecutionFlags
	ExecutionFlagsAvailable bool
	Dependencies            taskrunner.DependenciesResult
	Configuration           *workflowpkg.Configuration
	AssumeYes               bool
	Roots                   []string
}

// RuntimeOptions returns workflow runtime defaults derived from the context.
func (ctx PresetCommandContext) RuntimeOptions() *workflowpkg.RuntimeOptions {
	options := workflowpkg.RuntimeOptions{AssumeYes: ctx.AssumeYes}
	return &options
}

// PresetCommandResult captures command-specific preset and runtime adjustments.
type PresetCommandResult struct {
	Configuration     *workflowpkg.Configuration
	RuntimeOptions    *workflowpkg.RuntimeOptions
	PrepareOperations func([]*workflowpkg.OperationNode) error
}

// Execute runs the preset command using the provided request.
func (command PresetCommand) Execute(request PresetCommandRequest) error {
	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(request.Command)

	assumeYes := request.ConfiguredAssumeYes
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

	rootArguments := request.RootArguments
	if rootArguments == nil {
		rootArguments = request.Arguments
	}

	roots, rootsError := rootutils.Resolve(request.Command, rootArguments, request.ConfiguredRoots)
	if rootsError != nil {
		return rootsError
	}

	dependenciesOptions := request.DependenciesOptions
	dependenciesOptions.Command = request.Command
	if dependenciesOptions.Output == nil && request.Command != nil {
		dependenciesOptions.Output = request.Command.OutOrStdout()
	}
	if dependenciesOptions.Errors == nil && request.Command != nil {
		dependenciesOptions.Errors = request.Command.ErrOrStderr()
	}

	dependencyResult, dependencyError := taskrunner.BuildDependencies(
		taskrunner.DependenciesConfig{
			LoggerProvider:               command.LoggerProvider,
			HumanReadableLoggingProvider: command.HumanReadableLoggingProvider,
			RepositoryDiscoverer:         command.RepositoryDiscoverer,
			GitExecutor:                  command.GitExecutor,
			GitRepositoryManager:         command.GitRepositoryManager,
			GitHubResolver:               command.GitHubResolver,
			FileSystem:                   command.FileSystem,
			PrompterFactory:              command.PrompterFactory,
		},
		dependenciesOptions,
	)
	if dependencyError != nil {
		return dependencyError
	}

	workflowDependencies := dependencyResult.Workflow

	presetCatalog := command.resolvePresetCatalog()
	presetConfiguration, presetFound, presetError := presetCatalog.Load(request.PresetName)
	if presetError != nil {
		if len(request.PresetLoadErrorTemplate) > 0 {
			return fmt.Errorf(request.PresetLoadErrorTemplate, presetError)
		}
		return presetError
	}
	if !presetFound {
		if len(request.PresetMissingMessage) > 0 {
			return errors.New(request.PresetMissingMessage)
		}
		return fmt.Errorf("preset %q not found", request.PresetName)
	}

	presetContext := PresetCommandContext{
		Command:                 request.Command,
		Arguments:               request.Arguments,
		ExecutionFlags:          executionFlags,
		ExecutionFlagsAvailable: executionFlagsAvailable,
		Dependencies:            dependencyResult,
		Configuration:           &presetConfiguration,
		AssumeYes:               assumeYes,
		Roots:                   roots,
	}

	result := PresetCommandResult{}
	var configureError error
	if request.Configure != nil {
		result, configureError = request.Configure(presetContext)
		if configureError != nil {
			return configureError
		}
	}

	finalConfiguration := presetContext.Configuration
	if result.Configuration != nil {
		finalConfiguration = result.Configuration
	}
	if finalConfiguration == nil {
		return errors.New("preset command missing configuration")
	}

	runtimeOptions := presetContext.RuntimeOptions()
	if result.RuntimeOptions != nil {
		runtimeOptions = result.RuntimeOptions
	}

	nodes, operationsError := workflowpkg.BuildOperations(*finalConfiguration)
	if operationsError != nil {
		if len(request.BuildErrorTemplate) > 0 {
			return fmt.Errorf(request.BuildErrorTemplate, operationsError)
		}
		return operationsError
	}

	if result.PrepareOperations != nil {
		if prepareError := result.PrepareOperations(nodes); prepareError != nil {
			return prepareError
		}
	}

	executor := ResolveOperationExecutor(command.WorkflowExecutorFactory, nodes, workflowDependencies)
	executionContext := context.Background()
	if request.Command != nil {
		if ctx := request.Command.Context(); ctx != nil {
			executionContext = ctx
		}
	}
	_, runError := executor.Execute(executionContext, roots, *runtimeOptions)
	return runError
}

func (command PresetCommand) resolvePresetCatalog() PresetCatalog {
	if command.PresetCatalogFactory != nil {
		if catalog := command.PresetCatalogFactory(); catalog != nil {
			return catalog
		}
	}
	return NewEmbeddedPresetCatalog()
}
