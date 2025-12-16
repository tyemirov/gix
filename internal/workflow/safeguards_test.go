package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/worktree"
)

func TestEvaluateSafeguardsRequireClean(t *testing.T) {
	t.Parallel()

	executor := &recordingGitExecutor{worktreeClean: false, currentBranch: "master"}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	env := &Environment{RepositoryManager: manager}
	repo := &RepositoryState{Path: "/repositories/sample"}

	pass, reason, evalErr := EvaluateSafeguards(context.Background(), env, repo, map[string]any{
		"require_clean": map[string]any{"enabled": true},
	})
	require.NoError(t, evalErr)
	require.False(t, pass)
	require.Equal(t, "repository not clean: M file.txt", reason)
}

func TestEvaluateSafeguardsRequireCleanIgnoresPaths(t *testing.T) {
	t.Parallel()

	executor := &recordingGitExecutor{
		worktreeClean:   false,
		currentBranch:   "master",
		worktreeEntries: []string{"?? .DS_Store"},
	}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	env := &Environment{RepositoryManager: manager}
	repo := &RepositoryState{Path: "/repositories/sample"}

	pass, reason, evalErr := EvaluateSafeguards(context.Background(), env, repo, map[string]any{
		"require_clean": map[string]any{
			"enabled":            true,
			"ignore_dirty_paths": []string{".DS_Store"},
		},
	})
	require.NoError(t, evalErr)
	require.True(t, pass)
	require.Empty(t, reason)

	executor.worktreeEntries = []string{"?? .DS_Store", " M main.go"}
	pass, reason, evalErr = EvaluateSafeguards(context.Background(), env, repo, map[string]any{
		"require_clean": map[string]any{
			"enabled":            true,
			"ignore_dirty_paths": []string{".DS_Store"},
		},
	})
	require.NoError(t, evalErr)
	require.False(t, pass)
	require.Equal(t, "repository not clean: M main.go", reason)
}

func TestEvaluateSafeguardsRequireCleanIgnoresUntrackedByDefault(t *testing.T) {
	t.Parallel()

	executor := &recordingGitExecutor{
		worktreeClean:   false,
		currentBranch:   "master",
		worktreeEntries: []string{"?? scratch.txt"},
	}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	env := &Environment{RepositoryManager: manager}
	repo := &RepositoryState{Path: "/repositories/sample"}

	pass, reason, evalErr := EvaluateSafeguards(context.Background(), env, repo, map[string]any{
		"require_clean": true,
	})
	require.NoError(t, evalErr)
	require.True(t, pass)
	require.Empty(t, reason)
}

func TestEvaluateSafeguardsRequireCleanStringValue(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		requireClean  any
		worktreeClean bool
		expectedPass  bool
		reason        string
	}{
		"enabled": {
			requireClean:  "true",
			worktreeClean: false,
			expectedPass:  false,
			reason:        "repository not clean: M file.txt",
		},
		"disabled": {
			requireClean:  "false",
			worktreeClean: false,
			expectedPass:  true,
			reason:        "",
		},
	}

	for name, testCase := range testCases {
		testCase := testCase
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			executor := &recordingGitExecutor{worktreeClean: testCase.worktreeClean, currentBranch: "master"}
			manager, err := gitrepo.NewRepositoryManager(executor)
			require.NoError(t, err)

			env := &Environment{RepositoryManager: manager}
			repo := &RepositoryState{Path: "/repositories/sample"}

			pass, reason, evalErr := EvaluateSafeguards(context.Background(), env, repo, map[string]any{
				"require_clean": testCase.requireClean,
			})
			require.NoError(t, evalErr)
			require.Equal(t, testCase.expectedPass, pass)
			require.Equal(t, testCase.reason, reason)
		})
	}
}

func TestEvaluateSafeguardsRequireChanges(t *testing.T) {
	t.Parallel()

	executor := &recordingGitExecutor{worktreeClean: true, currentBranch: "master"}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	env := &Environment{RepositoryManager: manager}
	repo := &RepositoryState{Path: "/repositories/sample"}

	pass, reason, evalErr := EvaluateSafeguards(context.Background(), env, repo, map[string]any{
		"require_changes": true,
	})
	require.NoError(t, evalErr)
	require.False(t, pass)
	require.Contains(t, reason, "requires changes")

	executor.worktreeClean = false
	executor.worktreeEntries = []string{" M README.md"}

	pass, reason, evalErr = EvaluateSafeguards(context.Background(), env, repo, map[string]any{
		"require_changes": true,
	})
	require.NoError(t, evalErr)
	require.True(t, pass)
	require.Empty(t, reason)
}

