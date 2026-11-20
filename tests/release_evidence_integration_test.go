package tests

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/tyemirov/gix/internal/semverdecision"
)

func TestReleaseNextAnalyzesCompleteLargeDiff(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	var mutex sync.Mutex
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, readError := io.ReadAll(request.Body)
		require.NoError(testInstance, readError)
		mutex.Lock()
		requests = append(requests, string(body))
		mutex.Unlock()
		if len(body) > 96*1024 {
			writer.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		if strings.Contains(string(body), "LATE_CONTRACT_REMOVAL") {
			_, _ = io.WriteString(writer, `{"impact":"incompatible","public_contract":"supported text command","reason":"The release removes a supported command."}`)
			return
		}
		_, _ = io.WriteString(writer, `{"impact":"compatible","public_contract":"","reason":"This packet contains compatible maintenance."}`)
	}))
	defer server.Close()
	repository := releaseEvidenceRepository(testInstance)
	noise := strings.Repeat("Routine maintenance preserves the supported public behavior.\n", 24000)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "a-maintenance.txt"), []byte(noise), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "z-command.txt"), []byte("LATE_CONTRACT_REMOVAL: reject the previously supported text command.\n"), 0o644))
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "fix: update behavior and maintenance")
	configuration := releaseEvidenceConfiguration(testInstance, server.URL)
	output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
		[]string{"--config", configuration, "release", "next", "semver", "--format", "json"})
	require.NoError(testInstance, runError, output)
	require.Contains(testInstance, output, `"next_version":"v2.0.0"`)
	require.Contains(testInstance, output, `"bump":"major"`)
	mutex.Lock()
	captured := append([]string(nil), requests...)
	mutex.Unlock()
	require.Greater(testInstance, len(captured), 2)
	for _, request := range captured {
		require.LessOrEqual(testInstance, len(request), 96*1024)
	}
	record := releaseEvidenceRecord(testInstance, repository)
	require.Contains(testInstance, string(record), "LATE_CONTRACT_REMOVAL")
	require.Contains(testInstance, string(record), "v2.0.0")
	require.NotContains(testInstance, string(record), "proxy-secret")
}

func releaseEvidenceRepository(testInstance *testing.T) string {
	testInstance.Helper()
	repository := createGitRepository(testInstance, gitRepositoryOptions{DirectoryName: "release-evidence-fixture", InitialBranch: "master"})
	configureGitIdentity(testInstance, repository)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "z-command.txt"), []byte("The text command is supported.\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "behavior.txt"), []byte("OLD_SUPPORTED_BEHAVIOR: existing consumers may use the text command.\n"), 0o644))
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "fixture baseline")
	runGit(testInstance, repository, "tag", "v1.2.3")
	return repository
}

func TestReleaseNextRetrievesCommittedContext(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, readError := io.ReadAll(request.Body)
		require.NoError(testInstance, readError)
		if !strings.Contains(string(body), "OLD_SUPPORTED_BEHAVIOR") {
			_, _ = io.WriteString(writer, `{"impact":"unresolved","public_contract":"text command","reason":"The previous supported behavior needs source context.","context_requests":[{"path":"behavior.txt","source":"old","start_line":1,"line_count":1}]}`)
			return
		}
		_, _ = io.WriteString(writer, `{"impact":"incompatible","public_contract":"text command","reason":"The release removes the command used by existing consumers."}`)
	}))
	defer server.Close()
	repository := releaseEvidenceRepository(testInstance)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "z-command.txt"), []byte("REQUEST_CONTEXT_INSPECTION: the text command is removed.\n"), 0o644))
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "change command behavior")
	// A working copy edit must never become evidence for the committed release.
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "behavior.txt"), []byte("UNCOMMITTED_BEHAVIOR_MUST_NOT_APPEAR\n"), 0o644))
	configuration := releaseEvidenceConfiguration(testInstance, server.URL)
	output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
		[]string{"--config", configuration, "release", "next", "semver", "--format", "json"})
	require.NoError(testInstance, runError, output)
	require.Contains(testInstance, output, `"next_version":"v2.0.0"`)
	record := string(releaseEvidenceRecord(testInstance, repository))
	require.Contains(testInstance, record, "OLD_SUPPORTED_BEHAVIOR")
	require.NotContains(testInstance, record, "UNCOMMITTED_BEHAVIOR_MUST_NOT_APPEAR")
}

