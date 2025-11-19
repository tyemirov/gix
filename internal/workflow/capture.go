package workflow

import (
	"fmt"
	"strings"
)

// CaptureKind enumerates supported capture types for branch actions.
type CaptureKind string

const (
	CaptureKindBranch CaptureKind = "branch"
	CaptureKindCommit CaptureKind = "commit"

	captureOptionKey   = "capture"
	restoreOptionKey   = "restore"
	captureVariableKey = "variable"
	captureKindKey     = "value"
	restoreVariableKey = "variable"
	restoreKindKey     = "value"
)

// BranchCaptureSpec describes a capture directive attached to branch.change.
type BranchCaptureSpec struct {
	Name VariableName
	Kind CaptureKind
}

// BranchRestoreSpec describes a restore directive attached to branch.change.
type BranchRestoreSpec struct {
	Name         VariableName
	Kind         CaptureKind
	KindExplicit bool
}

// ParseBranchCaptureSpec extracts a capture specification from action options.
func ParseBranchCaptureSpec(options map[string]any) (*BranchCaptureSpec, error) {
	reader := newOptionReader(options)
	captureMap, exists, err := reader.mapValue(captureOptionKey)
	if err != nil || !exists {
		return nil, err
	}

	captureReader := newOptionReader(captureMap)
	variableValue, variableExists, variableErr := captureReader.stringValue(captureVariableKey)
	if variableErr != nil {
		return nil, variableErr
	}
	if !variableExists || len(strings.TrimSpace(variableValue)) == 0 {
		return nil, fmt.Errorf("branch.change capture requires %q", captureVariableKey)
	}
	name, nameErr := NewVariableName(variableValue)
	if nameErr != nil {
		return nil, nameErr
	}

	kindValue, kindExists, kindErr := captureReader.stringValue(captureKindKey)
	if kindErr != nil {
		return nil, kindErr
	}
	if !kindExists {
		return nil, fmt.Errorf("branch.change capture requires %q", captureKindKey)
	}
	kind, kindErr := parseCaptureKind(kindValue)
	if kindErr != nil {
		return nil, kindErr
	}

	return &BranchCaptureSpec{Name: name, Kind: kind}, nil
}

// ParseBranchRestoreSpec extracts a restore specification from action options.
func ParseBranchRestoreSpec(options map[string]any) (*BranchRestoreSpec, error) {
	reader := newOptionReader(options)
	restoreMap, exists, err := reader.mapValue(restoreOptionKey)
	if err != nil || !exists {
		return nil, err
	}

	restoreReader := newOptionReader(restoreMap)
	variableValue, variableExists, variableErr := restoreReader.stringValue(restoreVariableKey)
	if variableErr != nil {
		return nil, variableErr
	}
	if !variableExists || len(strings.TrimSpace(variableValue)) == 0 {
		return nil, fmt.Errorf("branch.change restore requires %q", restoreVariableKey)
	}
	name, nameErr := NewVariableName(variableValue)
	if nameErr != nil {
		return nil, nameErr
	}

	kindValue, kindExists, kindErr := restoreReader.stringValue(restoreKindKey)
	if kindErr != nil {
		return nil, kindErr
	}

	spec := &BranchRestoreSpec{Name: name}
	if kindExists {
		kind, parseErr := parseCaptureKind(kindValue)
		if parseErr != nil {
			return nil, parseErr
		}
		spec.Kind = kind
		spec.KindExplicit = true
	}

	return spec, nil
}

func parseCaptureKind(raw string) (CaptureKind, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(CaptureKindBranch):
		return CaptureKindBranch, nil
	case string(CaptureKindCommit):
		return CaptureKindCommit, nil
	default:
		return "", fmt.Errorf("unsupported capture value %q", raw)
	}
}
