package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/gix/internal/commitmsg"
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/filesystem"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/pkg/llm"
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

	templateData := buildTaskTemplateData(repository, taskDefinition, nil)
	planner := newTaskPlanner(taskDefinition, templateData)

	plan, planError := planner.BuildPlan(environment, repository)
	require.NoError(testInstance, planError)
	testInstance.Logf("plan branch=%q", plan.branchName)

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

func TestTaskPlannerBuildPlanAppliesEnvironmentVariables(t *testing.T) {
	fileSystem := newFakeFileSystem(nil)
	environment := &Environment{FileSystem: fileSystem}

	inspection := audit.RepositoryInspection{
		Path:                "/repositories/sample",
		FinalOwnerRepo:      "octocat/sample",
		RemoteDefaultBranch: "main",
	}
	repository := NewRepositoryState(inspection)

	taskDefinition := TaskDefinition{
		Name:        "UseCapturedOutput",
		EnsureClean: true,
		Files: []TaskFileDefinition{{
			PathTemplate:    "README.md",
			ContentTemplate: "{{ index .Environment \"captured_message\" }}",
			Mode:            TaskFileModeOverwrite,
			Permissions:     defaultTaskFilePermissions,
		}},
		Commit: TaskCommitDefinition{
			MessageTemplate: "{{ index .Environment \"captured_message\" }}",
		},
	}

	variables := map[string]string{"captured_message": "feat: captured message"}
	templateData := buildTaskTemplateData(repository, taskDefinition, variables)
	planner := newTaskPlanner(taskDefinition, templateData)

	plan, planError := planner.BuildPlan(environment, repository)
	require.NoError(t, planError)
	require.Equal(t, "feat: captured message", plan.commitMessage)
	require.Len(t, plan.fileChanges, 1)
	require.Equal(t, []byte("feat: captured message"), plan.fileChanges[0].content)
}

func TestPlanFileChangesReplaceMode(t *testing.T) {
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "go.mod")
	require.NoError(t, os.WriteFile(filePath, []byte("module github.com/temirov/example\n"), 0o644))

	repository := &RepositoryState{Path: tempDir}
	taskDefinition := TaskDefinition{
		Files: []TaskFileDefinition{{
			PathTemplate: "go.mod",
			Mode:         taskFileModeReplace,
			Replacements: []TaskReplacementDefinition{
				{FromTemplate: "github.com/temirov", ToTemplate: "github.com/tyemirov"},
			},
		}},
	}

	environment := &Environment{FileSystem: filesystem.OSFileSystem{}}
	templateData := buildTaskTemplateData(repository, taskDefinition, nil)
	planner := newTaskPlanner(taskDefinition, templateData)

	changes, err := planner.planFileChanges(environment, repository)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.True(t, changes[0].apply)
	require.Contains(t, string(changes[0].content), "github.com/tyemirov/example")
}

