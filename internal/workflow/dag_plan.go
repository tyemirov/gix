package workflow

import (
	"errors"
	"fmt"
	"strings"
)

var errOperationCycleDetected = errors.New("workflow operations contain cycle")

// OperationNode represents a workflow operation with dependency metadata.
type OperationNode struct {
	Name         string
	Operation    Operation
	Dependencies []string
}

// OperationStage groups operations that may execute in parallel.
type OperationStage struct {
	Operations []*OperationNode
}

func planOperationStages(nodes []*OperationNode) ([]OperationStage, error) {
	if len(nodes) == 0 {
		return nil, nil
	}

	nameToNode := make(map[string]*OperationNode, len(nodes))
	inDegree := make(map[string]int, len(nodes))
	adjacency := make(map[string][]string, len(nodes))

	for nodeIndex := range nodes {
		node := nodes[nodeIndex]
		if node == nil {
			return nil, errors.New("workflow operation node is nil")
		}
		if node.Operation == nil {
			return nil, fmt.Errorf("workflow step %q missing operation implementation", node.Name)
		}

		name := strings.TrimSpace(node.Name)
		if len(name) == 0 {
			name = node.Operation.Name()
		}
		if len(name) == 0 {
			return nil, errors.New("workflow operation missing name")
		}
		if _, exists := nameToNode[name]; exists {
			return nil, fmt.Errorf("workflow operation %q defined multiple times", name)
		}

		nameToNode[name] = node
		inDegree[name] = 0

		sanitizedDependencies := make([]string, 0, len(node.Dependencies))
		seenDependencies := make(map[string]struct{}, len(node.Dependencies))
		for dependencyIndex := range node.Dependencies {
			dependencyName := strings.TrimSpace(node.Dependencies[dependencyIndex])
			if len(dependencyName) == 0 {
				continue
			}
			if dependencyName == name {
				return nil, fmt.Errorf("workflow step %q cannot depend on itself", name)
			}
			if _, alreadyIncluded := seenDependencies[dependencyName]; alreadyIncluded {
				continue
			}
			seenDependencies[dependencyName] = struct{}{}
			sanitizedDependencies = append(sanitizedDependencies, dependencyName)
		}
		node.Dependencies = sanitizedDependencies
	}

	for nodeIndex := range nodes {
		node := nodes[nodeIndex]
		for _, dependencyName := range node.Dependencies {
			if _, exists := nameToNode[dependencyName]; !exists {
				return nil, fmt.Errorf("workflow step %q depends on unknown step %q", node.Name, dependencyName)
			}
			inDegree[node.Name]++
			adjacency[dependencyName] = append(adjacency[dependencyName], node.Name)
		}
	}

	ready := make([]string, 0)
	for _, node := range nodes {
		if inDegree[node.Name] == 0 {
			ready = append(ready, node.Name)
		}
	}

	stages := make([]OperationStage, 0)
	processed := 0
	processedSet := make(map[string]struct{}, len(nodes))

	for len(ready) > 0 {
		stageNames := ready
		ready = nil

		stage := OperationStage{
			Operations: make([]*OperationNode, 0, len(stageNames)),
		}

		for _, name := range stageNames {
			stage.Operations = append(stage.Operations, nameToNode[name])
			processed++
			processedSet[name] = struct{}{}
		}

		stages = append(stages, stage)

		nextReadySet := make(map[string]struct{})
		for _, name := range stageNames {
			for _, dependent := range adjacency[name] {
				if _, alreadyProcessed := processedSet[dependent]; alreadyProcessed {
					continue
				}
				inDegree[dependent]--
				if inDegree[dependent] == 0 {
					nextReadySet[dependent] = struct{}{}
				}
			}
		}

		for _, node := range nodes {
			if _, available := nextReadySet[node.Name]; available {
				ready = append(ready, node.Name)
			}
		}
	}

	if processed != len(nodes) {
		return nil, errOperationCycleDetected
	}

	return stages, nil
}
