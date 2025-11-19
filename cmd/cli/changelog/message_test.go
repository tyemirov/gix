package changelog

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/gitrepo"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/pkg/llm"
)

func TestMessageCommandValidatesSinceInputs(t *testing.T) {
	tempDir := t.TempDir()
	apiKeyEnv := "TEST_LLM_KEY"
	t.Setenv(apiKeyEnv, "token")

	builder := MessageCommandBuilder{
		GitExecutor: &fakeGitExecutor{},
		ConfigurationProvider: func() MessageConfiguration {
			return MessageConfiguration{
				Roots:     []string{tempDir},
				APIKeyEnv: apiKeyEnv,
				Model:     "mock-model",
			}.Sanitize()
		},
		ClientFactory: func(config llm.Config) (llm.ChatClient, error) {
			return &fakeChatClient{}, nil
		},
	}

	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Name: flagutils.DefaultRootFlagName, Usage: flagutils.DefaultRootFlagUsage, Enabled: true})
	command.SetContext(context.Background())

	command.SetArgs([]string{"--since-tag", "v0.1.0", "--since-date", "2025-10-07"})
	err = command.Execute()
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "only one of --since-tag or --since-date"))
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
	calls    int
}

func (client *fakeChatClient) Chat(ctx context.Context, request llm.ChatRequest) (string, error) {
	clientCopy := request
	client.request = &clientCopy
	client.calls++
	if client.err != nil {
		return "", client.err
	}
	return client.response, nil
}

type mockDiscoverer struct {
	roots []string
}

func (discoverer mockDiscoverer) DiscoverRepositories([]string) ([]string, error) {
	return append([]string{}, discoverer.roots...), nil
}

func TestMessageCommandOutputsChangelogOnce(t *testing.T) {
	tempDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tempDir, ".git"), 0o755))

	apiKeyEnv := "TEST_LLM_MESSAGE_KEY"
	t.Setenv(apiKeyEnv, "mock-api-key")

	executor := &fakeGitExecutor{
		responses: map[string]string{
			"rev-parse --is-inside-work-tree": "true\n",
			"remote get-url origin":           "",
			"rev-parse --abbrev-ref HEAD":     "main\n",
			"describe --tags --abbrev=0":      "v0.9.0\n",
			"log --no-merges --date=short --pretty=format:%h %ad %an %s --max-count=200 v0.9.0..HEAD": "abc123 2025-10-07 Alice Add feature\n",
			"diff --stat v0.9.0..HEAD":      " internal/app.go | 5 ++++-\n",
			"diff --unified=3 v0.9.0..HEAD": "diff --git a/internal/app.go b/internal/app.go\n",
		},
	}

	manager, err := gitrepo.NewRepositoryManager(executor)
	require.NoError(t, err)

	client := &fakeChatClient{response: "## [v1.0.0]\n\n### Features ✨\n- Highlight\n"}

	builder := MessageCommandBuilder{
		GitExecutor: executor,
		GitManager:  manager,
		Discoverer:  mockDiscoverer{roots: []string{tempDir}},
		ConfigurationProvider: func() MessageConfiguration {
			return MessageConfiguration{
				Roots:          []string{tempDir},
				APIKeyEnv:      apiKeyEnv,
				Model:          "mock-model",
				Version:        "v1.0.0",
				SinceReference: "",
			}.Sanitize()
		},
		ClientFactory: func(config llm.Config) (llm.ChatClient, error) {
			client.config = config
			return client, nil
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

	out := output.String()
	require.Equal(t, 1, strings.Count(out, "## [v1.0.0]"), "expected changelog heading once, output: %q", out)
	require.NotContains(t, out, "TASK_PLAN", "workflow logs should be suppressed for changelog command")
	require.NotContains(t, out, "TASK_APPLY", "workflow logs should be suppressed for changelog command")
	require.Equal(t, 1, client.calls, "chat client should be invoked exactly once")
}
