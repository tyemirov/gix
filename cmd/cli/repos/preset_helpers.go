package repos

import (
	workflowcmd "github.com/tyemirov/gix/cmd/cli/workflow"
	"github.com/tyemirov/gix/internal/repos/shared"
)

type presetCommandDependencies struct {
	LoggerProvider               workflowcmd.LoggerProvider
	HumanReadableLoggingProvider func() bool
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	GitHubResolver               shared.GitHubMetadataResolver
	FileSystem                   shared.FileSystem
	PrompterFactory              workflowcmd.PrompterFactory
	PresetCatalogFactory         func() workflowcmd.PresetCatalog
	WorkflowExecutorFactory      workflowcmd.OperationExecutorFactory
}

func newPresetCommand(dependencies presetCommandDependencies) workflowcmd.PresetCommand {
	return workflowcmd.PresetCommand{
		LoggerProvider:               dependencies.LoggerProvider,
		HumanReadableLoggingProvider: dependencies.HumanReadableLoggingProvider,
		RepositoryDiscoverer:         dependencies.Discoverer,
		GitExecutor:                  dependencies.GitExecutor,
		GitRepositoryManager:         dependencies.GitManager,
		GitHubResolver:               dependencies.GitHubResolver,
		FileSystem:                   dependencies.FileSystem,
		PrompterFactory:              dependencies.PrompterFactory,
		PresetCatalogFactory:         dependencies.PresetCatalogFactory,
		WorkflowExecutorFactory:      dependencies.WorkflowExecutorFactory,
	}
}
