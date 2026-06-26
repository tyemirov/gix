package syncflow

import (
	"context"
	"fmt"
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
	"github.com/tyemirov/gix/internal/repos/prompt"
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
	commands           []execshell.CommandDetails
	statusOutput       string
	diffStatOutput     string
	diffOutput         string
	revListOutput      string
	missingReferences  map[string]bool
	ignoredPaths       map[string]bool
	cachedIgnoredPaths map[string]bool
	commandErrors      map[string]error
	mergeError         error
	blockedBranch      string
	blockedWorktree    string
	worktreeRemoved    bool
	currentBranch      string
}

const (
	strictSyncGitAbbrevRefFlag = "--abbrev-ref"
	strictSyncGitHeadReference = "HEAD"
)

func (executor *strictSyncGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	if len(details.Arguments) == 0 {
		return execshell.ExecutionResult{}, nil
	}
	if executor.commandErrors != nil {
		if commandErr, exists := executor.commandErrors[strings.Join(details.Arguments, " ")]; exists {
			return execshell.ExecutionResult{}, commandErr
		}
	}
	switch details.Arguments[0] {
	case "status":
		if details.WorkingDirectory == executor.blockedWorktree && commandHasArgument(details.Arguments, gitPorcelainBranchFlagConstant) {
			return execshell.ExecutionResult{StandardOutput: fmt.Sprintf("## %s...origin/%s\n M README.md\n", executor.blockedBranch, executor.blockedBranch)}, nil
		}
		if details.WorkingDirectory == executor.blockedWorktree {
			return execshell.ExecutionResult{StandardOutput: "M  README.md\n"}, nil
		}
		return execshell.ExecutionResult{StandardOutput: executor.statusOutput}, nil
	case "check-ignore":
		ignored := make([]string, 0)
		for _, candidatePath := range strings.Split(string(details.StandardInput), "\n") {
			trimmedPath := strings.TrimSpace(candidatePath)
			if trimmedPath == "" {
				continue
			}
			if executor.ignoredPaths[trimmedPath] {
				ignored = append(ignored, trimmedPath)
			}
		}
		if len(ignored) == 0 {
			return execshell.ExecutionResult{}, commandFailedErrorWithExitCode("", 1)
		}
		return execshell.ExecutionResult{StandardOutput: strings.Join(ignored, "\n") + "\n"}, nil
	case "ls-files":
		if commandHasArgument(details.Arguments, "--cached") && commandHasArgument(details.Arguments, "--ignored") {
			ignored := make([]string, 0)
			for argumentIndex := range details.Arguments {
				if details.Arguments[argumentIndex] != gitPathspecSeparatorConstant {
					continue
				}
				for pathIndex := argumentIndex + 1; pathIndex < len(details.Arguments); pathIndex++ {
					candidatePath := details.Arguments[pathIndex]
					if executor.cachedIgnoredPaths[candidatePath] {
						ignored = append(ignored, candidatePath)
					}
				}
				break
			}
			if len(ignored) == 0 {
				return execshell.ExecutionResult{}, nil
			}
			return execshell.ExecutionResult{StandardOutput: strings.Join(ignored, "\n") + "\n"}, nil
		}
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
		if len(details.Arguments) > 2 && details.Arguments[1] == strictSyncGitAbbrevRefFlag && details.Arguments[2] == strictSyncGitHeadReference {
			currentBranch := executor.currentBranch
			if currentBranch == "" {
				currentBranch = defaultSyncBaseBranch
			}
			return execshell.ExecutionResult{StandardOutput: currentBranch + "\n"}, nil
		}
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
		if len(details.Arguments) > 1 && details.Arguments[1] == executor.blockedBranch && !executor.worktreeRemoved {
			return execshell.ExecutionResult{}, commandFailedError(fmt.Sprintf("fatal: %q is already used by worktree at %q", executor.blockedBranch, executor.blockedWorktree))
		}
		if len(details.Arguments) > 2 && details.Arguments[1] == gitCreateBranchFlagConstant && executor.missingReferences != nil {
			delete(executor.missingReferences, "refs/heads/"+details.Arguments[2])
		}
	case "worktree":
		if len(details.Arguments) > 2 && details.Arguments[1] == gitWorktreeListSubcommandConstant {
			return execshell.ExecutionResult{StandardOutput: fmt.Sprintf("worktree /tmp/project\nbranch refs/heads/master\n\nworktree %s\nbranch refs/heads/%s\n", executor.blockedWorktree, executor.blockedBranch)}, nil
		}
		if len(details.Arguments) > 2 && details.Arguments[1] == gitWorktreeRemoveSubcommandConstant {
			executor.worktreeRemoved = true
		}
	}
	return execshell.ExecutionResult{}, nil
}

