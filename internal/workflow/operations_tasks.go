package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/githubcli"
)

const (
	optionTasksKeyConstant             = "tasks"
	optionTaskNameKeyConstant          = "name"
	optionTaskEnsureCleanKeyConstant   = "ensure_clean"
	optionTaskBranchKeyConstant        = "branch"
	optionTaskFilesKeyConstant         = "files"
	optionTaskCommitMessageKeyConstant = "commit_message"
	optionTaskPullRequestKeyConstant   = "pull_request"
	optionTaskActionsKeyConstant       = "actions"
	optionTaskSafeguardsKeyConstant    = "safeguards"

	optionTaskBranchNameKeyConstant       = "name"
	optionTaskBranchStartPointKeyConstant = "start_point"
	optionTaskBranchPushRemoteKeyConstant = "push_remote"

	optionTaskFilePathKeyConstant        = "path"
	optionTaskFileContentKeyConstant     = "content"
	optionTaskFileModeKeyConstant        = "mode"
	optionTaskFilePermissionsKeyConstant = "permissions"

	optionTaskPRTitleKeyConstant       = "title"
	optionTaskPRBodyKeyConstant        = "body"
	optionTaskPRBaseKeyConstant        = "base"
	optionTaskPRDraftKeyConstant       = "draft"
	optionTaskActionTypeKeyConstant    = "type"
	optionTaskActionOptionsKeyConstant = "options"
)

const (
	taskLogPrefixPlan   = "TASK-PLAN"
	taskLogPrefixApply  = "TASK-APPLY"
	taskLogPrefixSkip   = "TASK-SKIP"
	taskLogPrefixNoop   = "TASK-NOOP"
	taskLogPrefixCancel = "TASK-CANCEL"
)

const (
	defaultTaskFilePermissions = fs.FileMode(0o644)
	defaultTaskPushRemote      = "origin"
)

type taskFileExistsMode string

const (
	taskFileModeOverwrite    taskFileExistsMode = "overwrite"
	taskFileModeSkipIfExists taskFileExistsMode = "skip-if-exists"
)

// TaskOperation executes declarative repository tasks (file mutations, commits, and PRs).
type TaskOperation struct {
	tasks []TaskDefinition
}

// Definitions returns a copy of the task definitions associated with the operation.
func (operation *TaskOperation) Definitions() []TaskDefinition {
	if operation == nil || len(operation.tasks) == 0 {
		return nil
	}
	definitions := make([]TaskDefinition, len(operation.tasks))
	copy(definitions, operation.tasks)
	return definitions
}

// TaskDefinition describes a single repository task.
type TaskDefinition struct {
	Name        string
	EnsureClean bool
	Branch      TaskBranchDefinition
	Files       []TaskFileDefinition
	Actions     []TaskActionDefinition
	Commit      TaskCommitDefinition
	PullRequest *TaskPullRequestDefinition
	Safeguards  map[string]any
}

// TaskBranchDefinition describes branch behavior for a task.
type TaskBranchDefinition struct {
	NameTemplate       string
	StartPointTemplate string
	PushRemote         string
}

// TaskFileDefinition captures file mutation instructions.
type TaskFileDefinition struct {
	PathTemplate    string
	ContentTemplate string
	Mode            taskFileExistsMode
	Permissions     fs.FileMode
}

// TaskCommitDefinition describes commit metadata for a task.
type TaskCommitDefinition struct {
	MessageTemplate string
}

// TaskActionDefinition describes an imperative action executed as part of a task.
type TaskActionDefinition struct {
	Type    string
	Options map[string]any
}

// TaskPullRequestDefinition configures optional pull request creation.
type TaskPullRequestDefinition struct {
	TitleTemplate string
	BodyTemplate  string
	BaseTemplate  string
	Draft         bool
}

// TaskTemplateData exposes templating values for task rendering.
type TaskTemplateData struct {
	Task        TaskDefinition
	Repository  TaskRepositoryTemplateData
	Environment map[string]string
}

