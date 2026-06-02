package syncflow

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
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/workflow"
	"github.com/tyemirov/utils/llm"
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

type statefulBranchCaptureExecutor struct {
	commands      []execshell.CommandDetails
	currentBranch string
}

func (executor *statefulBranchCaptureExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	if len(details.Arguments) == 0 {
		return execshell.ExecutionResult{}, nil
	}
	switch details.Arguments[0] {
	case "status":
		return execshell.ExecutionResult{}, nil
	case "remote":
		return execshell.ExecutionResult{StandardOutput: "origin\n"}, nil
	case "rev-parse":
		if len(details.Arguments) > 2 && details.Arguments[1] == "--abbrev-ref" && details.Arguments[2] == "HEAD" {
			return execshell.ExecutionResult{StandardOutput: executor.currentBranch + "\n"}, nil
		}
	case "switch":
		if len(details.Arguments) > 1 {
			executor.currentBranch = details.Arguments[1]
		}
		return execshell.ExecutionResult{}, nil
	}
	return execshell.ExecutionResult{}, nil
}

func (executor *statefulBranchCaptureExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type strictSyncGitExecutor struct {
	commands          []execshell.CommandDetails
	statusOutput      string
	diffStatOutput    string
	diffOutput        string
	revListOutput     string
	missingReferences map[string]bool
	mergeError        error
}

func (executor *strictSyncGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	if len(details.Arguments) == 0 {
		return execshell.ExecutionResult{}, nil
	}
	switch details.Arguments[0] {
	case "status":
		return execshell.ExecutionResult{StandardOutput: executor.statusOutput}, nil
	case "diff":
		if commandHasArgument(details.Arguments, "--stat") {
			output := executor.diffStatOutput
			if output == "" {
				output = " README.md | 1 +\n"
			}
			return execshell.ExecutionResult{StandardOutput: output}, nil
		}
		output := executor.diffOutput
		if output == "" {
			output = "diff --git a/README.md b/README.md\n+sync work\n"
		}
		return execshell.ExecutionResult{StandardOutput: output}, nil
	case "rev-parse":
		if len(details.Arguments) > 2 && details.Arguments[1] == "--verify" && executor.missingReferences[details.Arguments[2]] {
			return execshell.ExecutionResult{}, commandFailedError("fatal: Needed a single revision")
		}
		if strings.Join(details.Arguments, " ") == "rev-parse --verify origin/feature/foo" {
			return execshell.ExecutionResult{}, nil
		}
	case "rev-list":
		output := executor.revListOutput
		if output == "" {
			output = "0\n"
		}
		return execshell.ExecutionResult{StandardOutput: output}, nil
	case "merge":
		if executor.mergeError != nil {
			return execshell.ExecutionResult{}, executor.mergeError
		}
	case "switch":
		if len(details.Arguments) > 2 && details.Arguments[1] == gitCreateBranchFlagConstant && executor.missingReferences != nil {
			delete(executor.missingReferences, "refs/heads/"+details.Arguments[2])
		}
	}
	return execshell.ExecutionResult{}, nil
}

func (executor *strictSyncGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type strictSyncGitHubExecutor struct {
	output   string
	commands []execshell.CommandDetails
}

func (executor *strictSyncGitHubExecutor) ExecuteGit(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func (executor *strictSyncGitHubExecutor) ExecuteGitHubCLI(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	return execshell.ExecutionResult{StandardOutput: executor.output}, nil
}

type strictSyncChatClient struct {
	response string
	requests []llm.ChatRequest
}

func (client *strictSyncChatClient) Chat(_ context.Context, request llm.ChatRequest) (string, error) {
	client.requests = append(client.requests, request)
	if client.response == "" {
		return "feat: sync dirty work", nil
	}
	return client.response, nil
}

func TestHandleBranchSyncActionUsesRepositoryDefault(t *testing.T) {
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

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	require.GreaterOrEqual(t, len(executor.recorded), 3)
	require.Equal(t, []string{"switch", "main"}, executor.recorded[2].Arguments)
	require.Len(t, reporter.events, 1)
	require.Equal(t, branchResolutionSourceRemoteDefault, reporter.events[0].Details["source"])
}

func TestHandleBranchSyncActionUsesConfiguredFallback(t *testing.T) {
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

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	require.GreaterOrEqual(t, len(executor.recorded), 3)
	require.Equal(t, []string{"switch", "develop"}, executor.recorded[2].Arguments)
	require.Len(t, reporter.events, 1)
	require.Equal(t, branchResolutionSourceConfigured, reporter.events[0].Details["source"])
}

func TestHandleBranchSyncActionErrorsWhenBranchCannotBeResolved(t *testing.T) {
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

	require.Error(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
}

func execShellOutput(output string) execshell.ExecutionResult {
	return execshell.ExecutionResult{StandardOutput: output}
}

func TestHandleBranchSyncActionRefreshesBranch(t *testing.T) {
	executor := newScriptedGitExecutor("origin\n", "")
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

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	require.Len(t, executor.recorded, 10)
	require.Equal(t, []string{"status", "--porcelain"}, executor.recorded[0].Arguments)
	require.Equal(t, []string{"remote"}, executor.recorded[1].Arguments)
	require.Equal(t, []string{"fetch", "--prune", "origin"}, executor.recorded[2].Arguments)
	require.Equal(t, []string{"switch", "feature/foo"}, executor.recorded[3].Arguments)
	require.Equal(t, []string{"pull", "--ff-only"}, executor.recorded[4].Arguments)
	require.Equal(t, []string{"config", "--get", "branch.feature/foo.remote"}, executor.recorded[5].Arguments)
	require.Equal(t, []string{"status", "--porcelain"}, executor.recorded[6].Arguments)
	require.Equal(t, []string{"fetch", "--prune"}, executor.recorded[7].Arguments)
	require.Equal(t, []string{"checkout", "feature/foo"}, executor.recorded[8].Arguments)
	require.Equal(t, []string{"pull", "--ff-only"}, executor.recorded[9].Arguments)
	require.Contains(t, buffer.String(), "REFRESHED: /tmp/project (feature/foo)")
	require.Len(t, reporter.events, 1)
	require.Equal(t, "true", reporter.events[0].Details["refresh"])
	require.Equal(t, "true", reporter.events[0].Details["require_clean"])
}

func TestHandleBranchSyncActionStrictPRBranchMergesBaseAndPushes(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Feature","headRefName":"feature/foo"}]`}
	githubClient, githubClientError := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientError)
	buffer := &strings.Builder{}
	environment := &workflow.Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: gitManager,
		GitHubClient:      githubClient,
		Logger:            zap.NewNop(),
		Output:            buffer,
		Errors:            io.Discard,
		Reporter:          &recordingReporter{},
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			LocalBranch:    "master",
			FinalOwnerRepo: "owner/project",
		},
	}
	parameters := map[string]any{
		taskOptionBranchName:         "feature/foo",
		taskOptionBranchRemote:       shared.OriginRemoteNameConstant,
		taskOptionRequirePullRequest: true,
		taskOptionBaseBranch:         "master",
		taskOptionRequireClean:       true,
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "fetch --prune origin")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "switch feature/foo")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "reset --hard origin/feature/foo")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "merge --no-edit origin/master")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "push origin feature/foo")
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "pull --rebase")
	require.Contains(t, buffer.String(), "SYNCED: /tmp/project (feature/foo)")
	require.Len(t, githubExecutor.commands, 1)
	require.Contains(t, githubExecutor.commands[0].Arguments, "list")
}

