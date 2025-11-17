package release

import (
	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/repos/shared"
)

type presetCommandDependencies struct {
	LoggerProvider               workflowcmd.LoggerProvider
	HumanReadableLoggingProvider func() bool
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
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
		FileSystem:                   dependencies.FileSystem,
		PresetCatalogFactory:         dependencies.PresetCatalogFactory,
		WorkflowExecutorFactory:      dependencies.WorkflowExecutorFactory,
	}
}
