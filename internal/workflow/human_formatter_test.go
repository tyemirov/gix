package workflow

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/repos/shared"
)

func TestWorkflowHumanFormatterPrintsStepSummaries(t *testing.T) {
	formatter := NewHumanEventFormatter()
	var buffer bytes.Buffer

	repo := "tyemirov/scheduler"
	path := "/tmp/repos/scheduler"

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeWorkflowStepSummary,
		RepositoryIdentifier: repo,
		RepositoryPath:       path,
		Details: map[string]string{
			"step":    "protocol-to-git-https",
			"outcome": "no-op",
			"reason":  "already canonical",
		},
	}, &buffer)

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	require.Equal(t, []string{
		"-- tyemirov/scheduler (/tmp/repos/scheduler) --",
		"  step name: protocol-to-git-https",
		"  outcome: no-op",
		"  reason: already canonical",
	}, lines)
}

func TestWorkflowHumanFormatterSeparatesRepositories(t *testing.T) {
	formatter := NewHumanEventFormatter()
	var buffer bytes.Buffer

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeWorkflowStepSummary,
		RepositoryIdentifier: "tyemirov/alpha",
		RepositoryPath:       "/tmp/repos/alpha",
		Details: map[string]string{
			"step":    "folders",
			"outcome": "no-op",
			"reason":  "already normalized",
		},
	}, &buffer)

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeWorkflowStepSummary,
		RepositoryIdentifier: "tyemirov/beta",
		RepositoryPath:       "/tmp/repos/beta",
		Details: map[string]string{
			"step":    "remotes",
			"outcome": "applied",
			"reason":  "origin now git@github.com:tyemirov/beta.git",
		},
	}, &buffer)

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	require.Equal(t, []string{
		"-- tyemirov/alpha (/tmp/repos/alpha) --",
		"  step name: folders",
		"  outcome: no-op",
		"  reason: already normalized",
		"",
		"-- tyemirov/beta (/tmp/repos/beta) --",
		"  step name: remotes",
		"  outcome: applied",
		"  reason: origin now git@github.com:tyemirov/beta.git",
	}, lines)
}
