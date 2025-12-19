package workflow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	fileReplacePatternOptionKey    = "pattern"
	fileReplacePatternsOptionKey   = "patterns"
	fileReplaceFindOptionKey       = "find"
	fileReplaceReplaceOptionKey    = "replace"
	fileReplaceCommandOptionKey    = "command"
	fileReplaceSafeguardsOptionKey = "safeguards"

	fileReplacePlanMessageTemplate       = "REPLACE-PLAN: %s file=%s replacements=%d\n"
	fileReplaceApplyMessageTemplate      = "REPLACE-APPLY: %s file=%s replacements=%d\n"
	fileReplaceSkipMessageTemplate       = "REPLACE-SKIP: %s reason=%s\n"
	fileReplaceNoopMessageTemplate       = "REPLACE-NOOP: %s reason=%s\n"
	fileReplaceCommandPlanTemplate       = "REPLACE-COMMAND-PLAN: %s command=%s\n"
	fileReplaceCommandApplyTemplate      = "REPLACE-COMMAND: %s command=%s\n"
	fileReplaceCommandSupportMessage     = "replacement command execution requires shell support"
	fileReplaceMissingFindMessage        = "replacement action requires non-empty 'find'"
	fileReplaceMissingPatternMessage     = "replacement action requires at least one 'pattern'"
	fileReplaceMissingRepositoryError    = "replacement action requires repository manager and filesystem"
	replacementCommandDescriptorConstant = "replacement command"
	doubleStarPatternToken               = "**"
)

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

	hardSafeguards, softSafeguards := splitSafeguardSets(safeguardMap, safeguardDefaultSoftSkip)
	if len(hardSafeguards) > 0 {
		pass, reason, evaluationError := EvaluateSafeguards(ctx, environment, repository, hardSafeguards)
		if evaluationError != nil {
			return evaluationError
		}
		if !pass {
			writeReplacementMessage(environment, fileReplaceSkipMessageTemplate, repository.Path, reason)
			return repositorySkipError{reason: reason}
		}
	}
	if len(softSafeguards) > 0 {
		pass, reason, evaluationError := EvaluateSafeguards(ctx, environment, repository, softSafeguards)
		if evaluationError != nil {
			return evaluationError
		}
		if !pass {
			writeReplacementMessage(environment, fileReplaceSkipMessageTemplate, repository.Path, reason)
			return nil
		}
	}

	matchingFiles, matchError := collectReplacementTargets(repository.Path, patterns)
	if matchError != nil {
		return matchError
	}

	plans, planningError := buildReplacementPlans(environment.FileSystem, repository.Path, matchingFiles, findValue, replaceValue)
	if planningError != nil {
		return planningError
	}

	if len(plans) == 0 {
		writeReplacementMessage(environment, fileReplaceNoopMessageTemplate, repository.Path, "no matches")
		return nil
	}
	describeReplacementPlan(environment, repository.Path, plans)

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
	return parseShellCommandArguments(raw, replacementCommandDescriptorConstant)
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

		normalizedPath := filepath.ToSlash(relativePath)
		for _, pattern := range patterns {
			matched, matchError := matchReplacementPattern(pattern, normalizedPath)
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

func matchReplacementPattern(pattern string, value string) (bool, error) {
	normalizedPattern := filepath.ToSlash(pattern)
	normalizedValue := filepath.ToSlash(value)

	if strings.Contains(normalizedPattern, doubleStarPatternToken) {
		return matchDoubleStarPattern(normalizedPattern, normalizedValue)
	}

	return pathpkg.Match(normalizedPattern, normalizedValue)
}

func matchDoubleStarPattern(pattern string, value string) (bool, error) {
	patternSegments := splitPatternSegments(pattern)
	valueSegments := splitPatternSegments(value)
	memo := make(map[globMatchMemoKey]bool)
	return matchDoubleStarSegments(patternSegments, valueSegments, 0, 0, memo)
}

func splitPatternSegments(value string) []string {
	trimmed := strings.Trim(value, "/")
	if len(trimmed) == 0 {
		return nil
	}
	rawSegments := strings.Split(trimmed, "/")
	segments := make([]string, 0, len(rawSegments))
	for _, segment := range rawSegments {
		if len(segment) == 0 {
			continue
		}
		segments = append(segments, segment)
	}
	return segments
}

type globMatchMemoKey struct {
	patternIndex int
	valueIndex   int
}

func matchDoubleStarSegments(patternSegments []string, valueSegments []string, patternIndex int, valueIndex int, memo map[globMatchMemoKey]bool) (bool, error) {
	key := globMatchMemoKey{patternIndex: patternIndex, valueIndex: valueIndex}
	if cached, exists := memo[key]; exists {
		return cached, nil
	}

	if patternIndex >= len(patternSegments) {
		result := valueIndex >= len(valueSegments)
		memo[key] = result
		return result, nil
	}

	segment := patternSegments[patternIndex]
	if segment == doubleStarPatternToken {
		nextIndex := patternIndex + 1
		for nextIndex < len(patternSegments) && patternSegments[nextIndex] == doubleStarPatternToken {
			nextIndex++
		}
		if nextIndex >= len(patternSegments) {
			memo[key] = true
			return true, nil
		}
		for candidate := valueIndex; candidate <= len(valueSegments); candidate++ {
			match, matchError := matchDoubleStarSegments(patternSegments, valueSegments, nextIndex, candidate, memo)
			if matchError != nil {
				return false, matchError
			}
			if match {
				memo[key] = true
				return true, nil
			}
		}
		memo[key] = false
		return false, nil
	}

	if valueIndex >= len(valueSegments) {
		memo[key] = false
		return false, nil
	}

	matched, matchError := pathpkg.Match(segment, valueSegments[valueIndex])
	if matchError != nil {
		return false, matchError
	}
	if !matched {
		memo[key] = false
		return false, nil
	}

	result, resultError := matchDoubleStarSegments(patternSegments, valueSegments, patternIndex+1, valueIndex+1, memo)
	if resultError != nil {
		return false, resultError
	}
	memo[key] = result
	return result, nil
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
