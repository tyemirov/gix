package commit

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/commitmsg"
	"github.com/temirov/gix/internal/execshell"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/llm"
)

func TestMessageCommandGeneratesCommitMessage(t *testing.T) {
	tempDir := t.TempDir()
	apiKeyEnv := "TEST_LLM_KEY"
	t.Setenv(apiKeyEnv, "test-api-key")

	executor := &fakeGitExecutor{
		responses: map[string]string{
			"status --short":                   " M file.go\n",
			"diff --unified=3 --cached --stat": " file.go | 2 +-",
			"diff --unified=3 --cached":        "diff --git a/file.go b/file.go\n@@\n-old\n+new\n",
		},
	}
	client := &fakeChatClient{response: "feat: update file"}

	runner := &recordingTaskRunner{}

	builder := MessageCommandBuilder{
		GitExecutor: executor,
		ConfigurationProvider: func() MessageConfiguration {
			return MessageConfiguration{
				Roots:      []string{tempDir},
				APIKeyEnv:  apiKeyEnv,
				Model:      "mock-model",
				DiffSource: "staged",
			}.Sanitize()
		},
		ClientFactory: func(config llm.Config) (commitmsg.ChatClient, error) {
			client.config = config
			return client, nil
		},
		Discoverer: mockDiscoverer{roots: []string{tempDir}},
		TaskRunnerFactory: func(deps workflow.Dependencies) TaskRunnerExecutor {
			runner.dependencies = deps
			return runner
		},
	}

	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Name: flagutils.DefaultRootFlagName, Usage: flagutils.DefaultRootFlagUsage, Enabled: true})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetContext(context.Background())

	err = command.Execute()
	require.NoError(t, err)
	require.NotNil(t, runner)
	require.Equal(t, []string{tempDir}, runner.roots)
	require.Len(t, runner.definitions, 1)
	require.Len(t, runner.definitions[0].Actions, 1)
	action := runner.definitions[0].Actions[0]
	require.Equal(t, taskTypeCommitMessage, action.Type)
	require.Equal(t, "staged", action.Options[taskOptionCommitDiffSource])
	require.Equal(t, 0, action.Options[taskOptionCommitMaxTokens])
	require.NotNil(t, action.Options[taskOptionCommitClient])
	require.Equal(t, "mock-model", client.config.Model)
	require.Equal(t, "test-api-key", client.config.APIKey)
	require.Nil(t, client.request)
}

func TestMessageCommandValidatesDiffSource(t *testing.T) {
	tempDir := t.TempDir()
	apiKeyEnv := "TEST_LLM_KEY"
	t.Setenv(apiKeyEnv, "token")

	builder := MessageCommandBuilder{
		GitExecutor: &fakeGitExecutor{},
		ConfigurationProvider: func() MessageConfiguration {
			return MessageConfiguration{
				Roots:      []string{tempDir},
				APIKeyEnv:  apiKeyEnv,
				Model:      "model",
				DiffSource: "invalid",
			}.Sanitize()
		},
		ClientFactory: func(config llm.Config) (commitmsg.ChatClient, error) {
			return &fakeChatClient{}, nil
		},
	}

	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Name: flagutils.DefaultRootFlagName, Usage: flagutils.DefaultRootFlagUsage, Enabled: true})
	command.SetContext(context.Background())
	command.SetArgs([]string{"--diff-source", "invalid"})
	err = command.Execute()
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "unsupported diff source"))
}

type fakeGitExecutor struct {
	responses map[string]string
	calls     [][]string
}

func (executor *fakeGitExecutor) ExecuteGit(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	key := strings.Join(details.Arguments, " ")
	executor.calls = append(executor.calls, details.Arguments)
	if executor.responses == nil {
		return execshell.ExecutionResult{}, nil
	}
	value, ok := executor.responses[key]
	if !ok {
		return execshell.ExecutionResult{}, nil
	}
	return execshell.ExecutionResult{StandardOutput: value}, nil
}

func (executor *fakeGitExecutor) ExecuteGitHubCLI(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type fakeChatClient struct {
	config   llm.Config
	response string
	err      error
	request  *llm.ChatRequest
}

func (client *fakeChatClient) Chat(ctx context.Context, request llm.ChatRequest) (string, error) {
	clientCopy := request
	client.request = &clientCopy
	if client.err != nil {
		return "", client.err
	}
	return client.response, nil
}

type recordingTaskRunner struct {
	dependencies   workflow.Dependencies
	roots          []string
	definitions    []workflow.TaskDefinition
	runtimeOptions workflow.RuntimeOptions
}

func (runner *recordingTaskRunner) Run(ctx context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) error {
	runner.roots = append([]string{}, roots...)
	runner.definitions = append([]workflow.TaskDefinition{}, definitions...)
	runner.runtimeOptions = options
	return nil
}

type mockDiscoverer struct {
	roots []string
}

func (discoverer mockDiscoverer) DiscoverRepositories([]string) ([]string, error) {
	return append([]string{}, discoverer.roots...), nil
}
