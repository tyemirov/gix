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

func TestSyncAuditsLargeAdditiveIssueAndChangelogConflictsBySemanticRegion(testInstance *testing.T) {
	testInstance.Helper()

	const (
		baseBranchName   = "feature/review-base"
		targetBranchName = "bugfix/semantic-merge-target"
	)

	issuePrefix := "# ISSUES\n\n## BugFixes\n\n"
	oursIssueBlock := "- [x] [B041] Preserve the local rollback fix.\n  Resolution:\n  Local rollback remains authoritative.\n\n"
	theirsIssueBlock := "- [x] [B042] Preserve the incoming license pin.\n  Resolution:\n  Incoming pin remains authoritative.\n\n- [x] [B043] Preserve the incoming review validation.\n  Resolution:\n  Incoming validation remains authoritative.\n\n"
	var stableIssueSuffixBuilder strings.Builder
	stableIssueSuffixBuilder.WriteString("## Maintenance\n\n")
	for issueIndex := 1; issueIndex <= 600; issueIndex++ {
		_, _ = fmt.Fprintf(
			&stableIssueSuffixBuilder,
			"- [ ] [M%03d] Stable unrelated maintenance entry %03d.\n",
			issueIndex,
			issueIndex,
		)
	}
	stableIssueSuffix := stableIssueSuffixBuilder.String()

	changelogPrefix := "# Changelog\n\n## Unreleased\n\n### Bug Fixes\n"
	oursChangelogBlock := "- Restored the local branch after rejected AI merge resolution.\n"
	theirsChangelogBlock := "- Pinned license rollout clones to inspected commits.\n- Validated existing rollout pull requests before skipping them.\n"
	changelogSuffix := "\n### Documentation\n- Stable unrelated documentation note.\n"

	baseIssues := issuePrefix + stableIssueSuffix
	oursIssues := issuePrefix + oursIssueBlock + stableIssueSuffix
	theirsIssues := issuePrefix + theirsIssueBlock + stableIssueSuffix + "\n"
	expectedIssues := issuePrefix + oursIssueBlock + theirsIssueBlock + stableIssueSuffix + "\n"
	baseChangelog := changelogPrefix + changelogSuffix
	oursChangelog := changelogPrefix + oursChangelogBlock + changelogSuffix
	theirsChangelog := changelogPrefix + theirsChangelogBlock + changelogSuffix
	expectedChangelog := changelogPrefix + oursChangelogBlock + theirsChangelogBlock + changelogSuffix

	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	issuesPath := filepath.Join(repositoryPath, ".mprlab", "ISSUES.md")
	changelogPath := filepath.Join(repositoryPath, "CHANGELOG.md")
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(issuesPath), 0o755))
	require.NoError(testInstance, os.WriteFile(issuesPath, []byte(baseIssues), 0o644))
	require.NoError(testInstance, os.WriteFile(changelogPath, []byte(baseChangelog), 0o644))
	runGit(testInstance, repositoryPath, "add", ".mprlab/ISSUES.md", "CHANGELOG.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "seed semantic merge fixture")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", baseBranchName)
	require.NoError(testInstance, os.WriteFile(issuesPath, []byte(theirsIssues), 0o644))
	require.NoError(testInstance, os.WriteFile(changelogPath, []byte(theirsChangelog), 0o644))
	runGit(testInstance, repositoryPath, "add", ".mprlab/ISSUES.md", "CHANGELOG.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "add incoming issue and changelog entries")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", baseBranchName)
	incomingCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))

	runGit(testInstance, repositoryPath, "switch", "master")
	runGit(testInstance, repositoryPath, "switch", "-c", targetBranchName)
	require.NoError(testInstance, os.WriteFile(issuesPath, []byte(oursIssues), 0o644))
	require.NoError(testInstance, os.WriteFile(changelogPath, []byte(oursChangelog), 0o644))
	runGit(testInstance, repositoryPath, "add", ".mprlab/ISSUES.md", "CHANGELOG.md")
	runGit(testInstance, repositoryPath, "commit", "-m", "add local issue and changelog entries")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", targetBranchName)
	targetCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))

	var requestCount atomic.Int64
	requestBodies := make(chan string, 2)
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		requestBody, requestReadError := io.ReadAll(request.Body)
		if requestReadError != nil {
			http.Error(responseWriter, requestReadError.Error(), http.StatusBadRequest)
			return
		}
		requestBodies <- string(requestBody)
		requestNumber := requestCount.Add(1)
		if requestNumber > 2 {
			http.Error(responseWriter, "additive resolution exceeded one semantic audit per conflict region", http.StatusInternalServerError)
			return
		}
		if requestNumber == 1 {
			advanceReferenceCommand := exec.Command(
				"git",
				"-C",
				repositoryPath,
				"update-ref",
				"refs/remotes/origin/"+baseBranchName,
				targetCommit,
				incomingCommit,
			)
			advanceReferenceCommand.Env = buildGitCommandEnvironment(nil)
			advanceReferenceOutput, advanceReferenceErr := advanceReferenceCommand.CombinedOutput()
			if advanceReferenceErr != nil {
				http.Error(responseWriter, string(advanceReferenceOutput), http.StatusInternalServerError)
				return
			}
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(
			responseWriter,
			`{"choices":[{"message":{"role":"assistant","content":%q}}]}`,
			"GIX_MERGE_REVIEW_APPROVED",
		)
	}))
	testInstance.Cleanup(llmServer.Close)

	configurationPath := filepath.Join(testInstance.TempDir(), "config.yml")
	configurationContent := fmt.Sprintf(`common:
  log_level: error
  log_format: console
  require_clean: true
github:
  credential: test-github-key
llm:
  openai:
    priority: 1
    model: mock-model
    base_url: %q
    credential: test-key
  llm_proxy:
    priority: 2
    provider: meta
    model: muse-spark-1.1
    base_url: "https://llm-proxy.example"
    credential: test-proxy-key
  max_completion_tokens: 64
  effort: "high"
  timeout_seconds: 5
operations:
  - command: ["sync"]
    with:
      remote: origin
      require_clean: true
`, llmServer.URL)
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))

	gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	require.NoError(
		testInstance,
		os.WriteFile(
			githubLogPath,
			[]byte("created-pr --base "+baseBranchName+" --head "+targetBranchName+"\n"),
			0o600,
		),
	)

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: buildSyncMergedBranchExecutablePath(testInstance),
			EnvironmentOverrides: map[string]string{
				syncMergedBranchGitLogVariable:    gitLogPath,
				syncMergedBranchGitHubLogVariable: githubLogPath,
				syncMergedBranchNameVariable:      targetBranchName,
				syncMergedBranchMergedVariable:    "false",
			},
		},
		syncMergedBranchIntegrationTimeout,
		"",
		[]string{
			syncRefreshIntegrationRunCommand,
			syncRefreshIntegrationModulePath,
			"--config",
			configurationPath,
			syncRefreshIntegrationLogLevelFlag,
			syncRefreshIntegrationErrorLogLevel,
			"sync",
			targetBranchName,
			"--roots",
			repositoryPath,
		},
	)
	require.NoError(testInstance, runError, output)
	require.Contains(testInstance, output, "MERGE_CONFLICT")
	require.Contains(testInstance, output, "derived .mprlab/ISSUES.md conflict region 1/1 candidate using concurrent insertions; requesting semantic audit")
	require.Contains(testInstance, output, "derived CHANGELOG.md conflict region 1/1 candidate using concurrent insertions; requesting semantic audit")
	require.Equal(testInstance, 2, strings.Count(output, "semantic audit approved"))
	require.Contains(testInstance, output, "accepted inherited whitespace for .mprlab/ISSUES.md from incoming parent origin/"+baseBranchName)
	require.Contains(testInstance, output, "merge conflict resolution completed")
	require.NotContains(testInstance, output, "AI_MERGE_ROLLBACK")
	require.NotContains(testInstance, output, "AI_MERGE_HANDOFF")
	require.Equal(testInstance, int64(2), requestCount.Load())
	require.Equal(testInstance, expectedIssues, readTextFile(testInstance, issuesPath))
	require.Equal(testInstance, expectedChangelog, readTextFile(testInstance, changelogPath))
	require.Equal(testInstance, targetCommit, strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "origin/"+baseBranchName)))
	require.Equal(testInstance, 1, strings.Count(readTextFile(testInstance, issuesPath), "[B041]"))
	require.Equal(testInstance, 1, strings.Count(readTextFile(testInstance, issuesPath), "[B042]"))
	require.Equal(testInstance, 1, strings.Count(readTextFile(testInstance, issuesPath), "[B043]"))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))

	firstRequest := <-requestBodies
	secondRequest := <-requestBodies
	requestsByPath := map[string]string{}
	for _, requestBody := range []string{firstRequest, secondRequest} {
		switch {
		case strings.Contains(requestBody, ".mprlab/ISSUES.md"):
			requestsByPath[".mprlab/ISSUES.md"] = requestBody
		case strings.Contains(requestBody, "CHANGELOG.md"):
			requestsByPath["CHANGELOG.md"] = requestBody
		}
	}
	require.Len(testInstance, requestsByPath, 2)
	require.Contains(testInstance, requestsByPath[".mprlab/ISSUES.md"], "semantic fidelity auditor")
	require.Contains(testInstance, requestsByPath[".mprlab/ISSUES.md"], "Preserve the local rollback fix.")
	require.Contains(testInstance, requestsByPath[".mprlab/ISSUES.md"], "Preserve the incoming license pin.")
	require.Contains(testInstance, requestsByPath[".mprlab/ISSUES.md"], "Preserve the incoming review validation.")
	require.NotContains(testInstance, requestsByPath[".mprlab/ISSUES.md"], "Stable unrelated maintenance entry 600")
	require.Contains(testInstance, requestsByPath["CHANGELOG.md"], "semantic fidelity auditor")
	require.Contains(testInstance, requestsByPath["CHANGELOG.md"], "Restored the local branch")
	require.Contains(testInstance, requestsByPath["CHANGELOG.md"], "Pinned license rollout clones")
	require.Contains(testInstance, requestsByPath["CHANGELOG.md"], "Validated existing rollout pull requests")
	require.NotContains(testInstance, requestsByPath["CHANGELOG.md"], "Stable unrelated documentation note")

	headWithParents := strings.Fields(runGit(testInstance, repositoryPath, "rev-list", "--parents", "-n", "1", "HEAD"))
	require.Len(testInstance, headWithParents, 3)
	require.Equal(testInstance, targetCommit, headWithParents[1])
	require.Equal(testInstance, incomingCommit, headWithParents[2])
	require.Equal(
		testInstance,
		strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD")),
		strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "origin/"+targetBranchName)),
	)

	mergeHeadCommand := exec.Command("git", "-C", repositoryPath, "rev-parse", "--verify", "MERGE_HEAD")
	mergeHeadCommand.Env = buildGitCommandEnvironment(nil)
	mergeHeadOutput, mergeHeadError := mergeHeadCommand.CombinedOutput()
	require.Error(testInstance, mergeHeadError, string(mergeHeadOutput))
	gitLog := readTextFile(testInstance, gitLogPath)
	require.Contains(testInstance, gitLog, "merge --no-edit origin/"+baseBranchName)
	require.Contains(testInstance, gitLog, "commit --no-edit")
	require.Contains(testInstance, gitLog, "push origin "+targetBranchName)
	require.NotContains(testInstance, gitLog, "merge --abort")
}

