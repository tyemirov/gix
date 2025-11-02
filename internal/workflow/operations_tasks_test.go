package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/audit"
	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/gitrepo"
)

func TestTaskPlannerBuildPlanRendersTemplates(testInstance *testing.T) {
	fileSystem := newFakeFileSystem(nil)
	environment := &Environment{FileSystem: fileSystem}

	inspection := audit.RepositoryInspection{
		Path:                "/repositories/sample",
		FinalOwnerRepo:      "octocat/sample",
		RemoteDefaultBranch: "main",
		LocalBranch:         "develop",
	}
	repository := NewRepositoryState(inspection)

	taskDefinition := TaskDefinition{
		Name:        "Add Docs",
		EnsureClean: true,
		Branch: TaskBranchDefinition{
			NameTemplate: "feature/{{ .Repository.Name }}/docs update",
		},
		Files: []TaskFileDefinition{{
			PathTemplate:    "docs/{{ .Repository.Name }}.md",
			ContentTemplate: "Repository: {{ .Repository.FullName }}",
			Mode:            taskFileModeOverwrite,
			Permissions:     defaultTaskFilePermissions,
		}},
		Commit: TaskCommitDefinition{
			MessageTemplate: " docs: update {{ .Task.Name }} ",
		},
	}

	templateData := buildTaskTemplateData(repository, taskDefinition)
	planner := newTaskPlanner(taskDefinition, templateData)

	plan, planError := planner.BuildPlan(environment, repository)
	require.NoError(testInstance, planError)

	require.False(testInstance, plan.skipped)
	require.Equal(testInstance, "feature-sample-docs-update", plan.branchName)
	require.Equal(testInstance, "main", plan.startPoint)
	require.Equal(testInstance, "docs: update Add Docs", plan.commitMessage)
	require.Len(testInstance, plan.fileChanges, 1)

	fileChange := plan.fileChanges[0]
	require.Equal(testInstance, "docs/sample.md", fileChange.relativePath)
	require.Equal(testInstance, filepath.Join(repository.Path, "docs/sample.md"), fileChange.absolutePath)
	require.True(testInstance, fileChange.apply)
	require.Equal(testInstance, []byte("Repository: octocat/sample"), fileChange.content)
	require.Equal(testInstance, defaultTaskFilePermissions, fileChange.permissions)
	require.Nil(testInstance, plan.pullRequest)
}

func TestTaskPlannerBuildPlanSupportsActions(testInstance *testing.T) {
	fileSystem := newFakeFileSystem(nil)
	environment := &Environment{FileSystem: fileSystem}

	inspection := audit.RepositoryInspection{
		Path:                "/repositories/sample",
		FinalOwnerRepo:      "octocat/sample",
		RemoteDefaultBranch: "main",
	}
	repository := NewRepositoryState(inspection)

	taskDefinition := TaskDefinition{
		Name: "Remote Update",
		Actions: []TaskActionDefinition{{
			Type: "repo.remote.update",
			Options: map[string]any{
				"owner":  "{{ .Repository.Owner }}",
				"dryRun": true,
			},
		}},
		Commit: TaskCommitDefinition{},
	}

	templateData := buildTaskTemplateData(repository, taskDefinition)
	planner := newTaskPlanner(taskDefinition, templateData)

	plan, planError := planner.BuildPlan(environment, repository)
	require.NoError(testInstance, planError)

	require.False(testInstance, plan.skipped)
	require.Empty(testInstance, plan.fileChanges)
	require.Nil(testInstance, plan.pullRequest)
	require.Len(testInstance, plan.actions, 1)
	action := plan.actions[0]
	require.Equal(testInstance, "repo.remote.update", action.actionType)
	require.Equal(testInstance, "octocat", action.parameters["owner"])
	require.Equal(testInstance, true, action.parameters["dryrun"])
}

func TestTaskExecutorExecuteActionsUnknownType(testInstance *testing.T) {
	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	environment := &Environment{DryRun: true}
	plan := taskPlan{actions: []taskAction{{actionType: "unknown.action", parameters: map[string]any{}}}}
	executor := newTaskExecutor(environment, repository, plan)

	executionError := executor.executeActions(context.Background())
	require.Error(testInstance, executionError)
}

