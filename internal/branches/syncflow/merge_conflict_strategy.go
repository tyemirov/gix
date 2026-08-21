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

	mergedContent, merged := mergeConflictNonOverlappingTokenEdits(region.Base, region.Ours, region.Theirs)
	if !merged {
		return mergeConflictDeterministicResolution{}, false
	}
	return mergeConflictDeterministicResolution{
		Content:               mergedContent,
		Strategy:              "non-overlapping token edits",
		RequiresSemanticAudit: true,
	}, true
}

func mergeConflictNonOverlappingTokenEdits(base string, ours string, theirs string) (string, bool) {
	baseTokens := mergeConflictTokens(base)
	oursTokens := mergeConflictTokens(ours)
	theirsTokens := mergeConflictTokens(theirs)
	oursEdits, oursEditsAvailable := mergeConflictTokenEdits(baseTokens, oursTokens)
	theirsEdits, theirsEditsAvailable := mergeConflictTokenEdits(baseTokens, theirsTokens)
	if !oursEditsAvailable || !theirsEditsAvailable {
		return "", false
	}

	mergedEdits, editsCompatible := mergeConflictCompatibleTokenEdits(oursEdits, theirsEdits)
	if !editsCompatible {
		return "", false
	}
	return applyMergeConflictTokenEdits(baseTokens, mergedEdits)
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
		return merged[leftIndex].End < merged[rightIndex].End
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
	if left.Start == right.Start && left.End == right.End {
		return left.Replacement != right.Replacement
	}
	leftInsertion := left.Start == left.End
	rightInsertion := right.Start == right.End
	switch {
	case leftInsertion && rightInsertion:
		return left.Start == right.Start
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

func mergeConflictMissingReplacementIntents(base string, variant string, candidate string) ([]string, bool) {
	baseTokens := mergeConflictTokens(base)
	edits, editsAvailable := mergeConflictTokenEdits(baseTokens, mergeConflictTokens(variant))
	if !editsAvailable {
		return nil, false
	}
	normalizedBase := mergeConflictWithoutWhitespace(base)
	normalizedVariant := mergeConflictWithoutWhitespace(variant)
	normalizedCandidate := mergeConflictWithoutWhitespace(candidate)
	missingIntents := make([]string, 0, len(edits))
	for _, edit := range edits {
		normalizedReplacement := mergeConflictWithoutWhitespace(edit.Replacement)
		if normalizedReplacement == "" {
			continue
		}
		baseOccurrences := strings.Count(normalizedBase, normalizedReplacement)
		variantOccurrences := strings.Count(normalizedVariant, normalizedReplacement)
		candidateOccurrences := strings.Count(normalizedCandidate, normalizedReplacement)
		preservesNewOccurrences := variantOccurrences > baseOccurrences && candidateOccurrences >= variantOccurrences
		if !preservesNewOccurrences && !mergeConflictCandidateMatchesEditContext(baseTokens, edit, normalizedReplacement, normalizedCandidate) {
			missingIntents = append(missingIntents, edit.Replacement)
		}
	}
	return missingIntents, true
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
