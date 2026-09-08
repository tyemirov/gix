package tests

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncExplicitBranchNamesHaveEqualBehavior(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, branch := range []string{"main", "master", "qqq", "wwww", "default", "release/trunk"} {
		for _, role := range []string{"default", "new"} {
			t.Run(branch+"/"+role, func(t *testing.T) {
				t.Parallel()
				defaultBranch := "base"
				if role == "default" {
					defaultBranch = branch
				}
				fixture := newSyncFixture(t, defaultBranch)
				base := strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", defaultBranch))
				writeFile(t, filepath.Join(fixture.repository, "work.txt"), "file work\n")
				output, err := fixture.run(t, binary, "sync", branch)
				require.NoError(t, err, output)
				require.Equal(t, branch, strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
				require.Equal(t, "file work\n", runGit(t, fixture.remote, "show", branch+":work.txt"))
				require.Equal(t, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD")), strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", branch)))
				require.Empty(t, runGit(t, fixture.repository, "status", "--porcelain"))
				log := readTextFile(t, fixture.githubLog)
				require.NotContains(t, log, "/branches/", "sync must not query branch protection")
				if role == "default" {
					require.NotContains(t, log, "pr create ")
				} else {
					require.Contains(t, log, "created-pr --base base --head "+branch+" ")
					require.Equal(t, base, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", defaultBranch)))
					require.Equal(t, "origin/"+branch, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "--abbrev-ref", "@{upstream}")))
				}
			})
		}
	}
}

func TestSyncDefaultSelectionAndWorkStates(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	for _, scenario := range []struct {
		name     string
		branch   string
		dirty    bool
		ahead    bool
		behind   bool
		stash    bool
		implicit bool
	}{
		{name: "implicit_dirty", branch: "qqq", dirty: true, implicit: true},
		{name: "implicit_ahead", branch: "qqq", ahead: true, implicit: true},
		{name: "explicit_dirty", branch: "qqq", dirty: true},
		{name: "explicit_ahead", branch: "qqq", ahead: true},
		{name: "explicit_diverged", branch: "qqq", ahead: true, behind: true},
		{name: "explicit_dirty_diverged", branch: "qqq", dirty: true, ahead: true, behind: true},
		{name: "explicit_behind", branch: "qqq", behind: true},
		{name: "explicit_current", branch: "qqq"},
		{name: "explicit_stash", branch: "qqq", dirty: true, behind: true, stash: true},
	} {
		testInstance.Run(scenario.name, func(testInstance *testing.T) {
			fixture := newSyncFixture(testInstance, scenario.branch)
			if scenario.ahead {
				fixture.commitFile(testInstance, "local.txt", "local work\n")
			}
			localBefore := strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD"))
			if scenario.behind {
				upstream := filepath.Join(fixture.workspace, "upstream")
				runGitWithDir(testInstance, "", "clone", fixture.remote, upstream)
				configureGitIdentity(testInstance, upstream)
				require.NoError(testInstance, os.WriteFile(filepath.Join(upstream, "remote.txt"), []byte("remote work\n"), 0o644))
				runGit(testInstance, upstream, "add", "remote.txt")
				runGit(testInstance, upstream, "commit", "-m", "remote work")
				runGit(testInstance, upstream, "push", "origin", scenario.branch)
			}
			remoteBefore := strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", "HEAD"))
			if scenario.dirty {
				require.NoError(testInstance, os.WriteFile(filepath.Join(fixture.repository, "README.md"), []byte("pending work\n"), 0o644))
				runGit(testInstance, fixture.repository, "add", "README.md")
			}

			arguments := []string{"sync"}
			if !scenario.implicit {
				arguments = append(arguments, scenario.branch)
			}
			if scenario.stash {
				arguments = append(arguments, "--stash")
			}
			output, runError := fixture.run(testInstance, binaryPath, arguments...)
			require.NoError(testInstance, runError, output)
			require.Contains(testInstance, output, "SYNCED:")
			require.NotContains(testInstance, output, "SYNC_SWITCH_ROLLBACK")
			currentBranch := strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current"))
			githubLog := readTextFile(testInstance, fixture.githubLog)
			gitLog := readTextFile(testInstance, fixture.gitLog)
			if scenario.implicit {
				require.NotEqual(testInstance, scenario.branch, currentBranch)
				require.Contains(testInstance, githubLog, "created-pr --base "+scenario.branch+" --head "+currentBranch)
				require.NotContains(testInstance, githubLog, "--draft")
				require.Equal(testInstance, localBefore, strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", scenario.branch)))
				require.Equal(testInstance, remoteBefore, strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", scenario.branch)))
				require.NotContains(testInstance, gitLog, "push origin "+scenario.branch+" ")
			} else {
				require.Equal(testInstance, scenario.branch, currentBranch)
				require.NotContains(testInstance, githubLog, "pr create ")
				require.NotContains(testInstance, githubLog, "api repos/owner/project/branches/"+url.PathEscape(scenario.branch)+" ")
			}
			require.Equal(testInstance, strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", currentBranch)), strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD")))
			runGit(testInstance, fixture.repository, "merge-base", "--is-ancestor", localBefore, currentBranch)
			runGit(testInstance, fixture.repository, "merge-base", "--is-ancestor", remoteBefore, currentBranch)
			if !scenario.implicit && (scenario.dirty && !scenario.stash || scenario.ahead) {
				require.Contains(testInstance, gitLog, "push origin "+scenario.branch+" ")
			}
			if scenario.dirty {
				require.Equal(testInstance, "pending work\n", readTextFile(testInstance, filepath.Join(fixture.repository, "README.md")))
			}
			status := strings.TrimSpace(runGit(testInstance, fixture.repository, "status", "--porcelain"))
			if scenario.stash {
				require.Equal(testInstance, "M  README.md", status)
			} else {
				require.Empty(testInstance, status)
			}
		})
	}
}

