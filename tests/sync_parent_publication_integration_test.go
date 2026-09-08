package tests

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncParentRemoteStatePreservesChildWork(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, state := range []string{"remote_only", "behind", "diverged"} {
		for _, openReview := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s_open_review_%t", state, openReview), func(t *testing.T) {
				t.Parallel()
				fixture := newSyncFixture(t, "qqq")
				if openReview {
					writeFile(t, filepath.Join(fixture.repository, "parent.txt"), "reviewed parent work\n")
				}
				output, err := fixture.run(t, binary, "sync", "parent")
				require.NoError(t, err, output)
				output, err = fixture.run(t, binary, "sync", "child")
				require.NoError(t, err, output)
				if state == "remote_only" {
					runGit(t, fixture.repository, "branch", "-D", "parent")
				} else {
					upstream := filepath.Join(fixture.workspace, "upstream")
					runGitWithDir(t, "", "clone", "--branch", "parent", fixture.remote, upstream)
					configureGitIdentity(t, upstream)
					writeFile(t, filepath.Join(upstream, "incoming.txt"), "incoming parent work\n")
					runGit(t, upstream, "add", "incoming.txt")
					runGit(t, upstream, "commit", "-m", "advance parent")
					runGit(t, upstream, "push", "origin", "parent")
					if state == "diverged" {
						runGit(t, fixture.repository, "switch", "parent")
						fixture.commitFile(t, "local-parent.txt", "unpublished parent work\n")
						runGit(t, fixture.repository, "switch", "child")
					}
				}
				writeFile(t, filepath.Join(fixture.repository, "README.md"), "staged child work\n")
				runGit(t, fixture.repository, "add", "README.md")
				writeFile(t, filepath.Join(fixture.repository, "README.md"), "complete child work\n")
				writeFile(t, filepath.Join(fixture.repository, "child.txt"), "untracked child work\n")
				output, err = fixture.run(t, binary, "sync")
				require.NoError(t, err, output)
				require.Equal(t, "child", strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
				require.Equal(t, "initial\n", runGit(t, fixture.remote, "show", "parent:README.md"))
				require.Equal(t, "complete child work\n", runGit(t, fixture.remote, "show", "child:README.md"))
				require.Equal(t, "untracked child work\n", runGit(t, fixture.remote, "show", "child:child.txt"))
				require.Equal(t, "README.md\nchild.txt\n", runGit(t, fixture.remote, "diff", "--name-only", "parent...child"))
				if state != "remote_only" {
					require.Equal(t, "incoming parent work\n", runGit(t, fixture.remote, "show", "child:incoming.txt"))
				}
				if state == "diverged" {
					require.Equal(t, "unpublished parent work\n", runGit(t, fixture.remote, "show", "parent:local-parent.txt"))
					require.Equal(t, "unpublished parent work\n", runGit(t, fixture.remote, "show", "child:local-parent.txt"))
				}
				require.Contains(t, readTextFile(t, fixture.githubLog), "created-pr --base parent --head child ")
				require.Empty(t, runGit(t, fixture.repository, "status", "--porcelain"))
				require.Empty(t, runGit(t, fixture.repository, "stash", "list"))
			})
		}
	}
}

func TestSyncBranchPublicationWithRejectedTag(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, scenario := range []struct {
		name, branch         string
		upToDate, failReview bool
	}{
		{name: "default_updated", branch: "qqq"},
		{name: "feature_up_to_date", branch: "feature", upToDate: true},
		{name: "feature_updated", branch: "feature"},
		{name: "feature_review_failure", branch: "feature", failReview: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			fixture := newSyncFixture(t, "qqq")
			if scenario.upToDate {
				runGit(t, fixture.repository, "switch", "-c", scenario.branch)
				fixture.commitFile(t, "work.txt", "published branch data\n")
				runGit(t, fixture.repository, "push", "-u", "origin", scenario.branch)
			}
			runGit(t, fixture.repository, "config", "push.followTags", "true")
			runGit(t, fixture.repository, "tag", "-a", "rejected-tag", "-m", "extra ref")
			runGit(t, fixture.remote, "config", "receive.hideRefs", "refs/tags/rejected-tag")
			if !scenario.upToDate {
				writeFile(t, filepath.Join(fixture.repository, "work.txt"), "published branch data\n")
			}
			if scenario.failReview {
				fixture.environment[syncMergedBranchFailPullRequestHeadVariable] = scenario.branch
			}
			output, err := fixture.run(t, binary, "sync", scenario.branch)
			if scenario.failReview {
				require.Error(t, err, output)
				require.Contains(t, output, "strict sync published remote changes but finalization failed")
			} else {
				require.NoError(t, err, output)
			}
			require.Contains(t, output, "SYNC_PUSH_REJECTED")
			require.Contains(t, output, "rejected-tag")
			require.NotContains(t, output, "did not receive the changes")
			require.NotContains(t, output, "SYNC_SWITCH_ROLLBACK")
			require.Equal(t, scenario.branch, strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
			require.Equal(t, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD")), strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", scenario.branch)))
			if !scenario.upToDate {
				require.Equal(t, "published branch data\n", runGit(t, fixture.remote, "show", scenario.branch+":work.txt"))
			}
			require.Empty(t, runGit(t, fixture.remote, "for-each-ref", "--format=%(refname)", "refs/tags/"))
			if scenario.branch != "qqq" && !scenario.failReview {
				require.Contains(t, readTextFile(t, fixture.githubLog), "created-pr --base qqq --head feature ")
			}
		})
	}
}

func TestSyncStackParentPublicationWithRejectedTag(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	fixture := newSyncFixture(t, "qqq")
	runGit(t, fixture.repository, "switch", "-c", "parent")
	fixture.commitFile(t, "parent.txt", "unpublished parent work\n")
	runGit(t, fixture.repository, "tag", "-a", "rejected-tag", "-m", "extra ref")
	runGit(t, fixture.repository, "config", "push.followTags", "true")
	runGit(t, fixture.remote, "config", "receive.hideRefs", "refs/tags/rejected-tag")
	writeFile(t, filepath.Join(fixture.repository, "child.txt"), "pending child work\n")
	output, err := fixture.run(t, binary, "sync", "child")
	require.NoError(t, err, output)
	require.Contains(t, output, "SYNC_PUSH_REJECTED")
	require.NotContains(t, output, "SYNC_PUBLICATION_DEFERRED")
	require.NotContains(t, output, "did not receive the changes")
	require.Equal(t, "child", strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
	require.Equal(t, "unpublished parent work\n", runGit(t, fixture.remote, "show", "parent:parent.txt"))
	require.Equal(t, "pending child work\n", runGit(t, fixture.remote, "show", "child:child.txt"))
	require.Equal(t, "child.txt\n", runGit(t, fixture.remote, "diff", "--name-only", "parent...child"))
	require.Contains(t, readTextFile(t, fixture.githubLog), "created-pr --base qqq --head parent ")
	require.Contains(t, readTextFile(t, fixture.githubLog), "created-pr --base parent --head child ")
	require.Empty(t, runGit(t, fixture.repository, "status", "--porcelain"))
	require.Empty(t, runGit(t, fixture.repository, "stash", "list"))
}
