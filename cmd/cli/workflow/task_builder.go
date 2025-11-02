package workflow

import (
	"errors"
	"fmt"
	"strings"

	workflowpkg "github.com/temirov/gix/internal/workflow"
)

const (
	taskNameConvertProtocol        = "Convert remote protocol"
	taskNameUpdateCanonicalRemote  = "Update canonical remote"
	taskNameRenameDirectories      = "Rename repository directories"
	taskNamePromoteDefaultBranch   = "Promote default branch to %s"
	taskNameGenerateAuditReport    = "Generate audit report"
	defaultMigrationRemoteFallback = "origin"
	defaultMigrationTargetFallback = "master"
)

func buildWorkflowTasks(nodes []*workflowpkg.OperationNode) ([]workflowpkg.TaskDefinition, workflowpkg.RuntimeOptions, error) {
	orderedNodes, orderError := orderOperationNodes(nodes)
	if orderError != nil {
		return nil, workflowpkg.RuntimeOptions{}, orderError
	}

	taskDefinitions := make([]workflowpkg.TaskDefinition, 0)
	accumulatedRuntime := workflowpkg.RuntimeOptions{}

	for nodeIndex := range orderedNodes {
		node := orderedNodes[nodeIndex]
		if node == nil {
			continue
		}

		operation := node.Operation
		if operation == nil {
			continue
		}

		switch typedOperation := operation.(type) {
		case *workflowpkg.TaskOperation:
			taskDefinitions = append(taskDefinitions, typedOperation.Definitions()...)

		case *workflowpkg.CanonicalRemoteOperation:
			options := map[string]any{}
			ownerConstraint := strings.TrimSpace(typedOperation.OwnerConstraint)
			if len(ownerConstraint) > 0 {
				options["owner"] = ownerConstraint
			}
			taskDefinitions = append(taskDefinitions, workflowpkg.TaskDefinition{
				Name:        taskNameUpdateCanonicalRemote,
				EnsureClean: false,
				Actions: []workflowpkg.TaskActionDefinition{
					{Type: "repo.remote.update", Options: options},
				},
			})

		case *workflowpkg.ProtocolConversionOperation:
			options := map[string]any{
				"from": string(typedOperation.FromProtocol),
				"to":   string(typedOperation.ToProtocol),
			}
			taskDefinitions = append(taskDefinitions, workflowpkg.TaskDefinition{
				Name:        taskNameConvertProtocol,
				EnsureClean: false,
				Actions: []workflowpkg.TaskActionDefinition{
					{Type: "repo.remote.convert-protocol", Options: options},
				},
			})

		case *workflowpkg.RenameOperation:
			options := map[string]any{
				"require_clean": typedOperation.RequireCleanWorktree,
				"include_owner": typedOperation.IncludeOwner,
			}
			taskDefinitions = append(taskDefinitions, workflowpkg.TaskDefinition{
				Name:        taskNameRenameDirectories,
				EnsureClean: false,
				Actions: []workflowpkg.TaskActionDefinition{
					{Type: "repo.folder.rename", Options: options},
				},
			})

			accumulatedRuntime.IncludeNestedRepositories = true
			accumulatedRuntime.ProcessRepositoriesByDescendingDepth = true
			if typedOperation.RequireCleanWorktree {
				accumulatedRuntime.CaptureInitialWorktreeStatus = true
			}

		case *workflowpkg.BranchMigrationOperation:
			if len(typedOperation.Targets) == 0 {
				continue
			}
			if len(typedOperation.Targets) > 1 {
				return nil, workflowpkg.RuntimeOptions{}, fmt.Errorf("default-branch step supports a single target; received %d", len(typedOperation.Targets))
			}

			target := typedOperation.Targets[0]
			options := map[string]any{}

			trimmedTarget := strings.TrimSpace(target.TargetBranch)
			if len(trimmedTarget) == 0 {
				trimmedTarget = defaultMigrationTargetFallback
			}
			options["target"] = trimmedTarget

			if trimmedSource := strings.TrimSpace(target.SourceBranch); len(trimmedSource) > 0 {
				options["source"] = trimmedSource
			}

			if trimmedRemote := strings.TrimSpace(target.RemoteName); len(trimmedRemote) > 0 {
				options["remote"] = trimmedRemote
			} else {
				options["remote"] = defaultMigrationRemoteFallback
			}

			if !target.PushToRemote {
				options["push"] = false
			}
			if target.DeleteSourceBranch {
				options["delete_source_branch"] = true
			}

			taskDefinitions = append(taskDefinitions, workflowpkg.TaskDefinition{
				Name:        fmt.Sprintf(taskNamePromoteDefaultBranch, trimmedTarget),
				EnsureClean: false,
				Actions: []workflowpkg.TaskActionDefinition{
					{Type: "branch.default", Options: options},
				},
			})

		case *workflowpkg.AuditReportOperation:
			options := map[string]any{}
			if trimmedOutput := strings.TrimSpace(typedOperation.OutputPath); len(trimmedOutput) > 0 {
				options["output"] = trimmedOutput
			}
			taskDefinitions = append(taskDefinitions, workflowpkg.TaskDefinition{
				Name:        taskNameGenerateAuditReport,
				EnsureClean: false,
				Actions: []workflowpkg.TaskActionDefinition{
					{Type: "audit.report", Options: options},
				},
			})

		default:
			return nil, workflowpkg.RuntimeOptions{}, fmt.Errorf("unsupported workflow operation: %s", operation.Name())
		}
	}

	return taskDefinitions, accumulatedRuntime, nil
}

