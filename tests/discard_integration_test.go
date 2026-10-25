package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func discardRepository(t *testing.T) string {
	t.Helper()
	root := createGitRepository(t, gitRepositoryOptions{})
	runGit(t, root, "config", "user.email", "discard@example.com")
	runGit(t, root, "config", "user.name", "Discard Test")
	for path, contents := range map[string]string{"one.txt": "one\n", "two.txt": "two\n", "dir/child.txt": "child\n", ".gitignore": "ignored.txt\n", "literal[1]*\n.txt": "literal\n", ":(glob)magic": "magic\n", "binary.dat": "\x00\x01\xff"} {
		discardWrite(t, root, path, contents)
	}
	require.NoError(t, os.Symlink("one.txt", filepath.Join(root, "link")))
	runGit(t, root, "add", "--all")
	runGit(t, root, "commit", "-m", "initial")
	discardWrite(t, root, "local.txt", "local commit\n")
	runGit(t, root, "add", "local.txt")
	runGit(t, root, "commit", "-m", "local commit")
	runGit(t, root, "tag", "keep")
	return root
}

func discardWrite(t *testing.T, root, path, contents string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, path), []byte(contents), 0o644))
}

func discardRun(t *testing.T, root string, args ...string) (string, error) {
	t.Helper()
	home := t.TempDir()
	output, err := runBinaryIntegrationCommand(t, buildIntegrationBinary(t, integrationRepositoryRoot(t)), root, map[string]string{
		"GIX_TEST_DISABLE_CONFIG_INJECTION": "1", "HOME": home, "XDG_CONFIG_HOME": home, "GH_TOKEN": "", "GITHUB_TOKEN": "", "OPENAI_API_KEY": "", "LLM_PROXY_SECRET_KEY": "",
	}, 30*time.Second, append([]string{"discard"}, args...))
	require.NoFileExists(t, filepath.Join(home, ".gix", "config.yml"))
	require.NoFileExists(t, filepath.Join(home, "gix", "config.yml"))
	return output, err
}

func TestDiscardAllWithLargeIgnoredTree(t *testing.T) {
	root := discardRepository(t)
	const trackedCount = 5000
	const ignoredCount = 40000
	discardWrite(t, root, ".gitignore", "ignored.txt\nnode_modules/\n")
	for index := range trackedCount {
		discardWrite(t, root, fmt.Sprintf("src/file-%04d.txt", index), "committed\n")
	}
	runGit(t, root, "add", "--all")
	runGit(t, root, "commit", "-m", "large tracked tree")
	refs := runGit(t, root, "show-ref")
	for index := range trackedCount {
		discardWrite(t, root, fmt.Sprintf("src/file-%04d.txt", index), "dirty\n")
	}
	for index := range ignoredCount {
		discardWrite(t, root, fmt.Sprintf("node_modules/package-%03d/lib/file-%04d.js", index/200, index), "ignored dependency\n")
	}
	started := time.Now()
	output, err := discardRun(t, root, "--all")
	t.Logf("discard with %d tracked and %d ignored files took %s", trackedCount, ignoredCount, time.Since(started))
	require.NoError(t, err, output)
	require.Empty(t, runGit(t, root, "diff", "HEAD", "--name-only"))
	require.Equal(t, refs, runGit(t, root, "show-ref"))
	for index := range trackedCount {
		require.Equal(t, "committed\n", readTextFile(t, filepath.Join(root, fmt.Sprintf("src/file-%04d.txt", index))))
	}
	for index := range ignoredCount {
		require.Equal(t, "ignored dependency\n", readTextFile(t, filepath.Join(root, fmt.Sprintf("node_modules/package-%03d/lib/file-%04d.js", index/200, index))))
	}
}

