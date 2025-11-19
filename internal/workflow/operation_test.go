package workflow

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvironmentStoreCaptureValueRespectsOverwrite(t *testing.T) {
	name, nameErr := NewVariableName("initial_branch")
	require.NoError(t, nameErr)

	environment := &Environment{Variables: NewVariableStore()}
	environment.StoreCaptureValue(name, "first\n", true)
	environment.StoreCaptureValue(name, "second", false)

	storedValue, exists := environment.CapturedValue(name)
	require.True(t, exists)
	require.Equal(t, "first", storedValue)

	directValue, directExists := environment.Variables.Get(name)
	require.True(t, directExists)
	require.Equal(t, "first", directValue)

	capturedVariableName, capturedNameErr := NewVariableName("Captured.initial_branch")
	require.NoError(t, capturedNameErr)
	namespacedValue, namespacedExists := environment.Variables.Get(capturedVariableName)
	require.True(t, namespacedExists)
	require.Equal(t, "first", namespacedValue)

	environment.StoreCaptureValue(name, "updated", true)
	updatedValue, updatedExists := environment.CapturedValue(name)
	require.True(t, updatedExists)
	require.Equal(t, "updated", updatedValue)
}
