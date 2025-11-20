package cd

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/workflow"
)

type recordingReporter struct {
	events []shared.Event
}

func (reporter *recordingReporter) Report(event shared.Event) {
	reporter.events = append(reporter.events, event)
}

func (reporter *recordingReporter) Summary() string {
	return ""
}

func (reporter *recordingReporter) SummaryData() shared.SummaryData {
	return shared.SummaryData{}
}

func (reporter *recordingReporter) PrintSummary() {
}

func (reporter *recordingReporter) RecordEvent(string, shared.EventLevel) {
}

func (reporter *recordingReporter) RecordOperationDuration(string, time.Duration) {
}

func (reporter *recordingReporter) RecordStageDuration(string, time.Duration) {
}

type branchCaptureExecutor struct {
	commands []execshell.CommandDetails
}

func (executor *branchCaptureExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	if len(details.Arguments) == 0 {
		return execshell.ExecutionResult{}, nil
	}
	switch details.Arguments[0] {
	case "remote":
		return execshell.ExecutionResult{StandardOutput: "origin\n"}, nil
	case "rev-parse":
		if len(details.Arguments) > 2 && details.Arguments[1] == "--abbrev-ref" && details.Arguments[2] == "HEAD" {
			return execshell.ExecutionResult{StandardOutput: "main\n"}, nil
		}
		if len(details.Arguments) > 1 && details.Arguments[1] == "HEAD" {
			return execshell.ExecutionResult{StandardOutput: "abcd1234\n"}, nil
		}
	}
	return execshell.ExecutionResult{}, nil
}

func (executor *branchCaptureExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func TestHandleBranchChangeActionUsesRepositoryDefault(t *testing.T) {
	executor := &stubGitExecutor{
		responses: []stubGitResponse{
			{result: execShellOutput("origin\n")},
		},
	}
	reporter := &recordingReporter{}
	environment := &workflow.Environment{
		GitExecutor: executor,
		Logger:      zap.NewNop(),
		Output:      io.Discard,
		Errors:      io.Discard,
		Reporter:    reporter,
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			RemoteDefaultBranch: "main",
		},
	}

	parameters := map[string]any{
		taskOptionBranchRemote: shared.OriginRemoteNameConstant,
	}

	require.NoError(t, handleBranchChangeAction(context.Background(), environment, repository, parameters))
	require.GreaterOrEqual(t, len(executor.recorded), 3)
	require.Equal(t, []string{"switch", "main"}, executor.recorded[2].Arguments)
	require.Len(t, reporter.events, 1)
	require.Equal(t, branchResolutionSourceRemoteDefault, reporter.events[0].Details["source"])
}

func TestHandleBranchChangeActionUsesConfiguredFallback(t *testing.T) {
	executor := &stubGitExecutor{
		responses: []stubGitResponse{
			{result: execShellOutput("origin\n")},
		},
	}
	reporter := &recordingReporter{}
	environment := &workflow.Environment{
		GitExecutor: executor,
		Logger:      zap.NewNop(),
		Output:      io.Discard,
		Errors:      io.Discard,
		Reporter:    reporter,
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			RemoteDefaultBranch: "",
		},
	}

	parameters := map[string]any{
		taskOptionBranchRemote:            shared.OriginRemoteNameConstant,
		taskOptionConfiguredDefaultBranch: "develop",
	}

	require.NoError(t, handleBranchChangeAction(context.Background(), environment, repository, parameters))
	require.GreaterOrEqual(t, len(executor.recorded), 3)
	require.Equal(t, []string{"switch", "develop"}, executor.recorded[2].Arguments)
	require.Len(t, reporter.events, 1)
	require.Equal(t, branchResolutionSourceConfigured, reporter.events[0].Details["source"])
}

