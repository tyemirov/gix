package workflow

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/audit"
	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/githubauth"
	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/gitrepo"
	migrate "github.com/temirov/gix/internal/migrate"
)

type scriptedExecutor struct {
	gitHandlers          map[string]func(execshell.CommandDetails) (execshell.ExecutionResult, error)
	githubHandlers       map[string]func(execshell.CommandDetails) (execshell.ExecutionResult, error)
	gitCommands          []execshell.CommandDetails
	githubCommands       []execshell.CommandDetails
	defaultBranchFailure *execshell.CommandFailedError
}

func newScriptedExecutor() *scriptedExecutor {
	return &scriptedExecutor{
		gitHandlers:    map[string]func(execshell.CommandDetails) (execshell.ExecutionResult, error){},
		githubHandlers: map[string]func(execshell.CommandDetails) (execshell.ExecutionResult, error){},
	}
}

func (executor *scriptedExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.gitCommands = append(executor.gitCommands, details)
	key := strings.Join(details.Arguments, " ")
	if handler, exists := executor.gitHandlers[key]; exists {
		return handler(details)
	}
	return execshell.ExecutionResult{}, nil
}

func (executor *scriptedExecutor) ExecuteGitHubCLI(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.githubCommands = append(executor.githubCommands, details)
	key := strings.Join(details.Arguments, " ")
	if handler, exists := executor.githubHandlers[key]; exists {
		return handler(details)
	}
	if executor.defaultBranchFailure != nil {
		for _, argument := range details.Arguments {
			if strings.Contains(argument, "default_branch=") {
				failure := *executor.defaultBranchFailure
				failure.Command = execshell.ShellCommand{Name: execshell.CommandGitHub, Details: details}
				return execshell.ExecutionResult{}, failure
			}
		}
	}
	if len(details.Arguments) > 0 {
		switch details.Arguments[0] {
		case "pr":
			return execshell.ExecutionResult{StandardOutput: "[]\n"}, nil
		case "repo":
			if len(details.Arguments) >= 3 && details.Arguments[1] == "view" {
				return execshell.ExecutionResult{StandardOutput: `{"defaultBranchRef":{"name":"main"},"nameWithOwner":"owner/example","description":"","isInOrganization":false}`}, nil
			}
		case "api":
			if len(details.Arguments) > 1 && details.Arguments[1] == "graphql" {
				return execshell.ExecutionResult{StandardOutput: `{"data":{"search":{"nodes":[{"name":"loopaware","nameWithOwner":"tyemirov/loopaware","defaultBranchRef":{"name":"master"}}]}}}`}, nil
			}
			joined := strings.Join(details.Arguments, " ")
			if strings.Contains(joined, "/pages") && strings.Contains(joined, "GET") {
				return execshell.ExecutionResult{StandardOutput: `{"build_type":"legacy","source":{"branch":"main","path":"/docs"}}`}, nil
			}
			if strings.Contains(joined, "/branches/") && strings.Contains(joined, "/protection") {
				failure := execshell.CommandFailedError{
					Command: execshell.ShellCommand{Name: execshell.CommandGitHub, Details: details},
					Result:  execshell.ExecutionResult{ExitCode: 1, StandardError: "404"},
				}
				return execshell.ExecutionResult{}, failure
			}
		}
	}
	return execshell.ExecutionResult{}, nil
}

func branchMissing(details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, execshell.CommandFailedError{
		Command: execshell.ShellCommand{Name: execshell.CommandGit, Details: details},
		Result:  execshell.ExecutionResult{ExitCode: 1, StandardError: "not found"},
	}
}