func TestSyncRepairsRejectedSemanticAuditCorrectionsBeforeCommit(testInstance *testing.T) {
	const (
		baseBranchName         = "feature/semantic-review-base"
		targetBranchName       = "bugfix/semantic-review-target"
		conflictedPath         = "reviewers.txt"
		baseContent            = "reviewers: alice\n"
		oursContent            = "reviewers: alice, bob\n"
		theirsContent          = "reviewers: alice, carol\n"
		markerBearingContent   = "<<<<<<< OURS\n" + oursContent + "=======\n" + theirsContent + ">>>>>>> THEIRS\n"
		expectedMergedContent  = "reviewers: alice, bob, carol\n"
		reviewApprovedResponse = "GIX_MERGE_REVIEW_APPROVED"
	)

	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	conflictedFilePath := filepath.Join(repositoryPath, conflictedPath)
	require.NoError(testInstance, os.WriteFile(conflictedFilePath, []byte(baseContent), 0o644))
	runGit(testInstance, repositoryPath, "add", conflictedPath)
	runGit(testInstance, repositoryPath, "commit", "-m", "seed semantic review fixture")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", baseBranchName)
	require.NoError(testInstance, os.WriteFile(conflictedFilePath, []byte(theirsContent), 0o644))
	runGit(testInstance, repositoryPath, "add", conflictedPath)
	runGit(testInstance, repositoryPath, "commit", "-m", "add incoming reviewer")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", baseBranchName)

	runGit(testInstance, repositoryPath, "switch", "master")
	runGit(testInstance, repositoryPath, "switch", "-c", targetBranchName)
	require.NoError(testInstance, os.WriteFile(conflictedFilePath, []byte(oursContent), 0o644))
	runGit(testInstance, repositoryPath, "add", conflictedPath)
	runGit(testInstance, repositoryPath, "commit", "-m", "add local reviewer")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", targetBranchName)

	var requestCount atomic.Int64
	requestBodies := make(chan string, 4)
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(responseWriter, request)
			return
		}
		requestBody, requestReadError := io.ReadAll(request.Body)
		if requestReadError != nil {
			http.Error(responseWriter, requestReadError.Error(), http.StatusBadRequest)
			return
		}
		requestBodies <- string(requestBody)
		requestIndex := requestCount.Add(1)

		var response string
		switch requestIndex {
		case 1:
			response = semanticMergeResponse(markerBearingContent)
		case 2:
			response = semanticMergeResponse(oursContent)
		case 3:
			response = semanticMergeResponse(expectedMergedContent)
		case 4:
			response = reviewApprovedResponse
		default:
			http.Error(responseWriter, "semantic resolution exceeded its bounded strategy ladder", http.StatusInternalServerError)
			return
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(responseWriter, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, response)
	}))
	testInstance.Cleanup(llmServer.Close)

	configurationPath := filepath.Join(testInstance.TempDir(), "config.yml")
	configurationContent := fmt.Sprintf(`common:
  log_level: error
  log_format: console
  require_clean: true
github:
  credential: test-github-key
llm:
  openai:
    priority: 1
    model: mock-model
    base_url: %q
    credential: test-key
  llm_proxy:
    priority: 2
    provider: meta
    model: muse-spark-1.1
    base_url: "https://llm-proxy.example"
    credential: test-proxy-key
  max_completion_tokens: 64
  effort: "high"
  timeout_seconds: 5
operations:
  - command: ["sync"]
    with:
      remote: origin
      require_clean: true
`, llmServer.URL)
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))

	gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	require.NoError(
		testInstance,
		os.WriteFile(
			githubLogPath,
			[]byte("created-pr --base "+baseBranchName+" --head "+targetBranchName+"\n"),
			0o600,
		),
	)

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: buildSyncMergedBranchExecutablePath(testInstance),
			EnvironmentOverrides: map[string]string{
				syncMergedBranchGitLogVariable:    gitLogPath,
				syncMergedBranchGitHubLogVariable: githubLogPath,
				syncMergedBranchNameVariable:      targetBranchName,
				syncMergedBranchMergedVariable:    "false",
			},
		},
		syncMergedBranchIntegrationTimeout,
		"",
		[]string{
			syncRefreshIntegrationRunCommand,
			syncRefreshIntegrationModulePath,
			"--config",
			configurationPath,
			syncRefreshIntegrationLogLevelFlag,
			syncRefreshIntegrationErrorLogLevel,
			"sync",
			targetBranchName,
			"--roots",
			repositoryPath,
		},
	)
	require.NoError(testInstance, runError, output)
	require.Contains(testInstance, output, "derived reviewers.txt conflict region 1/1 candidate using compatible token edits; requesting semantic audit")
	require.Contains(testInstance, output, "semantic audit attempt 1/4 rejected")
	require.Contains(testInstance, output, "llm left conflict markers")
	require.Contains(testInstance, output, "retrying semantic audit")
	require.Contains(testInstance, output, "semantic audit approved")
	require.Contains(testInstance, output, "merge conflict resolution completed")
	require.NotContains(testInstance, output, "AI_MERGE_ROLLBACK")
	require.NotContains(testInstance, output, "AI_MERGE_HANDOFF")
	require.Equal(testInstance, int64(4), requestCount.Load())
	require.Equal(testInstance, expectedMergedContent, readTextFile(testInstance, conflictedFilePath))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))

	firstRequest := <-requestBodies
	secondRequest := <-requestBodies
	thirdRequest := <-requestBodies
	fourthRequest := <-requestBodies
	require.Contains(testInstance, firstRequest, "semantic fidelity auditor")
	require.Contains(testInstance, firstRequest, "reviewers: alice, bob\\n")
	require.Contains(testInstance, firstRequest, "reviewers: alice, carol\\n")
	require.Contains(testInstance, firstRequest, "LOCALLY VALIDATED CANDIDATE:\\nreviewers: alice, bob, carol\\n")
	require.Contains(testInstance, secondRequest, "llm left conflict markers")
	require.Contains(testInstance, secondRequest, "reviewers: alice, bob\\n")
	require.Contains(testInstance, thirdRequest, "does not preserve THEIRS replacement intent")
	require.Contains(testInstance, thirdRequest, "reviewers: alice, bob\\n")
	require.Contains(testInstance, fourthRequest, "reviewers: alice, bob, carol\\n")
	require.Contains(testInstance, fourthRequest, "semantic fidelity auditor")

	gitLog := readTextFile(testInstance, gitLogPath)
	require.Contains(testInstance, gitLog, "commit --no-edit")
	require.Contains(testInstance, gitLog, "push origin "+targetBranchName)
	require.NotContains(testInstance, gitLog, "merge --abort")
}

