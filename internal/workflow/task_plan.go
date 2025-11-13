package workflow

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/temirov/gix/internal/repos/shared"
)

type taskPlan struct {
	task          TaskDefinition
	repository    *RepositoryState
	branchName    string
	startPoint    string
	commitMessage string
	pullRequest   *taskPlanPullRequest
	fileChanges   []taskFileChange
	actions       []taskAction
	skipReason    string
	skipped       bool
	variables     map[string]string
}

type taskPlanPullRequest struct {
	title string
	body  string
	base  string
	draft bool
}

type taskFileChange struct {
	relativePath string
	absolutePath string
	content      []byte
	mode         taskFileExistsMode
	permissions  fs.FileMode
	skipReason   string
	apply        bool
}

type taskAction struct {
	actionType string
	parameters map[string]any
}

type taskPlanner struct {
	task         TaskDefinition
	templateData TaskTemplateData
}

func newTaskPlanner(task TaskDefinition, data TaskTemplateData) taskPlanner {
	return taskPlanner{task: task, templateData: data}
}

func (planner taskPlanner) BuildPlan(environment *Environment, repository *RepositoryState) (taskPlan, error) {
	plan := taskPlan{task: planner.task, repository: repository}

	branchName, branchError := planner.renderTemplate(planner.task.Branch.NameTemplate, planner.defaultBranchName())
	if branchError != nil {
		return taskPlan{}, branchError
	}
	plan.branchName = sanitizeBranchName(branchName)

	startPointTemplate := planner.task.Branch.StartPointTemplate
	if len(strings.TrimSpace(startPointTemplate)) == 0 {
		startPointTemplate = "{{ .Repository.DefaultBranch }}"
	}

	startPoint, startPointError := planner.renderTemplate(startPointTemplate, planner.templateData.Repository.DefaultBranch)
	if startPointError != nil {
		return taskPlan{}, startPointError
	}
	plan.startPoint = strings.TrimSpace(startPoint)

	commitMessage, commitError := planner.renderTemplate(planner.task.Commit.MessageTemplate, "")
	if commitError != nil {
		return taskPlan{}, commitError
	}
	plan.commitMessage = strings.TrimSpace(commitMessage)
	if len(plan.commitMessage) == 0 {
		plan.commitMessage = fmt.Sprintf("Apply task %s", planner.task.Name)
	}

	fileChanges, fileError := planner.planFileChanges(environment, repository)
	if fileError != nil {
		return taskPlan{}, fileError
	}
	plan.fileChanges = fileChanges

	actions, actionsError := planner.planActions()
	if actionsError != nil {
		return taskPlan{}, actionsError
	}
	plan.actions = actions

	if planner.task.PullRequest != nil {
		pr, prError := planner.planPullRequest(*planner.task.PullRequest)
		if prError != nil {
			return taskPlan{}, prError
		}
		plan.pullRequest = pr
	}

	if !hasApplicableChanges(plan.fileChanges) && len(plan.actions) == 0 {
		plan.skipped = true
		plan.skipReason = "no changes"
	}

	return plan, nil
}

func (planner taskPlanner) planFileChanges(environment *Environment, repository *RepositoryState) ([]taskFileChange, error) {
	changes := make([]taskFileChange, 0, len(planner.task.Files))
	seenPaths := map[string]struct{}{}

	for _, fileDefinition := range planner.task.Files {
		renderedPath, pathError := planner.renderTemplate(fileDefinition.PathTemplate, "")
		if pathError != nil {
			return nil, pathError
		}

		relativePath := filepath.Clean(renderedPath)
		if len(relativePath) == 0 || relativePath == "." || relativePath == ".." {
			return nil, fmt.Errorf("invalid file path %q after templating", renderedPath)
		}
		if filepath.IsAbs(relativePath) {
			return nil, fmt.Errorf("file path %q must be relative", renderedPath)
		}
		if _, exists := seenPaths[relativePath]; exists {
			return nil, fmt.Errorf("duplicate file path %s", relativePath)
		}
		seenPaths[relativePath] = struct{}{}

		content, contentError := planner.renderTemplate(fileDefinition.ContentTemplate, "")
		if contentError != nil {
			return nil, contentError
		}

		fileChange := taskFileChange{
			relativePath: relativePath,
			absolutePath: filepath.Join(repository.Path, relativePath),
			content:      []byte(content),
			mode:         fileDefinition.Mode,
			permissions:  fileDefinition.Permissions,
		}

		if environment != nil && environment.FileSystem != nil {
			existingContent, readError := environment.FileSystem.ReadFile(fileChange.absolutePath)
			switch {
			case readError == nil:
				switch fileDefinition.Mode {
				case TaskFileModeSkipIfExists:
					fileChange.apply = false
					fileChange.skipReason = "exists"
				case TaskFileModeLineEdit:
					if ensureLinesSatisfied(existingContent, fileChange.content) {
						fileChange.apply = false
						fileChange.skipReason = "lines-present"
					} else {
						fileChange.apply = true
					}
				default:
					if bytes.Equal(existingContent, fileChange.content) {
						fileChange.apply = false
						fileChange.skipReason = "unchanged"
					} else {
						fileChange.apply = true
					}
				}
			case errors.Is(readError, fs.ErrNotExist):
				fileChange.apply = true
			default:
				return nil, readError
			}
		} else {
			fileChange.apply = true
		}

		changes = append(changes, fileChange)
	}

	sort.Slice(changes, func(left, right int) bool {
		return changes[left].relativePath < changes[right].relativePath
	})

	return changes, nil
}

