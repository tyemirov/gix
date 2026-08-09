package semver

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/v5/internal/execshell"
	"github.com/tyemirov/gix/v5/internal/llmclient"
	"github.com/tyemirov/utils/llm"
)

func TestCommandReturnsOneClosedDecision(t *testing.T) {
	executor := &commandGitExecutor{responses: map[string]string{
		"log --pretty=format:%s%n%b%x1e v1.2.3..HEAD": "feat: add reports\n\x1e",
		"diff --stat v1.2.3..HEAD":                    " report.go | 4 ++++\n",
		"diff --unified=3 v1.2.3..HEAD":               "diff --git a/report.go b/report.go\n",
		"show HEAD:CHANGELOG.md":                      "# Changelog\n\n## [Unreleased]\n\n- Added reports.\n",
	}}
	client := &commandChatClient{response: `{"bump":"minor","reason":"The release adds a compatible reporting capability."}`}
	builder := CommandBuilder{
		GitExecutor: executor,
		ConfigurationProvider: func() Configuration {
			return Configuration{
				ConnectionProfiles: commandConnectionProfiles(),
				TimeoutSeconds:     30,
			}
		},
		ClientFactory: func(llmclient.Config) (llm.ChatClient, error) {
			return client, nil
		},
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"v1.2.3"})
	command.SetContext(context.Background())

	executionError := command.Execute()

	require.NoError(t, executionError)
	require.JSONEq(t, `{"bump":"minor","reason":"The release adds a compatible reporting capability.","deterministic_floor":"minor"}`, output.String())
	require.Equal(t, 1, client.calls)
}

func TestCommandRequiresOneBoundary(t *testing.T) {
	command, buildError := (CommandBuilder{}).Build()
	require.NoError(t, buildError)
	command.SetArgs(nil)

	executionError := command.Execute()

	require.Error(t, executionError)
	require.Contains(t, executionError.Error(), "accepts 1 arg")
}

type commandGitExecutor struct {
	responses map[string]string
}

func (executor *commandGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{StandardOutput: executor.responses[strings.Join(details.Arguments, " ")]}, nil
}

func (executor *commandGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type commandChatClient struct {
	response string
	calls    int
}

func (client *commandChatClient) Chat(context.Context, llm.ChatRequest) (string, error) {
	client.calls++
	return client.response, nil
}

func commandConnectionProfiles() llmclient.ConnectionProfiles {
	return llmclient.ConnectionProfiles{
		OpenAI: llmclient.OpenAIConnectionProfile{
			Priority:   1,
			BaseURL:    "https://api.openai.com/v1",
			Credential: "test-key",
			Model:      "test-model",
		},
		LLMProxy: llmclient.LLMProxyConnectionProfile{
			Priority: 2,
			BaseURL:  "https://llm-proxy.example",
			Provider: "test-provider",
			Model:    "test-model",
		},
	}
}
