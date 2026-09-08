package tests

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncExplicitQQQDefaultFromCheckout(testInstance *testing.T) {
	const defaultBranch = "qqq"
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	for _, startingBranch := range []string{defaultBranch, "main", "master"} {
		testInstance.Run("from_"+startingBranch, func(testInstance *testing.T) {
			fixture := newSyncFixture(testInstance, defaultBranch)
			configuration := strings.Replace(readTextFile(testInstance, fixture.config), "remote: origin", "remote: origin\n      roots: [.]", 1)
			require.NoError(testInstance, os.WriteFile(fixture.config, []byte(configuration), 0o600))
			require.Equal(testInstance, "refs/heads/qqq", strings.TrimSpace(runGit(testInstance, fixture.remote, "symbolic-ref", "HEAD")))
			require.Contains(testInstance, runGit(testInstance, fixture.repository, "ls-remote", "--symref", "origin", "HEAD"), "ref: refs/heads/qqq\tHEAD")
			fixture.commitFile(testInstance, "local-default.txt", "local default work\n")
			localDefaultHead := strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", defaultBranch))
			expectedBranches := []string{defaultBranch}
			var sourceHead string
			if startingBranch != defaultBranch {
				runGit(testInstance, fixture.repository, "switch", "-c", startingBranch, "origin/qqq")
				fixture.commitFile(testInstance, "source-only.txt", "unrelated source work\n")
				runGit(testInstance, fixture.repository, "push", "origin", startingBranch)
				sourceHead = strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", startingBranch))
				expectedBranches = append(expectedBranches, startingBranch)
			}
			sort.Strings(expectedBranches)

			upstream := filepath.Join(fixture.workspace, "upstream")
			runGitWithDir(testInstance, "", "clone", fixture.remote, upstream)
			configureGitIdentity(testInstance, upstream)
			require.NoError(testInstance, os.WriteFile(filepath.Join(upstream, "incoming.txt"), []byte("remote default work\n"), 0o644))
			runGit(testInstance, upstream, "add", "incoming.txt")
			runGit(testInstance, upstream, "commit", "-m", "fix: update qqq remotely")
			runGit(testInstance, upstream, "push", "origin", defaultBranch)
			remoteDefaultHead := strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", defaultBranch))

			require.NoError(testInstance, os.WriteFile(filepath.Join(fixture.repository, "README.md"), []byte("staged pending work\n"), 0o644))
			runGit(testInstance, fixture.repository, "add", "README.md")
			require.NoError(testInstance, os.WriteFile(filepath.Join(fixture.repository, "README.md"), []byte("complete pending work\n"), 0o644))

			fixture.environment[pathEnvironmentVariableNameConstant] = fixture.executablePath
			fixture.environment["GIX_SYNC_TEST_REPOSITORY"] = fixture.repository
			arguments := []string{"--config", fixture.config, "sync", defaultBranch}
			output, runError := runBinaryIntegrationCommandWithInput(testInstance, binaryPath, fixture.repository, fixture.environment, syncMergedBranchIntegrationTimeout, "", arguments)
			require.NoError(testInstance, runError, output)
			testInstance.Logf("gix sync qqq from %s:\n%s", startingBranch, output)
			require.Contains(testInstance, output, "SYNCED:")
			require.Contains(testInstance, output, "(qqq)")
			require.Equal(testInstance, defaultBranch, strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current")))
			publishedHead := strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD"))
			require.Equal(testInstance, publishedHead, strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", defaultBranch)))
			runGit(testInstance, fixture.repository, "merge-base", "--is-ancestor", localDefaultHead, publishedHead)
			runGit(testInstance, fixture.repository, "merge-base", "--is-ancestor", remoteDefaultHead, publishedHead)
			require.Equal(testInstance, "complete pending work\n", runGit(testInstance, fixture.remote, "show", "qqq:README.md"))
			require.Equal(testInstance, "local default work\n", runGit(testInstance, fixture.remote, "show", "qqq:local-default.txt"))
			require.Equal(testInstance, "remote default work\n", runGit(testInstance, fixture.remote, "show", "qqq:incoming.txt"))
			require.NoFileExists(testInstance, filepath.Join(fixture.repository, "source-only.txt"))
			require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, fixture.repository, "status", "--porcelain")))
			for _, repository := range []string{fixture.repository, fixture.remote} {
				require.Equal(testInstance, expectedBranches, strings.Fields(runGit(testInstance, repository, "for-each-ref", "--format=%(refname:short)", "refs/heads/")))
				if sourceHead != "" {
					require.Equal(testInstance, sourceHead, strings.TrimSpace(runGit(testInstance, repository, "rev-parse", startingBranch)))
				}
			}
			require.Equal(testInstance, "refs/heads/qqq", strings.TrimSpace(runGit(testInstance, fixture.remote, "symbolic-ref", "HEAD")))
			require.NotContains(testInstance, readTextFile(testInstance, fixture.githubLog), "pr create ")
			require.Equal(testInstance, 1, strings.Count(readTextFile(testInstance, fixture.gitLog), "push origin qqq "))
			llmCalls := fixture.llmCalls.Load()
			require.Positive(testInstance, llmCalls)

			output, runError = runBinaryIntegrationCommandWithInput(testInstance, binaryPath, fixture.repository, fixture.environment, syncMergedBranchIntegrationTimeout, "", arguments)
			require.NoError(testInstance, runError, output)
			require.Contains(testInstance, output, "(qqq)")
			require.Equal(testInstance, defaultBranch, strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current")))
			require.Equal(testInstance, publishedHead, strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD")))
			require.Equal(testInstance, publishedHead, strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", defaultBranch)))
			require.Equal(testInstance, llmCalls, fixture.llmCalls.Load())
			require.Equal(testInstance, 1, strings.Count(readTextFile(testInstance, fixture.gitLog), "push origin qqq "))
			require.NotContains(testInstance, readTextFile(testInstance, fixture.githubLog), "pr create ")
		})
	}
}
