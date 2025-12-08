package prompt_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/repos/prompt"
	"github.com/tyemirov/gix/internal/repos/shared"
)

type stubConfirmationPrompter struct {
	responses           []shared.ConfirmationResult
	calls               int
	returnError         error
	defaultConfirmation shared.ConfirmationResult
}

func (prompter *stubConfirmationPrompter) Confirm(string) (shared.ConfirmationResult, error) {
	prompter.calls++
	if prompter.returnError != nil {
		return shared.ConfirmationResult{}, prompter.returnError
	}
	if len(prompter.responses) > 0 && prompter.calls <= len(prompter.responses) {
		return prompter.responses[prompter.calls-1], nil
	}
	return prompter.defaultConfirmation, nil
}

func TestSessionPrompterSkipsPromptWhenAssumeYesEnabled(testInstance *testing.T) {
	base := &stubConfirmationPrompter{
		defaultConfirmation: shared.ConfirmationResult{Confirmed: true},
	}
	state := prompt.NewSessionState(true)
	dispatcher := prompt.NewSessionPrompter(base, state)

	result, err := dispatcher.Confirm("Confirm action? ")
	require.NoError(testInstance, err)
	require.True(testInstance, result.Confirmed)
	require.Equal(testInstance, 0, base.calls)
}

func TestSessionPrompterEnablesAssumeYesOnApplyAll(testInstance *testing.T) {
	base := &stubConfirmationPrompter{
		responses: []shared.ConfirmationResult{
			{Confirmed: true, ApplyToAll: true},
		},
	}
	state := prompt.NewSessionState(false)
	dispatcher := prompt.NewSessionPrompter(base, state)

	firstResult, firstError := dispatcher.Confirm("Confirm action? ")
	require.NoError(testInstance, firstError)
	require.True(testInstance, firstResult.Confirmed)
	require.True(testInstance, firstResult.ApplyToAll)
	require.Equal(testInstance, 1, base.calls)
	require.True(testInstance, state.IsAssumeYesEnabled())

	secondResult, secondError := dispatcher.Confirm("Confirm another action? ")
	require.NoError(testInstance, secondError)
	require.True(testInstance, secondResult.Confirmed)
	require.False(testInstance, secondResult.ApplyToAll)
	require.Equal(testInstance, 1, base.calls)
}