func TestReleaseNextUsesAuthoritativeChangedFileBounds(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	repository := releaseEvidenceRepository(testInstance)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "apple_review.go"), []byte(strings.Repeat("old line\n", 18)), 0o644))
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "add baseline source")
	runGit(testInstance, repository, "tag", "-f", "v1.2.3")
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "apple_review.go"), []byte(strings.Repeat("new line\n", 46)), 0o644))
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "repair source behavior")
	var requests []string
	var requestsMutex sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, readError := io.ReadAll(request.Body)
		require.NoError(testInstance, readError)
		requestsMutex.Lock()
		defer requestsMutex.Unlock()
		requests = append(requests, string(body))
		if len(requests) == 1 {
			count := 160
			if strings.Contains(string(body), `[true,18,`) && strings.Contains(string(body), `[true,46,`) {
				count = 18
			}
			_, _ = fmt.Fprintf(writer, `{"impact":"unresolved","public_contract":"","reason":"Inspect exact committed source.","context_requests":[{"path":"apple_review.go","source":"old","start_line":1,"line_count":%d}]}`, count)
			return
		}
		_, _ = io.WriteString(writer, `{"impact":"compatible","public_contract":"","reason":"The supported behavior is preserved."}`)
	}))
	defer server.Close()
	configuration := releaseEvidenceConfiguration(testInstance, server.URL)
	output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
		[]string{"--config", configuration, "release", "next", "semver", "--format", "json"})
	require.NoError(testInstance, runError, output)
	require.Contains(testInstance, output, `"next_version":"v1.2.4"`)
	requestsMutex.Lock()
	defer requestsMutex.Unlock()
	require.Len(testInstance, requests, 3)
	for _, request := range requests {
		require.Contains(testInstance, request, "Changed-file source catalog")
		require.Contains(testInstance, request, `[true,18,`)
		require.Contains(testInstance, request, `[true,46,`)
	}
}

func TestReleaseNextReadsLongCommittedBasename(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	repository := releaseEvidenceRepository(testInstance)
	path := "new-source-000-" + strings.Repeat("x", 220) + ".txt"
	require.Equal(testInstance, 239, len(path))
	oldContent, newContent := "Old committed behavior.\n界\n", "New committed behavior.\n界🙂\nFinal line"
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, path), []byte(oldContent), 0o644))
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "long basename baseline")
	runGit(testInstance, repository, "tag", "-f", "v1.2.3")
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, path), []byte(newContent), 0o644))
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "repair long basename source")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = fmt.Fprintf(writer, `{"impact":"unresolved","public_contract":"","reason":"Inspect exact committed snapshots.","context_requests":[{"path":%q,"source":"old","start_line":1,"line_count":2},{"path":%q,"source":"new","start_line":1,"line_count":3}]}`, path, path)
			return
		}
		_, _ = io.WriteString(writer, releaseCompatibleDecision)
	}))
	defer server.Close()
	output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
		[]string{"--config", releaseEvidenceConfiguration(testInstance, server.URL), "release", "next", "semver", "--format", "json"})
	require.NoError(testInstance, runError, output)
	require.Contains(testInstance, output, `"next_version":"v1.2.4"`)
	require.EqualValues(testInstance, 3, calls.Load())
	analysis := releaseRecordedAnalysis(testInstance, repository)
	require.Len(testInstance, analysis.Packets, 1)
	catalog := releasePacketCatalog(testInstance, analysis.Packets[0].Content)
	require.Len(testInstance, catalog, 2)
	require.Len(testInstance, analysis.Context, 2)
	for index, content := range []string{oldContent, newContent} {
		source := []string{"old", "new"}[index]
		row := catalog[index]
		require.Equal(testInstance, path, row.Path)
		require.Equal(testInstance, source, row.Source)
		require.True(testInstance, row.Present)
		require.Equal(testInstance, index+2, row.LineCount)
		require.Equal(testInstance, utf8.RuneCountInString(content), row.RuneCount)
		maxLine := 0
		for _, line := range strings.SplitAfter(content, "\n") {
			maxLine = max(maxLine, utf8.RuneCountInString(line))
		}
		require.Equal(testInstance, maxLine, row.MaxLineRunes)
		formatted := fmt.Sprintf("Source %s, path %s, lines %d count %d, absent=false:\n\n", source, path, row.LineCount, row.LineCount)
		require.Equal(testInstance, utf8.RuneCountInString(formatted), row.FormatOverheadRunes)
		evidence := analysis.Context[index]
		require.Equal(testInstance, path, evidence.Path)
		require.Equal(testInstance, source, evidence.Source)
		require.Equal(testInstance, 1, evidence.StartLine)
		require.Equal(testInstance, row.LineCount, evidence.LineCount)
		require.False(testInstance, evidence.Absent)
		require.Equal(testInstance, content, evidence.Content)
		require.Equal(testInstance, fmt.Sprintf("%x", sha256.Sum256([]byte(content))), evidence.SHA256)
	}
}