func TestHandleBranchChangeActionErrorsWhenBranchCannotBeResolved(t *testing.T) {
	executor := &stubGitExecutor{
		responses: []stubGitResponse{
			{result: execShellOutput("origin\n")},
		},
	}
	environment := &workflow.Environment{
		GitExecutor: executor,
		Logger:      zap.NewNop(),
		Output:      io.Discard,
		Errors:      io.Discard,
	}
	repository := &workflow.RepositoryState{
		Path:       "/tmp/project",
		Inspection: audit.RepositoryInspection{},
	}

	parameters := map[string]any{
		taskOptionBranchRemote: shared.OriginRemoteNameConstant,
	}

	require.Error(t, handleBranchChangeAction(context.Background(), environment, repository, parameters))
}

func execShellOutput(output string) execshell.ExecutionResult {
	return execshell.ExecutionResult{StandardOutput: output}
}

func TestHandleBranchChangeActionRefreshesBranch(t *testing.T) {
	executor := &scriptedGitExecutor{remoteOutput: "origin\n"}
	gitManager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerError)
	reporter := &recordingReporter{}
	buffer := &strings.Builder{}
	environment := &workflow.Environment{
		GitExecutor:       executor,
		RepositoryManager: gitManager,
		Logger:            zap.NewNop(),
		Output:            buffer,
		Errors:            io.Discard,
		Reporter:          reporter,
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
	}

	parameters := map[string]any{
		taskOptionBranchName:     "feature/foo",
		taskOptionBranchRemote:   shared.OriginRemoteNameConstant,
		taskOptionRefreshEnabled: true,
		taskOptionRequireClean:   true,
	}

	require.NoError(t, handleBranchChangeAction(context.Background(), environment, repository, parameters))
	require.Len(t, executor.recorded, 10)
	require.Equal(t, []string{"remote"}, executor.recorded[0].Arguments)
	require.Equal(t, []string{"fetch", "--prune", "origin"}, executor.recorded[1].Arguments)
	require.Equal(t, []string{"switch", "feature/foo"}, executor.recorded[2].Arguments)
	require.Equal(t, []string{"pull", "--rebase"}, executor.recorded[3].Arguments)
	require.Equal(t, []string{"config", "--get", "branch.feature/foo.remote"}, executor.recorded[4].Arguments)
	require.Equal(t, []string{"status", "--porcelain"}, executor.recorded[5].Arguments)
	require.Equal(t, []string{"status", "--porcelain"}, executor.recorded[6].Arguments)
	require.Equal(t, []string{"fetch", "--prune"}, executor.recorded[7].Arguments)
	require.Equal(t, []string{"checkout", "feature/foo"}, executor.recorded[8].Arguments)
	require.Equal(t, []string{"pull", "--ff-only"}, executor.recorded[9].Arguments)
	require.Contains(t, buffer.String(), "REFRESHED: /tmp/project (feature/foo)")
	require.Len(t, reporter.events, 1)
	require.Equal(t, "true", reporter.events[0].Details["refresh"])
	require.Equal(t, "true", reporter.events[0].Details["require_clean"])
}

func TestHandleBranchChangeActionSkipsRefreshWhenDirty(t *testing.T) {
	executor := &scriptedGitExecutor{remoteOutput: "origin\n", statusOutput: " M dirty.txt\n"}
	repoManager, managerErr := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerErr)

	reporter := &recordingReporter{}
	environment := &workflow.Environment{
		GitExecutor:       executor,
		RepositoryManager: repoManager,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          reporter,
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
	}

	parameters := map[string]any{
		taskOptionBranchName:     "feature/foo",
		taskOptionRefreshEnabled: true,
		taskOptionRequireClean:   true,
	}

	err := handleBranchChangeAction(context.Background(), environment, repository, parameters)
	require.NoError(t, err)
	require.Len(t, executor.recorded, 6)
	require.Equal(t, []string{"remote"}, executor.recorded[0].Arguments)
	require.Equal(t, []string{"fetch", "--prune", "origin"}, executor.recorded[1].Arguments)
	require.Equal(t, []string{"switch", "feature/foo"}, executor.recorded[2].Arguments)
	require.Equal(t, []string{"pull", "--rebase"}, executor.recorded[3].Arguments)
	require.Equal(t, []string{"config", "--get", "branch.feature/foo.remote"}, executor.recorded[4].Arguments)
	require.Equal(t, []string{"status", "--porcelain"}, executor.recorded[5].Arguments)
	require.Len(t, reporter.events, 2)
	require.Equal(t, shared.EventCodeTaskSkip, reporter.events[0].Code)
	require.Contains(t, reporter.events[0].Message, "refresh skipped (dirty worktree)")
	require.Equal(t, "M dirty.txt", reporter.events[0].Details["status"])
	require.Equal(t, shared.EventCodeRepoSwitched, reporter.events[1].Code)
}