func TestSyncExplicitBranchWithoutReviewDelta(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, state := range []string{"new", "local", "remote", "local_remote", "new_stashed"} {
		t.Run(state, func(t *testing.T) {
			fixture := newSyncFixture(t, "qqq")
			initial := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
			if state == "local" || state == "local_remote" {
				runGit(t, fixture.repository, "branch", "wwww")
			}
			if state == "remote" || state == "local_remote" {
				runGit(t, fixture.repository, "push", "origin", "qqq:refs/heads/wwww")
			}
			args := []string{"sync", "wwww"}
			if state == "new_stashed" {
				writeFile(t, filepath.Join(fixture.repository, "README.md"), "stashed work\n")
				args = append(args, "--stash")
			}
			output, err := fixture.run(t, binary, args...)
			require.NoError(t, err, output)
			require.Contains(t, output, "SYNCED:")
			require.Equal(t, "wwww", strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
			require.Equal(t, initial, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "wwww")))
			require.Equal(t, initial, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "qqq")))
			require.NotContains(t, readTextFile(t, fixture.githubLog), "pr create ")
			if state == "new_stashed" {
				require.Equal(t, "stashed work\n", readTextFile(t, filepath.Join(fixture.repository, "README.md")))
			} else {
				require.Empty(t, runGit(t, fixture.repository, "status", "--porcelain"))
			}
			require.Empty(t, runGit(t, fixture.repository, "stash", "list"))
		})
	}
}

func TestSyncWithoutDestinationCreatesBranchOnlyForWork(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	for _, dirty := range []bool{false, true} {
		testInstance.Run(fmt.Sprintf("dirty_%t", dirty), func(testInstance *testing.T) {
			fixture := newSyncFixture(testInstance, "qqq")
			originalHead := strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD"))

			if dirty {
				writeFile(testInstance, filepath.Join(fixture.repository, "README.md"), "pending work\n")
			}
			output, runError := fixture.run(testInstance, binaryPath, "sync")
			require.NoError(testInstance, runError, output)
			current := strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current"))
			if dirty {
				require.NotEqual(testInstance, "qqq", current)
			} else {
				require.Equal(testInstance, "qqq", current)
				require.Equal(testInstance, "qqq", strings.TrimSpace(runGit(testInstance, fixture.remote, "for-each-ref", "--format=%(refname:short)", "refs/heads/")))
			}
			require.NotEmpty(testInstance, current)
			require.Equal(testInstance, originalHead, strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "qqq")))
			require.Equal(testInstance, originalHead, strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", "qqq")))
			require.Equal(testInstance, strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD")), strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", current)))
			require.NotContains(testInstance, readTextFile(testInstance, fixture.githubLog), "api repos/owner/project/branches/")
			if dirty {
				require.Equal(testInstance, "pending work\n", runGit(testInstance, fixture.remote, "show", current+":README.md"))
			} else {
				require.Equal(testInstance, originalHead, strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD")))
				require.NotContains(testInstance, readTextFile(testInstance, fixture.githubLog), "pr create ")
			}
			require.Empty(testInstance, runGit(testInstance, fixture.repository, "status", "--porcelain"))
		})
	}
}

