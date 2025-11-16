package workflow

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/audit"
)

type stubRepositoryOperation struct {
	name        string
	executeFunc func(context.Context, *Environment, *RepositoryState) error
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
	require.Contains(t, result.operationOutcomes, "clean-worktree-guard@octocat/example")
	require.NotContains(t, result.operationOutcomes, "mutating-step@octocat/example")
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
	require.Contains(t, result.operationOutcomes, "clean-worktree-guard@octocat/example")
	require.NotContains(t, result.operationOutcomes, "mutating-step@octocat/example")
	require.NotContains(t, result.operationOutcomes, "post-guard@octocat/example")
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
