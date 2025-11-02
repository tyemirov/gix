package workflow

import (
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
	taskDefinitions := make([]workflowpkg.TaskDefinition, 0)
	accumulatedRuntime := workflowpkg.RuntimeOptions{}

	for nodeIndex := range nodes {
		node := nodes[nodeIndex]
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
