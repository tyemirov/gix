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

	"github.com/stretchr/testify/require"
)

func TestSyncReadsRelatedMergeEvidence(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"resolve", "review", "expanded_review"} {
		t.Run(phase, func(t *testing.T) {
			fixture := newRelatedMergeFixture(t)
			var requests, contextRequests atomic.Int64
			fixture.run(t, func(body []byte, input mergePlanInputForTest) (string, error) {
				requests.Add(1)
				if input.Path == relatedPolicyPath {
					return mergePlanSourceFixtureResponse(body, fixture.ours[relatedPolicyPath])
				}
				if strings.Contains(string(body), relatedPrivateSentinel) {
					return "", fmt.Errorf("untracked input reached the model")
				}
				if input.Phase == phase || (phase == "expanded_review" && input.Phase == "review") {
					if phase == "expanded_review" && contextRequests.Load() == 0 {
						contextRequests.Add(1)
						return `{"status":"needs_context","reason":"Read the complete test source."}`, nil
					}
					if len(input.RelatedFiles) == 0 {
						contextRequests.Add(1)
						return `{"status":"needs_context","reason":"Read the policy merge and the schema before selecting the field.","paths":["a-policy.txt","schema.py"]}`, nil
					}
				}
				if contextRequests.Load() > 0 {
					if len(input.RelatedFiles) != 2 {
						return "", fmt.Errorf("missing requested repository evidence")
					}
					policy, schema := input.RelatedFiles[0], input.RelatedFiles[1]
					if policy.Path != relatedPolicyPath || !policy.Resolved || policy.Deleted || len(policy.Versions) != 4 {
						return "", fmt.Errorf("missing policy sources or prior merge result: %+v", policy)
					}
					for index, source := range []string{"base", "ours", "theirs", "index"} {
						expected := []string{fixture.base[relatedPolicyPath], fixture.ours[relatedPolicyPath], fixture.theirs[relatedPolicyPath], fixture.ours[relatedPolicyPath]}[index]
						if policy.Versions[index].Source != source || policy.Versions[index].Content != expected {
							return "", fmt.Errorf("incorrect %s policy evidence", source)
						}
					}
					if schema.Path != relatedSchemaPath || schema.Resolved || len(schema.Versions) != 1 || schema.Versions[0].Source != "index" || schema.Versions[0].Content != fixture.theirs[relatedSchemaPath] {
						return "", fmt.Errorf("incorrect schema evidence: %+v", schema)
					}
				}
				return mergePlanSourceFixtureResponse(body, fixture.ours[relatedTestPath])
			}, "")
			expectedRequests := int64(5)
			if phase == "expanded_review" {
				expectedRequests++
			}
			require.Equal(t, expectedRequests, requests.Load())
			require.Positive(t, contextRequests.Load())
		})
	}
}

