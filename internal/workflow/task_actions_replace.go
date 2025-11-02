package workflow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/repos/shared"
)

const (
	fileReplacePatternOptionKey    = "pattern"
	fileReplacePatternsOptionKey   = "patterns"
	fileReplaceFindOptionKey       = "find"
	fileReplaceReplaceOptionKey    = "replace"
	fileReplaceCommandOptionKey    = "command"
	fileReplaceSafeguardsOptionKey = "safeguards"

	fileReplacePlanMessageTemplate    = "REPLACE-PLAN: %s file=%s replacements=%d\n"
	fileReplaceApplyMessageTemplate   = "REPLACE-APPLY: %s file=%s replacements=%d\n"
	fileReplaceSkipMessageTemplate    = "REPLACE-SKIP: %s reason=%s\n"
	fileReplaceNoopMessageTemplate    = "REPLACE-NOOP: %s reason=%s\n"
	fileReplaceCommandPlanTemplate    = "REPLACE-COMMAND-PLAN: %s command=%s\n"
	fileReplaceCommandApplyTemplate   = "REPLACE-COMMAND: %s command=%s\n"
	fileReplaceCommandSupportMessage  = "replacement command execution requires shell support"
	fileReplaceMissingFindMessage     = "replacement action requires non-empty 'find'"
	fileReplaceMissingPatternMessage  = "replacement action requires at least one 'pattern'"
	fileReplaceMissingRepositoryError = "replacement action requires repository manager and filesystem"
)

type shellCommandExecutor interface {
	Execute(context.Context, execshell.ShellCommand) (execshell.ExecutionResult, error)
}

func handleFileReplaceAction(ctx context.Context, environment *Environment, repository *RepositoryState, parameters map[string]any) error {
	if environment == nil || repository == nil {
		return nil
	}

	if environment.RepositoryManager == nil || environment.FileSystem == nil {
		return errors.New(fileReplaceMissingRepositoryError)
	}

	reader := newOptionReader(parameters)

	findValue, findExists, findError := reader.stringValue(fileReplaceFindOptionKey)
	if findError != nil {
		return findError
	}
	if !findExists || len(findValue) == 0 {
		return errors.New(fileReplaceMissingFindMessage)
	}

	replaceValue, _, replaceError := reader.stringValue(fileReplaceReplaceOptionKey)
	if replaceError != nil {
		return replaceError
	}

	patterns, patternsError := readReplacementPatterns(reader)
	if patternsError != nil {
		return patternsError
	}
	if len(patterns) == 0 {
		return errors.New(fileReplaceMissingPatternMessage)
	}

	commandArguments, commandError := parseReplacementCommand(parameters[fileReplaceCommandOptionKey])
	if commandError != nil {
		return commandError
	}

	safeguardMap, _, safeguardsError := reader.mapValue(fileReplaceSafeguardsOptionKey)
	if safeguardsError != nil {
		return safeguardsError
	}

	pass, reason, evaluationError := EvaluateSafeguards(ctx, environment, repository, safeguardMap)
	if evaluationError != nil {
		return evaluationError
	}
	if !pass {
		writeReplacementMessage(environment, fileReplaceSkipMessageTemplate, repository.Path, reason)
		return nil
	}

	matchingFiles, matchError := collectReplacementTargets(repository.Path, patterns)
	if matchError != nil {
		return matchError
	}

	plans, planningError := buildReplacementPlans(environment.FileSystem, repository.Path, matchingFiles, findValue, replaceValue)
	if planningError != nil {
		return planningError
	}

	if environment.DryRun {
		describeReplacementPlan(environment, repository.Path, plans)
		if len(commandArguments) > 0 && len(plans) > 0 {
			writeReplacementMessage(environment, fileReplaceCommandPlanTemplate, repository.Path, strings.Join(commandArguments, " "))
		}
		return nil
	}

	if len(plans) == 0 {
		writeReplacementMessage(environment, fileReplaceNoopMessageTemplate, repository.Path, "no matches")
		return nil
	}

	if applyError := applyReplacementPlans(environment.FileSystem, plans); applyError != nil {
		return applyError
	}

	describeReplacementOutcome(environment, repository.Path, plans)

	if len(commandArguments) == 0 {
		return nil
	}

	executor, ok := environment.GitExecutor.(shellCommandExecutor)
	if !ok {
		return errors.New(fileReplaceCommandSupportMessage)
	}

	command := execshell.ShellCommand{
		Name: execshell.CommandName(commandArguments[0]),
		Details: execshell.CommandDetails{
			Arguments:        commandArguments[1:],
			WorkingDirectory: repository.Path,
		},
	}

	if _, executionError := executor.Execute(ctx, command); executionError != nil {
		return executionError
	}

	writeReplacementMessage(environment, fileReplaceCommandApplyTemplate, repository.Path, strings.Join(commandArguments, " "))
	return nil
}

type replacementPlan struct {
	absolutePath string
	relativePath string
	replacements int
	content      []byte
}

