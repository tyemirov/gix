package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
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

	require.ErrorIs(t, op.Execute(context.Background(), env, state), errRepositorySkipped)
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

func TestGitBranchCleanupOperationDeletesBranchWhenNoMutations(t *testing.T) {
	gitExecutor := &recordingGitExecutor{
		branchExists: true,
	}
	op, buildErr := buildGitBranchCleanupOperation(map[string]any{
		"branch": "automation/{{ .Repository.Name }}-cleanup",
		"base":   "master",
	})
	require.NoError(t, buildErr)

	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:           "/repositories/sample",
		FinalOwnerRepo: "octocat/sample",
	})
	state := &State{Repositories: []*RepositoryState{repository}}
	env := &Environment{
		GitExecutor: gitExecutor,
		State:       state,
	}

	require.NoError(t, op.Execute(context.Background(), env, state))
	require.True(t, commandArgumentsExist(gitExecutor.commands, []string{"branch", "-D", "automation/sample-cleanup"}))
}

func TestGitBranchCleanupOperationKeepsBranchWhenCommitsBeyondBase(t *testing.T) {
	gitExecutor := &recordingGitExecutor{
		branchExists: true,
		revListOutput: map[string]string{
			"master..automation/sample-cleanup": "abc123\n",
		},
	}
	op, buildErr := buildGitBranchCleanupOperation(map[string]any{
		"branch": "automation/{{ .Repository.Name }}-cleanup",
		"base":   "master",
	})
	require.NoError(t, buildErr)

	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:           "/repositories/sample",
		FinalOwnerRepo: "octocat/sample",
	})
	state := &State{Repositories: []*RepositoryState{repository}}
	env := &Environment{
		GitExecutor: gitExecutor,
		State:       state,
	}

	require.NoError(t, op.Execute(context.Background(), env, state))
	require.False(t, commandArgumentsExist(gitExecutor.commands, []string{"branch", "-D", "automation/sample-cleanup"}))
}

func TestGitBranchCleanupOperationUsesRepositoryDefaultBaseBranch(t *testing.T) {
	gitExecutor := &recordingGitExecutor{
		branchExists: true,
	}
	op, buildErr := buildGitBranchCleanupOperation(map[string]any{
		"branch": "automation/{{ .Repository.Name }}-cleanup",
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
		GitExecutor: gitExecutor,
		State:       state,
	}

	require.NoError(t, op.Execute(context.Background(), env, state))
	require.True(t, commandArgumentsExist(gitExecutor.commands, []string{"branch", "-D", "automation/sample-cleanup"}))
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

func TestPullRequestCreateOperationSkipsWhenBranchMissing(t *testing.T) {
	gitExecutor := &recordingGitExecutor{}
	client, clientErr := githubcli.NewClient(gitExecutor)
	require.NoError(t, clientErr)

	op, buildErr := buildPullRequestCreateOperation(map[string]any{
		"branch": "{{ index .Environment \"missing_branch\" }}",
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
	require.Len(t, gitExecutor.githubCommands, 0)
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

func TestGitStageCommitOperationPrefersRecordedMutations(t *testing.T) {
	gitExecutor := &recordingGitExecutor{worktreeClean: true}
	op, buildErr := buildGitStageCommitOperation(map[string]any{
		"paths":          []any{"."},
		"commit_message": "refactor: apply workflow changes",
	})
	require.NoError(t, buildErr)

	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	state := &State{Repositories: []*RepositoryState{repository}}
	env := &Environment{
		GitExecutor: gitExecutor,
		State:       state,
	}
	env.RecordMutatedFile(repository, "go.mod")
	env.RecordMutatedFile(repository, "nested/lib.go")

	require.NoError(t, op.Execute(context.Background(), env, state))
	require.True(t, commandArgumentsExist(gitExecutor.commands, []string{"add", "go.mod"}))
	require.True(t, commandArgumentsExist(gitExecutor.commands, []string{"add", "nested/lib.go"}))
	require.False(t, commandArgumentsExist(gitExecutor.commands, []string{"add", "."}))
}

func TestGitStageCommitOperationFiltersMutationsByPatterns(t *testing.T) {
	gitExecutor := &recordingGitExecutor{worktreeClean: true}
	op, buildErr := buildGitStageCommitOperation(map[string]any{
		"paths":          []any{"docs/**"},
		"commit_message": "chore: docs",
	})
	require.NoError(t, buildErr)

	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	state := &State{Repositories: []*RepositoryState{repository}}
	env := &Environment{
		GitExecutor: gitExecutor,
		State:       state,
	}
	env.RecordMutatedFile(repository, "docs/guide.md")
	env.RecordMutatedFile(repository, "go.mod")

	require.NoError(t, op.Execute(context.Background(), env, state))
	require.True(t, commandArgumentsExist(gitExecutor.commands, []string{"add", "docs/guide.md"}))
	require.False(t, commandArgumentsExist(gitExecutor.commands, []string{"add", "go.mod"}))
}

func TestGitStageCommitOperationRespectsRequireChangesSafeguard(t *testing.T) {
	gitExecutor := &recordingGitExecutor{worktreeClean: true}
	repoManager, managerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerErr)

	op, buildErr := buildGitStageCommitOperation(map[string]any{
		"paths":          []any{"README.md"},
		"commit_message": "docs: update",
		"safeguards": map[string]any{
			"soft_skip": map[string]any{"require_changes": true},
		},
	})
	require.NoError(t, buildErr)

	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	state := &State{Repositories: []*RepositoryState{repository}}
	env := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repoManager,
	}

	require.NoError(t, op.Execute(context.Background(), env, state))
	require.False(t, commandArgumentsExist(gitExecutor.commands, []string{"commit", "-m", "docs: update"}))
}

func TestGitStageCommitOperationRequiresChangesWhenConfigured(t *testing.T) {
	gitExecutor := &recordingGitExecutor{
		worktreeClean:   false,
		worktreeEntries: []string{" M README.md"},
	}
	repoManager, managerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerErr)

	op, buildErr := buildGitStageCommitOperation(map[string]any{
		"paths":          []any{"README.md"},
		"commit_message": "docs: update",
		"safeguards": map[string]any{
			"soft_skip": map[string]any{"require_changes": true},
		},
	})
	require.NoError(t, buildErr)

	repository := NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/sample"})
	state := &State{Repositories: []*RepositoryState{repository}}
	env := &Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: repoManager,
	}

	require.NoError(t, op.Execute(context.Background(), env, state))
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

func TestPullRequestOpenOperationSkipsWhenBranchMissing(t *testing.T) {
	gitExecutor := &recordingGitExecutor{
		worktreeClean: true,
		currentBranch: "main",
		remoteURLs:    map[string]string{"origin": "git@github.com:sample/repo.git"},
		existingRefs:  map[string]bool{},
	}
	repoManager, managerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerErr)

	client, clientErr := githubcli.NewClient(gitExecutor)
	require.NoError(t, clientErr)

	op, buildErr := buildPullRequestOpenOperation(map[string]any{
		"branch": "{{ index .Environment \"missing_branch\" }}",
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
	require.Len(t, gitExecutor.githubCommands, 0)
	require.Len(t, gitExecutor.commands, 0)
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

	require.ErrorIs(t, op.Execute(context.Background(), env, state), errRepositorySkipped)
	require.False(t, commandArgumentsExist(gitExecutor.commands, []string{"add", "README.md"}))
	require.False(t, commandArgumentsExist(gitExecutor.commands, []string{"commit", "-m", "docs: update"}))
}
