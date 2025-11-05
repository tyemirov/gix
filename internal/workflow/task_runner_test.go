package workflow

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
)

type noopGitExecutor struct{}

type emptyDiscoverer struct{}

func (noopGitExecutor) ExecuteGit(_ context.Context, _ execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func (noopGitExecutor) ExecuteGitHubCLI(_ context.Context, _ execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func (emptyDiscoverer) DiscoverRepositories(_ []string) ([]string, error) {
	return []string{}, nil
}

func TestTaskRunnerRunSkipsWithoutTasks(testInstance *testing.T) {
	runner := NewTaskRunner(Dependencies{})
	err := runner.Run(context.Background(), []string{"/tmp"}, nil, RuntimeOptions{})
	require.NoError(testInstance, err)
}

func TestTaskRunnerRunWithTasksNoRepositories(testInstance *testing.T) {
	executor := noopGitExecutor{}
	repositoryManager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(testInstance, managerError)

	gitHubClient, clientError := githubcli.NewClient(executor)
	require.NoError(testInstance, clientError)

	dependencies := Dependencies{
		RepositoryDiscoverer: emptyDiscoverer{},
		GitExecutor:          executor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         gitHubClient,
		Output:               io.Discard,
		Errors:               io.Discard,
	}

	runner := NewTaskRunner(dependencies)

	definitions := []TaskDefinition{{
		Name:    "Update Remote",
		Actions: []TaskActionDefinition{{Type: taskActionCanonicalRemote, Options: map[string]any{}}},
		Commit:  TaskCommitDefinition{},
	}}

	err := runner.Run(context.Background(), []string{"/tmp"}, definitions, RuntimeOptions{DryRun: true})
	require.NoError(testInstance, err)
}

func TestTaskRunnerRunSkipsGitHubMetadataRequirement(testInstance *testing.T) {
	executor := noopGitExecutor{}
	repositoryManager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(testInstance, managerError)

	dependencies := Dependencies{
		RepositoryDiscoverer: emptyDiscoverer{},
		GitExecutor:          executor,
		RepositoryManager:    repositoryManager,
		Output:               io.Discard,
		Errors:               io.Discard,
	}

	runner := NewTaskRunner(dependencies)

	definitions := []TaskDefinition{{
		Name:    "Update Remote",
		Actions: []TaskActionDefinition{{Type: taskActionCanonicalRemote, Options: map[string]any{}}},
		Commit:  TaskCommitDefinition{},
	}}

	err := runner.Run(
		context.Background(),
		[]string{"/tmp"},
		definitions,
		RuntimeOptions{DryRun: true, SkipRepositoryMetadata: true},
	)
	require.NoError(testInstance, err)
}
