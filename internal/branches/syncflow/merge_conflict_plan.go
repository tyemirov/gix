package syncflow

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"strconv"
	"strings"
)

const (
	mergeConflictPlanMarkerSize       = 32
	mergeConflictPlanContextLineLimit = 8
	mergeConflictPlanContextByteLimit = 4096
	mergeConflictPlanTargetLabel      = "TARGET"
	mergeConflictPlanBaseLabel        = "BASE"
	mergeConflictPlanIncomingLabel    = "INCOMING"
)

type mergeConflictStage struct {
	Mode     string
	ObjectID string
	Content  string
	Present  bool
}

type mergeConflictWorktreeSnapshot struct {
	Content     string
	Permissions os.FileMode
	Present     bool
}

type mergeConflictHunk struct {
	ID            string
	Path          string
	Base          string
	Target        string
	Incoming      string
	ContextBefore string
	ContextAfter  string
}

type mergeConflictPlanPart struct {
	Content string
	HunkID  string
}

type mergeConflictPlan struct {
	Path  string
	Parts []mergeConflictPlanPart
	Hunks []mergeConflictHunk
}

type mergeConflictHunkResponse struct {
	HunkID  string `json:"hunk_id"`
	Content string `json:"content"`
}

func newMergeConflictPlan(path string, diff3Content string) (mergeConflictPlan, error) {
	startMarker := mergeConflictPlanMarker("<", mergeConflictPlanTargetLabel)
	baseMarker := mergeConflictPlanMarker("|", mergeConflictPlanBaseLabel)
	separatorMarker := strings.Repeat("=", mergeConflictPlanMarkerSize)
	endMarker := mergeConflictPlanMarker(">", mergeConflictPlanIncomingLabel)

	plan := mergeConflictPlan{Path: path}
	var stableRegion strings.Builder
	var targetRegion strings.Builder
	var baseRegion strings.Builder
	var incomingRegion strings.Builder
	state := mergeConflictMarkerStateOutside

	for _, line := range mergeConflictLines(diff3Content) {
		lineWithoutTerminator := mergeConflictLineWithoutTerminator(line)
		switch state {
		case mergeConflictMarkerStateOutside:
			if lineWithoutTerminator != startMarker {
				stableRegion.WriteString(line)
				continue
			}
			plan.Parts = append(plan.Parts, mergeConflictPlanPart{Content: stableRegion.String()})
			stableRegion.Reset()
			targetRegion.Reset()
			baseRegion.Reset()
			incomingRegion.Reset()
			state = mergeConflictMarkerStateOurs
		case mergeConflictMarkerStateOurs:
			if lineWithoutTerminator == baseMarker {
				state = mergeConflictMarkerStateBase
				continue
			}
			if mergeConflictPlanLineIsMarker(lineWithoutTerminator) {
				return mergeConflictPlan{}, fmt.Errorf("invalid target marker sequence in %s", path)
			}
			targetRegion.WriteString(line)
		case mergeConflictMarkerStateBase:
			if lineWithoutTerminator == separatorMarker {
				state = mergeConflictMarkerStateTheirs
				continue
			}
			if mergeConflictPlanLineIsMarker(lineWithoutTerminator) {
				return mergeConflictPlan{}, fmt.Errorf("invalid base marker sequence in %s", path)
			}
			baseRegion.WriteString(line)
		case mergeConflictMarkerStateTheirs:
			if lineWithoutTerminator == endMarker {
				hunkIndex := len(plan.Hunks)
				hunk := mergeConflictHunk{
					ID:       mergeConflictHunkID(path, hunkIndex, baseRegion.String(), targetRegion.String(), incomingRegion.String()),
					Path:     path,
					Base:     baseRegion.String(),
					Target:   targetRegion.String(),
					Incoming: incomingRegion.String(),
				}
				plan.Hunks = append(plan.Hunks, hunk)
				plan.Parts = append(plan.Parts, mergeConflictPlanPart{HunkID: hunk.ID})
				state = mergeConflictMarkerStateOutside
				continue
			}
			if mergeConflictPlanLineIsMarker(lineWithoutTerminator) {
				return mergeConflictPlan{}, fmt.Errorf("invalid incoming marker sequence in %s", path)
			}
			incomingRegion.WriteString(line)
		}
	}

	if state != mergeConflictMarkerStateOutside {
		return mergeConflictPlan{}, fmt.Errorf("unterminated diff3 conflict in %s", path)
	}
	plan.Parts = append(plan.Parts, mergeConflictPlanPart{Content: stableRegion.String()})
	plan.populateHunkContexts()
	return plan, nil
}

