package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mergeCorpusCase struct {
	Evaluate            bool     `json:"evaluate"`
	Name                string   `json:"name"`
	Path                string   `json:"path"`
	Base                string   `json:"base"`
	Ours                string   `json:"ours"`
	Theirs              string   `json:"theirs"`
	Expected            string   `json:"expected"`
	Action              string   `json:"action"`
	Combined            string   `json:"combined"`
	RejectedCombination string   `json:"rejected_combination"`
	Failure             string   `json:"failure"`
	Responses           []string `json:"responses"`
	Rejection           string   `json:"rejection"`
	ContextRecord       string   `json:"context_record"`
	ReviewWindows       []string `json:"review_windows"`
	ReviewRejection     string   `json:"review_rejection"`
	Stash               bool     `json:"stash"`
}

func TestSyncResolutionPlanCorpus(t *testing.T) {
	cases := loadMergeCorpus(t)
	padding := strings.Repeat("Historical context.\n", 2000)
	recordStart := "- [ ] [I001] Read the complete record.\n" + strings.Repeat("  Earlier requirement.\n", 80)
	recordEnd := strings.Repeat("  Later requirement.\n", 80)
	ours := recordStart + "  Selected policy.\n" + recordEnd
	cases = append(cases, mergeCorpusCase{Name: "expand_complete_record_context", Path: "ISSUES.md", Base: padding + recordStart + "  Base policy.\n" + recordEnd, Ours: padding + ours, Theirs: padding + recordStart + "  Incoming policy.\n" + recordEnd, Expected: padding + ours, Action: "ours", ContextRecord: ours})
	runMergeCorpus(t, cases, "")
}

func TestSyncResolutionPlanProviderEvaluation(t *testing.T) {
	configuration := os.Getenv("GIX_MERGE_EVAL_CONFIG")
	if configuration == "" {
		t.Skip("set GIX_MERGE_EVAL_CONFIG to qualify a live model")
	}
	absolute, err := filepath.Abs(configuration)
	require.NoError(t, err)
	cases := []mergeCorpusCase{}
	for _, record := range loadMergeCorpus(t) {
		if record.Evaluate {
			cases = append(cases, record)
		}
	}
	runMergeCorpus(t, cases, absolute)
}

func loadMergeCorpus(t *testing.T) []mergeCorpusCase {
	var cases []mergeCorpusCase
	require.NoError(t, json.Unmarshal([]byte(readTextFile(t, filepath.Join(integrationRepositoryRoot(t), "tests/testdata/merge-contract/corpus.json"))), &cases))
	return cases
}

