package workflow

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/audit"
	repoerrors "github.com/tyemirov/gix/internal/repos/errors"
	"github.com/tyemirov/gix/internal/repos/shared"
)

func TestLogRepositoryOperationErrorFormatsStructuredMessage(testInstance *testing.T) {
	errorBuffer := &bytes.Buffer{}

	state := &State{
		Repositories: []*RepositoryState{
			{
				Path: "/repositories/sample",
				Inspection: audit.RepositoryInspection{
					FinalOwnerRepo: "canonical/example",
				},
			},
		},
	}

	reporter := shared.NewStructuredReporter(io.Discard, errorBuffer, shared.WithRepositoryHeaders(false))
	environment := &Environment{
		Output:   io.Discard,
		Errors:   errorBuffer,
		State:    state,
		Reporter: reporter,
	}

	repositoryError := repoerrors.WrapMessage(
		repoerrors.OperationCanonicalRemote,
		"/repositories/sample",
		repoerrors.ErrOriginOwnerMissing,
		"UPDATE-REMOTE-SKIP: /repositories/sample (error: could not parse origin owner/repo)\n",
	)

	require.True(testInstance, logRepositoryOperationError(environment, repositoryError))
	events := parseStructuredEvents(errorBuffer.String())
	require.Len(testInstance, events, 1)
	event := requireEventByCode(testInstance, events, strings.ToUpper(string(repoerrors.ErrOriginOwnerMissing)))
	require.Equal(testInstance, "/repositories/sample", event["path"])
	require.Equal(testInstance, "canonical/example", event["repo"])
}

func TestRenameOperationLogsStructuredErrors(testInstance *testing.T) {
	errorBuffer := &bytes.Buffer{}

	state := &State{
		Repositories: []*RepositoryState{
			{
				Path: "/repositories/legacy",
				Inspection: audit.RepositoryInspection{
					FinalOwnerRepo:    "canonical/example",
					DesiredFolderName: "example",
					FolderName:        "legacy",
				},
			},
		},
	}

	reporter := shared.NewStructuredReporter(io.Discard, errorBuffer, shared.WithRepositoryHeaders(false))
	environment := &Environment{
		Output:   io.Discard,
		Errors:   errorBuffer,
		State:    state,
		Reporter: reporter,
	}

	operation := &RenameOperation{}

	executionError := operation.Execute(context.Background(), environment, state)
	require.NoError(testInstance, executionError)

	events := parseStructuredEvents(errorBuffer.String())
	require.Len(testInstance, events, 1)
	event := requireEventByCode(testInstance, events, strings.ToUpper(string(repoerrors.ErrFilesystemUnavailable)))
	require.Equal(testInstance, "/repositories/legacy", event["path"])
	require.Equal(testInstance, "canonical/example", event["repo"])
}