func TestDiscardScopedLiteralPaths(t *testing.T) {
	root := discardRepository(t)
	refs := runGit(t, root, "show-ref")
	discardWrite(t, root, "one.txt", "staged\n")
	runGit(t, root, "add", "one.txt")
	discardWrite(t, root, "one.txt", "unstaged\n")
	discardWrite(t, root, "two.txt", "keep staged\n")
	runGit(t, root, "add", "two.txt")
	discardWrite(t, root, "two.txt", "keep dirty\n")
	discardWrite(t, root, "dir/child.txt", "changed\n")
	for _, path := range []string{"literal[1]*\n.txt", ":(glob)magic"} {
		discardWrite(t, root, path, "changed\n")
	}
	discardWrite(t, root, "untracked.txt", "untracked\n")
	discardWrite(t, root, "ignored.txt", "ignored\n")
	output, err := discardRun(t, root, "one.txt", "dir", "literal[1]*\n.txt", ":(glob)magic")
	require.NoError(t, err, output)
	require.Equal(t, "one\n", readTextFile(t, filepath.Join(root, "one.txt")))
	require.Equal(t, "child\n", readTextFile(t, filepath.Join(root, "dir/child.txt")))
	require.Equal(t, "literal\n", readTextFile(t, filepath.Join(root, "literal[1]*\n.txt")))
	require.Equal(t, "magic\n", readTextFile(t, filepath.Join(root, ":(glob)magic")))
	require.Equal(t, "keep dirty\n", readTextFile(t, filepath.Join(root, "two.txt")))
	require.Equal(t, "untracked\n", readTextFile(t, filepath.Join(root, "untracked.txt")))
	require.Equal(t, "ignored\n", readTextFile(t, filepath.Join(root, "ignored.txt")))
	require.Equal(t, refs, runGit(t, root, "show-ref"))
	require.Equal(t, "two.txt\n", runGit(t, root, "diff", "--cached", "--name-only"))
	require.Equal(t, "keep staged\n", runGit(t, root, "show", ":two.txt"))
}

func TestDiscardAllFromSubdirectory(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(map[bool]string{false: "repository", true: "linked_worktree"}[linked], func(t *testing.T) {
			root := discardRepository(t)
			if linked {
				worktree := filepath.Join(t.TempDir(), "linked")
				runGit(t, root, "worktree", "add", "-b", "linked", worktree)
				root = worktree
			}
			refs := runGit(t, root, "show-ref")
			runGit(t, root, "mv", "one.txt", "renamed.txt")
			runGit(t, root, "rm", "two.txt")
			discardWrite(t, root, "added.txt", "added\n")
			runGit(t, root, "add", "added.txt")
			discardWrite(t, root, "intent.txt", "intent\n")
			runGit(t, root, "add", "-N", "intent.txt")
			discardWrite(t, root, "binary.dat", "\x00changed\xff")
			require.NoError(t, os.Remove(filepath.Join(root, "link")))
			require.NoError(t, os.Symlink("two.txt", filepath.Join(root, "link")))
			discardWrite(t, root, "dir/untracked.txt", "untracked\n")
			discardWrite(t, root, "ignored.txt", "ignored\n")
			output, err := discardRun(t, filepath.Join(root, "dir"), "--all")
			require.NoError(t, err, output)
			require.Equal(t, "one\n", readTextFile(t, filepath.Join(root, "one.txt")))
			require.Equal(t, "two\n", readTextFile(t, filepath.Join(root, "two.txt")))
			require.Equal(t, []byte{0, 1, 255}, []byte(readTextFile(t, filepath.Join(root, "binary.dat"))))
			target, linkErr := os.Readlink(filepath.Join(root, "link"))
			require.NoError(t, linkErr)
			require.Equal(t, "one.txt", target)
			for _, path := range []string{"added.txt", "intent.txt", "renamed.txt"} {
				require.NoFileExists(t, filepath.Join(root, path))
			}
			require.Empty(t, runGit(t, root, "diff", "HEAD", "--name-only"))
			require.Equal(t, "untracked\n", readTextFile(t, filepath.Join(root, "dir/untracked.txt")))
			require.Equal(t, "ignored\n", readTextFile(t, filepath.Join(root, "ignored.txt")))
			require.Equal(t, refs, runGit(t, root, "show-ref"))
		})
	}
}