// TaskRepositoryTemplateData provides repository metadata for templating.
type TaskRepositoryTemplateData struct {
	Path                  string
	Owner                 string
	Name                  string
	FullName              string
	DefaultBranch         string
	PathDepth             int
	InitialClean          bool
	HasNestedRepositories bool
}

// Name identifies the operation type.
func (operation *TaskOperation) Name() string {
	return string(OperationTypeApplyTasks)
}

// Execute runs the configured tasks across repositories.
func (operation *TaskOperation) Execute(executionContext context.Context, environment *Environment, state *State) error {
	if operation == nil || environment == nil || state == nil {
		return nil
	}

	for _, repository := range state.Repositories {
		if repository == nil {
			continue
		}
		for _, task := range operation.tasks {
			if err := operation.executeTask(executionContext, environment, repository, task); err != nil {
				return err
			}
		}
	}

	return nil
}

func (operation *TaskOperation) executeTask(executionContext context.Context, environment *Environment, repository *RepositoryState, task TaskDefinition) error {
	if len(task.Safeguards) > 0 {
		pass, reason, evalError := EvaluateSafeguards(executionContext, environment, repository, task.Safeguards)
		if evalError != nil {
			return evalError
		}
		if !pass {
			if environment != nil && environment.Output != nil {
				trimmedReason := strings.TrimSpace(reason)
				if len(trimmedReason) == 0 {
					trimmedReason = "safeguard failed"
				}
				fmt.Fprintf(environment.Output, "%s: %s %s %s\n", taskLogPrefixSkip, task.Name, repository.Path, trimmedReason)
			}
			return nil
		}
	}

	templateData := buildTaskTemplateData(repository, task)

	planner := newTaskPlanner(task, templateData)
	plan, planError := planner.BuildPlan(environment, repository)
	if planError != nil {
		return planError
	}

	if environment.DryRun {
		plan.describe(environment, taskLogPrefixPlan)
		if len(plan.actions) > 0 {
			actionExecutor := newTaskActionExecutor(environment)
			for _, action := range plan.actions {
				if err := actionExecutor.execute(executionContext, repository, action); err != nil {
					return err
				}
			}
		}
		return nil
	}

	executor := newTaskExecutor(environment, repository, plan)
	return executor.Execute(executionContext)
}

// buildTaskOperation constructs a TaskOperation from declarative options.
func buildTaskOperation(options map[string]any) (Operation, error) {
	reader := newOptionReader(options)
	entries, exists, err := reader.mapSlice(optionTasksKeyConstant)
	if err != nil {
		return nil, err
	}
	if !exists || len(entries) == 0 {
		return nil, errors.New("apply-tasks step requires at least one task entry")
	}

	tasks := make([]TaskDefinition, 0, len(entries))
	for _, entry := range entries {
		task, buildError := buildTaskDefinition(entry)
		if buildError != nil {
			return nil, buildError
		}
		tasks = append(tasks, task)
	}

	return &TaskOperation{tasks: tasks}, nil
}