func TestSyncRejectsUnavailableRelatedMergeEvidence(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name    string
		paths   []string
		files   map[string]string
		failure string
	}{
		{name: "parent_path", paths: []string{"../outside"}, failure: "invalid related repository path"},
		{name: "absolute_path", paths: []string{filepath.Join(t.TempDir(), "outside")}, failure: "invalid related repository path"},
		{name: "unclean_path", paths: []string{"dir/../schema.py"}, failure: "invalid related repository path"},
		{name: "current_file", paths: []string{relatedTestPath}, failure: "invalid related repository path"},
		{name: "untracked_private_file", paths: []string{".env"}, failure: "path is not a tracked file"},
		{name: "untracked_file", paths: []string{"untracked.txt"}, failure: "path is not a tracked file"},
		{name: "missing_file", paths: []string{"missing.txt"}, failure: "path is not a tracked file"},
		{name: "literal_pattern", paths: []string{"schema.*"}, failure: "path is not a tracked file"},
		{name: "directory", paths: []string{"nested"}, files: map[string]string{"nested/source.txt": "tracked source\n"}, failure: "path must select one exact tracked file"},
		{name: "binary", paths: []string{"binary.dat"}, files: map[string]string{"binary.dat": "binary\x00data"}, failure: "related Git blob is not UTF-8 text"},
		{name: "invalid_utf8", paths: []string{"invalid.txt"}, files: map[string]string{"invalid.txt": "text\xff"}, failure: "related Git blob is not UTF-8 text"},
		{name: "oversized_file", paths: []string{"large.txt"}, files: map[string]string{"large.txt": strings.Repeat("x", 256*1024+1)}, failure: "related context exceeds 262144 content bytes"},
		{name: "oversized_original_source", paths: []string{relatedPolicyPath}, files: map[string]string{relatedPolicyPath: strings.Repeat("p", 256*1024+1) + "\n"}, failure: "related context exceeds 262144 content bytes"},
		{name: "combined_byte_limit", paths: []string{"large-a.txt", "large-b.txt"}, files: map[string]string{"large-a.txt": strings.Repeat("a", 140*1024), "large-b.txt": strings.Repeat("b", 140*1024)}, failure: "related context exceeds 262144 content bytes"},
		{name: "file_limit", paths: []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}, failure: "related context exceeds 8 files"},
		{name: "duplicate_path", paths: []string{relatedSchemaPath, relatedSchemaPath}, failure: "duplicate related repository path"},
		{name: "repeated_request", paths: []string{relatedSchemaPath}, failure: "requested related files were already supplied"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newRelatedMergeFixture(t, scenario.files)
			require.NoError(t, os.WriteFile(filepath.Join(fixture.repository, "untracked.txt"), []byte(relatedPrivateSentinel), 0o600))
			var contextRequests atomic.Int64
			fixture.run(t, func(body []byte, input mergePlanInputForTest) (string, error) {
				if input.Path == relatedPolicyPath {
					return mergePlanSourceFixtureResponse(body, fixture.ours[relatedPolicyPath])
				}
				if strings.Contains(string(body), relatedPrivateSentinel) {
					return "", fmt.Errorf("untracked input reached the model")
				}
				contextRequests.Add(1)
				response, err := json.Marshal(map[string]any{"status": "needs_context", "reason": "Read the exact source evidence.", "paths": scenario.paths})
				return string(response), err
			}, scenario.failure)
			expectedRequests := int64(1)
			if scenario.name == "repeated_request" {
				expectedRequests++
			}
			require.Equal(t, expectedRequests, contextRequests.Load())
			require.Equal(t, relatedPrivateSentinel, readTextFile(t, filepath.Join(fixture.repository, "untracked.txt")))
		})
	}
}

func TestSyncKeepsRelatedMergeDisagreementUnresolved(t *testing.T) {
	t.Parallel()
	fixture := newRelatedMergeFixture(t, map[string]string{relatedSchemaPath: "accepted_field = 'environment_file'\n"})
	var reviews atomic.Int64
	fixture.run(t, func(body []byte, input mergePlanInputForTest) (string, error) {
		if input.Path == relatedPolicyPath {
			return mergePlanSourceFixtureResponse(body, fixture.ours[relatedPolicyPath])
		}
		if input.Phase == "review" {
			reviews.Add(1)
			if len(input.RelatedFiles) == 0 {
				return `{"status":"needs_context","reason":"Check the earlier policy result against the schema.","paths":["a-policy.txt","schema.py"]}`, nil
			}
			if len(input.RelatedFiles) != 2 || !input.RelatedFiles[0].Resolved || len(input.RelatedFiles[0].Versions) != 4 ||
				input.RelatedFiles[0].Versions[3].Content != fixture.ours[relatedPolicyPath] ||
				len(input.RelatedFiles[1].Versions) != 1 || input.RelatedFiles[1].Versions[0].Content != fixture.theirs[relatedSchemaPath] {
				return "", fmt.Errorf("missing conflicting policy and schema evidence")
			}
			return `{"status":"unresolved","reason":"The earlier policy result contradicts the schema; correction exceeds this conflict's edit scope."}`, nil
		}
		return mergePlanSourceFixtureResponse(body, fixture.ours[relatedTestPath])
	}, "earlier policy result contradicts the schema")
	require.EqualValues(t, 2, reviews.Load())
}