func TestDiscardRejectsInvalidSelectorsBeforeMutation(t *testing.T) {
	for _, args := range [][]string{nil, {"--all", "one.txt"}, {"one.txt", "missing"}, {"untracked.txt"}, {"../outside"}, {"*.txt"}, {"--roots", ".", "--all"}, {"--remote", "origin", "--all"}, {"--web", "--all"}, {"--version", "--all"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			root := discardRepository(t)
			discardWrite(t, root, "one.txt", "dirty\n")
			runGit(t, root, "add", "one.txt")
			discardWrite(t, root, "untracked.txt", "keep\n")
			before := runGit(t, root, "status", "--porcelain=v1", "-z")
			output, err := discardRun(t, root, args...)
			require.Error(t, err, output)
			require.Equal(t, before, runGit(t, root, "status", "--porcelain=v1", "-z"))
			require.Equal(t, "dirty\n", readTextFile(t, filepath.Join(root, "one.txt")))
		})
	}
	for _, unborn := range []bool{false, true} {
		t.Run(map[bool]string{false: "non_repository", true: "unborn"}[unborn], func(t *testing.T) {
			root := t.TempDir()
			if unborn {
				runGit(t, root, "init")
			}
			output, err := discardRun(t, root, "--all")
			require.Error(t, err, output)
		})
	}
}

