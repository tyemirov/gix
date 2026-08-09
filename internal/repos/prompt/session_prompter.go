package prompt

import (
	"sync"
	"sync/atomic"

	"github.com/tyemirov/gix/internal/repos/shared"
)

// SessionState tracks the shared confirmation policy across a command session.
type SessionState struct {
	assumeYes atomic.Bool
}

// NewSessionState constructs a SessionState initialized with the provided value.
func NewSessionState(initialAssumeYes bool) *SessionState {
	state := &SessionState{}
	state.assumeYes.Store(initialAssumeYes)
	return state
}

// IsAssumeYesEnabled reports whether prompts should be bypassed.
func (state *SessionState) IsAssumeYesEnabled() bool {
	if state == nil {
		return false
	}
	return state.assumeYes.Load()
}

// EnableAssumeYes permanently enables the assume-yes behavior for subsequent prompts.
func (state *SessionState) EnableAssumeYes() {
	if state == nil {
		return
	}
	state.assumeYes.Store(true)
}

type sessionPrompter struct {
	basePrompter shared.ConfirmationPrompter
	promptState  *SessionState
	mutex        sync.Mutex
}

// NewSessionPrompter wraps a base prompter so selecting Apply-to-All upgrades the shared state.
func NewSessionPrompter(base shared.ConfirmationPrompter, state *SessionState) shared.ConfirmationPrompter {
	if base == nil {
		return nil
	}
	return &sessionPrompter{basePrompter: base, promptState: state}
}

func (dispatcher *sessionPrompter) Confirm(prompt string) (shared.ConfirmationResult, error) {
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
