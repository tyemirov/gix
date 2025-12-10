package workflow

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/repos/shared"
)

func TestWorkflowHumanFormatterGroupsPhases(t *testing.T) {
	formatter := NewHumanEventFormatter()
	var buffer bytes.Buffer

	repo := "tyemirov/scheduler"
	path := "/tmp/repos/scheduler"

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskPlan,
		RepositoryIdentifier: repo,
		RepositoryPath:       path,
		Details:              map[string]string{"task": "Switch to master"},
	}, &buffer)

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeRemoteUpdate,
		RepositoryIdentifier: repo,
		RepositoryPath:       path,
		Message:              "origin now ssh://git@github.com/tyemirov/scheduler.git",
	}, &buffer)

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeRepoSwitched,
		RepositoryIdentifier: repo,
		RepositoryPath:       path,
		Details: map[string]string{
			"branch":  "master",
			"created": "true",
		},
	}, &buffer)

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskApply,
		RepositoryIdentifier: repo,
		RepositoryPath:       path,
		Details: map[string]string{
			"phase": string(LogPhaseBranch),
			"task":  "Switch to master",
		},
	}, &buffer)

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskPlan,
		RepositoryIdentifier: repo,
		RepositoryPath:       path,
		Details:              map[string]string{"task": "Ensure gitignore entries"},
	}, &buffer)

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskApply,
		RepositoryIdentifier: repo,
		RepositoryPath:       path,
		Details: map[string]string{
			"phase": string(LogPhaseFiles),
			"task":  "Ensure gitignore entries",
		},
	}, &buffer)

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskPlan,
		RepositoryIdentifier: repo,
		RepositoryPath:       path,
		Details:              map[string]string{"task": "Git Stage Commit"},
	}, &buffer)

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskApply,
		RepositoryIdentifier: repo,
		RepositoryPath:       path,
		Details: map[string]string{
			"phase": string(LogPhaseGit),
			"task":  "Git Stage Commit",
		},
	}, &buffer)

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	require.Equal(t, []string{
		"-- tyemirov/scheduler (/tmp/repos/scheduler) --",
		"  • remote/folder:",
		"    - origin now ssh://git@github.com/tyemirov/scheduler.git",
		"  • branch:",
		"    - master (created)",
		"  • files:",
		"    - Ensure gitignore entries",
		"  • git:",
		"    - Git Stage Commit",
	}, lines)
}

func TestWorkflowHumanFormatterHandlesMultipleRepositories(t *testing.T) {
	formatter := NewHumanEventFormatter()
	var buffer bytes.Buffer

	firstRepo := "tyemirov/alpha"
	secondRepo := "tyemirov/beta"

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskPlan,
		RepositoryIdentifier: firstRepo,
		RepositoryPath:       "/tmp/repos/alpha",
		Details:              map[string]string{"task": "Update files"},
	}, &buffer)
	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskApply,
		RepositoryIdentifier: firstRepo,
		RepositoryPath:       "/tmp/repos/alpha",
		Details: map[string]string{
			"phase": string(LogPhaseFiles),
			"task":  "Update files",
		},
	}, &buffer)

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskPlan,
		RepositoryIdentifier: secondRepo,
		RepositoryPath:       "/tmp/repos/beta",
		Details:              map[string]string{"task": "Push branch"},
	}, &buffer)
	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskApply,
		RepositoryIdentifier: secondRepo,
		RepositoryPath:       "/tmp/repos/beta",
		Details: map[string]string{
			"phase": string(LogPhaseGit),
			"task":  "Push branch",
		},
	}, &buffer)

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	require.Equal(t, []string{
		"-- tyemirov/alpha (/tmp/repos/alpha) --",
		"  • files:",
		"    - Update files",
		"",
		"-- tyemirov/beta (/tmp/repos/beta) --",
		"  • git:",
		"    - Push branch",
	}, lines)
}

func TestWorkflowHumanFormatterFallbackForBranchPhaseWithoutSwitch(t *testing.T) {
	formatter := NewHumanEventFormatter()
	var buffer bytes.Buffer

	repo := "tyemirov/solo"
	path := "/tmp/repos/solo"

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskPlan,
		RepositoryIdentifier: repo,
		RepositoryPath:       path,
		Details:              map[string]string{"task": "Switch branch"},
	}, &buffer)
	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskApply,
		RepositoryIdentifier: repo,
		RepositoryPath:       path,
		Details: map[string]string{
			"phase": string(LogPhaseBranch),
			"task":  "Switch branch",
		},
	}, &buffer)

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	require.Equal(t, []string{
		"-- tyemirov/solo (/tmp/repos/solo) --",
		"  • branch:",
		"    - Switch branch",
	}, lines)
}

func TestWorkflowHumanFormatterWritesEventSummary(t *testing.T) {
	formatter := NewHumanEventFormatter()
	var buffer bytes.Buffer

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeWorkflowOperationSuccess,
		RepositoryIdentifier: "canonical/example",
		RepositoryPath:       "/tmp/repos/example",
		Details:              map[string]string{"path": "/tmp/repos/example"},
	}, &buffer)

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	require.Contains(t, lines, "-- canonical/example (/tmp/repos/example) --")
	found := false
	for _, line := range lines {
		if strings.Contains(line, "event=WORKFLOW_OPERATION_SUCCESS path=/tmp/repos/example") {
			found = true
			break
		}
	}
	require.True(t, found, "expected workflow summary")
}

func TestWorkflowHumanFormatterPreservesSeverityForPhaseEvents(t *testing.T) {
	formatter := NewHumanEventFormatter()
	var buffer bytes.Buffer

	repositoryIdentifier := "tyemirov/severity"
	repositoryPath := "/tmp/repos/severity"

	formatter.HandleEvent(shared.Event{
		Level:                shared.EventLevelWarn,
		Code:                 shared.EventCodeRemoteSkip,
		RepositoryIdentifier: repositoryIdentifier,
		RepositoryPath:       repositoryPath,
		Message:              "missing origin owner",
	}, &buffer)

	formatter.HandleEvent(shared.Event{
		Level:                shared.EventLevelError,
		Code:                 shared.EventCodeNamespaceError,
		RepositoryIdentifier: repositoryIdentifier,
		RepositoryPath:       repositoryPath,
		Message:              "template validation failed",
	}, &buffer)

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	require.Equal(t, []string{
		"-- tyemirov/severity (/tmp/repos/severity) --",
		"  • remote/folder:",
		"    - ⚠ missing origin owner",
		"  • files:",
		"    - ✖ template validation failed",
	}, lines)
}

func TestWorkflowHumanFormatterRecordsIssues(t *testing.T) {
	formatter := NewHumanEventFormatter()
	var buffer bytes.Buffer

	repo := "tyemirov/issues"
	path := "/tmp/repos/issues"

	formatter.HandleEvent(shared.Event{
		Code:                 shared.EventCodeTaskSkip,
		Level:                shared.EventLevelWarn,
		RepositoryIdentifier: repo,
		RepositoryPath:       path,
		Message:              "git pull declined",
	}, &buffer)

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	require.Equal(t, []string{
		"-- tyemirov/issues (/tmp/repos/issues) --",
		"    issues:",
		"      - ⚠ git pull declined",
	}, lines)
}
