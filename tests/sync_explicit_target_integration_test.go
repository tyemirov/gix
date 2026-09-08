package tests

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	explicitSyncSource  = "b1"
	explicitSyncTarget  = "b2"
	explicitSyncDefault = "qqq"
)

type explicitSyncTargetLocation string

const (
	explicitSyncLocalRemote explicitSyncTargetLocation = "local_and_remote"
	explicitSyncLocalOnly   explicitSyncTargetLocation = "local_only"
	explicitSyncRemoteOnly  explicitSyncTargetLocation = "remote_only"
	explicitSyncNew         explicitSyncTargetLocation = "new"
)

type explicitSyncFixture struct {
	syncFixture
	sourceHead  string
	targetHead  string
	defaultHead string
}

func newExplicitSyncFixture(t *testing.T, location explicitSyncTargetLocation) explicitSyncFixture {
	t.Helper()
	fixture := explicitSyncFixture{syncFixture: newSyncFixture(t, explicitSyncDefault)}
	fixture.commitFile(t, "remove.txt", "remove this file\n")
	fixture.commitFile(t, "rename.txt", "preserve renamed contents\n")
	fixture.commitFile(t, "binary.dat", "initial\x00bytes")
	runGit(t, fixture.repository, "push", "origin", explicitSyncDefault)
	fixture.defaultHead = strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
	if location != explicitSyncNew {
		runGit(t, fixture.repository, "switch", "-c", explicitSyncTarget)
		fixture.commitFile(t, "target-only.txt", "existing target work\n")
		fixture.targetHead = strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
		if location != explicitSyncLocalOnly {
			runGit(t, fixture.repository, "push", "-u", "origin", explicitSyncTarget)
		}
	}
	runGit(t, fixture.repository, "switch", "-c", explicitSyncSource, explicitSyncDefault)
	fixture.commitFile(t, "source-only.txt", "existing source work\n")
	runGit(t, fixture.repository, "push", "-u", "origin", explicitSyncSource)
	fixture.sourceHead = strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
	if location == explicitSyncRemoteOnly {
		runGit(t, fixture.repository, "branch", "-D", explicitSyncTarget)
	}
	return fixture
}

