package worktree

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tyemirov/gix/internal/repos/shared"
)

// ErrGitRepositoryManagerNotConfigured indicates a clean check was attempted without a repository manager.
var ErrGitRepositoryManagerNotConfigured = errors.New("git repository manager required")

// IgnorePattern describes a path pattern that should be excluded from worktree status checks.
type IgnorePattern struct {
	value   string
	isDir   bool
	hasGlob bool
}

// StatusCheckResult captures filtered worktree status entries.
type StatusCheckResult struct {
	Entries []string
}

// Clean reports whether the filtered status entries are empty.
func (result StatusCheckResult) Clean() bool {
	return len(result.Entries) == 0
}

// CheckStatus collects worktree status entries, filters them, and reports whether the repository is clean.
func CheckStatus(ctx context.Context, manager shared.GitRepositoryManager, repositoryPath string, patterns []IgnorePattern) (StatusCheckResult, error) {
	if manager == nil {
		return StatusCheckResult{}, ErrGitRepositoryManagerNotConfigured
	}

	statusEntries, statusError := manager.WorktreeStatus(ctx, strings.TrimSpace(repositoryPath))
	if statusError != nil {
		return StatusCheckResult{}, statusError
	}

	filtered := FilterStatusEntries(statusEntries, patterns)
	return StatusCheckResult{Entries: filtered}, nil
}

// FilterStatusEntries removes untracked/ignored entries and applies ignore patterns to the remaining entries.
func FilterStatusEntries(entries []string, patterns []IgnorePattern) []string {
	if len(entries) == 0 {
		return nil
	}

	remaining := make([]string, 0, len(entries))
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if len(trimmed) == 0 {
			continue
		}
		if statusEntryIsUntrackedOrIgnored(trimmed) {
			continue
		}

		if len(patterns) > 0 && pathMatchesIgnorePatterns(statusEntryPath(trimmed), patterns) {
			continue
		}

		remaining = append(remaining, trimmed)
	}
	return remaining
}

// SplitStatusEntries separates tracked/ignored status entries while preserving untracked/ignored entries for logging purposes.
func SplitStatusEntries(entries []string, patterns []IgnorePattern) (tracked []string, untracked []string) {
	if len(entries) == 0 {
		return nil, nil
	}

	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if len(trimmed) == 0 {
			continue
		}
		path := statusEntryPath(trimmed)
		if pathMatchesIgnorePatterns(path, patterns) {
			continue
		}

		if statusEntryIsUntrackedOrIgnored(trimmed) {
			untracked = append(untracked, trimmed)
			continue
		}

		tracked = append(tracked, trimmed)
	}

	return tracked, untracked
}

// BuildIgnorePatterns constructs ignore patterns from string entries.
func BuildIgnorePatterns(entries []string) []IgnorePattern {
	return parseIgnorePatternEntries(entries)
}

// ParseIgnorePatterns normalizes ignore pattern values.
func ParseIgnorePatterns(raw any) []IgnorePattern {
	switch typed := raw.(type) {
	case []string:
		return parseIgnorePatternEntries(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, entry := range typed {
			value, ok := entry.(string)
			if !ok {
				continue
			}
			values = append(values, value)
		}
		return parseIgnorePatternEntries(values)
	case string:
		return parseIgnorePatternEntries([]string{typed})
	default:
		return nil
	}
}

// DeduplicatePatterns removes duplicate ignore patterns while preserving order.
func DeduplicatePatterns(patterns []IgnorePattern) []IgnorePattern {
	if len(patterns) == 0 {
		return nil
	}

	unique := make([]IgnorePattern, 0, len(patterns))
	seen := make(map[string]struct{}, len(patterns))

	for _, pattern := range patterns {
		key := pattern.key()
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, pattern)
	}

	return unique
}

func parseIgnorePatternEntries(entries []string) []IgnorePattern {
	patterns := make([]IgnorePattern, 0, len(entries))
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
		patterns = append(patterns, IgnorePattern{
			value:   cleaned,
			isDir:   isDir,
			hasGlob: strings.ContainsAny(trimmed, "*?["),
		})
	}
	return patterns
}

func (pattern IgnorePattern) key() string {
	return fmt.Sprintf("%s|dir=%t|glob=%t", pattern.value, pattern.isDir, pattern.hasGlob)
}

func pathMatchesIgnorePatterns(path string, patterns []IgnorePattern) bool {
	normalized := strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(path)), "./")
	for _, pattern := range patterns {
		switch {
		case pattern.hasGlob:
			if matches, _ := filepath.Match(pattern.value, normalized); matches {
				return true
			}
		case pattern.isDir:
			if normalized == pattern.value || strings.HasPrefix(normalized, pattern.value+"/") {
				return true
			}
		default:
			if strings.HasPrefix(normalized, pattern.value) {
				return true
			}
		}
	}

	return false
}

func statusEntryPath(entry string) string {
	trimmed := strings.TrimSpace(entry)
	if len(trimmed) <= len(gitStatusUntrackedPrefix) {
		return ""
	}

	pathPart := strings.TrimSpace(trimmed[2:])
	if len(pathPart) == 0 {
		return ""
	}

	if strings.Contains(pathPart, " -> ") {
		sections := strings.Split(pathPart, " -> ")
		return strings.TrimSpace(sections[len(sections)-1])
	}

	return pathPart
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