func TestHandleBranchChangeActionRefreshesWithUntrackedChanges(t *testing.T) {
	executor := &scriptedGitExecutor{remoteOutput: "origin\n", statusOutput: "?? notes.tmp\n"}
	gitManager, managerErr := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerErr)
	reporter := &recordingReporter{}
	buffer := &strings.Builder{}
	environment := &workflow.Environment{
		GitExecutor:       executor,
		RepositoryManager: gitManager,
		Logger:            zap.NewNop(),
		Output:            buffer,
		Errors:            io.Discard,
		Reporter:          reporter,
	}
	repository := &workflow.RepositoryState{Path: "/tmp/project"}

	parameters := map[string]any{
		taskOptionBranchName:     "feature/foo",
		taskOptionRefreshEnabled: true,
		taskOptionRequireClean:   true,
	}

	require.NoError(t, handleBranchChangeAction(context.Background(), environment, repository, parameters))
	require.Contains(t, buffer.String(), "REFRESHED: /tmp/project (feature/foo)")
	require.Len(t, reporter.events, 1)
	require.Equal(t, shared.EventCodeRepoSwitched, reporter.events[0].Code)
}

func TestHandleBranchChangeActionCapturesBranch(t *testing.T) {
	executor := &branchCaptureExecutor{}
	reporter := &recordingReporter{}
	variableName, nameErr := workflow.NewVariableName("initial_branch")
	require.NoError(t, nameErr)

	gitManager, managerErr := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerErr)

	environment := &workflow.Environment{
		GitExecutor:       executor,
		RepositoryManager: gitManager,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          reporter,
		Variables:         workflow.NewVariableStore(),
	}
	repository := &workflow.RepositoryState{Path: "/tmp/project"}
	parameters := map[string]any{
		taskOptionBranchName:   "feature/gitignore",
		taskOptionBranchRemote: shared.OriginRemoteNameConstant,
		"capture": map[string]any{
			"name":  "initial_branch",
			"value": "branch",
		},
	}

	require.NoError(t, handleBranchChangeAction(context.Background(), environment, repository, parameters))
	value, exists := environment.Variables.Get(variableName)
	require.True(t, exists)
	require.Equal(t, "main", value)
	capturedVariableName, capturedNameErr := workflow.NewVariableName("Captured.initial_branch")
	require.NoError(t, capturedNameErr)
	capturedValue, capturedExists := environment.Variables.Get(capturedVariableName)
	require.True(t, capturedExists)
	require.Equal(t, "main", capturedValue)
	kind, kindExists := environment.CaptureKindForVariable(variableName)
	require.True(t, kindExists)
	require.Equal(t, workflow.CaptureKindBranch, kind)
}

func TestHandleBranchChangeActionCapturesCommit(t *testing.T) {
	executor := &branchCaptureExecutor{}
	gitManager, managerErr := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerErr)
	environment := &workflow.Environment{
		GitExecutor:       executor,
		RepositoryManager: gitManager,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Variables:         workflow.NewVariableStore(),
	}
	repository := &workflow.RepositoryState{Path: "/tmp/project"}
	parameters := map[string]any{
		taskOptionBranchName:   "feature/gitignore",
		taskOptionBranchRemote: shared.OriginRemoteNameConstant,
		"capture": map[string]any{
			"name":  "initial_commit",
			"value": "commit",
		},
	}

	require.NoError(t, handleBranchChangeAction(context.Background(), environment, repository, parameters))
	name, _ := workflow.NewVariableName("initial_commit")
	value, exists := environment.Variables.Get(name)
	require.True(t, exists)
	require.Equal(t, "abcd1234", value)
	capturedVariableName, capturedNameErr := workflow.NewVariableName("Captured.initial_commit")
	require.NoError(t, capturedNameErr)
	capturedValue, capturedExists := environment.Variables.Get(capturedVariableName)
	require.True(t, capturedExists)
	require.Equal(t, "abcd1234", capturedValue)
	kind, kindExists := environment.CaptureKindForVariable(name)
	require.True(t, kindExists)
	require.Equal(t, workflow.CaptureKindCommit, kind)
}

