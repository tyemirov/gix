package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/audit"
)

type stubRepositoryOperation struct {
	name        string
	executeFunc func(context.Context, *Environment, *RepositoryState) error
}

type stubGlobalOperation struct {
	name        string
	executeFunc func(context.Context, *Environment, *State) error
}

func (operation *stubGlobalOperation) Name() string {
	return operation.name
}

func (operation *stubGlobalOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	if operation.executeFunc == nil {
		return nil
	}
	return operation.executeFunc(ctx, environment, state)
}

func (operation *stubRepositoryOperation) Name() string {
	return operation.name
}

func (operation *stubRepositoryOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	if operation.executeFunc == nil {
		return nil
	}
	return operation.executeFunc(ctx, environment, nil)
}

func (operation *stubRepositoryOperation) ExecuteForRepository(
	ctx context.Context,
	environment *Environment,
	repository *RepositoryState,
) error {
	if operation.executeFunc == nil {
		return nil
	}
	return operation.executeFunc(ctx, environment, repository)
}

func (operation *stubRepositoryOperation) IsRepositoryScoped() bool {
	return true
}

func TestRunOperationStagesCancelsRepositoryAfterSafeguardFailure(t *testing.T) {
	t.Helper()

	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:           "/repositories/example",
		FinalOwnerRepo: "octocat/example",
	})
	environment := &Environment{}
	state := &State{Repositories: []*RepositoryState{repository}}

	safeguard := &stubRepositoryOperation{
		name: "clean-worktree-guard",
		executeFunc: func(context.Context, *Environment, *RepositoryState) error {
			return errRepositorySkipped
		},
	}

	followUpExecuted := 0
	followUp := &stubRepositoryOperation{
		name: "mutating-step",
		executeFunc: func(context.Context, *Environment, *RepositoryState) error {
			followUpExecuted++
			return nil
		},
	}

	stages := []OperationStage{{
		Operations: []*OperationNode{
			{Name: "clean-worktree-guard", Operation: safeguard},
			{Name: "mutating-step", Operation: followUp},
		},
	}}

	result := runOperationStages(context.Background(), stages, environment, state, nil, 1)

	require.Equal(t, 0, followUpExecuted, "follow-up operation should not run after safeguard failure")
	require.Len(t, result.stageOutcomes, 1)
	require.Equal(t, []string{"octocat/example:clean-worktree-guard"}, result.stageOutcomes[0].Operations)
	require.Contains(t, result.operationOutcomes, "octocat/example:clean-worktree-guard")
	require.NotContains(t, result.operationOutcomes, "octocat/example:mutating-step")
}

func TestRunOperationStagesSkipsLaterStagesAfterSafeguardFailure(t *testing.T) {
	t.Helper()

	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:           "/repositories/example",
		FinalOwnerRepo: "octocat/example",
	})
	environment := &Environment{}
	state := &State{Repositories: []*RepositoryState{repository}}

	safeguard := &stubRepositoryOperation{
		name: "clean-worktree-guard",
		executeFunc: func(context.Context, *Environment, *RepositoryState) error {
			return errRepositorySkipped
		},
	}

	stageOneFollowExecuted := 0
	stageOneFollow := &stubRepositoryOperation{
		name: "mutating-step",
		executeFunc: func(context.Context, *Environment, *RepositoryState) error {
			stageOneFollowExecuted++
			return nil
		},
	}

	stageTwoExecuted := 0
	stageTwoOperation := &stubRepositoryOperation{
		name: "post-guard",
		executeFunc: func(context.Context, *Environment, *RepositoryState) error {
			stageTwoExecuted++
			return nil
		},
	}

	stages := []OperationStage{
		{
			Operations: []*OperationNode{
				{Name: "clean-worktree-guard", Operation: safeguard},
				{Name: "mutating-step", Operation: stageOneFollow},
			},
		},
		{
			Operations: []*OperationNode{
				{Name: "post-guard", Operation: stageTwoOperation},
			},
		},
	}

	result := runOperationStages(context.Background(), stages, environment, state, nil, 1)

	require.Equal(t, 0, stageOneFollowExecuted, "stage one follow-up should not run")
	require.Equal(t, 0, stageTwoExecuted, "stage two should not run after safeguard failure")
	require.Len(t, result.stageOutcomes, 1)
	require.Equal(t, []string{"octocat/example:clean-worktree-guard"}, result.stageOutcomes[0].Operations)
	require.Contains(t, result.operationOutcomes, "octocat/example:clean-worktree-guard")
	require.NotContains(t, result.operationOutcomes, "octocat/example:mutating-step")
	require.NotContains(t, result.operationOutcomes, "octocat/example:post-guard")
}