func (executor *strictSyncGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type strictSyncGitHubExecutor struct {
	output   string
	outputs  []string
	commands []execshell.CommandDetails
}

func (executor *strictSyncGitHubExecutor) ExecuteGit(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func (executor *strictSyncGitHubExecutor) ExecuteGitHubCLI(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	outputIndex := len(executor.commands) - 1
	if outputIndex < len(executor.outputs) {
		return execshell.ExecutionResult{StandardOutput: executor.outputs[outputIndex]}, nil
	}
	return execshell.ExecutionResult{StandardOutput: executor.output}, nil
}

type strictSyncPrompter struct {
	result  shared.ConfirmationResult
	err     error
	prompts []string
}

func (prompter *strictSyncPrompter) Confirm(prompt string) (shared.ConfirmationResult, error) {
	prompter.prompts = append(prompter.prompts, prompt)
	if prompter.err != nil {
		return shared.ConfirmationResult{}, prompter.err
	}
	return prompter.result, nil
}

type strictSyncChatClient struct {
	response  string
	responses []string
	requests  []llm.ChatRequest
}

func (client *strictSyncChatClient) Chat(_ context.Context, request llm.ChatRequest) (string, error) {
	client.requests = append(client.requests, request)
	responseIndex := len(client.requests) - 1
	if responseIndex < len(client.responses) {
		return client.responses[responseIndex], nil
	}
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
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Feature","headRefName":"feature/foo","baseRefName":"master"}]`}
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

func TestHandleBranchSyncActionStrictPRBranchUsesOpenPullRequestBase(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":665,"title":"Feature","headRefName":"feature/foo","baseRefName":"tyemirov/bugfix/B012-parent"}]`}
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
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "merge --no-edit origin/tyemirov/bugfix/B012-parent")
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "merge --no-edit origin/master")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "push origin feature/foo")
	require.Len(t, githubExecutor.commands, 1)
	require.False(t, commandHasArgument(githubExecutor.commands[0].Arguments, "--base"))
	require.Contains(t, githubExecutor.commands[0].Arguments, "--head")
	require.Contains(t, githubExecutor.commands[0].Arguments, "feature/foo")
	require.Contains(t, buffer.String(), "SYNCED: /tmp/project (feature/foo)")
}

func TestHandleBranchSyncActionStrictPRBranchRejectsOpenPullRequestWithoutBase(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":665,"title":"Feature","headRefName":"feature/foo"}]`}
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
	require.Contains(t, syncError.Error(), `open pull request for branch "feature/foo" did not report a base branch`)
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "merge --no-edit")
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "push origin feature/foo")
}