func mergeConflictPlanMarker(character string, label string) string {
	return strings.Repeat(character, mergeConflictPlanMarkerSize) + " " + label
}

func mergeConflictPlanLineIsMarker(line string) bool {
	return strings.HasPrefix(line, strings.Repeat("<", mergeConflictPlanMarkerSize)) ||
		strings.HasPrefix(line, strings.Repeat("|", mergeConflictPlanMarkerSize)) ||
		strings.HasPrefix(line, strings.Repeat("=", mergeConflictPlanMarkerSize)) ||
		strings.HasPrefix(line, strings.Repeat(">", mergeConflictPlanMarkerSize))
}

func mergeConflictLineWithoutTerminator(line string) string {
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
}

func mergeConflictHunkID(path string, hunkIndex int, base string, target string, incoming string) string {
	digest := sha256.Sum256([]byte(path + "\x00" + strconv.Itoa(hunkIndex) + "\x00" + base + "\x00" + target + "\x00" + incoming))
	return fmt.Sprintf("%x", digest)
}

func (plan *mergeConflictPlan) populateHunkContexts() {
	hunkByID := make(map[string]int, len(plan.Hunks))
	for hunkIndex := range plan.Hunks {
		hunkByID[plan.Hunks[hunkIndex].ID] = hunkIndex
	}
	for partIndex := range plan.Parts {
		part := plan.Parts[partIndex]
		hunkIndex, exists := hunkByID[part.HunkID]
		if !exists {
			continue
		}
		if partIndex > 0 {
			plan.Hunks[hunkIndex].ContextBefore = mergeConflictTailContext(plan.Parts[partIndex-1].Content)
		}
		if partIndex+1 < len(plan.Parts) {
			plan.Hunks[hunkIndex].ContextAfter = mergeConflictHeadContext(plan.Parts[partIndex+1].Content)
		}
	}
}

func mergeConflictTailContext(content string) string {
	lines := mergeConflictLines(content)
	if len(lines) > mergeConflictPlanContextLineLimit {
		lines = lines[len(lines)-mergeConflictPlanContextLineLimit:]
	}
	contextValue := strings.Join(lines, "")
	if len(contextValue) <= mergeConflictPlanContextByteLimit {
		return contextValue
	}
	return contextValue[len(contextValue)-mergeConflictPlanContextByteLimit:]
}

func mergeConflictHeadContext(content string) string {
	lines := mergeConflictLines(content)
	if len(lines) > mergeConflictPlanContextLineLimit {
		lines = lines[:mergeConflictPlanContextLineLimit]
	}
	contextValue := strings.Join(lines, "")
	if len(contextValue) <= mergeConflictPlanContextByteLimit {
		return contextValue
	}
	return contextValue[:mergeConflictPlanContextByteLimit]
}

func resolveDeterministicConflictHunk(hunk mergeConflictHunk) (string, bool) {
	switch {
	case hunk.Target == hunk.Incoming:
		return hunk.Target, true
	case hunk.Target == hunk.Base:
		return hunk.Incoming, true
	case hunk.Incoming == hunk.Base:
		return hunk.Target, true
	case hunk.Base == "":
		resolution := mergeConflictCombineInsertions(hunk.Target, hunk.Incoming)
		if !mergeConflictHunkIsDeterministicIssueMerge(hunk, resolution) {
			return "", false
		}
		return resolution, true
	}

	targetBaseCount := strings.Count(hunk.Target, hunk.Base)
	incomingBaseCount := strings.Count(hunk.Incoming, hunk.Base)
	var resolution string
	switch {
	case targetBaseCount == 1 && incomingBaseCount == 1:
		targetPrefix, targetSuffix := mergeConflictSplitAroundBase(hunk.Target, hunk.Base)
		incomingPrefix, incomingSuffix := mergeConflictSplitAroundBase(hunk.Incoming, hunk.Base)
		resolution = mergeConflictCombineInsertions(targetPrefix, incomingPrefix) +
			hunk.Base +
			mergeConflictCombineInsertions(targetSuffix, incomingSuffix)
	case targetBaseCount == 1 && incomingBaseCount == 0:
		targetPrefix, targetSuffix := mergeConflictSplitAroundBase(hunk.Target, hunk.Base)
		resolution = targetPrefix + hunk.Incoming + targetSuffix
	case targetBaseCount == 0 && incomingBaseCount == 1:
		incomingPrefix, incomingSuffix := mergeConflictSplitAroundBase(hunk.Incoming, hunk.Base)
		resolution = incomingPrefix + hunk.Target + incomingSuffix
	default:
		return "", false
	}
	if !mergeConflictHunkIsDeterministicIssueMerge(hunk, resolution) {
		return "", false
	}
	if !mergeConflictHunkPreservesIntent(resolution, hunk, true) {
		return "", false
	}
	return resolution, true
}