func (fixture explicitSyncFixture) assertPublished(t *testing.T, location explicitSyncTargetLocation) {
	t.Helper()
	require.Equal(t, explicitSyncTarget, strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
	head := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
	require.Equal(t, head, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", explicitSyncTarget)))
	require.Empty(t, runGit(t, fixture.repository, "status", "--porcelain"))
	for _, repository := range []string{fixture.repository, fixture.remote} {
		require.Equal(t, fixture.sourceHead, strings.TrimSpace(runGit(t, repository, "rev-parse", explicitSyncSource)))
		require.Equal(t, fixture.defaultHead, strings.TrimSpace(runGit(t, repository, "rev-parse", explicitSyncDefault)))
		require.Equal(t, []string{explicitSyncSource, explicitSyncTarget, explicitSyncDefault}, strings.Fields(runGit(t, repository, "for-each-ref", "--format=%(refname:short)", "refs/heads/")))
	}
	if location == explicitSyncNew {
		runGit(t, fixture.repository, "merge-base", "--is-ancestor", fixture.sourceHead, head)
		require.Equal(t, "existing source work\n", runGit(t, fixture.remote, "show", "b2:source-only.txt"))
		require.Contains(t, readTextFile(t, fixture.githubLog), "created-pr --base b1 --head b2 ")
	} else {
		runGit(t, fixture.repository, "merge-base", "--is-ancestor", fixture.targetHead, head)
		require.Equal(t, "existing target work\n", runGit(t, fixture.remote, "show", "b2:target-only.txt"))
		require.NotContains(t, strings.Fields(runGit(t, fixture.remote, "ls-tree", "--name-only", explicitSyncTarget)), "source-only.txt")
	}
	require.NotContains(t, readTextFile(t, fixture.githubLog), "--draft")
}

func TestSyncExplicitTargetDirtyFiles(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	dirtyCases := []struct {
		name     string
		prepare  func(*testing.T, explicitSyncFixture)
		path     string
		contents string
		absent   string
	}{
		{name: "staged", path: "README.md", contents: "pending work\n", prepare: func(t *testing.T, fixture explicitSyncFixture) {
			writeExplicitSyncFile(t, fixture, "README.md", "pending work\n")
			runGit(t, fixture.repository, "add", "README.md")
		}},
		{name: "unstaged", path: "README.md", contents: "pending work\n", prepare: func(t *testing.T, fixture explicitSyncFixture) {
			writeExplicitSyncFile(t, fixture, "README.md", "pending work\n")
		}},
		{name: "mixed_index_and_worktree", path: "README.md", contents: "complete pending work\n", prepare: func(t *testing.T, fixture explicitSyncFixture) {
			writeExplicitSyncFile(t, fixture, "README.md", "staged pending work\n")
			runGit(t, fixture.repository, "add", "README.md")
			writeExplicitSyncFile(t, fixture, "README.md", "complete pending work\n")
		}},
		{name: "untracked", path: "new.txt", contents: "untracked work\n", prepare: func(t *testing.T, fixture explicitSyncFixture) {
			writeExplicitSyncFile(t, fixture, "new.txt", "untracked work\n")
		}},
		{name: "deleted", absent: "remove.txt", prepare: func(t *testing.T, fixture explicitSyncFixture) {
			require.NoError(t, os.Remove(filepath.Join(fixture.repository, "remove.txt")))
		}},
		{name: "renamed", path: "renamed.txt", contents: "preserve renamed contents\n", absent: "rename.txt", prepare: func(t *testing.T, fixture explicitSyncFixture) {
			runGit(t, fixture.repository, "mv", "rename.txt", "renamed.txt")
		}},
		{name: "binary", path: "binary.dat", contents: "pending\x00bytes", prepare: func(t *testing.T, fixture explicitSyncFixture) {
			writeExplicitSyncFile(t, fixture, "binary.dat", "pending\x00bytes")
		}},
	}
	for _, location := range []explicitSyncTargetLocation{explicitSyncLocalRemote, explicitSyncLocalOnly, explicitSyncRemoteOnly, explicitSyncNew} {
		for _, scenario := range dirtyCases {
			t.Run(string(location)+"/"+scenario.name, func(t *testing.T) {
				t.Parallel()
				fixture := newExplicitSyncFixture(t, location)
				scenario.prepare(t, fixture)
				output, err := fixture.run(t, binary, "sync", explicitSyncTarget)
				require.NoError(t, err, output)
				require.Contains(t, output, "(b2)")
				fixture.assertPublished(t, location)
				if scenario.path != "" {
					require.Equal(t, scenario.contents, runGit(t, fixture.remote, "show", explicitSyncTarget+":"+scenario.path))
				}
				if scenario.absent != "" {
					require.NotContains(t, strings.Fields(runGit(t, fixture.remote, "ls-tree", "--name-only", explicitSyncTarget)), scenario.absent)
				}
				head := runGit(t, fixture.remote, "rev-parse", explicitSyncTarget)
				calls := fixture.llmCalls.Load()
				creates := strings.Count(readTextFile(t, fixture.githubLog), "pr create ")
				output, err = fixture.run(t, binary, "sync", explicitSyncTarget)
				require.NoError(t, err, output)
				fixture.assertPublished(t, location)
				require.Equal(t, head, runGit(t, fixture.remote, "rev-parse", explicitSyncTarget))
				require.Equal(t, calls, fixture.llmCalls.Load())
				require.Equal(t, creates, strings.Count(readTextFile(t, fixture.githubLog), "pr create "))
			})
		}
	}
}

func writeExplicitSyncFile(t *testing.T, fixture explicitSyncFixture, name, contents string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(fixture.repository, name), []byte(contents), 0o644))
}

