package workflow

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVariableStoreSeedPreventsCapturedOverride(t *testing.T) {
	name, err := NewVariableName("generated_commit")
	require.NoError(t, err)

	store := NewVariableStore()
	store.Seed(name, "user-value")
	store.Set(name, "captured-value")

	value, exists := store.Get(name)
	require.True(t, exists)
	require.Equal(t, "user-value", value)
}

func TestVariableStoreSeedOverridesCapturedValue(t *testing.T) {
	name, err := NewVariableName("generated_commit")
	require.NoError(t, err)

	store := NewVariableStore()
	store.Set(name, "captured-value")
	store.Seed(name, "user-value")

	value, exists := store.Get(name)
	require.True(t, exists)
	require.Equal(t, "user-value", value)
}

func TestVariableStoreSetOverridesPreviousCapturedValue(t *testing.T) {
	name, err := NewVariableName("generated_commit")
	require.NoError(t, err)

	store := NewVariableStore()
	store.Set(name, "first")
	store.Set(name, "second")

	value, exists := store.Get(name)
	require.True(t, exists)
	require.Equal(t, "second", value)
}
