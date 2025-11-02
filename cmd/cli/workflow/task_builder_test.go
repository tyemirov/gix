package workflow

import (
	"testing"

	"github.com/temirov/gix/internal/repos/shared"
	workflowpkg "github.com/temirov/gix/internal/workflow"
)

func TestBuildWorkflowTasksHonorsDependencies(t *testing.T) {
	nodes := []*workflowpkg.OperationNode{
		{
			Name:         "rename",
			Operation:    &workflowpkg.RenameOperation{RequireCleanWorktree: true},
			Dependencies: []string{"convert"},
		},
		{
			Name: "convert",
			Operation: &workflowpkg.ProtocolConversionOperation{
				FromProtocol: shared.RemoteProtocolHTTPS,
				ToProtocol:   shared.RemoteProtocolSSH,
			},
		},
		{
			Name:         "audit",
			Operation:    &workflowpkg.AuditReportOperation{OutputPath: "report.md"},
			Dependencies: []string{"rename"},
		},
	}

	tasks, runtime, err := buildWorkflowTasks(nodes)
	if err != nil {
		t.Fatalf("buildWorkflowTasks returned error: %v", err)
	}

	expectedNames := []string{
		taskNameConvertProtocol,
		taskNameRenameDirectories,
		taskNameGenerateAuditReport,
	}

	if len(tasks) != len(expectedNames) {
		t.Fatalf("expected %d tasks, received %d", len(expectedNames), len(tasks))
	}

	for index, expected := range expectedNames {
		if tasks[index].Name != expected {
			t.Errorf("task %d expected name %q, received %q", index, expected, tasks[index].Name)
		}
	}

	if !runtime.IncludeNestedRepositories {
		t.Error("expected runtime to include nested repositories")
	}
	if !runtime.ProcessRepositoriesByDescendingDepth {
		t.Error("expected runtime to process repositories by descending depth")
	}
	if !runtime.CaptureInitialWorktreeStatus {
		t.Error("expected runtime to capture initial worktree status")
	}
}

func TestBuildWorkflowTasksDetectsCycles(t *testing.T) {
	nodes := []*workflowpkg.OperationNode{
		{
			Name:         "first",
			Operation:    &workflowpkg.CanonicalRemoteOperation{},
			Dependencies: []string{"second"},
		},
		{
			Name:         "second",
			Operation:    &workflowpkg.RenameOperation{},
			Dependencies: []string{"first"},
		},
	}

	_, _, err := buildWorkflowTasks(nodes)
	if err == nil {
		t.Fatal("expected cycle detection error, received nil")
	}
}
