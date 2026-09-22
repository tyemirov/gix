package tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageCommitSelectedSourceContract(testInstance *testing.T) {
	binary := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	for _, scenario := range []struct {
		name, source             string
		staged, worktree, binary bool
		deleted, addedEmpty      bool
		wantRequest              bool
	}{
		{name: "untracked_only", source: "staged"},
		{name: "unstaged_only", source: "staged", worktree: true},
		{name: "staged_only_worktree_source", source: "worktree", staged: true},
		{name: "staged_scoped_evidence", source: "staged", staged: true, worktree: true, wantRequest: true},
		{name: "deleted_staged", source: "staged", staged: true, deleted: true, wantRequest: true},
		{name: "empty_addition", source: "staged", staged: true, addedEmpty: true, wantRequest: true},
		{name: "binary_staged", source: "staged", staged: true, binary: true, wantRequest: true},
		{name: "worktree_change", source: "worktree", worktree: true, wantRequest: true},
	} {
		testInstance.Run(scenario.name, func(testInstance *testing.T) {
			repo := createGitRepository(testInstance, gitRepositoryOptions{DirectoryName: "source-contract", InitialBranch: "master"})
			for _, name := range []string{"selected.txt", "outside.txt"} {
				if scenario.addedEmpty && name == "selected.txt" {
					continue
				}
				require.NoError(testInstance, os.WriteFile(filepath.Join(repo, name), []byte("initial\n"), 0o644))
			}
			runGit(testInstance, repo, "add", "--all")
			runGit(testInstance, repo, "commit", "-m", "initial")
			require.NoError(testInstance, os.WriteFile(filepath.Join(repo, "unrelated.txt"), []byte("unrelated pending work\n"), 0o644))
			if scenario.staged {
				contents := []byte("selected change\n")
				if scenario.binary {
					contents = []byte{0, 1, 2, 3}
				}
				require.NoError(testInstance, os.WriteFile(filepath.Join(repo, "selected.txt"), contents, 0o644))
				if scenario.deleted {
					require.NoError(testInstance, os.Remove(filepath.Join(repo, "selected.txt")))
				}
				if scenario.addedEmpty {
					require.NoError(testInstance, os.WriteFile(filepath.Join(repo, "selected.txt"), nil, 0o644))
				}
				runGit(testInstance, repo, "add", "--all", "--", "selected.txt")
			}
			if scenario.worktree {
				require.NoError(testInstance, os.WriteFile(filepath.Join(repo, "outside.txt"), []byte("outside change\n"), 0o644))
			}
			var mutex sync.Mutex
			var requests []string
			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
				body, err := io.ReadAll(request.Body)
				if err != nil {
					http.Error(responseWriter, err.Error(), http.StatusBadRequest)
					return
				}
				mutex.Lock()
				requests = append(requests, string(body))
				mutex.Unlock()
				responseWriter.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(responseWriter, `{"choices":[{"message":{"role":"assistant","content":"fix: selected change"}}]}`)
			}))
			testInstance.Cleanup(server.Close)
			config := writeDirtySyncMergedBranchConfiguration(testInstance, server.URL)
			contents := strings.ReplaceAll(readTextFile(testInstance, config), "diff_source: staged", "diff_source: "+scenario.source)
			require.NoError(testInstance, os.WriteFile(config, []byte(contents), 0o600))
			output, err := runBinaryIntegrationCommand(testInstance, binary, repo, nil, syncStateTransitionTimeout,
				[]string{"--config", config, "--roots", repo, "message", "commit"})
			mutex.Lock()
			defer mutex.Unlock()
			if !scenario.wantRequest {
				require.Error(testInstance, err, output)
				require.Contains(testInstance, output, "no changes detected")
				require.Empty(testInstance, requests)
				return
			}
			require.NoError(testInstance, err, output)
			require.Len(testInstance, requests, 1)
			if scenario.source == "staged" {
				require.NotContains(testInstance, requests[0], "unrelated.txt")
				require.NotContains(testInstance, requests[0], "outside.txt")
				require.Contains(testInstance, requests[0], "selected.txt")
			}
		})
	}
}

