package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	syncProtectedVariable          = "GIX_SYNC_TEST_PROTECTED"
	syncProtectionErrorVariable    = "GIX_SYNC_TEST_PROTECTION_ERROR"
	syncProtectionResponseVariable = "GIX_SYNC_TEST_PROTECTION_RESPONSE"
)

func TestSyncDefaultPublicationPolicy(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	for _, scenario := range []struct {
		name      string
		branch    string
		protected bool
		dirty     bool
		ahead     bool
		behind    bool
		stash     bool
	}{
		{name: "dirty_main", branch: "main", protected: true, dirty: true},
		{name: "dirty_master", branch: "master", protected: true, dirty: true},
		{name: "dirty_qqq", branch: "qqq", protected: true, dirty: true},
		{name: "dirty_default", branch: "default", protected: true, dirty: true},
		{name: "dirty_slash", branch: "release/trunk", protected: true, dirty: true},
		{name: "unprotected_dirty", branch: "qqq", dirty: true},
		{name: "unprotected_divergent", branch: "qqq", ahead: true, behind: true},
		{name: "protected_ahead", branch: "qqq", protected: true, ahead: true},
		{name: "protected_divergent", branch: "main", protected: true, ahead: true, behind: true},
		{name: "protected_dirty_divergent", branch: "default", protected: true, dirty: true, ahead: true, behind: true},
		{name: "protected_behind", branch: "master", protected: true, behind: true},
		{name: "protected_current", branch: "qqq", protected: true},
		{name: "protected_stash", branch: "qqq", protected: true, dirty: true, behind: true, stash: true},
	} {
		testInstance.Run(scenario.name, func(testInstance *testing.T) {
			fixture := newProtectedSyncFixture(testInstance, scenario.branch)
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
			fixture.environment[syncProtectedVariable] = fmt.Sprint(scenario.protected)
			arguments := []string{"sync", scenario.branch}
			if scenario.stash {
				arguments = append(arguments, "--stash")
			}
			output, runError := fixture.run(testInstance, binaryPath, arguments...)
			require.NoError(testInstance, runError, output)
			require.Contains(testInstance, output, "SYNCED:")
			require.NotContains(testInstance, output, "SYNC_SWITCH_ROLLBACK")
			currentBranch := strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current"))
			needsReview := scenario.protected && (scenario.ahead || scenario.dirty && !scenario.stash)
			githubLog := readTextFile(testInstance, fixture.githubLog)
			gitLog := readTextFile(testInstance, fixture.gitLog)
			if needsReview {
				require.NotEqual(testInstance, scenario.branch, currentBranch)
				require.Contains(testInstance, githubLog, "created-pr --base "+scenario.branch+" --head "+currentBranch)
				require.NotContains(testInstance, githubLog, "--draft")
				require.Equal(testInstance, localBefore, strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", scenario.branch)))
				require.Equal(testInstance, remoteBefore, strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", scenario.branch)))
				require.Equal(testInstance, strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD")), strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", currentBranch)))
				runGit(testInstance, fixture.repository, "merge-base", "--is-ancestor", localBefore, currentBranch)
				runGit(testInstance, fixture.repository, "merge-base", "--is-ancestor", remoteBefore, currentBranch)
			} else {
				require.Equal(testInstance, scenario.branch, currentBranch)
				require.NotContains(testInstance, githubLog, "pr create ")
				require.Equal(testInstance, strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", scenario.branch)), strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD")))
			}
			if scenario.protected {
				require.NotContains(testInstance, gitLog, "push origin "+scenario.branch+" ")
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
			if scenario.dirty && !scenario.stash || scenario.ahead {
				require.Contains(testInstance, githubLog, "api repos/owner/project/branches/"+url.PathEscape(scenario.branch)+" ")
			}
		})
	}
}