func TestHandleBranchSyncActionStrictPRBranchAutoCommitsDirtySameBranch(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		statusOutput:  " M README.md\n",
		revListOutput: "1\n",
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Feature","headRefName":"feature/foo"}]`}
	githubClient, githubClientError := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientError)
	chatClient := &strictSyncChatClient{}
	environment := &workflow.Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: gitManager,
		GitHubClient:      githubClient,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          &recordingReporter{},
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			LocalBranch:    "feature/foo",
			FinalOwnerRepo: "owner/project",
		},
	}
	parameters := map[string]any{
		taskOptionBranchName:         "feature/foo",
		taskOptionBranchRemote:       shared.OriginRemoteNameConstant,
		taskOptionRequirePullRequest: true,
		taskOptionBaseBranch:         "master",
		taskOptionRequireClean:       false,
		taskOptionWorktreeCommitMessage: worktreeAdoptionCommitMessageOptions{
			Client: chatClient,
		},
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "reset")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "add --all -- README.md")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "commit -m feat: sync dirty work")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "merge --no-edit origin/feature/foo")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "merge --no-edit origin/master")
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "reset --hard origin/feature/foo")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "push origin feature/foo")
	require.Len(t, chatClient.requests, 1)
}

func TestHandleBranchSyncActionStrictPRBranchRequireCleanRejectsDirtyWorktree(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{statusOutput: " M README.md\n"}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Feature","headRefName":"feature/foo"}]`}
	githubClient, githubClientError := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientError)
	chatClient := &strictSyncChatClient{}
	environment := &workflow.Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: gitManager,
		GitHubClient:      githubClient,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          &recordingReporter{},
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			LocalBranch:    "feature/foo",
			FinalOwnerRepo: "owner/project",
		},
	}
	parameters := map[string]any{
		taskOptionBranchName:         "feature/foo",
		taskOptionBranchRemote:       shared.OriginRemoteNameConstant,
		taskOptionRequirePullRequest: true,
		taskOptionBaseBranch:         "master",
		taskOptionRequireClean:       true,
		taskOptionWorktreeCommitMessage: worktreeAdoptionCommitMessageOptions{
			Client: chatClient,
		},
	}

	syncError := handleBranchSyncAction(context.Background(), environment, repository, parameters)
	require.Error(t, syncError)
	require.Contains(t, syncError.Error(), "worktree is dirty")
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "commit -m")
	require.Len(t, chatClient.requests, 0)
}