func runMergeCorpus(t *testing.T, cases []mergeCorpusCase, providerConfiguration string) {
	binary := buildIntegrationBinary(t, integrationRepositoryRoot(t))
	for _, record := range cases {
		t.Run(record.Name, func(t *testing.T) {
			workspace := syncHomeWorkspace(t)
			remote := filepath.Join(workspace, "remote.git")
			repository := filepath.Join(workspace, "project")
			createSyncGitHubBackedRepository(t, remote, repository)
			path := filepath.Join(repository, record.Path)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
			require.NoError(t, os.WriteFile(path, []byte(record.Base), 0o644))
			runGit(t, repository, "add", ".")
			runGit(t, repository, "commit", "-m", "base")
			runGit(t, repository, "push", "-u", "origin", "master")
			runGit(t, repository, "switch", "-c", "incoming")
			require.NoError(t, os.WriteFile(path, []byte(record.Theirs), 0o644))
			runGit(t, repository, "commit", "-am", "incoming")
			runGit(t, repository, "push", "origin", "HEAD:master")
			runGit(t, repository, "switch", "master")
			require.NoError(t, os.WriteFile(path, []byte(record.Ours), 0o644))
			if !record.Stash {
				runGit(t, repository, "commit", "-am", "local")
			}
			originalHead := runGit(t, repository, "rev-parse", "HEAD")
			originalStatus := runGit(t, repository, "status", "--porcelain")
			originalStashes := runGit(t, repository, "stash", "list")
			var proposals, reviews, requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestIndex := int(requests.Add(1)) - 1
				if requestIndex < len(record.Responses) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, record.Responses[requestIndex])
					return
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				var chat struct {
					Messages []struct {
						Content string `json:"content"`
					} `json:"messages"`
				}
				if err := json.Unmarshal(body, &chat); err != nil || len(chat.Messages) == 0 {
					http.Error(w, "invalid chat", 400)
					return
				}
				_, raw, ok := strings.Cut(chat.Messages[len(chat.Messages)-1].Content, "GIX_MERGE_INPUT\n")
				if !ok {
					http.Error(w, "missing explicit merge plan contract", 400)
					return
				}
				var input struct {
					Phase  string `json:"phase"`
					Blocks []struct {
						ID string `json:"id"`
					} `json:"blocks"`
					Candidate *struct {
						Content string `json:"content"`
					} `json:"candidate"`
					Context   struct{ Base, Ours, Theirs string } `json:"context"`
					Placement struct{ Before, After string }      `json:"placement"`
				}
				if err := json.Unmarshal([]byte(raw), &input); err != nil {
					http.Error(w, err.Error(), 400)
					return
				}
				if record.ContextRecord != "" && requestIndex == 0 {
					if !strings.Contains(input.Context.Ours, record.ContextRecord) {
						http.Error(w, "incomplete issue read context", 400)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, `{"status":"needs_context","reason":"Read the historical context."}`)
					return
				}
				expectedOurs, expectedTheirs := record.Ours, record.Theirs
				if record.Stash {
					expectedOurs, expectedTheirs = expectedTheirs, expectedOurs
				}
				if input.Context.Ours != expectedOurs || input.Context.Theirs != expectedTheirs || input.Context.Base != record.Base {
					http.Error(w, "missing complete source context", 400)
					return
				}
				response := map[string]any{}
				if input.Phase == "review" {
					reviewIndex := int(reviews.Add(1)) - 1
					if reviewIndex < len(record.ReviewWindows) && (input.Candidate == nil || input.Placement.Before+input.Candidate.Content+input.Placement.After != record.ReviewWindows[reviewIndex]) {
						response = map[string]any{"status": "rejected", "reason": "The candidate lacks its exact destination context, including the success response and closing brace."}
					} else if record.ReviewRejection != "" {
						response = map[string]any{"status": "rejected", "reason": record.ReviewRejection}
					} else if record.RejectedCombination != "" && proposals.Load() == 1 {
						response = map[string]any{"status": "rejected", "reason": "The audit and notification calls must remain inside the enabled condition."}
					} else {
						response["status"] = "approved"
					}
				} else {
					attempt := proposals.Add(1)
					if len(input.Blocks) != 1 {
						http.Error(w, fmt.Sprintf("expected one overlap, got %d", len(input.Blocks)), 400)
						return
					}
					decision := map[string]any{"id": input.Blocks[0].ID, "action": record.Action, "reason": "Preserve the selected contract and all independent changes."}
					if record.Action == "combine" {
						content := record.Combined
						if attempt == 1 && record.RejectedCombination != "" {
							content = record.RejectedCombination
						}
						decision["content"] = content
					}
					response = map[string]any{"status": "resolved", "decisions": []any{decision}}
				}
				encoded, err := json.Marshal(response)
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"role":"assistant","content":%q}}]}`, string(encoded))
			}))
			t.Cleanup(server.Close)
			configuration := writeReportedLifecycleSemanticConfiguration(t, server.URL)
			if providerConfiguration != "" {
				configuration = providerConfiguration
			}
			executionTimeout := syncStateTransitionTimeout
			if providerConfiguration != "" {
				executionTimeout = 15 * time.Minute
			}
			arguments := []string{"--config", configuration, "--roots", repository, "sync", "master"}
			if record.Stash {
				arguments = append(arguments, "--stash")
			}
			output, err := runBinaryIntegrationCommand(t, binary, repository, map[string]string{
				pathEnvironmentVariableNameConstant: buildSyncMergedBranchExecutablePath(t),
				syncMergedBranchGitLogVariable:      filepath.Join(t.TempDir(), "git.log"),
				syncMergedBranchGitHubLogVariable:   filepath.Join(t.TempDir(), "gh.log"),
				syncMergedBranchNameVariable:        "master",
				syncMergedBranchMergedVariable:      "false",
			}, executionTimeout, arguments)
			require.Equal(t, originalStashes, runGit(t, repository, "stash", "list"))
			if providerConfiguration == "" && len(record.ReviewWindows) != 0 {
				require.EqualValues(t, len(record.ReviewWindows), reviews.Load(), output)
			}
			if record.Rejection != "" {
				require.Contains(t, output, record.Rejection)
			}
			if record.Failure != "" {
				require.Error(t, err, output)
				require.Contains(t, output, record.Failure)
				require.Equal(t, originalHead, runGit(t, repository, "rev-parse", "HEAD"))
				require.Equal(t, record.Ours, readTextFile(t, path))
				require.Equal(t, record.Theirs, runGit(t, remote, "show", "master:"+record.Path))
				require.Equal(t, originalStatus, runGit(t, repository, "status", "--porcelain"))
				return
			}
			require.NoError(t, err, output)
			require.Equal(t, record.Expected, readTextFile(t, path))
			if record.Stash {
				require.Equal(t, record.Theirs, runGit(t, remote, "show", "master:"+record.Path))
				require.Equal(t, record.Theirs, runGit(t, repository, "show", "HEAD:"+record.Path))
				require.Equal(t, "M  "+record.Path+"\n", runGit(t, repository, "status", "--porcelain"))
				require.Equal(t, record.Expected, runGit(t, repository, "show", ":"+record.Path))
			} else {
				require.Equal(t, record.Expected, runGit(t, remote, "show", "master:"+record.Path))
				require.Empty(t, runGit(t, repository, "status", "--porcelain"))
			}
			if providerConfiguration == "" {
				if record.ContextRecord != "" {
					require.Contains(t, output, "expanded semantic read context to complete source files")
				}
				require.Positive(t, reviews.Load(), output)
			}
		})
	}
}
