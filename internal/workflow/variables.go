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
	values map[VariableName]string
}

// NewVariableStore constructs an empty variable store.
func NewVariableStore() *VariableStore {
	return &VariableStore{values: make(map[VariableName]string)}
}

// Set assigns a value to the provided variable name.
func (store *VariableStore) Set(name VariableName, value string) {
	if store == nil {
		return
	}
	store.mutex.Lock()
	store.values[name] = strings.TrimSpace(value)
	store.mutex.Unlock()
}

// Get looks up the value for the provided variable name.
func (store *VariableStore) Get(name VariableName) (string, bool) {
	if store == nil {
		return "", false
	}
	store.mutex.RLock()
	value, exists := store.values[name]
	store.mutex.RUnlock()
	return value, exists
}

// Snapshot returns a copy of the stored variables keyed by string names.
func (store *VariableStore) Snapshot() map[string]string {
	if store == nil {
		return nil
	}
	store.mutex.RLock()
	defer store.mutex.RUnlock()
	snapshot := make(map[string]string, len(store.values))
	for name, value := range store.values {
		snapshot[string(name)] = value
	}
	return snapshot
}