func TestSyncBoundsSuccessiveRelatedContextRequests(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name    string
		batches [][]string
		files   map[string]string
		failure string
	}{
		{name: "file_limit", batches: [][]string{{"1", "2", "3", "4", "5", "6", "7", "8"}, {"9"}}, files: map[string]string{"1": "one", "2": "two", "3": "three", "4": "four", "5": "five", "6": "six", "7": "seven", "8": "eight", "9": "nine"}, failure: "related context exceeds 8 files"},
		{name: "byte_limit", batches: [][]string{{"large-a.txt"}, {"large-b.txt"}}, files: map[string]string{"large-a.txt": strings.Repeat("a", 140*1024), "large-b.txt": strings.Repeat("b", 140*1024)}, failure: "related context exceeds 262144 content bytes"},
		{name: "request_budget", batches: [][]string{{"1"}, {"2"}, {"3"}, {"4"}}, files: map[string]string{"1": "one", "2": "two", "3": "three", "4": "four"}, failure: "context requests exhausted the semantic review budget"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newRelatedMergeFixture(t, scenario.files)
			var requests atomic.Int64
			fixture.run(t, func(body []byte, input mergePlanInputForTest) (string, error) {
				if input.Path == relatedPolicyPath {
					return mergePlanSourceFixtureResponse(body, fixture.ours[relatedPolicyPath])
				}
				index := int(requests.Add(1)) - 1
				if index >= len(scenario.batches) {
					return "", fmt.Errorf("context request budget was bypassed")
				}
				if len(input.RelatedFiles) != index*len(scenario.batches[0]) {
					return "", fmt.Errorf("earlier related evidence was not retained")
				}
				response, err := json.Marshal(map[string]any{"status": "needs_context", "reason": "Read additional source evidence.", "paths": scenario.batches[index]})
				return string(response), err
			}, scenario.failure)
			require.EqualValues(t, len(scenario.batches), requests.Load())
		})
	}
}

func TestSyncReadsUnresolvedRelatedStages(t *testing.T) {
	t.Parallel()
	fixture := newRelatedMergeFixture(t)
	var expanded atomic.Bool
	fixture.run(t, func(body []byte, input mergePlanInputForTest) (string, error) {
		if input.Path == relatedPolicyPath {
			if !expanded.Load() {
				expanded.Store(true)
				return `{"status":"needs_context","reason":"Read the concurrent test changes.","paths":["z-tests.txt"]}`, nil
			}
			if len(input.RelatedFiles) != 1 || input.RelatedFiles[0].Resolved || len(input.RelatedFiles[0].Versions) != 3 {
				return "", fmt.Errorf("missing unresolved source stages")
			}
			for index, source := range []string{"base", "ours", "theirs"} {
				version := input.RelatedFiles[0].Versions[index]
				expected := []string{fixture.base[relatedTestPath], fixture.ours[relatedTestPath], fixture.theirs[relatedTestPath]}[index]
				if version.Source != source || version.Content != expected {
					return "", fmt.Errorf("incorrect unresolved %s source", source)
				}
			}
			return mergePlanSourceFixtureResponse(body, fixture.ours[relatedPolicyPath])
		}
		return mergePlanSourceFixtureResponse(body, fixture.ours[relatedTestPath])
	}, "")
	require.True(t, expanded.Load())
}

func TestSyncReadsRelatedSymlinkBlobWithoutFollowingTarget(t *testing.T) {
	t.Parallel()
	fixture := newRelatedMergeFixture(t)
	outside := filepath.Join(t.TempDir(), "private.txt")
	require.NoError(t, os.WriteFile(outside, []byte(relatedPrivateSentinel), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(fixture.repository, "source-link")))
	runGit(t, fixture.repository, "add", "source-link")
	runGit(t, fixture.repository, "commit", "-m", "track source link")
	fixture.localCommit = strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
	fixture.run(t, func(body []byte, input mergePlanInputForTest) (string, error) {
		if strings.Contains(string(body), relatedPrivateSentinel) {
			return "", fmt.Errorf("symlink target reached the model")
		}
		if input.Path == relatedTestPath {
			if len(input.RelatedFiles) == 0 {
				return `{"status":"needs_context","reason":"Read the tracked source link.","paths":["source-link"]}`, nil
			}
			versions := input.RelatedFiles[0].Versions
			if len(versions) != 1 || versions[0].Source != "index" || versions[0].Content != outside {
				return "", fmt.Errorf("related link content is not the Git blob")
			}
		}
		return mergePlanSourceFixtureResponse(body, fixture.ours[input.Path])
	}, "")
	require.Equal(t, relatedPrivateSentinel, readTextFile(t, outside))
}

