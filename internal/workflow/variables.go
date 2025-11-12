package workflow

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var workflowVariableNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// VariableName identifies a stored workflow variable.
type VariableName string

// NewVariableName normalizes and validates variable identifiers.
func NewVariableName(raw string) (VariableName, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("workflow variable name cannot be empty")
	}
	if !workflowVariableNamePattern.MatchString(trimmed) {
		return "", fmt.Errorf("workflow variable name %q must match %s", trimmed, workflowVariableNamePattern.String())
	}
	return VariableName(trimmed), nil
}

// VariableStore stores workflow variables with concurrent access safety.
type VariableStore struct {
	mutex  sync.RWMutex
	values map[VariableName]variableEntry
}

type variableEntry struct {
	value  string
	locked bool
}

// NewVariableStore constructs an empty variable store.
func NewVariableStore() *VariableStore {
	return &VariableStore{values: make(map[VariableName]variableEntry)}
}

// Seed assigns an immutable user-provided value.
func (store *VariableStore) Seed(name VariableName, value string) {
	store.set(name, value, true)
}

// Set assigns a value produced by workflow actions.
func (store *VariableStore) Set(name VariableName, value string) {
	store.set(name, value, false)
}

func (store *VariableStore) set(name VariableName, value string, locked bool) {
	if store == nil {
		return
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()

	entry, exists := store.values[name]
	switch {
	case exists && entry.locked && !locked:
		return
	case locked:
		store.values[name] = variableEntry{value: strings.TrimSpace(value), locked: true}
	default:
		store.values[name] = variableEntry{value: strings.TrimSpace(value), locked: locked}
	}
}

// Get looks up the value for the provided variable name.
func (store *VariableStore) Get(name VariableName) (string, bool) {
	if store == nil {
		return "", false
	}
	store.mutex.RLock()
	entry, exists := store.values[name]
	store.mutex.RUnlock()
	return entry.value, exists
}

// Snapshot returns a copy of the stored variables keyed by string names.
func (store *VariableStore) Snapshot() map[string]string {
	if store == nil {
		return nil
	}
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	snapshot := make(map[string]string, len(store.values))
	for name, entry := range store.values {
		snapshot[string(name)] = entry.value
	}
	return snapshot
}