func buildTaskDefinition(raw map[string]any) (TaskDefinition, error) {
	reader := newOptionReader(raw)

	name, nameExists, nameError := reader.stringValue(optionTaskNameKeyConstant)
	if nameError != nil {
		return TaskDefinition{}, nameError
	}
	if !nameExists || len(name) == 0 {
		return TaskDefinition{}, errors.New("task name must be provided")
	}

	ensureClean, ensureCleanExists, ensureCleanError := reader.boolValue(optionTaskEnsureCleanKeyConstant)
	if ensureCleanError != nil {
		return TaskDefinition{}, ensureCleanError
	}
	if !ensureCleanExists {
		ensureClean = true
	}

	branchDefinition, branchError := buildTaskBranchDefinition(reader)
	if branchError != nil {
		return TaskDefinition{}, branchError
	}

	files, filesError := buildTaskFiles(reader)
	if filesError != nil {
		return TaskDefinition{}, filesError
	}
	actions, actionsError := buildTaskActions(reader)
	if actionsError != nil {
		return TaskDefinition{}, actionsError
	}
	if len(files) == 0 && len(actions) == 0 {
		return TaskDefinition{}, fmt.Errorf("task %s must declare at least one file or action", name)
	}

	commitDefinition, commitError := buildTaskCommitDefinition(reader)
	if commitError != nil {
		return TaskDefinition{}, commitError
	}

	pullRequestDefinition, pullRequestError := buildTaskPullRequestDefinition(reader)
	if pullRequestError != nil {
		return TaskDefinition{}, pullRequestError
	}

	safeguards, _, safeguardsError := reader.mapValue(optionTaskSafeguardsKeyConstant)
	if safeguardsError != nil {
		return TaskDefinition{}, safeguardsError
	}

	return TaskDefinition{
		Name:        name,
		EnsureClean: ensureClean,
		Branch:      branchDefinition,
		Files:       files,
		Actions:     actions,
		Commit:      commitDefinition,
		PullRequest: pullRequestDefinition,
		Safeguards:  safeguards,
	}, nil
}

func buildTaskBranchDefinition(reader optionReader) (TaskBranchDefinition, error) {
	branchOptions, exists, err := reader.mapValue(optionTaskBranchKeyConstant)
	if err != nil {
		return TaskBranchDefinition{}, err
	}
	if !exists {
		return TaskBranchDefinition{PushRemote: defaultTaskPushRemote}, nil
	}

	branchReader := newOptionReader(branchOptions)
	nameTemplate, _, nameError := branchReader.stringValue(optionTaskBranchNameKeyConstant)
	if nameError != nil {
		return TaskBranchDefinition{}, nameError
	}
	startPointTemplate, _, startPointError := branchReader.stringValue(optionTaskBranchStartPointKeyConstant)
	if startPointError != nil {
		return TaskBranchDefinition{}, startPointError
	}
	pushRemote, pushRemoteExists, pushRemoteError := branchReader.stringValue(optionTaskBranchPushRemoteKeyConstant)
	if pushRemoteError != nil {
		return TaskBranchDefinition{}, pushRemoteError
	}
	if !pushRemoteExists || len(pushRemote) == 0 {
		pushRemote = defaultTaskPushRemote
	}

	return TaskBranchDefinition{NameTemplate: nameTemplate, StartPointTemplate: startPointTemplate, PushRemote: pushRemote}, nil
}

func buildTaskFiles(reader optionReader) ([]TaskFileDefinition, error) {
	fileEntries, exists, err := reader.mapSlice(optionTaskFilesKeyConstant)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	files := make([]TaskFileDefinition, 0, len(fileEntries))
	for _, entry := range fileEntries {
		fileReader := newOptionReader(entry)
		pathTemplate, pathExists, pathError := fileReader.stringValue(optionTaskFilePathKeyConstant)
		if pathError != nil {
			return nil, pathError
		}
		if !pathExists || len(pathTemplate) == 0 {
			return nil, errors.New("task file path must be provided")
		}

		contentTemplate, contentExists, contentError := fileReader.stringValue(optionTaskFileContentKeyConstant)
		if contentError != nil {
			return nil, contentError
		}
		if !contentExists {
			return nil, fmt.Errorf("task file %s must provide content", pathTemplate)
		}

		modeValue, _, modeError := fileReader.stringValue(optionTaskFileModeKeyConstant)
		if modeError != nil {
			return nil, modeError
		}
		mode := parseTaskFileMode(modeValue)

		permissions, _, permissionsError := fileReader.stringValue(optionTaskFilePermissionsKeyConstant)
		if permissionsError != nil {
			return nil, permissionsError
		}
		parsedPermissions, parseError := parseFilePermissions(permissions)
		if parseError != nil {
			return nil, parseError
		}

		files = append(files, TaskFileDefinition{
			PathTemplate:    pathTemplate,
			ContentTemplate: contentTemplate,
			Mode:            mode,
			Permissions:     parsedPermissions,
		})
	}

	return files, nil
}

