package workflow

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/repos/remotes"
	"github.com/tyemirov/gix/internal/repos/shared"
)

type capturingFormatter struct {
	events []shared.Event
}

func (formatter *capturingFormatter) HandleEvent(event shared.Event, _ io.Writer) {
	formatter.events = append(formatter.events, event)
}

type stepLoggingGitRepositoryManager struct {
	setRemoteURLError error
}

func (manager *stepLoggingGitRepositoryManager) CheckCleanWorktree(context.Context, string) (bool, error) {
	return true, nil
}

func (manager *stepLoggingGitRepositoryManager) WorktreeStatus(context.Context, string) ([]string, error) {
	return nil, nil
}

func (manager *stepLoggingGitRepositoryManager) GetCurrentBranch(context.Context, string) (string, error) {
	return "master", nil
}

func (manager *stepLoggingGitRepositoryManager) GetRemoteURL(context.Context, string, string) (string, error) {
	return "", nil
}

func (manager *stepLoggingGitRepositoryManager) SetRemoteURL(context.Context, string, string, string) error {
	return manager.setRemoteURLError
}

func TestReportRepositoryEventIncludesStepDetail(t *testing.T) {
	repository := &RepositoryState{
		Path: "/tmp/repos/step-test",
	}

	capture := &capturingFormatter{events: make([]shared.Event, 0, 1)}
	reporter := shared.NewStructuredReporter(
		&bytes.Buffer{},
		&bytes.Buffer{},
		shared.WithEventFormatter(capture),
	)

	environment := &Environment{
		Reporter:        reporter,
		currentStepName: "remotes",
	}

	environment.ReportRepositoryEvent(
		repository,
		shared.EventLevelInfo,
		shared.EventCodeRemoteUpdate,
		"origin now ssh://git@github.com/canonical/example.git",
		map[string]string{},
	)

	require.Len(t, capture.events, 1)
	event := capture.events[0]
	require.Equal(t, "remotes", event.Details["step"])
}

func TestStepScopedReporterInjectsStepForRepositoryExecutors(t *testing.T) {
	capture := &capturingFormatter{events: make([]shared.Event, 0, 1)}
	reporter := shared.NewStructuredReporter(
		&bytes.Buffer{},
		&bytes.Buffer{},
		shared.WithEventFormatter(capture),
	)

	environment := &Environment{
		Reporter:        reporter,
		currentStepName: "remotes",
	}

	repositoryPath, err := shared.NewRepositoryPath("/tmp/repos/remotes-step-test")
	require.NoError(t, err)

	originOwnerRepository, err := shared.NewOwnerRepository("origin/example")
	require.NoError(t, err)
	canonicalOwnerRepository, err := shared.NewOwnerRepository("canonical/example")
	require.NoError(t, err)

	dependencies := remotes.Dependencies{
		GitManager: &stepLoggingGitRepositoryManager{},
		Reporter:   environment.stepScopedReporter(),
	}

	options := remotes.Options{
		RepositoryPath:           repositoryPath,
		OriginOwnerRepository:    &originOwnerRepository,
		CanonicalOwnerRepository: &canonicalOwnerRepository,
		RemoteProtocol:           shared.RemoteProtocolSSH,
		ConfirmationPolicy:       shared.ConfirmationPolicyFromBool(true),
	}

	require.NoError(t, remotes.Execute(context.Background(), dependencies, options))

	require.Len(t, capture.events, 1)
	require.Equal(t, shared.EventCodeRemoteUpdate, capture.events[0].Code)
	require.Equal(t, "remotes", capture.events[0].Details["step"])
}