func TestBranchMigrationOperationRequiresSingleTarget(testInstance *testing.T) {
	executor := newScriptedExecutor()
	executor.gitHandlers["remote"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: "origin\n"}, nil
	}

	repositoryManager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(testInstance, managerError)

	githubClient, clientError := githubcli.NewClient(executor)
	require.NoError(testInstance, clientError)

	operation := &BranchMigrationOperation{Targets: []BranchMigrationTarget{
		{RemoteName: "origin", SourceBranch: "main", TargetBranch: "master", PushToRemote: true},
		{RemoteName: "upstream", SourceBranch: "develop", TargetBranch: "main"},
	}}

	environment := &Environment{RepositoryManager: repositoryManager, GitExecutor: executor, GitHubClient: githubClient}

	executionError := operation.Execute(context.Background(), environment, &State{})

	require.Error(testInstance, executionError)
	require.EqualError(testInstance, executionError, migrationMultipleTargetsUnsupportedMessageConstant)
}

func TestBranchMigrationOperationReturnsActionableDefaultBranchError(testInstance *testing.T) {
	testInstance.Setenv(githubauth.EnvGitHubCLIToken, "test-token")
	testInstance.Setenv(githubauth.EnvGitHubToken, "test-token")
	executor := newScriptedExecutor()
	executor.gitHandlers["remote"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: "origin\n"}, nil
	}
	executor.defaultBranchFailure = &execshell.CommandFailedError{
		Result: execshell.ExecutionResult{
			ExitCode:      1,
			StandardError: "GraphQL: branch not found",
		},
	}
	executor.githubHandlers["repo view owner/example --json defaultBranchRef,nameWithOwner,description,isInOrganization"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: `{"defaultBranchRef":{"name":"main"},"nameWithOwner":"owner/example","description":"","isInOrganization":false}`}, nil
	}
	executor.githubHandlers["api repos/owner/example -X PATCH -f default_branch=master -H Accept: application/vnd.github+json"] = func(details execshell.CommandDetails) (execshell.ExecutionResult, error) {
		failure := execshell.CommandFailedError{
			Command: execshell.ShellCommand{Name: execshell.CommandGitHub, Details: details},
			Result:  execshell.ExecutionResult{ExitCode: 1, StandardError: "GraphQL: branch not found"},
		}
		return execshell.ExecutionResult{}, failure
	}

	repositoryManager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(testInstance, managerError)

	githubClient, clientError := githubcli.NewClient(executor)
	require.NoError(testInstance, clientError)

	operation := &BranchMigrationOperation{Targets: []BranchMigrationTarget{
		{RemoteName: "origin", SourceBranch: "main", TargetBranch: "master", PushToRemote: true},
	}}

	repositoryPath := testInstance.TempDir()

	state := &State{
		Repositories: []*RepositoryState{
			{
				Path: repositoryPath,
				Inspection: audit.RepositoryInspection{
					CanonicalOwnerRepo:  "owner/example",
					FinalOwnerRepo:      "owner/example",
					LocalBranch:         "main",
					RemoteDefaultBranch: "main",
				},
			},
		},
	}

	environment := &Environment{
		RepositoryManager: repositoryManager,
		GitExecutor:       executor,
		GitHubClient:      githubClient,
	}

	executionError := operation.Execute(context.Background(), environment, state)

	require.False(testInstance, len(executor.githubCommands) == 0, "expected GitHub commands to be executed")
	require.Error(testInstance, executionError)

	var updateError migrate.DefaultBranchUpdateError
	require.ErrorAs(testInstance, executionError, &updateError)

	errorMessage := executionError.Error()
	require.Contains(testInstance, errorMessage, repositoryPath)
	require.Contains(testInstance, errorMessage, "owner/example")
	require.Contains(testInstance, errorMessage, "source=main")
	require.Contains(testInstance, errorMessage, "target=master")
	require.Contains(testInstance, errorMessage, "GraphQL: branch not found")
	require.NotContains(testInstance, errorMessage, "default branch update failed")
}