func TestHandleBranchChangeActionCaptureRespectsOverwrite(t *testing.T) {
	executor := &branchCaptureExecutor{}
	reporter := &recordingReporter{}
	variableName, nameErr := workflow.NewVariableName("initial_branch")
	require.NoError(t, nameErr)
	capturedVariableName, capturedNameErr := workflow.NewVariableName("Captured.initial_branch")
	require.NoError(t, capturedNameErr)

	gitManager, managerErr := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerErr)

	environment := &workflow.Environment{
		GitExecutor:       executor,
		RepositoryManager: gitManager,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          reporter,
		Variables:         workflow.NewVariableStore(),
	}
	environment.StoreCaptureValue(variableName, "preserved", true)
	repository := &workflow.RepositoryState{Path: "/tmp/project"}
	parameters := map[string]any{
		taskOptionBranchName:   "feature/gitignore",
		taskOptionBranchRemote: shared.OriginRemoteNameConstant,
		"capture": map[string]any{
			"name":      "initial_branch",
			"value":     "branch",
			"overwrite": false,
		},
	}

	require.NoError(t, handleBranchChangeAction(context.Background(), environment, repository, parameters))
	value, exists := environment.Variables.Get(variableName)
	require.True(t, exists)
	require.Equal(t, "preserved", value)
	sharedValue, sharedExists := environment.Variables.Get(capturedVariableName)
	require.True(t, sharedExists)
	require.Equal(t, "preserved", sharedValue)
}

func TestHandleBranchChangeActionRestoresBranch(t *testing.T) {
	executor := &branchCaptureExecutor{}
	variableName, _ := workflow.NewVariableName("initial_branch")
	gitManager, managerErr := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerErr)
	environment := &workflow.Environment{
		GitExecutor:       executor,
		RepositoryManager: gitManager,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Variables:         workflow.NewVariableStore(),
	}
	environment.Variables.Set(variableName, "develop")
	environment.RecordCaptureKind(variableName, workflow.CaptureKindBranch)
	repository := &workflow.RepositoryState{Path: "/tmp/project"}
	parameters := map[string]any{
		"restore": map[string]any{
			"from": "initial_branch",
		},
	}

	require.NoError(t, handleBranchChangeAction(context.Background(), environment, repository, parameters))
	require.True(t, len(executor.commands) >= 3)
	foundSwitch := false
	for _, command := range executor.commands {
		if len(command.Arguments) >= 2 && command.Arguments[0] == "switch" && command.Arguments[1] == "develop" {
			foundSwitch = true
			break
		}
	}
	require.True(t, foundSwitch)
}

func TestHandleBranchChangeActionRestoresCommit(t *testing.T) {
	executor := &branchCaptureExecutor{}
	variableName, _ := workflow.NewVariableName("initial_commit")
	gitManager, managerErr := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerErr)
	environment := &workflow.Environment{
		GitExecutor:       executor,
		RepositoryManager: gitManager,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Variables:         workflow.NewVariableStore(),
	}
	environment.Variables.Set(variableName, "deadbeef")
	environment.RecordCaptureKind(variableName, workflow.CaptureKindCommit)
	repository := &workflow.RepositoryState{Path: "/tmp/project"}
	parameters := map[string]any{
		"restore": map[string]any{
			"from":  "initial_commit",
			"value": "commit",
		},
	}

	require.NoError(t, handleBranchChangeAction(context.Background(), environment, repository, parameters))
	foundCheckout := false
	for _, command := range executor.commands {
		if len(command.Arguments) >= 2 && command.Arguments[0] == "checkout" && command.Arguments[1] == "deadbeef" {
			foundCheckout = true
			break
		}
	}
	require.True(t, foundCheckout)
}
