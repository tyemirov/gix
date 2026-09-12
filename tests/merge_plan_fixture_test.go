package tests

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	mergePlanApprovedForTest     = `{"status":"approved"}`
	mergePlanRejectedForTest     = `{"status":"rejected","reason":"fixture review rejected the semantic change"}`
	mergePlanUnknownBlockForTest = `{"status":"resolved","decisions":[{"id":"unknown","action":"ours","reason":"Invalid block fixture."}]}`
)

type mergePlanInputForTest struct {
	Phase     string                                    `json:"phase"`
	Path      string                                    `json:"path"`
	Blocks    []struct{ ID, Base, Ours, Theirs string } `json:"blocks"`
	Candidate *struct{ Content string }                 `json:"candidate"`
	Placement struct{ Before, After string }            `json:"placement"`
}

func decodeMergePlanInputForTest(body []byte) (mergePlanInputForTest, error) {
	var chat struct{ Messages []struct{ Content string } }
	var input mergePlanInputForTest
	if err := json.Unmarshal(body, &chat); err != nil {
		return input, err
	}
	if len(chat.Messages) == 0 {
		return input, fmt.Errorf("missing chat messages")
	}
	_, raw, ok := strings.Cut(chat.Messages[len(chat.Messages)-1].Content, "GIX_MERGE_INPUT\n")
	if !ok {
		return input, fmt.Errorf("missing merge plan")
	}
	err := json.Unmarshal([]byte(raw), &input)
	return input, err
}

// Source fixtures define the expected file independently of the merge engine.
// The mock supplies only selections of source blocks present in that fixture.
func mergePlanSourceFixtureResponse(body []byte, expected string) (string, error) {
	input, err := decodeMergePlanInputForTest(body)
	if err != nil {
		return "", err
	}
	if input.Phase == "review" {
		return mergePlanApprovedForTest, nil
	}
	decisions := []map[string]string{}
	for _, block := range input.Blocks {
		action := ""
		switch {
		case block.Ours != "" && strings.Contains(expected, block.Ours):
			action = "ours"
		case block.Theirs != "" && strings.Contains(expected, block.Theirs):
			action = "theirs"
		default:
			return "", fmt.Errorf("fixture has no exact source for block %s: ours %q, theirs %q", block.ID, block.Ours, block.Theirs)
		}
		decisions = append(decisions, map[string]string{"id": block.ID, "action": action, "reason": "Select the source that satisfies this fixture's acceptance contract."})
	}
	encoded, err := json.Marshal(map[string]any{"status": "resolved", "decisions": decisions})
	return string(encoded), err
}

func semanticMergeResponse(content string) string {
	encoded, err := json.Marshal(map[string]any{"status": "resolved", "decisions": []map[string]string{{"id": "conflict-1", "action": "combine", "content": content, "reason": "Combine the compatible source changes."}}})
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