func TestReleaseNextCatalogsCompleteCommittedSnapshots(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	repository := releaseEvidenceRepository(testInstance)
	oldFiles := map[string]string{"renamed-old.txt": "unchanged rename\n", "deleted.txt": "界\nlast", "empty.txt": "", "no-final-newline.txt": "old"}
	for path, content := range oldFiles {
		require.NoError(testInstance, os.WriteFile(filepath.Join(repository, path), []byte(content), 0o644))
	}
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "snapshot baseline")
	runGit(testInstance, repository, "tag", "-f", "v1.2.3")
	require.NoError(testInstance, os.Rename(filepath.Join(repository, "renamed-old.txt"), filepath.Join(repository, "renamed-new.txt")))
	require.NoError(testInstance, os.Remove(filepath.Join(repository, "deleted.txt")))
	newFiles := map[string]string{"renamed-new.txt": "unchanged rename\n", "empty.txt": "界\n", "added-empty.txt": "", "no-final-newline.txt": "界🙂", "noise.txt": strings.Repeat("界🙂 unchanged public behavior\n", 1800)}
	for path, content := range newFiles {
		require.NoError(testInstance, os.WriteFile(filepath.Join(repository, path), []byte(content), 0o644))
	}
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "update committed snapshots")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, releaseCompatibleDecision)
	}))
	defer server.Close()
	output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
		[]string{"--config", releaseEvidenceConfiguration(testInstance, server.URL), "release", "next", "semver", "--format", "json"})
	require.NoError(testInstance, runError, output)
	analysis := releaseRecordedAnalysis(testInstance, repository)
	require.Greater(testInstance, len(analysis.Packets), 1)
	var firstCatalog []releaseSourceBounds
	for _, packet := range analysis.Packets {
		require.LessOrEqual(testInstance, utf8.RuneCountInString(packet.Content), 32000)
		require.Equal(testInstance, fmt.Sprintf("%x", sha256.Sum256([]byte(packet.Content))), packet.SHA256)
		catalog := releasePacketCatalog(testInstance, packet.Content)
		if firstCatalog == nil {
			firstCatalog = catalog
		} else {
			require.Equal(testInstance, firstCatalog, catalog)
		}
	}
	paths := []string{"added-empty.txt", "deleted.txt", "empty.txt", "no-final-newline.txt", "noise.txt", "renamed-new.txt", "renamed-old.txt"}
	require.Len(testInstance, firstCatalog, len(paths)*2)
	for index, path := range paths {
		for sourceIndex, source := range []string{"old", "new"} {
			files := oldFiles
			if source == "new" {
				files = newFiles
			}
			content, present := files[path]
			row := firstCatalog[index*2+sourceIndex]
			require.Equal(testInstance, path, row.Path)
			require.Equal(testInstance, source, row.Source)
			require.Equal(testInstance, present, row.Present)
			require.Equal(testInstance, utf8.RuneCountInString(content), row.RuneCount)
			lines := strings.SplitAfter(content, "\n")
			if lines[len(lines)-1] == "" {
				lines = lines[:len(lines)-1]
			}
			require.Equal(testInstance, len(lines), row.LineCount)
			maxLine := 0
			for _, line := range lines {
				maxLine = max(maxLine, utf8.RuneCountInString(line))
			}
			require.Equal(testInstance, maxLine, row.MaxLineRunes)
			headerContent := ""
			if !present {
				headerContent = "File is absent from this committed source."
			}
			maxRange := max(1, len(lines))
			formatted := fmt.Sprintf("Source %s, path %s, lines %d count %d, absent=%t:\n%s\n", source, path, maxRange, maxRange, !present, headerContent)
			require.Equal(testInstance, utf8.RuneCountInString(formatted), row.FormatOverheadRunes)
		}
	}
	for _, coverage := range analysis.Coverage {
		require.Equal(testInstance, coverage.RuneCount, coverage.CoveredRunes)
	}
	for index, request := range analysis.Requests {
		require.Contains(testInstance, request.Messages[1].Content, fmt.Sprintf("model_calls_used=%d model_calls_remaining=%d", index, 128-index))
		require.Contains(testInstance, request.Messages[1].Content, "context_runes_used=0 context_runes_remaining=24000")
	}
}

func TestReleaseNextRejectsOversizeCompleteCatalog(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	repository := releaseEvidenceRepository(testInstance)
	catalogDirectory := filepath.Join(repository, "catalog-"+strings.Repeat("x", 150))
	require.NoError(testInstance, os.MkdirAll(catalogDirectory, 0o755))
	for index := range 120 {
		path := filepath.Join(catalogDirectory, fmt.Sprintf("new-source-%03d-%s.txt", index, strings.Repeat("x", 110)))
		require.NoError(testInstance, os.WriteFile(path, nil, 0o644))
	}
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "add empty committed sources")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(writer, releaseCompatibleDecision)
	}))
	defer server.Close()
	output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
		[]string{"--config", releaseEvidenceConfiguration(testInstance, server.URL), "release", "next", "semver", "--format", "json"})
	require.Error(testInstance, runError, output)
	require.Contains(testInstance, output, "complete changed-file source catalog")
	require.Contains(testInstance, output, "exceed evidence packet limit")
	require.NotContains(testInstance, output, `"next_version"`)
	require.Zero(testInstance, calls.Load())
	require.Contains(testInstance, string(releaseEvidenceRecord(testInstance, repository)), "complete changed-file source catalog")
}

