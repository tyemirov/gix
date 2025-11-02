package workflow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

var (
	errSafeguardEnvironment = errors.New("safeguards require workflow environment and repository state")
	errSafeguardRepoManager = errors.New("repository manager not configured for safeguard evaluation")
	errSafeguardFileSystem  = errors.New("filesystem not configured for safeguard evaluation")
)

// EvaluateSafeguards validates whether the current repository satisfies the provided safeguards.
// It returns false with a human-readable reason when a safeguard fails.
func EvaluateSafeguards(ctx context.Context, environment *Environment, repository *RepositoryState, raw map[string]any) (bool, string, error) {
	if len(raw) == 0 {
		return true, "", nil
	}
	if environment == nil || repository == nil {
		return false, "", errSafeguardEnvironment
	}

	reader := newOptionReader(raw)
	repositoryPath := strings.TrimSpace(repository.Path)

	requireClean, requireCleanExists, requireCleanError := reader.boolValue("require_clean")
	if requireCleanError != nil {
		return false, "", requireCleanError
	}
	if requireCleanExists && requireClean {
		if environment.RepositoryManager == nil {
			return false, "", errSafeguardRepoManager
		}
		clean, cleanError := environment.RepositoryManager.CheckCleanWorktree(ctx, repositoryPath)
		if cleanError != nil {
			return false, "", cleanError
		}
		if !clean {
			return false, "repository not clean", nil
		}
	}

	requiredBranch, branchExists, branchError := reader.stringValue("branch")
	if branchError != nil {
		return false, "", branchError
	}
	if branchExists && len(strings.TrimSpace(requiredBranch)) > 0 {
		if environment.RepositoryManager == nil {
			return false, "", errSafeguardRepoManager
		}
		currentBranch, branchReadError := environment.RepositoryManager.GetCurrentBranch(ctx, repositoryPath)
		if branchReadError != nil {
			return false, "", branchReadError
		}
		if strings.TrimSpace(currentBranch) != requiredBranch {
			return false, fmt.Sprintf("requires branch %s", requiredBranch), nil
		}
	}

	for _, candidateBranch := range parseBranchList(raw["branch_in"]) {
		if len(candidateBranch) == 0 {
			continue
		}
		if environment.RepositoryManager == nil {
			return false, "", errSafeguardRepoManager
		}
		currentBranch, branchReadError := environment.RepositoryManager.GetCurrentBranch(ctx, repositoryPath)
		if branchReadError != nil {
			return false, "", branchReadError
		}
		if matchesBranch(candidateBranch, currentBranch) {
			goto pathsCheck
		}
	}
	if branches := parseBranchList(raw["branch_in"]); len(branches) > 0 {
		return false, fmt.Sprintf("requires branch in %s", strings.Join(branches, ", ")), nil
	}

pathsCheck:
	requiredPaths := parseSafeguardPaths(raw["paths"])
	if len(requiredPaths) > 0 {
		if environment.FileSystem == nil {
			return false, "", errSafeguardFileSystem
		}
		for _, relativePath := range requiredPaths {
			absolutePath := filepath.Join(repositoryPath, relativePath)
			if _, statError := environment.FileSystem.Stat(absolutePath); statError != nil {
				if errors.Is(statError, fs.ErrNotExist) {
					return false, fmt.Sprintf("missing required path %s", relativePath), nil
				}
				return false, "", statError
			}
		}
	}

	return true, "", nil
}

func parseBranchList(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return sanitizeBranchSlice(typed)
	case []any:
		branches := make([]string, 0, len(typed))
		for _, entry := range typed {
			value, ok := entry.(string)
			if !ok {
				continue
			}
			branches = append(branches, value)
		}
		return sanitizeBranchSlice(branches)
	case string:
		return sanitizeBranchSlice([]string{typed})
	default:
		return nil
	}
}

func sanitizeBranchSlice(values []string) []string {
	sanitized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if len(trimmed) == 0 {
			continue
		}
		sanitized = append(sanitized, trimmed)
	}
	return sanitized
}

func matchesBranch(expected string, actual string) bool {
	return strings.TrimSpace(actual) == strings.TrimSpace(expected)
}

func parseSafeguardPaths(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return sanitizePathSlice(typed)
	case []any:
		paths := make([]string, 0, len(typed))
		for _, entry := range typed {
			value, ok := entry.(string)
			if !ok {
				continue
			}
			paths = append(paths, value)
		}
		return sanitizePathSlice(paths)
	case string:
		return sanitizePathSlice([]string{typed})
	default:
		return nil
	}
}

func sanitizePathSlice(paths []string) []string {
	sanitized := make([]string, 0, len(paths))
	for _, pathValue := range paths {
		trimmed := strings.TrimSpace(pathValue)
		if len(trimmed) == 0 {
			continue
		}
		cleaned := strings.TrimPrefix(trimmed, "./")
		if len(cleaned) == 0 {
			continue
		}
		sanitized = append(sanitized, filepath.Clean(cleaned))
	}
	return sanitized
}
