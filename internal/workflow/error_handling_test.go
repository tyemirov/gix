package workflow

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/audit"
	repoerrors "github.com/temirov/gix/internal/repos/errors"
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

	environment := &Environment{
		Errors: errorBuffer,
		State:  state,
	}

	repositoryError := repoerrors.WrapMessage(
		repoerrors.OperationCanonicalRemote,
		"/repositories/sample",
		repoerrors.ErrOriginOwnerMissing,
		"UPDATE-REMOTE-SKIP: /repositories/sample (error: could not parse origin owner/repo)\n",
	)

	require.True(testInstance, logRepositoryOperationError(environment, repositoryError))
	require.Equal(
		testInstance,
		"origin_owner_missing: canonical/example (/repositories/sample) UPDATE-REMOTE-SKIP: /repositories/sample (error: could not parse origin owner/repo)\n",
		errorBuffer.String(),
	)
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

	environment := &Environment{
		Output: io.Discard,
		Errors: errorBuffer,
		State:  state,
	}

	operation := &RenameOperation{}

	executionError := operation.Execute(context.Background(), environment, state)
	require.NoError(testInstance, executionError)

	require.Equal(
		testInstance,
		"filesystem_unavailable: canonical/example (/repositories/legacy) filesystem unavailable\n",
		errorBuffer.String(),
	)
}
