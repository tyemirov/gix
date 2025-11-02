package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type dagTestOperation struct {
	name string
}

func (operation dagTestOperation) Name() string {
	return operation.name
}

func (operation dagTestOperation) Execute(_ context.Context, _ *Environment, _ *State) error {
	return nil
}

func buildDagNode(name string, dependencies ...string) *OperationNode {
	return &OperationNode{
		Name:         name,
		Operation:    dagTestOperation{name: name},
		Dependencies: dependencies,
	}
}

func stageOperationNames(stages []OperationStage) [][]string {
	names := make([][]string, len(stages))
	for stageIndex := range stages {
		stage := stages[stageIndex]
		layer := make([]string, 0, len(stage.Operations))
		for _, node := range stage.Operations {
			if node != nil {
				layer = append(layer, node.Name)
			}
		}
		names[stageIndex] = layer
	}
	return names
}

func TestPlanOperationStagesProducesTopologicalLayers(t *testing.T) {
	nodes := []*OperationNode{
		buildDagNode("prepare"),
		buildDagNode("convert-protocol", "prepare"),
		buildDagNode("update-remote", "prepare"),
		buildDagNode("rename-directories", "update-remote", "convert-protocol"),
	}

	stages, err := planOperationStages(nodes)
	require.NoError(t, err)

	names := stageOperationNames(stages)
	require.Len(t, names, 3)
	require.ElementsMatch(t, []string{"prepare"}, names[0])
	require.ElementsMatch(t, []string{"convert-protocol", "update-remote"}, names[1])
	require.ElementsMatch(t, []string{"rename-directories"}, names[2])
}

func TestPlanOperationStagesRejectsCycles(t *testing.T) {
	nodes := []*OperationNode{
		buildDagNode("alpha", "gamma"),
		buildDagNode("beta", "alpha"),
		buildDagNode("gamma", "beta"),
	}

	stages, err := planOperationStages(nodes)
	require.Error(t, err)
	require.Nil(t, stages)
}
