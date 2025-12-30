package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
	repoerrors "github.com/tyemirov/gix/internal/repos/errors"
	"github.com/tyemirov/gix/internal/repos/shared"
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

	dependencies := humanReadableDependencies(Dependencies{
		RepositoryDiscoverer: executorStubRepositoryDiscoverer{repositories: []string{repositoryPath}},
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         githubClient,
		FileSystem:           nil,
		Prompter:             nil,
		Output:               &bytes.Buffer{},
		Errors:               &bytes.Buffer{},
	})

	operation := failingOperation{}
	executor := NewExecutor([]Operation{operation}, dependencies)

	outcome, executionError := executor.Execute(
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
	require.Equal(testInstance, 1, outcome.RepositoryCount)
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

	dependencies := humanReadableDependencies(Dependencies{
		RepositoryDiscoverer: executorStubRepositoryDiscoverer{repositories: []string{repositoryPath}},
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         githubClient,
		Output:               outputBuffer,
		Errors:               outputBuffer,
	})

	executor := NewExecutor([]Operation{repoSwitchOperation{}}, dependencies)

	workingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	require.NoError(testInstance, os.Chdir(repositoryPath))
	testInstance.Cleanup(func() { _ = os.Chdir(workingDirectory) })

	outcome, executionError := executor.Execute(
		context.Background(),
		[]string{"."},
		RuntimeOptions{SkipRepositoryMetadata: true},
	)
	require.NoError(testInstance, executionError)

	require.Equal(testInstance, 1, outcome.ReporterSummaryData.EventCounts[shared.EventCodeRepoSwitched])
	require.Equal(testInstance, 1, outcome.RepositoryCount)
}

func TestExecutorSkipsMetadataWhenGitHubClientMissing(testInstance *testing.T) {
	tempDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(tempDirectory, "sample")
	require.NoError(testInstance, os.Mkdir(repositoryPath, 0o755))

	gitExecutor := newStubWorkflowGitExecutor()
	repositoryManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(testInstance, managerError)

	dependencies := humanReadableDependencies(Dependencies{
		RepositoryDiscoverer: executorStubRepositoryDiscoverer{repositories: []string{repositoryPath}},
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		Output:               &bytes.Buffer{},
		Errors:               &bytes.Buffer{},
	})

	executor := NewExecutor([]Operation{repoSwitchOperation{}}, dependencies)

	outcome, executionError := executor.Execute(
		context.Background(),
		[]string{repositoryPath},
		RuntimeOptions{SkipRepositoryMetadata: true},
	)
	require.NoError(testInstance, executionError)
	require.Equal(testInstance, 1, outcome.RepositoryCount)
}

func TestExecutorEmitsDiscoveryStepSummary(testInstance *testing.T) {
	tempDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(tempDirectory, "sample")
	require.NoError(testInstance, os.Mkdir(repositoryPath, 0o755))

	gitExecutor := newStubWorkflowGitExecutor()
	repositoryManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(testInstance, managerError)

	outputBuffer := &bytes.Buffer{}

	dependencies := humanReadableDependencies(Dependencies{
		RepositoryDiscoverer: executorStubRepositoryDiscoverer{repositories: []string{repositoryPath}},
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		Output:               outputBuffer,
		Errors:               outputBuffer,
	})

	executor := NewExecutor([]Operation{noopOperation{}}, dependencies)

	outcome, executionError := executor.Execute(
		context.Background(),
		[]string{repositoryPath},
		RuntimeOptions{SkipRepositoryMetadata: true},
	)
	require.NoError(testInstance, executionError)

	require.Equal(testInstance, 1, outcome.ReporterSummaryData.TotalRepositories)
	require.Equal(testInstance, 1, outcome.RepositoryCount)
	outputText := strings.TrimSpace(outputBuffer.String())
	require.NotEmpty(testInstance, outputText)
	require.Contains(testInstance, outputText, "step name: discovery")
	require.Contains(testInstance, outputText, "reason: discovered")
}

