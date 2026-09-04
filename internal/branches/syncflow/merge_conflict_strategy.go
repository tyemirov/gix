package syncflow

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	mergeConflictTokenDiffMatrixMaximumCells        = 4_000_000
	mergeConflictReplacementIntentContextTokenCount = 3
)

type mergeConflictDeterministicResolution struct {
	Content               string
	Strategy              string
	RequiresSemanticAudit bool
}

type mergeConflictTokenEdit struct {
	Start       int
	End         int
	Replacement string
}

type mergeConflictTokenMatch struct {
	BaseIndex    int
	VariantIndex int
}

type mergeConflictConcurrentInsertionAnalysis struct {
	Candidate    string
	Alternatives []string
}

func deterministicMergeConflictRegionResolution(region mergeConflictRegion) (mergeConflictDeterministicResolution, bool) {
	switch {
	case region.Ours == region.Theirs:
		return mergeConflictDeterministicResolution{
			Content:  region.Ours,
			Strategy: "identical sides",
		}, true
	case region.BasePresent && region.Ours == region.Base:
		return mergeConflictDeterministicResolution{
			Content:  region.Theirs,
			Strategy: "incoming-only change",
		}, true
	case region.BasePresent && region.Theirs == region.Base:
		return mergeConflictDeterministicResolution{
			Content:  region.Ours,
			Strategy: "local-only change",
		}, true
	case region.BasePresent && region.Base == "":
		if issueAnalysis, related := analyzeMergeConflictIssueInsertions(region.Ours, region.Theirs); related {
			return mergeConflictDeterministicResolution{Content: issueAnalysis.candidate(), Strategy: mergeConflictIssueInsertionStrategy, RequiresSemanticAudit: true}, true
		}
		if insertionAnalysis, overlapping := analyzeMergeConflictConcurrentInsertions(region.Ours, region.Theirs); overlapping {
			return mergeConflictDeterministicResolution{
				Content:               insertionAnalysis.Candidate,
				Strategy:              "overlapping concurrent insertions",
				RequiresSemanticAudit: true,
			}, true
		}
		return mergeConflictDeterministicResolution{
			Content:               region.Ours + region.Theirs,
			Strategy:              "concurrent insertions",
			RequiresSemanticAudit: true,
		}, true
	}

	return mergeConflictTokenEditResolution(region.Base, region.Ours, region.Theirs)
}

func analyzeMergeConflictConcurrentInsertions(ours string, theirs string) (mergeConflictConcurrentInsertionAnalysis, bool) {
	oursWordTokens := mergeConflictWordTokens(mergeConflictTokens(ours))
	theirsWordTokens := mergeConflictWordTokens(mergeConflictTokens(theirs))
	if len(oursWordTokens) == 0 || len(theirsWordTokens) == 0 {
		return mergeConflictConcurrentInsertionAnalysis{}, false
	}
	switch {
	case len(oursWordTokens) <= len(theirsWordTokens) && mergeConflictTokenSequenceContained(oursWordTokens, theirsWordTokens):
		if len(oursWordTokens) == len(theirsWordTokens) {
			return mergeConflictConcurrentInsertionAnalysis{
				Candidate:    ours,
				Alternatives: []string{ours, theirs},
			}, true
		}
		return mergeConflictConcurrentInsertionAnalysis{
			Candidate:    theirs,
			Alternatives: []string{theirs},
		}, true
	case len(theirsWordTokens) < len(oursWordTokens) && mergeConflictTokenSequenceContained(theirsWordTokens, oursWordTokens):
		return mergeConflictConcurrentInsertionAnalysis{
			Candidate:    ours,
			Alternatives: []string{ours},
		}, true
	default:
		return mergeConflictConcurrentInsertionAnalysis{}, false
	}
}

func mergeConflictWordTokens(tokens []string) []string {
	var wordTokens []string
	for _, token := range tokens {
		wordTokens = append(wordTokens, strings.FieldsFunc(token, func(character rune) bool {
			return !unicode.IsLetter(character) && !unicode.IsNumber(character) && character != '_'
		})...)
	}
	return wordTokens
}

func mergeConflictTokenSequenceContained(subsequence []string, sequence []string) bool {
	if len(subsequence) > len(sequence) {
		return false
	}
	subsequenceIndex := 0
	for _, token := range sequence {
		if subsequenceIndex == len(subsequence) {
			break
		}
		if token == subsequence[subsequenceIndex] {
			subsequenceIndex++
		}
	}
	return subsequenceIndex == len(subsequence)
}