func TestReleaseNextFitsCompleteCompactCatalog(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	repository := releaseEvidenceRepository(testInstance)
	paths := make([]string, 134)
	for index := range paths {
		paths[index] = fmt.Sprintf("source-%03d-%s.txt", index, strings.Repeat("context", 8))
		if index < 120 {
			require.NoError(testInstance, os.WriteFile(filepath.Join(repository, paths[index]), []byte(strings.Repeat("old", 900)+"\n"), 0o644))
		}
	}
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "large catalog baseline")
	runGit(testInstance, repository, "tag", "-f", "v1.2.3")
	for _, path := range paths {
		require.NoError(testInstance, os.WriteFile(filepath.Join(repository, path), []byte(strings.Repeat("new", 900)+"\n"), 0o644))
	}
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "repair bounded source behavior")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(writer, releaseCompatibleDecision)
	}))
	defer server.Close()
	output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
		[]string{"--config", releaseEvidenceConfiguration(testInstance, server.URL), "release", "next", "semver", "--format", "json"})
	require.NoError(testInstance, runError, output)
	require.Contains(testInstance, output, `"next_version":"v1.2.4"`)
	analysis := releaseRecordedAnalysis(testInstance, repository)
	require.Greater(testInstance, len(analysis.Packets), 1)
	require.Equal(testInstance, len(analysis.Packets)*2, int(calls.Load()))
	require.LessOrEqual(testInstance, calls.Load(), int32(128))
	var catalog []releaseSourceBounds
	fragments := map[string]string{}
	maxPacketRunes, compactCatalogRunes := 0, 0
	for _, packet := range analysis.Packets {
		packetRunes := utf8.RuneCountInString(packet.Content)
		maxPacketRunes = max(maxPacketRunes, packetRunes)
		require.LessOrEqual(testInstance, packetRunes, 32000)
		require.Equal(testInstance, fmt.Sprintf("%x", sha256.Sum256([]byte(packet.Content))), packet.SHA256)
		rows := releasePacketCatalog(testInstance, packet.Content)
		if catalog == nil {
			catalog = rows
			catalogStart := strings.Index(packet.Content, "Changed-file source catalog")
			catalogEnd := strings.Index(packet.Content, "\nEvidence packet:") + 1
			require.Greater(testInstance, catalogEnd, catalogStart)
			compactCatalogRunes = utf8.RuneCountInString(packet.Content[catalogStart:catalogEnd])
		} else {
			require.Equal(testInstance, catalog, rows)
		}
		for _, fragment := range packet.Fragments {
			require.Equal(testInstance, utf8.RuneCountInString(fragments[fragment.Kind]), fragment.OffsetRunes)
			fragments[fragment.Kind] += fragment.Content
		}
	}
	require.Len(testInstance, catalog, 268)
	verboseRunes, presentCount := 0, 0
	for index, row := range catalog {
		require.Equal(testInstance, paths[index/2], row.Path)
		require.Equal(testInstance, []string{"old", "new"}[index%2], row.Source)
		present := index%2 == 1 || index/2 < 120
		require.Equal(testInstance, present, row.Present)
		if present {
			presentCount++
			require.Equal(testInstance, 1, row.LineCount)
			require.Equal(testInstance, 2701, row.RuneCount)
			require.Equal(testInstance, 2701, row.MaxLineRunes)
		} else {
			require.Zero(testInstance, row.LineCount)
			require.Zero(testInstance, row.RuneCount)
			require.Zero(testInstance, row.MaxLineRunes)
		}
		verbose, encodeError := json.Marshal(row)
		require.NoError(testInstance, encodeError)
		verboseRunes += utf8.RuneCountInString(string(verbose)) + 1
	}
	require.Equal(testInstance, 254, presentCount)
	require.Greater(testInstance, verboseRunes, 32000)
	require.Less(testInstance, compactCatalogRunes, 32000)
	require.Less(testInstance, compactCatalogRunes, verboseRunes)
	totalEvidenceRunes := 0
	for _, coverage := range analysis.Coverage {
		content := fragments[coverage.Kind]
		require.Equal(testInstance, coverage.RuneCount, utf8.RuneCountInString(content))
		require.Equal(testInstance, coverage.RuneCount, coverage.CoveredRunes)
		require.Equal(testInstance, fmt.Sprintf("%x", sha256.Sum256([]byte(content))), coverage.SHA256)
		totalEvidenceRunes += coverage.RuneCount
	}
	testInstance.Logf("catalog paths=134 snapshots=268 present=%d absent=%d verbose_runes=%d compact_runes=%d evidence_runes=%d packets=%d max_packet_runes=%d calls=%d remaining_calls=%d", presentCount, len(catalog)-presentCount, verboseRunes, compactCatalogRunes, totalEvidenceRunes, len(analysis.Packets), maxPacketRunes, calls.Load(), 128-calls.Load())
}