func TestBranchMigrationOperationCreatesLocalBranchWithoutRemote(testInstance *testing.T) {
	executor := newScriptedExecutor()
	executor.gitHandlers["remote"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: ""}, nil
	}
	executor.gitHandlers["show-ref --verify --quiet refs/heads/master"] = branchMissing

	repositoryManager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(testInstance, managerError)

	githubClient, clientError := githubcli.NewClient(executor)
	require.NoError(testInstance, clientError)

	operation := &BranchMigrationOperation{Targets: []BranchMigrationTarget{
		{RemoteName: "origin", TargetBranch: "master"},
	}}

	outputBuffer := &bytes.Buffer{}

	state := &State{
		Repositories: []*RepositoryState{
			{
				Path: "/repository",
				Inspection: audit.RepositoryInspection{
					LocalBranch: "main",
				},
			},
		},
	}

	environment := &Environment{
		RepositoryManager: repositoryManager,
		GitExecutor:       executor,
		GitHubClient:      githubClient,
		Output:            outputBuffer,
	}

	executionError := operation.Execute(context.Background(), environment, state)

	require.NoError(testInstance, executionError)

	foundBranchCreation := false
	foundCheckout := false
	for _, commandDetails := range executor.gitCommands {
		joined := strings.Join(commandDetails.Arguments, " ")
		if joined == "branch master main" {
			foundBranchCreation = true
		}
		if joined == "checkout master" {
			foundCheckout = true
		}
		if strings.HasPrefix(joined, "push ") {
			testInstance.Fatalf("unexpected push command recorded: %s", joined)
		}
	}
	require.True(testInstance, foundBranchCreation)
	require.True(testInstance, foundCheckout)
	require.Contains(testInstance, outputBuffer.String(), "WORKFLOW-DEFAULT: /repository (main → master) safe_to_delete=false")
}

func TestBranchMigrationOperationCreatesRemoteBranchWhenMissing(testInstance *testing.T) {
	testInstance.Setenv(githubauth.EnvGitHubToken, "test-token")
	testInstance.Setenv(githubauth.EnvGitHubCLIToken, "test-token")
	executor := newScriptedExecutor()
	executor.gitHandlers["remote"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: "origin\n"}, nil
	}
	executor.gitHandlers["show-ref --verify --quiet refs/heads/master"] = branchMissing
	executor.gitHandlers["ls-remote --heads origin master"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: ""}, nil
	}

	repositoryManager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(testInstance, managerError)

	githubClient, clientError := githubcli.NewClient(executor)
	require.NoError(testInstance, clientError)

	operation := &BranchMigrationOperation{Targets: []BranchMigrationTarget{
		{RemoteName: "origin", SourceBranch: "main", TargetBranch: "master"},
	}}

	outputBuffer := &bytes.Buffer{}

	state := &State{
		Repositories: []*RepositoryState{
			{
				Path: "/repository",
				Inspection: audit.RepositoryInspection{
					CanonicalOwnerRepo:  "owner/example",
					LocalBranch:         "main",
					RemoteDefaultBranch: "main",
				},
			},
		},
	}

	environment := &Environment{
		RepositoryManager: repositoryManager,
		GitExecutor:       executor,
		GitHubClient:      githubClient,
		Output:            outputBuffer,
	}

	executionError := operation.Execute(context.Background(), environment, state)

	require.NoError(testInstance, executionError)

	foundBranchCreation := false
	for _, commandDetails := range executor.gitCommands {
		joined := strings.Join(commandDetails.Arguments, " ")
		if joined == "branch master main" {
			foundBranchCreation = true
		}
	}
	require.True(testInstance, foundBranchCreation)
	require.Contains(testInstance, outputBuffer.String(), "WORKFLOW-DEFAULT: /repository (main → master)")
}

func TestEnsureRemoteBranchPushesWhenMissing(testInstance *testing.T) {
	executor := newScriptedExecutor()
	executor.gitHandlers["ls-remote --heads origin master"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: ""}, nil
	}
	executor.gitHandlers["push origin master:master"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{}, nil
	}

	executionError := ensureRemoteBranch(context.Background(), executor, "/repository", "origin", "master")

	require.NoError(testInstance, executionError)

	recordedPush := false
	for _, commandDetails := range executor.gitCommands {
		if strings.Join(commandDetails.Arguments, " ") == "push origin master:master" {
			recordedPush = true
			break
		}
	}
	require.True(testInstance, recordedPush)
}

