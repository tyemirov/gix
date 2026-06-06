package shared

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
)

// CheckIgnoredPaths returns the subset of relative paths ignored by git when evaluated from worktreeRoot.
func CheckIgnoredPaths(ctx context.Context, executor GitExecutor, worktreeRoot string, relativePaths []string) (map[string]struct{}, error) {
	ignored := make(map[string]struct{})
	if executor == nil || len(relativePaths) == 0 {
		return ignored, nil
	}

	normalized := make([]string, 0, len(relativePaths))
	indexMap := make(map[string]string, len(relativePaths))
	for _, relativePath := range relativePaths {
		slashed := filepath.ToSlash(relativePath)
		normalized = append(normalized, slashed)
		indexMap[slashed] = relativePath
	}

	command := execshell.CommandDetails{
		Arguments:        []string{"check-ignore", "--stdin"},
		WorkingDirectory: worktreeRoot,
		StandardInput:    []byte(strings.Join(normalized, "\n")),
	}

	result, err := executor.ExecuteGit(ctx, command)
	if err != nil {
		var failed execshell.CommandFailedError
		if errors.As(err, &failed) && failed.Result.ExitCode == 1 {
			result = execshell.ExecutionResult{}
		} else if isNotGitRepositoryError(err) {
			return ignored, nil
		} else {
			return nil, err
		}
	}

	markIgnoredPathOutput(ignored, indexMap, result.StandardOutput, true)

	cachedIgnoredArguments := []string{"ls-files", "--cached", "--ignored", "--exclude-standard", "--"}
	cachedIgnoredArguments = append(cachedIgnoredArguments, normalized...)
	cachedIgnoredCommand := execshell.CommandDetails{
		Arguments:        cachedIgnoredArguments,
		WorkingDirectory: worktreeRoot,
	}
	cachedIgnoredResult, cachedIgnoredErr := executor.ExecuteGit(ctx, cachedIgnoredCommand)
	if cachedIgnoredErr != nil {
		if isNotGitRepositoryError(cachedIgnoredErr) {
			return ignored, nil
		}
		return nil, cachedIgnoredErr
	}
	markIgnoredPathOutput(ignored, indexMap, cachedIgnoredResult.StandardOutput, false)

	return ignored, nil
}

func markIgnoredPathOutput(ignored map[string]struct{}, indexMap map[string]string, output string, includeAncestorMatches bool) {
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		markIgnoredPath(ignored, indexMap, trimmed, includeAncestorMatches)
	}
}

func markIgnoredPath(ignored map[string]struct{}, indexMap map[string]string, gitPath string, includeAncestorMatches bool) {
	normalizedKey := normalizeIgnoredGitPath(gitPath)
	for candidate, original := range indexMap {
		if ignoredPathMatchesCandidate(normalizedKey, candidate, includeAncestorMatches) {
			ignored[original] = struct{}{}
		}
	}
}

func ignoredPathMatchesCandidate(ignoredPath string, candidatePath string, includeAncestorMatches bool) bool {
	normalizedCandidate := normalizeIgnoredGitPath(candidatePath)
	if ignoredPath == normalizedCandidate {
		return true
	}
	return includeAncestorMatches && strings.HasPrefix(normalizedCandidate, ignoredPath+"/")
}

func normalizeIgnoredGitPath(gitPath string) string {
	return strings.Trim(strings.ReplaceAll(strings.TrimSpace(gitPath), "\\", "/"), "/")
}

// FilterIgnoredRepositories removes repositories ignored by ancestor worktrees according to gitignore rules.
func FilterIgnoredRepositories(ctx context.Context, executor GitExecutor, repositoryPaths []string) ([]string, error) {
	if executor == nil || len(repositoryPaths) == 0 {
		return repositoryPaths, nil
	}

	normalizedPaths := make([]string, len(repositoryPaths))
	repositorySet := make(map[string]struct{}, len(repositoryPaths))
	for index := range repositoryPaths {
		cleaned := filepath.Clean(repositoryPaths[index])
		normalizedPaths[index] = cleaned
		repositorySet[cleaned] = struct{}{}
	}

	parentToChildren := make(map[string][]string)
	for _, repositoryPath := range normalizedPaths {
		parent := closestAncestor(repositoryPath, repositorySet)
		if len(parent) == 0 {
			continue
		}

		relativePath, relErr := filepath.Rel(parent, repositoryPath)
		if relErr != nil || relativePath == "." {
			continue
		}

		parentToChildren[parent] = append(parentToChildren[parent], relativePath)
	}

	if len(parentToChildren) == 0 {
		return repositoryPaths, nil
	}

	ignored := make(map[string]struct{})
	for parent, children := range parentToChildren {
		if len(children) == 0 {
			continue
		}

		ignoredRelatives, ignoreErr := CheckIgnoredPaths(ctx, executor, parent, children)
		if ignoreErr != nil {
			return nil, ignoreErr
		}

		for relative := range ignoredRelatives {
			ignored[filepath.Clean(filepath.Join(parent, relative))] = struct{}{}
		}
	}

	if len(ignored) == 0 {
		return repositoryPaths, nil
	}

	filtered := make([]string, 0, len(repositoryPaths))
	for index := range repositoryPaths {
		if _, skip := ignored[normalizedPaths[index]]; skip {
			continue
		}
		filtered = append(filtered, repositoryPaths[index])
	}

	return filtered, nil
}

func closestAncestor(repositoryPath string, repositorySet map[string]struct{}) string {
	current := filepath.Dir(repositoryPath)
	for current != repositoryPath {
		if _, exists := repositorySet[current]; exists {
			return current
		}
		next := filepath.Dir(current)
		if next == current {
			break
		}
		current = next
	}
	return ""
}

func isNotGitRepositoryError(err error) bool {
	var failed execshell.CommandFailedError
	if errors.As(err, &failed) {
		normalized := normalizeGitErrorOutput(strings.TrimSpace(failed.Result.StandardError), failed.Error())
		return strings.Contains(normalized, "not a git repository")
	}
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "not a git repository")
}

func normalizeGitErrorOutput(primary string, fallback string) string {
	if trimmed := strings.ToLower(strings.TrimSpace(primary)); len(trimmed) > 0 {
		return trimmed
	}
	return strings.ToLower(strings.TrimSpace(fallback))
}