func TestRunOperationStagesSupportsParallelRepositories(t *testing.T) {
	t.Helper()

	repositoryOne := NewRepositoryState(audit.RepositoryInspection{
		Path:           "/repositories/one",
		FinalOwnerRepo: "octocat/one",
	})
	repositoryTwo := NewRepositoryState(audit.RepositoryInspection{
		Path:           "/repositories/two",
		FinalOwnerRepo: "octocat/two",
	})
	environment := &Environment{}
	state := &State{Repositories: []*RepositoryState{repositoryOne, repositoryTwo}}

	var recorded []string
	var recordMutex sync.Mutex
	operation := &stubRepositoryOperation{
		name: "parallel-step",
		executeFunc: func(_ context.Context, _ *Environment, repository *RepositoryState) error {
			recordMutex.Lock()
			recorded = append(recorded, repository.Path)
			recordMutex.Unlock()
			return nil
		},
	}

	stages := []OperationStage{{
		Operations: []*OperationNode{{Name: "parallel-step", Operation: operation}},
	}}

	result := runOperationStages(context.Background(), stages, environment, state, nil, 2)

	require.ElementsMatch(t, []string{repositoryOne.Path, repositoryTwo.Path}, recorded)
	require.Len(t, result.stageOutcomes, 2)
	require.Equal(t,
		[]string{fmt.Sprintf("%s:%s", repositoryLabel(repositoryOne), "parallel-step")},
		result.stageOutcomes[0].Operations,
	)
	require.Equal(t,
		[]string{fmt.Sprintf("%s:%s", repositoryLabel(repositoryTwo), "parallel-step")},
		result.stageOutcomes[1].Operations,
	)
}

func TestRunOperationStagesStopsRepositoryDispatchAfterCancellation(t *testing.T) {
	repositories := []*RepositoryState{
		NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/one", FinalOwnerRepo: "octocat/one"}),
		NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/two", FinalOwnerRepo: "octocat/two"}),
		NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/three", FinalOwnerRepo: "octocat/three"}),
	}

	executionContext, cancelExecution := context.WithCancel(context.Background())
	errorOutput := &bytes.Buffer{}
	environment := &Environment{Errors: errorOutput}
	state := &State{Repositories: repositories}

	executionCount := 0
	operation := &stubRepositoryOperation{
		name: "cancel-step",
		executeFunc: func(context.Context, *Environment, *RepositoryState) error {
			executionCount++
			cancelExecution()
			return context.Canceled
		},
	}

	stages := []OperationStage{{
		Operations: []*OperationNode{{Name: "cancel-step", Operation: operation}},
	}}

	result := runOperationStages(executionContext, stages, environment, state, nil, 1)

	require.Equal(t, 1, executionCount)
	require.Empty(t, result.failures)
	require.Empty(t, errorOutput.String())
}

