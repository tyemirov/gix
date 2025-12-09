package workflow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/tyemirov/gix/internal/repos/worktree"
)

var (
	errSafeguardEnvironment = errors.New("safeguards require workflow environment and repository state")
	errSafeguardRepoManager = errors.New("repository manager not configured for safeguard evaluation")
	errSafeguardFileSystem  = errors.New("filesystem not configured for safeguard evaluation")
)

const maxStatusReasonEntries = 5

type safeguardFallback int

const (
	safeguardDefaultHardStop safeguardFallback = iota
	safeguardDefaultSoftSkip
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

	requireClean, ignoredPatterns, requireCleanExists, requireCleanError := readRequireCleanDirective(raw)
	if requireCleanError != nil {
		return false, "", requireCleanError
	}
	if requireCleanExists && requireClean {
		if environment.RepositoryManager == nil {
			return false, "", errSafeguardRepoManager
		}
		statusResult, statusError := worktree.CheckStatus(ctx, environment.RepositoryManager, repositoryPath, ignoredPatterns)
		if statusError != nil {
			return false, "", statusError
		}
		if len(statusResult.Entries) > 0 {
			displayEntries := statusResult.Entries
			if len(displayEntries) > maxStatusReasonEntries {
				displayEntries = append([]string(nil), statusResult.Entries[:maxStatusReasonEntries]...)
			} else {
				displayEntries = append([]string(nil), statusResult.Entries...)
			}
			for index := range displayEntries {
				displayEntries[index] = strings.TrimSpace(displayEntries[index])
			}
			reason := fmt.Sprintf("repository not clean: %s", strings.Join(displayEntries, ", "))
			if len(statusResult.Entries) > maxStatusReasonEntries {
				reason = fmt.Sprintf("%s (+%d more)", reason, len(statusResult.Entries)-maxStatusReasonEntries)
			}
			return false, reason, nil
		}
	}

	requireChanges, requireChangesExists, requireChangesErr := reader.boolValue("require_changes")
	if requireChangesErr != nil {
		return false, "", requireChangesErr
	}
	if requireChangesExists && requireChanges {
		if environment.RepositoryManager == nil {
			return false, "", errSafeguardRepoManager
		}
		statusResult, statusError := worktree.CheckStatus(ctx, environment.RepositoryManager, repositoryPath, nil)
		if statusError != nil {
			return false, "", statusError
		}
		if len(statusResult.Entries) == 0 {
			return false, "requires changes", nil
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
	fileExistsPaths := parseSafeguardPaths(raw["file_exists"])
	if len(fileExistsPaths) > 0 {
		requiredPaths = append(requiredPaths, fileExistsPaths...)
	}
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

func splitSafeguardSets(raw map[string]any, fallback safeguardFallback) (map[string]any, map[string]any) {
	if len(raw) == 0 {
		return nil, nil
	}

	normalized := make(map[string]any, len(raw))
	for key, value := range raw {
		normalized[strings.TrimSpace(strings.ToLower(key))] = value
	}

	var hard, soft map[string]any

	if hardValue, exists := normalized["hard_stop"]; exists {
		hard = convertSafeguardMap(hardValue)
	}
	if softValue, exists := normalized["soft_skip"]; exists {
		soft = convertSafeguardMap(softValue)
	}

	if hard != nil || soft != nil {
		return trimEmptyMap(hard), trimEmptyMap(soft)
	}

	switch fallback {
	case safeguardDefaultSoftSkip:
		return nil, cloneSafeguardMap(raw)
	default:
		return cloneSafeguardMap(raw), nil
	}
}

func convertSafeguardMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneSafeguardMap(typed)
	case map[interface{}]interface{}:
		cloned := make(map[string]any, len(typed))
		for key, val := range typed {
			cloned[fmt.Sprint(key)] = val
		}
		return cloned
	default:
		return nil
	}
}

func cloneSafeguardMap(raw map[string]any) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(raw))
	for key, value := range raw {
		cloned[key] = value
	}
	return cloned
}

func trimEmptyMap(raw map[string]any) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

func readRequireCleanDirective(raw map[string]any) (bool, []worktree.IgnorePattern, bool, error) {
	if len(raw) == 0 {
		return false, nil, false, nil
	}
	value, exists := raw["require_clean"]
	if !exists {
		return false, nil, false, nil
	}
	switch typed := value.(type) {
	case bool:
		return typed, worktree.ParseIgnorePatterns(raw["ignore_dirty_paths"]), true, nil
	case map[string]any:
		return parseRequireCleanMap(typed)
	case map[interface{}]interface{}:
		return parseRequireCleanMap(convertSafeguardMap(typed))
	default:
		reader := newOptionReader(raw)
		enabled, _, enabledErr := reader.boolValue("require_clean")
		if enabledErr != nil {
			return false, nil, true, fmt.Errorf("require_clean must be a boolean or map")
		}
		return enabled, worktree.ParseIgnorePatterns(raw["ignore_dirty_paths"]), true, nil
	}
}

func parseRequireCleanMap(raw map[string]any) (bool, []worktree.IgnorePattern, bool, error) {
	if len(raw) == 0 {
		return true, nil, true, nil
	}
	reader := newOptionReader(raw)
	enabled, enabledExists, enabledErr := reader.boolValue("enabled")
	if enabledErr != nil {
		return false, nil, true, enabledErr
	}
	if !enabledExists {
		enabled = true
	}
	return enabled, worktree.ParseIgnorePatterns(raw["ignore_dirty_paths"]), true, nil
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
