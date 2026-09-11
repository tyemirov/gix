package tests

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncIntentToAdd(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	for _, testCase := range []struct {
		Name       string
		Stash      bool
		FailMatch  string
		Occurrence string
		Missing    bool
		Ignored    bool
	}{
		{Name: "stash_restores_intent_and_mixed_changes", Stash: true},
		{Name: "commit_publishes_intent_files"},
		{Name: "snapshot_failure_preserves_index", Stash: true, FailMatch: "stash push", Occurrence: "1"},
		{Name: "invocation_failure_restores_index", Stash: true, FailMatch: "stash push", Occurrence: "2"},
		{Name: "switch_failure_restores_index", Stash: true, FailMatch: "switch --no-guess master", Occurrence: "1"},
		{Name: "missing_intent_file_preserves_state", Stash: true, Missing: true},
		{Name: "ignored_intent_commit_failure_preserves_state", FailMatch: "commit -m", Occurrence: "2", Ignored: true},
	} {
		testInstance.Run(testCase.Name, func(testInstance *testing.T) {
			remotePath, repositoryPath := createSyncStateTransitionRepository(testInstance)
			runGit(testInstance, repositoryPath, "switch", "-c", syncStateTransitionBranchName)
			writeFile := func(name, content string) {
				require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, name), []byte(content), 0o644))
			}
			writeFile("saved.txt", "existing stash\n")
			runGit(testInstance, repositoryPath, "stash", "push", "-u", "-m", "operator-existing")
			files := map[string]string{
				"README.md":          "staged change\nunstaged follow-up\n",
				"new config.yaml":    "enabled: true\n",
				"empty.txt":          "",
				"literal[1]\nnew.js": "export const enabled = true;\n",
				"staged-empty.txt":   "",
				"untracked.txt":      "untracked content\n",
			}
			writeFile("README.md", "staged change\n")
			runGit(testInstance, repositoryPath, "add", "README.md")
			for name, content := range files {
				writeFile(name, content)
			}
			if testCase.Ignored {
				require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, ".git", "info", "exclude"), []byte("new config.yaml\n"), 0o644))
				require.NoError(testInstance, os.Mkdir(filepath.Join(repositoryPath, "zlater"), 0o755))
				files["zlater/state.go"] = "package state\n"
				writeFile("zlater/state.go", files["zlater/state.go"])
			}
			runGit(testInstance, repositoryPath, "--literal-pathspecs", "add", "-N", "-f", "--", "new config.yaml", "empty.txt", "literal[1]\nnew.js")
			runGit(testInstance, repositoryPath, "add", "staged-empty.txt")
			if testCase.Missing {
				require.NoError(testInstance, os.Remove(filepath.Join(repositoryPath, "empty.txt")))
				delete(files, "empty.txt")
			}
			statusBefore := runGit(testInstance, repositoryPath, "status", "--porcelain=v1", "-z", "--untracked-files=all")
			stashesBefore := runGit(testInstance, repositoryPath, "stash", "list", "--format=%H %s")
			headBefore := runGit(testInstance, repositoryPath, "rev-parse", "HEAD")
			visibleBefore := runGit(testInstance, repositoryPath, "diff", "--cached", "--name-only", "--ita-visible-in-index", "-z")
			stagedBefore := runGit(testInstance, repositoryPath, "diff", "--cached", "--name-only", "--ita-invisible-in-index", "-z")
			llmServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fix: preserve new files"}}]}`))
			}))
			testInstance.Cleanup(llmServer.Close)
			configurationPath := writeDirtySyncMergedBranchConfiguration(testInstance, llmServer.URL)
			gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
			environment := map[string]string{
				pathEnvironmentVariableNameConstant:       buildSyncMergedBranchExecutablePath(testInstance),
				syncMergedBranchAPIKeyVariable:            "test-key",
				syncMergedBranchGitLogVariable:            gitLogPath,
				syncMergedBranchNameVariable:              "master",
				syncMergedBranchMergedVariable:            "false",
				syncMergedBranchFailGitMatchVariable:      testCase.FailMatch,
				syncMergedBranchFailGitOccurrenceVariable: testCase.Occurrence,
				syncMergedBranchFailGitStateVariable:      filepath.Join(testInstance.TempDir(), "failure-count"),
			}
			arguments := []string{"--config", configurationPath, "--log-level", "error", "--roots", repositoryPath, "sync", "master"}
			if testCase.Stash {
				arguments = append(arguments, "--stash")
			}
			output, runError := runBinaryIntegrationCommand(testInstance, binaryPath, repositoryPath, environment, syncStateTransitionTimeout, arguments)
			if testCase.FailMatch != "" || testCase.Missing {
				require.Error(testInstance, runError, output)
				if testCase.Missing {
					require.Contains(testInstance, output, "inspect intent-to-add file")
					require.Contains(testInstance, output, "empty.txt")
					require.NoFileExists(testInstance, filepath.Join(repositoryPath, "empty.txt"))
				} else {
					require.Contains(testInstance, output, "simulated git failure")
				}
				require.Equal(testInstance, syncStateTransitionBranchName, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
				require.Equal(testInstance, headBefore, runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
			} else {
				require.NoError(testInstance, runError, output)
				require.Contains(testInstance, output, "SYNCED:")
				require.Equal(testInstance, "master", strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--show-current")))
			}
			for name, content := range files {
				require.Equal(testInstance, content, readTextFile(testInstance, filepath.Join(repositoryPath, name)), name)
			}
			require.Equal(testInstance, stashesBefore, runGit(testInstance, repositoryPath, "stash", "list", "--format=%H %s"))
			requireSyncIndexUnlocked(testInstance, repositoryPath)
			if testCase.Stash || testCase.FailMatch != "" {
				require.Equal(testInstance, statusBefore, runGit(testInstance, repositoryPath, "status", "--porcelain=v1", "-z", "--untracked-files=all"))
				require.Equal(testInstance, visibleBefore, runGit(testInstance, repositoryPath, "diff", "--cached", "--name-only", "--ita-visible-in-index", "-z"))
				require.Equal(testInstance, stagedBefore, runGit(testInstance, repositoryPath, "diff", "--cached", "--name-only", "--ita-invisible-in-index", "-z"))
				require.Equal(testInstance, "staged change\n", runGit(testInstance, repositoryPath, "show", ":README.md"))
				require.Equal(testInstance, headBefore, runGit(testInstance, remotePath, "rev-parse", "refs/heads/master"))
			} else {
				require.Empty(testInstance, runGit(testInstance, repositoryPath, "status", "--porcelain"))
				for name, content := range files {
					require.Equal(testInstance, content, runGit(testInstance, remotePath, "show", "refs/heads/master:"+name), name)
				}
			}
		})
	}
}