func TestSyncReadsRelatedLiteralPaths(t *testing.T) {
	t.Parallel()
	const path = " schema [1] café.txt "
	const content = "literal source content\n"
	fixture := newRelatedMergeFixture(t, map[string]string{path: content})
	fixture.run(t, func(body []byte, input mergePlanInputForTest) (string, error) {
		if input.Path == relatedTestPath {
			if len(input.RelatedFiles) == 0 {
				response, err := json.Marshal(map[string]any{"status": "needs_context", "reason": "Read the literal tracked file.", "paths": []string{path}})
				return string(response), err
			}
			file := input.RelatedFiles[0]
			if file.Path != path || len(file.Versions) != 1 || file.Versions[0].Content != content {
				return "", fmt.Errorf("related path or content was normalized")
			}
		}
		return mergePlanSourceFixtureResponse(body, fixture.ours[input.Path])
	}, "")
	require.Equal(t, content, readTextFile(t, filepath.Join(fixture.repository, path)))
}

func TestSyncReadsPreviouslyDeletedRelatedFile(t *testing.T) {
	t.Parallel()
	fixture := newRelatedMergeFixture(t)
	runGit(t, fixture.repository, "switch", "incoming")
	runGit(t, fixture.repository, "rm", relatedPolicyPath)
	runGit(t, fixture.repository, "commit", "-m", "remove obsolete policy")
	runGit(t, fixture.repository, "push", "origin", "HEAD:master")
	fixture.remoteCommit = strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "master"))
	runGit(t, fixture.repository, "switch", "master")
	fixture.fileSelectionResponse = "GIX_MERGE_SELECT_THEIRS"
	fixture.deletedPaths = []string{relatedPolicyPath}
	fixture.run(t, func(body []byte, input mergePlanInputForTest) (string, error) {
		if len(input.RelatedFiles) == 0 {
			return `{"status":"needs_context","reason":"Read the earlier deletion and original policy sources.","paths":["a-policy.txt"]}`, nil
		}
		file := input.RelatedFiles[0]
		if file.Path != relatedPolicyPath || !file.Resolved || !file.Deleted || len(file.Versions) != 2 ||
			file.Versions[0].Source != "base" || file.Versions[0].Content != fixture.base[relatedPolicyPath] ||
			file.Versions[1].Source != "ours" || file.Versions[1].Content != fixture.ours[relatedPolicyPath] {
			return "", fmt.Errorf("missing deleted file sources or deletion result")
		}
		return mergePlanSourceFixtureResponse(body, fixture.ours[relatedTestPath])
	}, "")
}

const (
	relatedPolicyPath      = "a-policy.txt"
	relatedSchemaPath      = "schema.py"
	relatedTestPath        = "z-tests.txt"
	relatedPrivateSentinel = "UNTRACKED_PRIVATE_CONTEXT_MUST_STAY_LOCAL"
)

type relatedMergeFixture struct {
	repository, remote, workspace string
	base, ours, theirs            map[string]string
	localCommit, remoteCommit     string
	fileSelectionResponse         string
	deletedPaths                  []string
}

func newRelatedMergeFixture(t *testing.T, additional ...map[string]string) relatedMergeFixture {
	t.Helper()
	workspace := syncHomeWorkspace(t)
	fixture := relatedMergeFixture{
		workspace:  workspace,
		repository: filepath.Join(workspace, "project"),
		remote:     filepath.Join(workspace, "remote.git"),
		base: map[string]string{
			relatedPolicyPath: "Manifest field: original\n",
			relatedSchemaPath: "accepted_field = 'environment.source'\n",
			relatedTestPath:   "required_field = original\n",
		},
		ours: map[string]string{
			relatedPolicyPath: "Manifest field: environment.source\n",
			relatedTestPath:   "required_field = environment.source\n",
		},
		theirs: map[string]string{
			relatedPolicyPath: "Manifest field: environment_file\n",
			relatedTestPath:   "required_field = environment_file\n",
		},
	}
	fixture.base[".gitignore"] = ".env\nuntracked.txt\n"
	fixture.theirs[relatedSchemaPath] = "accepted_field = 'environment.source'\n"
	for _, files := range additional {
		for path, content := range files {
			fixture.base[path] = content
			if path == relatedSchemaPath {
				fixture.theirs[path] = content
			}
		}
	}
	for _, files := range []map[string]string{fixture.base, fixture.ours, fixture.theirs} {
		files[relatedTestPath] = strings.Repeat("# unchanged assertion\n", 2_000) + "# See a-policy.txt and schema.py.\n" + files[relatedTestPath]
	}
	createSyncGitHubBackedRepository(t, fixture.remote, fixture.repository)
	write := func(files map[string]string) {
		for path, content := range files {
			require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(fixture.repository, path)), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(fixture.repository, path), []byte(content), 0o644))
		}
	}
	write(fixture.base)
	runGit(t, fixture.repository, "add", ".")
	runGit(t, fixture.repository, "commit", "-m", "base")
	runGit(t, fixture.repository, "push", "-u", "origin", "master")
	runGit(t, fixture.repository, "switch", "-c", "incoming")
	write(fixture.theirs)
	runGit(t, fixture.repository, "commit", "-am", "incoming")
	runGit(t, fixture.repository, "push", "origin", "HEAD:master")
	fixture.remoteCommit = strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "master"))
	runGit(t, fixture.repository, "switch", "master")
	write(fixture.ours)
	runGit(t, fixture.repository, "commit", "-am", "local")
	fixture.localCommit = strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD"))
	require.NoError(t, os.WriteFile(filepath.Join(fixture.repository, ".env"), []byte(relatedPrivateSentinel), 0o600))
	return fixture
}

