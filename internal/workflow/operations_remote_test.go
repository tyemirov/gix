package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/audit"
	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/gitrepo"
	"github.com/temirov/gix/internal/repos/shared"
)

func TestCanonicalRemoteOperationSkipsWhenRemoteMissing(testInstance *testing.T) {
	errorBuffer := &bytes.Buffer{}

	repositoryPath := "/tmp/workflow/repository"
	state := &State{
		Repositories: []*RepositoryState{
			{
				Path: repositoryPath,
				Inspection: audit.RepositoryInspection{
					RemoteProtocol: shared.RemoteProtocolHTTPS,
				},
			},
		},
	}

	commandExecutor := &remoteCommandExecutor{
		responses: map[string]commandResponse{
			"remote get-url origin": {err: errors.New("remote not found")},
		},
	}
	repositoryManager, managerError := gitrepo.NewRepositoryManager(commandExecutor)
	require.NoError(testInstance, managerError)

	environment := &Environment{
		RepositoryManager: repositoryManager,
		GitExecutor:       commandExecutor,
		Output:            io.Discard,
		Errors:            errorBuffer,
	}

	operation := &CanonicalRemoteOperation{}
	require.NoError(testInstance, operation.Execute(context.Background(), environment, state))

	require.Contains(
		testInstance,
		errorBuffer.String(),
		"remote_missing: local-only (/tmp/workflow/repository) SKIP: remote 'origin' not configured",
	)
}

func TestCanonicalRemoteOperationWarnsWhenMetadataUnavailable(testInstance *testing.T) {
	errorBuffer := &bytes.Buffer{}

	repositoryPath := "/tmp/workflow/legacy"
	state := &State{
		Repositories: []*RepositoryState{
			{
				Path: repositoryPath,
				Inspection: audit.RepositoryInspection{
					OriginURL:      "ssh://git.example.com/legacy",
					RemoteProtocol: shared.RemoteProtocolSSH,
				},
			},
		},
	}

	commandExecutor := &remoteCommandExecutor{
		responses: map[string]commandResponse{
			"remote get-url origin": {result: execshell.ExecutionResult{StandardOutput: "ssh://git.example.com/legacy\n"}},
		},
	}
	repositoryManager, managerError := gitrepo.NewRepositoryManager(commandExecutor)
	require.NoError(testInstance, managerError)

	environment := &Environment{
		RepositoryManager: repositoryManager,
		GitExecutor:       commandExecutor,
		Output:            io.Discard,
		Errors:            errorBuffer,
	}

	operation := &CanonicalRemoteOperation{}
	require.NoError(testInstance, operation.Execute(context.Background(), environment, state))

	require.Contains(
		testInstance,
		errorBuffer.String(),
		"origin_owner_missing: local-only (/tmp/workflow/legacy) SKIP: remote metadata unavailable for remote 'origin'",
	)
}

type commandResponse struct {
	result execshell.ExecutionResult
	err    error
}

type remoteCommandExecutor struct {
	responses map[string]commandResponse
}

func (executor *remoteCommandExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	key := strings.Join(details.Arguments, " ")
	if response, found := executor.responses[key]; found {
		return response.result, response.err
	}
	return execshell.ExecutionResult{}, fmt.Errorf("unexpected git command: %s", key)
}

func (executor *remoteCommandExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, errors.New("github cli unavailable")
}