func orderOperationNodes(nodes []*workflowpkg.OperationNode) ([]*workflowpkg.OperationNode, error) {
	if len(nodes) == 0 {
		return nil, nil
	}

	sanitizedNames := make(map[*workflowpkg.OperationNode]string, len(nodes))
	nameToNode := make(map[string]*workflowpkg.OperationNode, len(nodes))
	inDegree := make(map[string]int, len(nodes))
	adjacency := make(map[string][]string, len(nodes))

	for _, node := range nodes {
		if node == nil || node.Operation == nil {
			continue
		}

		name := strings.TrimSpace(node.Name)
		if len(name) == 0 {
			name = strings.TrimSpace(node.Operation.Name())
		}
		if len(name) == 0 {
			return nil, errors.New("workflow operation missing name")
		}
		if _, exists := nameToNode[name]; exists {
			return nil, fmt.Errorf("workflow operation %q defined multiple times", name)
		}

		sanitizedNames[node] = name
		nameToNode[name] = node
		inDegree[name] = 0
		adjacency[name] = make([]string, 0)
	}

	for _, node := range nodes {
		if node == nil || node.Operation == nil {
			continue
		}

		nodeName := sanitizedNames[node]
		for _, dependency := range node.Dependencies {
			dependencyName := strings.TrimSpace(dependency)
			if len(dependencyName) == 0 {
				continue
			}
			if _, exists := nameToNode[dependencyName]; !exists {
				return nil, fmt.Errorf("workflow step %q depends on unknown step %q", node.Name, dependencyName)
			}

			adjacency[dependencyName] = append(adjacency[dependencyName], nodeName)
			inDegree[nodeName]++
		}
	}

	ready := make(map[string]struct{}, len(nameToNode))
	for _, node := range nodes {
		if node == nil || node.Operation == nil {
			continue
		}

		name := sanitizedNames[node]
		if inDegree[name] == 0 {
			ready[name] = struct{}{}
		}
	}

	ordered := make([]*workflowpkg.OperationNode, 0, len(nameToNode))
	processed := 0

	for len(ready) > 0 {
		nextName := nextReadyNodeName(ready, nodes, sanitizedNames)
		if len(nextName) == 0 {
			break
		}

		delete(ready, nextName)
		ordered = append(ordered, nameToNode[nextName])
		processed++

		for _, dependent := range adjacency[nextName] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				ready[dependent] = struct{}{}
			}
		}
	}

	if processed != len(nameToNode) {
		return nil, errors.New("workflow operations contain cycle")
	}

	return ordered, nil
}

func nextReadyNodeName(ready map[string]struct{}, nodes []*workflowpkg.OperationNode, names map[*workflowpkg.OperationNode]string) string {
	for _, node := range nodes {
		if node == nil || node.Operation == nil {
			continue
		}

		name := names[node]
		if _, exists := ready[name]; exists {
			return name
		}
	}

	return ""
}
