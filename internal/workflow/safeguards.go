package workflow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	pathpkg "path"
	"path/filepath"
	"strings"
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

	requireClean, requireCleanExists, requireCleanError := reader.boolValue("require_clean")
	if requireCleanError != nil {
		return false, "", requireCleanError
	}
	if requireCleanExists && requireClean {
		if environment.RepositoryManager == nil {
			return false, "", errSafeguardRepoManager
		}
		statusEntries, statusError := environment.RepositoryManager.WorktreeStatus(ctx, repositoryPath)
		if statusError != nil {
			return false, "", statusError
		}
		ignoredPatterns := parseDirtyIgnorePatterns(raw["ignore_dirty_paths"])
		remainingEntries := filterIgnoredStatusEntries(statusEntries, ignoredPatterns)
		if len(remainingEntries) > 0 {
			displayEntries := remainingEntries
			if len(displayEntries) > maxStatusReasonEntries {
				displayEntries = append([]string(nil), remainingEntries[:maxStatusReasonEntries]...)
			} else {
				displayEntries = append([]string(nil), remainingEntries...)
			}
			for index := range displayEntries {
				displayEntries[index] = strings.TrimSpace(displayEntries[index])
			}
			reason := fmt.Sprintf("repository not clean: %s", strings.Join(displayEntries, ", "))
			if len(remainingEntries) > maxStatusReasonEntries {
				reason = fmt.Sprintf("%s (+%d more)", reason, len(remainingEntries)-maxStatusReasonEntries)
			}
			return false, reason, nil
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

type dirtyIgnorePattern struct {
	value   string
	isDir   bool
	hasGlob bool
}

func parseDirtyIgnorePatterns(raw any) []dirtyIgnorePattern {
	switch typed := raw.(type) {
	case []string:
		return buildDirtyIgnorePatterns(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, entry := range typed {
			value, ok := entry.(string)
			if !ok {
				continue
			}
			values = append(values, value)
		}
		return buildDirtyIgnorePatterns(values)
	case string:
		return buildDirtyIgnorePatterns([]string{typed})
	default:
		return nil
	}
}

func buildDirtyIgnorePatterns(entries []string) []dirtyIgnorePattern {
	patterns := make([]dirtyIgnorePattern, 0, len(entries))
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if len(trimmed) == 0 {
			continue
		}
		isDir := strings.HasSuffix(trimmed, "/")
		normalized := strings.TrimSuffix(trimmed, "/")
		normalized = strings.TrimPrefix(normalized, "./")
		if len(normalized) == 0 {
			continue
		}
		cleaned := filepath.ToSlash(filepath.Clean(normalized))
		if cleaned == "." {
			continue
		}
		patterns = append(patterns, dirtyIgnorePattern{
			value:   cleaned,
			isDir:   isDir,
			hasGlob: strings.ContainsAny(trimmed, "*?["),
		})
	}
	return patterns
}

func (pattern dirtyIgnorePattern) matches(path string) bool {
	if len(pattern.value) == 0 {
		return false
	}
	normalized := filepath.ToSlash(strings.TrimSpace(path))
	if len(normalized) == 0 {
		return false
	}
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimPrefix(normalized, "/")

	if pattern.hasGlob {
		matched, err := pathpkg.Match(pattern.value, normalized)
		return err == nil && matched
	}

	if pattern.isDir {
		return normalized == pattern.value || strings.HasPrefix(normalized, pattern.value+"/")
	}

	return normalized == pattern.value
}

func filterIgnoredStatusEntries(entries []string, patterns []dirtyIgnorePattern) []string {
	if len(entries) == 0 || len(patterns) == 0 {
		return entries
	}
	remaining := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !(statusEntryIsUntrackedOrIgnored(entry) && statusEntryMatchesIgnore(entry, patterns)) {
			remaining = append(remaining, entry)
		}
	}
	return remaining
}

func statusEntryMatchesIgnore(entry string, patterns []dirtyIgnorePattern) bool {
	path := extractStatusPath(entry)
	if len(path) == 0 {
		return false
	}
	for _, pattern := range patterns {
		if pattern.matches(path) {
			return true
		}
	}
	return false
}

func extractStatusPath(entry string) string {
	trimmed := strings.TrimSpace(entry)
	if len(trimmed) == 0 {
		return ""
	}
	spaceIndex := strings.Index(trimmed, " ")
	if spaceIndex == -1 {
		return trimmed
	}
	return strings.TrimSpace(trimmed[spaceIndex+1:])
}

const (
	gitStatusUntrackedPrefix = "??"
	gitStatusIgnoredPrefix   = "!!"
)

func statusEntryIsUntrackedOrIgnored(entry string) bool {
	trimmed := strings.TrimSpace(entry)
	if len(trimmed) < len(gitStatusUntrackedPrefix) {
		return false
	}
	return strings.HasPrefix(trimmed, gitStatusUntrackedPrefix) || strings.HasPrefix(trimmed, gitStatusIgnoredPrefix)
}
