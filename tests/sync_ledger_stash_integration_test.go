package tests

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	ledgerReplayFixture = "tests/testdata/merge-contract/ledger-20260911"
	ledgerReplayBranch  = "tyemirov/B003-mpr-ui-migration"
)

var ledgerReplayIntentPaths = []string{
	"internal/controlplane/web/config-ui.yaml",
	"internal/controlplane/web/js/profile.js",
}

type ledgerReplayReferences struct {
	Source       string `json:"source"`
	TargetBefore string `json:"target-before"`
	Target       string `json:"target"`
	Stash        string `json:"stash"`
}

type ledgerReplaySnapshot struct {
	Files  map[string]string `json:"files"`
	Index  string            `json:"index"`
	Status string            `json:"status"`
}

func TestSyncLedgerStashReplay(t *testing.T) {
	runLedgerStashReplay(t, "")
}

func TestSyncLedgerStashProviderEvaluation(t *testing.T) {
	configuration := os.Getenv("GIX_MERGE_EVAL_CONFIG")
	if configuration == "" {
		t.Skip("set GIX_MERGE_EVAL_CONFIG to qualify the complete Ledger case")
	}
	absolute, err := filepath.Abs(configuration)
	require.NoError(t, err)
	runLedgerStashReplay(t, absolute)
}