func TestTaskExecutorExecuteActionsCanonicalRemote(testInstance *testing.T) {
	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:                "/repositories/sample",
		OriginOwnerRepo:     "octocat/sample",
		CanonicalOwnerRepo:  "github/sample",
		RemoteDefaultBranch: "main",
	})
	environment := &Environment{DryRun: true}
	plan := taskPlan{actions: []taskAction{{actionType: taskActionCanonicalRemote, parameters: map[string]any{}}}}
	executor := newTaskExecutor(environment, repository, plan)

	executionError := executor.executeActions(context.Background())
	require.NoError(testInstance, executionError)
}

func TestTaskExecutorExecuteActionsRelease(testInstance *testing.T) {
	gitExecutor := &recordingGitExecutor{}
	outputBuffer := &bytes.Buffer{}
	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	environment := &Environment{GitExecutor: gitExecutor, Output: outputBuffer}
	actionParameters := map[string]any{
		"tag":     "v1.2.3",
		"message": "Release v1.2.3",
		"remote":  "origin",
	}
	plan := taskPlan{actions: []taskAction{{actionType: taskActionReleaseTag, parameters: actionParameters}}}
	executor := newTaskExecutor(environment, repository, plan)

	executionError := executor.executeActions(context.Background())
	require.NoError(testInstance, executionError)
	require.Len(testInstance, gitExecutor.commands, 2)
	expectedMessage := fmt.Sprintf(releaseActionMessageTemplate+"\n", repository.Path, "v1.2.3")
	require.Equal(testInstance, expectedMessage, outputBuffer.String())
}

func TestTaskExecutorExecuteActionsReleaseRequiresTag(testInstance *testing.T) {
	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	environment := &Environment{}
	plan := taskPlan{actions: []taskAction{{actionType: taskActionReleaseTag, parameters: map[string]any{}}}}
	executor := newTaskExecutor(environment, repository, plan)

	executionError := executor.executeActions(context.Background())
	require.Error(testInstance, executionError)
	require.Contains(testInstance, executionError.Error(), "release action requires 'tag'")
}

func TestTaskExecutorExecuteActionsBranchCleanup(testInstance *testing.T) {
	originalHandler, handlerExists := taskActionHandlers["repo.branches.cleanup"]
	RegisterTaskAction("repo.branches.cleanup", testBranchCleanupHandler)
	defer func() {
		if handlerExists {
			taskActionHandlers["repo.branches.cleanup"] = originalHandler
		} else {
			delete(taskActionHandlers, "repo.branches.cleanup")
		}
	}()

	executor := &branchCleanupExecutor{}
	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	environment := &Environment{GitExecutor: executor}
	actionParameters := map[string]any{
		"remote": "origin",
		"limit":  "25",
	}
	plan := taskPlan{actions: []taskAction{{actionType: "repo.branches.cleanup", parameters: actionParameters}}}
	taskExecutor := newTaskExecutor(environment, repository, plan)

	executionError := taskExecutor.executeActions(context.Background())
	require.NoError(testInstance, executionError)
	require.NotEmpty(testInstance, executor.gitCommands)
	require.NotEmpty(testInstance, executor.githubCommands)
	require.Equal(testInstance, "ls-remote", firstArgument(executor.gitCommands[0]))
	require.Equal(testInstance, "pr", firstArgument(executor.githubCommands[0]))
}

func TestTaskExecutorExecuteActionsOnlyDoesNotEmitApplyLog(testInstance *testing.T) {
	const actionType = "test.action.only"

	originalHandler, handlerExists := taskActionHandlers[actionType]
	RegisterTaskAction(actionType, func(context.Context, *Environment, *RepositoryState, map[string]any) error {
		return nil
	})
	defer func() {
		if handlerExists {
			taskActionHandlers[actionType] = originalHandler
		} else {
			delete(taskActionHandlers, actionType)
		}
	}()

	output := &bytes.Buffer{}
	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	environment := &Environment{Output: output}
	plan := taskPlan{
		task: TaskDefinition{Name: "Actions Only"},
		actions: []taskAction{
			{actionType: actionType, parameters: map[string]any{}},
		},
		repository: repository,
	}
	executor := newTaskExecutor(environment, repository, plan)

	executionError := executor.Execute(context.Background())
	require.NoError(testInstance, executionError)
	require.NotContains(testInstance, output.String(), taskLogPrefixApply)
}