func TestRunOperationStagesPreservesFailureBeforeCancellation(t *testing.T) {
	repositories := []*RepositoryState{
		NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/one", FinalOwnerRepo: "octocat/one"}),
		NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/two", FinalOwnerRepo: "octocat/two"}),
		NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/three", FinalOwnerRepo: "octocat/three"}),
	}

	executionContext, cancelExecution := context.WithCancel(context.Background())
	errorOutput := &bytes.Buffer{}
	environment := &Environment{Errors: errorOutput}
	state := &State{Repositories: repositories}

	executionCount := 0
	operation := &stubRepositoryOperation{
		name: "cleanup-step",
		executeFunc: func(context.Context, *Environment, *RepositoryState) error {
			executionCount++
			if executionCount == 1 {
				return errors.New("remote unavailable")
			}
			cancelExecution()
			return context.Canceled
		},
	}

	stages := []OperationStage{{
		Operations: []*OperationNode{{Name: "cleanup-step", Operation: operation}},
	}}

	result := runOperationStages(executionContext, stages, environment, state, nil, 1)

	require.Equal(t, 2, executionCount)
	require.Len(t, result.failures, 1)
	require.Equal(t, "cleanup-step: remote unavailable\n", errorOutput.String())
}

func TestRunOperationStagesPreservesParallelFailureDuringCancellation(t *testing.T) {
	repositories := []*RepositoryState{
		NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/one", FinalOwnerRepo: "octocat/one"}),
		NewRepositoryState(audit.RepositoryInspection{Path: "/repositories/two", FinalOwnerRepo: "octocat/two"}),
	}

	executionContext, cancelExecution := context.WithCancel(context.Background())
	errorOutput := &bytes.Buffer{}
	environment := &Environment{Errors: errorOutput}
	state := &State{Repositories: repositories}
	var operationBarrier sync.WaitGroup
	operationBarrier.Add(len(repositories))

	operation := &stubRepositoryOperation{
		name: "cleanup-step",
		executeFunc: func(_ context.Context, _ *Environment, repository *RepositoryState) error {
			operationBarrier.Done()
			operationBarrier.Wait()
			if repository.Path == repositories[0].Path {
				cancelExecution()
				return context.Canceled
			}
			<-executionContext.Done()
			return errors.New("remote unavailable")
		},
	}

	stages := []OperationStage{{
		Operations: []*OperationNode{{Name: "cleanup-step", Operation: operation}},
	}}

	result := runOperationStages(executionContext, stages, environment, state, nil, 2)

	require.Len(t, result.failures, 1)
	require.EqualError(t, result.failures[0].err, "remote unavailable")
	require.Equal(t, "cleanup-step: remote unavailable\n", errorOutput.String())
	require.Len(t, result.stageOutcomes, 1)
	require.Equal(t, []string{"octocat/two:cleanup-step"}, result.stageOutcomes[0].Operations)
	require.Contains(t, result.operationOutcomes, "octocat/two:cleanup-step")
	require.NotContains(t, result.operationOutcomes, "octocat/one:cleanup-step")
}

func TestRunOperationStagesPreservesGlobalFailureDuringCancellation(t *testing.T) {
	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:           "/repositories/example",
		FinalOwnerRepo: "octocat/example",
	})
	executionContext, cancelExecution := context.WithCancel(context.Background())
	errorOutput := &bytes.Buffer{}
	environment := &Environment{Errors: errorOutput}
	state := &State{Repositories: []*RepositoryState{repository}}

	operation := &stubGlobalOperation{
		name: "global-step",
		executeFunc: func(context.Context, *Environment, *State) error {
			cancelExecution()
			return errors.New("API unavailable")
		},
	}
	stages := []OperationStage{{
		Operations: []*OperationNode{{Name: "global-step", Operation: operation}},
	}}

	result := runOperationStages(executionContext, stages, environment, state, nil, 1)

	require.Len(t, result.failures, 1)
	require.EqualError(t, result.failures[0].err, "API unavailable")
	require.Equal(t, "global-step: API unavailable\n", errorOutput.String())
	require.Len(t, result.stageOutcomes, 1)
	require.Equal(t, []string{"global-step"}, result.stageOutcomes[0].Operations)
	require.Contains(t, result.operationOutcomes, "global-step")
}

