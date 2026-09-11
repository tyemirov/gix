package syncflow

import "strings"

const mergePlanContextLines = 40

func initialMergeReadContext(file mergeConflictFile, region mergeConflictRegion) mergeReadContext {
	complete := mergeReadContext{Scope: mergePlanFileContext, Base: file.Base, Ours: file.Ours, Theirs: file.Theirs}
	if len(file.Base)+len(file.Ours)+len(file.Theirs) <= mergePlanContextLimit {
		return complete
	}
	return mergeReadContext{Scope: mergePlanRegionContext,
		Base:   mergeSourceContext(file.Base, region.Base),
		Ours:   mergeSourceContext(file.Ours, region.Ours),
		Theirs: mergeSourceContext(file.Theirs, region.Theirs)}
}

// Include complete issue records when the hunk intersects a record. For other
// text, include surrounding lines. An ambiguous location requires the full file.
func mergeSourceContext(source, region string) string {
	if region == "" || strings.Count(source, region) != 1 {
		return source
	}
	offset := strings.Index(source, region)
	first := strings.Count(source[:offset], "\n")
	last := first + len(mergeConflictLines(region))
	lines := mergeConflictLines(source)
	start, end := max(0, first-mergePlanContextLines), min(len(lines), last+mergePlanContextLines)
	recordStart := -1
	for index := 0; index <= first && index < len(lines); index++ {
		if mergeConflictIssueHeader.MatchString(lines[index]) {
			recordStart = index
		}
		if strings.HasPrefix(lines[index], "#") {
			recordStart = -1
		}
	}
	if recordStart >= 0 {
		start = recordStart
		end = len(lines)
		for index := last; index < len(lines); index++ {
			if mergeConflictIssueHeader.MatchString(lines[index]) || strings.HasPrefix(lines[index], "#") {
				end = index
				break
			}
		}
	}
	return strings.Join(lines[start:end], "")
}