func TestBranchMigrationOperationInfersIdentifierFromRemote(testInstance *testing.T) {
	testInstance.Setenv(githubauth.EnvGitHubToken, "test-token")
	testInstance.Setenv(githubauth.EnvGitHubCLIToken, "test-token")
	executor := newScriptedExecutor()
	executor.gitHandlers["remote"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: "origin\n"}, nil
	}
	executor.gitHandlers["remote get-url origin"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: "git@github.com:temirov/loopaware.git\n"}, nil
	}
	executor.gitHandlers["show-ref --verify --quiet refs/heads/master"] = branchMissing
	executor.gitHandlers["ls-remote --heads origin master"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: ""}, nil
	}
	executor.githubHandlers["repo view temirov/loopaware --json defaultBranchRef,nameWithOwner,description,isInOrganization"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: `{"defaultBranchRef":{"name":"master"},"nameWithOwner":"tyemirov/loopaware","description":"","isInOrganization":false}`}, nil
	}
	executor.githubHandlers["repo view tyemirov/loopaware --json defaultBranchRef,nameWithOwner,description,isInOrganization"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: `{"defaultBranchRef":{"name":"master"},"nameWithOwner":"tyemirov/loopaware","description":"","isInOrganization":false}`}, nil
	}

	repositoryManager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(testInstance, managerError)

	githubClient, clientError := githubcli.NewClient(executor)
	require.NoError(testInstance, clientError)

	operation := &BranchMigrationOperation{Targets: []BranchMigrationTarget{
		{RemoteName: "origin", SourceBranch: "main", TargetBranch: "master", PushToRemote: true},
	}}

	outputBuffer := &bytes.Buffer{}

	state := &State{
		Repositories: []*RepositoryState{
			{
				Path: "/repository",
				Inspection: audit.RepositoryInspection{
					RemoteDefaultBranch: "main",
					LocalBranch:         "main",
					FinalOwnerRepo:      "temirov/loopaware",
				},
			},
		},
	}

	environment := &Environment{
		RepositoryManager: repositoryManager,
		GitExecutor:       executor,
		GitHubClient:      githubClient,
		Output:            outputBuffer,
	}

	executionError := operation.Execute(context.Background(), environment, state)

	require.NoError(testInstance, executionError)
	require.NotEmpty(testInstance, executor.githubCommands)

	foundDefaultBranchUpdate := false
	foundCanonicalIdentifier := false
	for _, commandDetails := range executor.githubCommands {
		joined := strings.Join(commandDetails.Arguments, " ")
		if strings.Contains(joined, "default_branch=") {
			foundDefaultBranchUpdate = true
			if strings.Contains(joined, "repos/tyemirov/loopaware") {
				foundCanonicalIdentifier = true
			}
		}
	}
	require.True(testInstance, foundDefaultBranchUpdate)
	require.True(testInstance, foundCanonicalIdentifier)
	require.Contains(testInstance, outputBuffer.String(), "WORKFLOW-DEFAULT: /repository (main → master)")
}

