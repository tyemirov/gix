package workflow

import "io/fs"

const (
	optionTasksKeyConstant                   = "tasks"
	optionTaskNameKeyConstant                = "name"
	optionTaskEnsureCleanKeyConstant         = "ensure_clean"
	optionTaskEnsureCleanVariableKeyConstant = "ensure_clean_variable"
	optionTaskBranchKeyConstant              = "branch"
	optionTaskFilesKeyConstant               = "files"
	optionTaskCommitMessageKeyConstant       = "commit_message"
	optionTaskPullRequestKeyConstant         = "pull_request"
	optionTaskActionsKeyConstant             = "actions"
	optionTaskSafeguardsKeyConstant          = "safeguards"

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
	defaultTaskFilePermissions = fs.FileMode(0o644)
	defaultTaskPushRemote      = "origin"
)

type taskFileExistsMode string

const (
	taskFileModeOverwrite  taskFileExistsMode = "overwrite"
	taskFileModeSkipIfExists taskFileExistsMode = "skip-if-exists"
	taskFileModeLineEdit   taskFileExistsMode = "line-edit"
)

// TaskFileMode enumerates file handling semantics for task-managed files.
type TaskFileMode = taskFileExistsMode

const (
	// TaskFileModeOverwrite replaces existing files or creates new ones when absent.
	TaskFileModeOverwrite TaskFileMode = taskFileModeOverwrite
	// TaskFileModeSkipIfExists preserves existing files by skipping writes.
	TaskFileModeSkipIfExists TaskFileMode = taskFileModeSkipIfExists
	// TaskFileModeLineEdit appends any missing lines from the provided content while preserving existing entries.
	TaskFileModeLineEdit TaskFileMode = taskFileModeLineEdit
)

// TaskDefinition describes a single repository task.
type TaskDefinition struct {
	Name                string
	EnsureClean         bool
	EnsureCleanVariable string
	Branch              TaskBranchDefinition
	Files               []TaskFileDefinition
	Actions             []TaskActionDefinition
	Commit              TaskCommitDefinition
	PullRequest         *TaskPullRequestDefinition
	Safeguards          map[string]any
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