func TestHandleBranchSyncActionStrictPRBranchAutoCommitsDirtySameBranch(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		statusOutput:  " M README.md\n",
		revListOutput: "1\n",
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Feature","headRefName":"feature/foo","baseRefName":"master"}]`}
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

func TestHandleBranchSyncActionStrictPRBranchFiltersIgnoredDirtyPathsBeforeStaging(t *testing.T) {
	testCases := []struct {
		name               string
		statusEntries      []string
		ignoredPaths       map[string]bool
		cachedIgnoredPaths map[string]bool
		commitMessage      string
		expectedCommands   []string
		rejectedCommands   []string
	}{
		{
			name: "untracked ignored entries mixed with generated metadata",
			statusEntries: []string{
				"?? python/llm_proxy_client.egg-info/PKG-INFO",
				"?? python/llm_proxy_client.egg-info/SOURCES.txt",
				"!! python/llm_proxy_client/__pycache__/client.cpython-313.pyc",
				"!! python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc",
			},
			ignoredPaths: map[string]bool{
				"python/llm_proxy_client/__pycache__/client.cpython-313.pyc":        true,
				"python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc": true,
			},
			commitMessage: "fix: sync generated client metadata",
			expectedCommands: []string{
				"check-ignore --stdin",
				"add --all -- python/llm_proxy_client.egg-info/PKG-INFO python/llm_proxy_client.egg-info/SOURCES.txt",
			},
			rejectedCommands: []string{
				"add --all -- python/llm_proxy_client/__pycache__",
				"add --all -- python/tests/__pycache__",
			},
		},
		{
			name: "cached ignored tracked modifications mixed with scripts",
			statusEntries: []string{
				" M python/llm_proxy_client/__pycache__/client.cpython-313.pyc",
				" M python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc",
				" M scripts/deploy.sh",
			},
			cachedIgnoredPaths: map[string]bool{
				"python/llm_proxy_client/__pycache__/client.cpython-313.pyc":        true,
				"python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc": true,
			},
			commitMessage: "fix: sync release scripts",
			expectedCommands: []string{
				"check-ignore --stdin",
				"ls-files --cached --ignored --exclude-standard -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc scripts/deploy.sh",
				"restore --staged --worktree -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc",
				"add --all -- scripts/deploy.sh",
			},
			rejectedCommands: []string{
				"add --all -- python/llm_proxy_client/__pycache__",
				"add --all -- python/tests/__pycache__",
			},
		},
		{
			name: "cached ignored deletion mixed with source update",
			statusEntries: []string{
				" D python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc",
				" M python/llm_proxy_client/client.py",
			},
			cachedIgnoredPaths: map[string]bool{
				"python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc": true,
			},
			commitMessage: "fix: sync python client",
			expectedCommands: []string{
				"check-ignore --stdin",
				"ls-files --cached --ignored --exclude-standard -- python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc python/llm_proxy_client/client.py",
				"restore --staged --worktree -- python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc",
				"add --all -- python/llm_proxy_client/client.py",
			},
			rejectedCommands: []string{
				"add --all -- python/tests/__pycache__",
			},
		},
		{
			name: "mixed check-ignore and cached ignored entries",
			statusEntries: []string{
				" M python/llm_proxy_client/__pycache__/client.cpython-313.pyc",
				"!! python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc",
				" M scripts/release.sh",
			},
			ignoredPaths: map[string]bool{
				"python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc": true,
			},
			cachedIgnoredPaths: map[string]bool{
				"python/llm_proxy_client/__pycache__/client.cpython-313.pyc": true,
			},
			commitMessage: "fix: sync release script",
			expectedCommands: []string{
				"check-ignore --stdin",
				"restore --staged --worktree -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc",
				"add --all -- scripts/release.sh",
			},
			rejectedCommands: []string{
				"add --all -- python/llm_proxy_client/__pycache__",
				"add --all -- python/tests/__pycache__",
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			gitExecutor := &strictSyncGitExecutor{
				statusOutput:       strings.Join(testCase.statusEntries, "\n") + "\n",
				revListOutput:      "1\n",
				ignoredPaths:       testCase.ignoredPaths,
				cachedIgnoredPaths: testCase.cachedIgnoredPaths,
			}
			gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
			require.NoError(t, managerError)
			githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Feature","headRefName":"feature/foo","baseRefName":"master"}]`}
			githubClient, githubClientError := githubcli.NewClient(githubExecutor)
			require.NoError(t, githubClientError)
			chatClient := &strictSyncChatClient{response: testCase.commitMessage}
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
			recordedCommands := recordedGitCommands(gitExecutor.commands)
			for _, expectedCommand := range testCase.expectedCommands {
				require.Contains(t, recordedCommands, expectedCommand)
			}
			for _, rejectedCommand := range testCase.rejectedCommands {
				require.NotContains(t, recordedCommands, rejectedCommand)
			}
			require.Contains(t, recordedCommands, "commit -m "+testCase.commitMessage)
			require.Contains(t, recordedCommands, "push origin feature/foo")
			require.Len(t, chatClient.requests, 1)
		})
	}
}

func TestHandleBranchSyncActionStrictPRBranchRestoresTrackedIgnoredOnlyStatus(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		statusOutput: strings.Join([]string{
			" M python/llm_proxy_client/__pycache__/client.cpython-313.pyc",
			" D python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc",
		}, "\n") + "\n",
		cachedIgnoredPaths: map[string]bool{
			"python/llm_proxy_client/__pycache__/client.cpython-313.pyc":        true,
			"python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc": true,
		},
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":8,"title":"Generated","headRefName":"gix/sync-dirty-work","baseRefName":"master"}]`}
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
	recordedCommands := recordedGitCommands(gitExecutor.commands)
	require.Contains(t, recordedCommands, "check-ignore --stdin")
	require.Contains(t, recordedCommands, "ls-files --cached --ignored --exclude-standard -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc")
	require.Contains(t, recordedCommands, "restore --staged --worktree -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc")
	require.Contains(t, recordedCommands, "switch master")
	require.Contains(t, recordedCommands, "reset --hard origin/master")
	require.NotContains(t, recordedCommands, "switch -c gix/sync-dirty-work")
	require.NotContains(t, recordedCommands, "add --all")
	require.NotContains(t, recordedCommands, "commit -m")
	require.Len(t, chatClient.requests, 0)
}

func TestHandleBranchSyncActionStrictPRBranchRestoresStagedTrackedIgnoredOnlyStatusWithRequireClean(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		statusOutput: strings.Join([]string{
			"M  python/llm_proxy_client/__pycache__/client.cpython-313.pyc",
			"D  python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc",
		}, "\n") + "\n",
		cachedIgnoredPaths: map[string]bool{
			"python/llm_proxy_client/__pycache__/client.cpython-313.pyc":        true,
			"python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc": true,
		},
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Feature","headRefName":"feature/foo","baseRefName":"master"}]`}
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

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	recordedCommands := recordedGitCommands(gitExecutor.commands)
	require.Contains(t, recordedCommands, "restore --staged --worktree -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc")
	require.Contains(t, recordedCommands, "switch feature/foo")
	require.Contains(t, recordedCommands, "reset --hard origin/feature/foo")
	require.Contains(t, recordedCommands, "merge --no-edit origin/master")
	require.Less(t, recordedGitCommandIndex(gitExecutor.commands, "fetch --prune origin"), recordedGitCommandIndex(gitExecutor.commands, "restore --staged --worktree -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc"))
	require.Less(t, recordedGitCommandIndex(gitExecutor.commands, "restore --staged --worktree -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc"), recordedGitCommandIndex(gitExecutor.commands, "switch feature/foo"))
	require.NotContains(t, recordedCommands, "add --all")
	require.NotContains(t, recordedCommands, "commit -m")
	require.Len(t, chatClient.requests, 0)
}

func TestHandleBranchSyncActionStrictPRBranchRestoresTrackedIgnoredStatusBeforeStashingDirtyWork(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		statusOutput: strings.Join([]string{
			" M python/llm_proxy_client/__pycache__/client.cpython-313.pyc",
			" M README.md",
		}, "\n") + "\n",
		cachedIgnoredPaths: map[string]bool{
			"python/llm_proxy_client/__pycache__/client.cpython-313.pyc": true,
		},
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Feature","headRefName":"feature/foo","baseRefName":"master"}]`}
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
		taskOptionStashChanges:       true,
		taskOptionRequireClean:       false,
		taskOptionWorktreeCommitMessage: worktreeAdoptionCommitMessageOptions{
			Client: chatClient,
		},
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	recordedCommands := recordedGitCommands(gitExecutor.commands)
	restoreCommand := "restore --staged --worktree -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc"
	stashPushCommand := "stash push --include-untracked"
	require.Contains(t, recordedCommands, restoreCommand)
	require.Contains(t, recordedCommands, stashPushCommand)
	require.Contains(t, recordedCommands, "stash pop")
	require.Less(t, recordedGitCommandIndex(gitExecutor.commands, "fetch --prune origin"), recordedGitCommandIndex(gitExecutor.commands, restoreCommand))
	require.Less(t, recordedGitCommandIndex(gitExecutor.commands, restoreCommand), recordedGitCommandIndex(gitExecutor.commands, stashPushCommand))
	require.Less(t, recordedGitCommandIndex(gitExecutor.commands, stashPushCommand), recordedGitCommandIndex(gitExecutor.commands, "switch feature/foo"))
	require.Less(t, recordedGitCommandIndex(gitExecutor.commands, "switch feature/foo"), recordedGitCommandIndex(gitExecutor.commands, "stash pop"))
	require.NotContains(t, recordedCommands, "add --all")
	require.NotContains(t, recordedCommands, "commit -m")
	require.Len(t, chatClient.requests, 0)
}

func TestHandleBranchSyncActionStrictPRBranchDoesNotRestoreTrackedIgnoredStatusWhenFetchFails(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		statusOutput: " M python/llm_proxy_client/__pycache__/client.cpython-313.pyc\n",
		cachedIgnoredPaths: map[string]bool{
			"python/llm_proxy_client/__pycache__/client.cpython-313.pyc": true,
		},
		commandErrors: map[string]error{
			"fetch --prune origin": commandFailedError("fatal: fetch failed"),
		},
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	environment := &workflow.Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: gitManager,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          &recordingReporter{},
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			LocalBranch: "master",
		},
	}
	parameters := map[string]any{
		taskOptionBranchName:         "master",
		taskOptionBranchRemote:       shared.OriginRemoteNameConstant,
		taskOptionRequirePullRequest: true,
		taskOptionBaseBranch:         "master",
		taskOptionRequireClean:       false,
	}

	syncError := handleBranchSyncAction(context.Background(), environment, repository, parameters)
	require.Error(t, syncError)
	require.Contains(t, syncError.Error(), "failed to fetch updates")
	recordedCommands := recordedGitCommands(gitExecutor.commands)
	require.Contains(t, recordedCommands, "fetch --prune origin")
	require.NotContains(t, recordedCommands, "restore --staged --worktree")
	require.NotContains(t, recordedCommands, "reset --hard origin/master")
}