func ensureLinesSatisfied(existingContent []byte, desired []byte) bool {
	desiredLines := parseEnsureLines(desired)
	if len(desiredLines) == 0 {
		return true
	}
	existingSet := buildEnsureLineSet(existingContent)
	for _, line := range desiredLines {
		if _, ok := existingSet[line]; !ok {
			return false
		}
	}
	return true
}

func parseEnsureLines(content []byte) []string {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lines := make([]string, 0)
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if trimmed == "" {
			continue
		}
		lines = append(lines, trimmed)
	}
	return lines
}

func buildEnsureLineSet(content []byte) map[string]struct{} {
	set := make(map[string]struct{})
	for _, line := range parseEnsureLines(content) {
		set[line] = struct{}{}
	}
	return set
}

func (planner taskPlanner) planActions() ([]taskAction, error) {
	planned := make([]taskAction, 0, len(planner.task.Actions))
	for _, definition := range planner.task.Actions {
		if len(strings.TrimSpace(definition.Type)) == 0 {
			continue
		}

		parameters := make(map[string]any, len(definition.Options))
		for key, value := range definition.Options {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			if len(normalizedKey) == 0 {
				continue
			}
			switch typedValue := value.(type) {
			case string:
				rendered, renderError := planner.renderTemplate(typedValue, "")
				if renderError != nil {
					return nil, renderError
				}
				parameters[normalizedKey] = strings.TrimSpace(rendered)
			default:
				parameters[normalizedKey] = typedValue
			}
		}

		planned = append(planned, taskAction{
			actionType: definition.Type,
			parameters: parameters,
		})
	}
	return planned, nil
}

func (planner taskPlanner) planPullRequest(definition TaskPullRequestDefinition) (*taskPlanPullRequest, error) {
	title, titleError := planner.renderTemplate(definition.TitleTemplate, "")
	if titleError != nil {
		return nil, titleError
	}
	title = strings.TrimSpace(title)
	if len(title) == 0 {
		return nil, errors.New("pull request title is empty after templating")
	}

	body, bodyError := planner.renderTemplate(definition.BodyTemplate, "")
	if bodyError != nil {
		return nil, bodyError
	}

	baseTemplate := definition.BaseTemplate
	if len(strings.TrimSpace(baseTemplate)) == 0 {
		baseTemplate = "{{ .Repository.DefaultBranch }}"
	}
	base, baseError := planner.renderTemplate(baseTemplate, "")
	if baseError != nil {
		return nil, baseError
	}

	return &taskPlanPullRequest{title: title, body: body, base: strings.TrimSpace(base), draft: definition.Draft}, nil
}

func (planner taskPlanner) renderTemplate(rawTemplate string, fallback string) (string, error) {
	trimmed := strings.TrimSpace(rawTemplate)
	if len(trimmed) == 0 {
		return fallback, nil
	}

	tmpl, parseError := template.New("task").Parse(trimmed)
	if parseError != nil {
		return "", parseError
	}

	var buffer bytes.Buffer
	if executeError := tmpl.Execute(&buffer, planner.templateData); executeError != nil {
		return "", executeError
	}
	return buffer.String(), nil
}