func TestPlanFileChangesReplaceModeSupportsGlob(t *testing.T) {
	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "pkg"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "pkg", "one.go"), []byte(`package pkg
import "github.com/temirov/foo"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "pkg", "two.go"), []byte(`package pkg
import "github.com/temirov/bar"
`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, "pkg", "nested", "inner"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, "pkg", "nested", "inner", "three.go"), []byte(`package pkg
import "github.com/temirov/baz"
`), 0o644))

	repository := &RepositoryState{Path: tempDir}
	taskDefinition := TaskDefinition{
		Files: []TaskFileDefinition{{
			PathTemplate: "**/*.go",
			Mode:         taskFileModeReplace,
			Replacements: []TaskReplacementDefinition{
				{FromTemplate: "github.com/temirov", ToTemplate: "github.com/tyemirov"},
			},
		}},
	}

	environment := &Environment{FileSystem: filesystem.OSFileSystem{}}
	templateData := buildTaskTemplateData(repository, taskDefinition, nil)
	planner := newTaskPlanner(taskDefinition, templateData)

	changes, err := planner.planFileChanges(environment, repository)
	require.NoError(t, err)
	require.Len(t, changes, 3)
	paths := make([]string, 0, len(changes))
	for _, change := range changes {
		require.True(t, change.apply)
		require.Contains(t, string(change.content), "github.com/tyemirov")
		paths = append(paths, change.relativePath)
	}
	require.ElementsMatch(t, []string{
		"pkg/one.go",
		"pkg/two.go",
		"pkg/nested/inner/three.go",
	}, paths)
}

func TestBuildTaskOperationInjectsLLMConfiguration(t *testing.T) {
	options := map[string]any{
		optionTaskLLMKeyConstant: map[string]any{
			optionTaskLLMModelKeyConstant:     "gpt-test",
			optionTaskLLMAPIKeyEnvKeyConstant: "WORKFLOW_TEST_KEY",
		},
		optionTasksKeyConstant: []any{
			map[string]any{
				optionTaskNameKeyConstant:        "Generate Message",
				optionTaskEnsureCleanKeyConstant: false,
				optionTaskActionsKeyConstant: []any{
					map[string]any{
						optionTaskActionTypeKeyConstant:    taskActionCommitMessage,
						optionTaskActionOptionsKeyConstant: map[string]any{commitOptionDiffSource: string(commitmsg.DiffSourceStaged)},
					},
				},
			},
		},
	}

	operation, buildErr := buildTaskOperation(options)
	require.NoError(t, buildErr)

	taskOperation, ok := operation.(*TaskOperation)
	require.True(t, ok)
	require.NotNil(t, taskOperation.llmConfiguration)

	actionOptions := taskOperation.tasks[0].Actions[0].Options
	injected, exists := actionOptions[commitOptionClient]
	require.True(t, exists)
	_, ok = injected.(*TaskLLMClientConfiguration)
	require.True(t, ok)
}

func TestCommitMessageActionCapturesOutput(t *testing.T) {
	executor := &stubLLMGitExecutor{responses: map[string]string{
		"status --short":                   " M main.go\n",
		"diff --unified=3 --stat --cached": " main.go | 1 +\n",
		"diff --unified=3 --cached":        "diff --git a/main.go b/main.go\n",
	}}
	environment := &Environment{
		GitExecutor: executor,
		Output:      &bytes.Buffer{},
		Variables:   NewVariableStore(),
	}
	repository := &RepositoryState{Path: "/repositories/sample"}
	client := &stubCommitChatClient{response: "feat: capture commit"}
	parameters := map[string]any{
		commitOptionDiffSource:     string(commitmsg.DiffSourceStaged),
		commitOptionClient:         client,
		taskActionCaptureOptionKey: "generated_commit",
	}

	executionErr := handleCommitMessageAction(context.Background(), environment, repository, parameters)
	require.NoError(t, executionErr)
	require.Equal(t, 1, client.calls)

	name, nameErr := NewVariableName("generated_commit")
	require.NoError(t, nameErr)
	value, exists := environment.Variables.Get(name)
	require.True(t, exists)
	require.Equal(t, "feat: capture commit", value)
}

func TestCommitMessageActionPreservesUserProvidedVariable(t *testing.T) {
	executor := &stubLLMGitExecutor{responses: map[string]string{
		"status --short":                   " M main.go\n",
		"diff --unified=3 --stat --cached": " main.go | 1 +\n",
		"diff --unified=3 --cached":        "diff --git a/main.go b/main.go\n",
	}}
	environment := &Environment{
		GitExecutor: executor,
		Output:      &bytes.Buffer{},
		Variables:   NewVariableStore(),
	}
	name, err := NewVariableName("generated_commit")
	require.NoError(t, err)
	environment.Variables.Seed(name, "feat: user provided")

	parameters := map[string]any{
		commitOptionDiffSource:     string(commitmsg.DiffSourceStaged),
		commitOptionClient:         &stubCommitChatClient{response: "feat: capture commit"},
		taskActionCaptureOptionKey: "generated_commit",
	}

	executionErr := handleCommitMessageAction(context.Background(), environment, &RepositoryState{Path: "/tmp/repos/demo"}, parameters)
	require.NoError(t, executionErr)

	value, exists := environment.Variables.Get(name)
	require.True(t, exists)
	require.Equal(t, "feat: user provided", value)
}

func TestTaskExecutorSkipsPushWhenRemoteMissing(t *testing.T) {
	gitExecutor := &recordingGitExecutor{
		remoteURLs:    map[string]string{"origin": ""},
		worktreeClean: true,
		currentBranch: "main",
		existingRefs:  map[string]bool{"main": true},
	}
	repositoryManager, managerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerErr)

	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:                "/repositories/sample",
		RemoteDefaultBranch: "main",
	})

	fileSystem := newFakeFileSystem(nil)
	plan := planForTask(t, &Environment{FileSystem: fileSystem}, repository, TaskDefinition{
		Name: "Apply file",
		Branch: TaskBranchDefinition{
			NameTemplate: "feature/{{ .Repository.Name }}",
			PushRemote:   "origin",
		},
		Files: []TaskFileDefinition{{
			PathTemplate:    "docs/sample.md",
			ContentTemplate: "Hello world",
			Mode:            TaskFileModeOverwrite,
			Permissions:     defaultTaskFilePermissions,
		}},
		Commit: TaskCommitDefinition{MessageTemplate: "docs: seed"},
	})

	output := &bytes.Buffer{}
	reporter := shared.NewStructuredReporter(output, output, shared.WithRepositoryHeaders(false))
	environment := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repositoryManager,
		FileSystem:        fileSystem,
		Reporter:          reporter,
	}

	executor := newTaskExecutor(environment, repository, plan)
	require.ErrorIs(t, executor.Execute(context.Background()), errRepositorySkipped)

	for _, command := range gitExecutor.commands {
		require.NotEqual(t, "push", firstArgument(command.Arguments))
	}
	require.Contains(t, output.String(), "remote missing")
}

func TestTaskExecutorSkipsPushWhenRemoteLookupFails(t *testing.T) {
	gitExecutor := &recordingGitExecutor{
		remoteErrors: map[string]error{
			"origin": execshell.CommandFailedError{
				Command: execshell.ShellCommand{Name: execshell.CommandGit},
				Result:  execshell.ExecutionResult{ExitCode: 128},
			},
		},
		worktreeClean: true,
		currentBranch: "main",
		existingRefs:  map[string]bool{"main": true},
	}
	repositoryManager, managerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerErr)

	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:                "/repositories/sample",
		RemoteDefaultBranch: "main",
	})

	fileSystem := newFakeFileSystem(nil)
	plan := planForTask(t, &Environment{FileSystem: fileSystem}, repository, TaskDefinition{
		Name: "Apply file",
		Branch: TaskBranchDefinition{
			NameTemplate: "feature/{{ .Repository.Name }}",
			PushRemote:   "origin",
		},
		Files: []TaskFileDefinition{{
			PathTemplate:    "docs/sample.md",
			ContentTemplate: "Hello world",
			Mode:            TaskFileModeOverwrite,
			Permissions:     defaultTaskFilePermissions,
		}},
		Commit: TaskCommitDefinition{MessageTemplate: "docs: seed"},
	})

	output := &bytes.Buffer{}
	reporter := shared.NewStructuredReporter(output, output, shared.WithRepositoryHeaders(false))
	environment := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repositoryManager,
		FileSystem:        fileSystem,
		Reporter:          reporter,
	}

	executor := newTaskExecutor(environment, repository, plan)
	require.ErrorIs(t, executor.Execute(context.Background()), errRepositorySkipped)

	for _, command := range gitExecutor.commands {
		require.NotEqual(t, "push", firstArgument(command.Arguments))
	}
	require.Contains(t, output.String(), "remote lookup failed")
}

func TestTaskOperationSkipsDuplicateRepositories(t *testing.T) {
	t.Parallel()

	executor := &stubLLMGitExecutor{responses: map[string]string{
		"describe --tags --abbrev=0": "v0.9.0\n",
		"log --no-merges --date=short --pretty=format:%h %ad %an %s --max-count=200 v0.9.0..HEAD": "abc123 2025-10-07 Alice Add feature\n",
		"diff --stat v0.9.0..HEAD":      " internal/app.go | 5 ++++-\n",
		"diff --unified=3 v0.9.0..HEAD": "diff --git a/internal/app.go b/internal/app.go\n",
	}}

	client := &stubChangelogChatClient{response: "## [v1.0.0]\n\n### Features ✨\n- Highlight"}

	environment := &Environment{
		GitExecutor: executor,
		Output:      &bytes.Buffer{},
	}

	task := TaskDefinition{
		Name:        "Generate changelog section",
		EnsureClean: false,
		Actions: []TaskActionDefinition{
			{
				Type: taskActionChangelog,
				Options: map[string]any{
					changelogOptionClient:  client,
					changelogOptionVersion: "v1.0.0",
				},
			},
		},
	}

	repository := &RepositoryState{Path: "/repositories/sample"}
	duplicate := &RepositoryState{Path: "/repositories/sample"}
	state := &State{Repositories: []*RepositoryState{repository, duplicate}}

	executionError := (&TaskOperation{tasks: []TaskDefinition{task}}).Execute(context.Background(), environment, state)
	require.NoError(t, executionError)
	require.Equal(t, 1, client.calls)
}

func TestTaskOperationFallsBackWhenStartPointMissing(t *testing.T) {
	t.Parallel()

	gitExecutor := &recordingGitExecutor{
		branchExists:  false,
		worktreeClean: true,
		currentBranch: "main",
	}
	fileSystem := newFakeFileSystem(nil)
	repositoryManager, managerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerErr)

	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:                "/repositories/sample",
		FinalOwnerRepo:      "octocat/sample",
		RemoteDefaultBranch: "master",
		LocalBranch:         "main",
	})

	task := TaskDefinition{
		Name:        "Rewrite Namespace",
		EnsureClean: false,
		Files: []TaskFileDefinition{{
			PathTemplate:    "README.md",
			ContentTemplate: "updated",
			Mode:            taskFileModeOverwrite,
			Permissions:     defaultTaskFilePermissions,
		}},
	}

	planner := newTaskPlanner(task, buildTaskTemplateData(repository, task, nil))
	outputBuffer := &bytes.Buffer{}
	reporter := shared.NewStructuredReporter(outputBuffer, outputBuffer, shared.WithRepositoryHeaders(false))
	environment := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repositoryManager,
		FileSystem:        fileSystem,
		Output:            outputBuffer,
		Reporter:          reporter,
	}

	plan, planErr := planner.BuildPlan(environment, repository)
	require.NoError(t, planErr)
	require.Equal(t, "master", plan.startPoint)

	gitExecutor.existingRefs = map[string]bool{
		plan.branchName: false,
		plan.startPoint: false,
	}

	executor := newTaskExecutor(environment, repository, plan)
	executionErr := executor.Execute(context.Background())
	require.ErrorIs(t, executionErr, errRepositorySkipped)

	output := outputBuffer.String()
	require.Contains(t, output, "event=TASK_SKIP")
	require.Contains(t, output, "start point missing")

	for _, details := range gitExecutor.commands {
		require.NotEqual(t, []string{"checkout", "master"}, details.Arguments)
	}
}

func TestTaskExecutorLogsDirtyRepositoryDetails(t *testing.T) {
	fileSystem := newFakeFileSystem(nil)
	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	gitExecutor := &recordingGitExecutor{worktreeClean: false}
	repositoryManager, managerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerErr)
	taskDefinition := TaskDefinition{
		Name:        "Dirty Task",
		EnsureClean: true,
		Files: []TaskFileDefinition{{
			PathTemplate:    "README.md",
			ContentTemplate: "updated",
			Mode:            taskFileModeOverwrite,
			Permissions:     defaultTaskFilePermissions,
		}},
	}
	templateData := buildTaskTemplateData(repository, taskDefinition, nil)
	planner := newTaskPlanner(taskDefinition, templateData)
	plan, planErr := planner.BuildPlan(&Environment{FileSystem: fileSystem}, repository)
	require.NoError(t, planErr)
	outputBuffer := &bytes.Buffer{}
	reporter := shared.NewStructuredReporter(outputBuffer, outputBuffer, shared.WithRepositoryHeaders(false))
	environment := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repositoryManager,
		FileSystem:        fileSystem,
		Output:            outputBuffer,
		Reporter:          reporter,
	}
	executor := newTaskExecutor(environment, repository, plan)
	executionErr := executor.Execute(context.Background())
	require.ErrorIs(t, executionErr, errRepositorySkipped)
	output := outputBuffer.String()
	require.Contains(t, output, "event=TASK_SKIP")
	require.Contains(t, output, "repository dirty")
}

func TestTaskExecutorEnsureCleanVariableDisablesCleanCheck(t *testing.T) {
	fileSystem := newFakeFileSystem(nil)
	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	gitExecutor := &recordingGitExecutor{worktreeClean: false}
	repositoryManager, managerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerErr)

	taskDefinition := TaskDefinition{
		Name:                "Variable Clean Task",
		EnsureClean:         true,
		EnsureCleanVariable: "license_require_clean",
		Files: []TaskFileDefinition{{
			PathTemplate:    "LICENSE",
			ContentTemplate: "content",
			Mode:            taskFileModeOverwrite,
			Permissions:     defaultTaskFilePermissions,
		}},
	}

	plan := taskPlan{
		task:      taskDefinition,
		variables: map[string]string{"license_require_clean": "false"},
	}

	output := &bytes.Buffer{}
	reporter := shared.NewStructuredReporter(output, output, shared.WithRepositoryHeaders(false))
	environment := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repositoryManager,
		FileSystem:        fileSystem,
		Output:            output,
		Reporter:          reporter,
	}

	executor := newTaskExecutor(environment, repository, plan)
	executionErr := executor.Execute(context.Background())
	require.NoError(t, executionErr)

	for _, command := range gitExecutor.commands {
		require.NotEqual(t, "status", firstArgument(command.Arguments))
	}
}

func TestTaskExecutorResolveEnsureCleanRecognizesTrueValues(t *testing.T) {
	executor := taskExecutor{
		plan: taskPlan{
			task: TaskDefinition{
				EnsureClean:         false,
				EnsureCleanVariable: "require_clean",
			},
			variables: map[string]string{"require_clean": "YES"},
		},
	}

	require.True(t, executor.resolveEnsureClean())
}

func TestTaskExecutorResolveEnsureCleanIgnoresUnknownValues(t *testing.T) {
	executor := taskExecutor{
		plan: taskPlan{
			task: TaskDefinition{
				EnsureClean:         true,
				EnsureCleanVariable: "require_clean",
			},
			variables: map[string]string{"require_clean": "maybe"},
		},
	}

	require.True(t, executor.resolveEnsureClean())
}

func TestTaskExecutorRestoresOriginalBranchAfterApply(t *testing.T) {
	fileSystem := newFakeFileSystem(map[string][]byte{
		"/repositories/sample/README.md": []byte("original"),
	})
	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	gitExecutor := &recordingGitExecutor{
		worktreeClean: true,
		currentBranch: "main",
	}
	repositoryManager, managerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerErr)

	taskDefinition := TaskDefinition{
		Name:        "Apply Task",
		EnsureClean: true,
		Branch: TaskBranchDefinition{
			NameTemplate:       "feature/apply-task",
			StartPointTemplate: "main",
		},
		Files: []TaskFileDefinition{{
			PathTemplate:    "README.md",
			ContentTemplate: "updated",
			Mode:            TaskFileModeOverwrite,
			Permissions:     defaultTaskFilePermissions,
		}},
		Commit: TaskCommitDefinition{MessageTemplate: "apply task"},
	}
	plan := planForTask(t, &Environment{FileSystem: fileSystem}, repository, taskDefinition)

	output := &bytes.Buffer{}
	reporter := shared.NewStructuredReporter(output, output, shared.WithRepositoryHeaders(false))
	environment := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repositoryManager,
		FileSystem:        fileSystem,
		Output:            output,
		Reporter:          reporter,
	}

	executor := newTaskExecutor(environment, repository, plan)
	require.ErrorIs(t, executor.Execute(context.Background()), errRepositorySkipped)

	foundRestore := false
	for _, command := range gitExecutor.commands {
		if len(command.Arguments) == 2 && command.Arguments[0] == "checkout" && command.Arguments[1] == "main" {
			foundRestore = true
			break
		}
	}
	require.True(t, foundRestore, "expected checkout main command to restore original branch")
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
				"owner": "{{ .Repository.Owner }}",
			},
		}},
		Commit: TaskCommitDefinition{},
	}

	templateData := buildTaskTemplateData(repository, taskDefinition, nil)
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
	require.Len(testInstance, action.parameters, 1)
}

func TestTaskExecutorExecuteActionsUnknownType(testInstance *testing.T) {
	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	environment := &Environment{}
	plan := planForActions(testInstance, repository, []TaskActionDefinition{{Type: "unknown.action"}})
	executor := newTaskExecutor(environment, repository, plan)

	executionError := executor.Execute(context.Background())
	require.Error(testInstance, executionError)
}

func TestTaskExecutorExecuteActionsCanonicalRemote(testInstance *testing.T) {
	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:                "/repositories/sample",
		OriginOwnerRepo:     "octocat/sample",
		CanonicalOwnerRepo:  "github/sample",
		RemoteDefaultBranch: "main",
	})
	environment := &Environment{}
	plan := planForActions(testInstance, repository, []TaskActionDefinition{{Type: taskActionCanonicalRemote}})
	executor := newTaskExecutor(environment, repository, plan)

	executionError := executor.Execute(context.Background())
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
	plan := planForActions(testInstance, repository, []TaskActionDefinition{{
		Type:    taskActionReleaseTag,
		Options: actionParameters,
	}})
	executor := newTaskExecutor(environment, repository, plan)

	executionError := executor.Execute(context.Background())
	require.NoError(testInstance, executionError)
	require.Len(testInstance, gitExecutor.commands, 2)
	expectedMessage := fmt.Sprintf(releaseActionMessageTemplate+"\n", repository.Path, "v1.2.3")
	require.Equal(testInstance, expectedMessage, outputBuffer.String())
}

func TestTaskExecutorExecuteActionsReleaseRequiresTag(testInstance *testing.T) {
	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	environment := &Environment{}
	plan := planForActions(testInstance, repository, []TaskActionDefinition{{
		Type:    taskActionReleaseTag,
		Options: map[string]any{},
	}})
	executor := newTaskExecutor(environment, repository, plan)

	executionError := executor.Execute(context.Background())
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
	plan := planForActions(testInstance, repository, []TaskActionDefinition{{
		Type:    "repo.branches.cleanup",
		Options: actionParameters,
	}})
	taskExecutor := newTaskExecutor(environment, repository, plan)

	executionError := taskExecutor.Execute(context.Background())
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
	reporter := shared.NewStructuredReporter(output, output, shared.WithRepositoryHeaders(false))
	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	environment := &Environment{Output: output, Reporter: reporter}
	plan := planForActions(testInstance, repository, []TaskActionDefinition{{Type: actionType}})
	executor := newTaskExecutor(environment, repository, plan)

	executionError := executor.Execute(context.Background())
	require.NoError(testInstance, executionError)
	require.Contains(testInstance, output.String(), "event=TASK_APPLY")
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
	plan := planForActions(testInstance, repository, []TaskActionDefinition{{
		Type:    "repo.branches.cleanup",
		Options: map[string]any{},
	}})
	taskExecutor := newTaskExecutor(environment, repository, plan)

	executionError := taskExecutor.Execute(context.Background())
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

	templateData := buildTaskTemplateData(repository, taskDefinition, nil)
	planner := newTaskPlanner(taskDefinition, templateData)

	plan, planError := planner.BuildPlan(environment, repository)
	require.NoError(testInstance, planError)

	require.True(testInstance, plan.skipped)
	require.Equal(testInstance, "no changes", plan.skipReason)
	require.Len(testInstance, plan.fileChanges, 1)
	require.False(testInstance, plan.fileChanges[0].apply)
	require.Equal(testInstance, "unchanged", plan.fileChanges[0].skipReason)
}

func TestTaskPlannerSkipAppendIfMissingWhenAlreadyPresent(t *testing.T) {
	repositoryPath := "/repositories/ensure"
	fileSystem := newFakeFileSystem(map[string][]byte{
		filepath.Join(repositoryPath, ".gitignore"): []byte("# existing\n.env\n"),
	})
	environment := &Environment{FileSystem: fileSystem}

	repository := NewRepositoryState(audit.RepositoryInspection{Path: repositoryPath})
	taskDefinition := TaskDefinition{
		Name: "Ensure gitignore",
		Files: []TaskFileDefinition{{
			PathTemplate:    ".gitignore",
			ContentTemplate: "# existing\n.env\n",
			Mode:            TaskFileModeAppendIfMissing,
			Permissions:     defaultTaskFilePermissions,
		}},
	}

	planner := newTaskPlanner(taskDefinition, buildTaskTemplateData(repository, taskDefinition, nil))
	plan, planError := planner.BuildPlan(environment, repository)
	require.NoError(t, planError)
	require.True(t, plan.skipped)
	require.Equal(t, "no changes", plan.skipReason)
	require.Len(t, plan.fileChanges, 1)
	require.False(t, plan.fileChanges[0].apply)
	require.Equal(t, "lines-present", plan.fileChanges[0].skipReason)
}

func TestTaskPlannerExecutionStepsRestrictActions(t *testing.T) {
	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:                "/repositories/sample",
		FinalOwnerRepo:      "octocat/sample",
		RemoteDefaultBranch: "main",
	})
	fileSystem := newFakeFileSystem(nil)
	environment := &Environment{FileSystem: fileSystem}

	taskDefinition := TaskDefinition{
		Name: "Append gitignore",
		Steps: []taskExecutionStep{
			taskExecutionStepFilesApply,
		},
		Files: []TaskFileDefinition{{
			PathTemplate:    ".gitignore",
			ContentTemplate: ".env",
			Mode:            TaskFileModeAppendIfMissing,
			Permissions:     defaultTaskFilePermissions,
		}},
	}

	planner := newTaskPlanner(taskDefinition, buildTaskTemplateData(repository, taskDefinition, nil))
	plan, planErr := planner.BuildPlan(environment, repository)
	require.NoError(t, planErr)
	require.False(t, plan.skipped)
	require.Len(t, plan.workflowSteps, 1)
	require.Equal(t, "files.apply", plan.workflowSteps[0].Name())
}

func TestTaskPlannerExecutionStepsRequirePullRequestConfig(t *testing.T) {
	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:                "/repositories/sample",
		FinalOwnerRepo:      "octocat/sample",
		RemoteDefaultBranch: "main",
	})
	fileSystem := newFakeFileSystem(nil)
	environment := &Environment{FileSystem: fileSystem}

	taskDefinition := TaskDefinition{
		Name:  "Pull Request",
		Steps: []taskExecutionStep{taskExecutionStepPullRequest},
	}

	planner := newTaskPlanner(taskDefinition, buildTaskTemplateData(repository, taskDefinition, nil))
	_, planErr := planner.BuildPlan(environment, repository)
	require.Error(t, planErr)
	require.Contains(t, planErr.Error(), "pull_request configuration")
}

func TestTaskExecutorApplyAppendIfMissing(t *testing.T) {
	repositoryPath := "/repositories/ensure"
	fileSystem := newFakeFileSystem(map[string][]byte{
		filepath.Join(repositoryPath, ".gitignore"): []byte("# existing\n.env\n"),
	})
	environment := &Environment{FileSystem: fileSystem}
	repository := NewRepositoryState(audit.RepositoryInspection{Path: repositoryPath})

	taskDefinition := TaskDefinition{
		Name: "Ensure gitignore",
		Files: []TaskFileDefinition{{
			PathTemplate:    ".gitignore",
			ContentTemplate: "# existing\n.env\nbin/\n",
			Mode:            TaskFileModeAppendIfMissing,
			Permissions:     defaultTaskFilePermissions,
		}},
	}

	planner := newTaskPlanner(taskDefinition, buildTaskTemplateData(repository, taskDefinition, nil))
	plan, planError := planner.BuildPlan(environment, repository)
	require.NoError(t, planError)
	require.False(t, plan.skipped)
	require.Len(t, plan.fileChanges, 1)
	require.True(t, plan.fileChanges[0].apply)

	execCtx := &ExecutionContext{
		Environment: environment,
		Repository:  repository,
		Plan:        &plan,
	}
	action := filesApplyAction{changes: plan.fileChanges}
	require.NoError(t, action.Execute(context.Background(), execCtx))

	updated, readErr := fileSystem.ReadFile(filepath.Join(repositoryPath, ".gitignore"))
	require.NoError(t, readErr)
	contents := string(updated)
	require.Contains(t, contents, "bin/")
	require.Equal(t, 1, strings.Count(contents, "bin/"))
	require.Contains(t, contents, "# existing")
	require.Contains(t, contents, ".env")
}

func TestTaskExecutorAppendIfMissingAddsAllLinesWhenNoneExist(t *testing.T) {
	repositoryPath := "/repositories/ensure-all"
	fileSystem := newFakeFileSystem(map[string][]byte{
		filepath.Join(repositoryPath, ".gitignore"): []byte(""),
	})
	environment := &Environment{FileSystem: fileSystem}
	repository := NewRepositoryState(audit.RepositoryInspection{Path: repositoryPath})

	taskDefinition := TaskDefinition{
		Name: "Ensure gitignore block",
		Files: []TaskFileDefinition{{
			PathTemplate:    ".gitignore",
			ContentTemplate: "# Managed by gix gitignore workflow\n.env\ntools/\nbin/\n",
			Mode:            TaskFileModeAppendIfMissing,
			Permissions:     defaultTaskFilePermissions,
		}},
	}

	planner := newTaskPlanner(taskDefinition, buildTaskTemplateData(repository, taskDefinition, nil))
	plan, planError := planner.BuildPlan(environment, repository)
	require.NoError(t, planError)
	require.False(t, plan.skipped)
	require.Len(t, plan.fileChanges, 1)
	require.True(t, plan.fileChanges[0].apply)

	execCtx := &ExecutionContext{
		Environment: environment,
		Repository:  repository,
		Plan:        &plan,
	}
	action := filesApplyAction{changes: plan.fileChanges}
	require.NoError(t, action.Execute(context.Background(), execCtx))

	updated, readErr := fileSystem.ReadFile(filepath.Join(repositoryPath, ".gitignore"))
	require.NoError(t, readErr)
	require.Equal(t, "# Managed by gix gitignore workflow\n.env\ntools/\nbin/\n", string(updated))
}

func TestTaskExecutorAppendIfMissingAddsDotEnvWhenSimilarPatternExists(t *testing.T) {
	repositoryPath := "/repositories/dotenv-missing"
	fileSystem := newFakeFileSystem(map[string][]byte{
		filepath.Join(repositoryPath, ".gitignore"): []byte("# Managed by gix gitignore workflow\n.envrc\ntools/\nbin/\n"),
	})
	environment := &Environment{FileSystem: fileSystem}
	repository := NewRepositoryState(audit.RepositoryInspection{Path: repositoryPath})

	taskDefinition := TaskDefinition{
		Name: "Ensure gitignore block",
		Files: []TaskFileDefinition{{
			PathTemplate:    ".gitignore",
			ContentTemplate: "# Managed by gix gitignore workflow\n.env\ntools/\nbin/\n",
			Mode:            TaskFileModeAppendIfMissing,
			Permissions:     defaultTaskFilePermissions,
		}},
	}

	planner := newTaskPlanner(taskDefinition, buildTaskTemplateData(repository, taskDefinition, nil))
	plan, planError := planner.BuildPlan(environment, repository)
	require.NoError(t, planError)
	require.False(t, plan.skipped)
	require.Len(t, plan.fileChanges, 1)
	require.True(t, plan.fileChanges[0].apply)

	execCtx := &ExecutionContext{
		Environment: environment,
		Repository:  repository,
		Plan:        &plan,
	}
	action := filesApplyAction{changes: plan.fileChanges}
	require.NoError(t, action.Execute(context.Background(), execCtx))

	updated, readErr := fileSystem.ReadFile(filepath.Join(repositoryPath, ".gitignore"))
	require.NoError(t, readErr)
	contents := string(updated)
	require.Contains(t, contents, ".env\n")
	require.Contains(t, contents, ".envrc\n")
}

func TestTaskExecutorAppendIfMissingTreatsWhitespaceVariantsAsDistinct(t *testing.T) {
	repositoryPath := "/repositories/dotenv-whitespace"
	fileSystem := newFakeFileSystem(map[string][]byte{
		filepath.Join(repositoryPath, ".gitignore"): []byte("   .env   \n"),
	})
	environment := &Environment{FileSystem: fileSystem}
	repository := NewRepositoryState(audit.RepositoryInspection{Path: repositoryPath})

	taskDefinition := TaskDefinition{
		Name: "Ensure gitignore block",
		Files: []TaskFileDefinition{{
			PathTemplate:    ".gitignore",
			ContentTemplate: "# Managed by gix gitignore workflow\n.env\ntools/\nbin/\n",
			Mode:            TaskFileModeAppendIfMissing,
			Permissions:     defaultTaskFilePermissions,
		}},
	}

	planner := newTaskPlanner(taskDefinition, buildTaskTemplateData(repository, taskDefinition, nil))
	plan, planError := planner.BuildPlan(environment, repository)
	require.NoError(t, planError)
	require.False(t, plan.skipped)
	require.True(t, plan.fileChanges[0].apply)

	execCtx := &ExecutionContext{
		Environment: environment,
		Repository:  repository,
		Plan:        &plan,
	}
	action := filesApplyAction{changes: plan.fileChanges}
	require.NoError(t, action.Execute(context.Background(), execCtx))

	updated, readErr := fileSystem.ReadFile(filepath.Join(repositoryPath, ".gitignore"))
	require.NoError(t, readErr)
	require.Contains(t, string(updated), ".env\n")
	require.Contains(t, string(updated), "# Managed by gix gitignore workflow\n")
}

func TestTaskExecutorAppendIfMissingHandlesCarriageReturns(t *testing.T) {
	repositoryPath := "/repositories/ensure-carriage"
	fileSystem := newFakeFileSystem(map[string][]byte{
		filepath.Join(repositoryPath, ".gitignore"): []byte(""),
	})
	environment := &Environment{FileSystem: fileSystem}
	repository := NewRepositoryState(audit.RepositoryInspection{Path: repositoryPath})

	taskDefinition := TaskDefinition{
		Name: "Ensure gitignore CR block",
		Files: []TaskFileDefinition{{
			PathTemplate:    ".gitignore",
			ContentTemplate: "# Managed by gix gitignore workflow\r.env\rtools/\rbin/\r",
			Mode:            TaskFileModeAppendIfMissing,
			Permissions:     defaultTaskFilePermissions,
		}},
	}

	planner := newTaskPlanner(taskDefinition, buildTaskTemplateData(repository, taskDefinition, nil))
	plan, planError := planner.BuildPlan(environment, repository)
	require.NoError(t, planError)
	require.False(t, plan.skipped)
	require.Len(t, plan.fileChanges, 1)
	require.True(t, plan.fileChanges[0].apply)

	execCtx := &ExecutionContext{
		Environment: environment,
		Repository:  repository,
		Plan:        &plan,
	}
	action := filesApplyAction{changes: plan.fileChanges}
	require.NoError(t, action.Execute(context.Background(), execCtx))

	updated, readErr := fileSystem.ReadFile(filepath.Join(repositoryPath, ".gitignore"))
	require.NoError(t, readErr)
	require.Equal(t, "# Managed by gix gitignore workflow\n.env\ntools/\nbin/\n", string(updated))
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

	templateData := buildTaskTemplateData(repository, taskDefinition, nil)
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
	require.ErrorIs(testInstance, executionError, errRepositorySkipped)

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

	templateData := buildTaskTemplateData(repository, taskDefinition, nil)
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

	gitExecutor.existingRefs = map[string]bool{
		plan.branchName: false,
		plan.startPoint: true,
	}

	executor := newTaskExecutor(environment, repository, plan)

	executionError := executor.Execute(context.Background())
	require.NoError(testInstance, executionError)

	expectedPath := filepath.Join(repository.Path, "docs/sample.md")
	require.Equal(testInstance, []byte("Repository: octocat/sample"), fileSystem.files[expectedPath])

	collected := make([][]string, 0, len(gitExecutor.commands))
	for commandIndex := range gitExecutor.commands {
		collected = append(collected, gitExecutor.commands[commandIndex].Arguments)
	}

	require.Contains(testInstance, collected, []string{"status", "--porcelain"})
	require.Contains(testInstance, collected, []string{"checkout", "-B", "feature-sample-docs", "main"})
	require.Contains(testInstance, collected, []string{"add", "docs/sample.md"})
	require.Contains(testInstance, collected, []string{"commit", "-m", "docs: update Add Docs"})
	require.Contains(testInstance, collected, []string{"push", "--set-upstream", "origin", "feature-sample-docs"})
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

	output := &bytes.Buffer{}
	reporter := shared.NewStructuredReporter(output, output, shared.WithRepositoryHeaders(false))
	environment := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repositoryManager,
		FileSystem:        fileSystem,
		Output:            output,
		Reporter:          reporter,
	}

	task := TaskDefinition{
		Name:       "Guarded Task",
		Safeguards: map[string]any{"branch": "main"},
		Actions: []TaskActionDefinition{
			{Type: taskActionFileReplace, Options: map[string]any{"pattern": "*.md", "find": "foo", "replace": "bar"}},
		},
	}

	executionError := (&TaskOperation{tasks: []TaskDefinition{task}}).executeTask(context.Background(), environment, repository, task)
	require.ErrorIs(testInstance, executionError, errRepositorySkipped)
	require.Contains(testInstance, output.String(), "event=TASK_SKIP")
	require.Contains(testInstance, output.String(), "requires branch main")
}

func TestTaskExecutorSkipsSoftSafeguards(testInstance *testing.T) {
	gitExecutor := &recordingGitExecutor{
		worktreeClean: true,
		currentBranch: "feature/demo",
	}
	fileSystem := newFakeFileSystem(nil)

	repositoryManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(testInstance, managerError)

	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})

	output := &bytes.Buffer{}
	reporter := shared.NewStructuredReporter(output, output, shared.WithRepositoryHeaders(false))
	environment := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repositoryManager,
		FileSystem:        fileSystem,
		Output:            output,
		Reporter:          reporter,
	}

	task := TaskDefinition{
		Name: "Soft Guarded Task",
		Safeguards: map[string]any{
			"soft_skip": map[string]any{"branch": "main"},
		},
		Actions: []TaskActionDefinition{
			{Type: taskActionFileReplace, Options: map[string]any{"pattern": "*.md", "find": "foo", "replace": "bar"}},
		},
	}

	executionError := (&TaskOperation{tasks: []TaskDefinition{task}}).executeTask(context.Background(), environment, repository, task)
	require.NoError(testInstance, executionError)
	require.Contains(testInstance, output.String(), "event=TASK_SKIP")
	require.Contains(testInstance, output.String(), "requires branch main")
}

type stubLLMGitExecutor struct {
	responses map[string]string
}

func (executor *stubLLMGitExecutor) ExecuteGit(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	key := strings.Join(details.Arguments, " ")
	if output, ok := executor.responses[key]; ok {
		return execshell.ExecutionResult{StandardOutput: output}, nil
	}
	return execshell.ExecutionResult{}, nil
}

func (executor *stubLLMGitExecutor) ExecuteGitHubCLI(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type stubCommitChatClient struct {
	response string
	calls    int
}

func (client *stubCommitChatClient) Chat(ctx context.Context, request llm.ChatRequest) (string, error) {
	client.calls++
	return client.response, nil
}

type stubChangelogChatClient struct {
	response string
	calls    int
}

func (client *stubChangelogChatClient) Chat(ctx context.Context, request llm.ChatRequest) (string, error) {
	client.calls++
	return client.response, nil
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
	commands        []execshell.CommandDetails
	githubCommands  []execshell.CommandDetails
	branchExists    bool
	worktreeClean   bool
	worktreeEntries []string
	currentBranch   string
	existingRefs    map[string]bool
	remoteURLs      map[string]string
	remoteErrors    map[string]error
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
		if len(executor.worktreeEntries) > 0 {
			return execshell.ExecutionResult{StandardOutput: strings.Join(executor.worktreeEntries, "\n")}, nil
		}
		return execshell.ExecutionResult{StandardOutput: " M file.txt"}, nil
	case "rev-parse":
		if len(details.Arguments) >= 2 {
			switch details.Arguments[1] {
			case "--verify":
				target := ""
				if len(details.Arguments) >= 3 {
					target = details.Arguments[2]
				}
				if executor.existingRefs != nil {
					if exists, ok := executor.existingRefs[target]; ok {
						if exists {
							return execshell.ExecutionResult{}, nil
						}
						return execshell.ExecutionResult{}, execshell.CommandFailedError{
							Command: execshell.ShellCommand{Name: execshell.CommandGit, Details: details},
							Result:  execshell.ExecutionResult{ExitCode: 1},
						}
					}
				}
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
	case "remote":
		if len(details.Arguments) >= 3 && details.Arguments[1] == "get-url" {
			remoteName := details.Arguments[2]
			if executor.remoteErrors != nil {
				if remoteError, exists := executor.remoteErrors[remoteName]; exists {
					return execshell.ExecutionResult{}, remoteError
				}
			}
			if executor.remoteURLs != nil {
				if remoteURL, exists := executor.remoteURLs[remoteName]; exists {
					return execshell.ExecutionResult{StandardOutput: remoteURL + "\n"}, nil
				}
				return execshell.ExecutionResult{}, execshell.CommandFailedError{
					Command: execshell.ShellCommand{Name: execshell.CommandGit, Details: details},
					Result:  execshell.ExecutionResult{ExitCode: 1},
				}
			}
			return execshell.ExecutionResult{StandardOutput: "git@github.com:example/repo.git\n"}, nil
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

func planForTask(t *testing.T, environment *Environment, repository *RepositoryState, definition TaskDefinition) taskPlan {
	t.Helper()

	templateData := buildTaskTemplateData(repository, definition, nil)
	planner := newTaskPlanner(definition, templateData)
	plan, err := planner.BuildPlan(environment, repository)
	require.NoError(t, err)
	return plan
}

func planForActions(t *testing.T, repository *RepositoryState, actions []TaskActionDefinition) taskPlan {
	t.Helper()

	definition := TaskDefinition{
		Name:        "Actions Only",
		EnsureClean: false,
		Actions:     actions,
		Commit:      TaskCommitDefinition{},
	}
	return planForTask(t, nil, repository, definition)
}
