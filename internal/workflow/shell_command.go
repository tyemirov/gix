package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
)

const (
	shellCommandEntriesMessageTemplateConstant = "%s entries must be strings"
	shellCommandTypeMessageTemplateConstant    = "%s must be a string or list of strings"
	shellCommandDefaultDescriptorConstant      = "command"
)

type shellCommandExecutor interface {
	Execute(context.Context, execshell.ShellCommand) (execshell.ExecutionResult, error)
}

func parseShellCommandArguments(raw any, descriptor string) ([]string, error) {
	if raw == nil {
		return nil, nil
	}

	normalizedDescriptor := strings.TrimSpace(descriptor)
	if normalizedDescriptor == "" {
		normalizedDescriptor = shellCommandDefaultDescriptorConstant
	}

	switch typed := raw.(type) {
	case []string:
		return sanitizeCommandArguments(typed), nil
	case []any:
		values := make([]string, 0, len(typed))
		for entryIndex := range typed {
			entry := typed[entryIndex]
			value, ok := entry.(string)
			if !ok {
				return nil, fmt.Errorf(shellCommandEntriesMessageTemplateConstant, normalizedDescriptor)
			}
			values = append(values, value)
		}
		return sanitizeCommandArguments(values), nil
	case string:
		return sanitizeCommandArguments(strings.Fields(typed)), nil
	default:
		return nil, fmt.Errorf(shellCommandTypeMessageTemplateConstant, normalizedDescriptor)
	}
}

func sanitizeCommandArguments(arguments []string) []string {
	sanitized := make([]string, 0, len(arguments))
	for argumentIndex := range arguments {
		argument := strings.TrimSpace(arguments[argumentIndex])
		if argument == "" {
			continue
		}
		sanitized = append(sanitized, argument)
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}
