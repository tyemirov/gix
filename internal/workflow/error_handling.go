package workflow

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	repoerrors "github.com/tyemirov/gix/internal/repos/errors"
)

const (
	defaultOwnerRepositoryIdentifier = "local-only"
	unknownRepositoryPathIdentifier  = "(unknown-path)"
)

func logRepositoryOperationError(environment *Environment, err error) bool {
	if environment == nil {
		return true
	}

	var operationError repoerrors.OperationError
	if !errors.As(err, &operationError) {
		return false
	}

	if environment.Errors != nil {
		fmt.Fprint(environment.Errors, formatRepositoryOperationError(environment, operationError))
	}

	return true
}

func formatRepositoryOperationError(environment *Environment, operationError repoerrors.OperationError) string {
	code := strings.TrimSpace(operationError.Code())
	if len(code) == 0 {
		code = strings.TrimSpace(string(operationError.Operation()))
	}
	if len(code) == 0 {
		code = "unknown_error"
	}

	subjectPath := strings.TrimSpace(operationError.Subject())
	ownerRepository, repositoryPath := resolveRepositoryOwnerAndPath(environment, subjectPath)

	if len(ownerRepository) == 0 {
		ownerRepository = defaultOwnerRepositoryIdentifier
	}

	if len(repositoryPath) == 0 {
		repositoryPath = strings.TrimSpace(subjectPath)
	}
	if len(repositoryPath) == 0 {
		repositoryPath = unknownRepositoryPathIdentifier
	}

	message := strings.TrimSpace(operationError.Message())
	if len(message) == 0 {
		message = deriveOperationErrorMessage(operationError)
	}
	if len(message) == 0 {
		message = humanizeErrorCode(code)
	}
	if strings.EqualFold(message, code) {
		message = humanizeErrorCode(code)
	}

	return fmt.Sprintf("%s: %s (%s) %s\n", code, ownerRepository, repositoryPath, message)
}

func resolveRepositoryOwnerAndPath(environment *Environment, subject string) (string, string) {
	if environment == nil || environment.State == nil {
		return "", strings.TrimSpace(subject)
	}

	normalizedSubject := filepath.Clean(strings.TrimSpace(subject))
	if normalizedSubject == "." {
		normalizedSubject = ""
	}

	var (
		selectedRepository *RepositoryState
		longestMatch       int
	)

	for repositoryIndex := range environment.State.Repositories {
		repository := environment.State.Repositories[repositoryIndex]
		repositoryPath := filepath.Clean(repository.Path)
		if len(repositoryPath) == 0 {
			continue
		}

		if len(normalizedSubject) == 0 {
			if selectedRepository == nil {
				selectedRepository = repository
				longestMatch = len(repositoryPath)
			}
			continue
		}

		if normalizedSubject == repositoryPath {
			resolvedOwner := selectOwnerRepository(repository)
			resolvedPath := repositoryPath
			if len(resolvedPath) == 0 {
				resolvedPath = filepath.Clean(repository.Path)
			}
			return resolvedOwner, resolvedPath
		}

		pathWithSeparator := repositoryPath + string(filepath.Separator)
		if strings.HasPrefix(normalizedSubject, pathWithSeparator) && len(repositoryPath) > longestMatch {
			selectedRepository = repository
			longestMatch = len(repositoryPath)
		}
	}

	if selectedRepository == nil && len(environment.State.Repositories) == 1 {
		selectedRepository = environment.State.Repositories[0]
	}

	if selectedRepository == nil {
		return "", normalizedSubject
	}

	resolvedPath := normalizedSubject
	if len(resolvedPath) == 0 {
		resolvedPath = filepath.Clean(selectedRepository.Path)
	}

	return selectOwnerRepository(selectedRepository), resolvedPath
}

func selectOwnerRepository(repository *RepositoryState) string {
	if repository == nil {
		return ""
	}

	candidates := []string{
		strings.TrimSpace(repository.Inspection.FinalOwnerRepo),
		strings.TrimSpace(repository.Inspection.CanonicalOwnerRepo),
		strings.TrimSpace(repository.Inspection.OriginOwnerRepo),
	}

	for candidateIndex := range candidates {
		if len(candidates[candidateIndex]) > 0 {
			return candidates[candidateIndex]
		}
	}

	return ""
}

func deriveOperationErrorMessage(operationError repoerrors.OperationError) string {
	raw := strings.TrimSpace(operationError.Error())
	if len(raw) == 0 {
		return ""
	}

	subject := strings.TrimSpace(operationError.Subject())
	operation := strings.TrimSpace(string(operationError.Operation()))

	if len(operation) > 0 && len(subject) > 0 {
		prefix := fmt.Sprintf("%s[%s]:", operation, subject)
		if strings.HasPrefix(raw, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(raw, prefix))
		}
	}

	if len(operation) > 0 {
		prefix := fmt.Sprintf("%s:", operation)
		if strings.HasPrefix(raw, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(raw, prefix))
		}
	}

	return raw
}

func humanizeErrorCode(code string) string {
	if len(code) == 0 {
		return "unknown error"
	}
	readable := strings.ReplaceAll(code, "_", " ")
	return strings.TrimSpace(readable)
}
