package syncflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

type mergeChange struct {
	ID     string `json:"id"`
	Side   string `json:"side"`
	Start  int    `json:"-"`
	End    int    `json:"-"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type mergeDecisionBlock struct {
	ID      string        `json:"id"`
	Base    string        `json:"base"`
	Ours    string        `json:"ours"`
	Theirs  string        `json:"theirs"`
	Changes []mergeChange `json:"changes"`
}

type mergePlanPiece struct {
	Text  string
	Block string
}

type mergeChangeDisposition struct {
	ID      string `json:"id"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
}

type mergeDecisionPlan struct {
	Changes   []mergeChange            `json:"changes"`
	Blocks    []mergeDecisionBlock     `json:"blocks"`
	Pieces    []mergePlanPiece         `json:"-"`
	Automatic []mergeChangeDisposition `json:"automatic"`
}

type mergeBlockAction string

const (
	mergeActionOurs        mergeBlockAction = "ours"
	mergeActionTheirs      mergeBlockAction = "theirs"
	mergeActionCombine     mergeBlockAction = "combine"
	mergeSideOurs                           = "ours"
	mergeSideTheirs                         = "theirs"
	mergeOutcomeRetained                    = "retained"
	mergeOutcomeCombined                    = "combined"
	mergeOutcomeSuperseded                  = "superseded"
)

type mergeBlockDecision struct {
	ID      string           `json:"id"`
	Action  mergeBlockAction `json:"action"`
	Content *string          `json:"content,omitempty"`
	Reason  string           `json:"reason"`
}

type mergeModelStatus string

const (
	mergeStatusResolved     mergeModelStatus = "resolved"
	mergeStatusApproved     mergeModelStatus = "approved"
	mergeStatusRejected     mergeModelStatus = "rejected"
	mergeStatusNeedsContext mergeModelStatus = "needs_context"
	mergeStatusUnresolved   mergeModelStatus = "unresolved"
)

type mergeModelResponse struct {
	Status    mergeModelStatus     `json:"status"`
	Decisions []mergeBlockDecision `json:"decisions,omitempty"`
	Reason    string               `json:"reason,omitempty"`
}

type mergePlanCandidate struct {
	Content      string                   `json:"content"`
	Decisions    []mergeBlockDecision     `json:"decisions"`
	Dispositions []mergeChangeDisposition `json:"dispositions"`
}

