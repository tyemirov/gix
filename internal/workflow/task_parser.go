package workflow

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// ParseTaskFileMode normalizes a raw mode string into a TaskFileMode value.
func ParseTaskFileMode(raw string) TaskFileMode {
	return parseTaskFileMode(raw)
}

// buildTaskOperation constructs a TaskOperation from declarative options.
func buildTaskOperation(options map[string]any) (Operation, error) {
	reader := newOptionReader(options)

	llmConfiguration, llmConfigurationErr := buildTaskLLMConfiguration(reader)
	if llmConfigurationErr != nil {
		return nil, llmConfigurationErr
	}

	entries, exists, err := reader.mapSlice(optionTasksKeyConstant)
	if err != nil {
		return nil, err
	}
	if !exists || len(entries) == 0 {
		return nil, errors.New("tasks apply step requires at least one task entry")
	}

	tasks := make([]TaskDefinition, 0, len(entries))
	for _, entry := range entries {
		task, buildError := buildTaskDefinition(entry)
		if buildError != nil {
			return nil, buildError
		}
		tasks = append(tasks, task)
	}

	operation := &TaskOperation{
		tasks:            tasks,
		llmConfiguration: llmConfiguration,
	}
	operation.attachLLMConfiguration()
	return operation, nil
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

	ensureCleanVariable, ensureCleanVariableExists, ensureCleanVariableError := reader.stringValue(optionTaskEnsureCleanVariableKeyConstant)
	if ensureCleanVariableError != nil {
		return TaskDefinition{}, ensureCleanVariableError
	}
	ensureCleanVariable = strings.TrimSpace(ensureCleanVariable)
	if ensureCleanVariableExists && len(ensureCleanVariable) == 0 {
		return TaskDefinition{}, errors.New("ensure_clean_variable cannot be blank when provided")
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
		Name:                name,
		EnsureClean:         ensureClean,
		EnsureCleanVariable: ensureCleanVariable,
		Branch:              branchDefinition,
		Files:               files,
		Actions:             actions,
		Commit:              commitDefinition,
		PullRequest:         pullRequestDefinition,
		Safeguards:          safeguards,
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

	return TaskBranchDefinition{
		NameTemplate:       nameTemplate,
		StartPointTemplate: startPointTemplate,
		PushRemote:         pushRemote,
	}, nil
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
		if !pathExists || len(strings.TrimSpace(pathTemplate)) == 0 {
			return nil, errors.New("file path must be provided")
		}

		contentTemplate, _, contentError := fileReader.stringValue(optionTaskFileContentKeyConstant)
		if contentError != nil {
			return nil, contentError
		}

		modeValue, modeExists, modeError := fileReader.stringValue(optionTaskFileModeKeyConstant)
		if modeError != nil {
			return nil, modeError
		}
		mode := TaskFileModeOverwrite
		if modeExists {
			mode = parseTaskFileMode(modeValue)
		}

		permissionsValue, permissionsExists, permissionsError := fileReader.intValue(optionTaskFilePermissionsKeyConstant)
		if permissionsError != nil {
			return nil, permissionsError
		}
		permissions := defaultTaskFilePermissions
		if permissionsExists {
			permissions = fs.FileMode(permissionsValue)
		}

		files = append(files, TaskFileDefinition{
			PathTemplate:    pathTemplate,
			ContentTemplate: contentTemplate,
			Mode:            mode,
			Permissions:     permissions,
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
		actionType, typeExists, typeError := actionReader.stringValue(optionTaskActionTypeKeyConstant)
		if typeError != nil {
			return nil, typeError
		}
		if !typeExists || len(strings.TrimSpace(actionType)) == 0 {
			return nil, errors.New("action type must be provided")
		}

		options, _, optionsError := actionReader.mapValue(optionTaskActionOptionsKeyConstant)
		if optionsError != nil {
			return nil, optionsError
		}
		actions = append(actions, TaskActionDefinition{Type: actionType, Options: options})
	}
	return actions, nil
}

func buildTaskCommitDefinition(reader optionReader) (TaskCommitDefinition, error) {
	message, _, err := reader.stringValue(optionTaskCommitMessageKeyConstant)
	if err != nil {
		return TaskCommitDefinition{}, err
	}
	if len(strings.TrimSpace(message)) == 0 {
		message = "Apply task"
	}
	return TaskCommitDefinition{MessageTemplate: message}, nil
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
	titleTemplate, _, titleError := prReader.stringValue(optionTaskPRTitleKeyConstant)
	if titleError != nil {
		return nil, titleError
	}

	bodyTemplate, _, bodyError := prReader.stringValue(optionTaskPRBodyKeyConstant)
	if bodyError != nil {
		return nil, bodyError
	}

	baseTemplate, _, baseError := prReader.stringValue(optionTaskPRBaseKeyConstant)
	if baseError != nil {
		return nil, baseError
	}

	draft, _, draftError := prReader.boolValue(optionTaskPRDraftKeyConstant)
	if draftError != nil {
		return nil, draftError
	}

	return &TaskPullRequestDefinition{
		TitleTemplate: titleTemplate,
		BodyTemplate:  bodyTemplate,
		BaseTemplate:  baseTemplate,
		Draft:         draft,
	}, nil
}

func parseTaskFileMode(raw string) TaskFileMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(taskFileModeSkipIfExists):
		return TaskFileModeSkipIfExists
	case string(taskFileModeAppendIfMissing):
		return TaskFileModeAppendIfMissing
	case string(taskFileModeLegacyLineEdit):
		return TaskFileModeAppendIfMissing
	default:
		return TaskFileModeOverwrite
	}
}
