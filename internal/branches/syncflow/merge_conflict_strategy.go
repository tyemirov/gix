package syncflow

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	mergeConflictTokenDiffMaximumCells              = 4_000_000
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
		return mergeConflictDeterministicResolution{
			Content:               region.Ours + region.Theirs,
			Strategy:              "concurrent insertions",
			RequiresSemanticAudit: true,
		}, true
	}

	return mergeConflictTokenEditResolution(region.Base, region.Ours, region.Theirs)
}

func mergeConflictTokenEditResolution(base string, ours string, theirs string) (mergeConflictDeterministicResolution, bool) {
	baseTokens := mergeConflictTokens(base)
	oursTokens := mergeConflictTokens(ours)
	theirsTokens := mergeConflictTokens(theirs)
	oursEdits, oursEditsAvailable := mergeConflictTokenEdits(baseTokens, oursTokens)
	theirsEdits, theirsEditsAvailable := mergeConflictTokenEdits(baseTokens, theirsTokens)
	if !oursEditsAvailable || !theirsEditsAvailable {
		return mergeConflictDeterministicResolution{}, false
	}

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
		_, runeSize := utf8.DecodeRuneInString(value[offset:])
		nextOffset := offset + runeSize
		if nextOffset == len(value) {
			tokens = append(tokens, value[tokenStart:nextOffset])
			break
		}
		nextClass := mergeConflictTokenClass(value, nextOffset)
		if nextClass != currentClass {
			tokens = append(tokens, value[tokenStart:nextOffset])
			tokenStart = nextOffset
			currentClass = nextClass
		}
		offset = nextOffset
	}
	return tokens
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

func mergeConflictTokenEdits(baseTokens []string, variantTokens []string) ([]mergeConflictTokenEdit, bool) {
	rowCount := len(baseTokens) + 1
	columnCount := len(variantTokens) + 1
	if rowCount > mergeConflictTokenDiffMaximumCells/columnCount {
		return nil, false
	}
	cellCount := rowCount * columnCount

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
			edits = appendMergeConflictTokenEdit(
				edits,
				unmatchedBaseStart,
				baseIndex,
				variantTokens[unmatchedVariantStart:variantIndex],
			)
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
	edits = appendMergeConflictTokenEdit(
		edits,
		unmatchedBaseStart,
		len(baseTokens),
		variantTokens[unmatchedVariantStart:],
	)
	return edits, true
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

func mergeConflictMissingRegionReplacementIntents(base string, ours string, theirs string, candidate string) ([]string, []string, bool) {
	baseTokens := mergeConflictTokens(base)
	oursEdits, oursEditsAvailable := mergeConflictTokenEdits(baseTokens, mergeConflictTokens(ours))
	theirsEdits, theirsEditsAvailable := mergeConflictTokenEdits(baseTokens, mergeConflictTokens(theirs))
	if !oursEditsAvailable || !theirsEditsAvailable {
		return nil, nil, false
	}
	if mergeConflictCandidateMatchesDerivedVariant(baseTokens, oursEdits, theirsEdits, candidate) {
		return nil, nil, true
	}
	oursMissing := mergeConflictMissingReplacementIntents(base, ours, theirs, candidate, baseTokens, oursEdits, theirsEdits)
	theirsMissing := mergeConflictMissingReplacementIntents(base, theirs, ours, candidate, baseTokens, theirsEdits, oursEdits)
	return oursMissing, theirsMissing, true
}

func mergeConflictMissingReplacementIntents(
	base string,
	variant string,
	otherVariant string,
	candidate string,
	baseTokens []string,
	edits []mergeConflictTokenEdit,
	otherEdits []mergeConflictTokenEdit,
) []string {
	normalizedBase := mergeConflictWithoutWhitespace(base)
	normalizedVariant := mergeConflictWithoutWhitespace(variant)
	normalizedOtherVariant := mergeConflictWithoutWhitespace(otherVariant)
	normalizedCandidate := mergeConflictWithoutWhitespace(candidate)
	if normalizedVariant != "" && strings.Contains(normalizedCandidate, normalizedVariant) {
		return nil
	}
	compatibleVariant, compatibleVariantAvailable := mergeConflictVariantWithCompatibleOtherEdits(baseTokens, edits, otherEdits)
	normalizedCompatibleVariant := mergeConflictWithoutWhitespace(compatibleVariant)
	if compatibleVariantAvailable && normalizedCompatibleVariant != "" && strings.Contains(normalizedCandidate, normalizedCompatibleVariant) {
		return nil
	}
	missingIntents := make([]string, 0, len(edits))
	for _, edit := range edits {
		normalizedReplacement := mergeConflictWithoutWhitespace(edit.Replacement)
		if normalizedReplacement == "" {
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