func mergeConflictTokenEditResolution(base string, ours string, theirs string) (mergeConflictDeterministicResolution, bool) {
	baseTokens := mergeConflictTokens(base)
	oursTokens := mergeConflictTokens(ours)
	theirsTokens := mergeConflictTokens(theirs)
	oursEdits := mergeConflictTokenEdits(baseTokens, oursTokens)
	theirsEdits := mergeConflictTokenEdits(baseTokens, theirsTokens)

	mergedEdits, editsCompatible := mergeConflictCompatibleTokenEdits(oursEdits, theirsEdits)
	if editsCompatible {
		mergedContent, merged := applyMergeConflictTokenEdits(baseTokens, mergedEdits)
		if !merged {
			return mergeConflictDeterministicResolution{}, false
		}
		return mergeConflictDeterministicResolution{
			Content:               mergedContent,
			Strategy:              "compatible token edits",
			RequiresSemanticAudit: true,
		}, true
	}

	candidate, candidateAvailable := mergeConflictVariantWithCompatibleOtherEdits(baseTokens, oursEdits, theirsEdits)
	if !candidateAvailable {
		return mergeConflictDeterministicResolution{}, false
	}
	return mergeConflictDeterministicResolution{
		Content:               candidate,
		Strategy:              "local replacement alternative",
		RequiresSemanticAudit: true,
	}, true
}

func mergeConflictTokens(value string) []string {
	if value == "" {
		return nil
	}

	tokens := make([]string, 0, len(value))
	tokenStart := 0
	offset := 0
	currentClass := mergeConflictTokenClass(value, offset)
	for offset < len(value) {
		if literalEnd := mergeConflictCodeSpanEnd(value, offset); literalEnd > offset {
			if tokenStart < offset {
				tokens = append(tokens, value[tokenStart:offset])
			}
			tokens = append(tokens, value[offset:literalEnd])
			offset = literalEnd
			tokenStart = literalEnd
			if offset < len(value) {
				currentClass = mergeConflictTokenClass(value, offset)
			}
			continue
		}
		_, runeSize := utf8.DecodeRuneInString(value[offset:])
		nextOffset := offset + runeSize
		if nextOffset == len(value) {
			tokens = append(tokens, value[tokenStart:nextOffset])
			break
		}
		nextClass := mergeConflictTokenClass(value, nextOffset)
		if nextClass != currentClass || value[nextOffset] == '`' {
			tokens = append(tokens, value[tokenStart:nextOffset])
			tokenStart = nextOffset
			currentClass = nextClass
		}
		offset = nextOffset
	}
	return tokens
}

// Keep backtick code spans atomic so prose edits cannot attach to words inside code.
func mergeConflictCodeSpanEnd(value string, offset int) int {
	if value[offset] != '`' {
		return offset
	}
	delimiterEnd := offset
	for delimiterEnd < len(value) && value[delimiterEnd] == '`' {
		delimiterEnd++
	}
	delimiter := value[offset:delimiterEnd]
	searchOffset := delimiterEnd
	for searchOffset < len(value) {
		relativeStart := strings.IndexByte(value[searchOffset:], '`')
		if relativeStart < 0 {
			return delimiterEnd
		}
		closingStart := searchOffset + relativeStart
		closingEnd := closingStart
		for closingEnd < len(value) && value[closingEnd] == '`' {
			closingEnd++
		}
		if closingEnd-closingStart == len(delimiter) {
			return closingEnd
		}
		searchOffset = closingEnd
	}
	return delimiterEnd
}

func mergeConflictTokenClass(value string, offset int) uint8 {
	currentRune, _ := utf8.DecodeRuneInString(value[offset:])
	switch {
	case unicode.IsLetter(currentRune), unicode.IsNumber(currentRune), currentRune == '_':
		return 1
	case unicode.IsSpace(currentRune):
		return 2
	default:
		return 3
	}
}

func mergeConflictTokenEdits(baseTokens []string, variantTokens []string) []mergeConflictTokenEdit {
	rowCount := len(baseTokens) + 1
	columnCount := len(variantTokens) + 1
	if !mergeConflictTokenEditsRequireLinearMemory(baseTokens, variantTokens) {
		return mergeConflictMatrixTokenEdits(baseTokens, variantTokens, rowCount*columnCount, columnCount)
	}
	return mergeConflictLinearTokenEdits(baseTokens, variantTokens)
}