func buildTaskActions(reader optionReader) ([]TaskActionDefinition, error) {
	actionEntries, exists, err := reader.mapSlice(optionTaskActionsKeyConstant)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	actions := make([]TaskActionDefinition, 0, len(actionEntries))
	for _, entry := range actionEntries {
		actionReader := newOptionReader(entry)

		typeValue, typeExists, typeError := actionReader.stringValue(optionTaskActionTypeKeyConstant)
		if typeError != nil {
			return nil, typeError
		}
		trimmedType := strings.TrimSpace(typeValue)
		if !typeExists || len(trimmedType) == 0 {
			return nil, errors.New("task action type must be provided")
		}

		options, _, optionsError := actionReader.mapValue(optionTaskActionOptionsKeyConstant)
		if optionsError != nil {
			return nil, optionsError
		}

		normalizedOptions := make(map[string]any, len(options))
		for key, value := range options {
			normalizedOptions[key] = value
		}

		actions = append(actions, TaskActionDefinition{
			Type:    trimmedType,
			Options: normalizedOptions,
		})
	}

	return actions, nil
}

func buildTaskCommitDefinition(reader optionReader) (TaskCommitDefinition, error) {
	messageTemplate, exists, err := reader.stringValue(optionTaskCommitMessageKeyConstant)
	if err != nil {
		return TaskCommitDefinition{}, err
	}
	if !exists || len(messageTemplate) == 0 {
		messageTemplate = "Apply task {{ .Task.Name }}"
	}
	return TaskCommitDefinition{MessageTemplate: messageTemplate}, nil
}

func buildTaskPullRequestDefinition(reader optionReader) (*TaskPullRequestDefinition, error) {
	prOptions, exists, err := reader.mapValue(optionTaskPullRequestKeyConstant)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	prReader := newOptionReader(prOptions)
	titleTemplate, titleExists, titleError := prReader.stringValue(optionTaskPRTitleKeyConstant)
	if titleError != nil {
		return nil, titleError
	}
	if !titleExists || len(titleTemplate) == 0 {
		return nil, errors.New("pull request title must be provided")
	}

	bodyTemplate, _, bodyError := prReader.stringValue(optionTaskPRBodyKeyConstant)
	if bodyError != nil {
		return nil, bodyError
	}

	baseTemplate, _, baseError := prReader.stringValue(optionTaskPRBaseKeyConstant)
	if baseError != nil {
		return nil, baseError
	}

	draft, draftExists, draftError := prReader.boolValue(optionTaskPRDraftKeyConstant)
	if draftError != nil {
		return nil, draftError
	}
	if !draftExists {
		draft = false
	}

	return &TaskPullRequestDefinition{TitleTemplate: titleTemplate, BodyTemplate: bodyTemplate, BaseTemplate: baseTemplate, Draft: draft}, nil
}

func parseTaskFileMode(raw string) taskFileExistsMode {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	switch trimmed {
	case string(taskFileModeSkipIfExists):
		return taskFileModeSkipIfExists
	default:
		return taskFileModeOverwrite
	}
}

func parseFilePermissions(raw string) (fs.FileMode, error) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) == 0 {
		return defaultTaskFilePermissions, nil
	}

	value, parseError := strconv.ParseUint(trimmed, 8, 32)
	if parseError != nil {
		return 0, fmt.Errorf("invalid file permissions %s", raw)
	}

	return fs.FileMode(value), nil
}

