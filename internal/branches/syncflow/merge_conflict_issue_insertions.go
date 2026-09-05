package syncflow

import (
	"regexp"
	"strings"
)

const mergeConflictIssueInsertionStrategy = "related issue insertions"
const mergeConflictIssueInsertionError = "issue insertion result for %s conflict region %d must contain each issue once with an exact source alternative"

var mergeConflictIssueHeader = regexp.MustCompile(`^- \[[ x!\-]\] \[([BIMFP][0-9]{3}R?)\][ \t]`)

type mergeConflictIssueInsertion struct {
	ID      string
	Content string
}

type mergeConflictIssueInsertions struct {
	Prefix  string
	Entries []mergeConflictIssueInsertion
}

type mergeConflictIssueInsertionAnalysis struct {
	Prefix       string
	Order        []string
	Alternatives map[string][]string
}

// parseMergeConflictIssueInsertions accepts complete canonical issue records only.
// A partial record or unrelated surrounding text stays with the generic resolver.
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

func analyzeMergeConflictIssueInsertions(ours string, theirs string) (mergeConflictIssueInsertionAnalysis, bool) {
	local, localComplete := parseMergeConflictIssueInsertions(ours)
	incoming, incomingComplete := parseMergeConflictIssueInsertions(theirs)
	if !localComplete || !incomingComplete || local.Prefix != incoming.Prefix {
		return mergeConflictIssueInsertionAnalysis{}, false
	}
	analysis := mergeConflictIssueInsertionAnalysis{Prefix: local.Prefix, Alternatives: map[string][]string{}}
	for _, entry := range local.Entries {
		analysis.Order = append(analysis.Order, entry.ID)
		analysis.Alternatives[entry.ID] = []string{entry.Content}
	}
	related := false
	for _, entry := range incoming.Entries {
		alternatives, exists := analysis.Alternatives[entry.ID]
		if exists {
			related = true
		} else {
			analysis.Order = append(analysis.Order, entry.ID)
		}
		if !exists || alternatives[0] != entry.Content {
			analysis.Alternatives[entry.ID] = append(alternatives, entry.Content)
			if exists {
				if overlap, complete := analyzeMergeConflictConcurrentInsertions(alternatives[0], entry.Content); complete {
					analysis.Alternatives[entry.ID] = overlap.Alternatives
				}
			}
		}
	}
	return analysis, related
}

func (analysis mergeConflictIssueInsertionAnalysis) candidate() string {
	var result strings.Builder
	result.WriteString(analysis.Prefix)
	for _, id := range analysis.Order {
		result.WriteString(analysis.Alternatives[id][0])
	}
	return result.String()
}

func (analysis mergeConflictIssueInsertionAnalysis) accepts(content string) bool {
	candidate, complete := parseMergeConflictIssueInsertions(content)
	if !complete || candidate.Prefix != analysis.Prefix || len(candidate.Entries) != len(analysis.Order) {
		return false
	}
	for _, entry := range candidate.Entries {
		found := false
		for _, alternative := range analysis.Alternatives[entry.ID] {
			if entry.Content == alternative {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

const (
	mergeConflictOverlapInstructions  = "\n\nInsertion contract: Return an exact source alternative, including punctuation and whitespace. When one side contains the complete word sequence plus additional words, only that complete source alternative is valid. Equal word sequences permit either exact source alternative."
	mergeConflictAdditiveInstructions = "\n\nIndependent insertion contract: Return exact OURS followed by THEIRS, or exact THEIRS followed by OURS, including every byte and boundary newline."
)

func mergeConflictInsertionInstructions(region mergeConflictRegion) string {
	if !region.BasePresent || region.Base != "" {
		return ""
	}
	if _, overlapping := analyzeMergeConflictConcurrentInsertions(region.Ours, region.Theirs); overlapping {
		return mergeConflictOverlapInstructions
	}
	return mergeConflictAdditiveInstructions
}
