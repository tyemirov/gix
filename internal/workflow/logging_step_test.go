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

	ownerRepository, err := shared.NewOwnerRepository("tyemirov/gix")
	require.NoError(t, err)

	dependencies := remotes.Dependencies{
		Reporter: environment.stepScopedReporter(),
	}

	options := remotes.Options{
		RepositoryPath:           repositoryPath,
		OriginOwnerRepository:    &ownerRepository,
		CanonicalOwnerRepository: &ownerRepository,
		RemoteProtocol:           shared.RemoteProtocolSSH,
		ConfirmationPolicy:       shared.ConfirmationPolicyFromBool(true),
	}

	require.NoError(t, remotes.Execute(context.Background(), dependencies, options))

	require.Len(t, capture.events, 1)
	require.Equal(t, "remotes", capture.events[0].Details["step"])
}