func TestSyncProtectsClusterPreparation(testInstance *testing.T) {
	binary := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	for _, phase := range []string{"after_add", "before_checkpoint", "after_publish"} {
		for _, mutation := range []string{"reset", "partial", "add", "switch"} {
			testInstance.Run(phase+"_"+mutation, func(testInstance *testing.T) {
				root := testInstance.TempDir()
				remote, repo := filepath.Join(root, "remote.git"), filepath.Join(root, "project")
				createSyncGitHubBackedRepository(testInstance, remote, repo)
				require.NoError(testInstance, os.WriteFile(filepath.Join(repo, "README.md"), []byte("initial\n"), 0o644))
				runGit(testInstance, repo, "add", "README.md")
				runGit(testInstance, repo, "commit", "-m", "initial")
				runGit(testInstance, repo, "push", "-u", "origin", "master")
				for _, name := range []string{"_scratch/first.txt", "_scratch/second.txt", "outside.txt"} {
					target := filepath.Join(repo, name)
					require.NoError(testInstance, os.MkdirAll(filepath.Dir(target), 0o755))
					require.NoError(testInstance, os.WriteFile(target, []byte(name+" contents\n"), 0o644))
				}
				server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
					responseWriter.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(responseWriter, `{"choices":[{"message":{"role":"assistant","content":"fix: preserve selected files"}}]}`)
				}))
				testInstance.Cleanup(server.Close)
				config := writeDirtySyncMergedBranchConfiguration(testInstance, server.URL)
				marker := filepath.Join(root, "interference")
				output, err := runBinaryIntegrationCommand(testInstance, binary, repo, map[string]string{
					pathEnvironmentVariableNameConstant: buildSyncMergedBranchExecutablePath(testInstance),
					"GIX_SYNC_TEST_PREPARE_PHASE":       phase,
					"GIX_SYNC_TEST_PREPARE_MUTATION":    mutation,
					"GIX_SYNC_TEST_PREPARE_MARKER":      marker,
				}, syncStateTransitionTimeout, []string{"--config", config, "--roots", repo, "sync", "master"})
				if mutation == "switch" || phase == "after_publish" {
					require.Error(testInstance, err, output)
					require.Contains(testInstance, output, "SYNC_SWITCH_HANDOFF")
					require.NotContains(testInstance, output, "SYNC_SWITCH_ROLLBACK")
					require.Contains(testInstance, runGit(testInstance, repo, "stash", "list"), "gix strict-sync transaction snapshot")
					require.Equal(testInstance, runGit(testInstance, remote, "rev-parse", "master"), runGit(testInstance, repo, "rev-parse", "HEAD"))
					if mutation == "switch" {
						require.Equal(testInstance, "operator-change", strings.TrimSpace(runGit(testInstance, repo, "branch", "--show-current")))
					}
					for _, name := range []string{"_scratch/first.txt", "_scratch/second.txt", "outside.txt"} {
						require.Equal(testInstance, name+" contents\n", readTextFile(testInstance, filepath.Join(repo, name)))
					}
					expectedPaths := ""
					if phase == "after_publish" {
						switch mutation {
						case "partial":
							expectedPaths = "_scratch/first.txt\n"
						case "add":
							expectedPaths = "_scratch/first.txt\n_scratch/second.txt\noutside.txt\n"
						case "switch":
							expectedPaths = "_scratch/first.txt\n_scratch/second.txt\n"
						}
					}
					require.Equal(testInstance, expectedPaths, runGit(testInstance, repo, "diff", "--cached", "--name-only"))
					requireSyncIndexUnlocked(testInstance, repo)
					return
				}
				require.NoError(testInstance, err, output)
				require.Contains(testInstance, output, "SYNCED:")
				require.Contains(testInstance, readTextFile(testInstance, marker), "index.lock")
				require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repo, "status", "--porcelain")))
				for _, name := range []string{"_scratch/first.txt", "_scratch/second.txt", "outside.txt"} {
					require.Equal(testInstance, name+" contents\n", runGit(testInstance, remote, "show", "master:"+name))
				}
				first := runGit(testInstance, repo, "show", "--format=", "--name-only", "HEAD~1")
				require.Contains(testInstance, first, "_scratch/first.txt")
				require.Contains(testInstance, first, "_scratch/second.txt")
				require.NotContains(testInstance, first, "outside.txt")
				requireSyncIndexUnlocked(testInstance, repo)
			})
		}
	}
}

