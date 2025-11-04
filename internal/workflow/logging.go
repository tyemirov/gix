package workflow

import (
	"github.com/tyemirov/gix/internal/repos/shared"
)

// ReportRepositoryEvent emits a structured event for the provided repository context.
func (environment *Environment) ReportRepositoryEvent(repository *RepositoryState, level shared.EventLevel, code string, message string, details map[string]string) {
	if environment == nil || environment.Reporter == nil {
		return
	}

	repositoryIdentifier := ""
	repositoryPath := ""
	if repository != nil {
		repositoryIdentifier = selectOwnerRepository(repository)
		repositoryPath = repository.Path
	}

	metadata := make(map[string]string, len(details))
	for key, value := range details {
		metadata[key] = value
	}

	environment.Reporter.Report(shared.Event{
		Level:                level,
		Code:                 code,
		RepositoryIdentifier: repositoryIdentifier,
		RepositoryPath:       repositoryPath,
		Message:              message,
		Details:              metadata,
	})
}

// ReportPathEvent emits an event scoped to a repository path.
func (environment *Environment) ReportPathEvent(level shared.EventLevel, code string, repositoryPath string, message string, details map[string]string) {
	if environment == nil || environment.Reporter == nil {
		return
	}

	metadata := make(map[string]string, len(details))
	for key, value := range details {
		metadata[key] = value
	}

	environment.Reporter.Report(shared.Event{
		Level:          level,
		Code:           code,
		RepositoryPath: repositoryPath,
		Message:        message,
		Details:        metadata,
	})
}