func TestExecutorSeedsVariablesFromRuntimeOptions(testInstance *testing.T) {
	tempDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(tempDirectory, "sample")
	require.NoError(testInstance, os.Mkdir(repositoryPath, 0o755))

	gitExecutor := newStubWorkflowGitExecutor()
	repositoryManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(testInstance, managerError)

	githubClient, clientError := githubcli.NewClient(gitExecutor)
	require.NoError(testInstance, clientError)

	recording := &variableRecordingOperation{}
	dependencies := humanReadableDependencies(Dependencies{
		RepositoryDiscoverer: executorStubRepositoryDiscoverer{repositories: []string{repositoryPath}},
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         githubClient,
		Output:               &bytes.Buffer{},
		Errors:               &bytes.Buffer{},
	})

	executor := NewExecutor([]Operation{recording}, dependencies)
	outcome, executionError := executor.Execute(
		context.Background(),
		[]string{repositoryPath},
		RuntimeOptions{
			SkipRepositoryMetadata: true,
			Variables: map[string]string{
				"license_template": "apache",
				"scope":            "demo",
			},
		},
	)
	require.NoError(testInstance, executionError)
	require.Equal(testInstance, 1, outcome.RepositoryCount)
	require.Equal(testInstance, "apache", recording.variables["license_template"])
	require.Equal(testInstance, "demo", recording.variables["scope"])
}

func TestExecutorRecordsStageAndOperationOutcomes(testInstance *testing.T) {
	tempDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(tempDirectory, "sample")
	require.NoError(testInstance, os.Mkdir(repositoryPath, 0o755))

	gitExecutor := newStubWorkflowGitExecutor()
	repositoryManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(testInstance, managerError)

	githubClient, clientError := githubcli.NewClient(gitExecutor)
	require.NoError(testInstance, clientError)

	dependencies := humanReadableDependencies(Dependencies{
		RepositoryDiscoverer: executorStubRepositoryDiscoverer{repositories: []string{repositoryPath}},
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         githubClient,
		Output:               &bytes.Buffer{},
		Errors:               &bytes.Buffer{},
	})

	operations := []Operation{
		namedOperation{operationName: "alpha"},
		namedOperation{operationName: "beta"},
	}

	executor := NewExecutor(operations, dependencies)
	outcome, executionError := executor.Execute(
		context.Background(),
		[]string{repositoryPath},
		RuntimeOptions{SkipRepositoryMetadata: true},
	)
	require.NoError(testInstance, executionError)
	require.Equal(testInstance, 1, outcome.RepositoryCount)

	require.Len(testInstance, outcome.StageOutcomes, 2)
	require.Equal(
		testInstance,
		[]string{"alpha-1"},
		outcome.StageOutcomes[0].Operations,
	)
	require.Equal(
		testInstance,
		[]string{"beta-2"},
		outcome.StageOutcomes[1].Operations,
	)

	require.Len(testInstance, outcome.OperationOutcomes, 2)
	require.Equal(testInstance, "alpha-1", outcome.OperationOutcomes[0].Name)
	require.False(testInstance, outcome.OperationOutcomes[0].Failed)
	require.Equal(testInstance, "beta-2", outcome.OperationOutcomes[1].Name)
	require.False(testInstance, outcome.OperationOutcomes[1].Failed)

	require.Equal(testInstance, 1, outcome.ReporterSummaryData.TotalRepositories)
}