func TestSyncDerivesReplacementAndDeletionCandidatesBeforeSemanticAudit(testInstance *testing.T) {
	const (
		baseBranchName                 = "feature/versionless-lifecycle"
		targetBranchName               = "gix/migrate-lifecycle-manifest-to-schema-version-5"
		conflictedPath                 = "tests/lifecycle_contract_test.go"
		versionlessFunctionDeclaration = "func TestOperationalRepositoryOwnsVersionlessLifecycle(testingInstance *testing.T) {\n"
		reviewApprovedResponse         = "GIX_MERGE_REVIEW_APPROVED"
	)
	fixture := newReportedLifecycleConflictFixture()

	repositoryRoot := integrationRepositoryRoot(testInstance)
	workspacePath := syncHomeWorkspace(testInstance)
	remotePath := filepath.Join(workspacePath, "remote.git")
	repositoryPath := filepath.Join(workspacePath, "project")
	createSyncGitHubBackedRepository(testInstance, remotePath, repositoryPath)

	conflictedFilePath := filepath.Join(repositoryPath, conflictedPath)
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(conflictedFilePath), 0o755))
	require.NoError(testInstance, os.WriteFile(conflictedFilePath, []byte(fixture.Base), 0o644))
	runGit(testInstance, repositoryPath, "add", conflictedPath)
	runGit(testInstance, repositoryPath, "commit", "-m", "seed lifecycle contract fixture")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", "master")

	runGit(testInstance, repositoryPath, "switch", "-c", baseBranchName)
	require.NoError(testInstance, os.WriteFile(conflictedFilePath, []byte(fixture.Theirs), 0o644))
	runGit(testInstance, repositoryPath, "add", conflictedPath)
	runGit(testInstance, repositoryPath, "commit", "-m", "remove lifecycle schema version")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", baseBranchName)
	incomingCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))

	runGit(testInstance, repositoryPath, "switch", "master")
	runGit(testInstance, repositoryPath, "switch", "-c", targetBranchName)
	require.NoError(testInstance, os.WriteFile(conflictedFilePath, []byte(fixture.Ours), 0o644))
	runGit(testInstance, repositoryPath, "add", conflictedPath)
	runGit(testInstance, repositoryPath, "commit", "-m", "update lifecycle schema version")
	runGit(testInstance, repositoryPath, "push", "-u", "origin", targetBranchName)
	targetCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))

	var requestCount atomic.Int64
	requestBodies := make(chan string, 8)
	auditAttempts := [3]int{}
	llmServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		requestBody, requestReadError := io.ReadAll(request.Body)
		if requestReadError != nil {
			http.Error(responseWriter, requestReadError.Error(), http.StatusBadRequest)
			return
		}
		requestBodyText := string(requestBody)
		requestBodies <- requestBodyText
		requestCount.Add(1)
		regionIndex := 0
		for candidateRegionIndex := 1; candidateRegionIndex <= 2; candidateRegionIndex++ {
			if strings.Contains(requestBodyText, fmt.Sprintf("Conflict region: %d of 2", candidateRegionIndex)) {
				regionIndex = candidateRegionIndex
				break
			}
		}
		if regionIndex == 0 {
			http.Error(responseWriter, "reported lifecycle request has no conflict region", http.StatusBadRequest)
			return
		}
		if !strings.Contains(requestBodyText, "semantic fidelity auditor") {
			http.Error(responseWriter, "unexpected semantic candidate request for a derivable reported conflict region", http.StatusConflict)
			return
		}
		auditAttempts[regionIndex]++
		response := reviewApprovedResponse
		if auditAttempts[regionIndex] == 1 {
			correction := versionlessFunctionDeclaration
			if regionIndex == 2 {
				correction = ""
			}
			response = semanticMergeResponse(correction)
		}
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(responseWriter, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, response)
	}))
	testInstance.Cleanup(llmServer.Close)

	configurationPath := writeReportedLifecycleSemanticConfiguration(testInstance, llmServer.URL)

	gitLogPath := filepath.Join(testInstance.TempDir(), "git.log")
	githubLogPath := filepath.Join(testInstance.TempDir(), "gh.log")
	require.NoError(
		testInstance,
		os.WriteFile(
			githubLogPath,
			[]byte("created-pr --base "+baseBranchName+" --head "+targetBranchName+"\n"),
			0o600,
		),
	)

	output, runError := runIntegrationCommandWithInput(
		testInstance,
		repositoryRoot,
		integrationCommandOptions{
			PathVariable: buildSyncMergedBranchExecutablePath(testInstance),
			EnvironmentOverrides: map[string]string{
				syncMergedBranchGitLogVariable:    gitLogPath,
				syncMergedBranchGitHubLogVariable: githubLogPath,
				syncMergedBranchNameVariable:      targetBranchName,
				syncMergedBranchMergedVariable:    "false",
			},
		},
		syncMergedBranchIntegrationTimeout,
		"",
		[]string{
			syncRefreshIntegrationRunCommand,
			syncRefreshIntegrationModulePath,
			"--config",
			configurationPath,
			syncRefreshIntegrationLogLevelFlag,
			syncRefreshIntegrationErrorLogLevel,
			"sync",
			targetBranchName,
			"--roots",
			repositoryPath,
		},
	)
	require.NoError(testInstance, runError, output)
	require.NotContains(testInstance, output, "semantic candidate attempt")
	require.Contains(testInstance, output, "semantic audit approved")
	require.NotContains(testInstance, output, "does not preserve OURS replacement intent")
	require.NotContains(testInstance, output, "AI_MERGE_ROLLBACK")
	require.Equal(testInstance, 2, strings.Count(output, "semantic audit approved"))
	require.Equal(testInstance, int64(4), requestCount.Load())
	require.Equal(testInstance, fixture.Theirs, readTextFile(testInstance, conflictedFilePath))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "status", "--porcelain")))

	firstReplacementAuditRequest := <-requestBodies
	secondReplacementAuditRequest := <-requestBodies
	firstDeletionAuditRequest := <-requestBodies
	secondDeletionAuditRequest := <-requestBodies
	require.Contains(testInstance, firstReplacementAuditRequest, "SchemaV5")
	require.Contains(testInstance, firstReplacementAuditRequest, "Versionless")
	require.Contains(testInstance, firstReplacementAuditRequest, "semantic fidelity auditor")
	require.Contains(testInstance, secondReplacementAuditRequest, "Versionless")
	require.Contains(testInstance, secondReplacementAuditRequest, "semantic fidelity auditor")
	require.Contains(testInstance, firstDeletionAuditRequest, "schemaVersion != 5")
	require.Contains(testInstance, firstDeletionAuditRequest, "Conflict region: 2 of 2")
	require.Contains(testInstance, firstDeletionAuditRequest, "semantic fidelity auditor")
	require.Contains(testInstance, secondDeletionAuditRequest, "semantic fidelity auditor")
	require.Contains(testInstance, secondDeletionAuditRequest, "LOCALLY VALIDATED CANDIDATE:\\n\\n")

	headWithParents := strings.Fields(runGit(testInstance, repositoryPath, "rev-list", "--parents", "-n", "1", "HEAD"))
	require.Len(testInstance, headWithParents, 3)
	require.Equal(testInstance, targetCommit, headWithParents[1])
	require.Equal(testInstance, incomingCommit, headWithParents[2])
	require.Equal(
		testInstance,
		strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD")),
		strings.TrimSpace(runGit(testInstance, remotePath, "rev-parse", "refs/heads/"+targetBranchName)),
	)
	gitLog := readTextFile(testInstance, gitLogPath)
	require.Contains(testInstance, gitLog, "commit --no-edit")
	require.Contains(testInstance, gitLog, "push origin "+targetBranchName)
	require.NotContains(testInstance, gitLog, "merge --abort")
}

func writeReportedLifecycleSemanticConfiguration(testInstance *testing.T, baseURL string) string {
	testInstance.Helper()
	configurationPath := filepath.Join(testInstance.TempDir(), "reported-lifecycle-semantic-config.yml")
	configurationContent := fmt.Sprintf(`common:
  log_level: error
  log_format: console
  require_clean: true
github:
  credential: test-github-key
llm:
  openai:
    priority: 1
    model: mock-model
    base_url: %q
    credential: test-key
  llm_proxy:
    priority: 2
    provider: mock-provider
    model: mock-model
    base_url: %q
    credential: test-proxy-key
  max_completion_tokens: 64
  effort: "high"
  timeout_seconds: 5
operations:
  - command: ["sync"]
    with:
      remote: origin
      require_clean: true
`, baseURL, baseURL)
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte(configurationContent), 0o600))
	return configurationPath
}

func semanticMergeResponse(content string) string {
	return mergeResolutionContentBeginForTest + "\n" +
		content +
		"\nGIX_MERGE_RESOLUTION_CONTENT_END"
}

const (
	mergeResolutionContentBeginForTest         = "GIX_MERGE_RESOLUTION_CONTENT_BEGIN"
	mergeConflictResolutionAttemptCountForTest = 4
)