func (fixture relatedMergeFixture) run(t *testing.T, response func([]byte, mergePlanInputForTest) (string, error), failure string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		var input mergePlanInputForTest
		selection := fixture.fileSelectionResponse != "" && strings.Contains(string(body), "GIX_MERGE_SELECT_THEIRS")
		if err == nil && !selection {
			input, err = decodeMergePlanInputForTest(body)
		}
		var content string
		if err == nil {
			if selection {
				content = fixture.fileSelectionResponse
			} else {
				content, err = response(body, input)
			}
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": content}}}})
	}))
	t.Cleanup(server.Close)
	configuration := writeReportedLifecycleSemanticConfiguration(t, server.URL)
	output, err := runBinaryIntegrationCommand(t, buildIntegrationBinary(t, integrationRepositoryRoot(t)), fixture.repository, map[string]string{
		pathEnvironmentVariableNameConstant: buildSyncMergedBranchExecutablePath(t),
		syncMergedBranchGitLogVariable:      filepath.Join(fixture.workspace, "git.log"),
		syncMergedBranchGitHubLogVariable:   filepath.Join(fixture.workspace, "gh.log"),
		syncMergedBranchNameVariable:        "master",
		syncMergedBranchMergedVariable:      "false",
	}, syncStateTransitionTimeout, []string{"--config", configuration, "--roots", fixture.repository, "sync", "master"})
	if failure == "" {
		require.NoError(t, err, output)
		require.Contains(t, output, "expanded semantic read context to related tracked files")
		for path, expected := range fixture.ours {
			deleted := false
			for _, deletedPath := range fixture.deletedPaths {
				deleted = deleted || path == deletedPath
			}
			if deleted {
				_, readErr := os.ReadFile(filepath.Join(fixture.repository, path))
				require.ErrorIs(t, readErr, os.ErrNotExist)
				require.Empty(t, runGit(t, fixture.remote, "ls-tree", "--name-only", "master", "--", path))
				continue
			}
			require.Equal(t, expected, readTextFile(t, filepath.Join(fixture.repository, path)))
			require.Equal(t, expected, runGit(t, fixture.remote, "show", "master:"+path))
		}
		require.Empty(t, runGit(t, fixture.repository, "status", "--porcelain", "--untracked-files=no"))
	} else {
		require.Error(t, err, output)
		require.Contains(t, output, failure)
		require.Contains(t, output, "AI_MERGE_ROLLBACK")
		require.Equal(t, fixture.localCommit, strings.TrimSpace(runGit(t, fixture.repository, "rev-parse", "HEAD")))
		require.Equal(t, fixture.remoteCommit, strings.TrimSpace(runGit(t, fixture.remote, "rev-parse", "master")))
		for path, expected := range fixture.ours {
			require.Equal(t, expected, readTextFile(t, filepath.Join(fixture.repository, path)))
		}
	}
	require.Equal(t, relatedPrivateSentinel, readTextFile(t, filepath.Join(fixture.repository, ".env")))
	require.Empty(t, runGit(t, fixture.repository, "stash", "list"))
}
