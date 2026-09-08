package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncPublishedWorkSurvivesFreshCheckout(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, branch := range []string{"qqq", "feature/work"} {
		t.Run(branch, func(t *testing.T) {
			t.Parallel()
			fixture := newSyncFixture(t, "qqq")
			files := map[string]string{"README.md": "published text\n", "new.txt": "published addition\n", "binary.dat": "published\x00bytes"}
			for name, contents := range files {
				writeFile(t, filepath.Join(fixture.repository, name), contents)
			}
			output, err := fixture.run(t, binary, "sync", branch)
			require.NoError(t, err, output)
			published := strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", branch))
			fresh := filepath.Join(fixture.workspace, "fresh")
			runGitWithDir(t, "", "clone", "--branch", branch, fixture.remote, fresh)
			configureGitIdentity(t, fresh)
			// The second checkout has no local review metadata from the first.
			config := runGit(t, fresh, "config", "--local", "--list")
			require.NotContains(t, config, "gix-review-base")
			require.NoError(t, os.RemoveAll(fixture.repository))
			fixture.repository = fresh
			for name, contents := range files {
				require.Equal(t, contents, readTextFile(t, filepath.Join(fresh, name)))
			}
			output, err = fixture.run(t, binary, "sync", branch)
			require.NoError(t, err, output)
			require.Equal(t, published, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", branch)))
			writeFile(t, filepath.Join(fresh, "new.txt"), "work from replacement checkout\n")
			output, err = fixture.run(t, binary, "sync", branch)
			require.NoError(t, err, output)
			require.Equal(t, branch, strings.TrimSpace(runGit(t, fresh, "branch", "--show-current")))
			require.Equal(t, "work from replacement checkout\n", runGit(t, fixture.remote, "show", branch+":new.txt"))
			require.Equal(t, files["binary.dat"], runGit(t, fixture.remote, "show", branch+":binary.dat"))
			require.Empty(t, runGit(t, fresh, "status", "--porcelain"))
		})
	}
}

func TestSyncFileWorkNoLocalWorkPullsRemote(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	fixture := newSyncFixture(t, "qqq")
	upstream := filepath.Join(fixture.workspace, "upstream")
	runGitWithDir(t, "", "clone", fixture.remote, upstream)
	configureGitIdentity(t, upstream)
	writeFile(t, filepath.Join(upstream, "incoming.txt"), "remote file work\n")
	runGit(t, upstream, "add", "incoming.txt")
	runGit(t, upstream, "commit", "-m", "publish file work")
	runGit(t, upstream, "push", "origin", "qqq")
	output, err := fixture.run(t, binary, "sync")
	require.NoError(t, err, output)
	require.Equal(t, "qqq", strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
	require.Equal(t, "qqq", strings.TrimSpace(runGit(t, fixture.remote, "for-each-ref", "--format=%(refname:short)", "refs/heads/")))
	require.Equal(t, "remote file work\n", readTextFile(t, filepath.Join(fixture.repository, "incoming.txt")))
	require.Equal(t, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "qqq")), strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD")))
	require.NotContains(t, readTextFile(t, fixture.githubLog), "pr create ")
	require.Zero(t, fixture.llmCalls.Load())
}

func TestSyncFileWorkFromEmptyParent(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	fixture := newSyncFixture(t, "qqq")
	output, err := fixture.run(t, binary, "sync", "parent")
	require.NoError(t, err, output)
	writeFile(t, filepath.Join(fixture.repository, "child.txt"), "child file work\n")
	output, err = fixture.run(t, binary, "sync", "child")
	require.NoError(t, err, output)
	require.Equal(t, "child", strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
	require.Equal(t, "child file work\n", runGit(t, fixture.remote, "show", "child:child.txt"))
	require.Contains(t, readTextFile(t, fixture.githubLog), "created-pr --base parent --head child ")
	require.NotContains(t, readTextFile(t, fixture.githubLog), "created-pr --base qqq --head parent ")
}

func TestSyncFileWorkAfterGrandparentMerges(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	fixture := newSyncFixture(t, "qqq")
	writeFile(t, filepath.Join(fixture.repository, "grandparent.txt"), "grandparent work\n")
	output, err := fixture.run(t, binary, "sync", "grandparent")
	require.NoError(t, err, output)
	grandparentHead := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
	output, err = fixture.run(t, binary, "sync", "parent")
	require.NoError(t, err, output)
	fixture.commitFile(t, "parent.txt", "unpublished parent work\n")
	upstream := filepath.Join(fixture.workspace, "merger")
	runGitWithDir(t, "", "clone", fixture.remote, upstream)
	configureGitIdentity(t, upstream)
	runGit(t, upstream, "merge", "--squash", "origin/grandparent")
	runGit(t, upstream, "commit", "-m", "merge grandparent work")
	runGit(t, upstream, "push", "origin", "qqq")
	runGit(t, fixture.remote, "update-ref", "refs/pull/9/head", grandparentHead)
	runGit(t, upstream, "push", "origin", "--delete", "grandparent")
	writeFile(t, fixture.githubLog, readTextFile(t, fixture.githubLog)+fmt.Sprintf("merged-pr --base qqq --head grandparent --oid %s\n", grandparentHead))
	writeFile(t, filepath.Join(fixture.repository, "child.txt"), "child work\n")
	output, err = fixture.run(t, binary, "sync", "child")
	require.NoError(t, err, output)
	require.Equal(t, "child", strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
	require.Equal(t, "qqq", strings.TrimSpace(runGit(t, fixture.repository, "config", "--get", "branch.parent.gix-review-base")))
	require.Equal(t, "unpublished parent work\n", runGit(t, fixture.remote, "show", "parent:parent.txt"))
	require.Equal(t, "child work\n", runGit(t, fixture.remote, "show", "child:child.txt"))
	require.Contains(t, readTextFile(t, fixture.githubLog), "created-pr --base qqq --head parent ")
	require.Contains(t, readTextFile(t, fixture.githubLog), "created-pr --base parent --head child ")
}

func TestSyncFileWorkAfterEmptyChildParentMerges(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, existingChild := range []bool{false, true} {
		for _, dirty := range []bool{false, true} {
			for _, deleted := range []bool{false, true} {
				t.Run(fmt.Sprintf("existing_%t_dirty_%t_deleted_%t", existingChild, dirty, deleted), func(t *testing.T) {
					t.Parallel()
					fixture := newSyncFixture(t, "qqq")
					writeFile(t, filepath.Join(fixture.repository, "parent.txt"), "parent file work\n")
					output, err := fixture.run(t, binary, "sync", "parent")
					require.NoError(t, err, output)
					parentHead := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
					if existingChild {
						output, err = fixture.run(t, binary, "sync", "child")
						require.NoError(t, err, output)
					}
					require.NotContains(t, readTextFile(t, fixture.githubLog), "created-pr --base parent --head child ")
					upstream := filepath.Join(fixture.workspace, "merger")
					runGitWithDir(t, "", "clone", fixture.remote, upstream)
					configureGitIdentity(t, upstream)
					runGit(t, upstream, "merge", "--squash", "origin/parent")
					runGit(t, upstream, "commit", "-m", "merge parent work")
					runGit(t, upstream, "push", "origin", "qqq")
					runGit(t, fixture.remote, "update-ref", "refs/pull/9/head", parentHead)
					if deleted {
						runGit(t, upstream, "push", "origin", "--delete", "parent")
					}
					writeFile(t, fixture.githubLog, readTextFile(t, fixture.githubLog)+fmt.Sprintf("merged-pr --base qqq --head parent --oid %s\n", parentHead))
					if dirty {
						writeFile(t, filepath.Join(fixture.repository, "child.txt"), "new child work\n")
					}
					arguments := []string{"sync"}
					if !existingChild {
						arguments = append(arguments, "child")
					}
					output, err = fixture.run(t, binary, arguments...)
					require.NoError(t, err, output)
					require.Equal(t, "child", strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
					require.Equal(t, "qqq", strings.TrimSpace(runGit(t, fixture.repository, "config", "--get", "branch.child.gix-review-base")))
					require.Equal(t, "parent file work\n", runGit(t, fixture.remote, "show", "child:parent.txt"))
					if dirty {
						require.Equal(t, "new child work\n", runGit(t, fixture.remote, "show", "child:child.txt"))
						require.Contains(t, readTextFile(t, fixture.githubLog), "created-pr --base qqq --head child ")
					} else {
						require.NotContains(t, readTextFile(t, fixture.githubLog), "created-pr --base qqq --head child ")
					}
					require.Empty(t, runGit(t, fixture.repository, "status", "--porcelain"))
				})
			}
		}
	}
}

func TestSyncFileWorkPreservedAfterParentRejection(t *testing.T) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, dirty := range []bool{false, true} {
		for _, remoteParent := range []bool{false, true} {
			t.Run(fmt.Sprintf("dirty_%t_remote_parent_%t", dirty, remoteParent), func(t *testing.T) {
				t.Parallel()
				fixture := newSyncFixture(t, "qqq")
				initial := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
				runGit(t, fixture.repository, "switch", "-c", "parent")
				if remoteParent {
					runGit(t, fixture.repository, "push", "-u", "origin", "parent")
				}
				fixture.commitFile(t, "parent.txt", "unpublished parent work\n")
				parentHead := strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
				runGit(t, fixture.remote, "config", "receive.hideRefs", "refs/heads/parent")
				if dirty {
					writeFile(t, filepath.Join(fixture.repository, "child.txt"), "pending child work\n")
				}
				output, err := fixture.run(t, binary, "sync", "child")
				require.NoError(t, err, output)
				require.Contains(t, output, "SYNC_PUSH_REJECTED")
				require.Contains(t, output, "SYNC_PUBLICATION_DEFERRED")
				require.NotContains(t, output, "SYNC_SWITCH_ROLLBACK")
				require.Equal(t, "child", strings.TrimSpace(runGit(t, fixture.repository, "branch", "--show-current")))
				if dirty {
					require.Equal(t, "pending child work\n", runGit(t, fixture.repository, "show", "HEAD:child.txt"))
				}
				require.Equal(t, "unpublished parent work\n", runGit(t, fixture.repository, "show", "HEAD:parent.txt"))
				require.Equal(t, parentHead, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "parent")))
				require.NotContains(t, readTextFile(t, fixture.gitLog), "push -u origin child")
				require.NotContains(t, readTextFile(t, fixture.githubLog), "pr create ")
				require.Empty(t, runGit(t, fixture.remote, "for-each-ref", "--format=%(refname)", "refs/heads/child"))
				if remoteParent {
					require.Equal(t, initial, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "parent")))
				}
				require.Empty(t, runGit(t, fixture.repository, "status", "--porcelain"))
				require.Empty(t, runGit(t, fixture.repository, "stash", "list"))
				runGit(t, fixture.remote, "config", "--unset", "receive.hideRefs")
				output, err = fixture.run(t, binary, "sync")
				require.NoError(t, err, output)
				if dirty {
					require.Equal(t, "pending child work\n", runGit(t, fixture.remote, "show", "child:child.txt"))
				}
				require.Equal(t, "unpublished parent work\n", runGit(t, fixture.remote, "show", "parent:parent.txt"))
				if dirty {
					require.Equal(t, "child.txt", strings.TrimSpace(runGit(t, fixture.repository, "diff", "--name-only", "origin/parent...origin/child")))
					require.Contains(t, readTextFile(t, fixture.githubLog), "created-pr --base parent --head child ")
				} else {
					require.Empty(t, runGit(t, fixture.repository, "diff", "--name-only", "origin/parent...origin/child"))
					require.NotContains(t, readTextFile(t, fixture.githubLog), "created-pr --base parent --head child ")
				}
			})
		}
	}
}
