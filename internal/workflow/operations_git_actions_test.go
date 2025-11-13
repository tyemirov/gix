package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/audit"
	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/gitrepo"
)

func TestGitStageOperationStagesPaths(t *testing.T) {
	gitExecutor := &recordingGitExecutor{worktreeClean: true}
	op, buildErr := buildGitStageOperation(map[string]any{
		"paths": []any{"docs/POLICY.md", "README.md"},
	})
	require.NoError(t, buildErr)

	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:           "/repositories/sample",
		FinalOwnerRepo: "octocat/sample",
	})
	templateData := buildTaskTemplateData(repository, TaskDefinition{}, nil)
	rendered, _ := renderTemplateValue("feature/{{ .Repository.Name }}-docs", "", templateData)
	t.Logf("rendered branch template: %s", rendered)
	state := &State{Repositories: []*RepositoryState{repository}}
	env := &Environment{GitExecutor: gitExecutor}

	require.NoError(t, op.Execute(context.Background(), env, state))

	require.True(t, commandArgumentsExist(gitExecutor.commands, []string{"add", "docs/POLICY.md"}))
	require.True(t, commandArgumentsExist(gitExecutor.commands, []string{"add", "README.md"}))
}

func TestGitStageOperationRequiresCleanWorktree(t *testing.T) {
	gitExecutor := &recordingGitExecutor{
		worktreeClean: false,
		currentBranch: "main",
	}
	repoManager, managerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerErr)

	op, buildErr := buildGitStageOperation(map[string]any{
		"paths":        []any{"README.md"},
		"ensure_clean": true,
	})
	require.NoError(t, buildErr)

	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	state := &State{Repositories: []*RepositoryState{repository}}
	env := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repoManager,
	}

	require.NoError(t, op.Execute(context.Background(), env, state))
	require.False(t, commandArgumentsExist(gitExecutor.commands, []string{"add", "README.md"}))
}

func TestGitCommitOperationCommitsWithMessage(t *testing.T) {
	gitExecutor := &recordingGitExecutor{}
	op, buildErr := buildGitCommitOperation(map[string]any{
		"commit_message": "docs: update",
	})
	require.NoError(t, buildErr)

	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:                "/repositories/sample",
		FinalOwnerRepo:      "octocat/sample",
		RemoteDefaultBranch: "main",
	})
	state := &State{Repositories: []*RepositoryState{repository}}
	env := &Environment{GitExecutor: gitExecutor}

	require.NoError(t, op.Execute(context.Background(), env, state))

	require.True(t, commandArgumentsExist(gitExecutor.commands, []string{"commit", "-m", "docs: update"}))
}

func TestGitPushOperationPushesBranch(t *testing.T) {
	gitExecutor := &recordingGitExecutor{
		worktreeClean: true,
		currentBranch: "main",
		remoteURLs: map[string]string{
			"origin": "git@github.com:sample/repo.git",
		},
		existingRefs: map[string]bool{
			"feature-sample-docs": false,
		},
	}
	repoManager, managerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerErr)

	op, buildErr := buildGitPushOperation(map[string]any{
		"branch":      "feature/{{ .Repository.Name }}-docs",
		"push_remote": "origin",
	})
	require.NoError(t, buildErr)

	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:                "/repositories/sample",
		FinalOwnerRepo:      "octocat/sample",
		RemoteDefaultBranch: "main",
	})
	templateData := buildTaskTemplateData(repository, TaskDefinition{}, nil)
	rendered, _ := renderTemplateValue("feature/{{ .Repository.Name }}-docs", "", templateData)
	t.Logf("rendered branch template: %s", rendered)
	state := &State{Repositories: []*RepositoryState{repository}}
	env := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repoManager,
	}

	require.NoError(t, op.Execute(context.Background(), env, state))

	require.True(t, commandArgumentsExist(gitExecutor.commands, []string{"push", "--set-upstream", "origin", "feature/sample-docs"}))
}

