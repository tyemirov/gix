package workflow

import (
	"strings"
)

// TasksApplyDefinition describes serialized options for the tasks.apply operation.
type TasksApplyDefinition struct {
	Tasks []TaskDefinition
	LLM   *TaskLLMDefinition
}

// TaskLLMDefinition configures optional LLM client parameters for tasks.apply presets.
type TaskLLMDefinition struct {
	Model               string
	BaseURL             string
	APIKeyEnv           string
	TimeoutSeconds      int
	MaxCompletionTokens int
	Temperature         *float64
}

// Options returns the serialized configuration map for tasks.apply presets.
func (definition TasksApplyDefinition) Options() map[string]any {
	options := map[string]any{
		optionTasksKeyConstant: encodeTaskDefinitions(definition.Tasks),
	}
	if definition.LLM != nil {
		options[optionTaskLLMKeyConstant] = definition.LLM.encode()
	}
	return options
}

func (definition TaskLLMDefinition) encode() map[string]any {
	options := map[string]any{
		optionTaskLLMModelKeyConstant:     strings.TrimSpace(definition.Model),
		optionTaskLLMBaseURLKeyConstant:   strings.TrimSpace(definition.BaseURL),
		optionTaskLLMAPIKeyEnvKeyConstant: strings.TrimSpace(definition.APIKeyEnv),
	}
	if definition.TimeoutSeconds > 0 {
		options[optionTaskLLMTimeoutKeyConstant] = definition.TimeoutSeconds
	}
	if definition.MaxCompletionTokens > 0 {
		options[optionTaskLLMMaxTokensKeyConstant] = definition.MaxCompletionTokens
	}
	if definition.Temperature != nil {
		options[optionTaskLLMTemperatureKeyConstant] = *definition.Temperature
	}
	return options
}

func encodeTaskDefinitions(tasks []TaskDefinition) []any {
	if len(tasks) == 0 {
		return []any{}
	}
	entries := make([]any, 0, len(tasks))
	for _, task := range tasks {
		entries = append(entries, encodeTaskDefinition(task))
	}
	return entries
}

func encodeTaskDefinition(task TaskDefinition) map[string]any {
	entry := map[string]any{
		optionTaskNameKeyConstant: task.Name,
	}
	if !task.EnsureClean {
		entry[optionTaskEnsureCleanKeyConstant] = task.EnsureClean
	}
	if ensureCleanVariable := strings.TrimSpace(task.EnsureCleanVariable); ensureCleanVariable != "" {
		entry[optionTaskEnsureCleanVariableKeyConstant] = ensureCleanVariable
	}
	if branch := encodeTaskBranchDefinition(task.Branch); branch != nil {
		entry[optionTaskBranchKeyConstant] = branch
	}
	if len(task.Files) > 0 {
		entry[optionTaskFilesKeyConstant] = encodeTaskFiles(task.Files)
	}
	if len(task.Actions) > 0 {
		entry[optionTaskActionsKeyConstant] = encodeTaskActions(task.Actions)
	}
	if len(task.Safeguards) > 0 {
		entry[optionTaskSafeguardsKeyConstant] = cloneStringAnyMap(task.Safeguards)
	}
	if len(task.Steps) > 0 {
		entry[optionTaskStepsKeyConstant] = encodeTaskSteps(task.Steps)
	}
	entry[optionTaskCommitMessageKeyConstant] = task.Commit.MessageTemplate
	if task.PullRequest != nil {
		entry[optionTaskPullRequestKeyConstant] = encodeTaskPullRequest(*task.PullRequest)
	}
	return entry
}

func encodeTaskBranchDefinition(branch TaskBranchDefinition) map[string]any {
	if branch == (TaskBranchDefinition{}) {
		return nil
	}
	name := strings.TrimSpace(branch.NameTemplate)
	startPoint := strings.TrimSpace(branch.StartPointTemplate)
	pushRemote := strings.TrimSpace(branch.PushRemote)
	if name == "" && startPoint == "" && pushRemote == "" {
		return nil
	}
	entry := map[string]any{}
	if name != "" {
		entry[optionTaskBranchNameKeyConstant] = name
	}
	if startPoint != "" {
		entry[optionTaskBranchStartPointKeyConstant] = startPoint
	}
	if pushRemote != "" {
		entry[optionTaskBranchPushRemoteKeyConstant] = pushRemote
	}
	if len(entry) == 0 {
		return nil
	}
	return entry
}

func encodeTaskFiles(files []TaskFileDefinition) []any {
	entries := make([]any, 0, len(files))
	for _, file := range files {
		entry := map[string]any{
			optionTaskFilePathKeyConstant:        file.PathTemplate,
			optionTaskFileContentKeyConstant:     file.ContentTemplate,
			optionTaskFileModeKeyConstant:        string(file.Mode),
			optionTaskFilePermissionsKeyConstant: int(file.Permissions),
		}
		if len(file.Replacements) > 0 {
			entry[optionTaskFileReplacementsKeyConstant] = encodeTaskReplacements(file.Replacements)
		}
		entries = append(entries, entry)
	}
	return entries
}

func encodeTaskReplacements(replacements []TaskReplacementDefinition) []any {
	replacementEntries := make([]any, 0, len(replacements))
	for _, replacement := range replacements {
		replacementEntries = append(replacementEntries, map[string]any{
			optionTaskReplacementFromKeyConstant: replacement.FromTemplate,
			optionTaskReplacementToKeyConstant:   replacement.ToTemplate,
		})
	}
	return replacementEntries
}

func encodeTaskActions(actions []TaskActionDefinition) []any {
	actionEntries := make([]any, 0, len(actions))
	for _, action := range actions {
		entry := map[string]any{
			optionTaskActionTypeKeyConstant: action.Type,
		}
		if len(action.Options) > 0 {
			entry[optionTaskActionOptionsKeyConstant] = cloneStringAnyMap(action.Options)
		}
		actionEntries = append(actionEntries, entry)
	}
	return actionEntries
}

func encodeTaskSteps(steps []TaskExecutionStep) []string {
	stepEntries := make([]string, 0, len(steps))
	for _, step := range steps {
		stepEntries = append(stepEntries, string(step))
	}
	return stepEntries
}

func encodeTaskPullRequest(pullRequest TaskPullRequestDefinition) map[string]any {
	entry := map[string]any{
		optionTaskPRTitleKeyConstant: pullRequest.TitleTemplate,
		optionTaskPRBodyKeyConstant:  pullRequest.BodyTemplate,
		optionTaskPRBaseKeyConstant:  pullRequest.BaseTemplate,
	}
	if pullRequest.Draft {
		entry[optionTaskPRDraftKeyConstant] = pullRequest.Draft
	}
	return entry
}

func cloneStringAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}
