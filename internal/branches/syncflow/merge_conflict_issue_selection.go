package syncflow

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tyemirov/utils/llm"
)

const (
	mergeConflictIssueSelectionStrategy     = "issue source selection"
	mergeConflictIssueSelectionSystemPrompt = "You are the semantic fidelity auditor for related Git issue insertions. Select one complete current source record for every issue identifier from the supplied menu. Resolve conflicting requirements from the source evidence, including approved decisions and resolution evidence. A closed status alone does not prove that a record is current. Preserve independent issues. Return only the selection keys, separated by whitespace, exactly one key per issue. Gix copies the selected original records locally, including all punctuation and whitespace. Do not transcribe, rewrite, or combine records. Do not return an approval sentinel, content envelope, explanation, or markdown fence."
	mergeConflictIssueSelectionUserPrompt   = "Repository: %s\nPath: %s\nConflict region: %d of %d\nTarget branch: %s\nMerged reference: %s\n\nAvailable source records:\n%s"
	mergeConflictIssueSelectionKey          = "%s=%d"
	mergeConflictIssueSelectionMenuEntry    = "\nSelection key %s:\n%s\n"
)

func (analysis mergeConflictIssueInsertionAnalysis) selectSources(response string) (string, error) {
	selections := strings.Fields(response)
	if len(selections) != len(analysis.Order) {
		return "", fmt.Errorf("issue selection requires exactly one source key for each of %d identifiers", len(analysis.Order))
	}
	available := map[string]mergeConflictIssueInsertion{}
	for _, id := range analysis.Order {
		for index, content := range analysis.Alternatives[id] {
			key := fmt.Sprintf(mergeConflictIssueSelectionKey, id, index+1)
			available[key] = mergeConflictIssueInsertion{ID: id, Content: content}
		}
	}
	selected := map[string]string{}
	for _, key := range selections {
		entry, exists := available[key]
		if !exists {
			return "", fmt.Errorf("issue selection contains an unknown selection key %q", key)
		}
		if _, duplicate := selected[entry.ID]; duplicate {
			return "", fmt.Errorf("issue selection repeats identifier %s", entry.ID)
		}
		selected[entry.ID] = entry.Content
	}
	var result strings.Builder
	result.WriteString(analysis.Prefix)
	for _, id := range analysis.Order {
		result.WriteString(selected[id])
	}
	return result.String(), nil
}

func (service mergeConflictResolutionService) buildIssueSelectionRequest(options mergeConflictResolutionOptions, conflictFile mergeConflictFile, regionIndex, regionCount int, analysis mergeConflictIssueInsertionAnalysis, feedback string) llm.ChatRequest {
	var menu strings.Builder
	for _, id := range analysis.Order {
		for index, content := range analysis.Alternatives[id] {
			key := fmt.Sprintf(mergeConflictIssueSelectionKey, id, index+1)
			fmt.Fprintf(&menu, mergeConflictIssueSelectionMenuEntry, key, content)
		}
	}
	prompt := fmt.Sprintf(mergeConflictIssueSelectionUserPrompt,
		filepath.Base(filepath.Clean(service.repositoryPath)), conflictFile.Path,
		regionIndex+1, regionCount, options.TargetBranch, options.SourceReference, menu.String())
	if feedback != "" {
		prompt += "\n\n" + fmt.Sprintf(mergeConflictResolutionRejectedPrompt, feedback)
	}
	return llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: mergeConflictIssueSelectionSystemPrompt},
			{Role: "user", Content: prompt},
		},
		MaxTokens: service.commitMessages.MaxTokens,
	}
}