func mergeConflictTokenEditsRequireLinearMemory(baseTokens []string, variantTokens []string) bool {
	return len(baseTokens)+1 > mergeConflictTokenDiffMatrixMaximumCells/(len(variantTokens)+1)
}

func mergeConflictMatrixTokenEdits(baseTokens []string, variantTokens []string, cellCount int, columnCount int) []mergeConflictTokenEdit {
	longestSubsequences := make([]int, cellCount)
	for baseIndex := len(baseTokens) - 1; baseIndex >= 0; baseIndex-- {
		for variantIndex := len(variantTokens) - 1; variantIndex >= 0; variantIndex-- {
			cellIndex := baseIndex*columnCount + variantIndex
			if baseTokens[baseIndex] == variantTokens[variantIndex] {
				longestSubsequences[cellIndex] = 1 + longestSubsequences[(baseIndex+1)*columnCount+variantIndex+1]
				continue
			}
			skipBase := longestSubsequences[(baseIndex+1)*columnCount+variantIndex]
			skipVariant := longestSubsequences[baseIndex*columnCount+variantIndex+1]
			if skipBase >= skipVariant {
				longestSubsequences[cellIndex] = skipBase
			} else {
				longestSubsequences[cellIndex] = skipVariant
			}
		}
	}

	edits := make([]mergeConflictTokenEdit, 0)
	baseIndex := 0
	variantIndex := 0
	unmatchedBaseStart := 0
	unmatchedVariantStart := 0
	for baseIndex < len(baseTokens) && variantIndex < len(variantTokens) {
		if baseTokens[baseIndex] == variantTokens[variantIndex] {
			edits = appendMergeConflictTokenEdit(edits, unmatchedBaseStart, baseIndex, variantTokens[unmatchedVariantStart:variantIndex])
			baseIndex++
			variantIndex++
			unmatchedBaseStart = baseIndex
			unmatchedVariantStart = variantIndex
			continue
		}
		skipBase := longestSubsequences[(baseIndex+1)*columnCount+variantIndex]
		skipVariant := longestSubsequences[baseIndex*columnCount+variantIndex+1]
		if skipBase >= skipVariant {
			baseIndex++
		} else {
			variantIndex++
		}
	}
	edits = appendMergeConflictTokenEdit(edits, unmatchedBaseStart, len(baseTokens), variantTokens[unmatchedVariantStart:])
	return edits
}

func mergeConflictLinearTokenEdits(baseTokens []string, variantTokens []string) []mergeConflictTokenEdit {
	commonPrefixLength := 0
	for commonPrefixLength < len(baseTokens) &&
		commonPrefixLength < len(variantTokens) &&
		baseTokens[commonPrefixLength] == variantTokens[commonPrefixLength] {
		commonPrefixLength++
	}
	commonSuffixLength := 0
	for commonSuffixLength < len(baseTokens) &&
		commonSuffixLength < len(variantTokens) &&
		baseTokens[len(baseTokens)-commonSuffixLength-1] == variantTokens[len(variantTokens)-commonSuffixLength-1] {
		commonSuffixLength++
	}
	shorterTokenCount := min(len(baseTokens), len(variantTokens))
	if commonPrefixLength+commonSuffixLength > shorterTokenCount {
		if commonPrefixLength >= commonSuffixLength {
			commonSuffixLength = 0
		} else {
			commonPrefixLength = 0
		}
	}
	baseMiddleEnd := len(baseTokens) - commonSuffixLength
	variantMiddleEnd := len(variantTokens) - commonSuffixLength
	matches := mergeConflictTokenMatches(
		baseTokens[commonPrefixLength:baseMiddleEnd],
		variantTokens[commonPrefixLength:variantMiddleEnd],
		commonPrefixLength,
		commonPrefixLength,
	)
	edits := make([]mergeConflictTokenEdit, 0)
	baseCursor := commonPrefixLength
	variantCursor := commonPrefixLength
	for _, match := range matches {
		edits = appendMergeConflictTokenEdit(edits, baseCursor, match.BaseIndex, variantTokens[variantCursor:match.VariantIndex])
		baseCursor = match.BaseIndex + 1
		variantCursor = match.VariantIndex + 1
	}
	edits = appendMergeConflictTokenEdit(edits, baseCursor, baseMiddleEnd, variantTokens[variantCursor:variantMiddleEnd])
	return edits
}

