package workflow

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/repos/shared"
)

func TestWorkflowHumanFormatterCollapsesTasks(t *testing.T) {
	formatter := newWorkflowHumanFormatter()
	var buffer bytes.Buffer

	repo := "tyemirov/scheduler"
	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskPlan,
		RepositoryIdentifier: repo,
		Message:              "task plan ready",
		Details:              map[string]string{"task": "Ensure gitignore entries"},
	}, &buffer)
	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeRepoSwitched,
		RepositoryIdentifier: repo,
		Message:              "→ master",
		Details:              map[string]string{"branch": "master"},
	}, &buffer)
	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskApply,
		RepositoryIdentifier: repo,
	}, &buffer)
	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskSkip,
		RepositoryIdentifier: repo,
		Message:              "PULL-SKIP: missing tracking",
	}, &buffer)

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	require.Equal(t, "-- tyemirov/scheduler --", lines[0])
	require.Equal(t, "  ↪ switched to master", lines[1])
	require.Equal(t, "  ✓ Ensure gitignore entries", lines[2])
	require.Equal(t, "  ⚠ PULL-SKIP: missing tracking", lines[3])
}

func TestWorkflowHumanFormatterHandlesMultipleRepositories(t *testing.T) {
	formatter := newWorkflowHumanFormatter()
	var buffer bytes.Buffer

	firstRepo := "tyemirov/alpha"
	secondRepo := "tyemirov/beta"

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskPlan,
		RepositoryIdentifier: firstRepo,
		Details:              map[string]string{"task": "Switch to master"},
	}, &buffer)
	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskApply,
		RepositoryIdentifier: firstRepo,
	}, &buffer)

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskPlan,
		RepositoryIdentifier: secondRepo,
		Details:              map[string]string{"task": "Open Pull Request"},
	}, &buffer)
	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskApply,
		RepositoryIdentifier: secondRepo,
	}, &buffer)

	output := buffer.String()
	require.Contains(t, output, "-- tyemirov/alpha --")
	require.Contains(t, output, "-- tyemirov/beta --")
	require.Contains(t, output, "  ✓ Switch to master")
	require.Contains(t, output, "  ✓ Open Pull Request")
}

func TestWorkflowHumanFormatterWritesEventSummary(t *testing.T) {
	formatter := newWorkflowHumanFormatter()
	var buffer bytes.Buffer

	formatter.HandleEvent(shared.Event{
		Code:                 "PROTOCOL_UPDATE",
		RepositoryIdentifier: "canonical/example",
		Details:              map[string]string{"path": "/tmp/repo"},
	}, &buffer)

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	require.Contains(t, lines, "-- canonical/example --")
	found := false
	for _, line := range lines {
		if strings.Contains(line, "event=PROTOCOL_UPDATE path=/tmp/repo") {
			found = true
			break
		}
	}
	require.True(t, found, "expected protocol update summary")
}