func TestHandleBranchSyncActionStrictPRBranchRejectsMissingPullRequest(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[]`}
	githubClient, githubClientError := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientError)
	environment := &workflow.Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: gitManager,
		GitHubClient:      githubClient,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          &recordingReporter{},
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			LocalBranch:    "master",
			FinalOwnerRepo: "owner/project",
		},
	}
	parameters := map[string]any{
		taskOptionBranchName:         "feature/foo",
		taskOptionBranchRemote:       shared.OriginRemoteNameConstant,
		taskOptionRequirePullRequest: true,
		taskOptionBaseBranch:         "master",
		taskOptionRequireClean:       true,
	}

	syncError := handleBranchSyncAction(context.Background(), environment, repository, parameters)
	require.Error(t, syncError)
	require.Contains(t, syncError.Error(), "does not have an open pull request")
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "merge --no-edit origin/master")
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "push origin feature/foo")
}

func TestHandleBranchSyncActionStrictPRBranchCommitFlagUsesDirtySyncCommit(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		statusOutput:  " M README.md\n",
		revListOutput: "1\n",
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Feature","headRefName":"feature/foo"}]`}
	githubClient, githubClientError := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientError)
	chatClient := &strictSyncChatClient{response: "fix: describe dirty sync"}
	environment := &workflow.Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: gitManager,
		GitHubClient:      githubClient,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          &recordingReporter{},
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			LocalBranch:    "feature/foo",
			FinalOwnerRepo: "owner/project",
		},
	}
	parameters := map[string]any{
		taskOptionBranchName:         "feature/foo",
		taskOptionBranchRemote:       shared.OriginRemoteNameConstant,
		taskOptionRequirePullRequest: true,
		taskOptionBaseBranch:         "master",
		taskOptionRequireClean:       false,
		taskOptionCommitChanges:      true,
		taskOptionWorktreeCommitMessage: worktreeAdoptionCommitMessageOptions{
			Client: chatClient,
		},
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "add --all -- README.md")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "commit -m fix: describe dirty sync")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "merge --no-edit origin/feature/foo")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "merge --no-edit origin/master")
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "reset --hard origin/feature/foo")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "push origin feature/foo")
	require.Len(t, chatClient.requests, 1)
}

func TestHandleBranchSyncActionStrictPRBranchCreatesMissingRemoteBranchAndPullRequest(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		missingReferences: map[string]bool{
			"origin/feature/foo":     true,
			"refs/heads/feature/foo": true,
		},
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{}
	githubClient, githubClientError := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientError)
	environment := &workflow.Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: gitManager,
		GitHubClient:      githubClient,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          &recordingReporter{},
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			LocalBranch:    "master",
			FinalOwnerRepo: "owner/project",
		},
	}
	parameters := map[string]any{
		taskOptionBranchName:         "feature/foo",
		taskOptionBranchRemote:       shared.OriginRemoteNameConstant,
		taskOptionRequirePullRequest: true,
		taskOptionBaseBranch:         "master",
		taskOptionRequireClean:       true,
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "switch -c feature/foo origin/master")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "push -u origin feature/foo")
	require.Len(t, githubExecutor.commands, 1)
	require.Equal(t, []string{"pr", "create", "--repo", "owner/project", "--base", "master", "--head", "feature/foo", "--title", "feature/foo", "--body", strictSyncCreatedPRBody}, githubExecutor.commands[0].Arguments)
}