func mergeConflictTokenMatches(baseTokens []string, variantTokens []string, baseOffset int, variantOffset int) []mergeConflictTokenMatch {
	if len(baseTokens) == 0 || len(variantTokens) == 0 {
		return nil
	}
	if len(baseTokens) == 1 {
		for variantIndex, variantToken := range variantTokens {
			if baseTokens[0] == variantToken {
				return []mergeConflictTokenMatch{{BaseIndex: baseOffset, VariantIndex: variantOffset + variantIndex}}
			}
		}
		return nil
	}
	if len(variantTokens) == 1 {
		for baseIndex, baseToken := range baseTokens {
			if baseToken == variantTokens[0] {
				return []mergeConflictTokenMatch{{BaseIndex: baseOffset + baseIndex, VariantIndex: variantOffset}}
			}
		}
		return nil
	}

	baseMiddle := len(baseTokens) / 2
	variantMiddle := func() int {
		prefixLengths := mergeConflictTokenPrefixLCSLengths(baseTokens[:baseMiddle], variantTokens)
		suffixLengths := mergeConflictTokenSuffixLCSLengths(baseTokens[baseMiddle:], variantTokens)
		selectedMiddle := 0
		longestLength := -1
		for variantIndex := 0; variantIndex <= len(variantTokens); variantIndex++ {
			candidateLength := prefixLengths[variantIndex] + suffixLengths[variantIndex]
			if candidateLength > longestLength {
				selectedMiddle = variantIndex
				longestLength = candidateLength
			}
		}
		return selectedMiddle
	}()

	leftMatches := mergeConflictTokenMatches(baseTokens[:baseMiddle], variantTokens[:variantMiddle], baseOffset, variantOffset)
	rightMatches := mergeConflictTokenMatches(baseTokens[baseMiddle:], variantTokens[variantMiddle:], baseOffset+baseMiddle, variantOffset+variantMiddle)
	return append(leftMatches, rightMatches...)
}

func mergeConflictTokenPrefixLCSLengths(baseTokens []string, variantTokens []string) []int {
	lengths := make([]int, len(variantTokens)+1)
	for _, baseToken := range baseTokens {
		previousDiagonal := 0
		for variantIndex, variantToken := range variantTokens {
			previousLength := lengths[variantIndex+1]
			if baseToken == variantToken {
				lengths[variantIndex+1] = previousDiagonal + 1
			} else if lengths[variantIndex] > previousLength {
				lengths[variantIndex+1] = lengths[variantIndex]
			}
			previousDiagonal = previousLength
		}
	}
	return lengths
}

func mergeConflictTokenSuffixLCSLengths(baseTokens []string, variantTokens []string) []int {
	lengths := make([]int, len(variantTokens)+1)
	for baseIndex := len(baseTokens) - 1; baseIndex >= 0; baseIndex-- {
		previousDiagonal := 0
		for variantIndex := len(variantTokens) - 1; variantIndex >= 0; variantIndex-- {
			previousLength := lengths[variantIndex]
			if baseTokens[baseIndex] == variantTokens[variantIndex] {
				lengths[variantIndex] = previousDiagonal + 1
			} else if lengths[variantIndex+1] > previousLength {
				lengths[variantIndex] = lengths[variantIndex+1]
			}
			previousDiagonal = previousLength
		}
	}
	return lengths
}

func appendMergeConflictTokenEdit(edits []mergeConflictTokenEdit, start int, end int, replacementTokens []string) []mergeConflictTokenEdit {
	replacement := strings.Join(replacementTokens, "")
	if start == end && replacement == "" {
		return edits
	}
	return append(edits, mergeConflictTokenEdit{
		Start:       start,
		End:         end,
		Replacement: replacement,
	})
}