func TestRunOperationStagesPreservesPartialStageOutcomeAfterCancellation(t *testing.T) {
	repository := NewRepositoryState(audit.RepositoryInspection{
		Path:           "/repositories/example",
		FinalOwnerRepo: "octocat/example",
	})
	executionContext, cancelExecution := context.WithCancel(context.Background())
	environment := &Environment{}
	state := &State{Repositories: []*RepositoryState{repository}}

	completedOperation := &stubRepositoryOperation{name: "completed-step"}
	canceledOperation := &stubRepositoryOperation{
		name: "canceled-step",
		executeFunc: func(context.Context, *Environment, *RepositoryState) error {
			cancelExecution()
			return context.Canceled
		},
	}
	stages := []OperationStage{{
		Operations: []*OperationNode{
			{Name: "completed-step", Operation: completedOperation},
			{Name: "canceled-step", Operation: canceledOperation},
		},
	}}

	result := runOperationStages(executionContext, stages, environment, state, nil, 1)

	require.Empty(t, result.failures)
	require.Len(t, result.stageOutcomes, 1)
	require.Equal(t, []string{"octocat/example:completed-step"}, result.stageOutcomes[0].Operations)
	require.Contains(t, result.operationOutcomes, "octocat/example:completed-step")
	require.NotContains(t, result.operationOutcomes, "octocat/example:canceled-step")
}

func TestRunOperationStagesExecutesFullPipelinePerRepository(t *testing.T) {
	t.Helper()

	repositoryOne := NewRepositoryState(audit.RepositoryInspection{
		Path:           "/repositories/one",
		FinalOwnerRepo: "octocat/one",
	})
	repositoryTwo := NewRepositoryState(audit.RepositoryInspection{
		Path:           "/repositories/two",
		FinalOwnerRepo: "octocat/two",
	})

	environment := &Environment{}
	state := &State{Repositories: []*RepositoryState{repositoryOne, repositoryTwo}}

	executionHistory := make(map[string][]string)
	var historyMutex sync.Mutex

	recordOperation := func(repoPath string, step string) {
		historyMutex.Lock()
		defer historyMutex.Unlock()
		executionHistory[repoPath] = append(executionHistory[repoPath], step)
	}

	stageOne := &stubRepositoryOperation{
		name: "stage-one",
		executeFunc: func(_ context.Context, _ *Environment, repository *RepositoryState) error {
			recordOperation(repository.Path, "stage-one")
			return nil
		},
	}
	stageTwo := &stubRepositoryOperation{
		name: "stage-two",
		executeFunc: func(_ context.Context, _ *Environment, repository *RepositoryState) error {
			recordOperation(repository.Path, "stage-two")
			return nil
		},
	}

	stages := []OperationStage{
		{Operations: []*OperationNode{{Name: "stage-one", Operation: stageOne}}},
		{Operations: []*OperationNode{{Name: "stage-two", Operation: stageTwo}}},
	}

	runOperationStages(context.Background(), stages, environment, state, nil, 2)

	require.Len(t, executionHistory[repositoryOne.Path], 2)
	require.Equal(t, []string{"stage-one", "stage-two"}, executionHistory[repositoryOne.Path])
	require.Len(t, executionHistory[repositoryTwo.Path], 2)
	require.Equal(t, []string{"stage-one", "stage-two"}, executionHistory[repositoryTwo.Path])
}

func TestStageIsRepositoryScopedForBuiltinOperations(t *testing.T) {
	t.Helper()

	renameStage := OperationStage{
		Operations: []*OperationNode{
			{Name: "rename", Operation: &RenameOperation{}},
		},
	}
	require.True(t, stageIsRepositoryScoped(renameStage))

	canonicalRemoteStage := OperationStage{
		Operations: []*OperationNode{
			{Name: "canonical-remote", Operation: &CanonicalRemoteOperation{}},
		},
	}
	require.True(t, stageIsRepositoryScoped(canonicalRemoteStage))

	protocolStage := OperationStage{
		Operations: []*OperationNode{
			{Name: "protocol-convert", Operation: &ProtocolConversionOperation{}},
		},
	}
	require.True(t, stageIsRepositoryScoped(protocolStage))
}
