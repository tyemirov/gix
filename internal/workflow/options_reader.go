package workflow

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	optionFromKeyConstant               = "from"
	optionToKeyConstant                 = "to"
	optionRequireCleanKeyConstant       = "require_clean"
	optionIncludeOwnerKeyConstant       = "include_owner"
	optionOwnerKeyConstant              = "owner"
	optionPathsKeyConstant              = "paths"
	optionTargetsKeyConstant            = "targets"
	optionRemoteNameKeyConstant         = "remote_name"
	optionSourceBranchKeyConstant       = "source_branch"
	optionTargetBranchKeyConstant       = "target_branch"
	optionPushToRemoteKeyConstant       = "push_to_remote"
	optionDeleteSourceBranchKeyConstant = "delete_source_branch"
	optionOutputPathKeyConstant         = "output"
)

type optionReader struct {
	entries map[string]any
}

func newOptionReader(raw map[string]any) optionReader {
	normalized := make(map[string]any, len(raw))
	for key, value := range raw {
		normalized[strings.ToLower(strings.TrimSpace(key))] = value
	}
	return optionReader{entries: normalized}
}

func (reader optionReader) stringValue(key string) (string, bool, error) {
	value, exists := reader.entries[key]
	if !exists {
		return "", false, nil
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed), true, nil
	case fmt.Stringer:
		return strings.TrimSpace(typed.String()), true, nil
	default:
		return "", true, fmt.Errorf("option %s must be a string", key)
	}
}

func (reader optionReader) boolValue(key string) (bool, bool, error) {
	value, exists := reader.entries[key]
	if !exists {
		return false, false, nil
	}
	switch typed := value.(type) {
	case bool:
		return typed, true, nil
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(typed))
		if trimmed == "true" {
			return true, true, nil
		}
		if trimmed == "false" {
			return false, true, nil
		}
	default:
		return false, true, fmt.Errorf("option %s must be a boolean", key)
	}
	return false, true, fmt.Errorf("option %s must be a boolean", key)
}

func (reader optionReader) mapSlice(key string) ([]map[string]any, bool, error) {
	value, exists := reader.entries[key]
	if !exists {
		return nil, false, nil
	}
	listValue, ok := value.([]any)
	if !ok {
		return nil, true, fmt.Errorf("option %s must be a list", key)
	}
	maps := make([]map[string]any, 0, len(listValue))
	for index := range listValue {
		entry, ok := listValue[index].(map[string]any)
		if !ok {
			return nil, true, fmt.Errorf("option %s entries must be maps", key)
		}
		maps = append(maps, entry)
	}
	return maps, true, nil
}

func (reader optionReader) mapValue(key string) (map[string]any, bool, error) {
	value, exists := reader.entries[key]
	if !exists {
		return nil, false, nil
	}
	typed, ok := value.(map[string]any)
	if !ok {
		return nil, true, fmt.Errorf("option %s must be a map", key)
	}
	return typed, true, nil
}

func (reader optionReader) stringSlice(key string) ([]string, bool, error) {
	value, exists := reader.entries[key]
	if !exists {
		return nil, false, nil
	}
	listValue, ok := value.([]any)
	if ok {
		values := make([]string, 0, len(listValue))
		for _, entry := range listValue {
			switch typed := entry.(type) {
			case string:
				values = append(values, strings.TrimSpace(typed))
			default:
				return nil, true, fmt.Errorf("option %s entries must be strings", key)
			}
		}
		return values, true, nil
	}

	stringList, ok := value.([]string)
	if ok {
		values := make([]string, len(stringList))
		for index := range stringList {
			values[index] = strings.TrimSpace(stringList[index])
		}
		return values, true, nil
	}

	return nil, true, fmt.Errorf("option %s must be a list of strings", key)
}

func (reader optionReader) intValue(key string) (int, bool, error) {
	value, exists := reader.entries[key]
	if !exists {
		return 0, false, nil
	}

	switch typed := value.(type) {
	case int:
		return typed, true, nil
	case int64:
		return int(typed), true, nil
	case float64:
		return int(typed), true, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if len(trimmed) == 0 {
			return 0, true, fmt.Errorf("option %s must be an integer", key)
		}
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return 0, true, fmt.Errorf("option %s must be an integer", key)
		}
		return parsed, true, nil
	default:
		return 0, true, fmt.Errorf("option %s must be an integer", key)
	}
}