func mergeConflictCompatibleTokenEdits(ours []mergeConflictTokenEdit, theirs []mergeConflictTokenEdit) ([]mergeConflictTokenEdit, bool) {
	merged := append(append(make([]mergeConflictTokenEdit, 0, len(ours)+len(theirs)), ours...), theirs...)
	sort.SliceStable(merged, func(leftIndex int, rightIndex int) bool {
		if merged[leftIndex].Start != merged[rightIndex].Start {
			return merged[leftIndex].Start < merged[rightIndex].Start
		}
		if merged[leftIndex].End != merged[rightIndex].End {
			return merged[leftIndex].End < merged[rightIndex].End
		}
		if merged[leftIndex].Start == merged[leftIndex].End {
			return merged[leftIndex].Replacement < merged[rightIndex].Replacement
		}
		return false
	})

	deduplicated := make([]mergeConflictTokenEdit, 0, len(merged))
	for _, candidate := range merged {
		duplicate := false
		for _, existing := range deduplicated {
			if mergeConflictTokenEditsConflict(existing, candidate) {
				return nil, false
			}
			if existing.Start == candidate.Start &&
				existing.End == candidate.End &&
				existing.Replacement == candidate.Replacement {
				duplicate = true
				break
			}
		}
		if !duplicate {
			deduplicated = append(deduplicated, candidate)
		}
	}
	return deduplicated, true
}

func mergeConflictTokenEditsConflict(left mergeConflictTokenEdit, right mergeConflictTokenEdit) bool {
	leftInsertion := left.Start == left.End
	rightInsertion := right.Start == right.End
	if leftInsertion && rightInsertion {
		return false
	}
	if left.Start == right.Start && left.End == right.End {
		return left.Replacement != right.Replacement
	}
	switch {
	case leftInsertion:
		return right.Start < left.Start && left.Start < right.End
	case rightInsertion:
		return left.Start < right.Start && right.Start < left.End
	default:
		return left.Start < right.End && right.Start < left.End
	}
}

func applyMergeConflictTokenEdits(baseTokens []string, edits []mergeConflictTokenEdit) (string, bool) {
	var resolved strings.Builder
	baseCursor := 0
	for _, edit := range edits {
		if edit.Start < baseCursor || edit.End < edit.Start || edit.End > len(baseTokens) {
			return "", false
		}
		resolved.WriteString(strings.Join(baseTokens[baseCursor:edit.Start], ""))
		resolved.WriteString(edit.Replacement)
		baseCursor = edit.End
	}
	resolved.WriteString(strings.Join(baseTokens[baseCursor:], ""))
	return resolved.String(), true
}

func mergeConflictMissingRegionReplacementIntents(base string, ours string, theirs string, candidate string) ([]string, []string) {
	baseTokens := mergeConflictTokens(base)
	oursTokens := mergeConflictTokens(ours)
	theirsTokens := mergeConflictTokens(theirs)
	oursEdits := mergeConflictTokenEdits(baseTokens, oursTokens)
	theirsEdits := mergeConflictTokenEdits(baseTokens, theirsTokens)
	if mergeConflictCandidateMatchesDerivedVariant(baseTokens, oursEdits, theirsEdits, candidate) {
		return nil, nil
	}
	candidateEdits := mergeConflictTokenEdits(baseTokens, mergeConflictTokens(candidate))
	oursMissing := mergeConflictMissingReplacementIntents(base, ours, theirs, candidate, baseTokens, oursEdits, theirsEdits, candidateEdits)
	theirsMissing := mergeConflictMissingReplacementIntents(base, theirs, ours, candidate, baseTokens, theirsEdits, oursEdits, candidateEdits)
	return oursMissing, theirsMissing
}