func TestEvaluateSafeguardsRequireChangesPassesWhenWorkflowRecordedChanges(t *testing.T) {
	t.Parallel()

	env := &Environment{}
	repo := &RepositoryState{Path: "/repositories/sample"}
	env.RecordWorkflowChange(repo)

	pass, reason, evalErr := EvaluateSafeguards(context.Background(), env, repo, map[string]any{
		"require_changes": true,
	})
	require.NoError(t, evalErr)
	require.True(t, pass)
	require.Empty(t, reason)
}

func TestEvaluateSafeguardsBranchAndPaths(t *testing.T) {
	t.Parallel()

	executor := &recordingGitExecutor{worktreeClean: true, currentBranch: "develop"}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	env := &Environment{
		RepositoryManager: manager,
		FileSystem:        newFakeFileSystem(map[string][]byte{"/repositories/sample/go.mod": []byte("module example")}),
	}
	repo := &RepositoryState{Path: "/repositories/sample"}

	pass, reason, evalErr := EvaluateSafeguards(context.Background(), env, repo, map[string]any{
		"branch": "master",
		"paths":  []string{"go.mod"},
	})
	require.NoError(t, evalErr)
	require.False(t, pass)
	require.Equal(t, "requires branch master", reason)
}

func TestEvaluateSafeguardsBranchNot(t *testing.T) {
	t.Parallel()

	executor := &recordingGitExecutor{worktreeClean: true, currentBranch: "master"}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	env := &Environment{RepositoryManager: manager}
	repo := &RepositoryState{Path: "/repositories/sample"}

	pass, reason, evalErr := EvaluateSafeguards(context.Background(), env, repo, map[string]any{
		"branch_not": "master",
	})
	require.NoError(t, evalErr)
	require.False(t, pass)
	require.Equal(t, "skipped: already on branch master", reason)

	executor.currentBranch = "develop"

	pass, reason, evalErr = EvaluateSafeguards(context.Background(), env, repo, map[string]any{
		"branch_not": "master",
	})
	require.NoError(t, evalErr)
	require.True(t, pass)
	require.Empty(t, reason)
}

func TestEvaluateSafeguardsPasses(t *testing.T) {
	t.Parallel()

	executor := &recordingGitExecutor{worktreeClean: true, currentBranch: "master"}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	env := &Environment{
		RepositoryManager: manager,
		FileSystem:        newFakeFileSystem(map[string][]byte{"/repositories/sample/README.md": []byte("docs")}),
	}
	repo := &RepositoryState{Path: "/repositories/sample"}

	pass, reason, evalErr := EvaluateSafeguards(context.Background(), env, repo, map[string]any{
		"require_clean": map[string]any{"enabled": true},
		"branch_in":     []string{"dev", "master"},
		"paths":         []string{"README.md"},
	})
	require.NoError(t, evalErr)
	require.True(t, pass)
	require.Empty(t, reason)
}

func TestSplitSafeguardsFallbacks(t *testing.T) {
	hard, soft := splitSafeguardSets(map[string]any{"branch": "main"}, safeguardDefaultHardStop)
	require.Equal(t, "main", hard["branch"])
	require.Nil(t, soft)

	hard, soft = splitSafeguardSets(map[string]any{"branch": "main"}, safeguardDefaultSoftSkip)
	require.Nil(t, hard)
	require.Equal(t, "main", soft["branch"])
}

func TestSplitSafeguardsStructured(t *testing.T) {
	raw := map[string]any{
		"hard_stop": map[string]any{"require_clean": map[string]any{"enabled": true}},
		"soft_skip": map[string]any{"paths": []string{"README.md"}},
	}

	hard, soft := splitSafeguardSets(raw, safeguardDefaultHardStop)
	require.True(t, hard["require_clean"].(map[string]any)["enabled"].(bool))
	require.ElementsMatch(t, []string{"README.md"}, soft["paths"].([]string))
}

func TestFilterIgnoredStatusEntriesDropsUntrackedWithoutPatterns(t *testing.T) {
	t.Parallel()

	entries := []string{"?? cache.txt", "?? .DS_Store"}
	remaining := worktree.FilterStatusEntries(entries, nil)

	require.Empty(t, remaining)
}

func TestFilterStatusEntriesAppliesIgnorePatterns(t *testing.T) {
	t.Parallel()

	patterns := worktree.BuildIgnorePatterns([]string{"bin/", ".env.*"})
	entries := []string{"?? bin/temp.sh", " M bin/script.sh", "?? .env.local", " M .env.example", "A main.go"}

	remaining := worktree.FilterStatusEntries(entries, patterns)

	require.ElementsMatch(t, []string{"A main.go"}, remaining)
}
