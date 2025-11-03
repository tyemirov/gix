package workflow

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/gitrepo"
	repoerrors "github.com/temirov/gix/internal/repos/errors"
	"github.com/temirov/gix/internal/repos/shared"
)

func TestExecutorReturnsStructuredErrorMessage(testInstance *testing.T) {
	tempDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(tempDirectory, "sample")
	require.NoError(testInstance, os.Mkdir(repositoryPath, 0o755))

	gitExecutor := newStubWorkflowGitExecutor()
	repositoryManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(testInstance, managerError)

	githubClient, clientError := githubcli.NewClient(gitExecutor)
	require.NoError(testInstance, clientError)

	dependencies := Dependencies{
		RepositoryDiscoverer: executorStubRepositoryDiscoverer{repositories: []string{repositoryPath}},
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         githubClient,
		FileSystem:           nil,
		Prompter:             nil,
		Output:               &bytes.Buffer{},
		Errors:               &bytes.Buffer{},
	}

	operation := failingOperation{}
	executor := NewExecutor([]Operation{operation}, dependencies)

	executionError := executor.Execute(
		context.Background(),
		[]string{repositoryPath},
		RuntimeOptions{SkipRepositoryMetadata: true},
	)

	require.Error(testInstance, executionError)
	expectedMessage := fmt.Sprintf(
		"origin_owner_missing: canonical/example (%s) cannot resolve origin owner metadata",
		repositoryPath,
	)
	require.EqualError(testInstance, executionError, expectedMessage)
}

func TestExecutorDeduplicatesRelativeRoots(testInstance *testing.T) {
	tempDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(tempDirectory, "sample")
	require.NoError(testInstance, os.Mkdir(repositoryPath, 0o755))

	gitExecutor := newStubWorkflowGitExecutor()
	repositoryManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(testInstance, managerError)

	githubClient, clientError := githubcli.NewClient(gitExecutor)
	require.NoError(testInstance, clientError)

	outputBuffer := &bytes.Buffer{}
	errorBuffer := &bytes.Buffer{}

	dependencies := Dependencies{
		RepositoryDiscoverer: executorStubRepositoryDiscoverer{repositories: []string{repositoryPath}},
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         githubClient,
		Output:               outputBuffer,
		Errors:               errorBuffer,
	}

	executor := NewExecutor([]Operation{repoSwitchOperation{}}, dependencies)

	workingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	require.NoError(testInstance, os.Chdir(repositoryPath))
	testInstance.Cleanup(func() { _ = os.Chdir(workingDirectory) })

    executionError := executor.Execute(
        context.Background(),
        []string{"."},
        RuntimeOptions{SkipRepositoryMetadata: true},
    )
    require.NoError(testInstance, executionError)

    occurrences := strings.Count(outputBuffer.String(), "event="+shared.EventCodeRepoSwitched)
    if occurrences != 1 {
        testInstance.Logf("executor output:\n%s", outputBuffer.String())
    }
    require.Equal(testInstance, 1, occurrences)
}

type executorStubRepositoryDiscoverer struct {
	repositories []string
}

func (discoverer executorStubRepositoryDiscoverer) DiscoverRepositories(roots []string) ([]string, error) {
	return append([]string{}, discoverer.repositories...), nil
}

type stubWorkflowGitExecutor struct {
	responses map[string]execshell.ExecutionResult
}

func newStubWorkflowGitExecutor() *stubWorkflowGitExecutor {
	return &stubWorkflowGitExecutor{
		responses: map[string]execshell.ExecutionResult{
			"rev-parse --is-inside-work-tree":             {StandardOutput: "true\n"},
			"status --porcelain":                          {StandardOutput: ""},
			"remote get-url origin":                       {StandardOutput: "https://github.com/canonical/example.git\n"},
			"rev-parse --abbrev-ref HEAD":                 {StandardOutput: "main\n"},
			"ls-remote --symref origin HEAD":              {StandardOutput: "ref: refs/heads/main HEAD\n"},
			"remote update":                               {StandardOutput: ""},
			"for-each-ref --format=%(upstream:short)":     {StandardOutput: "origin/main\n"},
			"rev-parse HEAD":                              {StandardOutput: "deadbeef\n"},
			"rev-parse origin/main":                       {StandardOutput: "deadbeef\n"},
			"rev-parse --abbrev-ref @{-1}":                {StandardOutput: "main\n"},
			"symbolic-ref HEAD":                           {StandardOutput: "refs/heads/main\n"},
			"symbolic-ref refs/remotes/origin/HEAD":       {StandardOutput: "ref: refs/remotes/origin/main\n"},
			"rev-parse --verify refs/remotes/origin/main": {StandardOutput: "deadbeef\n"},
		},
	}
}

func (executor *stubWorkflowGitExecutor) ExecuteGit(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	key := strings.Join(details.Arguments, " ")
	if result, found := executor.responses[key]; found {
		return result, nil
	}
	return execshell.ExecutionResult{}, fmt.Errorf("unexpected git command: %s", key)
}

func (executor *stubWorkflowGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type failingOperation struct{}

func (operation failingOperation) Name() string {
	return "apply-tasks"
}

type repoSwitchOperation struct{}

func (operation repoSwitchOperation) Name() string {
	return "apply-tasks"
}

func (operation repoSwitchOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	if environment == nil || state == nil {
		return nil
	}
	for _, repository := range state.Repositories {
		if repository == nil {
			continue
		}
		environment.ReportRepositoryEvent(
			repository,
			shared.EventLevelInfo,
			shared.EventCodeRepoSwitched,
			"→ master",
			map[string]string{"branch": "master"},
		)
	}
	return nil
}

func (operation failingOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	if state == nil || len(state.Repositories) == 0 {
		return fmt.Errorf("missing repositories")
	}

	repositoryPath := state.Repositories[0].Path
	return repoerrors.WrapMessage(
		repoerrors.OperationCanonicalRemote,
		repositoryPath,
		repoerrors.ErrOriginOwnerMissing,
		"cannot resolve origin owner metadata",
	)
}