func TestTaskExecutorExecuteActionsBranchCleanupRequiresRemote(testInstance *testing.T) {
	originalHandler, handlerExists := taskActionHandlers["repo.branches.cleanup"]
	RegisterTaskAction("repo.branches.cleanup", testBranchCleanupHandler)
	defer func() {
		if handlerExists {
			taskActionHandlers["repo.branches.cleanup"] = originalHandler
		} else {
			delete(taskActionHandlers, "repo.branches.cleanup")
		}
	}()

	executor := &branchCleanupExecutor{}
	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	environment := &Environment{GitExecutor: executor}
	plan := taskPlan{actions: []taskAction{{actionType: "repo.branches.cleanup", parameters: map[string]any{}}}}
	taskExecutor := newTaskExecutor(environment, repository, plan)

	executionError := taskExecutor.executeActions(context.Background())
	require.Error(testInstance, executionError)
	require.Contains(testInstance, executionError.Error(), "branch cleanup action requires 'remote'")
}

func TestTaskPlannerSkipWhenFileUnchanged(testInstance *testing.T) {
	repositoryPath := "/repositories/sample"
	existingContent := []byte("Repository: octocat/sample")
	fileSystem := newFakeFileSystem(map[string][]byte{
		filepath.Join(repositoryPath, "docs/sample.md"): existingContent,
	})
	environment := &Environment{FileSystem: fileSystem}

	inspection := audit.RepositoryInspection{
		Path:                repositoryPath,
		FinalOwnerRepo:      "octocat/sample",
		RemoteDefaultBranch: "main",
	}
	repository := NewRepositoryState(inspection)

	taskDefinition := TaskDefinition{
		Name: "Add Docs",
		Branch: TaskBranchDefinition{
			NameTemplate: "feature/{{ .Repository.Name }}",
		},
		Files: []TaskFileDefinition{{
			PathTemplate:    "docs/{{ .Repository.Name }}.md",
			ContentTemplate: "Repository: {{ .Repository.FullName }}",
			Mode:            taskFileModeOverwrite,
			Permissions:     defaultTaskFilePermissions,
		}},
		Commit: TaskCommitDefinition{},
	}

	templateData := buildTaskTemplateData(repository, taskDefinition)
	planner := newTaskPlanner(taskDefinition, templateData)

	plan, planError := planner.BuildPlan(environment, repository)
	require.NoError(testInstance, planError)

	require.True(testInstance, plan.skipped)
	require.Equal(testInstance, "no changes", plan.skipReason)
	require.Len(testInstance, plan.fileChanges, 1)
	require.False(testInstance, plan.fileChanges[0].apply)
	require.Equal(testInstance, "unchanged", plan.fileChanges[0].skipReason)
}

func TestTaskExecutorSkipsWhenBranchExists(testInstance *testing.T) {
	gitExecutor := &recordingGitExecutor{
		branchExists:  true,
		worktreeClean: true,
		currentBranch: "master",
	}
	fileSystem := newFakeFileSystem(nil)

	repositoryManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(testInstance, managerError)

	githubClient, clientError := githubcli.NewClient(gitExecutor)
	require.NoError(testInstance, clientError)

	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:                "/repositories/sample",
		FinalOwnerRepo:      "octocat/sample",
		RemoteDefaultBranch: "main",
	})

	taskDefinition := TaskDefinition{
		Name: "Add Docs",
		Files: []TaskFileDefinition{{
			PathTemplate:    "docs/{{ .Repository.Name }}.md",
			ContentTemplate: "Repository: {{ .Repository.FullName }}",
			Mode:            taskFileModeOverwrite,
			Permissions:     defaultTaskFilePermissions,
		}},
		Commit: TaskCommitDefinition{},
	}

	templateData := buildTaskTemplateData(repository, taskDefinition)
	planner := newTaskPlanner(taskDefinition, templateData)

	environment := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repositoryManager,
		GitHubClient:      githubClient,
		FileSystem:        fileSystem,
	}

	plan, planError := planner.BuildPlan(environment, repository)
	require.NoError(testInstance, planError)

	executor := newTaskExecutor(environment, repository, plan)

	executionError := executor.Execute(context.Background())
	require.NoError(testInstance, executionError)

	require.Len(testInstance, fileSystem.files, 0)
	for commandIndex := range gitExecutor.commands {
		command := gitExecutor.commands[commandIndex].Arguments
		require.NotEqual(testInstance, "checkout", firstArgument(command))
		require.NotEqual(testInstance, "add", firstArgument(command))
		require.NotEqual(testInstance, "commit", firstArgument(command))
		require.NotEqual(testInstance, "push", firstArgument(command))
	}
}

