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

func TestSyncStashUntrackedDestinationCollisions(t *testing.T) {
	t.Parallel()
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, scenario := range []struct {
		name, path, local, incoming, expected    string
		failure, providerFailure, cleanupFailure bool
		allCollisions                            bool
		prefixCollision                          string
		transformedDiff, links                   bool
		removeResidual                           bool
	}{
		{name: "identical", path: "docs/ci.md", local: "shared\n", incoming: "shared\n", expected: "shared\n"},
		{name: "different", path: "docs/ci.md", local: "local requirement\n", incoming: "remote requirement\n", expected: "remote requirement\nlocal requirement\n"},
		{name: "literal path", path: "docs/ci [draft]*.md", local: "local requirement\n", incoming: "remote requirement\n", expected: "remote requirement\nlocal requirement\n"},
		{name: "binary identical", path: "asset.bin", local: "shared\x00bytes", incoming: "shared\x00bytes", expected: "shared\x00bytes"},
		{name: "binary conflict", path: "asset.bin", local: "local\x00bytes", incoming: "remote\x00bytes", failure: true},
		{name: "provider failure", path: "docs/ci.md", local: "local requirement\n", incoming: "remote requirement\n", failure: true, providerFailure: true},
		{name: "all collide", path: "docs/ci.md", local: "local requirement\n", incoming: "remote requirement\n", expected: "remote requirement\nlocal requirement\n", allCollisions: true},
		{name: "inherited whitespace", path: "docs/ci.md", local: "local requirement  \n", incoming: "remote requirement\n", expected: "remote requirement\nlocal requirement  \n"},
		{name: "cleanup failure", path: "docs/ci.md", local: "local requirement\n", incoming: "remote requirement\n", expected: "remote requirement\nlocal requirement\n", failure: true, cleanupFailure: true},
		{name: "directory into file", path: "docs/ci.md", local: "local requirement\n", incoming: "remote requirement\n", failure: true, prefixCollision: "nested"},
		{name: "file into directory", path: "docs/ci.md", local: "local requirement\n", incoming: "remote requirement\n", failure: true, prefixCollision: "nested/child.txt"},
		{name: "configured diff transforms", path: "docs/ci.md", local: "local requirement\n", incoming: "remote requirement\n", expected: "remote requirement\nlocal requirement\n", transformedDiff: true},
		{name: "identical links", path: "link.txt", links: true},
		{name: "different links", path: "link.txt", links: true, failure: true},
		{name: "residual restoration failure", path: "docs/ci.md", local: "local requirement\n", incoming: "remote requirement\n", expected: "remote requirement\nlocal requirement\n", failure: true, removeResidual: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			const sourceBranch = "gix/merged-review"
			fixture := createSyncMergedBranchFixture(t, sourceBranch)
			repository := fixture.RepositoryPath
			write := func(path, content string) {
				t.Helper()
				absolute := filepath.Join(repository, path)
				require.NoError(t, os.MkdirAll(filepath.Dir(absolute), 0o755))
				require.NoError(t, os.WriteFile(absolute, []byte(content), 0o644))
			}
			var outsidePaths []string
			if scenario.links {
				outside := t.TempDir()
				scenario.incoming = filepath.Join(outside, "incoming.txt")
				scenario.local = scenario.incoming
				if scenario.failure {
					scenario.local = filepath.Join(outside, "local.txt")
				}
				scenario.expected = scenario.incoming
				for _, path := range []string{scenario.incoming, scenario.local} {
					require.NoError(t, os.WriteFile(path, []byte("outside sentinel\n"), 0o644))
					outsidePaths = append(outsidePaths, path)
				}
			}
			writeCollision := func(content string) {
				t.Helper()
				if scenario.links {
					require.NoError(t, os.Symlink(content, filepath.Join(repository, scenario.path)))
				} else {
					write(scenario.path, content)
				}
			}
			readCollision := func() string {
				t.Helper()
				if scenario.links {
					target, err := os.Readlink(filepath.Join(repository, scenario.path))
					require.NoError(t, err)
					return target
				}
				return readTextFile(t, filepath.Join(repository, scenario.path))
			}
			runGit(t, repository, "switch", "master")
			writeCollision(scenario.incoming)
			if scenario.prefixCollision != "" {
				write(scenario.prefixCollision, "remote path\n")
				runGit(t, repository, "add", "--", scenario.prefixCollision)
			}
			runGit(t, repository, "add", "--", scenario.path)
			runGit(t, repository, "commit", "-m", "publish incoming file")
			runGit(t, repository, "push", "origin", "master")
			targetCommit := strings.TrimSpace(runGit(t, repository, "rev-parse", "HEAD"))
			runGit(t, repository, "switch", sourceBranch)
			sourceCommit := strings.TrimSpace(runGit(t, repository, "rev-parse", "HEAD"))
			write("operator.txt", "existing operator stash\n")
			runGit(t, repository, "stash", "push", "-u", "-m", "operator stash")
			existingStashes := runGit(t, repository, "stash", "list", "--format=%H")
			writeCollision(scenario.local)
			write("README.md", "staged requirement\n")
			runGit(t, repository, "add", "README.md")
			write("README.md", "staged requirement\nunstaged requirement\n")
			write("intent.txt", "intent requirement\n")
			runGit(t, repository, "add", "-N", "intent.txt")
			if !scenario.allCollisions {
				write("pending.txt", "untracked requirement\n")
				require.NoError(t, os.Symlink("pending.txt", filepath.Join(repository, "pending-link")))
			}
			if scenario.prefixCollision == "nested" {
				write("nested/child.txt", "local path\n")
			} else if scenario.prefixCollision != "" {
				write("nested", "local path\n")
			}
			if scenario.transformedDiff {
				write(".git/info/attributes", "README.md diff=collision\n")
				runGit(t, repository, "config", "diff.collision.textconv", "printf converted")
				runGit(t, repository, "config", "color.diff", "always")
				runGit(t, repository, "config", "diff.noprefix", "true")
			}
			initialStatus := runGit(t, repository, "status", "--porcelain")
			initialIndex := runGit(t, repository, "ls-files", "--format=%(path)\t%(objectname)\t%(stage)")
			remoteBefore := runGit(t, fixture.RemotePath, "for-each-ref", "--format=%(refname) %(objectname)")
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if requests.Add(1) == 1 && scenario.removeResidual {
					if err := os.Remove(filepath.Join(repository, "pending.txt")); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return
					}
				}
				if scenario.providerFailure {
					http.Error(w, "provider unavailable", http.StatusForbidden)
					return
				}
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
				response := semanticMergeResponse(scenario.expected)
				if input.Phase == "review" {
					if input.Candidate == nil || input.Candidate.Content != scenario.expected {
						http.Error(w, "unexpected resolved contents", http.StatusBadRequest)
						return
					}
					response = mergePlanApprovedForTest
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, response)
			}))
			t.Cleanup(server.Close)
			configuration := writeReportedLifecycleSemanticConfiguration(t, server.URL)
			gitLog := filepath.Join(t.TempDir(), "git.log")
			environment := map[string]string{
				pathEnvironmentVariableNameConstant: buildSyncMergedBranchExecutablePath(t),
				syncMergedBranchGitLogVariable:      gitLog,
				syncMergedBranchGitHubLogVariable:   filepath.Join(t.TempDir(), "gh.log"),
				syncMergedBranchNameVariable:        sourceBranch,
				syncMergedBranchMergedVariable:      "true",
			}
			if scenario.cleanupFailure {
				environment[syncMergedBranchFailGitMatchVariable] = "stash drop"
				environment[syncMergedBranchFailGitOccurrenceVariable] = "1"
				environment[syncMergedBranchFailGitStateVariable] = filepath.Join(t.TempDir(), "failure-count")
			}
			output, err := runBinaryIntegrationCommand(t, binary, repository, environment, syncStateTransitionTimeout, []string{"--config", configuration, "--roots", repository, "sync", "master", "--stash"})
			t.Log(output)
			for _, path := range outsidePaths {
				require.Equal(t, "outside sentinel\n", readTextFile(t, path), path)
			}
			require.Equal(t, remoteBefore, runGit(t, fixture.RemotePath, "for-each-ref", "--format=%(refname) %(objectname)"))
			require.Equal(t, sourceCommit, strings.TrimSpace(runGit(t, repository, "rev-parse", sourceBranch)))
			require.Equal(t, "staged requirement\nunstaged requirement\n", readTextFile(t, filepath.Join(repository, "README.md")))
			require.Equal(t, "staged requirement\n", runGit(t, repository, "show", ":README.md"))
			require.Equal(t, "intent requirement\n", readTextFile(t, filepath.Join(repository, "intent.txt")))
			require.Contains(t, runGit(t, repository, "diff", "--cached", "--name-only", "--ita-visible-in-index"), "intent.txt")
			require.NotContains(t, runGit(t, repository, "diff", "--cached", "--name-only", "--ita-invisible-in-index"), "intent.txt")
			if !scenario.allCollisions {
				require.Equal(t, "untracked requirement\n", readTextFile(t, filepath.Join(repository, "pending.txt")))
				link, linkErr := os.Readlink(filepath.Join(repository, "pending-link"))
				require.NoError(t, linkErr)
				require.Equal(t, "pending.txt", link)
			}
			require.Empty(t, runGit(t, repository, "diff", "--name-only", "--diff-filter=U"))
			if scenario.failure {
				require.Error(t, err, output)
				require.Contains(t, output, "SYNC_SWITCH_ROLLBACK")
				require.Equal(t, sourceBranch+"\n", runGit(t, repository, "branch", "--show-current"))
				require.Equal(t, initialStatus, runGit(t, repository, "status", "--porcelain"))
				require.Equal(t, initialIndex, runGit(t, repository, "ls-files", "--format=%(path)\t%(objectname)\t%(stage)"))
				require.Equal(t, scenario.local, readCollision())
				if scenario.providerFailure {
					require.Positive(t, requests.Load(), output)
				} else if scenario.links {
					require.Contains(t, output, "symbolic link conflict")
					require.Zero(t, requests.Load())
				} else if scenario.prefixCollision != "" {
					require.Contains(t, output, "conflicts with destination tracked path")
					require.Zero(t, requests.Load())
				} else if scenario.removeResidual {
					require.Contains(t, output, "verify restored strict sync untracked files")
				} else if scenario.cleanupFailure {
					require.Contains(t, output, "simulated git failure")
				} else {
					require.Contains(t, output, "binary conflict")
				}
				require.Equal(t, existingStashes, runGit(t, repository, "stash", "list", "--format=%H"))
			} else {
				require.NoError(t, err, output)
				require.Contains(t, output, "SYNCED:")
				require.Equal(t, "master\n", runGit(t, repository, "branch", "--show-current"))
				require.Equal(t, targetCommit, strings.TrimSpace(runGit(t, repository, "rev-parse", "HEAD")))
				require.Equal(t, scenario.expected, readCollision())
				require.Equal(t, scenario.incoming, runGit(t, repository, "show", ":"+scenario.path))
				require.Equal(t, existingStashes, runGit(t, repository, "stash", "list", "--format=%H"))
				expectedUntracked := "pending-link\npending.txt\n"
				if scenario.allCollisions {
					expectedUntracked = ""
				}
				require.Equal(t, expectedUntracked, runGit(t, repository, "ls-files", "--others", "--exclude-standard"))
				if scenario.local == scenario.incoming {
					require.Zero(t, requests.Load())
				} else {
					require.Positive(t, requests.Load())
				}
			}
		})
	}
}