func TestHandleBranchSyncActionStrictPRBranchStopsWhenTrackedIgnoredRestoreFails(t *testing.T) {
	restoreCommand := "restore --staged --worktree -- python/llm_proxy_client/__pycache__/client.cpython-313.pyc"
	gitExecutor := &strictSyncGitExecutor{
		statusOutput: " M python/llm_proxy_client/__pycache__/client.cpython-313.pyc\n",
		cachedIgnoredPaths: map[string]bool{
			"python/llm_proxy_client/__pycache__/client.cpython-313.pyc": true,
		},
		commandErrors: map[string]error{
			restoreCommand: commandFailedError("fatal: restore failed"),
		},
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	environment := &workflow.Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: gitManager,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          &recordingReporter{},
	}
	repository := &workflow.RepositoryState{
		Path: "/tmp/project",
		Inspection: audit.RepositoryInspection{
			LocalBranch: "master",
		},
	}
	parameters := map[string]any{
		taskOptionBranchName:         "master",
		taskOptionBranchRemote:       shared.OriginRemoteNameConstant,
		taskOptionRequirePullRequest: true,
		taskOptionBaseBranch:         "master",
		taskOptionRequireClean:       false,
	}

	syncError := handleBranchSyncAction(context.Background(), environment, repository, parameters)
	require.Error(t, syncError)
	require.Contains(t, syncError.Error(), "failed to restore ignored dirty sync paths")
	recordedCommands := recordedGitCommands(gitExecutor.commands)
	require.Contains(t, recordedCommands, restoreCommand)
	require.Less(t, recordedGitCommandIndex(gitExecutor.commands, "fetch --prune origin"), recordedGitCommandIndex(gitExecutor.commands, restoreCommand))
	require.NotContains(t, recordedCommands, "switch master")
	require.NotContains(t, recordedCommands, "reset --hard origin/master")
	require.NotContains(t, recordedCommands, "add --all")
	require.NotContains(t, recordedCommands, "commit -m")
}

