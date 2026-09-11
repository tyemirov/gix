package syncflow

import "strings"

const mergeConflictLineDiffMatrixMaximumCells = 4_000_000

type mergeConflictLineEdit struct {
	Start, End  int
	Replacement string
}
type mergeConflictLineMatch struct{ BaseIndex, VariantIndex int }

func mergeConflictLineEdits(baseLines []string, variantLines []string) []mergeConflictLineEdit {
	rowCount := len(baseLines) + 1
	columnCount := len(variantLines) + 1
	if !mergeConflictLineEditsRequireLinearMemory(baseLines, variantLines) {
		return mergeConflictMatrixLineEdits(baseLines, variantLines, rowCount*columnCount, columnCount)
	}
	return mergeConflictLinearLineEdits(baseLines, variantLines)
}

func mergeConflictLineEditsRequireLinearMemory(baseLines []string, variantLines []string) bool {
	return len(baseLines)+1 > mergeConflictLineDiffMatrixMaximumCells/(len(variantLines)+1)
}

func mergeConflictMatrixLineEdits(baseLines []string, variantLines []string, cellCount int, columnCount int) []mergeConflictLineEdit {
	longestSubsequences := make([]int, cellCount)
	for baseIndex := len(baseLines) - 1; baseIndex >= 0; baseIndex-- {
		for variantIndex := len(variantLines) - 1; variantIndex >= 0; variantIndex-- {
			cellIndex := baseIndex*columnCount + variantIndex
			if baseLines[baseIndex] == variantLines[variantIndex] {
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

	edits := make([]mergeConflictLineEdit, 0)
	baseIndex := 0
	variantIndex := 0
	unmatchedBaseStart := 0
	unmatchedVariantStart := 0
	for baseIndex < len(baseLines) && variantIndex < len(variantLines) {
		if baseLines[baseIndex] == variantLines[variantIndex] {
			edits = appendMergeConflictLineEdit(edits, unmatchedBaseStart, baseIndex, variantLines[unmatchedVariantStart:variantIndex])
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
	edits = appendMergeConflictLineEdit(edits, unmatchedBaseStart, len(baseLines), variantLines[unmatchedVariantStart:])
	return edits
}

func mergeConflictLinearLineEdits(baseLines []string, variantLines []string) []mergeConflictLineEdit {
	commonPrefixLength := 0
	for commonPrefixLength < len(baseLines) &&
		commonPrefixLength < len(variantLines) &&
		baseLines[commonPrefixLength] == variantLines[commonPrefixLength] {
		commonPrefixLength++
	}
	commonSuffixLength := 0
	for commonSuffixLength < len(baseLines) &&
		commonSuffixLength < len(variantLines) &&
		baseLines[len(baseLines)-commonSuffixLength-1] == variantLines[len(variantLines)-commonSuffixLength-1] {
		commonSuffixLength++
	}
	shorterLineCount := min(len(baseLines), len(variantLines))
	if commonPrefixLength+commonSuffixLength > shorterLineCount {
		if commonPrefixLength >= commonSuffixLength {
			commonSuffixLength = 0
		} else {
			commonPrefixLength = 0
		}
	}
	baseMiddleEnd := len(baseLines) - commonSuffixLength
	variantMiddleEnd := len(variantLines) - commonSuffixLength
	matches := mergeConflictLineMatches(
		baseLines[commonPrefixLength:baseMiddleEnd],
		variantLines[commonPrefixLength:variantMiddleEnd],
		commonPrefixLength,
		commonPrefixLength,
	)
	edits := make([]mergeConflictLineEdit, 0)
	baseCursor := commonPrefixLength
	variantCursor := commonPrefixLength
	for _, match := range matches {
		edits = appendMergeConflictLineEdit(edits, baseCursor, match.BaseIndex, variantLines[variantCursor:match.VariantIndex])
		baseCursor = match.BaseIndex + 1
		variantCursor = match.VariantIndex + 1
	}
	edits = appendMergeConflictLineEdit(edits, baseCursor, baseMiddleEnd, variantLines[variantCursor:variantMiddleEnd])
	return edits
}

func mergeConflictLineMatches(baseLines []string, variantLines []string, baseOffset int, variantOffset int) []mergeConflictLineMatch {
	if len(baseLines) == 0 || len(variantLines) == 0 {
		return nil
	}
	if len(baseLines) == 1 {
		for variantIndex, variantLine := range variantLines {
			if baseLines[0] == variantLine {
				return []mergeConflictLineMatch{{BaseIndex: baseOffset, VariantIndex: variantOffset + variantIndex}}
			}
		}
		return nil
	}
	if len(variantLines) == 1 {
		for baseIndex, baseLine := range baseLines {
			if baseLine == variantLines[0] {
				return []mergeConflictLineMatch{{BaseIndex: baseOffset + baseIndex, VariantIndex: variantOffset}}
			}
		}
		return nil
	}

	baseMiddle := len(baseLines) / 2
	variantMiddle := func() int {
		prefixLengths := mergeConflictLinePrefixLCSLengths(baseLines[:baseMiddle], variantLines)
		suffixLengths := mergeConflictLineSuffixLCSLengths(baseLines[baseMiddle:], variantLines)
		selectedMiddle := 0
		longestLength := -1
		for variantIndex := 0; variantIndex <= len(variantLines); variantIndex++ {
			candidateLength := prefixLengths[variantIndex] + suffixLengths[variantIndex]
			if candidateLength > longestLength {
				selectedMiddle = variantIndex
				longestLength = candidateLength
			}
		}
		return selectedMiddle
	}()

	leftMatches := mergeConflictLineMatches(baseLines[:baseMiddle], variantLines[:variantMiddle], baseOffset, variantOffset)
	rightMatches := mergeConflictLineMatches(baseLines[baseMiddle:], variantLines[variantMiddle:], baseOffset+baseMiddle, variantOffset+variantMiddle)
	return append(leftMatches, rightMatches...)
}

func mergeConflictLinePrefixLCSLengths(baseLines []string, variantLines []string) []int {
	lengths := make([]int, len(variantLines)+1)
	for _, baseLine := range baseLines {
		previousDiagonal := 0
		for variantIndex, variantLine := range variantLines {
			previousLength := lengths[variantIndex+1]
			if baseLine == variantLine {
				lengths[variantIndex+1] = previousDiagonal + 1
			} else if lengths[variantIndex] > previousLength {
				lengths[variantIndex+1] = lengths[variantIndex]
			}
			previousDiagonal = previousLength
		}
	}
	return lengths
}

func mergeConflictLineSuffixLCSLengths(baseLines []string, variantLines []string) []int {
	lengths := make([]int, len(variantLines)+1)
	for baseIndex := len(baseLines) - 1; baseIndex >= 0; baseIndex-- {
		previousDiagonal := 0
		for variantIndex := len(variantLines) - 1; variantIndex >= 0; variantIndex-- {
			previousLength := lengths[variantIndex]
			if baseLines[baseIndex] == variantLines[variantIndex] {
				lengths[variantIndex] = previousDiagonal + 1
			} else if lengths[variantIndex+1] > previousLength {
				lengths[variantIndex] = lengths[variantIndex+1]
			}
			previousDiagonal = previousLength
		}
	}
	return lengths
}

func appendMergeConflictLineEdit(edits []mergeConflictLineEdit, start int, end int, replacementLines []string) []mergeConflictLineEdit {
	replacement := strings.Join(replacementLines, "")
	if start == end && replacement == "" {
		return edits
	}
	return append(edits, mergeConflictLineEdit{
		Start:       start,
		End:         end,
		Replacement: replacement,
	})
}
