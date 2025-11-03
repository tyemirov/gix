package shared

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/temirov/gix/internal/execshell"
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
			return ignored, nil
		}
		return nil, err
	}

	for _, line := range strings.Split(result.StandardOutput, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		if original, exists := indexMap[trimmed]; exists {
			ignored[original] = struct{}{}
			continue
		}
		normalizedKey := strings.ReplaceAll(trimmed, "\\", "/")
		if original, exists := indexMap[normalizedKey]; exists {
			ignored[original] = struct{}{}
			continue
		}
		restoredKey := strings.ReplaceAll(trimmed, "/", string(filepath.Separator))
		if original, exists := indexMap[restoredKey]; exists {
			ignored[original] = struct{}{}
		}
	}

	return ignored, nil
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