func TestHandleBranchSyncActionStrictPRBranchTreatsIgnoredOnlyStatusAsClean(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		statusOutput: strings.Join([]string{
			"!! python/llm_proxy_client/__pycache__/client.cpython-313.pyc",
			"!! python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc",
		}, "\n") + "\n",
		ignoredPaths: map[string]bool{
			"python/llm_proxy_client/__pycache__/client.cpython-313.pyc":        true,
			"python/tests/__pycache__/test_client.cpython-313-pytest-9.0.3.pyc": true,
		},
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":8,"title":"Generated","headRefName":"gix/sync-dirty-work","baseRefName":"master"}]`}
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
	recordedCommands := recordedGitCommands(gitExecutor.commands)
	require.Contains(t, recordedCommands, "check-ignore --stdin")
	require.Contains(t, recordedCommands, "switch master")
	require.Contains(t, recordedCommands, "reset --hard origin/master")
	require.NotContains(t, recordedCommands, "switch -c gix/sync-dirty-work")
	require.NotContains(t, recordedCommands, "add --all")
	require.NotContains(t, recordedCommands, "commit -m")
	require.Len(t, chatClient.requests, 0)
}

func TestHandleBranchSyncActionStrictPRBranchAdoptsDirtySiblingWorktree(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		blockedBranch:   "feature/foo",
		blockedWorktree: "/tmp/project-feature",
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Feature","headRefName":"feature/foo","baseRefName":"master"}]`}
	githubClient, githubClientError := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientError)
	chatClient := &strictSyncChatClient{response: "fix: adopt sibling worktree"}
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
		taskOptionWorktreeCommitMessage: worktreeAdoptionCommitMessageOptions{
			Client: chatClient,
		},
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	recordedCommands := recordedGitCommands(gitExecutor.commands)
	require.Contains(t, recordedCommands, "worktree list --porcelain")
	require.Contains(t, recordedCommands, "status --porcelain --branch")
	require.Contains(t, recordedCommands, "add --all")
	require.Contains(t, recordedCommands, "commit -m fix: adopt sibling worktree")
	require.Contains(t, recordedCommands, "push --set-upstream origin feature/foo")
	require.Contains(t, recordedCommands, "worktree remove /tmp/project-feature")
	require.Contains(t, recordedCommands, "worktree prune")
	require.Contains(t, recordedCommands, "merge --no-edit origin/master")
	require.Contains(t, recordedCommands, "push origin feature/foo")
	require.Less(t, recordedGitCommandIndex(gitExecutor.commands, "worktree remove /tmp/project-feature"), recordedGitCommandLastIndex(gitExecutor.commands, "switch feature/foo"))
	require.GreaterOrEqual(t, recordedGitCommandCount(gitExecutor.commands, "fetch --prune origin"), 2)
	require.Less(t, recordedGitCommandIndex(gitExecutor.commands, "worktree prune"), recordedGitCommandLastIndex(gitExecutor.commands, "fetch --prune origin"))
	require.Less(t, recordedGitCommandLastIndex(gitExecutor.commands, "fetch --prune origin"), recordedGitCommandLastIndex(gitExecutor.commands, "switch feature/foo"))
	require.Len(t, chatClient.requests, 1)
	require.True(t, gitExecutor.worktreeRemoved)
}