func buildTaskTemplateData(repository *RepositoryState, task TaskDefinition) TaskTemplateData {
	owner, name := splitOwnerAndName(repository.Inspection.FinalOwnerRepo)
	defaultBranch := strings.TrimSpace(repository.Inspection.RemoteDefaultBranch)
	if len(defaultBranch) == 0 {
		defaultBranch = strings.TrimSpace(repository.Inspection.LocalBranch)
	}

	return TaskTemplateData{
		Task: task,
		Repository: TaskRepositoryTemplateData{
			Path:                  repository.Path,
			Owner:                 owner,
			Name:                  name,
			FullName:              repository.Inspection.FinalOwnerRepo,
			DefaultBranch:         defaultBranch,
			PathDepth:             repository.PathDepth,
			InitialClean:          repository.InitialCleanWorktree,
			HasNestedRepositories: repository.HasNestedRepositories,
		},
		Environment: map[string]string{},
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

// taskPlan captures the actions required for a repository/task pair.
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
		pathValue, pathError := planner.renderTemplate(fileDefinition.PathTemplate, "")
		if pathError != nil {
			return nil, pathError
		}
		relativePath := filepath.Clean(pathValue)
		if relativePath == "." || relativePath == "" {
			return nil, fmt.Errorf("task %s contains invalid file path", planner.task.Name)
		}
		if _, exists := seenPaths[relativePath]; exists {
			return nil, fmt.Errorf("task %s references file %s multiple times", planner.task.Name, relativePath)
		}
		seenPaths[relativePath] = struct{}{}

		contentValue, contentError := planner.renderTemplate(fileDefinition.ContentTemplate, "")
		if contentError != nil {
			return nil, contentError
		}

		absolutePath := filepath.Join(repository.Path, relativePath)
		fileChange := taskFileChange{
			relativePath: relativePath,
			absolutePath: absolutePath,
			content:      []byte(contentValue),
			mode:         fileDefinition.Mode,
			permissions:  fileDefinition.Permissions,
		}

		existingContent, readError := environment.FileSystem.ReadFile(absolutePath)
		if readError == nil {
			if fileDefinition.Mode == taskFileModeSkipIfExists {
				fileChange.apply = false
				fileChange.skipReason = "exists"
			} else if bytes.Equal(existingContent, fileChange.content) {
				fileChange.apply = false
				fileChange.skipReason = "unchanged"
			} else {
				fileChange.apply = true
			}
		} else {
			if !errors.Is(readError, fs.ErrNotExist) {
				return nil, readError
			}
			fileChange.apply = true
		}

		changes = append(changes, fileChange)
	}

	sort.Slice(changes, func(left, right int) bool {
		return changes[left].relativePath < changes[right].relativePath
	})

	return changes, nil
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

func (plan taskPlan) describe(environment *Environment, prefix string) {
	if environment == nil || environment.Output == nil {
		return
	}

	if len(plan.fileChanges) == 0 && plan.pullRequest == nil {
		// Action-only tasks defer to the underlying operation outputs.
		return
	}

	fmt.Fprintf(environment.Output, "%s: %s %s branch=%s base=%s\n", prefix, plan.task.Name, plan.repository.Path, plan.branchName, plan.startPoint)
	for _, change := range plan.fileChanges {
		action := "write"
		if !change.apply {
			action = fmt.Sprintf("skip (%s)", change.skipReason)
		}
		fmt.Fprintf(environment.Output, "%s: %s file=%s action=%s\n", prefix, plan.task.Name, change.relativePath, action)
	}

	for _, action := range plan.actions {
		fmt.Fprintf(environment.Output, "%s: %s action=%s params=%s\n", prefix, plan.task.Name, action.actionType, formatActionParameters(action.parameters))
	}

	if plan.pullRequest != nil {
		fmt.Fprintf(environment.Output, "%s: %s pull-request title=%q base=%s draft=%t\n", prefix, plan.task.Name, plan.pullRequest.title, plan.pullRequest.base, plan.pullRequest.draft)
	}
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
		values = append(values, fmt.Sprintf("%s=%v", key, parameters[key]))
	}

	return strings.Join(values, ", ")
}

type taskExecutor struct {
	environment *Environment
	repository  *RepositoryState
	plan        taskPlan
}

func newTaskExecutor(environment *Environment, repository *RepositoryState, plan taskPlan) taskExecutor {
	return taskExecutor{environment: environment, repository: repository, plan: plan}
}