func decodeMergeModelResponse(response string) (mergeModelResponse, error) {
	var result mergeModelResponse
	if err := validateMergeJSONKeys(json.NewDecoder(strings.NewReader(response))); err != nil {
		return result, fmt.Errorf("decode merge decision: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(response))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("decode merge decision: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return result, errors.New("merge decision must contain exactly one JSON object")
	}
	switch result.Status {
	case mergeStatusResolved:
		if result.Reason != "" {
			return result, errors.New("resolved decisions require reasons on their individual blocks")
		}
	case mergeStatusApproved:
		if len(result.Decisions) != 0 || result.Reason != "" {
			return result, errors.New("approval must contain only its status")
		}
	case mergeStatusRejected, mergeStatusNeedsContext, mergeStatusUnresolved:
		if strings.TrimSpace(result.Reason) == "" || len(result.Decisions) != 0 {
			return result, errors.New("merge outcome requires a reason and no decisions")
		}
	default:
		return result, fmt.Errorf("unknown merge outcome %q", result.Status)
	}
	return result, nil
}

// render constructs every fixed piece locally. Decisions can affect only their
// assigned overlap; all source changes receive a recorded disposition.
func (plan mergeDecisionPlan) render(decisions []mergeBlockDecision) (mergePlanCandidate, error) {
	if len(decisions) != len(plan.Blocks) {
		return mergePlanCandidate{}, fmt.Errorf("merge plan requires exactly one decision for each of %d blocks", len(plan.Blocks))
	}
	available := make(map[string]mergeDecisionBlock, len(plan.Blocks))
	for _, block := range plan.Blocks {
		available[block.ID] = block
	}
	resolved := make(map[string]string, len(decisions))
	result := mergePlanCandidate{Decisions: decisions, Dispositions: append([]mergeChangeDisposition(nil), plan.Automatic...)}
	for _, decision := range decisions {
		block, exists := available[decision.ID]
		if !exists {
			return result, fmt.Errorf("unknown merge block %q", decision.ID)
		}
		if _, duplicate := resolved[decision.ID]; duplicate {
			return result, fmt.Errorf("repeated merge block %q", decision.ID)
		}
		if strings.TrimSpace(decision.Reason) == "" {
			return result, fmt.Errorf("merge block %s requires a semantic reason", decision.ID)
		}
		var content string
		switch decision.Action {
		case mergeActionOurs, mergeActionTheirs:
			if decision.Content != nil {
				return result, fmt.Errorf("source selection for %s cannot rewrite source bytes", decision.ID)
			}
			content = block.Ours
			if decision.Action == mergeActionTheirs {
				content = block.Theirs
			}
		case mergeActionCombine:
			if decision.Content == nil {
				return result, fmt.Errorf("combined block %s requires explicit content, including an empty string for deletion", decision.ID)
			}
			content = *decision.Content
		default:
			return result, fmt.Errorf("unknown action %q for merge block %s", decision.Action, decision.ID)
		}
		if containsConflictMarker(content) {
			return result, fmt.Errorf("merge block %s contains conflict markers", decision.ID)
		}
		resolved[decision.ID] = content
		for _, change := range block.Changes {
			outcome := mergeOutcomeSuperseded
			if decision.Action == mergeActionCombine {
				outcome = mergeOutcomeCombined
			} else if string(decision.Action) == change.Side {
				outcome = mergeOutcomeRetained
			}
			result.Dispositions = append(result.Dispositions, mergeChangeDisposition{ID: change.ID, Outcome: outcome, Reason: decision.Reason})
		}
	}
	var content strings.Builder
	for _, piece := range plan.Pieces {
		if piece.Block == "" {
			content.WriteString(piece.Text)
		} else {
			content.WriteString(resolved[piece.Block])
		}
	}
	result.Content = content.String()
	return result, nil
}

func (plan *mergeDecisionPlan) appendGroup(base, ours, theirs string, changes []mergeChange) {
	plan.Changes = append(plan.Changes, changes...)
	if ours == theirs || ours == base || theirs == base {
		content := ours
		if ours == base {
			content = theirs
		}
		plan.Pieces = append(plan.Pieces, mergePlanPiece{Text: content})
		for _, change := range changes {
			plan.Automatic = append(plan.Automatic, mergeChangeDisposition{ID: change.ID, Outcome: mergeOutcomeRetained, Reason: "Exact identical or unilateral source change."})
		}
		return
	}
	id := fmt.Sprintf("conflict-%d", len(plan.Blocks)+1)
	plan.Blocks = append(plan.Blocks, mergeDecisionBlock{ID: id, Base: base, Ours: ours, Theirs: theirs, Changes: changes})
	plan.Pieces = append(plan.Pieces, mergePlanPiece{Block: id})
}

func buildMergeDecisionPlan(region mergeConflictRegion) mergeDecisionPlan {
	if plan, recognized := buildMergeIssueDecisionPlan(region); recognized {
		return plan
	}
	base := mergeConflictLines(region.Base)
	var changes []mergeChange
	for _, side := range []struct{ name, content string }{{mergeSideOurs, region.Ours}, {mergeSideTheirs, region.Theirs}} {
		var split []mergeConflictLineEdit
		for _, edit := range mergeConflictLineEdits(base, mergeConflictLines(side.content)) {
			replacements := mergeConflictLines(edit.Replacement)
			if edit.End-edit.Start == len(replacements) && len(replacements) > 1 {
				for offset, replacement := range replacements {
					split = append(split, mergeConflictLineEdit{Start: edit.Start + offset, End: edit.Start + offset + 1, Replacement: replacement})
				}
			} else {
				split = append(split, edit)
			}
		}
		for index, edit := range split {
			changes = append(changes, mergeChange{ID: fmt.Sprintf("%s-%d", side.name, index+1), Side: side.name, Start: edit.Start, End: edit.End, Before: strings.Join(base[edit.Start:edit.End], ""), After: edit.Replacement})
		}
	}
	sort.SliceStable(changes, func(left, right int) bool {
		if changes[left].Start != changes[right].Start {
			return changes[left].Start < changes[right].Start
		}
		return changes[left].End < changes[right].End
	})
	var groups [][]mergeChange
	for _, change := range changes {
		if len(groups) == 0 {
			groups = append(groups, []mergeChange{change})
			continue
		}
		group := groups[len(groups)-1]
		overlap := false
		for _, existing := range group {
			if mergeChangesOverlap(existing, change) {
				overlap = true
				break
			}
		}
		if overlap {
			groups[len(groups)-1] = append(group, change)
		} else {
			groups = append(groups, []mergeChange{change})
		}
	}
	plan := mergeDecisionPlan{}
	cursor := 0
	for _, group := range groups {
		start, end := group[0].Start, group[0].End
		for _, change := range group {
			end = max(end, change.End)
		}
		plan.Pieces = append(plan.Pieces, mergePlanPiece{Text: strings.Join(base[cursor:start], "")})
		original := strings.Join(base[start:end], "")
		renderSide := func(side string) string {
			var result strings.Builder
			position := start
			for _, change := range group {
				if change.Side != side {
					continue
				}
				result.WriteString(strings.Join(base[position:change.Start], ""))
				result.WriteString(change.After)
				position = change.End
			}
			result.WriteString(strings.Join(base[position:end], ""))
			return result.String()
		}
		plan.appendGroup(original, renderSide(mergeSideOurs), renderSide(mergeSideTheirs), group)
		cursor = end
	}
	plan.Pieces = append(plan.Pieces, mergePlanPiece{Text: strings.Join(base[cursor:], "")})
	return plan
}

func mergeChangesOverlap(left, right mergeChange) bool {
	if left.Start == left.End && right.Start == right.End {
		return left.Start == right.Start
	}
	if left.Start == left.End {
		return right.Start < left.Start && left.Start < right.End
	}
	if right.Start == right.End {
		return left.Start < right.Start && right.Start < left.End
	}
	return left.Start < right.End && right.Start < left.End
}

func buildMergeIssueDecisionPlan(region mergeConflictRegion) (mergeDecisionPlan, bool) {
	stages := make([]mergeConflictIssueInsertions, 3)
	for index, content := range []string{region.Base, region.Ours, region.Theirs} {
		if content == "" {
			continue
		}
		parsed, ok := parseMergeConflictIssueInsertions(content)
		if !ok {
			return mergeDecisionPlan{}, false
		}
		stages[index] = parsed
	}
	prefix := stages[1].Prefix
	if stages[2].Prefix != prefix || (region.Base != "" && stages[0].Prefix != prefix) {
		return mergeDecisionPlan{}, false
	}
	records := map[string][3]string{}
	edges := map[string]map[string]bool{}
	degrees := map[string]int{}
	for stageIndex, stage := range stages {
		for index, entry := range stage.Entries {
			versions := records[entry.ID]
			versions[stageIndex] = entry.Content
			records[entry.ID] = versions
			if _, exists := degrees[entry.ID]; !exists {
				degrees[entry.ID] = 0
			}
			if index == 0 {
				continue
			}
			previous := stage.Entries[index-1].ID
			if edges[previous] == nil {
				edges[previous] = map[string]bool{}
			}
			if !edges[previous][entry.ID] {
				edges[previous][entry.ID] = true
				degrees[entry.ID]++
			}
		}
	}
	if len(records) == 0 {
		return mergeDecisionPlan{}, false
	}
	var order []string
	for len(order) < len(records) {
		var ready []string
		for id, degree := range degrees {
			if degree == 0 {
				ready = append(ready, id)
			}
		}
		if len(ready) == 0 {
			return mergeDecisionPlan{}, false
		}
		sort.Strings(ready)
		id := ready[0]
		order = append(order, id)
		degrees[id] = -1
		for next := range edges[id] {
			degrees[next]--
		}
	}
	plan := mergeDecisionPlan{Pieces: []mergePlanPiece{{Text: prefix}}}
	for _, id := range order {
		versions := records[id]
		var changes []mergeChange
		for index, side := range []string{mergeSideOurs, mergeSideTheirs} {
			if versions[index+1] != versions[0] {
				changes = append(changes, mergeChange{ID: side + "-" + id, Side: side, Before: versions[0], After: versions[index+1]})
			}
		}
		plan.appendGroup(versions[0], versions[1], versions[2], changes)
	}
	return plan, true
}

// JSON duplicate keys otherwise silently replace earlier decisions.
func validateMergeJSONKeys(decoder *json.Decoder) error {
	value, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, container := value.(json.Delim)
	if !container {
		return nil
	}
	keys := map[string]bool{}
	for decoder.More() {
		if delimiter == '{' {
			token, err := decoder.Token()
			if err != nil {
				return err
			}
			key := token.(string)
			switch key {
			case "status", "decisions", "reason", "id", "action", "content":
			default:
				return fmt.Errorf("unknown JSON field %q", key)
			}
			if keys[key] {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			keys[key] = true
		}
		if err := validateMergeJSONKeys(decoder); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}