func TestBranchMigrationOperationSkipsRemotePushWhenUnavailable(testInstance *testing.T) {
	testInstance.Setenv(githubauth.EnvGitHubToken, "test-token")
	testInstance.Setenv(githubauth.EnvGitHubCLIToken, "test-token")
	executor := newScriptedExecutor()
	executor.gitHandlers["remote"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: "origin\n"}, nil
	}
	executor.gitHandlers["remote get-url origin"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: "git@github.com:owner/example.git\n"}, nil
	}
	executor.gitHandlers["show-ref --verify --quiet refs/heads/master"] = branchMissing
	executor.gitHandlers["ls-remote --heads origin master"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: ""}, nil
	}
	executor.gitHandlers["push origin master:master"] = func(details execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{}, execshell.CommandFailedError{
			Command: execshell.ShellCommand{Name: execshell.CommandGit, Details: details},
			Result:  execshell.ExecutionResult{ExitCode: 128, StandardError: "fatal: could not read from remote repository"},
		}
	}
	executor.githubHandlers["repo view owner/example --json defaultBranchRef,nameWithOwner,description,isInOrganization"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: `{"defaultBranchRef":{"name":"main"},"nameWithOwner":"owner/example","description":"","isInOrganization":false}`}, nil
	}

	repositoryManager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(testInstance, managerError)

	githubClient, clientError := githubcli.NewClient(executor)
	require.NoError(testInstance, clientError)

	operation := &BranchMigrationOperation{Targets: []BranchMigrationTarget{
		{RemoteName: "origin", SourceBranch: "main", TargetBranch: "master", PushToRemote: true},
	}}

	outputBuffer := &bytes.Buffer{}

	state := &State{
		Repositories: []*RepositoryState{
			{
				Path: "/repository",
				Inspection: audit.RepositoryInspection{
					FinalOwnerRepo:      "owner/example",
					LocalBranch:         "main",
					RemoteDefaultBranch: "main",
				},
			},
		},
	}

	environment := &Environment{
		RepositoryManager: repositoryManager,
		GitExecutor:       executor,
		GitHubClient:      githubClient,
		Output:            outputBuffer,
	}

	executionError := operation.Execute(context.Background(), environment, state)

	require.NoError(testInstance, executionError)
	require.Contains(testInstance, outputBuffer.String(), "WORKFLOW-DEFAULT: /repository (main → master)")

	foundPushAttempt := false
	for _, commandDetails := range executor.gitCommands {
		if strings.Join(commandDetails.Arguments, " ") == "push origin master:master" {
			foundPushAttempt = true
		}
	}
	require.True(testInstance, foundPushAttempt)
}

func TestBranchMigrationOperationFallsBackWhenIdentifierMissing(testInstance *testing.T) {
	executor := newScriptedExecutor()
	executor.gitHandlers["remote"] = func(execshell.CommandDetails) (execshell.ExecutionResult, error) {
		return execshell.ExecutionResult{StandardOutput: "origin\n"}, nil
	}
	executor.gitHandlers["show-ref --verify --quiet refs/heads/master"] = branchMissing

	repositoryManager, managerError := gitrepo.NewRepositoryManager(executor)
	require.NoError(testInstance, managerError)

	githubClient, clientError := githubcli.NewClient(executor)
	require.NoError(testInstance, clientError)

	operation := &BranchMigrationOperation{Targets: []BranchMigrationTarget{
		{RemoteName: "origin", TargetBranch: "master", PushToRemote: true},
	}}

	outputBuffer := &bytes.Buffer{}

	state := &State{
		Repositories: []*RepositoryState{
			{
				Path: "/repository",
				Inspection: audit.RepositoryInspection{
					LocalBranch:         "main",
					RemoteDefaultBranch: "main",
				},
			},
		},
	}

	environment := &Environment{
		RepositoryManager: repositoryManager,
		GitExecutor:       executor,
		GitHubClient:      githubClient,
		Output:            outputBuffer,
	}

	executionError := operation.Execute(context.Background(), environment, state)

	require.NoError(testInstance, executionError)
	require.Empty(testInstance, executor.githubCommands)

	foundBranchCreation := false
	for _, commandDetails := range executor.gitCommands {
		if strings.Join(commandDetails.Arguments, " ") == "branch master main" {
			foundBranchCreation = true
		}
		if strings.HasPrefix(strings.Join(commandDetails.Arguments, " "), "push ") {
			testInstance.Fatalf("unexpected push command recorded: %s", strings.Join(commandDetails.Arguments, " "))
		}
	}
	require.True(testInstance, foundBranchCreation)
	require.Contains(testInstance, outputBuffer.String(), "WORKFLOW-DEFAULT: /repository (main → master) safe_to_delete=false")
}