func (executor taskExecutor) Execute(executionContext context.Context) error {
	if executor.environment == nil {
		return nil
	}

	if executor.plan.skipped {
		executor.plan.describe(executor.environment, taskLogPrefixNoop)
		return nil
	}

	hasFileChanges := hasApplicableChanges(executor.plan.fileChanges)
	hasActions := len(executor.plan.actions) > 0

	if executor.plan.task.EnsureClean {
		clean, cleanError := executor.environment.RepositoryManager.CheckCleanWorktree(executionContext, executor.repository.Path)
		if cleanError != nil {
			return cleanError
		}
		if !clean {
			executor.logf(taskLogPrefixSkip, "repository dirty", nil)
			return nil
		}
	}

	originalBranch := ""
	cleanup := func() {}

	if hasFileChanges {
		if branchExists, existsError := executor.branchExists(executionContext, executor.plan.branchName); existsError != nil {
			return existsError
		} else if branchExists {
			executor.logf(taskLogPrefixSkip, "branch exists", map[string]any{"branch": executor.plan.branchName})
			return nil
		}

		var branchError error
		originalBranch, branchError = executor.environment.RepositoryManager.GetCurrentBranch(executionContext, executor.repository.Path)
		if branchError != nil {
			return branchError
		}

		cleanup = func() {
			if len(strings.TrimSpace(originalBranch)) == 0 {
				return
			}
			_ = executor.checkoutBranch(executionContext, originalBranch)
		}

		if len(strings.TrimSpace(executor.plan.startPoint)) > 0 {
			if err := executor.checkoutBranch(executionContext, executor.plan.startPoint); err != nil {
				cleanup()
				return err
			}
		}

		if err := executor.checkoutOrCreateTaskBranch(executionContext); err != nil {
			cleanup()
			return err
		}

		if err := executor.applyFileChanges(); err != nil {
			cleanup()
			return err
		}

		if err := executor.stageChanges(executionContext); err != nil {
			cleanup()
			return err
		}

		if err := executor.commitChanges(executionContext); err != nil {
			cleanup()
			return err
		}

		if err := executor.pushBranch(executionContext); err != nil {
			cleanup()
			return err
		}

		if executor.plan.pullRequest != nil {
			if err := executor.createPullRequest(executionContext); err != nil {
				cleanup()
				return err
			}
		}
	}

	if hasActions {
		if err := executor.executeActions(executionContext); err != nil {
			cleanup()
			return err
		}
	}

	if hasFileChanges || executor.plan.pullRequest != nil {
		logFields := map[string]any{}
		if hasFileChanges {
			logFields["branch"] = executor.plan.branchName
		}
		if hasActions {
			logFields["actions"] = len(executor.plan.actions)
		}
		if len(logFields) == 0 {
			logFields = nil
		}
		executor.logf(taskLogPrefixApply, "applied", logFields)
	}

	cleanup()
	return nil
}

func (executor taskExecutor) branchExists(executionContext context.Context, branchName string) (bool, error) {
	arguments := []string{"rev-parse", "--verify", branchName}
	_, err := executor.environment.GitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{Arguments: arguments, WorkingDirectory: executor.repository.Path})
	if err == nil {
		return true, nil
	}
	var commandError execshell.CommandFailedError
	if errors.As(err, &commandError) {
		return false, nil
	}
	return false, err
}

func (executor taskExecutor) checkoutBranch(executionContext context.Context, branch string) error {
	if len(strings.TrimSpace(branch)) == 0 {
		return nil
	}
	arguments := []string{"checkout", branch}
	_, err := executor.environment.GitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{Arguments: arguments, WorkingDirectory: executor.repository.Path})
	return err
}

func (executor taskExecutor) checkoutOrCreateTaskBranch(executionContext context.Context) error {
	arguments := []string{"checkout", "-B", executor.plan.branchName}
	if len(strings.TrimSpace(executor.plan.startPoint)) > 0 {
		arguments = append(arguments, executor.plan.startPoint)
	}
	_, err := executor.environment.GitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{Arguments: arguments, WorkingDirectory: executor.repository.Path})
	return err
}

