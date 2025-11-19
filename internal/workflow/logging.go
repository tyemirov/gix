package workflow

import (
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/repos/shared"
)

// ReportRepositoryEvent emits a structured event for the provided repository context.
func (environment *Environment) ReportRepositoryEvent(repository *RepositoryState, level shared.EventLevel, code string, message string, details map[string]string) {
	if environment == nil || environment.Reporter == nil {
		return
	}

	decorateHeaders := !environment.suppressHeaders

	repositoryIdentifier := ""
	repositoryPath := ""
	if repository != nil {
		repositoryIdentifier = selectOwnerRepository(repository)
		repositoryPath = repository.Path
		if decorateHeaders {
			message = environment.decorateRepositoryMessage(repositoryIdentifier, repositoryPath, message)
		}
	} else {
		environment.clearRepositoryHeader()
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
	environment.clearRepositoryHeader()

	metadata := make(map[string]string, len(details))
	for key, value := range details {
		metadata[key] = value
	}

	if repositoryPath != "" {
		if _, exists := metadata["path"]; !exists {
			metadata["path"] = repositoryPath
		}
	}

	environment.Reporter.Report(shared.Event{
		Level:   level,
		Code:    code,
		Message: message,
		Details: metadata,
	})
}

func formatRepositoryMessage(identifier string, path string, message string) string {
	if strings.TrimSpace(identifier) == "" || strings.TrimSpace(path) == "" {
		return strings.TrimSpace(message)
	}
	header := fmt.Sprintf("-- %s (%s) --", identifier, strings.TrimSpace(path))
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return header
	}
	return fmt.Sprintf("%s\n%s", header, trimmed)
}

func (environment *Environment) decorateRepositoryMessage(identifier string, path string, message string) string {
	if strings.TrimSpace(identifier) == "" || strings.TrimSpace(path) == "" {
		return message
	}
	key := fmt.Sprintf("%s|%s", identifier, path)
	environment.ensureSharedState()
	environment.sharedState.mutex.Lock()
	defer environment.sharedState.mutex.Unlock()
	if environment.sharedState.lastRepositoryKey == key {
		return message
	}
	environment.sharedState.lastRepositoryKey = key
	return formatRepositoryMessage(identifier, path, message)
}

func (environment *Environment) clearRepositoryHeader() {
	environment.ensureSharedState()
	environment.sharedState.mutex.Lock()
	environment.sharedState.lastRepositoryKey = ""
	environment.sharedState.mutex.Unlock()
}
