package tests

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

type syncFixture struct {
	workspace, repository, remote, config, gitLog, githubLog, executablePath string
	environment                                                              map[string]string
	llmCalls                                                                 *atomic.Int64
}

func newSyncFixture(testInstance *testing.T, branch string) syncFixture {
	testInstance.Helper()
	workspace := syncHomeWorkspace(testInstance)
	fixture := syncFixture{workspace: workspace, repository: filepath.Join(workspace, "project"), remote: filepath.Join(workspace, "remote.git"), gitLog: filepath.Join(workspace, "git.log"), githubLog: filepath.Join(workspace, "gh.log"), llmCalls: &atomic.Int64{}}
	createSyncGitHubBackedRepository(testInstance, fixture.remote, fixture.repository)
	runGit(testInstance, fixture.repository, "symbolic-ref", "HEAD", "refs/heads/"+branch)
	runGit(testInstance, fixture.remote, "symbolic-ref", "HEAD", "refs/heads/"+branch)
	fixture.commitFile(testInstance, "README.md", "initial\n")
	runGit(testInstance, fixture.repository, "push", "-u", "origin", branch)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fixture.llmCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fix: preserve file work"}}]}`))
	}))
	testInstance.Cleanup(server.Close)
	fixture.config = writeDirtySyncMergedBranchConfiguration(testInstance, server.URL)
	fixture.executablePath = buildSyncMergedBranchExecutablePath(testInstance)
	fixture.environment = map[string]string{syncMergedBranchGitLogVariable: fixture.gitLog, syncMergedBranchGitHubLogVariable: fixture.githubLog, syncMergedBranchDefaultBranchVariable: branch, syncMergedBranchMergedVariable: "false"}
	return fixture
}

func (fixture syncFixture) commitFile(testInstance *testing.T, name, contents string) {
	testInstance.Helper()
	require.NoError(testInstance, os.WriteFile(filepath.Join(fixture.repository, name), []byte(contents), 0o644))
	runGit(testInstance, fixture.repository, "add", name)
	runGit(testInstance, fixture.repository, "commit", "-m", "fix: preserve file work")
}

func (fixture syncFixture) run(testInstance *testing.T, binaryPath string, arguments ...string) (string, error) {
	fixture.environment["GIX_SYNC_TEST_REPOSITORY"] = fixture.repository

	testInstance.Helper()
	fixture.environment["PATH"] = fixture.executablePath
	return runBinaryIntegrationCommandWithInput(testInstance, binaryPath, integrationRepositoryRoot(testInstance), fixture.environment, syncMergedBranchIntegrationTimeout, "", append([]string{"--config", fixture.config, "--roots", fixture.repository}, arguments...))
}