func readReplacementPatterns(reader optionReader) ([]string, error) {
	collected := make([]string, 0, 4)

	if value, exists, err := reader.stringValue(fileReplacePatternOptionKey); err != nil {
		return nil, err
	} else if exists && len(value) > 0 {
		collected = append(collected, value)
	}

	rawPatterns, rawExists := reader.entries[fileReplacePatternsOptionKey]
	if rawExists {
		switch typed := rawPatterns.(type) {
		case []string:
			collected = append(collected, typed...)
		case []any:
			for _, entry := range typed {
				switch patternValue := entry.(type) {
				case string:
					collected = append(collected, patternValue)
				case map[string]any:
					entryReader := newOptionReader(patternValue)
					value, exists, err := entryReader.stringValue(fileReplacePatternOptionKey)
					if err != nil {
						return nil, err
					}
					if exists && len(value) > 0 {
						collected = append(collected, value)
					}
				default:
					return nil, fmt.Errorf("replacement patterns must be strings")
				}
			}
		case string:
			collected = append(collected, typed)
		default:
			return nil, fmt.Errorf("replacement patterns must be strings")
		}
	}

	unique := make([]string, 0, len(collected))
	seen := map[string]struct{}{}
	for _, pattern := range collected {
		trimmed := strings.TrimSpace(pattern)
		if len(trimmed) == 0 {
			continue
		}
		normalized := strings.TrimPrefix(trimmed, "./")
		if len(normalized) == 0 {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		unique = append(unique, normalized)
	}

	return unique, nil
}

func parseReplacementCommand(raw any) ([]string, error) {
	if raw == nil {
		return nil, nil
	}

	switch typed := raw.(type) {
	case []string:
		return sanitizeCommandArguments(typed), nil
	case []any:
		values := make([]string, 0, len(typed))
		for _, entry := range typed {
			value, ok := entry.(string)
			if !ok {
				return nil, fmt.Errorf("replacement command entries must be strings")
			}
			values = append(values, value)
		}
		return sanitizeCommandArguments(values), nil
	case string:
		return sanitizeCommandArguments(strings.Fields(typed)), nil
	default:
		return nil, fmt.Errorf("replacement command must be a string or list of strings")
	}
}

func sanitizeCommandArguments(arguments []string) []string {
	sanitized := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		trimmed := strings.TrimSpace(argument)
		if len(trimmed) == 0 {
			continue
		}
		sanitized = append(sanitized, trimmed)
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}

func collectReplacementTargets(repositoryPath string, patterns []string) ([]string, error) {
	matches := []string{}
	seen := map[string]struct{}{}

	walkError := filepath.WalkDir(repositoryPath, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}

		relativePath, relError := filepath.Rel(repositoryPath, path)
		if relError != nil {
			return relError
		}

		for _, pattern := range patterns {
			matched, matchError := filepath.Match(pattern, relativePath)
			if matchError != nil {
				return matchError
			}
			if matched {
				if _, exists := seen[path]; !exists {
					seen[path] = struct{}{}
					matches = append(matches, path)
				}
				break
			}
		}
		return nil
	})
	if walkError != nil {
		return nil, walkError
	}

	sort.Strings(matches)
	return matches, nil
}

func buildReplacementPlans(fileSystem shared.FileSystem, repositoryPath string, files []string, find string, replace string) ([]replacementPlan, error) {
	plans := make([]replacementPlan, 0, len(files))

	for _, absolutePath := range files {
		content, readError := fileSystem.ReadFile(absolutePath)
		if readError != nil {
			return nil, readError
		}

		original := string(content)
		occurrences := strings.Count(original, find)
		if occurrences == 0 {
			continue
		}

		updated := strings.ReplaceAll(original, find, replace)
		relativePath, relError := filepath.Rel(repositoryPath, absolutePath)
		if relError != nil {
			return nil, relError
		}

		plans = append(plans, replacementPlan{
			absolutePath: absolutePath,
			relativePath: relativePath,
			replacements: occurrences,
			content:      []byte(updated),
		})
	}

	sort.Slice(plans, func(left, right int) bool {
		return plans[left].relativePath < plans[right].relativePath
	})

	return plans, nil
}

func applyReplacementPlans(fileSystem shared.FileSystem, plans []replacementPlan) error {
	for _, plan := range plans {
		info, statError := fileSystem.Stat(plan.absolutePath)
		if statError != nil && !errors.Is(statError, fs.ErrNotExist) {
			return statError
		}

		mode := fs.FileMode(0o644)
		if info != nil {
			mode = info.Mode()
		}

		if writeError := fileSystem.WriteFile(plan.absolutePath, plan.content, mode); writeError != nil {
			return writeError
		}
	}
	return nil
}

func describeReplacementPlan(environment *Environment, repositoryPath string, plans []replacementPlan) {
	if len(plans) == 0 {
		writeReplacementMessage(environment, fileReplaceNoopMessageTemplate, repositoryPath, "no matches")
		return
	}

	for _, plan := range plans {
		writeReplacementMessage(environment, fileReplacePlanMessageTemplate, repositoryPath, plan.relativePath, plan.replacements)
	}
}

func describeReplacementOutcome(environment *Environment, repositoryPath string, plans []replacementPlan) {
	if len(plans) == 0 {
		writeReplacementMessage(environment, fileReplaceNoopMessageTemplate, repositoryPath, "no matches")
		return
	}

	for _, plan := range plans {
		writeReplacementMessage(environment, fileReplaceApplyMessageTemplate, repositoryPath, plan.relativePath, plan.replacements)
	}
}

func writeReplacementMessage(environment *Environment, template string, repositoryPath string, values ...any) {
	if environment == nil || environment.Output == nil {
		return
	}

	args := make([]any, 0, len(values)+1)
	args = append(args, repositoryPath)
	args = append(args, values...)
	fmt.Fprintf(environment.Output, template, args...)
}