func TestSyncSkipsWorkAlreadyPresentInDestination(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	root := testInstance.TempDir()
	remotePath, repositoryPath := filepath.Join(root, "remote.git"), filepath.Join(root, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)
	filePath := filepath.Join(repositoryPath, "README.md")
	require.NoError(testInstance, os.WriteFile(filePath, []byte("initial\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "initial")
	runGit(testInstance, repositoryPath, "branch", "feature/source")
	require.NoError(testInstance, os.WriteFile(filePath, []byte("already applied\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "apply destination change")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")
	destinationCommit := runGit(testInstance, repositoryPath, "rev-parse", "HEAD")
	runGit(testInstance, repositoryPath, "switch", "feature/source")
	require.NoError(testInstance, os.WriteFile(filePath, []byte("already applied\n"), 0o644))
	var mutex sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		requests++
		mutex.Unlock()
		http.Error(responseWriter, "no model request is needed", http.StatusBadRequest)
	}))
	testInstance.Cleanup(server.Close)
	config := writeDirtySyncMergedBranchConfiguration(testInstance, server.URL)
	output, err := runBinaryIntegrationCommand(testInstance, binaryPath, repositoryPath, map[string]string{
		pathEnvironmentVariableNameConstant: buildSyncMergedBranchExecutablePath(testInstance),
	}, syncStateTransitionTimeout, []string{"--config", config, "--roots", repositoryPath, "sync", "master"})
	require.NoError(testInstance, err, output)
	require.Contains(testInstance, output, "SYNCED:")
	require.Equal(testInstance, destinationCommit, runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))
	require.Equal(testInstance, "already applied\n", readTextFile(testInstance, filePath))
	mutex.Lock()
	defer mutex.Unlock()
	require.Zero(testInstance, requests)
}

// Keep the injected command at the Git boundary; all sync logic remains real.
func syncPrepareInterferenceScript() string {
	return `
if [ -n "$GIX_SYNC_TEST_PREPARE_PHASE" ] && [ ! -f "$GIX_SYNC_TEST_PREPARE_MARKER" ]; then
  inject=false
  if [ "$1" = "add" ]; then
    case "$*" in
      *"--all -- _scratch"*)
        "$real_git_path" "$@" || exit $?
        touch "$GIX_SYNC_TEST_PREPARE_MARKER.ready"
        if [ "$GIX_SYNC_TEST_PREPARE_PHASE" = "after_add" ]; then inject=true; else exit 0; fi
        ;;
    esac
  elif [ "$1" = "ls-files" ] && [ -f "$GIX_SYNC_TEST_PREPARE_MARKER.ready" ]; then
    if [ "$GIX_SYNC_TEST_PREPARE_PHASE" = "before_checkpoint" ]; then
      inject=true
    elif [ "$GIX_SYNC_TEST_PREPARE_PHASE" = "after_publish" ]; then
      live_index="$(env -u GIT_INDEX_FILE "$real_git_path" rev-parse --git-path index)"
      if [ ! -f "$live_index.lock" ]; then inject=true; fi
    fi
  fi
  if [ "$inject" = "true" ]; then
    case "$GIX_SYNC_TEST_PREPARE_MUTATION" in
      reset) env -u GIT_INDEX_FILE "$real_git_path" reset >"$GIX_SYNC_TEST_PREPARE_MARKER" 2>&1 ;;
      partial) env -u GIT_INDEX_FILE "$real_git_path" reset -- _scratch/second.txt >"$GIX_SYNC_TEST_PREPARE_MARKER" 2>&1 ;;
      add) env -u GIT_INDEX_FILE "$real_git_path" add -- outside.txt >"$GIX_SYNC_TEST_PREPARE_MARKER" 2>&1 ;;
      switch) env -u GIT_INDEX_FILE "$real_git_path" switch -c operator-change >"$GIX_SYNC_TEST_PREPARE_MARKER" 2>&1 ;;
    esac
    if [ "$1" = "add" ]; then exit 0; fi
  fi
fi
`
}