func mergeConflictMissingReplacementIntents(
	base string,
	variant string,
	otherVariant string,
	candidate string,
	baseTokens []string,
	edits []mergeConflictTokenEdit,
	otherEdits []mergeConflictTokenEdit,
	candidateEdits []mergeConflictTokenEdit,
) []string {
	normalizedBase := mergeConflictWithoutWhitespace(base)
	normalizedVariant := mergeConflictWithoutWhitespace(variant)
	normalizedOtherVariant := mergeConflictWithoutWhitespace(otherVariant)
	normalizedCandidate := mergeConflictWithoutWhitespace(candidate)
	hasDeletionIntent := mergeConflictTokenEditsContainDeletionIntent(baseTokens, edits)
	preservesVariant := normalizedVariant != "" && strings.Contains(normalizedCandidate, normalizedVariant)
	if !hasDeletionIntent && preservesVariant {
		return nil
	}
	compatibleVariant, compatibleVariantAvailable := mergeConflictVariantWithCompatibleOtherEdits(baseTokens, edits, otherEdits)
	normalizedCompatibleVariant := mergeConflictWithoutWhitespace(compatibleVariant)
	preservesCompatibleVariant := compatibleVariantAvailable && normalizedCompatibleVariant != "" && strings.Contains(normalizedCandidate, normalizedCompatibleVariant)
	if !hasDeletionIntent && preservesCompatibleVariant {
		return nil
	}
	missingIntents := make([]string, 0, len(edits))
	for _, edit := range edits {
		normalizedReplacement := mergeConflictWithoutWhitespace(edit.Replacement)
		if normalizedReplacement == "" {
			if edit.Start == edit.End || mergeConflictWithoutWhitespace(strings.Join(baseTokens[edit.Start:edit.End], "")) == "" {
				continue
			}
			if mergeConflictCandidatePreservesDeletionIntent(edit, candidateEdits) {
				continue
			}
			if mergeConflictCandidatePreservesReplacementAlternative(
				baseTokens,
				edit,
				otherEdits,
				normalizedBase,
				normalizedOtherVariant,
				normalizedCandidate,
			) {
				continue
			}
			missingIntents = append(missingIntents, fmt.Sprintf("delete BASE token range %d:%d", edit.Start, edit.End))
			continue
		}
		// Exact source text proves replacements. Deletions still require the checks above.
		if preservesVariant || preservesCompatibleVariant {
			continue
		}
		if mergeConflictCandidatePreservesReplacementIntent(
			baseTokens,
			edit,
			normalizedBase,
			normalizedVariant,
			normalizedCandidate,
		) {
			continue
		}
		if mergeConflictCandidatePreservesReplacementAlternative(
			baseTokens,
			edit,
			otherEdits,
			normalizedBase,
			normalizedOtherVariant,
			normalizedCandidate,
		) {
			continue
		}
		missingIntents = append(missingIntents, edit.Replacement)
	}
	return missingIntents
}

func mergeConflictTokenEditsContainDeletionIntent(baseTokens []string, edits []mergeConflictTokenEdit) bool {
	for _, edit := range edits {
		if edit.Start == edit.End || mergeConflictWithoutWhitespace(edit.Replacement) != "" {
			continue
		}
		if mergeConflictWithoutWhitespace(strings.Join(baseTokens[edit.Start:edit.End], "")) != "" {
			return true
		}
	}
	return false
}

func mergeConflictCandidatePreservesDeletionIntent(deletion mergeConflictTokenEdit, candidateEdits []mergeConflictTokenEdit) bool {
	coveredUntil := deletion.Start
	for _, candidateEdit := range candidateEdits {
		if candidateEdit.Start == candidateEdit.End || candidateEdit.End <= coveredUntil {
			continue
		}
		if candidateEdit.Start > coveredUntil {
			return false
		}
		coveredUntil = candidateEdit.End
		if coveredUntil >= deletion.End {
			return true
		}
	}
	return false
}

func mergeConflictCandidateMatchesDerivedVariant(
	baseTokens []string,
	oursEdits []mergeConflictTokenEdit,
	theirsEdits []mergeConflictTokenEdit,
	candidate string,
) bool {
	normalizedCandidate := mergeConflictWithoutWhitespace(candidate)
	oursCandidate, oursCandidateAvailable := mergeConflictVariantWithCompatibleOtherEdits(baseTokens, oursEdits, theirsEdits)
	if oursCandidateAvailable && normalizedCandidate == mergeConflictWithoutWhitespace(oursCandidate) {
		return true
	}
	theirsCandidate, theirsCandidateAvailable := mergeConflictVariantWithCompatibleOtherEdits(baseTokens, theirsEdits, oursEdits)
	return theirsCandidateAvailable && normalizedCandidate == mergeConflictWithoutWhitespace(theirsCandidate)
}

func mergeConflictVariantWithCompatibleOtherEdits(baseTokens []string, variantEdits []mergeConflictTokenEdit, otherEdits []mergeConflictTokenEdit) (string, bool) {
	compatibleOtherEdits := make([]mergeConflictTokenEdit, 0, len(otherEdits))
	for _, otherEdit := range otherEdits {
		conflicts := false
		for _, variantEdit := range variantEdits {
			if mergeConflictTokenEditsConflict(variantEdit, otherEdit) {
				conflicts = true
				break
			}
		}
		if !conflicts {
			compatibleOtherEdits = append(compatibleOtherEdits, otherEdit)
		}
	}
	mergedEdits, editsCompatible := mergeConflictCompatibleTokenEdits(variantEdits, compatibleOtherEdits)
	if !editsCompatible {
		return "", false
	}
	return applyMergeConflictTokenEdits(baseTokens, mergedEdits)
}