func TestRepositorySkipPreventsSubsequentOperations(testInstance *testing.T) {
	tempDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(tempDirectory, "sample")
	require.NoError(testInstance, os.Mkdir(repositoryPath, 0o755))

	gitExecutor := newStubWorkflowGitExecutor()
	repositoryManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(testInstance, managerError)

	githubClient, clientError := githubcli.NewClient(gitExecutor)
	require.NoError(testInstance, clientError)

	tracker := &trackingRepositoryOperation{}
	dependencies := humanReadableDependencies(Dependencies{
		RepositoryDiscoverer: executorStubRepositoryDiscoverer{repositories: []string{repositoryPath}},
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         githubClient,
		Output:               &bytes.Buffer{},
		Errors:               &bytes.Buffer{},
	})

	executor := NewExecutor([]Operation{repositorySkipOperation{}, tracker}, dependencies)
	_, executionError := executor.Execute(
		context.Background(),
		[]string{repositoryPath},
		RuntimeOptions{SkipRepositoryMetadata: true},
	)
	require.NoError(testInstance, executionError)
	require.Equal(testInstance, 0, tracker.executions)
}

func TestExecutorContinuesExecutingOperationsAfterFailure(testInstance *testing.T) {
	tempDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(tempDirectory, "sample")
	require.NoError(testInstance, os.Mkdir(repositoryPath, 0o755))

	gitExecutor := newStubWorkflowGitExecutor()
	repositoryManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(testInstance, managerError)

	buffer := &bytes.Buffer{}
	recorder := &recordingOperation{}

	dependencies := humanReadableDependencies(Dependencies{
		RepositoryDiscoverer: executorStubRepositoryDiscoverer{repositories: []string{repositoryPath}},
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		Output:               buffer,
		Errors:               buffer,
	})

	executor := NewExecutor([]Operation{failingOperation{}, recorder}, dependencies)

	outcome, executionError := executor.Execute(
		context.Background(),
		[]string{repositoryPath},
		RuntimeOptions{SkipRepositoryMetadata: true},
	)
	require.Error(testInstance, executionError)
	require.True(testInstance, recorder.executed)
	require.Equal(testInstance, 1, outcome.ReporterSummaryData.TotalRepositories)
}

func TestExecutorLogsAllErrorsFromJoinedOperationFailures(testInstance *testing.T) {
	tempDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(tempDirectory, "sample")
	require.NoError(testInstance, os.Mkdir(repositoryPath, 0o755))

	gitExecutor := newStubWorkflowGitExecutor()
	repositoryManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(testInstance, managerError)

	outputBuffer := &bytes.Buffer{}
	dependencies := humanReadableDependencies(Dependencies{
		RepositoryDiscoverer: executorStubRepositoryDiscoverer{repositories: []string{repositoryPath}},
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		Output:               outputBuffer,
		Errors:               outputBuffer,
		Logger:               nil,
	})

	executor := NewExecutor([]Operation{joinFailOperation{}}, dependencies)

	outcome, executionError := executor.Execute(
		context.Background(),
		[]string{repositoryPath},
		RuntimeOptions{SkipRepositoryMetadata: true},
	)
	require.Error(testInstance, executionError)
	eventCounts := outcome.ReporterSummaryData.EventCounts
	require.Equal(
		testInstance,
		1,
		eventCounts[strings.ToUpper(string(repoerrors.ErrNamespaceRewriteFailed))],
	)
	require.Equal(
		testInstance,
		1,
		eventCounts[strings.ToUpper(string(repoerrors.ErrOriginOwnerMissing))],
	)
}

func TestExecutorSuppressesWorkflowLogsWhenDisabled(t *testing.T) {
	tempDirectory := t.TempDir()
	repositoryPath := filepath.Join(tempDirectory, "sample")
	require.NoError(t, os.Mkdir(repositoryPath, 0o755))

	gitExecutor := newStubWorkflowGitExecutor()
	repositoryManager, managerErr := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerErr)

	output := &bytes.Buffer{}
	dependencies := humanReadableDependencies(Dependencies{
		RepositoryDiscoverer:   executorStubRepositoryDiscoverer{repositories: []string{repositoryPath}},
		GitExecutor:            gitExecutor,
		RepositoryManager:      repositoryManager,
		Output:                 output,
		Errors:                 output,
		DisableWorkflowLogging: true,
	})

	executor := NewExecutor([]Operation{noopOperation{}}, dependencies)
	_, err := executor.Execute(
		context.Background(),
		[]string{repositoryPath},
		RuntimeOptions{SkipRepositoryMetadata: true},
	)
	require.NoError(t, err)
	require.Empty(t, output.String())
}

