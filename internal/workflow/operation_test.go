package workflow

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCloneEnvironmentForRepositoryCopiesVariableStore(testInstance *testing.T) {
	base := &Environment{Variables: NewVariableStore(), sharedState: &environmentSharedState{}}
	base.Variables.Set(VariableName("shared"), "original")

	repoState := &State{Repositories: []*RepositoryState{{Path: "/tmp/repo1"}}}
	clone := cloneEnvironmentForRepository(base, repoState)

	require.NotNil(testInstance, clone.Variables)
	require.NotSame(testInstance, base.Variables, clone.Variables)

	clone.Variables.Set(VariableName("shared"), "updated")

	baseValue, _ := base.Variables.Get(VariableName("shared"))
	cloneValue, _ := clone.Variables.Get(VariableName("shared"))

	require.Equal(testInstance, "original", baseValue)
	require.Equal(testInstance, "updated", cloneValue)
}

func TestCaptureStateIsIsolatedPerRepository(testInstance *testing.T) {
	base := &Environment{Variables: NewVariableStore(), sharedState: &environmentSharedState{}}

	repositoryOne := &RepositoryState{Path: "/tmp/repo-one"}
	repositoryTwo := &RepositoryState{Path: "/tmp/repo-two"}

	environmentOne := cloneEnvironmentForRepository(base, &State{Repositories: []*RepositoryState{repositoryOne}})
	environmentTwo := cloneEnvironmentForRepository(base, &State{Repositories: []*RepositoryState{repositoryTwo}})

	environmentOne.StoreCaptureValue(VariableName("initial_branch"), "branch-one", false)
	environmentOne.RecordCaptureKind(VariableName("initial_branch"), CaptureKindBranch)

	environmentTwo.StoreCaptureValue(VariableName("initial_branch"), "branch-two", false)
	environmentTwo.RecordCaptureKind(VariableName("initial_branch"), CaptureKindCommit)

	valueOne, existsOne := environmentOne.CapturedValue(VariableName("initial_branch"))
	valueTwo, existsTwo := environmentTwo.CapturedValue(VariableName("initial_branch"))
	require.True(testInstance, existsOne)
	require.True(testInstance, existsTwo)

	require.Equal(testInstance, "branch-one", valueOne)
	require.Equal(testInstance, "branch-two", valueTwo)

	kindOne, kindExistsOne := environmentOne.CaptureKindForVariable(VariableName("initial_branch"))
	kindTwo, kindExistsTwo := environmentTwo.CaptureKindForVariable(VariableName("initial_branch"))

	require.True(testInstance, kindExistsOne)
	require.True(testInstance, kindExistsTwo)

	require.Equal(testInstance, CaptureKindBranch, kindOne)
	require.Equal(testInstance, CaptureKindCommit, kindTwo)

	variableValueOne, variableExistsOne := environmentOne.Variables.Get(VariableName("initial_branch"))
	variableValueTwo, variableExistsTwo := environmentTwo.Variables.Get(VariableName("initial_branch"))

	require.True(testInstance, variableExistsOne)
	require.True(testInstance, variableExistsTwo)
	require.Equal(testInstance, "branch-one", variableValueOne)
	require.Equal(testInstance, "branch-two", variableValueTwo)
}