func TestTaskExecutorAppliesChanges(testInstance *testing.T) {
	gitExecutor := &recordingGitExecutor{
		worktreeClean: true,
		currentBranch: "master",
	}
	fileSystem := newFakeFileSystem(nil)

	repositoryManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(testInstance, managerError)

	githubClient, clientError := githubcli.NewClient(gitExecutor)
	require.NoError(testInstance, clientError)

	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:                "/repositories/sample",
		FinalOwnerRepo:      "octocat/sample",
		RemoteDefaultBranch: "main",
		LocalBranch:         "master",
	})

	taskDefinition := TaskDefinition{
		Name:        "Add Docs",
		EnsureClean: true,
		Branch: TaskBranchDefinition{
			NameTemplate: "feature/{{ .Repository.Name }}-docs",
			PushRemote:   defaultTaskPushRemote,
		},
		Files: []TaskFileDefinition{{
			PathTemplate:    "docs/{{ .Repository.Name }}.md",
			ContentTemplate: "Repository: {{ .Repository.FullName }}",
			Mode:            taskFileModeOverwrite,
			Permissions:     defaultTaskFilePermissions,
		}},
		Commit: TaskCommitDefinition{
			MessageTemplate: "docs: update {{ .Task.Name }}",
		},
	}

	templateData := buildTaskTemplateData(repository, taskDefinition)
	planner := newTaskPlanner(taskDefinition, templateData)

	environment := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repositoryManager,
		GitHubClient:      githubClient,
		FileSystem:        fileSystem,
	}

	plan, planError := planner.BuildPlan(environment, repository)
	require.NoError(testInstance, planError)
	require.False(testInstance, plan.skipped)

	executor := newTaskExecutor(environment, repository, plan)

	executionError := executor.Execute(context.Background())
	require.NoError(testInstance, executionError)

	expectedPath := filepath.Join(repository.Path, "docs/sample.md")
	require.Equal(testInstance, []byte("Repository: octocat/sample"), fileSystem.files[expectedPath])

	expectedCommands := [][]string{
		{"status", "--porcelain"},
		{"rev-parse", "--verify", "feature-sample-docs"},
		{"rev-parse", "--abbrev-ref", "HEAD"},
		{"checkout", "main"},
		{"checkout", "-B", "feature-sample-docs", "main"},
		{"add", "docs/sample.md"},
		{"commit", "-m", "docs: update Add Docs"},
		{"push", "--set-upstream", "origin", "feature-sample-docs"},
		{"checkout", "master"},
	}

	collected := make([][]string, 0, len(gitExecutor.commands))
	for commandIndex := range gitExecutor.commands {
		collected = append(collected, gitExecutor.commands[commandIndex].Arguments)
	}
	require.Equal(testInstance, expectedCommands, collected)
}

func TestTaskExecutorSkipsWhenSafeguardFails(testInstance *testing.T) {
	gitExecutor := &recordingGitExecutor{
		worktreeClean: true,
		currentBranch: "feature/demo",
	}
	fileSystem := newFakeFileSystem(nil)

	repositoryManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(testInstance, managerError)

	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})

	environment := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repositoryManager,
		FileSystem:        fileSystem,
		Output:            &bytes.Buffer{},
	}

	task := TaskDefinition{
		Name:       "Guarded Task",
		Safeguards: map[string]any{"branch": "main"},
		Actions: []TaskActionDefinition{
			{Type: taskActionFileReplace, Options: map[string]any{"pattern": "*.md", "find": "foo", "replace": "bar"}},
		},
	}

	executionError := (&TaskOperation{tasks: []TaskDefinition{task}}).executeTask(context.Background(), environment, repository, task)
	require.NoError(testInstance, executionError)
	require.Contains(testInstance, environment.Output.(*bytes.Buffer).String(), "TASK-SKIP")
	require.Contains(testInstance, environment.Output.(*bytes.Buffer).String(), "requires branch main")
}

type fakeFileSystem struct {
	files map[string][]byte
}

func newFakeFileSystem(initial map[string][]byte) *fakeFileSystem {
	files := map[string][]byte{}
	for path, data := range initial {
		files[path] = append([]byte(nil), data...)
	}
	return &fakeFileSystem{files: files}
}

func (system *fakeFileSystem) Stat(path string) (fs.FileInfo, error) {
	data, exists := system.files[path]
	if !exists {
		return nil, fs.ErrNotExist
	}
	return fakeFileInfo{name: filepath.Base(path), size: int64(len(data))}, nil
}

