package workflow

import (
	"sync"
	"sync/atomic"

	"github.com/tyemirov/gix/internal/repos/shared"
)

// PromptState tracks the shared confirmation policy across operations.
type PromptState struct {
	assumeYes atomic.Bool
}

// NewPromptState constructs a PromptState initialized with the provided value.
func NewPromptState(initialAssumeYes bool) *PromptState {
	state := &PromptState{}
	state.assumeYes.Store(initialAssumeYes)
	return state
}

// IsAssumeYesEnabled reports whether prompts should be bypassed.
func (state *PromptState) IsAssumeYesEnabled() bool {
	if state == nil {
		return false
	}
	return state.assumeYes.Load()
}

// EnableAssumeYes permanently enables the assume-yes behavior for subsequent prompts.
func (state *PromptState) EnableAssumeYes() {
	if state == nil {
		return
	}
	state.assumeYes.Store(true)
}

type promptDispatcher struct {
	basePrompter shared.ConfirmationPrompter
	promptState  *PromptState
	mutex        sync.Mutex
}

func newPromptDispatcher(base shared.ConfirmationPrompter, state *PromptState) shared.ConfirmationPrompter {
	if base == nil {
		return nil
	}
	return &promptDispatcher{basePrompter: base, promptState: state}
}

func (dispatcher *promptDispatcher) Confirm(prompt string) (shared.ConfirmationResult, error) {
	if dispatcher.basePrompter == nil {
		return shared.ConfirmationResult{}, nil
	}
	dispatcher.mutex.Lock()
	defer dispatcher.mutex.Unlock()

	if dispatcher.promptState != nil && dispatcher.promptState.IsAssumeYesEnabled() {
		return shared.ConfirmationResult{Confirmed: true}, nil
	}

	result, confirmError := dispatcher.basePrompter.Confirm(prompt)
	if confirmError != nil {
		return shared.ConfirmationResult{}, confirmError
	}
	if result.ApplyToAll && dispatcher.promptState != nil {
		dispatcher.promptState.EnableAssumeYes()
	}
	return result, nil
}
