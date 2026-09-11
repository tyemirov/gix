package syncflow

import (
	"regexp"
	"strings"
)

var mergeConflictIssueHeader = regexp.MustCompile(`^- \[[ x!\-]\] \[([BIMFP][0-9]{3}R?)\][ \t]`)

type mergeConflictIssueInsertion struct {
	ID      string
	Content string
}

type mergeConflictIssueInsertions struct {
	Prefix  string
	Entries []mergeConflictIssueInsertion
}

func parseMergeConflictIssueInsertions(content string) (mergeConflictIssueInsertions, bool) {
	result := mergeConflictIssueInsertions{}
	seen := map[string]bool{}
	for _, line := range mergeConflictLines(content) {
		if header := mergeConflictIssueHeader.FindStringSubmatch(line); header != nil {
			id := header[1]
			if seen[id] {
				return mergeConflictIssueInsertions{}, false
			}
			seen[id] = true
			result.Entries = append(result.Entries, mergeConflictIssueInsertion{ID: id, Content: line})
			continue
		}
		if len(result.Entries) == 0 {
			if strings.TrimSpace(line) != "" {
				return mergeConflictIssueInsertions{}, false
			}
			result.Prefix += line
			continue
		}
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "  ") {
			return mergeConflictIssueInsertions{}, false
		}
		result.Entries[len(result.Entries)-1].Content += line
	}
	return result, len(result.Entries) > 0
}