func runLedgerStashReplay(t *testing.T, providerConfiguration string) {
	t.Helper()
	root := integrationRepositoryRoot(t)
	fixture := filepath.Join(root, ledgerReplayFixture)
	var references ledgerReplayReferences
	var initial, expected ledgerReplaySnapshot
	for name, destination := range map[string]any{"refs.json": &references, "initial.json": &initial, "expected.json": &expected} {
		require.NoError(t, json.Unmarshal([]byte(readTextFile(t, filepath.Join(fixture, name))), destination))
	}
	workspace := syncHomeWorkspace(t)
	remote := filepath.Join(workspace, "remote.git")
	repository := filepath.Join(workspace, "ledger")
	bundle := filepath.Join(fixture, "input.bundle")
	runGitWithDir(t, "", "init", "--bare", remote)
	runGit(t, remote, "symbolic-ref", "HEAD", "refs/heads/master")
	runGit(t, remote, "fetch", bundle, "refs/heads/target:refs/heads/master", "refs/heads/source:refs/heads/"+ledgerReplayBranch)
	runGitWithDir(t, "", "clone", "--branch", ledgerReplayBranch, remote, repository)
	configureGitIdentity(t, repository)
	runGit(t, repository, "fetch", bundle, "refs/heads/stash:refs/fixtures/stash")
	runGit(t, repository, "branch", "master", references.TargetBefore)
	runGit(t, repository, "config", "branch.master.remote", "origin")
	runGit(t, repository, "config", "branch.master.merge", "refs/heads/master")
	runGit(t, repository, "stash", "apply", "--index", references.Stash)
	runGit(t, repository, append([]string{"reset", "--"}, ledgerReplayIntentPaths...)...)
	runGit(t, repository, append([]string{"add", "--intent-to-add", "--"}, ledgerReplayIntentPaths...)...)
	require.Equal(t, initial, captureLedgerReplaySnapshot(t, repository), "complete original Ledger state")
	require.Equal(t, references.Source, strings.TrimSpace(runGit(t, repository, "rev-parse", "HEAD")))
	require.Equal(t, references.Target, strings.TrimSpace(runGit(t, repository, "rev-parse", "origin/master")))
	remoteBefore := runGit(t, remote, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads")

	expectedConflicts := map[string]string{}
	for _, path := range []string{".mprlab/ISSUES.md", "internal/controlplane/server.go"} {
		expectedConflicts[path] = readTextFile(t, filepath.Join(fixture, "expected", path)+".txt")
	}
	reviews := map[string]*atomic.Int64{}
	for path := range expectedConflicts {
		reviews[path] = &atomic.Int64{}
	}
	configuration := providerConfiguration
	if configuration == "" {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			input, err := decodeMergePlanInputForTest(body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			content, known := expectedConflicts[input.Path]
			if !known {
				http.Error(w, "unexpected Ledger conflict path: "+input.Path, http.StatusBadRequest)
				return
			}
			if input.Phase == "review" {
				if input.Candidate == nil || input.Placement.Before+input.Candidate.Content+input.Placement.After != content {
					http.Error(w, "Ledger audit lacks the exact complete expected file", http.StatusBadRequest)
					return
				}
				reviews[input.Path].Add(1)
			}
			response, err := mergePlanSourceFixtureResponse(body, content)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, response)
		}))
		t.Cleanup(server.Close)
		configuration = writeReportedLifecycleSemanticConfiguration(t, server.URL)
	}
	binary := buildIntegrationBinary(t, root)
	executionTimeout := syncStateTransitionTimeout
	if providerConfiguration != "" {
		executionTimeout = 5 * time.Minute
	}
	gitLog := filepath.Join(workspace, "git.log")
	githubLog := filepath.Join(workspace, "gh.log")
	output, err := runBinaryIntegrationCommand(t, binary, repository, map[string]string{
		pathEnvironmentVariableNameConstant: buildSyncMergedBranchExecutablePath(t),
		syncMergedBranchGitLogVariable:      gitLog,
		syncMergedBranchGitHubLogVariable:   githubLog,
		syncMergedBranchNameVariable:        ledgerReplayBranch,
		syncMergedBranchMergedVariable:      "true",
	}, executionTimeout, []string{"--config", configuration, "--roots", repository, "sync", "master", "--stash"})
	t.Logf("Complete Ledger CLI replay:\n%s", output)
	require.NoError(t, err, output)
	require.Contains(t, output, "detected 2 conflicted paths")
	for path, content := range expectedConflicts {
		require.Equal(t, content, readTextFile(t, filepath.Join(repository, path)), path)
		require.Contains(t, output, "validated all conflict regions for "+path)
		if providerConfiguration == "" {
			require.EqualValues(t, 1, reviews[path].Load(), path)
		}
	}
	require.Equal(t, expected, captureLedgerReplaySnapshot(t, repository), "complete resolved Ledger state")
	require.Equal(t, "master\n", runGit(t, repository, "branch", "--show-current"))
	require.Equal(t, references.Target, strings.TrimSpace(runGit(t, repository, "rev-parse", "HEAD")))
	require.Equal(t, references.Source, strings.TrimSpace(runGit(t, repository, "rev-parse", ledgerReplayBranch)))
	require.Equal(t, remoteBefore, runGit(t, remote, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads"))
	require.Empty(t, runGit(t, repository, "stash", "list"))
	require.Empty(t, runGit(t, repository, "diff", "--name-only", "--diff-filter=U"))
	require.Empty(t, runGit(t, repository, "ls-files", "--others", "--exclude-standard"))
	visible := strings.Fields(runGit(t, repository, "diff", "--cached", "--name-only", "--ita-visible-in-index"))
	invisible := strings.Fields(runGit(t, repository, "diff", "--cached", "--name-only", "--ita-invisible-in-index"))
	intent := []string{}
	for _, path := range visible {
		if !containsLedgerReplayPath(invisible, path) {
			intent = append(intent, path)
		}
	}
	require.Equal(t, ledgerReplayIntentPaths, intent)
	gitOperations := readTextFile(t, gitLog)
	appliedStash := ""
	for _, operation := range strings.Split(gitOperations, "\n") {
		if reference, ok := strings.CutPrefix(operation, "stash apply --index "); ok {
			appliedStash = reference
		}
	}
	require.NotEmpty(t, appliedStash)
	require.Equal(t, references.Source, strings.TrimSpace(runGit(t, repository, "rev-parse", appliedStash+"^1")))
	for _, suffix := range []string{"^{tree}", "^2^{tree}", "^3^{tree}"} {
		require.Equal(t, runGit(t, repository, "rev-parse", references.Stash+suffix), runGit(t, repository, "rev-parse", appliedStash+suffix), "original stash input "+suffix)
	}
	require.NotContains(t, readTextFile(t, githubLog), "created-pr")
	t.Logf("Verified %d complete file contents, index entries, both intent-to-add paths, source and target refs, and unchanged remote refs.", len(expected.Files))
}

func captureLedgerReplaySnapshot(t *testing.T, repository string) ledgerReplaySnapshot {
	t.Helper()
	files := map[string]string{}
	for _, path := range strings.Split(strings.TrimSuffix(runGit(t, repository, "ls-files", "-z"), "\x00"), "\x00") {
		content, err := os.ReadFile(filepath.Join(repository, path))
		require.NoError(t, err)
		files[path] = fmt.Sprintf("%x", sha256.Sum256(content))
	}
	return ledgerReplaySnapshot{Files: files, Index: runGit(t, repository, "ls-files", "--format=%(path)\t%(objectname)"), Status: runGit(t, repository, "status", "--porcelain")}
}

func containsLedgerReplayPath(paths []string, wanted string) bool {
	for _, path := range paths {
		if path == wanted {
			return true
		}
	}
	return false
}
