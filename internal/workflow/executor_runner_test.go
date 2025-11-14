package workflow

import (
	"context"
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

	result := runOperationStages(context.Background(), stages, environment, state, nil)

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

	result := runOperationStages(context.Background(), stages, environment, state, nil)

	require.Equal(t, 0, stageOneFollowExecuted, "stage one follow-up should not run")
	require.Equal(t, 0, stageTwoExecuted, "stage two should not run after safeguard failure")
	require.Len(t, result.stageOutcomes, 1)
	require.Equal(t, []string{"octocat/example:clean-worktree-guard"}, result.stageOutcomes[0].Operations)
	require.Contains(t, result.operationOutcomes, "clean-worktree-guard@octocat/example")
	require.NotContains(t, result.operationOutcomes, "mutating-step@octocat/example")
	require.NotContains(t, result.operationOutcomes, "post-guard@octocat/example")
}