func TestPullRequestCreateOperationCreatesPR(t *testing.T) {
	gitExecutor := &recordingGitExecutor{}
	client, clientErr := githubcli.NewClient(gitExecutor)
	require.NoError(t, clientErr)

	op, buildErr := buildPullRequestCreateOperation(map[string]any{
		"branch": "feature/{{ .Repository.Name }}",
		"title":  "chore({{ .Repository.Name }}): update",
		"body":   "Update {{ .Repository.FullName }}",
		"base":   "{{ .Repository.DefaultBranch }}",
	})
	require.NoError(t, buildErr)

	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:                "/repositories/sample",
		FinalOwnerRepo:      "octocat/sample",
		RemoteDefaultBranch: "main",
	})
	state := &State{Repositories: []*RepositoryState{repository}}
	env := &Environment{
		GitExecutor:  gitExecutor,
		GitHubClient: client,
	}

	require.NoError(t, op.Execute(context.Background(), env, state))
	require.NotEmpty(t, gitExecutor.githubCommands)
	require.Equal(t, "pr", firstArgument(gitExecutor.githubCommands[0].Arguments))
}

func TestGitStageCommitOperationStagesAndCommits(t *testing.T) {
	gitExecutor := &recordingGitExecutor{
		worktreeClean: true,
	}
	op, buildErr := buildGitStageCommitOperation(map[string]any{
		"paths":          []any{"README.md"},
		"commit_message": "docs: update",
	})
	require.NoError(t, buildErr)

	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	state := &State{Repositories: []*RepositoryState{repository}}
	env := &Environment{GitExecutor: gitExecutor}

	require.NoError(t, op.Execute(context.Background(), env, state))

	require.True(t, commandArgumentsExist(gitExecutor.commands, []string{"add", "README.md"}))
	require.True(t, commandArgumentsExist(gitExecutor.commands, []string{"commit", "-m", "docs: update"}))
}

func TestPullRequestOpenOperationPushesAndCreatesPR(t *testing.T) {
	gitExecutor := &recordingGitExecutor{
		worktreeClean: true,
		currentBranch: "main",
		remoteURLs:    map[string]string{"origin": "git@github.com:sample/repo.git"},
		existingRefs:  map[string]bool{"feature-sample-docs": false},
	}
	repoManager, managerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerErr)

	client, clientErr := githubcli.NewClient(gitExecutor)
	require.NoError(t, clientErr)

	op, buildErr := buildPullRequestOpenOperation(map[string]any{
		"branch": "feature/{{ .Repository.Name }}-docs",
		"title":  "chore({{ .Repository.Name }}): gitignore",
		"body":   "Ensure gitignore entries",
		"base":   "{{ .Repository.DefaultBranch }}",
	})
	require.NoError(t, buildErr)

	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:                "/repositories/sample",
		FinalOwnerRepo:      "octocat/sample",
		RemoteDefaultBranch: "main",
	})
	state := &State{Repositories: []*RepositoryState{repository}}
	env := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repoManager,
		GitHubClient:      client,
	}

	require.NoError(t, op.Execute(context.Background(), env, state))
	require.True(t, commandArgumentsExist(gitExecutor.commands, []string{"push", "--set-upstream", "origin", "feature/sample-docs"}))
	require.NotEmpty(t, gitExecutor.githubCommands)
	require.Equal(t, "pr", firstArgument(gitExecutor.githubCommands[0].Arguments))
}

func commandArgumentsExist(commands []execshell.CommandDetails, expected []string) bool {
	for _, command := range commands {
		if len(command.Arguments) != len(expected) {
			continue
		}
		match := true
		for index := range expected {
			if command.Arguments[index] != expected[index] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
func TestGitStageCommitOperationRequiresCleanWorktree(t *testing.T) {
	gitExecutor := &recordingGitExecutor{
		worktreeClean: false,
		currentBranch: "main",
	}
	repoManager, managerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerErr)

	op, buildErr := buildGitStageCommitOperation(map[string]any{
		"paths":          []any{"README.md"},
		"commit_message": "docs: update",
		"ensure_clean":   true,
	})
	require.NoError(t, buildErr)

	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	state := &State{Repositories: []*RepositoryState{repository}}
	env := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repoManager,
	}

	require.NoError(t, op.Execute(context.Background(), env, state))
	require.False(t, commandArgumentsExist(gitExecutor.commands, []string{"add", "README.md"}))
	require.False(t, commandArgumentsExist(gitExecutor.commands, []string{"commit", "-m", "docs: update"}))
}