func (executor taskExecutor) applyFileChanges() error {
	for _, change := range executor.plan.fileChanges {
		if !change.apply {
			continue
		}

		directory := filepath.Dir(change.absolutePath)
		if err := executor.environment.FileSystem.MkdirAll(directory, 0o755); err != nil {
			return err
		}
		if err := executor.environment.FileSystem.WriteFile(change.absolutePath, change.content, change.permissions); err != nil {
			return err
		}
	}
	return nil
}

func (executor taskExecutor) stageChanges(executionContext context.Context) error {
	for _, change := range executor.plan.fileChanges {
		if !change.apply {
			continue
		}
		_, err := executor.environment.GitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{Arguments: []string{"add", change.relativePath}, WorkingDirectory: executor.repository.Path})
		if err != nil {
			return err
		}
	}
	return nil
}

func (executor taskExecutor) commitChanges(executionContext context.Context) error {
	arguments := []string{"commit", "-m", executor.plan.commitMessage}
	_, err := executor.environment.GitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{Arguments: arguments, WorkingDirectory: executor.repository.Path})
	return err
}

func (executor taskExecutor) pushBranch(executionContext context.Context) error {
	arguments := []string{"push", "--set-upstream", executor.plan.task.Branch.PushRemote, executor.plan.branchName}
	_, err := executor.environment.GitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{Arguments: arguments, WorkingDirectory: executor.repository.Path})
	return err
}

func (executor taskExecutor) createPullRequest(executionContext context.Context) error {
	pr := executor.plan.pullRequest
	if pr == nil {
		return nil
	}

	repository := executor.repository.Inspection.FinalOwnerRepo
	if len(strings.TrimSpace(repository)) == 0 {
		return errors.New("unable to determine repository owner/name for pull request")
	}

	options := githubPullRequestOptions{
		Repository: repository,
		Title:      pr.title,
		Body:       pr.body,
		Base:       pr.base,
		Head:       executor.plan.branchName,
		Draft:      pr.draft,
	}
	return createPullRequest(executionContext, executor.environment, options)
}

func (executor taskExecutor) executeActions(executionContext context.Context) error {
	actionExecutor := newTaskActionExecutor(executor.environment)
	for _, action := range executor.plan.actions {
		if err := actionExecutor.execute(executionContext, executor.repository, action); err != nil {
			return err
		}
	}
	return nil
}

func (executor taskExecutor) logf(prefix string, message string, fields map[string]any) {
	if executor.environment == nil || executor.environment.Output == nil {
		return
	}

	if len(fields) == 0 {
		fmt.Fprintf(executor.environment.Output, "%s: %s %s %s\n", prefix, executor.plan.task.Name, executor.repository.Path, message)
		return
	}

	pairs := make([]string, 0, len(fields))
	for key, value := range fields {
		pairs = append(pairs, fmt.Sprintf("%s=%v", key, value))
	}
	sort.Strings(pairs)
	fmt.Fprintf(executor.environment.Output, "%s: %s %s %s %s\n", prefix, executor.plan.task.Name, executor.repository.Path, message, strings.Join(pairs, " "))
}

type githubPullRequestOptions struct {
	Repository string
	Title      string
	Body       string
	Base       string
	Head       string
	Draft      bool
}

func createPullRequest(executionContext context.Context, environment *Environment, options githubPullRequestOptions) error {
	if environment == nil || environment.GitHubClient == nil {
		return errors.New("GitHub client not configured for task execution")
	}

	return environment.GitHubClient.CreatePullRequest(executionContext, githubcli.PullRequestCreateOptions{
		Repository: options.Repository,
		Title:      options.Title,
		Body:       options.Body,
		Base:       options.Base,
		Head:       options.Head,
		Draft:      options.Draft,
	})
}