func TestHandleBranchSyncActionStrictPRBranchCreatesGeneratedBranchFromDirtyMaster(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		statusOutput: " M README.md\n",
		missingReferences: map[string]bool{
			"origin/sync/project/readme":     true,
			"refs/heads/sync/project/readme": true,
		},
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{}
	githubClient, githubClientError := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientError)
	chatClient := &strictSyncChatClient{response: "docs: update readme"}
	environment := &workflow.Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: gitManager,
		GitHubClient:      githubClient,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          &recordingReporter{},
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			LocalBranch:    "master",
			FinalOwnerRepo: "owner/project",
		},
	}
	parameters := map[string]any{
		taskOptionBranchName:         "master",
		taskOptionBranchRemote:       shared.OriginRemoteNameConstant,
		taskOptionRequirePullRequest: true,
		taskOptionBaseBranch:         "master",
		taskOptionRequireClean:       false,
		taskOptionWorktreeCommitMessage: worktreeAdoptionCommitMessageOptions{
			Client: chatClient,
		},
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "switch -c sync/project/readme origin/master")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "add --all -- README.md")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "commit -m docs: update readme")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "merge --no-edit origin/master")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "push -u origin sync/project/readme")
	require.Len(t, githubExecutor.commands, 1)
	require.Equal(t, []string{"pr", "create", "--repo", "owner/project", "--base", "master", "--head", "sync/project/readme", "--title", "sync/project/readme", "--body", strictSyncCreatedPRBody}, githubExecutor.commands[0].Arguments)
	require.Len(t, chatClient.requests, 1)
}

func TestHandleBranchSyncActionStrictPRBranchStopsBeforePushOnMergeConflict(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{mergeError: commandFailedError("CONFLICT (content): Merge conflict in README.md")}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Feature","headRefName":"feature/foo"}]`}
	githubClient, githubClientError := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientError)
	environment := &workflow.Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: gitManager,
		GitHubClient:      githubClient,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          &recordingReporter{},
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			LocalBranch:    "master",
			FinalOwnerRepo: "owner/project",
		},
	}
	parameters := map[string]any{
		taskOptionBranchName:         "feature/foo",
		taskOptionBranchRemote:       shared.OriginRemoteNameConstant,
		taskOptionRequirePullRequest: true,
		taskOptionBaseBranch:         "master",
		taskOptionRequireClean:       true,
	}

	syncError := handleBranchSyncAction(context.Background(), environment, repository, parameters)
	require.Error(t, syncError)
	require.Contains(t, syncError.Error(), "stopped with conflicts")
	require.Contains(t, syncError.Error(), "CONFLICT")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "merge --no-edit origin/master")
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "push origin feature/foo")
}

func recordedGitCommands(commands []execshell.CommandDetails) string {
	lines := make([]string, 0, len(commands))
	for _, command := range commands {
		lines = append(lines, strings.Join(command.Arguments, " "))
	}
	return strings.Join(lines, "\n")
}

func commandHasArgument(arguments []string, target string) bool {
	for argumentIndex := range arguments {
		if arguments[argumentIndex] == target {
			return true
		}
	}
	return false
}