func TestFormatOperationFailureSkipsTasksApplyPrefix(t *testing.T) {
	t.Helper()
	err := errors.New("git command exited with code 1 (switch master)")
	message := formatOperationFailure(nil, err, commandTasksApplyKey)
	require.Equal(t, "git command exited with code 1 (switch master)", message)
}

func TestFormatOperationFailureIncludesOperationName(t *testing.T) {
	t.Helper()
	err := errors.New("unable to rename folder")
	message := formatOperationFailure(nil, err, "repo.folder.rename")
	require.Equal(t, "repo.folder.rename: unable to rename folder", message)
}

type executorStubRepositoryDiscoverer struct {
	repositories []string
}

func (discoverer executorStubRepositoryDiscoverer) DiscoverRepositories(roots []string) ([]string, error) {
	return append([]string{}, discoverer.repositories...), nil
}

type noopOperation struct{}

func (noopOperation) Name() string {
	return "noop"
}

func (noopOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	return nil
}

type recordingOperation struct {
	executed bool
}

func (operation *recordingOperation) Name() string {
	return "recording"
}

func (operation *recordingOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	operation.executed = true
	return nil
}

type variableRecordingOperation struct {
	variables map[string]string
}

func (operation *variableRecordingOperation) Name() string {
	return "capture-variables"
}

func (operation *variableRecordingOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	if operation.variables == nil {
		operation.variables = make(map[string]string)
	}
	if environment != nil && environment.Variables != nil {
		for key, value := range environment.Variables.Snapshot() {
			operation.variables[key] = value
		}
	}
	return nil
}

type namedOperation struct {
	operationName string
}

func (operation namedOperation) Name() string {
	return operation.operationName
}

func (operation namedOperation) Execute(context.Context, *Environment, *State) error {
	return nil
}

type joinFailOperation struct{}

func (operation joinFailOperation) Name() string {
	return "join-failure"
}

func (operation joinFailOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	if state == nil || len(state.Repositories) == 0 {
		return fmt.Errorf("missing repositories")
	}

	repositoryPath := state.Repositories[0].Path
	errOne := repoerrors.WrapMessage(
		repoerrors.OperationCanonicalRemote,
		repositoryPath,
		repoerrors.ErrOriginOwnerMissing,
		"cannot resolve origin owner metadata",
	)
	errTwo := repoerrors.WrapMessage(
		repoerrors.OperationNamespaceRewrite,
		repositoryPath,
		repoerrors.ErrNamespaceRewriteFailed,
		"rewrite skipped",
	)
	return errors.Join(errOne, errTwo)
}

type repositorySkipOperation struct{}

func (repositorySkipOperation) Name() string {
	return "repository-skip"
}

func (repositorySkipOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	return nil
}

func (repositorySkipOperation) ExecuteForRepository(ctx context.Context, environment *Environment, repository *RepositoryState) error {
	return repositorySkipError{reason: "repository dirty"}
}

func (repositorySkipOperation) IsRepositoryScoped() bool {
	return true
}

type trackingRepositoryOperation struct {
	executions int
}

func (operation *trackingRepositoryOperation) Name() string {
	return "tracking"
}

func (operation *trackingRepositoryOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	return nil
}

func (operation *trackingRepositoryOperation) ExecuteForRepository(ctx context.Context, environment *Environment, repository *RepositoryState) error {
	operation.executions++
	return nil
}

func (operation *trackingRepositoryOperation) IsRepositoryScoped() bool {
	return true
}

func humanReadableDependencies(dependencies Dependencies) Dependencies {
	dependencies.HumanReadableLogging = true
	dependencies.ReporterOptions = append(
		dependencies.ReporterOptions,
		shared.WithEventFormatter(NewHumanEventFormatter()),
	)
	dependencies.DisableHeaderDecoration = true
	return dependencies
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
