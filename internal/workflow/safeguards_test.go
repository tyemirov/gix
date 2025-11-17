package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/gitrepo"
)

func TestEvaluateSafeguardsRequireClean(t *testing.T) {
	t.Parallel()

	executor := &recordingGitExecutor{worktreeClean: false, currentBranch: "master"}
	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	env := &Environment{RepositoryManager: manager}
	repo := &RepositoryState{Path: "/repositories/sample"}

	pass, reason, evalErr := EvaluateSafeguards(context.Background(), env, repo, map[string]any{"require_clean": true})
	require.NoError(t, evalErr)
	require.False(t, pass)
	require.Equal(t, "repository not clean: M file.txt", reason)
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
		"require_clean": true,
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
		"hard_stop": map[string]any{"require_clean": true},
		"soft_skip": map[string]any{"paths": []string{"README.md"}},
	}

	hard, soft := splitSafeguardSets(raw, safeguardDefaultHardStop)
	require.True(t, hard["require_clean"].(bool))
	require.ElementsMatch(t, []string{"README.md"}, soft["paths"].([]string))
}