func TestSyncExplicitTargetReviewHistory(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, scenario := range []struct {
		name          string
		marker        string
		localAdvance  bool
		remoteAdvance bool
		rejectDirty   bool
	}{
		{name: "no_pull_request"},
		{name: "open_pull_request", marker: "created-pr"},
		{name: "closed_pull_request", marker: "closed-pr"},
		{name: "local_ahead", marker: "created-pr", localAdvance: true},
		{name: "remote_ahead", marker: "created-pr", remoteAdvance: true},
		{name: "diverged", marker: "created-pr", localAdvance: true, remoteAdvance: true},
		{name: "merged_current_tip_rejected", marker: "merged-pr", rejectDirty: true},
		{name: "merged_before_new_local_commit", marker: "merged-pr", localAdvance: true},
		{name: "merged_before_new_remote_commit", marker: "merged-pr", remoteAdvance: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newExplicitSyncFixture(t, explicitSyncLocalRemote)
			if scenario.marker != "" {
				require.NoError(t, os.WriteFile(fixture.githubLog, []byte(fmt.Sprintf("%s --base qqq --head b2 --oid %s\n", scenario.marker, fixture.targetHead)), 0o600))
			}
			if scenario.marker == "merged-pr" {
				runGit(t, fixture.repository, "switch", explicitSyncDefault)
				runGit(t, fixture.repository, "merge", "--ff-only", explicitSyncTarget)
				runGit(t, fixture.repository, "push", "origin", explicitSyncDefault)
				fixture.defaultHead = fixture.targetHead
			}
			if scenario.localAdvance {
				runGit(t, fixture.repository, "switch", explicitSyncTarget)
				fixture.commitFile(t, "local-target.txt", "local target work\n")
				fixture.targetHead = strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", explicitSyncTarget))
			}
			var remoteHead string
			if scenario.remoteAdvance {
				clone := filepath.Join(fixture.workspace, "other-clone")
				runGitWithDir(t, "", "clone", "--branch", explicitSyncTarget, fixture.remote, clone)
				configureGitIdentity(t, clone)
				require.NoError(t, os.WriteFile(filepath.Join(clone, "remote-target.txt"), []byte("remote target work\n"), 0o644))
				runGit(t, clone, "add", "remote-target.txt")
				runGit(t, clone, "commit", "-m", "fix: update remote target")
				runGit(t, clone, "push", "origin", explicitSyncTarget)
				remoteHead = strings.TrimSpace(runGit(t, clone, "rev-parse", "HEAD"))
			}
			runGit(t, fixture.repository, "switch", explicitSyncSource)
			writeExplicitSyncFile(t, fixture, "README.md", "dirty source work\n")
			var before map[string]string
			if scenario.rejectDirty {
				runGit(t, fixture.repository, "add", "README.md")
				writeExplicitSyncFile(t, fixture, "README.md", "complete source work\n")
				writeExplicitSyncFile(t, fixture, "untracked.txt", "untracked source work\n")
				before = captureExplicitSyncState(t, fixture)
			}
			output, err := fixture.run(t, binary, "sync", explicitSyncTarget)
			if scenario.rejectDirty {
				require.Error(t, err, output)
				require.Contains(t, output, `cannot commit uncommitted changes on merged branch "b2"`)
				require.NotContains(t, output, "SYNCED:")
				require.Equal(t, before, captureExplicitSyncState(t, fixture))
				require.Zero(t, fixture.llmCalls.Load())
				require.NotRegexp(t, `(?m)^push `, readTextFile(t, fixture.gitLog))
				require.NotContains(t, readTextFile(t, fixture.githubLog), "pr create ")
				return
			}
			require.NoError(t, err, output)
			fixture.assertPublished(t, explicitSyncLocalRemote)
			require.Equal(t, "dirty source work\n", runGit(t, fixture.remote, "show", "b2:README.md"))
			if scenario.localAdvance {
				require.Equal(t, "local target work\n", runGit(t, fixture.remote, "show", "b2:local-target.txt"))
			}
			if scenario.remoteAdvance {
				runGit(t, fixture.repository, "merge-base", "--is-ancestor", remoteHead, explicitSyncTarget)
				require.Equal(t, "remote target work\n", runGit(t, fixture.remote, "show", "b2:remote-target.txt"))
			}
		})
	}
}