const releaseCompatibleDecision = `{"impact":"compatible","public_contract":"","reason":"The supported behavior is preserved."}`

type releaseSourceBounds struct {
	Path                string `json:"path"`
	Source              string `json:"source"`
	Present             bool   `json:"present"`
	LineCount           int    `json:"line_count"`
	RuneCount           int    `json:"rune_count"`
	MaxLineRunes        int    `json:"max_line_runes"`
	FormatOverheadRunes int    `json:"format_overhead_runes"`
}

func releasePacketCatalog(testInstance *testing.T, content string) []releaseSourceBounds {
	testInstance.Helper()
	require.Contains(testInstance, content, "JSONL rows [path,old,new]; snapshot tuple [present,line_count,rune_count,max_line_runes,format_overhead_runes]\n")
	var catalog []releaseSourceBounds
	prefix := strings.SplitN(content, "\nEvidence packet:", 2)[0]
	for _, line := range strings.Split(prefix, "\n") {
		if !strings.HasPrefix(line, `["`) {
			continue
		}
		var row []json.RawMessage
		require.NoError(testInstance, json.Unmarshal([]byte(line), &row))
		require.Len(testInstance, row, 3)
		var path string
		require.NoError(testInstance, json.Unmarshal(row[0], &path))
		for index, source := range []string{"old", "new"} {
			var tuple []json.RawMessage
			require.NoError(testInstance, json.Unmarshal(row[index+1], &tuple))
			require.Len(testInstance, tuple, 5)
			bounds := releaseSourceBounds{Path: path, Source: source}
			fields := []any{&bounds.Present, &bounds.LineCount, &bounds.RuneCount, &bounds.MaxLineRunes, &bounds.FormatOverheadRunes}
			for fieldIndex, field := range fields {
				require.NoError(testInstance, json.Unmarshal(tuple[fieldIndex], field))
			}
			catalog = append(catalog, bounds)
		}
	}
	require.NotEmpty(testInstance, catalog)
	return catalog
}

func releaseRecordedAnalysis(testInstance *testing.T, repository string) semverdecision.Result {
	testInstance.Helper()
	var record struct {
		Analysis semverdecision.Result `json:"analysis"`
	}
	require.NoError(testInstance, json.Unmarshal(releaseEvidenceRecord(testInstance, repository), &record))
	return record.Analysis
}

func TestReleaseNextDisclosesFreshGlobalAllowances(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	repository := releaseEvidenceRepository(testInstance)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "z-command.txt"), []byte("New command behavior.\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "noise.txt"), []byte(strings.Repeat("Maintenance preserves supported behavior.\n", 1000)), 0o644))
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "repair behavior")
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		call := calls.Add(1)
		var requested string
		switch call {
		case 1:
			requested = `{"path":"z-command.txt","source":"old","start_line":1,"line_count":1}`
		case 2:
			requested = `{"path":"z-command.txt","source":"new","start_line":1,"line_count":1}`
		case 4:
			requested = `{"path":"behavior.txt","source":"old","start_line":1,"line_count":1}`
		}
		if requested != "" {
			_, _ = fmt.Fprintf(writer, `{"impact":"unresolved","public_contract":"","reason":"Inspect source.","context_requests":[%s]}`, requested)
			return
		}
		_, _ = io.WriteString(writer, releaseCompatibleDecision)
	}))
	defer server.Close()
	output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
		[]string{"--config", releaseEvidenceConfiguration(testInstance, server.URL), "release", "next", "semver", "--format", "json"})
	require.NoError(testInstance, runError, output)
	analysis := releaseRecordedAnalysis(testInstance, repository)
	require.Greater(testInstance, len(analysis.Packets), 1)
	require.Len(testInstance, analysis.Context, 3)
	require.Equal(testInstance, len(analysis.Packets)*2+3, int(calls.Load()))
	used := 0
	for index, request := range analysis.Requests {
		if index == 1 || index == 2 || index == 4 {
			contextIndex := map[int]int{1: 0, 2: 1, 4: 2}[index]
			evidence := analysis.Context[contextIndex]
			formatted := fmt.Sprintf("Source %s, path %s, lines %d count %d, absent=%t:\n%s\n", evidence.Source, evidence.Path, evidence.StartLine, evidence.LineCount, evidence.Absent, evidence.Content)
			used += utf8.RuneCountInString(formatted)
		}
		content := request.Messages[1].Content
		require.Contains(testInstance, content, fmt.Sprintf("context_runes_used=%d context_runes_remaining=%d context_runes_limit=24000", used, 24000-used))
		require.Contains(testInstance, content, fmt.Sprintf("model_calls_used=%d model_calls_remaining=%d model_calls_limit=128", index, 128-index))
		rounds := 2
		if index == 1 || index == 4 {
			rounds = 1
		} else if index == 2 {
			rounds = 0
		}
		require.Contains(testInstance, content, fmt.Sprintf("context_rounds_remaining=%d context_rounds_limit=2", rounds))
		require.Contains(testInstance, request.Messages[0].Content, "sum the cost of every range")
	}
}