func mergeConflictCandidatePreservesReplacementAlternative(
	baseTokens []string,
	edit mergeConflictTokenEdit,
	otherEdits []mergeConflictTokenEdit,
	normalizedBase string,
	normalizedOtherVariant string,
	normalizedCandidate string,
) bool {
	for _, otherEdit := range otherEdits {
		if edit.Start == edit.End && otherEdit.Start == otherEdit.End {
			continue
		}
		if !mergeConflictTokenEditsConflict(edit, otherEdit) {
			continue
		}
		if normalizedCandidate == normalizedOtherVariant {
			return true
		}
		normalizedReplacement := mergeConflictWithoutWhitespace(otherEdit.Replacement)
		if normalizedReplacement != "" && mergeConflictCandidatePreservesReplacementIntent(
			baseTokens,
			otherEdit,
			normalizedBase,
			normalizedOtherVariant,
			normalizedCandidate,
		) {
			return true
		}
	}
	return false
}

func mergeConflictCandidatePreservesReplacementIntent(
	baseTokens []string,
	edit mergeConflictTokenEdit,
	normalizedBase string,
	normalizedVariant string,
	normalizedCandidate string,
) bool {
	normalizedReplacement := mergeConflictWithoutWhitespace(edit.Replacement)
	baseOccurrences := strings.Count(normalizedBase, normalizedReplacement)
	variantOccurrences := strings.Count(normalizedVariant, normalizedReplacement)
	candidateOccurrences := strings.Count(normalizedCandidate, normalizedReplacement)
	if variantOccurrences > baseOccurrences && candidateOccurrences >= variantOccurrences {
		return true
	}
	return mergeConflictCandidateMatchesEditContext(baseTokens, edit, normalizedReplacement, normalizedCandidate)
}

func mergeConflictCandidateMatchesEditContext(
	baseTokens []string,
	edit mergeConflictTokenEdit,
	normalizedReplacement string,
	normalizedCandidate string,
) bool {
	leftContext := mergeConflictReplacementIntentLeftContext(baseTokens, edit.Start)
	if leftContext != "" && strings.Contains(normalizedCandidate, leftContext+normalizedReplacement) {
		return true
	}
	rightContext := mergeConflictReplacementIntentRightContext(baseTokens, edit.End)
	return rightContext != "" && strings.Contains(normalizedCandidate, normalizedReplacement+rightContext)
}

func mergeConflictReplacementIntentLeftContext(baseTokens []string, editStart int) string {
	contextTokens := make([]string, 0, mergeConflictReplacementIntentContextTokenCount)
	for tokenIndex := editStart - 1; tokenIndex >= 0 && len(contextTokens) < mergeConflictReplacementIntentContextTokenCount; tokenIndex-- {
		normalizedToken := mergeConflictWithoutWhitespace(baseTokens[tokenIndex])
		if normalizedToken != "" {
			contextTokens = append(contextTokens, normalizedToken)
		}
	}
	var context strings.Builder
	for tokenIndex := len(contextTokens) - 1; tokenIndex >= 0; tokenIndex-- {
		context.WriteString(contextTokens[tokenIndex])
	}
	return context.String()
}

func mergeConflictReplacementIntentRightContext(baseTokens []string, editEnd int) string {
	var context strings.Builder
	nonWhitespaceTokenCount := 0
	for tokenIndex := editEnd; tokenIndex < len(baseTokens) && nonWhitespaceTokenCount < mergeConflictReplacementIntentContextTokenCount; tokenIndex++ {
		normalizedToken := mergeConflictWithoutWhitespace(baseTokens[tokenIndex])
		if normalizedToken != "" {
			context.WriteString(normalizedToken)
			nonWhitespaceTokenCount++
		}
	}
	return context.String()
}

func mergeConflictWithoutWhitespace(value string) string {
	var normalized strings.Builder
	normalized.Grow(len(value))
	for _, currentRune := range value {
		if unicode.IsSpace(currentRune) {
			continue
		}
		normalized.WriteRune(currentRune)
	}
	return normalized.String()
}