func TestSyncExplicitTargetPublicationFailures(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, scenario := range []struct {
		name               string
		gitFailure         string
		pullRequestFailure bool
		location           explicitSyncTargetLocation
	}{
		{name: "fetch", gitFailure: "fetch ", location: explicitSyncLocalRemote},
		{name: "commit", gitFailure: "commit -m", location: explicitSyncLocalRemote},
		{name: "push_existing", gitFailure: "push -u origin b2", location: explicitSyncLocalRemote},
		{name: "push_remote_only", gitFailure: "push -u origin b2", location: explicitSyncRemoteOnly},
		{name: "pull_request_existing", pullRequestFailure: true, location: explicitSyncLocalRemote},
		{name: "pull_request_new", pullRequestFailure: true, location: explicitSyncNew},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newExplicitSyncFixture(t, scenario.location)
			writeExplicitSyncFile(t, fixture, "README.md", "staged work\n")
			runGit(t, fixture.repository, "add", "README.md")
			writeExplicitSyncFile(t, fixture, "README.md", "complete work\n")
			writeExplicitSyncFile(t, fixture, "untracked.txt", "untracked work\n")
			before := captureExplicitSyncState(t, fixture)
			if scenario.pullRequestFailure {
				fixture.environment[syncMergedBranchFailPullRequestHeadVariable] = explicitSyncTarget
			} else {
				fixture.environment[syncMergedBranchFailGitMatchVariable] = scenario.gitFailure
				fixture.environment[syncMergedBranchFailGitOccurrenceVariable] = "1"
				fixture.environment[syncMergedBranchFailGitStateVariable] = filepath.Join(fixture.workspace, "failure-count")
			}
			output, err := fixture.run(t, binary, "sync", explicitSyncTarget)
			require.Error(t, err, output)
			if scenario.pullRequestFailure {
				require.Contains(t, output, "pull request")
				require.Equal(t, explicitSyncTarget, strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
				require.Equal(t, "complete work\n", runGit(t, fixture.remote, "show", "b2:README.md"))
				require.Equal(t, "untracked work\n", runGit(t, fixture.remote, "show", "b2:untracked.txt"))
			} else {
				require.Contains(t, output, "simulated git failure")
				require.Equal(t, before, captureExplicitSyncState(t, fixture))
			}
			delete(fixture.environment, syncMergedBranchFailPullRequestHeadVariable)
			delete(fixture.environment, syncMergedBranchFailGitMatchVariable)
			output, err = fixture.run(t, binary, "sync", explicitSyncTarget)
			require.NoError(t, err, output)
			fixture.assertPublished(t, scenario.location)
			require.Equal(t, "complete work\n", runGit(t, fixture.remote, "show", "b2:README.md"))
			require.Equal(t, "untracked work\n", runGit(t, fixture.remote, "show", "b2:untracked.txt"))
			base := explicitSyncDefault
			if scenario.location == explicitSyncNew {
				base = explicitSyncSource
			}
			require.Equal(t, 1, strings.Count(readTextFile(t, fixture.githubLog), "created-pr --base "+base+" --head b2 "))
		})
	}
}

func captureExplicitSyncState(t *testing.T, fixture explicitSyncFixture) map[string]string {
	t.Helper()
	return map[string]string{
		"branch":          runGit(t, fixture.repository, "branch", "--show-current"),
		"local_refs":      runGit(t, fixture.repository, "show-ref", "--heads"),
		"remote_refs":     runGit(t, fixture.remote, "show-ref", "--heads"),
		"status":          runGit(t, fixture.repository, "status", "--porcelain=v1", "-z", "--untracked-files=all"),
		"index_readme":    runGit(t, fixture.repository, "show", ":README.md"),
		"worktree_readme": readTextFile(t, filepath.Join(fixture.repository, "README.md")),
		"untracked":       readTextFile(t, filepath.Join(fixture.repository, "untracked.txt")),
		"stashes":         runGit(t, fixture.repository, "stash", "list", "--format=%H %s"),
	}
}