func (planner taskPlanner) defaultBranchName() string {
	defaultName := strings.TrimSpace(planner.task.Name)
	if len(defaultName) == 0 {
		defaultName = "task"
	}
	return fmt.Sprintf("automation/%s", sanitizeBranchName(defaultName))
}

func buildTaskTemplateData(repository *RepositoryState, task TaskDefinition, variables map[string]string) TaskTemplateData {
	if repository == nil {
		return TaskTemplateData{}
	}

	var environmentValues map[string]string
	if len(variables) > 0 {
		environmentValues = make(map[string]string, len(variables))
		for key, value := range variables {
			environmentValues[key] = value
		}
	}

	owner, name := splitOwnerAndName(repository.Inspection.FinalOwnerRepo)
	if len(owner) == 0 && len(name) == 0 {
		owner, name = splitOwnerAndName(repository.Inspection.CanonicalOwnerRepo)
	}

	return TaskTemplateData{
		Task: task,
		Repository: TaskRepositoryTemplateData{
			Path:                  repository.Path,
			Owner:                 owner,
			Name:                  name,
			FullName:              repository.Inspection.FinalOwnerRepo,
			DefaultBranch:         repository.Inspection.RemoteDefaultBranch,
			PathDepth:             repository.PathDepth,
			InitialClean:          repository.InitialCleanWorktree,
			HasNestedRepositories: repository.HasNestedRepositories,
		},
		Environment: environmentValues,
	}
}

func splitOwnerAndName(fullName string) (string, string) {
	trimmed := strings.TrimSpace(fullName)
	if len(trimmed) == 0 {
		return "", ""
	}

	segments := strings.Split(trimmed, "/")
	if len(segments) != 2 {
		return "", trimmed
	}
	return segments[0], segments[1]
}

func hasApplicableChanges(changes []taskFileChange) bool {
	for _, change := range changes {
		if change.apply {
			return true
		}
	}
	return false
}

func sanitizeBranchName(name string) string {
	trimmed := strings.TrimSpace(name)
	if len(trimmed) == 0 {
		return "task"
	}

	sanitized := strings.ReplaceAll(trimmed, " ", "-")
	sanitized = strings.ReplaceAll(sanitized, "\t", "-")
	sanitized = strings.ReplaceAll(sanitized, "\n", "-")
	sanitized = strings.ReplaceAll(sanitized, "@", "-")
	sanitized = strings.ReplaceAll(sanitized, "#", "-")
	sanitized = strings.ReplaceAll(sanitized, "^", "-")
	sanitized = strings.ReplaceAll(sanitized, "\\", "-")
	sanitized = strings.ReplaceAll(sanitized, "/", "-")
	sanitized = strings.Trim(sanitized, "-")
	if len(sanitized) == 0 {
		return "task"
	}
	return sanitized
}

func (plan taskPlan) reportPlan(environment *Environment) {
	if environment == nil || environment.Reporter == nil {
		return
	}

	details := map[string]string{
		"task":        plan.task.Name,
		"branch":      plan.branchName,
		"start_point": plan.startPoint,
	}

	if len(plan.fileChanges) > 0 {
		details["files"] = fmt.Sprintf("%d", len(plan.fileChanges))
	}

	if len(plan.actions) > 0 {
		details["actions"] = formatActionParameters(map[string]any{
			"count": len(plan.actions),
		})
	}

	environment.ReportRepositoryEvent(
		plan.repository,
		shared.EventLevelInfo,
		shared.EventCodeTaskPlan,
		"task plan ready",
		details,
	)
}

func formatActionParameters(parameters map[string]any) string {
	if len(parameters) == 0 {
		return "{}"
	}

	keys := make([]string, 0, len(parameters))
	for key := range parameters {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	values := make([]string, 0, len(keys))
	for _, key := range keys {
		formatted := formatActionParameterValue(parameters[key])
		values = append(values, fmt.Sprintf("%s=%s", key, formatted))
	}

	return strings.Join(values, ", ")
}

func formatActionParameterValue(value any) string {
	switch typed := value.(type) {
	case *TaskLLMClientConfiguration:
		if typed == nil {
			return "<llm-client:nil>"
		}
		return "<llm-client>"
	default:
		return fmt.Sprintf("%v", value)
	}
}