func TestHandleBranchSyncActionConfiguresTrackingRemoteWhenMissing(t *testing.T) {
	executor := newScriptedGitExecutor("origin\n", "")
	executor.configError = commandFailedErrorWithExitCode("error: key does not contain a section", 1)
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
	repository := &workflow.RepositoryState{Path: "/tmp/project"}

	parameters := map[string]any{
		taskOptionBranchName:     "feature/foo",
		taskOptionBranchRemote:   shared.OriginRemoteNameConstant,
		taskOptionRefreshEnabled: true,
		taskOptionRequireClean:   true,
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	require.Contains(t, buffer.String(), "REFRESHED: /tmp/project (feature/foo)")
	setUpstreamCommand := []string{"branch", "--set-upstream-to=origin/feature/foo", "feature/foo"}
	found := false
	for _, recorded := range executor.recorded {
		if len(recorded.Arguments) != len(setUpstreamCommand) {
			continue
		}
		match := true
		for index := range setUpstreamCommand {
			if recorded.Arguments[index] != setUpstreamCommand[index] {
				match = false
				break
			}
		}
		if match {
			found = true
			break
		}
	}
	require.True(t, found, "expected branch --set-upstream-to to be invoked")
	require.Len(t, reporter.events, 1)
	require.Equal(t, shared.EventCodeRepoSwitched, reporter.events[0].Code)
}

func TestHandleBranchSyncActionWarnsWhenRemoteBranchMissing(t *testing.T) {
	executor := newScriptedGitExecutor("origin\n", "")
	executor.configError = commandFailedErrorWithExitCode("error: key does not contain a section", 1)
	executor.revParseError = commandFailedError("fatal: ambiguous argument 'origin/feature/foo': unknown revision or path not in the working tree")
	gitManager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerError)
	reporter := &recordingReporter{}
	environment := &workflow.Environment{
		GitExecutor:       executor,
		RepositoryManager: gitManager,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          reporter,
	}
	repository := &workflow.RepositoryState{Path: "/tmp/project"}

	parameters := map[string]any{
		taskOptionBranchName:     "feature/foo",
		taskOptionBranchRemote:   shared.OriginRemoteNameConstant,
		taskOptionRefreshEnabled: true,
		taskOptionRequireClean:   true,
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	require.Len(t, reporter.events, 2)
	require.Equal(t, shared.EventCodeTaskSkip, reporter.events[0].Code)
	require.Equal(t, "origin/feature/foo", reporter.events[0].Details["remote_candidate"])
	require.Equal(t, shared.EventCodeRepoSwitched, reporter.events[1].Code)
}

func TestHandleBranchSyncActionSkipsRefreshWhenDirty(t *testing.T) {
	executor := newScriptedGitExecutor("origin\n", " M dirty.txt\n")
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

	err := handleBranchSyncAction(context.Background(), environment, repository, parameters)
	require.NoError(t, err)
	require.Len(t, executor.recorded, 5)
	require.Equal(t, []string{"status", "--porcelain"}, executor.recorded[0].Arguments)
	require.Equal(t, []string{"remote"}, executor.recorded[1].Arguments)
	require.Equal(t, []string{"fetch", "--prune", "origin"}, executor.recorded[2].Arguments)
	require.Equal(t, []string{"switch", "feature/foo"}, executor.recorded[3].Arguments)
	require.Equal(t, []string{"pull", "--ff-only"}, executor.recorded[4].Arguments)
	require.Len(t, reporter.events, 2)
	require.Equal(t, shared.EventCodeTaskSkip, reporter.events[0].Code)
	require.Contains(t, reporter.events[0].Message, "refresh skipped (dirty worktree)")
	require.Equal(t, "M dirty.txt", reporter.events[0].Details["status"])
	require.Equal(t, shared.EventCodeRepoSwitched, reporter.events[1].Code)
}

func TestHandleBranchSyncActionRefreshesWithUntrackedChanges(t *testing.T) {
	executor := newScriptedGitExecutor("origin\n", "?? notes.tmp\n")
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

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	require.Contains(t, buffer.String(), "REFRESHED: /tmp/project (feature/foo)")
	require.Len(t, reporter.events, 2)
	require.Equal(t, shared.EventCodeRepoDirty, reporter.events[0].Code)
	require.Contains(t, reporter.events[0].Message, "notes.tmp")
	require.Equal(t, "notes.tmp", reporter.events[0].Details["paths"])
	require.Equal(t, shared.EventCodeRepoSwitched, reporter.events[1].Code)
}

func TestHandleBranchSyncActionStashesDoesNotPopWhenOnlyUntrackedChangesPresent(t *testing.T) {
	executor := newScriptedGitExecutor("origin\n", "?? notes.tmp\n")
	gitManager, managerErr := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerErr)
	reporter := &recordingReporter{}
	environment := &workflow.Environment{
		GitExecutor:       executor,
		RepositoryManager: gitManager,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          reporter,
	}
	repository := &workflow.RepositoryState{Path: "/tmp/project"}

	parameters := map[string]any{
		taskOptionBranchName:     "feature/foo",
		taskOptionBranchRemote:   shared.OriginRemoteNameConstant,
		taskOptionRefreshEnabled: true,
		taskOptionRequireClean:   true,
		taskOptionStashChanges:   true,
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	for _, recorded := range executor.recorded {
		require.NotEqual(t, []string{"stash", "pop"}, recorded.Arguments)
	}
}

func TestHandleBranchSyncActionStashesTrackedChangesOnceWhenUntrackedPresent(t *testing.T) {
	executor := newScriptedGitExecutor("origin\n", " M tracked.txt\n?? notes.tmp\n")
	gitManager, managerErr := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerErr)
	reporter := &recordingReporter{}
	environment := &workflow.Environment{
		GitExecutor:       executor,
		RepositoryManager: gitManager,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          reporter,
	}
	repository := &workflow.RepositoryState{Path: "/tmp/project"}

	parameters := map[string]any{
		taskOptionBranchName:     "feature/foo",
		taskOptionBranchRemote:   shared.OriginRemoteNameConstant,
		taskOptionRefreshEnabled: true,
		taskOptionRequireClean:   true,
		taskOptionStashChanges:   true,
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	stashPushCount := 0
	stashPopCount := 0
	for _, recorded := range executor.recorded {
		if len(recorded.Arguments) < 2 || recorded.Arguments[0] != "stash" {
			continue
		}
		switch recorded.Arguments[1] {
		case "push":
			stashPushCount++
		case "pop":
			stashPopCount++
		}
	}
	require.Equal(t, 1, stashPushCount)
	require.Equal(t, 1, stashPopCount)
}

func TestHandleBranchSyncActionStashesTrackedChangesAroundSwitch(t *testing.T) {
	executor := newScriptedGitExecutor("origin\n", " M tracked.txt\n")
	gitManager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, managerError)
	reporter := &recordingReporter{}
	environment := &workflow.Environment{
		GitExecutor:       executor,
		RepositoryManager: gitManager,
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
		taskOptionBranchRemote:   shared.OriginRemoteNameConstant,
		taskOptionRefreshEnabled: true,
		taskOptionRequireClean:   true,
		taskOptionStashChanges:   true,
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))

	findCommandIndex := func(target ...string) int {
		for idx, recorded := range executor.recorded {
			if len(recorded.Arguments) != len(target) {
				continue
			}
			match := true
			for i := range recorded.Arguments {
				if recorded.Arguments[i] != target[i] {
					match = false
					break
				}
			}
			if match {
				return idx
			}
		}
		return -1
	}

	stashPushIndex := findCommandIndex("stash", "push")
	switchIndex := findCommandIndex("switch", "feature/foo")
	stashPopIndex := findCommandIndex("stash", "pop")

	require.NotEqual(t, -1, stashPushIndex)
	require.NotEqual(t, -1, switchIndex)
	require.NotEqual(t, -1, stashPopIndex)
	require.Less(t, stashPushIndex, switchIndex)
	require.Less(t, switchIndex, stashPopIndex)
}