func TestSyncExplicitTargetConflicts(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, scenario := range []struct {
		name, base, target, pending, expected, failure string
	}{
		{name: "compatible_text_edits", base: "Run alpha beta gamma.\n", target: "Run alpha beta gamma delta.\n", pending: "Run alpha gamma.\n", expected: "Run alpha gamma delta.\n"},
		{name: "binary_conflict", base: "base\x00bytes", target: "target\x00bytes", pending: "pending\x00bytes", failure: "binary conflict"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newExplicitSyncFixture(t, explicitSyncLocalRemote)
			// Both branches share the conflict base before their independent edits.
			fixture.commitFile(t, "conflict.txt", scenario.base)
			baseCommit := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
			runGit(t, fixture.repository, "switch", explicitSyncTarget)
			runGit(t, fixture.repository, "cherry-pick", baseCommit)
			fixture.commitFile(t, "conflict.txt", scenario.target)
			runGit(t, fixture.repository, "push", "origin", explicitSyncTarget)
			fixture.targetHead = strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
			runGit(t, fixture.repository, "switch", explicitSyncSource)
			runGit(t, fixture.repository, "push", "origin", explicitSyncSource)
			fixture.sourceHead = strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
			writeExplicitSyncFile(t, fixture, "conflict.txt", scenario.pending)
			writeExplicitSyncFile(t, fixture, "README.md", "staged work\n")
			runGit(t, fixture.repository, "add", "README.md")
			writeExplicitSyncFile(t, fixture, "README.md", "complete work\n")
			writeExplicitSyncFile(t, fixture, "untracked.txt", "untracked work\n")
			before := captureExplicitSyncState(t, fixture)
			var conflictCalls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				body, err := io.ReadAll(request.Body)
				if err != nil {
					http.Error(writer, err.Error(), http.StatusBadRequest)
					return
				}
				response := "fix: preserve pending work"
				if strings.Contains(string(body), "Conflict region:") {
					conflictCalls.Add(1)
					response = "GIX_MERGE_REVIEW_APPROVED"
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(writer, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, response)
			}))
			t.Cleanup(server.Close)
			fixture.config = writeDirtySyncMergedBranchConfiguration(t, server.URL)
			output, err := fixture.run(t, binary, "sync", explicitSyncTarget)
			if scenario.failure != "" {
				require.Error(t, err, output)
				require.Contains(t, output, scenario.failure)
				require.Equal(t, before, captureExplicitSyncState(t, fixture))
				require.Equal(t, scenario.pending, readTextFile(t, filepath.Join(fixture.repository, "conflict.txt")))
				require.Zero(t, conflictCalls.Load())
				return
			}
			require.NoError(t, err, output)
			fixture.assertPublished(t, explicitSyncLocalRemote)
			require.Equal(t, scenario.expected, runGit(t, fixture.remote, "show", "b2:conflict.txt"))
			require.Positive(t, conflictCalls.Load())
		})
	}
}

