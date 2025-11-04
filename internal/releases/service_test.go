package releases

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
	repoerrors "github.com/tyemirov/gix/internal/repos/errors"
)

type recordingGitExecutor struct {
	commands []execshell.CommandDetails
	errors   []error
}

func (executor *recordingGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	if len(executor.errors) == 0 {
		return execshell.ExecutionResult{}, nil
	}
	value := executor.errors[0]
	executor.errors = executor.errors[1:]
	if value != nil {
		return execshell.ExecutionResult{}, value
	}
	return execshell.ExecutionResult{}, nil
}

func (executor *recordingGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func TestReleaseExecutesTagAndPush(t *testing.T) {
	executor := &recordingGitExecutor{}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	result, releaseError := service.Release(context.Background(), Options{RepositoryPath: "/tmp/repo", TagName: "v1.2.3", RemoteName: "origin"})
	require.NoError(t, releaseError)
	require.Equal(t, Result{RepositoryPath: "/tmp/repo", TagName: "v1.2.3"}, result)
	require.Len(t, executor.commands, 2)
}

func TestReleaseDryRunSkipsGitCommands(t *testing.T) {
	executor := &recordingGitExecutor{}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	_, releaseError := service.Release(context.Background(), Options{RepositoryPath: "/tmp/repo", TagName: "v1.0.0", DryRun: true})
	require.NoError(t, releaseError)
	require.Empty(t, executor.commands)
}

func TestReleaseValidatesInputs(t *testing.T) {
	service, err := NewService(ServiceDependencies{GitExecutor: &recordingGitExecutor{}})
	require.NoError(t, err)

	_, releaseError := service.Release(context.Background(), Options{TagName: "v1.0.0"})
	require.Error(t, releaseError)

	_, releaseError = service.Release(context.Background(), Options{RepositoryPath: "/tmp/repo"})
	require.Error(t, releaseError)
}

func TestReleasePropagatesErrors(t *testing.T) {
	executor := &genericErrorExecutor{err: errors.New("tag failed")}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	_, releaseError := service.Release(context.Background(), Options{RepositoryPath: "/tmp/repo", TagName: "v1.0.0"})
	require.Error(t, releaseError)

	var operationError repoerrors.OperationError
	require.ErrorAs(t, releaseError, &operationError)
	require.Equal(t, repoerrors.OperationReleaseTag, operationError.Operation())
	require.Equal(t, repoerrors.ErrReleaseTagCreateFailed.Code(), operationError.Code())
	require.Equal(t, "/tmp/repo", operationError.Subject())
	require.Contains(t, operationError.Message(), "tag failed")
}

func TestReleaseAnnotateFailureIncludesCommandDetails(t *testing.T) {
	executor := &annotateFailureExecutor{exitCode: 128, standardError: "fatal: tag 'v1.0.0' already exists"}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	_, releaseError := service.Release(context.Background(), Options{RepositoryPath: "/tmp/repo", TagName: "v1.0.0"})
	require.Error(t, releaseError)

	var operationError repoerrors.OperationError
	require.ErrorAs(t, releaseError, &operationError)
	require.Equal(t, repoerrors.OperationReleaseTag, operationError.Operation())
	require.Equal(t, repoerrors.ErrReleaseTagCreateFailed.Code(), operationError.Code())
	require.Equal(t, "/tmp/repo", operationError.Subject())
	require.Contains(t, operationError.Message(), `failed to create tag "v1.0.0"`)
	require.Contains(t, operationError.Message(), "git tag -a")
	require.Contains(t, operationError.Message(), `-m "Release v1.0.0"`)
	require.Contains(t, operationError.Message(), "exit code 128")
	require.Contains(t, operationError.Message(), "fatal: tag 'v1.0.0' already exists")

	var commandFailed execshell.CommandFailedError
	require.ErrorAs(t, releaseError, &commandFailed)
	require.Equal(t, 128, commandFailed.Result.ExitCode)
}

func TestReleasePushFailureIncludesCommandDetails(t *testing.T) {
	executor := &pushFailureExecutor{exitCode: 1, standardError: "remote: permission denied"}
	service, err := NewService(ServiceDependencies{GitExecutor: executor})
	require.NoError(t, err)

	_, releaseError := service.Release(context.Background(), Options{RepositoryPath: "/tmp/repo", TagName: "v1.0.0", RemoteName: "origin"})
	require.Error(t, releaseError)

	var operationError repoerrors.OperationError
	require.ErrorAs(t, releaseError, &operationError)
	require.Equal(t, repoerrors.OperationReleaseTag, operationError.Operation())
	require.Equal(t, repoerrors.ErrReleaseTagPushFailed.Code(), operationError.Code())
	require.Equal(t, "/tmp/repo", operationError.Subject())
	require.Contains(t, operationError.Message(), `failed to push tag "v1.0.0" to origin`)
	require.Contains(t, operationError.Message(), "git push origin v1.0.0")
	require.Contains(t, operationError.Message(), "exit code 1")
	require.Contains(t, operationError.Message(), "remote: permission denied")

	var commandFailed execshell.CommandFailedError
	require.ErrorAs(t, releaseError, &commandFailed)
	require.Equal(t, 1, commandFailed.Result.ExitCode)
}

type genericErrorExecutor struct {
	err error
}

func (executor *genericErrorExecutor) ExecuteGit(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, executor.err
}

func (executor *genericErrorExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type annotateFailureExecutor struct {
	exitCode      int
	standardError string
}

func (executor *annotateFailureExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, execshell.CommandFailedError{
		Command: execshell.ShellCommand{
			Name:    execshell.CommandGit,
			Details: details,
		},
		Result: execshell.ExecutionResult{
			ExitCode:      executor.exitCode,
			StandardError: executor.standardError,
		},
	}
}

func (executor *annotateFailureExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type pushFailureExecutor struct {
	callIndex     int
	exitCode      int
	standardError string
}

func (executor *pushFailureExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.callIndex++
	if executor.callIndex == 1 {
		return execshell.ExecutionResult{}, nil
	}
	return execshell.ExecutionResult{}, execshell.CommandFailedError{
		Command: execshell.ShellCommand{
			Name:    execshell.CommandGit,
			Details: details,
		},
		Result: execshell.ExecutionResult{
			ExitCode:      executor.exitCode,
			StandardError: executor.standardError,
		},
	}
}

func (executor *pushFailureExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}