func TestHandleBranchSyncActionStrictPRBranchRequireCleanRejectsDirtyWorktree(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{statusOutput: " M README.md\n"}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Feature","headRefName":"feature/foo","baseRefName":"master"}]`}
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

func TestHandleBranchSyncActionStrictPRBranchPromptsToSyncMasterWhenPullRequestMerged(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{outputs: []string{
		`[]`,
		`[{"number":7,"title":"Merged","headRefName":"feature/foo"}]`,
	}}
	githubClient, githubClientError := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientError)
	output := &strings.Builder{}
	prompter := &strictSyncPrompter{result: shared.ConfirmationResult{Confirmed: true}}
	environment := &workflow.Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: gitManager,
		GitHubClient:      githubClient,
		Logger:            zap.NewNop(),
		Output:            output,
		Errors:            io.Discard,
		Reporter:          &recordingReporter{},
		Prompter:          prompter,
		PromptState:       prompt.NewSessionState(false),
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
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	recordedCommands := recordedGitCommands(gitExecutor.commands)
	require.Contains(t, recordedCommands, "switch master")
	require.Contains(t, recordedCommands, "reset --hard origin/master")
	require.NotContains(t, recordedCommands, "merge --no-edit origin/master")
	require.NotContains(t, recordedCommands, "push origin feature/foo")
	require.Equal(t, "SYNCED: /tmp/project (master)\n", output.String())
	require.Equal(t, []string{`Pull request for branch "feature/foo" into master is already merged. Sync master instead? [a/N/y] `}, prompter.prompts)
	require.Len(t, githubExecutor.commands, 2)
	require.Contains(t, strings.Join(githubExecutor.commands[0].Arguments, " "), "--state open")
	require.Contains(t, strings.Join(githubExecutor.commands[1].Arguments, " "), "--state merged")
}

func TestHandleBranchSyncActionStrictPRBranchPromptsToSyncMasterWhenMergedPullRequestRemotePruned(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		missingReferences: map[string]bool{
			"origin/feature/foo": true,
		},
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Merged","headRefName":"feature/foo"}]`}
	githubClient, githubClientError := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientError)
	output := &strings.Builder{}
	prompter := &strictSyncPrompter{result: shared.ConfirmationResult{Confirmed: true}}
	environment := &workflow.Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: gitManager,
		GitHubClient:      githubClient,
		Logger:            zap.NewNop(),
		Output:            output,
		Errors:            io.Discard,
		Reporter:          &recordingReporter{},
		Prompter:          prompter,
		PromptState:       prompt.NewSessionState(false),
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
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	recordedCommands := recordedGitCommands(gitExecutor.commands)
	require.Contains(t, recordedCommands, "switch master")
	require.Contains(t, recordedCommands, "reset --hard origin/master")
	require.NotContains(t, recordedCommands, "merge --no-edit origin/master")
	require.NotContains(t, recordedCommands, "push origin feature/foo")
	require.Equal(t, "SYNCED: /tmp/project (master)\n", output.String())
	require.Equal(t, []string{`Pull request for branch "feature/foo" into master is already merged. Sync master instead? [a/N/y] `}, prompter.prompts)
	require.Len(t, githubExecutor.commands, 1)
	require.Contains(t, strings.Join(githubExecutor.commands[0].Arguments, " "), "--state merged")
}

func TestHandleBranchSyncActionStrictPRBranchKeepsMissingPullRequestErrorWhenMergedSyncDeclined(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{outputs: []string{
		`[]`,
		`[{"number":7,"title":"Merged","headRefName":"feature/foo"}]`,
	}}
	githubClient, githubClientError := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientError)
	prompter := &strictSyncPrompter{}
	environment := &workflow.Environment{
		GitExecutor:       gitExecutor,
		RepositoryManager: gitManager,
		GitHubClient:      githubClient,
		Logger:            zap.NewNop(),
		Output:            io.Discard,
		Errors:            io.Discard,
		Reporter:          &recordingReporter{},
		Prompter:          prompter,
		PromptState:       prompt.NewSessionState(false),
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
	}

	syncError := handleBranchSyncAction(context.Background(), environment, repository, parameters)
	require.Error(t, syncError)
	require.Contains(t, syncError.Error(), "does not have an open pull request")
	recordedCommands := recordedGitCommands(gitExecutor.commands)
	require.NotContains(t, recordedCommands, "switch master")
	require.NotContains(t, recordedCommands, "reset --hard origin/master")
	require.Len(t, prompter.prompts, 1)
}

func TestHandleBranchSyncActionStrictPRBranchAssumeYesSyncsMasterForMergedPullRequest(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{outputs: []string{
		`[]`,
		`[{"number":7,"title":"Merged","headRefName":"feature/foo"}]`,
	}}
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
		PromptState:       prompt.NewSessionState(true),
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
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "reset --hard origin/master")
}

