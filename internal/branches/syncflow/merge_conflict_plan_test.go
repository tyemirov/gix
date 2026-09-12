package syncflow

import (
	"context"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMergePlanPreservesExactChangesAndBranchSymmetry(t *testing.T) {
	random := rand.New(rand.NewSource(21))
	alphabet := []string{"a\n", "  a\n", "\tb\r\n", "", "b\n", "c"}
	content := func() string {
		var text strings.Builder
		for count := random.Intn(12); count > 0; count-- {
			text.WriteString(alphabet[random.Intn(len(alphabet))])
		}
		return text.String()
	}
	render := func(region mergeConflictRegion, action mergeBlockAction) mergePlanCandidate {
		plan := buildMergeDecisionPlan(region)
		var decisions []mergeBlockDecision
		for _, block := range plan.Blocks {
			decisions = append(decisions, mergeBlockDecision{ID: block.ID, Action: action, Reason: "Choose the indicated source."})
		}
		candidate, err := plan.render(decisions)
		require.NoError(t, err)
		require.Len(t, candidate.Dispositions, len(plan.Changes))
		unique := map[string]bool{}
		for _, disposition := range candidate.Dispositions {
			require.False(t, unique[disposition.ID])
			unique[disposition.ID] = true
		}
		return candidate
	}
	for index := 0; index < 300; index++ {
		base, ours, theirs := content(), content(), content()
		require.Equal(t, ours, render(mergeConflictRegion{Base: base, BasePresent: true, Ours: ours, Theirs: base}, mergeActionOurs).Content)
		forward := render(mergeConflictRegion{Base: base, BasePresent: true, Ours: ours, Theirs: theirs}, mergeActionOurs)
		reverse := render(mergeConflictRegion{Base: base, BasePresent: true, Ours: theirs, Theirs: ours}, mergeActionTheirs)
		require.Equal(t, forward.Content, reverse.Content)
	}
}

func TestMergeLineDiffReconstructsLargeSourceExactly(t *testing.T) {
	base := strings.Repeat("stable\n", 2500)
	variant := "  inserted\n" + strings.Repeat("stable\n", 2400) + "tail"
	lines := mergeConflictLines(base)
	var reconstructed strings.Builder
	cursor := 0
	for _, edit := range mergeConflictLineEdits(lines, mergeConflictLines(variant)) {
		reconstructed.WriteString(strings.Join(lines[cursor:edit.Start], ""))
		reconstructed.WriteString(edit.Replacement)
		cursor = edit.End
	}
	reconstructed.WriteString(strings.Join(lines[cursor:], ""))
	require.Equal(t, variant, reconstructed.String())
}

func TestMergePlanValidatesDecisionBoundary(t *testing.T) {
	plan := buildMergeDecisionPlan(mergeConflictRegion{Base: "mode 1\nlimit 5\n", BasePresent: true, Ours: "mode 2\nlimit 5\n", Theirs: "mode 3\nlimit 7\n"})
	require.Len(t, plan.Blocks, 1)
	for _, response := range []string{
		`{"status":"resolved","decisions":[]}`,
		`{"status":"resolved","decisions":[{"id":"outside","action":"ours","reason":"scope"}]}`,
		`{"status":"resolved","decisions":[{"id":"conflict-1","action":"ours","content":"rewritten","reason":"scope"}]}`,
		`{"status":"resolved","decisions":[{"id":"conflict-1","action":"combine","reason":"missing content"}]}`,
		`{"status":"resolved","decisions":[{"id":"conflict-1","action":"ours"}]}`,
		`{"status":"resolved","decisions":[{"id":"conflict-1","action":"base","reason":"invalid action"}]}`,
		`{"status":"resolved","decisions":[{"id":"conflict-1","action":"combine","content":"<<<<<<< HEAD\n","reason":"marker"}]}`,
	} {
		decoded, err := decodeMergeModelResponse(response)
		require.NoError(t, err)
		_, err = plan.render(decoded.Decisions)
		require.Error(t, err, response)
	}
	empty := ""
	candidate, err := plan.render([]mergeBlockDecision{{ID: "conflict-1", Action: mergeActionCombine, Content: &empty, Reason: "Delete the obsolete mode field."}})
	require.NoError(t, err)
	require.Equal(t, "limit 7\n", candidate.Content)
	require.Equal(t, mergeOutcomeCombined, candidate.Dispositions[len(candidate.Dispositions)-1].Outcome)
}

func TestMergeDecisionProtocolRejectsAmbiguousJSON(t *testing.T) {
	for _, response := range []string{
		`{"status":"approved","status":"resolved"}`,
		`{"Status":"approved"}`,
		`{"status":"resolved","decisions":[{"id":"conflict-1","id":"conflict-2"}]}`,
		`{"status":"approved","unexpected":true}`,
		`{"status":"approved"} {"status":"approved"}`,
		`{"status":"unresolved"}`,
		`{"status":"rejected","reason":""}`,
		"GIX_MERGE_REVIEW_APPROVED",
		"```json\n{}\n```",
	} {
		_, err := decodeMergeModelResponse(response)
		require.Error(t, err, response)
	}
}

func TestMergePlanTerminatesExplicitOutcomesAndRepeatedCandidates(t *testing.T) {
	combine := `{"status":"resolved","decisions":[{"id":"conflict-1","action":"combine","content":"combined\n","reason":"combine sources"}]}`
	for _, fixture := range []struct {
		name      string
		responses []string
		expected  string
		calls     int
	}{
		{"unresolved", []string{`{"status":"unresolved","reason":"Conflicting approvals."}`}, "unresolved merge decision", 1},
		{"missing context", []string{`{"status":"needs_context","reason":"Need a product decision."}`}, "requires unavailable context", 1},
		{"empty provider", []string{" "}, "empty merge resolution", 1},
		{"repeated candidate", []string{combine, `{"status":"rejected","reason":"Drops a requirement."}`, combine}, "repeated a rejected candidate", 3},
		{"review required", []string{`{}`, `{}`, `{}`, combine}, "still requires semantic review", 4},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			client := &strictSyncChatClient{responses: fixture.responses}
			service := mergeConflictResolutionService{repositoryPath: t.TempDir()}
			document := mergeConflictDocument{NonConflictingRegions: []string{"", ""}, ConflictRegions: []mergeConflictRegion{{Base: "base\n", BasePresent: true, Ours: "ours\n", Theirs: "theirs\n"}}}
			_, err := service.resolvePlannedConflictRegion(context.Background(), client, mergeConflictResolutionOptions{}, mergeConflictFile{Path: "file.txt", Base: "base\n", Ours: "ours\n", Theirs: "theirs\n"}, document, 0, time.Second)
			require.ErrorContains(t, err, fixture.expected)
			require.Len(t, client.requests, fixture.calls)
		})
	}
}
