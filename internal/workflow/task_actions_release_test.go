package workflow

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/execshell"
)

type retagActionExecutor struct {
	commands []execshell.CommandDetails
}

func (executor *retagActionExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	if len(details.Arguments) >= 3 && details.Arguments[0] == "rev-parse" && details.Arguments[2] == "refs/tags/v1.0.0" {
		return execshell.ExecutionResult{StandardOutput: "oldtag\n"}, nil
	}
	if len(details.Arguments) >= 2 && details.Arguments[0] == "rev-parse" {
		return execshell.ExecutionResult{StandardOutput: "abcdef\n"}, nil
	}
	return execshell.ExecutionResult{}, nil
}

func (executor *retagActionExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func TestHandleReleaseRetagRequiresMappings(t *testing.T) {
	environment := &Environment{}
	repository := &RepositoryState{Path: "/tmp/repo"}

	err := handleReleaseRetagAction(context.Background(), environment, repository, map[string]any{})
	require.Error(t, err)
}

func TestHandleReleaseRetagExecutesService(t *testing.T) {
	executor := &retagActionExecutor{}
	buffer := &bytes.Buffer{}
	environment := &Environment{
		GitExecutor: executor,
		Output:      buffer,
	}
	repository := &RepositoryState{Path: "/tmp/repo"}
	parameters := map[string]any{
		"remote": "origin",
		"mappings": []any{
			map[string]any{"tag": "v1.0.0", "target": "main", "message": "Retag v1.0.0"},
		},
	}

	require.NoError(t, handleReleaseRetagAction(context.Background(), environment, repository, parameters))
	require.Contains(t, buffer.String(), "RETAGGED")
	require.True(t, len(executor.commands) >= 4)
}
