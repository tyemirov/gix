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

func TestSyncConflictFidelityContracts(t *testing.T) {
	repositoryRoot := integrationRepositoryRoot(t)
	binaryPath := buildIntegrationBinary(t, repositoryRoot)
	openIssue := readTextFile(t, filepath.Join(repositoryRoot, "tests/testdata/merge-fidelity/i002-open.txt"))
	closedIssue := readTextFile(t, filepath.Join(repositoryRoot, "tests/testdata/merge-fidelity/i002-closed.txt"))
	issuePrefix := "# ISSUES\n\n## Improvements\n\n"
	issueTail := "- [x] [I001] Stable issue.\n"
	otherIssue := "- [ ] [I003] Independent local request.\n\n"
	incomingIssue := "- [ ] [I004] Independent incoming request.\n\n"
	csvIssue := "- [ ] [I002] Add CSV export.\n  Export all orders.\n\n"
	ssoIssue := "- [ ] [I002] Add SSO login.\n  Authenticate company accounts.\n\n"
	localDependent := "- [ ] [I003] {I002} Validate CSV export.\n\n"
	incomingDependent := "- [ ] [I004] {I002} Validate SSO login.\n  Verify I002 with [I002].\n\n"
	ledgerBase := readTextFile(t, filepath.Join(repositoryRoot, "tests/testdata/merge-fidelity/ledger-adjacent-base.txt"))
	ledgerOurs := readTextFile(t, filepath.Join(repositoryRoot, "tests/testdata/merge-fidelity/ledger-adjacent-ours.txt"))
	ledgerTheirs := readTextFile(t, filepath.Join(repositoryRoot, "tests/testdata/merge-fidelity/ledger-adjacent-theirs.txt"))
	ledgerTail := "  Goal:\n  Preserve the I025 migration body outside the conflict.\n\n## Maintenance\n"
	ledgerExpected := issuePrefix + ledgerOurs[:strings.Index(ledgerOurs, "- [ ] [I025]")] + ledgerTheirs[strings.Index(ledgerTheirs, "- [ ] [I026]"):] + ledgerTail
	const selectOurs = "GIX_MERGE_SELECT_OURS"
	const selectTheirs = "GIX_MERGE_SELECT_THEIRS"
	cases := []struct {
		name, base, ours, theirs string
		oursAbsent, theirsAbsent bool
		responses                []string
		chooseExpected           bool
		expected                 string
		expectedAbsent, failure  bool
		providerFailure          bool
		dirty                    bool
		stash                    bool
		rejection                string
		archive                  string
	}{
		{name: "adjacent base issue keeps incoming dependency", base: issuePrefix + ledgerBase + ledgerTail, ours: issuePrefix + ledgerOurs + ledgerTail, theirs: issuePrefix + ledgerTheirs + ledgerTail, chooseExpected: true, expected: ledgerExpected},
		{name: "adjacent base issue keeps local dependency", base: issuePrefix + ledgerBase + ledgerTail, ours: issuePrefix + ledgerTheirs + ledgerTail, theirs: issuePrefix + ledgerOurs + ledgerTail, chooseExpected: true, expected: ledgerExpected},
		{name: "adjacent base issue stash", dirty: true, stash: true, base: issuePrefix + ledgerBase + ledgerTail, ours: issuePrefix + ledgerTheirs + ledgerTail, theirs: issuePrefix + ledgerOurs + ledgerTail, chooseExpected: true, expected: ledgerExpected},
		{name: "adjacent base issue rejects stale dependency", base: issuePrefix + ledgerBase + ledgerTail, ours: issuePrefix + ledgerOurs + ledgerTail, theirs: issuePrefix + ledgerTheirs + ledgerTail, responses: []string{mergePlanUnknownBlockForTest}, chooseExpected: true, expected: ledgerExpected, rejection: "unknown merge block"},
		{name: "adjacent base issue stash rejection restores edits", dirty: true, stash: true, base: issuePrefix + ledgerBase + ledgerTail, ours: issuePrefix + ledgerTheirs + ledgerTail, theirs: issuePrefix + ledgerOurs + ledgerTail, responses: []string{mergePlanUnknownBlockForTest, mergePlanUnknownBlockForTest, mergePlanUnknownBlockForTest, mergePlanUnknownBlockForTest}, failure: true, rejection: "unknown merge block"},

		{name: "collision preserves separate requests", base: issuePrefix + issueTail, ours: issuePrefix + csvIssue + issueTail, theirs: issuePrefix + ssoIssue + issueTail, chooseExpected: true, expected: issuePrefix + csvIssue + strings.ReplaceAll(ssoIssue, "I002", "I003") + issueTail},
		{name: "collision allocates multiple linked requests", base: issuePrefix + issueTail, ours: issuePrefix + csvIssue + localDependent + issueTail, theirs: issuePrefix + ssoIssue + strings.ReplaceAll(incomingDependent, "I004", "I003") + issueTail, chooseExpected: true, expected: issuePrefix + csvIssue + localDependent + strings.ReplaceAll(ssoIssue, "I002", "I004") + strings.ReplaceAll(strings.ReplaceAll(incomingDependent, "I004", "I005"), "I002", "I004") + issueTail},
		{name: "related issue metadata keeps identity", base: issuePrefix + issueTail, ours: issuePrefix + csvIssue + issueTail, theirs: issuePrefix + strings.ReplaceAll(csvIssue, "[I002] ", "[I002]  (P1) {I001} ") + issueTail, chooseExpected: true, expected: issuePrefix + strings.ReplaceAll(csvIssue, "[I002] ", "[I002]  (P1) {I001} ") + issueTail},
		{name: "collision preserves separate requests reversed", base: issuePrefix + issueTail, ours: issuePrefix + ssoIssue + issueTail, theirs: issuePrefix + csvIssue + issueTail, chooseExpected: true, expected: issuePrefix + ssoIssue + strings.ReplaceAll(csvIssue, "I002", "I003") + issueTail},
		{name: "collision dirty sync preserves both requests", dirty: true, base: issuePrefix + issueTail, ours: issuePrefix + csvIssue + issueTail, theirs: issuePrefix + ssoIssue + issueTail, chooseExpected: true, expected: issuePrefix + csvIssue + strings.ReplaceAll(ssoIssue, "I002", "I003") + issueTail},
		{name: "collision reserves whole tracker IDs", base: issuePrefix + issueTail + "- [x] [I009] Later record.\n", ours: issuePrefix + csvIssue + localDependent + issueTail + "- [x] [I009] Later record.\n", theirs: issuePrefix + ssoIssue + incomingDependent + issueTail + "- [x] [I009] Later record.\n", chooseExpected: true, expected: issuePrefix + csvIssue + localDependent + strings.ReplaceAll(ssoIssue+incomingDependent, "I002", "I010") + issueTail + "- [x] [I009] Later record.\n"},
		{name: "collision reserves archive IDs", archive: "# ARCHIVE\n\n- [x] [I020] Archived request.\n", base: issuePrefix + issueTail, ours: issuePrefix + csvIssue + issueTail, theirs: issuePrefix + ssoIssue + incomingDependent + issueTail, chooseExpected: true, expected: issuePrefix + csvIssue + strings.ReplaceAll(ssoIssue+incomingDependent, "I002", "I021") + issueTail},
		{name: "collision keeps recurring suffix", base: "# ISSUES\n\n## Maintenance\n\n- [x] [M003] Existing request.\n", ours: "# ISSUES\n\n## Maintenance\n\n- [ ] [M002R] Inspect CSV exports.\n\n- [x] [M003] Existing request.\n", theirs: "# ISSUES\n\n## Maintenance\n\n- [ ] [M002R] Inspect SSO logins.\n\n- [x] [M003] Existing request.\n", chooseExpected: true, expected: "# ISSUES\n\n## Maintenance\n\n- [ ] [M002R] Inspect CSV exports.\n\n- [ ] [M004R] Inspect SSO logins.\n\n- [x] [M003] Existing request.\n"},
		{name: "existing issue title changes keep identity", base: issuePrefix + "- [ ] [I002] Original title.\n\n" + issueTail, ours: issuePrefix + "- [ ] [I002] Local title.\n\n" + issueTail, theirs: issuePrefix + "- [ ] [I002] Incoming title.\n\n" + issueTail, chooseExpected: true, expected: issuePrefix + "- [ ] [I002] Local title.\n\n" + issueTail},
		{name: "compatible inline correction", base: "Run `alpha beta gamma`.\n", ours: "Run `alpha gamma`.\n", theirs: "Run `alpha beta gamma delta`.\n", responses: []string{semanticMergeResponse("Run `alpha gamma delta`.\n"), mergePlanApprovedForTest}, expected: "Run `alpha gamma delta`.\n"},
		{name: "compatible inline approval", base: "Run `alpha beta gamma`.\n", ours: "Run `alpha gamma`.\n", theirs: "Run `alpha beta gamma delta`.\n", responses: []string{semanticMergeResponse("Run `alpha gamma delta`.\n"), mergePlanApprovedForTest}, expected: "Run `alpha gamma delta`.\n"},
		{name: "inline correction preserves incoming deletion", base: "Run `alpha beta gamma`.\n", ours: "Run `alpha beta gamma delta`.\n", theirs: "Run `alpha gamma`.\n", responses: []string{semanticMergeResponse("Run `alpha gamma delta`.\n"), mergePlanApprovedForTest}, expected: "Run `alpha gamma delta`.\n"},
		{name: "inline correction cannot drop addition", base: "Run `alpha beta gamma`.\n", ours: "Run `alpha gamma`.\n", theirs: "Run `alpha beta gamma delta`.\n", responses: []string{semanticMergeResponse("Run `alpha gamma`.\n"), mergePlanRejectedForTest, semanticMergeResponse("Run `alpha gamma delta`.\n"), mergePlanApprovedForTest}, expected: "Run `alpha gamma delta`.\n", rejection: "fixture review rejected the semantic change"},
		{name: "inline correction cannot restore deletion", base: "Run `alpha beta gamma`.\n", ours: "Run `alpha gamma`.\n", theirs: "Run `alpha beta gamma delta`.\n", responses: []string{semanticMergeResponse("Run `alpha beta gamma delta`.\n"), mergePlanRejectedForTest, semanticMergeResponse("Run `alpha gamma delta`.\n"), mergePlanApprovedForTest}, expected: "Run `alpha gamma delta`.\n", rejection: "fixture review rejected the semantic change"},
		{name: "local deletion keeps incoming edit", base: "original\n", oursAbsent: true, theirs: "valuable incoming edit\n", responses: []string{selectTheirs}, expected: "valuable incoming edit\n"},
		{name: "incoming deletion keeps local edit", base: "original\n", ours: "valuable local edit\n", theirsAbsent: true, responses: []string{selectOurs}, expected: "valuable local edit\n"},
		{name: "explicit local deletion", base: "original\n", oursAbsent: true, theirs: "valuable incoming edit\n", responses: []string{selectOurs}, expectedAbsent: true},
		{name: "explicit incoming deletion", base: "original\n", ours: "valuable local edit\n", theirsAbsent: true, responses: []string{selectTheirs}, expectedAbsent: true},
		{name: "marker-free invalid choice rolls back", base: "original\n", oursAbsent: true, theirs: "valuable incoming edit\n", responses: []string{mergePlanApprovedForTest, mergePlanApprovedForTest, mergePlanApprovedForTest, mergePlanApprovedForTest}, failure: true, rejection: "requires an explicit OURS or THEIRS selection"},
		{name: "binary conflict rolls back", base: "base\x00bytes", ours: "ours\x00bytes", theirs: "theirs\x00bytes", failure: true, rejection: "binary conflict"},
		{name: "binary modify-delete rolls back", base: "base\x00bytes", oursAbsent: true, theirs: "theirs\x00bytes", failure: true, rejection: "binary conflict"},
		{name: "small deletion requires repair", base: "alpha beta gamma\n", ours: "alpha gamma\n", theirs: "alpha beta gamma delta\n", responses: []string{semanticMergeResponse("alpha beta gamma delta\n"), mergePlanRejectedForTest, semanticMergeResponse("alpha gamma delta\n"), mergePlanApprovedForTest}, expected: "alpha gamma delta\n", rejection: "fixture review rejected the semantic change"},
		{name: "incoming small deletion requires repair", base: "alpha beta gamma\n", ours: "alpha beta gamma delta\n", theirs: "alpha gamma\n", responses: []string{semanticMergeResponse("alpha beta gamma delta\n"), mergePlanRejectedForTest, semanticMergeResponse("alpha gamma delta\n"), mergePlanApprovedForTest}, expected: "alpha gamma delta\n", rejection: "fixture review rejected the semantic change"},
		{name: "invented operator requires repair", base: "before\nafter\n", ours: "before\nreturn enabled;\nafter\n", theirs: "before\nreturn !enabled;\nafter\n", responses: []string{semanticMergeResponse("return &&enabled;\n"), mergePlanRejectedForTest, semanticMergeResponse("return !enabled;\n"), mergePlanApprovedForTest}, expected: "before\nreturn !enabled;\nafter\n", rejection: "fixture review rejected the semantic change"},
		{name: "invented whitespace requires repair", base: "before\nafter\n", ours: "before\nreturn enabled;\nafter\n", theirs: "before\nreturn !enabled;\nafter\n", responses: []string{semanticMergeResponse("return  enabled;\n"), mergePlanRejectedForTest, semanticMergeResponse("return enabled;\n"), mergePlanApprovedForTest}, expected: "before\nreturn enabled;\nafter\n", rejection: "fixture review rejected the semantic change"},
		{name: "operator rejection rolls back", base: "before\nafter\n", ours: "before\nreturn enabled;\nafter\n", theirs: "before\nreturn !enabled;\nafter\n", responses: []string{semanticMergeResponse("return &&enabled;\n"), mergePlanRejectedForTest, semanticMergeResponse("return &&enabled;\n")}, failure: true, rejection: "repeated a rejected candidate"},
		{name: "related issue selection keeps local source", base: issuePrefix + issueTail, ours: issuePrefix + closedIssue + issueTail, theirs: issuePrefix + openIssue + issueTail, chooseExpected: true, expected: issuePrefix + closedIssue + issueTail},
		{name: "related issue selection rejects unknown choice", base: issuePrefix + issueTail, ours: issuePrefix + openIssue + issueTail, theirs: issuePrefix + closedIssue + issueTail, responses: []string{mergePlanUnknownBlockForTest}, chooseExpected: true, expected: issuePrefix + closedIssue + issueTail, rejection: "unknown merge block"},
		{name: "related issue selection rejects unknown identifier", base: issuePrefix + issueTail, ours: issuePrefix + openIssue + issueTail, theirs: issuePrefix + closedIssue + issueTail, responses: []string{mergePlanUnknownBlockForTest}, chooseExpected: true, expected: issuePrefix + closedIssue + issueTail, rejection: "unknown merge block"},
		{name: "related issue selection keeps source blank lines", base: issuePrefix + issueTail, ours: issuePrefix + openIssue + issueTail, theirs: issuePrefix + closedIssue + "\n\n" + issueTail, chooseExpected: true, expected: issuePrefix + closedIssue + "\n\n" + issueTail},
		{name: "dirty social I002 sync", dirty: true, base: issuePrefix + issueTail, ours: issuePrefix + openIssue + issueTail, theirs: issuePrefix + closedIssue + issueTail, chooseExpected: true, expected: issuePrefix + closedIssue + issueTail},
		{name: "dirty issue selection rejection restores edits", dirty: true, base: issuePrefix + issueTail, ours: issuePrefix + openIssue + issueTail, theirs: issuePrefix + closedIssue + issueTail, responses: []string{mergePlanUnknownBlockForTest, mergePlanUnknownBlockForTest, mergePlanUnknownBlockForTest, mergePlanUnknownBlockForTest}, failure: true, rejection: "unknown merge block"},
		{name: "issue selection duplicate displaces independent record", base: issuePrefix + issueTail, ours: issuePrefix + openIssue + otherIssue + issueTail, theirs: issuePrefix + closedIssue + incomingIssue + issueTail, responses: []string{mergePlanUnknownBlockForTest}, chooseExpected: true, expected: issuePrefix + closedIssue + otherIssue + incomingIssue + issueTail, rejection: "unknown merge block"},
		{name: "exact social I002 related insertions", base: issuePrefix + issueTail, ours: issuePrefix + openIssue + issueTail, theirs: issuePrefix + closedIssue + issueTail, chooseExpected: true, expected: issuePrefix + closedIssue + issueTail},
		{name: "related records preserve independent additions", base: issuePrefix + issueTail, ours: issuePrefix + openIssue + otherIssue + issueTail, theirs: issuePrefix + closedIssue + incomingIssue + issueTail, responses: []string{mergePlanUnknownBlockForTest}, chooseExpected: true, expected: issuePrefix + closedIssue + otherIssue + incomingIssue + issueTail, rejection: "unknown merge block"},
		{name: "duplicate related record requires repair", base: issuePrefix + issueTail, ours: issuePrefix + openIssue + issueTail, theirs: issuePrefix + closedIssue + issueTail, responses: []string{mergePlanUnknownBlockForTest}, chooseExpected: true, expected: issuePrefix + closedIssue + issueTail, rejection: "unknown merge block"},

		{name: "empty file is distinct from deletion", base: "original\n", oursAbsent: true, theirs: "", responses: []string{selectTheirs}, expected: ""},
		{name: "file selection provider failure rolls back", base: "original\n", oursAbsent: true, theirs: "valuable incoming edit\n", providerFailure: true, responses: []string{"", ""}, failure: true, rejection: "stopping semantic repair"},
		{name: "concatenation cannot restore a deletion", base: "alpha beta gamma\n", ours: "alpha gamma\n", theirs: "alpha beta gamma delta\n", responses: []string{semanticMergeResponse("alpha gamma\nalpha beta gamma delta\n"), mergePlanRejectedForTest, semanticMergeResponse("alpha gamma delta\n"), mergePlanApprovedForTest}, expected: "alpha gamma delta\n", rejection: "fixture review rejected the semantic change"},
		{name: "deletion rejection rolls back", base: "alpha beta gamma\n", ours: "alpha gamma\n", theirs: "alpha beta gamma delta\n", responses: []string{semanticMergeResponse("alpha beta gamma delta\n"), mergePlanRejectedForTest, semanticMergeResponse("alpha beta gamma delta\n")}, failure: true, rejection: "repeated a rejected candidate"},
		{name: "invented issue requirement rolls back", base: issuePrefix + issueTail, ours: issuePrefix + openIssue + issueTail, theirs: issuePrefix + closedIssue + issueTail, responses: []string{semanticMergeResponse(closedIssue + "  Invented requirement.\n"), mergePlanRejectedForTest, semanticMergeResponse(closedIssue + "  Invented requirement.\n")}, failure: true, rejection: "repeated a rejected candidate"},
		{name: "independent insertions retain both sides", base: "before\nafter\n", ours: "before\nlocal distinct entry\nafter\n", theirs: "before\nincoming separate entry\nafter\n", responses: []string{semanticMergeResponse("local distinct entry\n"), mergePlanRejectedForTest, semanticMergeResponse("local distinct entry\nincoming separate entry\n"), mergePlanApprovedForTest}, expected: "before\nlocal distinct entry\nincoming separate entry\nafter\n", rejection: "fixture review rejected the semantic change"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workspace := syncHomeWorkspace(t)
			remote := filepath.Join(workspace, "remote.git")
			repository := filepath.Join(workspace, "project")
			createSyncGitHubBackedRepository(t, remote, repository)
			relativePath := "content.txt"
			if strings.HasPrefix(tc.base, "# ISSUES") {
				relativePath = ".mprlab/ISSUES.md"
			}
			path := filepath.Join(repository, relativePath)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(tc.base), 0o644))
			if tc.archive != "" {
				require.NoError(t, os.WriteFile(filepath.Join(repository, ".mprlab", "ARCHIVE.md"), []byte(tc.archive), 0o644))
			}
			runGit(t, repository, "add", ".")
			runGit(t, repository, "commit", "-m", "base")
			runGit(t, repository, "push", "-u", "origin", "master")
			runGit(t, repository, "switch", "-c", "incoming")
			if tc.theirsAbsent {
				require.NoError(t, os.Remove(path))
			} else {
				require.NoError(t, os.WriteFile(path, []byte(tc.theirs), 0o644))
			}
			runGit(t, repository, "add", "-A")
			runGit(t, repository, "commit", "-m", "incoming")
			runGit(t, repository, "push", "origin", "HEAD:master")
			incomingCommit := strings.TrimSpace(runGit(t, remote, "rev-parse", "master"))
			runGit(t, repository, "switch", "master")
			if tc.oursAbsent {
				require.NoError(t, os.Remove(path))
			} else {
				require.NoError(t, os.WriteFile(path, []byte(tc.ours), 0o644))
			}
			if !tc.dirty {
				runGit(t, repository, "add", "-A")
				runGit(t, repository, "commit", "-m", "local")
			}
			initialStatus := runGit(t, repository, "status", "--porcelain")
			localCommit := strings.TrimSpace(runGit(t, repository, "rev-parse", "HEAD"))
			var requestCount atomic.Int64
			requestBodies := make(chan string, 8)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				if tc.dirty && !strings.Contains(string(body), "Conflict region:") {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"docs: record health requirement"}}]}`)
					return
				}
				requestBodies <- string(body)
				requestIndex := int(requestCount.Add(1)) - 1
				if tc.providerFailure {
					http.Error(w, "provider unavailable", http.StatusForbidden)
					return
				}
				if requestIndex >= len(tc.responses) && !tc.chooseExpected {
					http.Error(w, "unexpected model request", 500)
					return
				}
				command := exec.Command("git", "-C", remote, "rev-parse", "master")
				command.Env = buildGitCommandEnvironment(nil)
				output, err := command.CombinedOutput()
				if err != nil || strings.TrimSpace(string(output)) != incomingCommit {
					http.Error(w, "remote changed before resolution", 500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				response := ""
				if requestIndex < len(tc.responses) {
					response = tc.responses[requestIndex]
				} else {
					var responseErr error
					response, responseErr = mergePlanSourceFixtureResponse(body, tc.expected)
					if responseErr != nil {
						http.Error(w, responseErr.Error(), 500)
						return
					}
				}
				_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, response)
			}))
			t.Cleanup(server.Close)
			configuration := writeReportedLifecycleSemanticConfiguration(t, server.URL)
			if tc.dirty {
				configuration = writeDirtySyncMergedBranchConfiguration(t, server.URL)
			}
			gitLog := filepath.Join(t.TempDir(), "git.log")
			ghLog := filepath.Join(t.TempDir(), "gh.log")
			arguments := []string{"--config", configuration, "--roots", repository, "sync", "master"}
			if tc.stash {
				arguments = append(arguments, "--stash")
			}
			output, err := runBinaryIntegrationCommand(t, binaryPath, repository, map[string]string{
				pathEnvironmentVariableNameConstant: buildSyncMergedBranchExecutablePath(t),
				syncMergedBranchGitLogVariable:      gitLog,
				syncMergedBranchGitHubLogVariable:   ghLog,
				syncMergedBranchNameVariable:        "master",
				syncMergedBranchMergedVariable:      "false",
			}, syncStateTransitionTimeout, arguments)
			if tc.chooseExpected {
				require.GreaterOrEqual(t, requestCount.Load(), int64(len(tc.responses)+1), output)
				require.LessOrEqual(t, requestCount.Load(), int64(len(tc.responses)+2), output)
			} else {
				require.Equal(t, int64(len(tc.responses)), requestCount.Load(), output)
			}
			if tc.rejection != "" {
				require.Contains(t, output, tc.rejection)
			}
			if tc.failure {
				require.Error(t, err, output)
				if tc.stash {
					require.Contains(t, output, "SYNC_SWITCH_ROLLBACK")
				} else {
					require.Contains(t, output, "AI_MERGE_ROLLBACK")
				}
				require.Equal(t, localCommit, strings.TrimSpace(runGit(t, repository, "rev-parse", "HEAD")))
				require.Equal(t, incomingCommit, strings.TrimSpace(runGit(t, remote, "rev-parse", "master")))
				if tc.oursAbsent {
					require.NoFileExists(t, path)
				} else {
					require.Equal(t, tc.ours, readTextFile(t, path))
				}
			} else {
				require.NoError(t, err, output)
				require.Contains(t, output, "merge conflict resolution completed")
				if tc.expectedAbsent {
					require.NoFileExists(t, path)
				} else {
					require.Equal(t, tc.expected, readTextFile(t, path))
					published := tc.expected
					if tc.stash {
						published = tc.theirs
					}
					require.Equal(t, published, runGit(t, remote, "show", "master:"+relativePath))
				}
				require.Equal(t, strings.TrimSpace(runGit(t, repository, "rev-parse", "HEAD")), strings.TrimSpace(runGit(t, remote, "rev-parse", "master")))
			}
			if tc.failure {
				require.Equal(t, initialStatus, runGit(t, repository, "status", "--porcelain"))
			} else if tc.stash {
				require.Equal(t, "M  "+relativePath+"\n", runGit(t, repository, "status", "--porcelain"))
				require.Equal(t, tc.expected, runGit(t, repository, "show", ":"+relativePath))
			} else {
				require.Empty(t, strings.TrimSpace(runGit(t, repository, "status", "--porcelain")))
			}
			command := exec.Command("git", "-C", repository, "rev-parse", "--verify", "MERGE_HEAD")
			command.Env = buildGitCommandEnvironment(nil)
			require.Error(t, command.Run())
			if (tc.oursAbsent || tc.theirsAbsent) && len(tc.responses) > 0 {
				require.Contains(t, <-requestBodies, "file absent in this stage")
			}
		})
	}
}

func TestSyncRenameConflictRequiresGitResolution(t *testing.T) {
	root := integrationRepositoryRoot(t)
	binary := buildIntegrationBinary(t, root)
	for _, originalPath := range []string{"original.txt", "aaa-original.txt"} {
		t.Run(originalPath, func(t *testing.T) {
			workspace := syncHomeWorkspace(t)
			remote := filepath.Join(workspace, "remote.git")
			repository := filepath.Join(workspace, "project")
			createSyncGitHubBackedRepository(t, remote, repository)
			require.NoError(t, os.WriteFile(filepath.Join(repository, originalPath), []byte("unique original content\n"), 0o644))
			runGit(t, repository, "add", ".")
			runGit(t, repository, "commit", "-m", "base")
			runGit(t, repository, "push", "-u", "origin", "master")
			runGit(t, repository, "switch", "-c", "incoming")
			runGit(t, repository, "mv", originalPath, "incoming.txt")
			runGit(t, repository, "commit", "-m", "rename incoming")
			runGit(t, repository, "push", "origin", "HEAD:master")
			incomingCommit := strings.TrimSpace(runGit(t, remote, "rev-parse", "master"))
			runGit(t, repository, "switch", "master")
			runGit(t, repository, "mv", originalPath, "local.txt")
			runGit(t, repository, "commit", "-m", "rename local")
			localCommit := strings.TrimSpace(runGit(t, repository, "rev-parse", "HEAD"))
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"GIX_MERGE_SELECT_OURS"}}]}`)
			}))
			defer server.Close()
			configuration := writeReportedLifecycleSemanticConfiguration(t, server.URL)
			output, err := runBinaryIntegrationCommand(t, binary, repository, map[string]string{
				pathEnvironmentVariableNameConstant: buildSyncMergedBranchExecutablePath(t),
				syncMergedBranchGitLogVariable:      filepath.Join(t.TempDir(), "git.log"),
				syncMergedBranchGitHubLogVariable:   filepath.Join(t.TempDir(), "gh.log"),
				syncMergedBranchNameVariable:        "master",
				syncMergedBranchMergedVariable:      "false",
			}, syncStateTransitionTimeout, []string{"--config", configuration, "--roots", repository, "sync", "master"})
			require.Error(t, err, output)
			require.Contains(t, output, "structural conflict")
			require.Contains(t, output, "AI_MERGE_ROLLBACK")
			require.Zero(t, requests.Load())
			require.Equal(t, localCommit, strings.TrimSpace(runGit(t, repository, "rev-parse", "HEAD")))
			require.Equal(t, incomingCommit, strings.TrimSpace(runGit(t, remote, "rev-parse", "master")))
			require.Equal(t, "unique original content\n", readTextFile(t, filepath.Join(repository, "local.txt")))
			require.NoFileExists(t, filepath.Join(repository, "incoming.txt"))
			require.NoFileExists(t, filepath.Join(repository, originalPath))
			require.Empty(t, strings.TrimSpace(runGit(t, repository, "status", "--porcelain")))
			command := exec.Command("git", "-C", repository, "rev-parse", "--verify", "MERGE_HEAD")
			command.Env = buildGitCommandEnvironment(nil)
			require.Error(t, command.Run())
		})
	}
}