func TestReleaseNextReusesGlobalCommittedContext(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	for _, scenario := range []string{"later_candidate_and_audit", "submitted_request_limit"} {
		testInstance.Run(scenario, func(testInstance *testing.T) {
			repository := releaseEvidenceRepository(testInstance)
			candidateContent := "CANDIDATE_CONTEXT_AUTHORITY:" + strings.Repeat("界", 9000) + "\n"
			auditContent := "AUDIT_CONTEXT_AUTHORITY:" + strings.Repeat("🙂", 9000) + "\n"
			require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "candidate-context.txt"), []byte(candidateContent), 0o644))
			require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "audit-context.txt"), []byte(auditContent), 0o644))
			runGit(testInstance, repository, "add", ".")
			runGit(testInstance, repository, "commit", "-m", "immutable context baseline")
			runGit(testInstance, repository, "tag", "-f", "v1.2.3")
			require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "noise.txt"), []byte(strings.Repeat("Stable supported behavior.\n", 3000)), 0o644))
			runGit(testInstance, repository, "add", ".")
			runGit(testInstance, repository, "commit", "-m", "repair internal maintenance")
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				body, readError := io.ReadAll(request.Body)
				require.NoError(testInstance, readError)
				call := calls.Add(1)
				if call >= 5 && (!strings.Contains(string(body), "CANDIDATE_CONTEXT_AUTHORITY:") || !strings.Contains(string(body), "AUDIT_CONTEXT_AUTHORITY:")) {
					_, _ = io.WriteString(writer, `{"impact":"unresolved","public_contract":"","reason":"Earlier charged context was withheld."}`)
					return
				}
				var requested string
				switch call {
				case 1:
					requested = `{"path":"candidate-context.txt","source":"old","start_line":1,"line_count":1}`
				case 3:
					requested = `{"path":"audit-context.txt","source":"new","start_line":1,"line_count":1}`
				case 5:
					if scenario == "submitted_request_limit" {
						_, _ = fmt.Fprintf(writer, `{"impact":"compatible","public_contract":"","reason":%q}`, strings.Repeat("x", 20000))
						return
					}
				}
				if requested != "" {
					_, _ = fmt.Fprintf(writer, `{"impact":"unresolved","public_contract":"","reason":"Inspect committed authority.","context_requests":[%s]}`, requested)
					return
				}
				_, _ = io.WriteString(writer, releaseCompatibleDecision)
			}))
			defer server.Close()
			output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
				[]string{"--config", releaseEvidenceConfiguration(testInstance, server.URL), "release", "next", "semver", "--format", "json"})
			if scenario == "submitted_request_limit" {
				require.Error(testInstance, runError, output)
				require.Contains(testInstance, output, "audit submitted request exceeds 64000 rune limit")
				require.EqualValues(testInstance, 5, calls.Load())
				require.NotContains(testInstance, output, `"next_version"`)
			} else {
				require.NoError(testInstance, runError, output)
				require.Contains(testInstance, output, `"next_version":"v1.2.4"`)
			}
			analysis := releaseRecordedAnalysis(testInstance, repository)
			require.GreaterOrEqual(testInstance, len(analysis.Packets), 3)
			require.Len(testInstance, analysis.Context, 2)
			formatted := make([]string, 2)
			for index, evidence := range analysis.Context {
				require.Equal(testInstance, []string{candidateContent, auditContent}[index], evidence.Content)
				formatted[index] = fmt.Sprintf("Source %s, path %s, lines %d count %d, absent=%t:\n%s\n", evidence.Source, evidence.Path, evidence.StartLine, evidence.LineCount, evidence.Absent, evidence.Content)
			}
			usedRunes := 0
			for index, request := range analysis.Requests {
				if index == 1 {
					usedRunes += utf8.RuneCountInString(formatted[0])
				} else if index == 3 {
					usedRunes += utf8.RuneCountInString(formatted[1])
				}
				content := request.Messages[1].Content
				require.Contains(testInstance, content, fmt.Sprintf("context_runes_used=%d context_runes_remaining=%d context_runes_limit=24000", usedRunes, 24000-usedRunes))
				if index >= 1 {
					require.Equal(testInstance, 1, strings.Count(content, formatted[0]))
				}
				if index >= 3 {
					require.Equal(testInstance, 1, strings.Count(content, formatted[1]))
				}
				requestRunes := 0
				for _, message := range request.Messages {
					requestRunes += utf8.RuneCountInString(message.Content)
				}
				if scenario == "submitted_request_limit" && index == len(analysis.Requests)-1 {
					require.Greater(testInstance, requestRunes, 64000)
					require.LessOrEqual(testInstance, requestRunes-usedRunes, 64000)
				} else {
					require.LessOrEqual(testInstance, requestRunes, 64000)
				}
			}
			if scenario == "submitted_request_limit" {
				require.Len(testInstance, analysis.Requests, 6)
			} else {
				require.Equal(testInstance, len(analysis.Packets)*2+2, int(calls.Load()))
			}
			for _, coverage := range analysis.Coverage {
				require.Equal(testInstance, coverage.RuneCount, coverage.CoveredRunes)
			}
		})
	}
}