func (system *fakeFileSystem) Rename(oldPath string, newPath string) error {
	data, exists := system.files[oldPath]
	if !exists {
		return fs.ErrNotExist
	}
	system.files[newPath] = append([]byte(nil), data...)
	delete(system.files, oldPath)
	return nil
}

func (system *fakeFileSystem) Abs(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	return filepath.Abs(path)
}

func (system *fakeFileSystem) MkdirAll(path string, permissions fs.FileMode) error {
	return nil
}

func (system *fakeFileSystem) ReadFile(path string) ([]byte, error) {
	data, exists := system.files[path]
	if !exists {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (system *fakeFileSystem) WriteFile(path string, data []byte, permissions fs.FileMode) error {
	system.files[path] = append([]byte(nil), data...)
	return nil
}

type fakeFileInfo struct {
	name string
	size int64
}

func (info fakeFileInfo) Name() string      { return info.name }
func (info fakeFileInfo) Size() int64       { return info.size }
func (info fakeFileInfo) Mode() fs.FileMode { return 0 }
func (info fakeFileInfo) ModTime() time.Time {
	return time.Time{}
}
func (info fakeFileInfo) IsDir() bool { return false }
func (info fakeFileInfo) Sys() any    { return nil }

type recordingGitExecutor struct {
	commands       []execshell.CommandDetails
	githubCommands []execshell.CommandDetails
	branchExists   bool
	worktreeClean  bool
	currentBranch  string
}

func (executor *recordingGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	if len(details.Arguments) == 0 {
		return execshell.ExecutionResult{}, nil
	}

	switch details.Arguments[0] {
	case "status":
		if executor.worktreeClean {
			return execshell.ExecutionResult{StandardOutput: ""}, nil
		}
		return execshell.ExecutionResult{StandardOutput: " M file.txt"}, nil
	case "rev-parse":
		if len(details.Arguments) >= 2 {
			switch details.Arguments[1] {
			case "--verify":
				if executor.branchExists {
					return execshell.ExecutionResult{}, nil
				}
				return execshell.ExecutionResult{}, execshell.CommandFailedError{
					Command: execshell.ShellCommand{Name: execshell.CommandGit, Details: details},
					Result:  execshell.ExecutionResult{ExitCode: 1},
				}
			case "--abbrev-ref":
				branch := executor.currentBranch
				if len(branch) == 0 {
					branch = "master"
				}
				return execshell.ExecutionResult{StandardOutput: branch}, nil
			}
		}
	}

	return execshell.ExecutionResult{}, nil
}

func (executor *recordingGitExecutor) ExecuteGitHubCLI(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.githubCommands = append(executor.githubCommands, details)
	return execshell.ExecutionResult{}, nil
}

func firstArgument(arguments []string) string {
	if len(arguments) == 0 {
		return ""
	}
	return arguments[0]
}

type branchCleanupExecutor struct {
	gitCommands    [][]string
	githubCommands [][]string
}

func (executor *branchCleanupExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.gitCommands = append(executor.gitCommands, append([]string{}, details.Arguments...))
	if len(details.Arguments) > 0 && details.Arguments[0] == "ls-remote" {
		return execshell.ExecutionResult{StandardOutput: "", ExitCode: 0}, nil
	}
	return execshell.ExecutionResult{ExitCode: 0}, nil
}

func (executor *branchCleanupExecutor) ExecuteGitHubCLI(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.githubCommands = append(executor.githubCommands, append([]string{}, details.Arguments...))
	if len(details.Arguments) > 0 && details.Arguments[0] == "pr" {
		return execshell.ExecutionResult{StandardOutput: "[]", ExitCode: 0}, nil
	}
	return execshell.ExecutionResult{ExitCode: 0}, nil
}

func testBranchCleanupHandler(ctx context.Context, environment *Environment, repository *RepositoryState, parameters map[string]any) error {
	if environment == nil || repository == nil {
		return nil
	}

	remoteValue, remoteExists := parameters["remote"]
	remote := strings.TrimSpace(fmt.Sprint(remoteValue))
	if !remoteExists || len(remote) == 0 || remote == "<nil>" {
		return errors.New("branch cleanup action requires 'remote'")
	}

	if environment.GitExecutor != nil {
		_, _ = environment.GitExecutor.ExecuteGit(ctx, execshell.CommandDetails{Arguments: []string{"ls-remote", "--heads", remote}})
		_, _ = environment.GitExecutor.ExecuteGitHubCLI(ctx, execshell.CommandDetails{Arguments: []string{"pr", "list"}})
	}

	return nil
}