func TestHandleBranchSyncActionStrictPRBranchCommitFlagUsesDirtySyncCommitWithRequireClean(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{
		statusOutput:  " M README.md\n",
		revListOutput: "1\n",
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Feature","headRefName":"feature/foo","baseRefName":"master"}]`}
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
		taskOptionRequireClean:       true,
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
	pullRequestBody := "## Summary\n- Updates README rendering from the branch diff."
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
	chatClient := &strictSyncChatClient{responses: []string{pullRequestBody}}
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
		taskOptionWorktreeCommitMessage: worktreeAdoptionCommitMessageOptions{
			Client: chatClient,
		},
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "switch -c feature/foo origin/master")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "push -u origin feature/foo")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "diff --stat origin/master...feature/foo")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "diff --unified=3 origin/master...feature/foo")
	require.Less(t, recordedGitCommandIndex(gitExecutor.commands, "diff --stat origin/master...feature/foo"), recordedGitCommandIndex(gitExecutor.commands, "push -u origin feature/foo"))
	require.Len(t, githubExecutor.commands, 1)
	require.Equal(t, []string{"pr", "create", "--repo", "owner/project", "--base", "master", "--head", "feature/foo", "--title", "feature/foo", "--body", pullRequestBody}, githubExecutor.commands[0].Arguments)
	require.Len(t, chatClient.requests, 1)
	require.Contains(t, chatClient.requests[0].Messages[1].Content, "Comparison range: origin/master...feature/foo")
	require.Contains(t, chatClient.requests[0].Messages[1].Content, "README.md | 1 +")
	require.Contains(t, chatClient.requests[0].Messages[1].Content, "diff --git a/README.md b/README.md")
}

func TestHandleBranchSyncActionStrictPRBranchUsesExplicitPullRequestMetadata(t *testing.T) {
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
	chatClient := &strictSyncChatClient{response: "generated body should not be used"}
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
		taskOptionPullRequestTitle:   "docs: explain sync",
		taskOptionPullRequestBody:    "Explain the reviewer-facing reason.",
		taskOptionWorktreeCommitMessage: worktreeAdoptionCommitMessageOptions{
			Client: chatClient,
		},
	}

	require.NoError(t, handleBranchSyncAction(context.Background(), environment, repository, parameters))
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "switch -c feature/foo origin/master")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "push -u origin feature/foo")
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "diff --stat origin/master...feature/foo")
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "diff --unified=3 origin/master...feature/foo")
	require.Len(t, githubExecutor.commands, 1)
	require.Equal(t, []string{"pr", "create", "--repo", "owner/project", "--base", "master", "--head", "feature/foo", "--title", "docs: explain sync", "--body", "Explain the reviewer-facing reason."}, githubExecutor.commands[0].Arguments)
	require.Len(t, chatClient.requests, 0)
}

func TestHandleBranchSyncActionStrictPRBranchCreatesGeneratedBranchFromDirtyMaster(t *testing.T) {
	generatedBranchName := "gix/cancel-upstream-request-on-downstream-timeout"
	pullRequestBody := "## Summary\n- Updates README content from the dirty master branch."
	gitExecutor := &strictSyncGitExecutor{
		statusOutput: " M README.md\n",
		missingReferences: map[string]bool{
			"origin/" + generatedBranchName:     true,
			"refs/heads/" + generatedBranchName: true,
		},
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{}
	githubClient, githubClientError := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientError)
	chatClient := &strictSyncChatClient{responses: []string{"fix: cancel upstream request on downstream timeout", "docs: update readme", pullRequestBody}}
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
	require.NotEqual(t, -1, recordedGitCommandIndex(gitExecutor.commands, "switch -c "+generatedBranchName))
	require.Equal(t, -1, recordedGitCommandIndex(gitExecutor.commands, "switch -c "+generatedBranchName+" origin/master"))
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "add --all -- README.md")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "commit -m docs: update readme")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "merge --no-edit origin/master")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "push -u origin "+generatedBranchName)
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "diff --stat origin/master..."+generatedBranchName)
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "diff --unified=3 origin/master..."+generatedBranchName)
	require.Less(t, recordedGitCommandIndex(gitExecutor.commands, "diff --stat origin/master..."+generatedBranchName), recordedGitCommandIndex(gitExecutor.commands, "push -u origin "+generatedBranchName))
	require.Len(t, githubExecutor.commands, 1)
	require.Equal(t, []string{"pr", "create", "--repo", "owner/project", "--base", "master", "--head", generatedBranchName, "--title", generatedBranchName, "--body", pullRequestBody}, githubExecutor.commands[0].Arguments)
	require.Len(t, chatClient.requests, 3)
	require.Contains(t, chatClient.requests[0].Messages[1].Content, "Diff source: ALL")
	require.Contains(t, chatClient.requests[2].Messages[1].Content, "Comparison range: origin/master..."+generatedBranchName)
	require.Contains(t, chatClient.requests[2].Messages[1].Content, "README.md | 1 +")
	require.Contains(t, chatClient.requests[2].Messages[1].Content, "diff --git a/README.md b/README.md")
}