func TestHandleBranchSyncActionCapturesBranch(t *testing.T) {
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

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
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

func TestHandleBranchSyncActionCapturesOriginalBranchBeforeSwitch(t *testing.T) {
	executor := &statefulBranchCaptureExecutor{currentBranch: "feature/local-work"}
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
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			RemoteDefaultBranch: "master",
		},
	}
	parameters := map[string]any{
		taskOptionBranchRemote: shared.OriginRemoteNameConstant,
		"capture": map[string]any{
			"name":  "initial_branch",
			"value": "branch",
		},
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	value, exists := environment.Variables.Get(variableName)
	require.True(t, exists)
	require.Equal(t, "feature/local-work", value)
	require.Equal(t, "master", executor.currentBranch)
}

func TestHandleBranchSyncActionCapturesCommit(t *testing.T) {
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

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
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

func TestHandleBranchSyncActionCaptureRespectsOverwrite(t *testing.T) {
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

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	value, exists := environment.Variables.Get(variableName)
	require.True(t, exists)
	require.Equal(t, "preserved", value)
	sharedValue, sharedExists := environment.Variables.Get(capturedVariableName)
	require.True(t, sharedExists)
	require.Equal(t, "preserved", sharedValue)
}

func TestHandleBranchSyncActionRestoresBranch(t *testing.T) {
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

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
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

func TestHandleBranchSyncActionRestoresCommit(t *testing.T) {
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

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	foundCheckout := false
	for _, command := range executor.commands {
		if len(command.Arguments) >= 2 && command.Arguments[0] == "checkout" && command.Arguments[1] == "deadbeef" {
			foundCheckout = true
			break
		}
	}
	require.True(t, foundCheckout)
}
