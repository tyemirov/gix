package tests

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncBoundsDestinationContext(t *testing.T) {
	const requestLimit = 64 * 1024
	const placementLimit = 32 * 1024
	const conflictPath = "settings.txt"
	padding := strings.Repeat("unchanged_setting = café λ\n", 40_000)
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, fixture := range []struct{ name, before, after, expandPhase string }{
		{name: "large_prefix", before: padding},
		{name: "large_suffix", after: padding},
		{name: "large_both_boundaries", before: padding, after: padding},
		{name: "expand_during_decision", before: padding, after: padding, expandPhase: "resolve"},
		{name: "expand_during_review", before: padding, after: padding, expandPhase: "review"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			workspace := syncHomeWorkspace(t)
			remote, repository := filepath.Join(workspace, "remote.git"), filepath.Join(workspace, "project")
			createSyncGitHubBackedRepository(t, remote, repository)
			path := filepath.Join(repository, conflictPath)
			base := fixture.before + "mode = 1\nlimit = 5\n" + fixture.after
			ours := fixture.before + "mode = 2\nlimit = 5\n" + fixture.after
			theirs := fixture.before + "mode = 3\nlimit = 7\n" + fixture.after
			expected := fixture.before + "mode = 2\nlimit = 7\n" + fixture.after
			require.NoError(t, os.WriteFile(path, []byte(base), 0o644))
			runGit(t, repository, "add", ".")
			runGit(t, repository, "commit", "-m", "base")
			runGit(t, repository, "push", "-u", "origin", "master")
			runGit(t, repository, "switch", "-c", "incoming")
			require.NoError(t, os.WriteFile(path, []byte(theirs), 0o644))
			runGit(t, repository, "commit", "-am", "incoming")
			runGit(t, repository, "push", "origin", "HEAD:master")
			runGit(t, repository, "switch", "master")
			require.NoError(t, os.WriteFile(path, []byte(ours), 0o644))
			runGit(t, repository, "commit", "-am", "local")

			var expanded atomic.Bool
			var requests, reviews, boundedBytes atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				if !expanded.Load() {
					boundedBytes.Store(int64(max(int(boundedBytes.Load()), len(body))))
					if len(body) > requestLimit {
						http.Error(w, fmt.Sprintf("provider input exceeds %d bytes: %d", requestLimit, len(body)), http.StatusRequestEntityTooLarge)
						return
					}
				}
				input, err := decodeMergePlanInputForTest(body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				if err := checkMergePlanPlacementForTest(input.Placement, fixture.before, fixture.after); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				if expanded.Load() {
					if input.Placement.Before != fixture.before || input.Placement.After != fixture.after ||
						input.Context.Scope != "file" || input.Context.Base != base || input.Context.Ours != ours || input.Context.Theirs != theirs {
						http.Error(w, "context request did not supply the complete source and destination", http.StatusBadRequest)
						return
					}
				} else if len(input.Placement.Before)+len(input.Placement.After) > placementLimit {
					http.Error(w, "initial destination context exceeds its budget", http.StatusBadRequest)
					return
				}
				response := ""
				if !expanded.Load() && input.Phase == fixture.expandPhase {
					expanded.Store(true)
					response = `{"status":"needs_context","reason":"Read the omitted source and destination text."}`
				} else {
					response, err = mergePlanSourceFixtureResponse(body, expected)
					if err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					if input.Phase == "review" {
						reviews.Add(1)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, response)
			}))
			t.Cleanup(server.Close)
			configuration := writeReportedLifecycleSemanticConfiguration(t, server.URL)
			output, err := runBinaryIntegrationCommand(t, binary, repository, map[string]string{
				pathEnvironmentVariableNameConstant: buildSyncMergedBranchExecutablePath(t),
				syncMergedBranchGitLogVariable:      filepath.Join(workspace, "git.log"),
				syncMergedBranchGitHubLogVariable:   filepath.Join(workspace, "gh.log"),
				syncMergedBranchNameVariable:        "master",
				syncMergedBranchMergedVariable:      "false",
			}, syncStateTransitionTimeout, []string{"--config", configuration, "--roots", repository, "sync", "master"})
			t.Logf("Largest initial request: %d bytes; requests: %d", boundedBytes.Load(), requests.Load())
			require.NoError(t, err, output)
			require.Equal(t, expected, readTextFile(t, path))
			require.Equal(t, expected, runGit(t, remote, "show", "master:"+conflictPath))
			require.Empty(t, runGit(t, repository, "status", "--porcelain"))
			require.Empty(t, runGit(t, repository, "stash", "list"))
			require.EqualValues(t, 1, reviews.Load())
			expectedRequests := int64(2)
			if fixture.expandPhase != "" {
				expectedRequests++
				require.True(t, expanded.Load())
			}
			require.Equal(t, expectedRequests, requests.Load())
		})
	}
}