func mergeConflictSplitAroundBase(content string, base string) (string, string) {
	baseOffset := strings.Index(content, base)
	return content[:baseOffset], content[baseOffset+len(base):]
}

func mergeConflictCombineInsertions(target string, incoming string) string {
	switch {
	case target == "":
		return incoming
	case incoming == "":
		return target
	case target == incoming, strings.Contains(target, incoming):
		return target
	case strings.Contains(incoming, target):
		return incoming
	default:
		return target + incoming
	}
}

type mergeConflictIssueRecord struct {
	Content string
	Present bool
}

func mergeConflictHunkIsDeterministicIssueMerge(hunk mergeConflictHunk, resolution string) bool {
	if pathpkg.Base(pathpkg.Clean(hunk.Path)) != "ISSUES.md" {
		return false
	}
	baseRecords, baseValid := mergeConflictIssueRecords(hunk.Base)
	targetRecords, targetValid := mergeConflictIssueRecords(hunk.Target)
	incomingRecords, incomingValid := mergeConflictIssueRecords(hunk.Incoming)
	resolutionRecords, resolutionValid := mergeConflictIssueRecords(resolution)
	if !baseValid || !targetValid || !incomingValid || !resolutionValid {
		return false
	}

	recordIDs := make(map[string]struct{}, len(baseRecords)+len(targetRecords)+len(incomingRecords))
	for recordID := range baseRecords {
		recordIDs[recordID] = struct{}{}
	}
	for recordID := range targetRecords {
		recordIDs[recordID] = struct{}{}
	}
	for recordID := range incomingRecords {
		recordIDs[recordID] = struct{}{}
	}
	expectedRecords := make(map[string]mergeConflictIssueRecord, len(recordIDs))
	for recordID := range recordIDs {
		expectedRecord, deterministic := mergeConflictThreeWayIssueRecord(
			baseRecords[recordID],
			targetRecords[recordID],
			incomingRecords[recordID],
		)
		if !deterministic {
			return false
		}
		if expectedRecord.Present {
			expectedRecords[recordID] = expectedRecord
		}
	}
	if len(expectedRecords) != len(resolutionRecords) {
		return false
	}
	for recordID, expectedRecord := range expectedRecords {
		if resolutionRecords[recordID] != expectedRecord {
			return false
		}
	}
	return true
}

func mergeConflictThreeWayIssueRecord(
	base mergeConflictIssueRecord,
	target mergeConflictIssueRecord,
	incoming mergeConflictIssueRecord,
) (mergeConflictIssueRecord, bool) {
	switch {
	case target == incoming:
		return target, true
	case target == base:
		return incoming, true
	case incoming == base:
		return target, true
	default:
		return mergeConflictIssueRecord{}, false
	}
}

func mergeConflictIssueRecords(content string) (map[string]mergeConflictIssueRecord, bool) {
	records := map[string]mergeConflictIssueRecord{}
	var currentRecord strings.Builder
	var leadingContent strings.Builder
	currentRecordID := ""
	for _, line := range mergeConflictLines(content) {
		recordID, recordStart := mergeConflictIssueRecordID(line)
		if !recordStart {
			if currentRecordID == "" {
				leadingContent.WriteString(line)
			} else {
				currentRecord.WriteString(line)
			}
			continue
		}
		if currentRecordID != "" {
			records[currentRecordID] = mergeConflictIssueRecord{Content: currentRecord.String(), Present: true}
		} else if strings.TrimSpace(leadingContent.String()) != "" {
			return nil, false
		}
		if _, duplicate := records[recordID]; duplicate {
			return nil, false
		}
		currentRecordID = recordID
		currentRecord.Reset()
		currentRecord.WriteString(line)
	}
	if currentRecordID == "" {
		return records, strings.TrimSpace(leadingContent.String()) == ""
	}
	records[currentRecordID] = mergeConflictIssueRecord{Content: currentRecord.String(), Present: true}
	return records, true
}

