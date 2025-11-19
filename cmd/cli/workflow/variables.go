package workflow

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	workflowpkg "github.com/tyemirov/gix/internal/workflow"
)

func parseVariableAssignments(assignments []string) (map[string]string, error) {
	if len(assignments) == 0 {
		return nil, nil
	}

	result := make(map[string]string, len(assignments))
	for _, assignment := range assignments {
		trimmed := strings.TrimSpace(assignment)
		if len(trimmed) == 0 {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("workflow variables must be in key=value format: %s", assignment)
		}
		key := strings.TrimSpace(parts[0])
		value := parts[1]
		if len(key) == 0 {
			return nil, fmt.Errorf("workflow variable key cannot be empty (%s)", assignment)
		}
		normalized, normalizeError := normalizeVariableName(key)
		if normalizeError != nil {
			return nil, normalizeError
		}
		result[normalized] = value
	}

	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func loadVariablesFromFiles(paths []string) (map[string]string, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	combined := make(map[string]string)
	for _, rawPath := range paths {
		trimmed := strings.TrimSpace(rawPath)
		if len(trimmed) == 0 {
			continue
		}
		fileVars, fileError := loadVariablesFromFile(trimmed)
		if fileError != nil {
			return nil, fileError
		}
		for key, value := range fileVars {
			combined[key] = value
		}
	}

	if len(combined) == 0 {
		return nil, nil
	}
	return combined, nil
}

func loadVariablesFromFile(path string) (map[string]string, error) {
	content, readError := os.ReadFile(path)
	if readError != nil {
		return nil, fmt.Errorf("failed to read variable file %q: %w", path, readError)
	}

	var parsed map[string]any
	if unmarshalError := yaml.Unmarshal(content, &parsed); unmarshalError != nil {
		return nil, fmt.Errorf("failed to parse variable file %q: %w", path, unmarshalError)
	}

	result := make(map[string]string, len(parsed))
	for rawKey, value := range parsed {
		normalizedKey, normalizeError := normalizeVariableName(rawKey)
		if normalizeError != nil {
			return nil, fmt.Errorf("invalid variable name %q in %s: %w", rawKey, path, normalizeError)
		}
		result[normalizedKey] = fmt.Sprint(value)
	}

	return result, nil
}

func normalizeVariableName(raw string) (string, error) {
	name, err := workflowpkg.NewVariableName(raw)
	if err != nil {
		return "", err
	}
	return string(name), nil
}