func TestSyncDefaultCreatesFreshBranchWithNarrowFetch(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	fixture := newSyncFixture(t, "qqq")
	fixture.commitFile(t, "local.txt", "unpublished work\n")
	output, err := fixture.run(t, binary, "sync")
	require.NoError(t, err, output)
	first := strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current"))
	head := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
	runGit(t, fixture.repository, "switch", "qqq")
	runGit(t, fixture.repository, "branch", "-D", first)
	runGit(t, fixture.repository, "update-ref", "-d", "refs/remotes/origin/"+first)
	runGit(t, fixture.repository, "config", "remote.origin.fetch", "+refs/heads/qqq:refs/remotes/origin/qqq")
	output, err = fixture.run(t, binary, "sync")
	require.NoError(t, err, output)
	second := strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current"))
	require.NotEqual(t, first, second)
	require.NotEqual(t, "qqq", second)
	require.Equal(t, head, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", first)))
	require.Equal(t, head, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", second)))
	require.Equal(t, 2, strings.Count(readTextFile(t, fixture.githubLog), "created-pr --base qqq --head "))
}

func TestSyncDefaultCreatesNewBranchOnEachInvocation(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	fixture := newSyncFixture(testInstance, "qqq")
	fixture.commitFile(testInstance, "local.txt", "local work\n")

	output, runError := fixture.run(testInstance, binaryPath, "sync")
	require.NoError(testInstance, runError, output)
	reviewBranch := strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current"))
	runGit(testInstance, fixture.repository, "switch", "qqq")
	output, runError = fixture.run(testInstance, binaryPath, "sync")
	require.NoError(testInstance, runError, output)
	require.NotEqual(testInstance, reviewBranch, strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current")))
	require.Equal(testInstance, 2, strings.Count(readTextFile(testInstance, fixture.githubLog), "created-pr --base qqq --head "))
}

func TestSyncGeneratedBranchUsesConfiguredReviewMetadata(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	fixture := newSyncFixture(testInstance, "qqq")
	fixture.commitFile(testInstance, "local.txt", "local work\n")

	configuration := strings.Replace(readTextFile(testInstance, fixture.config), "remote: origin", "remote: origin\n      pull_request:\n        title: Explicit review title\n        body: Explicit review body", 1)
	require.NoError(testInstance, os.WriteFile(fixture.config, []byte(configuration), 0o600))
	output, runError := fixture.run(testInstance, binaryPath, "sync")
	require.NoError(testInstance, runError, output)
	require.Contains(testInstance, readTextFile(testInstance, fixture.githubLog), "--title Explicit review title --body Explicit review body")
	require.Zero(testInstance, fixture.llmCalls.Load())
}

func TestSyncGeneratedBranchKeepsExistingReviewBase(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	fixture := newSyncFixture(testInstance, "qqq")
	fixture.commitFile(testInstance, "local.txt", "local work\n")

	output, runError := fixture.run(testInstance, binaryPath, "sync")
	require.NoError(testInstance, runError, output)
	reviewBranch := strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current"))
	runGit(testInstance, fixture.repository, "push", "origin", "qqq:refs/heads/other-base")
	log := strings.Replace(readTextFile(testInstance, fixture.githubLog), "created-pr --base qqq --head "+reviewBranch, "created-pr --base other-base --head "+reviewBranch, 1)
	require.NoError(testInstance, os.WriteFile(fixture.githubLog, []byte(log), 0o600))
	runGit(testInstance, fixture.repository, "switch", "qqq")
	output, runError = fixture.run(testInstance, binaryPath, "sync")
	require.NoError(testInstance, runError, output)
	currentBranch := strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current"))
	require.NotEqual(testInstance, reviewBranch, currentBranch)
	require.Contains(testInstance, readTextFile(testInstance, fixture.githubLog), "created-pr --base qqq --head "+currentBranch)
}

func TestSyncExplicitDefaultFromFeaturePreservesTargetScope(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	fixture := newSyncFixture(testInstance, "qqq")
	runGit(testInstance, fixture.repository, "switch", "-c", "feature/source")
	fixture.commitFile(testInstance, "feature.txt", "unrelated feature\n")
	sourceHead := strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD"))
	require.NoError(testInstance, os.WriteFile(filepath.Join(fixture.repository, "README.md"), []byte("pending work\n"), 0o644))

	output, runError := fixture.run(testInstance, binaryPath, "sync", "qqq")
	require.NoError(testInstance, runError, output)
	require.Equal(testInstance, sourceHead, strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "feature/source")))
	require.NoFileExists(testInstance, filepath.Join(fixture.repository, "feature.txt"))
	require.Equal(testInstance, "pending work\n", readTextFile(testInstance, filepath.Join(fixture.repository, "README.md")))
	require.Equal(testInstance, "qqq", strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current")))
	require.NotContains(testInstance, readTextFile(testInstance, fixture.githubLog), "pr create ")
}
