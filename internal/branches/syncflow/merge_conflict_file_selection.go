package syncflow

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/utils/llm"
)

const (
	mergeConflictSelectOurs            = "GIX_MERGE_SELECT_OURS"
	mergeConflictSelectTheirs          = "GIX_MERGE_SELECT_THEIRS"
	mergeConflictSelectionStrategy     = "file conflict selection"
	mergeConflictSelectionSystemPrompt = "Resolve a Git file conflict from BASE, OURS, and THEIRS. A missing file is a deletion, not an empty file. Evaluate both the deletion intent and the concurrent edit. Select one coherent current alternative. Return exactly " + mergeConflictSelectOurs + " or " + mergeConflictSelectTheirs + ". Do not return file contents or an audit approval."
	mergeConflictSelectionUserPrompt   = "Path: %s\nTarget branch: %s\nMerged reference: %s\n\nBASE:\n%s\n\nOURS:\n%s\n\nTHEIRS:\n%s"
	mergeConflictSelectionError        = "file conflict for %s requires an explicit OURS or THEIRS selection"
	mergeConflictBinaryError           = "binary conflict for %s requires an explicit Git resolution"
)

func mergeConflictFileIsText(file mergeConflictFile) bool {
	for _, content := range []string{file.Base, file.Ours, file.Theirs, file.WorktreeContent} {
		if !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
			return false
		}
	}
	return true
}

func (service mergeConflictResolutionService) resolveMarkerFreeConflictFile(ctx context.Context, clientProvider mergeConflictResolutionClientProvider, options mergeConflictResolutionOptions, file mergeConflictFile, timeout time.Duration) (mergeConflictFileResolution, error) {
	ours := mergeConflictFileResolution{Delete: !file.OursPresent, Content: file.Ours}
	theirs := mergeConflictFileResolution{Delete: !file.TheirsPresent, Content: file.Theirs}
	switch {
	case file.OursPresent == file.TheirsPresent && file.Ours == file.Theirs:
		return ours, nil
	case file.BasePresent == file.OursPresent && file.Base == file.Ours:
		return theirs, nil
	case file.BasePresent == file.TheirsPresent && file.Base == file.Theirs:
		return ours, nil
	}
	client, err := clientProvider()
	if err != nil {
		return mergeConflictFileResolution{}, err
	}
	request := llm.ChatRequest{Messages: []llm.Message{
		{Role: "system", Content: mergeConflictSelectionSystemPrompt},
		{Role: "user", Content: fmt.Sprintf(mergeConflictSelectionUserPrompt, file.Path, options.TargetBranch, options.SourceReference, file.Base, file.Ours, file.Theirs)},
	}, MaxTokens: service.commitMessages.MaxTokens}
	attemptTimeout := mergeConflictResolutionSemanticAttemptTimeout(service.commitMessages, timeout)
	for attempt := 1; attempt <= mergeConflictResolutionMaxSemanticAttempts; attempt++ {
		subject := fmt.Sprintf("%s %s attempt %d/%d", file.Path, mergeConflictSelectionStrategy, attempt, mergeConflictResolutionMaxSemanticAttempts)
		response, requestErr := service.requestMergeConflictResolution(ctx, client, request, subject, file.Path, attemptTimeout)
		if requestErr == nil && strings.TrimSpace(response) == "" {
			requestErr = fmt.Errorf(mergeConflictResolutionEmptyResponse, file.Path)
		}
		if requestErr != nil {
			service.reportSemanticProviderRoundFailed(file.Path, 0, 1, attempt, mergeConflictSelectionStrategy, requestErr)
			return mergeConflictFileResolution{}, requestErr
		}
		var resolution mergeConflictFileResolution
		switch strings.TrimSpace(response) {
		case mergeConflictSelectOurs:
			resolution = ours
		case mergeConflictSelectTheirs:
			resolution = theirs
		default:
			feedback := fmt.Sprintf(mergeConflictSelectionError, file.Path)
			service.reportSemanticAttemptRejected(file.Path, 0, 1, attempt, mergeConflictSelectionStrategy, feedback, false)
			request.Messages = append(request.Messages, llm.Message{Role: "user", Content: feedback})
			continue
		}
		service.report(shared.EventLevelInfo, shared.EventCodeAIMergeValidation,
			fmt.Sprintf("resolved marker-free conflict for %s with explicit selection %s", file.Path, strings.TrimSpace(response)), map[string]string{"path": file.Path, "strategy": mergeConflictSelectionStrategy})
		return resolution, nil
	}
	return mergeConflictFileResolution{}, fmt.Errorf(mergeConflictResolutionExhaustedTemplate, file.Path, 1, mergeConflictResolutionMaxSemanticAttempts, fmt.Errorf(mergeConflictSelectionError, file.Path))
}