func TestHandleBranchSyncActionStrictPRBranchSkipsStaleGeneratedRemoteBranch(t *testing.T) {
	generatedBranchName := "gix/cancel-upstream-request-on-downstream-timeout"
	collisionBranchName := generatedBranchName + "-2"
	pullRequestBody := "## Summary\n- Updates README content without reusing the stale branch."
	gitExecutor := &strictSyncGitExecutor{
		statusOutput: " M README.md\n",
		missingReferences: map[string]bool{
			"origin/" + collisionBranchName:     true,
			"refs/heads/" + collisionBranchName: true,
		},
	}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[]`}
	githubClient, githubClientError := githubcli.NewClient(githubExecutor)
	require.NoError(t, githubClientError)
	chatClient := &strictSyncChatClient{responses: []string{"fix: cancel upstream request on downstream timeout", "docs: update readme", pullRequestBody}}
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
	require.Equal(t, -1, recordedGitCommandIndex(gitExecutor.commands, "switch -c "+generatedBranchName+" origin/master"))
	require.NotEqual(t, -1, recordedGitCommandIndex(gitExecutor.commands, "switch -c "+collisionBranchName))
	require.Equal(t, -1, recordedGitCommandIndex(gitExecutor.commands, "switch -c "+collisionBranchName+" origin/master"))
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "push -u origin "+collisionBranchName)
	require.Len(t, githubExecutor.commands, 2)
	require.Equal(t, []string{"pr", "create", "--repo", "owner/project", "--base", "master", "--head", collisionBranchName, "--title", collisionBranchName, "--body", pullRequestBody}, githubExecutor.commands[1].Arguments)
	require.Len(t, chatClient.requests, 3)
	require.Contains(t, chatClient.requests[0].Messages[1].Content, "Diff source: ALL")
	require.Contains(t, chatClient.requests[2].Messages[1].Content, "Comparison range: origin/master..."+collisionBranchName)
}

func TestGeneratedSyncBranchNameLimitsSemanticSlug(t *testing.T) {
	longSemanticSubject := "fix: cancel upstream request on downstream timeout while preserving router context propagation"
	gitExecutor := &strictSyncGitExecutor{statusOutput: " M README.md\n"}
	chatClient := &strictSyncChatClient{response: longSemanticSubject}

	branchName, branchNameErr := generatedSyncBranchName(context.Background(), gitExecutor, "/tmp/project", worktreeAdoptionCommitMessageOptions{Client: chatClient})

	require.NoError(t, branchNameErr)
	require.Equal(t, "gix/cancel-upstream-request-on-downstream-timeout-while", branchName)
	_, semanticSlug, found := strings.Cut(branchName, "/")
	require.True(t, found)
	require.LessOrEqual(t, len(semanticSlug), strictSyncGeneratedSemanticSlugLimit)
	collisionBranchName := generatedSyncBranchCandidateName(branchName, 1)
	require.Equal(t, "gix/cancel-upstream-request-on-downstream-timeout-while-2", collisionBranchName)
	_, collisionSemanticSlug, collisionFound := strings.Cut(collisionBranchName, "/")
	require.True(t, collisionFound)
	require.LessOrEqual(t, len(collisionSemanticSlug), strictSyncGeneratedSemanticSlugLimit)
}

func TestHandleBranchSyncActionStrictPRBranchDoesNotPushWhenPullRequestBodyGenerationFails(t *testing.T) {
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
	chatClient := &strictSyncChatClient{responses: []string{""}}
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
		taskOptionWorktreeCommitMessage: worktreeAdoptionCommitMessageOptions{
			Client: chatClient,
		},
	}

	syncError := handleBranchSyncAction(context.Background(), environment, repository, parameters)
	require.Error(t, syncError)
	require.Contains(t, syncError.Error(), "empty_response")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "switch -c feature/foo origin/master")
	require.Contains(t, recordedGitCommands(gitExecutor.commands), "diff --stat origin/master...feature/foo")
	require.NotContains(t, recordedGitCommands(gitExecutor.commands), "push -u origin feature/foo")
	require.Len(t, githubExecutor.commands, 0)
	require.Len(t, chatClient.requests, 1)
}

func TestHandleBranchSyncActionStrictPRBranchStopsBeforePushOnMergeConflict(t *testing.T) {
	gitExecutor := &strictSyncGitExecutor{mergeError: commandFailedError("CONFLICT (content): Merge conflict in README.md")}
	gitManager, managerError := gitrepo.NewRepositoryManager(gitExecutor)
	require.NoError(t, managerError)
	githubExecutor := &strictSyncGitHubExecutor{output: `[{"number":7,"title":"Feature","headRefName":"feature/foo","baseRefName":"master"}]`}
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

func recordedGitCommandIndex(commands []execshell.CommandDetails, target string) int {
	for commandIndex := range commands {
		if strings.Join(commands[commandIndex].Arguments, " ") == target {
			return commandIndex
		}
	}
	return -1
}

func recordedGitCommandLastIndex(commands []execshell.CommandDetails, target string) int {
	for commandIndex := len(commands) - 1; commandIndex >= 0; commandIndex-- {
		if strings.Join(commands[commandIndex].Arguments, " ") == target {
			return commandIndex
		}
	}
	return -1
}

func recordedGitCommandCount(commands []execshell.CommandDetails, target string) int {
	count := 0
	for commandIndex := range commands {
		if strings.Join(commands[commandIndex].Arguments, " ") == target {
			count++
		}
	}
	return count
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