func mergeConflictIssueRecordID(line string) (string, bool) {
	line = mergeConflictLineWithoutTerminator(line)
	if len(line) < 10 ||
		!strings.HasPrefix(line, "- [") ||
		!strings.ContainsRune(" x-!", rune(line[3])) ||
		line[4] != ']' ||
		line[5] != ' ' ||
		line[6] != '[' {
		return "", false
	}
	identifierEnd := strings.IndexByte(line[7:], ']')
	if identifierEnd < 0 {
		return "", false
	}
	identifierClose := 7 + identifierEnd
	if identifierClose+1 < len(line) && line[identifierClose+1] != ' ' {
		return "", false
	}
	identifier := line[7:identifierClose]
	if len(identifier) < 2 || identifier[0] < 'A' || identifier[0] > 'Z' {
		return "", false
	}
	digitObserved := false
	for characterIndex := 1; characterIndex < len(identifier); characterIndex++ {
		character := identifier[characterIndex]
		switch {
		case character >= '0' && character <= '9':
			digitObserved = true
		case digitObserved && character >= 'A' && character <= 'Z':
		default:
			return "", false
		}
	}
	return identifier, digitObserved
}

func mergeConflictHunkPreservesIntent(resolution string, hunk mergeConflictHunk, requireIncoming bool) bool {
	if !mergeConflictResolutionContainsRegions(resolution, mergeConflictIntentFragments(hunk.Base, hunk.Target)) {
		return false
	}
	if requireIncoming && !mergeConflictResolutionContainsRegions(resolution, mergeConflictIntentFragments(hunk.Base, hunk.Incoming)) {
		return false
	}
	return true
}

func mergeConflictIntentFragments(base string, side string) []string {
	if side == base {
		return nil
	}
	if base == "" {
		if side == "" {
			return nil
		}
		return []string{side}
	}
	if strings.Count(side, base) == 1 {
		prefix, suffix := mergeConflictSplitAroundBase(side, base)
		fragments := make([]string, 0, 2)
		if prefix != "" {
			fragments = append(fragments, prefix)
		}
		if suffix != "" {
			fragments = append(fragments, suffix)
		}
		return fragments
	}
	if side == "" {
		return nil
	}
	return []string{side}
}

func (plan mergeConflictPlan) render(resolutions map[string]string) (string, error) {
	var content strings.Builder
	for partIndex := range plan.Parts {
		part := plan.Parts[partIndex]
		if part.HunkID == "" {
			content.WriteString(part.Content)
			continue
		}
		resolution, exists := resolutions[part.HunkID]
		if !exists {
			return "", fmt.Errorf("missing resolution for hunk %s in %s", part.HunkID, plan.Path)
		}
		content.WriteString(resolution)
	}
	return content.String(), nil
}

func parseMergeConflictHunkResponse(response string) (mergeConflictHunkResponse, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(response)))
	decoder.DisallowUnknownFields()
	var parsed mergeConflictHunkResponse
	if decodeErr := decoder.Decode(&parsed); decodeErr != nil {
		return mergeConflictHunkResponse{}, fmt.Errorf("decode structured hunk resolution: %w", decodeErr)
	}
	var trailing any
	if trailingErr := decoder.Decode(&trailing); !errors.Is(trailingErr, io.EOF) {
		return mergeConflictHunkResponse{}, errors.New("decode structured hunk resolution: trailing JSON content")
	}
	return parsed, nil
}

func validateMergeConflictHunkResponse(options mergeConflictResolutionOptions, hunk mergeConflictHunk, response mergeConflictHunkResponse) (string, error) {
	if response.HunkID != hunk.ID {
		return "", fmt.Errorf("hunk response id %q does not match %q", response.HunkID, hunk.ID)
	}
	resolvedContent := normalizeMergeConflictHunkTerminator(response.Content, hunk)
	if containsConflictMarker(resolvedContent) {
		return "", fmt.Errorf("hunk %s contains conflict markers", hunk.ID)
	}
	requireIncoming := options.Mode == mergeConflictResolutionModeStash
	if !mergeConflictHunkPreservesIntent(resolvedContent, hunk, requireIncoming) {
		if requireIncoming {
			return "", fmt.Errorf("hunk %s does not preserve target and stashed intent", hunk.ID)
		}
		return "", fmt.Errorf("hunk %s does not preserve target intent", hunk.ID)
	}
	return resolvedContent, nil
}

func normalizeMergeConflictHunkTerminator(resolution string, hunk mergeConflictHunk) string {
	if resolution == "" || strings.HasSuffix(resolution, "\n") {
		return resolution
	}
	for _, candidate := range []string{hunk.Target, hunk.Incoming, hunk.Base} {
		switch {
		case strings.HasSuffix(candidate, "\r\n"):
			return resolution + "\r\n"
		case strings.HasSuffix(candidate, "\n"):
			return resolution + "\n"
		}
	}
	return resolution
}
