package syncflow

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIssueInsertionsWithAdjacentBaseRecords(t *testing.T) {
	const (
		baseRecord        = "- [ ] [I001] {F001} Existing request.\n"
		changedRecord     = "- [ ] [I001] {F001,F002} Existing request.\n"
		otherChange       = "- [ ] [I001] {F003} Existing request.\n"
		secondRecord      = "- [ ] [I002] Another existing request.\n"
		openRecord        = "- [ ] [I003] New request.\n\n"
		closedRecord      = "- [x] [I003] New request.\n  Resolution:\n  Added the result.\n\n"
		independentRecord = "- [ ] [I004] Independent request.\n\n"
	)
	for _, testCase := range []struct {
		name, base, ours, theirs, expected string
	}{
		{"incoming edit", baseRecord, closedRecord + baseRecord, openRecord + independentRecord + changedRecord, closedRecord + independentRecord + changedRecord},
		{"local edit", baseRecord, openRecord + independentRecord + changedRecord, closedRecord + baseRecord, closedRecord + independentRecord + changedRecord},
		{"same edit", baseRecord, closedRecord + changedRecord, openRecord + changedRecord, closedRecord + changedRecord},
		{"multiple base records", baseRecord + secondRecord, closedRecord + changedRecord + secondRecord, openRecord + baseRecord + secondRecord, closedRecord + changedRecord + secondRecord},
		{"incompatible base edits", baseRecord, closedRecord + changedRecord, openRecord + otherChange, closedRecord + changedRecord},
		{"base deletion", baseRecord, closedRecord, openRecord + baseRecord, closedRecord},
		{"base reorder", baseRecord + secondRecord, closedRecord + secondRecord + baseRecord, openRecord + baseRecord + secondRecord, ""},
		{"insertion after base", baseRecord, closedRecord + baseRecord, baseRecord + openRecord, ""},
		{"unrelated new records", baseRecord, closedRecord + baseRecord, independentRecord + changedRecord, closedRecord + independentRecord + changedRecord},
		{"partial base record", "  Existing body.\n", closedRecord + baseRecord, openRecord + changedRecord, ""},
		{"different prefixes", baseRecord, "\n" + closedRecord + baseRecord, openRecord + changedRecord, ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			region := mergeConflictRegion{BasePresent: true, Base: testCase.base, Ours: testCase.ours, Theirs: testCase.theirs}
			plan, applicable := buildMergeIssueDecisionPlan(region)
			if testCase.expected == "" {
				require.False(t, applicable)
				return
			}
			require.True(t, applicable)
			var decisions []mergeBlockDecision
			for _, block := range plan.Blocks {
				action := mergeActionOurs
				if !strings.Contains(testCase.expected, block.Ours) {
					action = mergeActionTheirs
				}
				decisions = append(decisions, mergeBlockDecision{ID: block.ID, Action: action, Reason: "Choose the intended fixture contract."})
			}
			candidate, err := plan.render(decisions)
			require.NoError(t, err)
			require.Equal(t, testCase.expected, candidate.Content)
			require.Len(t, candidate.Dispositions, len(plan.Changes))
		})
	}
}