func TestSyncExplicitTargetStash(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, location := range []explicitSyncTargetLocation{explicitSyncLocalRemote, explicitSyncRemoteOnly, explicitSyncNew} {
		t.Run(string(location), func(t *testing.T) {
			fixture := newExplicitSyncFixture(t, location)
			writeExplicitSyncFile(t, fixture, "README.md", "staged work\n")
			runGit(t, fixture.repository, "add", "README.md")
			writeExplicitSyncFile(t, fixture, "README.md", "complete work\n")
			writeExplicitSyncFile(t, fixture, "untracked.txt", "untracked work\n")
			before := captureExplicitSyncState(t, fixture)
			output, err := fixture.run(t, binary, "sync", explicitSyncTarget, "--stash")
			require.NoError(t, err, output)
			after := captureExplicitSyncState(t, fixture)
			require.Equal(t, "b2\n", after["branch"])
			for _, field := range []string{"status", "index_readme", "worktree_readme", "untracked", "stashes"} {
				require.Equal(t, before[field], after[field], field)
			}
			require.Equal(t, fixture.sourceHead, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", explicitSyncSource)))
			if location == explicitSyncNew {
				require.Equal(t, fixture.sourceHead, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", explicitSyncTarget)))
				require.NotContains(t, readTextFile(t, fixture.githubLog), "created-pr --base b1 --head b2 ")
			} else {
				require.Equal(t, before["remote_refs"], after["remote_refs"])
				require.Equal(t, fixture.targetHead, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", explicitSyncTarget)))
			}
		})
	}
}

func TestSyncExplicitTargetProviderBoundary(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, scenario := range []struct {
		name    string
		command []string
	}{
		{name: "provider_unavailable"},
		{name: "concurrent_checkout", command: []string{"switch", explicitSyncSource}},
		{name: "concurrent_index", command: []string{"add", "outside.txt"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newExplicitSyncFixture(t, explicitSyncLocalRemote)
			writeExplicitSyncFile(t, fixture, "README.md", "staged work\n")
			runGit(t, fixture.repository, "add", "README.md")
			writeExplicitSyncFile(t, fixture, "README.md", "complete work\n")
			writeExplicitSyncFile(t, fixture, "untracked.txt", "untracked work\n")
			before := captureExplicitSyncState(t, fixture)
			var requests atomic.Int64
			mutationErrors := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requestNumber := requests.Add(1)
				if scenario.command == nil {
					http.Error(writer, "test provider unavailable", http.StatusForbidden)
					return
				}
				if requestNumber == 1 {
					if scenario.name == "concurrent_index" {
						if err := os.WriteFile(filepath.Join(fixture.repository, "outside.txt"), []byte("outside work\n"), 0o644); err != nil {
							mutationErrors <- err
							http.Error(writer, err.Error(), http.StatusInternalServerError)
							return
						}
					}
					command := exec.Command("git", append([]string{"-C", fixture.repository}, scenario.command...)...)
					command.Env = buildGitCommandEnvironment(nil)
					output, err := command.CombinedOutput()
					if err != nil {
						mutationErrors <- fmt.Errorf("outside Git command: %w: %s", err, output)
						return
					}
					mutationErrors <- nil
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fix: preserve pending work"}}]}`))
			}))
			t.Cleanup(server.Close)
			fixture.config = writeDirtySyncMergedBranchConfiguration(t, server.URL)
			configuration := strings.ReplaceAll(readTextFile(t, fixture.config), "https://llm-proxy.example", server.URL)
			require.NoError(t, os.WriteFile(fixture.config, []byte(configuration), 0o600))
			output, err := fixture.run(t, binary, "sync", explicitSyncTarget)
			require.Error(t, err, output)
			require.Positive(t, requests.Load())
			if scenario.command == nil {
				require.Contains(t, output, "test provider unavailable")
				require.Equal(t, before, captureExplicitSyncState(t, fixture))
				return
			}
			select {
			case mutationError := <-mutationErrors:
				require.NoError(t, mutationError)
			default:
				t.Fatal("the provider did not execute the outside Git command")
			}
			require.Contains(t, output, "SYNC_SWITCH_HANDOFF")
			after := captureExplicitSyncState(t, fixture)
			for _, field := range []string{"local_refs", "remote_refs", "worktree_readme", "untracked"} {
				require.Equal(t, before[field], after[field], field)
			}
			if scenario.name == "concurrent_checkout" {
				require.Equal(t, "b1\n", after["branch"])
			} else {
				require.Equal(t, "b2\n", after["branch"])
				require.Equal(t, "outside work\n", runGit(t, fixture.repository, "show", ":outside.txt"))
				require.Equal(t, "outside work\n", readTextFile(t, filepath.Join(fixture.repository, "outside.txt")))
			}
			require.NotRegexp(t, `(?m)^push `, readTextFile(t, fixture.gitLog))
		})
	}
}