func TestReleaseNextRejectsExactInvalidContextWithoutRetry(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	for _, scenario := range []string{"eof", "aggregate_24616"} {
		testInstance.Run(scenario, func(testInstance *testing.T) {
			repository := releaseEvidenceRepository(testInstance)
			path := "publication-guide.txt"
			guide := func(source string, cost int) string {
				header := fmt.Sprintf("Source %s, path %s, lines 1 count 200, absent=false:\n\n", source, path)
				return strings.Repeat("x", cost-utf8.RuneCountInString(header)-200) + strings.Repeat("\n", 200)
			}
			require.NoError(testInstance, os.WriteFile(filepath.Join(repository, path), []byte(guide("old", 11788)), 0o644))
			runGit(testInstance, repository, "add", ".")
			runGit(testInstance, repository, "commit", "-m", "baseline guide")
			runGit(testInstance, repository, "tag", "-f", "v1.2.3")
			require.NoError(testInstance, os.WriteFile(filepath.Join(repository, path), []byte(guide("new", 12828)), 0o644))
			runGit(testInstance, repository, "add", ".")
			runGit(testInstance, repository, "commit", "-m", "repair guide")
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				requests := fmt.Sprintf(`{"path":%q,"source":"old","start_line":1,"line_count":201}`, path)
				if scenario == "aggregate_24616" {
					requests = fmt.Sprintf(`{"path":%q,"source":"old","start_line":1,"line_count":200},{"path":%q,"source":"new","start_line":1,"line_count":200}`, path, path)
				}
				_, _ = fmt.Fprintf(writer, `{"impact":"unresolved","public_contract":"","reason":"Inspect exact source.","context_requests":[%s]}`, requests)
			}))
			defer server.Close()
			output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
				[]string{"--config", releaseEvidenceConfiguration(testInstance, server.URL), "release", "next", "semver", "--format", "json"})
			require.Error(testInstance, runError, output)
			require.EqualValues(testInstance, 1, calls.Load())
			require.NotContains(testInstance, output, `"next_version"`)
			analysis := releaseRecordedAnalysis(testInstance, repository)
			require.Len(testInstance, analysis.Requests, 1)
			if scenario == "eof" {
				require.Contains(testInstance, output, "exceeds committed file lines")
				require.Empty(testInstance, analysis.Context)
			} else {
				require.Contains(testInstance, output, "global 24000 rune limit")
				require.Len(testInstance, analysis.Context, 1)
				cost := 0
				for index, source := range []string{"old", "new"} {
					expected := []int{11788, 12828}[index]
					formatted := fmt.Sprintf("Source %s, path %s, lines 1 count 200, absent=false:\n%s\n", source, path, guide(source, expected))
					require.Equal(testInstance, expected, utf8.RuneCountInString(formatted))
					cost += utf8.RuneCountInString(formatted)
				}
				require.Equal(testInstance, 24616, cost)
			}
		})
	}
}