func TestSyncDefaultProtectionLookupFailurePreservesWork(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	for _, scenario := range []struct{ name, response, failure string }{
		{name: "forbidden", failure: "HTTP 403: inaccessible branch"},
		{name: "not_found", failure: "HTTP 404: inaccessible branch"},
		{name: "missing_field", response: `{}`},
		{name: "null_field", response: `{"protected":null}`},
		{name: "wrong_type", response: `{"protected":"false"}`},
		{name: "malformed", response: `{`},
	} {
		testInstance.Run(scenario.name, func(testInstance *testing.T) {
			fixture := newProtectedSyncFixture(testInstance, "qqq")
			headBefore := strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD"))
			require.NoError(testInstance, os.WriteFile(filepath.Join(fixture.repository, "README.md"), []byte("staged\n"), 0o644))
			runGit(testInstance, fixture.repository, "add", "README.md")
			require.NoError(testInstance, os.WriteFile(filepath.Join(fixture.repository, "README.md"), []byte("unstaged\n"), 0o644))
			fixture.environment[syncProtectionErrorVariable] = scenario.failure
			fixture.environment[syncProtectionResponseVariable] = scenario.response
			output, runError := fixture.run(testInstance, binaryPath, "sync", "qqq")
			require.Error(testInstance, runError, output)
			require.Contains(testInstance, output, "publication policy")
			require.Contains(testInstance, output, "qqq")
			require.Equal(testInstance, int64(0), fixture.llmCalls.Load())
			require.NotContains(testInstance, readTextFile(testInstance, fixture.gitLog), "commit -m ")
			require.NotContains(testInstance, readTextFile(testInstance, fixture.gitLog), "push origin ")
			require.Equal(testInstance, headBefore, strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD")))
			require.Equal(testInstance, headBefore, strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", "HEAD")))
			require.Equal(testInstance, "staged\n", runGit(testInstance, fixture.repository, "show", ":README.md"))
			require.Equal(testInstance, "unstaged\n", readTextFile(testInstance, filepath.Join(fixture.repository, "README.md")))
		})
	}
}

func TestSyncProtectedDefaultPreservesPublicationRecovery(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	for _, phase := range []string{"push", "pull_request"} {
		testInstance.Run(phase, func(testInstance *testing.T) {
			fixture := newProtectedSyncFixture(testInstance, "qqq")
			originalHead := strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD"))
			require.NoError(testInstance, os.WriteFile(filepath.Join(fixture.repository, "README.md"), []byte("pending work\n"), 0o644))
			fixture.environment[syncProtectedVariable] = "true"
			if phase == "push" {
				fixture.environment[syncMergedBranchFailGitMatchVariable] = "push -u origin gix/"
				fixture.environment[syncMergedBranchFailGitOccurrenceVariable] = "1"
				fixture.environment[syncMergedBranchFailGitStateVariable] = filepath.Join(fixture.workspace, "push-failure")
			} else {
				fixture.environment[syncMergedBranchFailPullRequestHeadVariable] = "gix/preserve-protected-work"
			}
			output, runError := fixture.run(testInstance, binaryPath, "sync", "qqq")
			require.Error(testInstance, runError, output)
			require.NotContains(testInstance, output, "SYNCED:")
			require.Equal(testInstance, originalHead, strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "qqq")))
			require.Equal(testInstance, originalHead, strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", "qqq")))
			require.Equal(testInstance, "pending work\n", readTextFile(testInstance, filepath.Join(fixture.repository, "README.md")))
			currentBranch := strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current"))
			if phase == "push" {
				require.Contains(testInstance, output, "SYNC_SWITCH_ROLLBACK")
				require.Equal(testInstance, "qqq", currentBranch)
				require.Equal(testInstance, "M README.md", strings.TrimSpace(runGit(testInstance, fixture.repository, "status", "--porcelain")))
				require.Equal(testInstance, "qqq", strings.TrimSpace(runGit(testInstance, fixture.repository, "for-each-ref", "--format=%(refname:short)", "refs/heads/")))
			} else {
				require.Contains(testInstance, output, "SYNC_SWITCH_HANDOFF")
				require.NotContains(testInstance, output, "SYNC_SWITCH_ROLLBACK")
				require.NotEqual(testInstance, "qqq", currentBranch)
				require.Equal(testInstance, strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD")), strings.TrimSpace(runGit(testInstance, fixture.remote, "rev-parse", currentBranch)))
				delete(fixture.environment, syncMergedBranchFailPullRequestHeadVariable)
				output, runError = fixture.run(testInstance, binaryPath, "sync")
				require.NoError(testInstance, runError, output)
				require.Contains(testInstance, readTextFile(testInstance, fixture.githubLog), "created-pr --base qqq --head "+currentBranch)
			}
		})
	}
}

func TestSyncProtectedDefaultReusesCommittedReview(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	fixture := newProtectedSyncFixture(testInstance, "qqq")
	fixture.commitFile(testInstance, "local.txt", "local work\n")
	fixture.environment[syncProtectedVariable] = "true"
	output, runError := fixture.run(testInstance, binaryPath, "sync", "qqq")
	require.NoError(testInstance, runError, output)
	reviewBranch := strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current"))
	output, runError = fixture.run(testInstance, binaryPath, "sync", "qqq")
	require.NoError(testInstance, runError, output)
	require.Equal(testInstance, reviewBranch, strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current")))
	require.Equal(testInstance, 1, strings.Count(readTextFile(testInstance, fixture.githubLog), "created-pr --base qqq --head "))
}

func TestSyncProtectedDefaultHonorsConfiguredReviewMetadata(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	fixture := newProtectedSyncFixture(testInstance, "qqq")
	fixture.commitFile(testInstance, "local.txt", "local work\n")
	fixture.environment[syncProtectedVariable] = "true"
	configuration := strings.Replace(readTextFile(testInstance, fixture.config), "remote: origin", "remote: origin\n      pull_request:\n        title: Explicit review title\n        body: Explicit review body", 1)
	require.NoError(testInstance, os.WriteFile(fixture.config, []byte(configuration), 0o600))
	output, runError := fixture.run(testInstance, binaryPath, "sync", "qqq")
	require.NoError(testInstance, runError, output)
	require.Contains(testInstance, readTextFile(testInstance, fixture.githubLog), "--title Explicit review title --body Explicit review body")
	require.Zero(testInstance, fixture.llmCalls.Load())
}

func TestSyncProtectedDefaultRejectsReviewWithDifferentBase(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	fixture := newProtectedSyncFixture(testInstance, "qqq")
	fixture.commitFile(testInstance, "local.txt", "local work\n")
	fixture.environment[syncProtectedVariable] = "true"
	output, runError := fixture.run(testInstance, binaryPath, "sync", "qqq")
	require.NoError(testInstance, runError, output)
	reviewBranch := strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current"))
	runGit(testInstance, fixture.repository, "push", "origin", "qqq:refs/heads/other-base")
	log := strings.Replace(readTextFile(testInstance, fixture.githubLog), "created-pr --base qqq --head "+reviewBranch, "created-pr --base other-base --head "+reviewBranch, 1)
	require.NoError(testInstance, os.WriteFile(fixture.githubLog, []byte(log), 0o600))
	output, runError = fixture.run(testInstance, binaryPath, "sync", "qqq")
	require.NoError(testInstance, runError, output)
	currentBranch := strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current"))
	require.NotEqual(testInstance, reviewBranch, currentBranch)
	require.Contains(testInstance, readTextFile(testInstance, fixture.githubLog), "created-pr --base qqq --head "+currentBranch)
}

func TestSyncProtectedDefaultFromFeaturePreservesTargetScope(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	fixture := newProtectedSyncFixture(testInstance, "qqq")
	runGit(testInstance, fixture.repository, "switch", "-c", "feature/source")
	fixture.commitFile(testInstance, "feature.txt", "unrelated feature\n")
	sourceHead := strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "HEAD"))
	require.NoError(testInstance, os.WriteFile(filepath.Join(fixture.repository, "README.md"), []byte("pending work\n"), 0o644))
	fixture.environment[syncProtectedVariable] = "true"
	output, runError := fixture.run(testInstance, binaryPath, "sync", "qqq")
	require.NoError(testInstance, runError, output)
	require.Equal(testInstance, sourceHead, strings.TrimSpace(runGit(testInstance, fixture.repository, "rev-parse", "feature/source")))
	require.NoFileExists(testInstance, filepath.Join(fixture.repository, "feature.txt"))
	require.Equal(testInstance, "pending work\n", readTextFile(testInstance, filepath.Join(fixture.repository, "README.md")))
	require.Contains(testInstance, readTextFile(testInstance, fixture.githubLog), "created-pr --base qqq --head ")
}

func TestSyncNamedMainRemainsOrdinaryUnderProtectedDefault(testInstance *testing.T) {
	binaryPath := buildIntegrationBinary(testInstance, integrationRepositoryRoot(testInstance))
	fixture := newProtectedSyncFixture(testInstance, "qqq")
	require.NoError(testInstance, os.WriteFile(filepath.Join(fixture.repository, "README.md"), []byte("pending work\n"), 0o644))
	fixture.environment[syncProtectedVariable] = "true"
	output, runError := fixture.run(testInstance, binaryPath, "sync", "main")
	require.NoError(testInstance, runError, output)
	require.Equal(testInstance, "main", strings.TrimSpace(runGit(testInstance, fixture.repository, "branch", "--show-current")))
	require.Contains(testInstance, readTextFile(testInstance, fixture.githubLog), "created-pr --base qqq --head main")
}

type protectedSyncFixture struct {
	workspace, repository, remote, config, gitLog, githubLog, executablePath string
	environment                                                              map[string]string
	llmCalls                                                                 *atomic.Int64
}

func newProtectedSyncFixture(testInstance *testing.T, branch string) protectedSyncFixture {
	testInstance.Helper()
	workspace := syncHomeWorkspace(testInstance)
	fixture := protectedSyncFixture{workspace: workspace, repository: filepath.Join(workspace, "project"), remote: filepath.Join(workspace, "remote.git"), gitLog: filepath.Join(workspace, "git.log"), githubLog: filepath.Join(workspace, "gh.log"), llmCalls: &atomic.Int64{}}
	createSyncGitHubBackedRepository(testInstance, fixture.remote, fixture.repository)
	runGit(testInstance, fixture.repository, "symbolic-ref", "HEAD", "refs/heads/"+branch)
	runGit(testInstance, fixture.remote, "symbolic-ref", "HEAD", "refs/heads/"+branch)
	fixture.commitFile(testInstance, "README.md", "initial\n")
	runGit(testInstance, fixture.repository, "push", "-u", "origin", branch)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fixture.llmCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fix: preserve protected work"}}]}`))
	}))
	testInstance.Cleanup(server.Close)
	fixture.config = writeDirtySyncMergedBranchConfiguration(testInstance, server.URL)
	fixture.executablePath = buildSyncMergedBranchExecutablePath(testInstance)
	fixture.environment = map[string]string{syncMergedBranchGitLogVariable: fixture.gitLog, syncMergedBranchGitHubLogVariable: fixture.githubLog, syncMergedBranchDefaultBranchVariable: branch, syncMergedBranchMergedVariable: "false"}
	return fixture
}

func (fixture protectedSyncFixture) commitFile(testInstance *testing.T, name, contents string) {
	testInstance.Helper()
	require.NoError(testInstance, os.WriteFile(filepath.Join(fixture.repository, name), []byte(contents), 0o644))
	runGit(testInstance, fixture.repository, "add", name)
	runGit(testInstance, fixture.repository, "commit", "-m", "fix: preserve protected work")
}

func (fixture protectedSyncFixture) run(testInstance *testing.T, binaryPath string, arguments ...string) (string, error) {
	testInstance.Helper()
	fixture.environment["PATH"] = fixture.executablePath
	return runBinaryIntegrationCommandWithInput(testInstance, binaryPath, integrationRepositoryRoot(testInstance), fixture.environment, syncMergedBranchIntegrationTimeout, "", append([]string{"--config", fixture.config, "--roots", fixture.repository}, arguments...))
}