func TestDiscardPreservesUntrackedObstructions(t *testing.T) {
	for _, path := range []string{"one.txt", "ignored.txt", "dir"} {
		t.Run(path, func(t *testing.T) {
			root := discardRepository(t)
			if path == "ignored.txt" {
				discardWrite(t, root, path, "tracked\n")
				runGit(t, root, "add", "-f", path)
				runGit(t, root, "commit", "-m", "tracked ignored")
			}
			runGit(t, root, "rm", "-r", "--", path)
			if path == "dir" {
				discardWrite(t, root, "dir/child.txt", "untracked replacement\n")
			} else {
				discardWrite(t, root, path, "untracked replacement\n")
			}
			discardWrite(t, root, "two.txt", "dirty\n")
			runGit(t, root, "add", "two.txt")
			before := runGit(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
			output, err := discardRun(t, root, "--all")
			require.Error(t, err, output)
			require.Contains(t, output, "untracked")
			obstruction := path
			if path == "dir" {
				obstruction = "dir/child.txt"
			}
			require.Equal(t, "untracked replacement\n", readTextFile(t, filepath.Join(root, obstruction)))
			require.Equal(t, before, runGit(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all"))
			require.Equal(t, "dirty\n", readTextFile(t, filepath.Join(root, "two.txt")))
		})
	}
}

func TestDiscardFileDirectoryTransitions(t *testing.T) {
	root := discardRepository(t)
	runGit(t, root, "rm", "one.txt")
	discardWrite(t, root, "one.txt/nested", "replacement\n")
	runGit(t, root, "add", "one.txt")
	runGit(t, root, "rm", "dir/child.txt")
	require.NoError(t, os.RemoveAll(filepath.Join(root, "dir")))
	discardWrite(t, root, "dir", "replacement\n")
	runGit(t, root, "add", "dir")
	output, err := discardRun(t, root, "--all")
	require.NoError(t, err, output)
	require.Equal(t, "one\n", readTextFile(t, filepath.Join(root, "one.txt")))
	require.Equal(t, "child\n", readTextFile(t, filepath.Join(root, "dir/child.txt")))
	require.Empty(t, runGit(t, root, "status", "--porcelain=v1"))
}

func TestDiscardRejectsOperationsAndSparsePaths(t *testing.T) {
	for _, marker := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply", "sequencer"} {
		t.Run(marker, func(t *testing.T) {
			root := discardRepository(t)
			discardWrite(t, root, "one.txt", "dirty\n")
			markerPath := strings.TrimSpace(runGit(t, root, "rev-parse", "--git-path", marker))
			if !filepath.IsAbs(markerPath) {
				markerPath = filepath.Join(root, markerPath)
			}
			require.NoError(t, os.WriteFile(markerPath, []byte("in progress\n"), 0o644))
			output, err := discardRun(t, root, "--all")
			require.Error(t, err, output)
			require.Contains(t, output, "in progress")
			require.Equal(t, "dirty\n", readTextFile(t, filepath.Join(root, "one.txt")))
		})
	}
	t.Run("skip_worktree", func(t *testing.T) {
		root := discardRepository(t)
		runGit(t, root, "update-index", "--skip-worktree", "one.txt")
		discardWrite(t, root, "one.txt", "dirty\n")
		output, err := discardRun(t, root, "one.txt")
		require.Error(t, err, output)
		require.Contains(t, output, "sparse")
		require.Equal(t, "dirty\n", readTextFile(t, filepath.Join(root, "one.txt")))
	})
}

func TestDiscardRelativePathsAndCleanRepeat(t *testing.T) {
	root := discardRepository(t)
	head := runGit(t, root, "rev-parse", "HEAD")
	branch := runGit(t, root, "symbolic-ref", "HEAD")
	discardWrite(t, root, "one.txt", "changed\n")
	output, err := discardRun(t, filepath.Join(root, "dir"), "../one.txt")
	require.NoError(t, err, output)
	require.Equal(t, "one\n", readTextFile(t, filepath.Join(root, "one.txt")))
	for _, args := range [][]string{{"one.txt"}, {"--all"}, {"--all"}} {
		output, err := discardRun(t, root, args...)
		require.NoError(t, err, output)
	}
	require.Equal(t, head, runGit(t, root, "rev-parse", "HEAD"))
	require.Equal(t, branch, runGit(t, root, "symbolic-ref", "HEAD"))
	require.Empty(t, runGit(t, root, "status", "--porcelain=v1"))
}

func TestDiscardPreservesSubmoduleCheckout(t *testing.T) {
	root := discardRepository(t)
	child := discardRepository(t)
	runGit(t, root, "-c", "protocol.file.allow=always", "submodule", "add", child, "nested")
	runGit(t, root, "commit", "-m", "add submodule")
	nested := filepath.Join(root, "nested")
	nestedHead := runGit(t, nested, "rev-parse", "HEAD")
	discardWrite(t, nested, "one.txt", "submodule dirty\n")
	discardWrite(t, nested, "untracked.txt", "submodule untracked\n")
	discardWrite(t, root, "one.txt", "parent dirty\n")
	output, err := discardRun(t, root, "--all")
	require.NoError(t, err, output)
	require.Equal(t, "one\n", readTextFile(t, filepath.Join(root, "one.txt")))
	require.Equal(t, "submodule dirty\n", readTextFile(t, filepath.Join(nested, "one.txt")))
	require.Equal(t, "submodule untracked\n", readTextFile(t, filepath.Join(nested, "untracked.txt")))
	require.Equal(t, nestedHead, runGit(t, nested, "rev-parse", "HEAD"))
	for _, path := range []string{"nested", "nested/one.txt"} {
		output, err := discardRun(t, root, path)
		require.Error(t, err, output)
		require.Contains(t, output, "submodule")
	}
	runGit(t, nested, "config", "user.name", "Discard Test")
	runGit(t, nested, "config", "user.email", "discard@example.com")
	runGit(t, nested, "add", "one.txt")
	runGit(t, nested, "commit", "-m", "submodule change")
	runGit(t, root, "add", "nested")
	discardWrite(t, root, "one.txt", "parent changed again\n")
	output, err = discardRun(t, root, "--all")
	require.Error(t, err, output)
	require.Contains(t, output, "submodule")
	require.Equal(t, "parent changed again\n", readTextFile(t, filepath.Join(root, "one.txt")))
}

func TestDiscardRejectsUnmergedIndex(t *testing.T) {
	root := discardRepository(t)
	runGit(t, root, "switch", "-c", "other")
	discardWrite(t, root, "one.txt", "other branch\n")
	runGit(t, root, "add", "one.txt")
	runGit(t, root, "commit", "-m", "other")
	runGit(t, root, "switch", "main")
	discardWrite(t, root, "one.txt", "main branch\n")
	runGit(t, root, "add", "one.txt")
	runGit(t, root, "commit", "-m", "main")
	// Leave conflict entries while removing the operation marker to exercise the index boundary.
	mergeOutput, mergeErr := runBinaryGitDiscard(t, root, "merge", "other")
	require.Error(t, mergeErr, mergeOutput)
	require.NoError(t, os.Remove(filepath.Join(root, ".git", "MERGE_HEAD")))
	before := runGit(t, root, "diff", "--name-only", "--diff-filter=U", "-z")
	contents := readTextFile(t, filepath.Join(root, "one.txt"))
	output, err := discardRun(t, root, "--all")
	require.Error(t, err, output)
	require.Contains(t, output, "unmerged")
	require.Equal(t, before, runGit(t, root, "diff", "--name-only", "--diff-filter=U", "-z"))
	require.Equal(t, contents, readTextFile(t, filepath.Join(root, "one.txt")))
}

func runBinaryGitDiscard(t *testing.T, root string, arguments ...string) (string, error) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = root
	command.Env = buildGitCommandEnvironment(nil)
	output, err := command.CombinedOutput()
	return string(output), err
}

func TestDiscardPreservesAncestorAndDescendantObstructions(t *testing.T) {
	for _, ancestor := range []bool{false, true} {
		t.Run(map[bool]string{false: "descendant", true: "ancestor"}[ancestor], func(t *testing.T) {
			root := discardRepository(t)
			obstruction := "one.txt/untracked"
			if ancestor {
				runGit(t, root, "rm", "-r", "dir")
				obstruction = "dir"
			} else {
				runGit(t, root, "rm", "one.txt")
			}
			discardWrite(t, root, obstruction, "untracked bytes\n")
			discardWrite(t, root, "two.txt", "dirty bytes\n")
			before := runGit(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
			output, err := discardRun(t, root, "--all")
			require.Error(t, err, output)
			require.Contains(t, output, "untracked")
			require.Equal(t, "untracked bytes\n", readTextFile(t, filepath.Join(root, obstruction)))
			require.Equal(t, "dirty bytes\n", readTextFile(t, filepath.Join(root, "two.txt")))
			require.Equal(t, before, runGit(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all"))
		})
	}
}

func discardRequireFilesystemAlias(t *testing.T, root, original, alias string) {
	t.Helper()
	discardWrite(t, root, original, "alias capability\n")
	originalInfo, err := os.Lstat(filepath.Join(root, original))
	require.NoError(t, err)
	aliasInfo, err := os.Lstat(filepath.Join(root, alias))
	if os.IsNotExist(err) {
		t.Skip("filesystem treats these case or Unicode spellings as distinct paths")
	}
	require.NoError(t, err)
	if !os.SameFile(originalInfo, aliasInfo) {
		t.Skip("filesystem treats these case or Unicode spellings as distinct objects")
	}
	require.NoError(t, os.Remove(filepath.Join(root, original)))
}

func TestDiscardPreservesFilesystemAliases(t *testing.T) {
	for _, scenario := range []struct{ name, original, alias string }{
		{"case", "one.txt", "ONE.TXT"},
		{"unicode", "caf\u00e9.txt", "cafe\u0301.txt"},
		{"ancestor", "dir/child.txt", "DIR/CHILD.TXT"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := discardRepository(t)
			probe := t.TempDir()
			discardRequireFilesystemAlias(t, probe, scenario.original, scenario.alias)
			if scenario.name == "unicode" {
				discardWrite(t, root, scenario.original, "committed unicode\n")
				runGit(t, root, "add", scenario.original)
				runGit(t, root, "commit", "-m", "unicode")
			}
			runGit(t, root, "rm", "--", scenario.original)
			discardWrite(t, root, scenario.alias, "untracked alias bytes\n")
			discardWrite(t, root, "two.txt", "unrelated dirty\n")
			before := runGit(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
			output, err := discardRun(t, root, "--all")
			require.Equal(t, "untracked alias bytes\n", readTextFile(t, filepath.Join(root, scenario.alias)))
			require.Error(t, err, output)
			require.Equal(t, "unrelated dirty\n", readTextFile(t, filepath.Join(root, "two.txt")))
			require.Equal(t, before, runGit(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all"))
		})
	}
}

func TestDiscardPreservesIndexedFilesystemAliases(t *testing.T) {
	for _, scenario := range []string{"rename_scoped", "rename_new_scoped", "rename_all", "addition_scoped"} {
		t.Run(scenario, func(t *testing.T) {
			root := discardRepository(t)
			discardRequireFilesystemAlias(t, t.TempDir(), "one.txt", "ONE.TXT")
			if scenario == "addition_scoped" {
				discardWrite(t, root, "ONE.TXT", "indexed alias bytes\n")
				runGit(t, root, "-c", "core.ignorecase=false", "add", "--", "ONE.TXT")
			} else {
				runGit(t, root, "mv", "one.txt", "ONE.TXT")
				discardWrite(t, root, "ONE.TXT", "indexed alias bytes\n")
			}
			before := runGit(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
			args := []string{"one.txt"}
			if scenario == "rename_all" {
				args = []string{"--all"}
			}
			if scenario == "addition_scoped" || scenario == "rename_new_scoped" {
				args = []string{"ONE.TXT"}
			}
			output, err := discardRun(t, root, args...)
			if scenario == "rename_all" {
				require.NoError(t, err, output)
				require.Equal(t, "one\n", readTextFile(t, filepath.Join(root, "one.txt")))
				require.Empty(t, runGit(t, root, "status", "--porcelain=v1"))
				return
			}
			require.Equal(t, "indexed alias bytes\n", readTextFile(t, filepath.Join(root, "ONE.TXT")))
			require.Error(t, err, output)
			require.Equal(t, before, runGit(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all"))
		})
	}
}

func TestDiscardRestoresSelectedSymlinkWithoutChangingTarget(t *testing.T) {
	root := discardRepository(t)
	discardWrite(t, root, "two.txt", "protected staged bytes\n")
	runGit(t, root, "add", "two.txt")
	discardWrite(t, root, "two.txt", "protected unstaged bytes\n")
	require.NoError(t, os.Remove(filepath.Join(root, "one.txt")))
	require.NoError(t, os.Symlink("two.txt", filepath.Join(root, "one.txt")))
	output, err := discardRun(t, root, "one.txt")
	require.NoError(t, err, output)
	require.Equal(t, "one\n", readTextFile(t, filepath.Join(root, "one.txt")))
	require.Equal(t, "protected unstaged bytes\n", readTextFile(t, filepath.Join(root, "two.txt")))
	require.Equal(t, "protected staged bytes\n", runGit(t, root, "show", ":two.txt"))
	require.Equal(t, "two.txt\n", runGit(t, root, "diff", "--cached", "--name-only"))
}

func TestDiscardPreservesHardlinkAliases(t *testing.T) {
	for _, protected := range []string{"untracked.txt", "ignored.txt", "two.txt"} {
		t.Run(protected, func(t *testing.T) {
			root := discardRepository(t)
			discardWrite(t, root, "one.txt", "shared bytes\n")
			if protected == "two.txt" {
				require.NoError(t, os.Remove(filepath.Join(root, protected)))
			}
			require.NoError(t, os.Link(filepath.Join(root, "one.txt"), filepath.Join(root, protected)))
			before := runGit(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
			output, err := discardRun(t, root, "one.txt")
			require.Error(t, err, output)
			require.Contains(t, output, "aliases protected path")
			require.Equal(t, "shared bytes\n", readTextFile(t, filepath.Join(root, protected)))
			require.Equal(t, before, runGit(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all"))
		})
	}
}

func TestDiscardPreservesFilesystemAliasAncestors(t *testing.T) {
	for _, ancestor := range []bool{false, true} {
		t.Run(map[bool]string{false: "selected_directory", true: "protected_directory"}[ancestor], func(t *testing.T) {
			root := discardRepository(t)
			discardRequireFilesystemAlias(t, t.TempDir(), "one.txt", "ONE.TXT")
			protected := "ONE.TXT/untracked.txt"
			if ancestor {
				runGit(t, root, "rm", "-r", "dir")
				protected = "DIR"
			} else {
				runGit(t, root, "rm", "one.txt")
			}
			discardWrite(t, root, protected, "protected bytes\n")
			discardWrite(t, root, "two.txt", "dirty bytes\n")
			before := runGit(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
			output, err := discardRun(t, root, "--all")
			require.Error(t, err, output)
			require.Contains(t, output, "aliases protected path")
			require.Equal(t, "protected bytes\n", readTextFile(t, filepath.Join(root, protected)))
			require.Equal(t, "dirty bytes\n", readTextFile(t, filepath.Join(root, "two.txt")))
			require.Equal(t, before, runGit(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all"))
		})
	}
}