func TestReleaseNextRetainsFailedEvidence(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	cases := []struct {
		name, response, expected string
		status                   int
	}{
		{name: "unresolved", response: `{"impact":"unresolved","public_contract":"text command","reason":"The supported behavior remains unresolved."}`, expected: "unresolved"},
		{name: "unsafe_context", response: `{"impact":"unresolved","public_contract":"text command","reason":"More context is needed.","context_requests":[{"path":"../outside.txt","source":"old","start_line":1,"line_count":1}]}`, expected: "context"},
		{name: "provider", status: http.StatusInternalServerError, response: "controlled provider failure", expected: "candidate.llm"},
	}
	for _, scenario := range cases {
		testInstance.Run(scenario.name, func(testInstance *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if scenario.status != 0 {
					writer.WriteHeader(scenario.status)
				}
				_, _ = io.WriteString(writer, scenario.response)
			}))
			defer server.Close()
			repository := releaseEvidenceRepository(testInstance)
			require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "z-command.txt"), []byte("The command behavior changes.\n"), 0o644))
			runGit(testInstance, repository, "add", ".")
			runGit(testInstance, repository, "commit", "-m", "change command behavior")
			configuration := releaseEvidenceConfiguration(testInstance, server.URL)
			output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
				[]string{"--config", configuration, "release", "next", "semver", "--format", "json"})
			require.Error(testInstance, runError, output)
			require.Contains(testInstance, output, scenario.expected)
			require.NotContains(testInstance, output, `"next_version"`)
			var record map[string]json.RawMessage
			require.NoError(testInstance, json.Unmarshal(releaseEvidenceRecord(testInstance, repository), &record))
			require.Contains(testInstance, string(record["failure"]), scenario.expected)
		})
	}
}

func TestReleaseNextStopsWhenEvidenceCannotBeSaved(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, `{"impact":"compatible","public_contract":"","reason":"The release preserves supported behavior."}`)
	}))
	defer server.Close()
	repository := releaseEvidenceRepository(testInstance)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "z-command.txt"), []byte("The supported command has a compatible repair.\n"), 0o644))
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "fix command behavior")
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, ".git", "gix"), []byte("An existing file prevents evidence directory creation.\n"), 0o644))
	configuration := releaseEvidenceConfiguration(testInstance, server.URL)
	output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
		[]string{"--config", configuration, "release", "next", "semver", "--format", "json"})
	require.Error(testInstance, runError, output)
	require.Contains(testInstance, output, "create release evidence directory")
	require.NotContains(testInstance, output, `"next_version"`)
}

func TestReleaseNextStopsWhenCompleteEvidenceExceedsBudget(testInstance *testing.T) {
	workingDirectory, err := os.Getwd()
	require.NoError(testInstance, err)
	binary := buildIntegrationBinary(testInstance, filepath.Dir(workingDirectory))
	var mutex sync.Mutex
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		calls++
		mutex.Unlock()
		_, _ = io.WriteString(writer, `{"impact":"compatible","public_contract":"","reason":"The release preserves supported behavior."}`)
	}))
	defer server.Close()
	repository := releaseEvidenceRepository(testInstance)
	noise := strings.Repeat("Routine maintenance preserves the supported public behavior.\n", 60000)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repository, "a-maintenance.txt"), []byte(noise), 0o644))
	runGit(testInstance, repository, "add", ".")
	runGit(testInstance, repository, "commit", "-m", "update maintenance text")
	configuration := releaseEvidenceConfiguration(testInstance, server.URL)
	output, runError := runBinaryIntegrationCommand(testInstance, binary, repository, map[string]string{}, time.Minute,
		[]string{"--config", configuration, "release", "next", "semver", "--format", "json"})
	require.Error(testInstance, runError, output)
	require.Contains(testInstance, output, "provider call limit")
	require.NotContains(testInstance, output, `"next_version"`)
	mutex.Lock()
	actualCalls := calls
	mutex.Unlock()
	require.Zero(testInstance, actualCalls)
	require.Contains(testInstance, string(releaseEvidenceRecord(testInstance, repository)), "provider call limit")
}

func releaseEvidenceConfiguration(testInstance *testing.T, serverURL string) string {
	testInstance.Helper()
	configuration := filepath.Join(testInstance.TempDir(), "config.yml")
	content := fmt.Sprintf(`common:
  log_level: error
  log_format: console
github:
  credential: ""
llm:
  openai:
    priority: 2
    model: gpt-5.6-terra
    base_url: https://api.openai.com/v1
    credential: ""
  llm_proxy:
    priority: 1
    provider: meta
    model: muse-spark-1.1
    base_url: %q
    credential: proxy-secret
  max_completion_tokens: 1200
  effort: high
  timeout_seconds: 5
operations: []
workflow: []
`, serverURL)
	require.NoError(testInstance, os.WriteFile(configuration, []byte(content), 0o600))
	return configuration
}

func releaseEvidenceRecord(testInstance *testing.T, repository string) []byte {
	testInstance.Helper()
	records, err := filepath.Glob(filepath.Join(repository, ".git", "gix", "release-decisions", "*.json"))
	require.NoError(testInstance, err)
	require.Len(testInstance, records, 1)
	content, err := os.ReadFile(records[0])
	require.NoError(testInstance, err)
	require.True(testInstance, json.Valid(content))
	return content
}
