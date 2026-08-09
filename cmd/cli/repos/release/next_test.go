package release

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/v5/internal/execshell"
	"github.com/tyemirov/gix/v5/internal/llmclient"
	"github.com/tyemirov/utils/llm"
)

func TestNextCommandSelectsEstablishedSemVer(t *testing.T) {
	executor := &nextGitExecutor{responses: map[string]string{
		"rev-parse --show-toplevel": "/repo\n",
		"rev-parse HEAD":            "abc123\n",
		"tag --list":                "v1.2.3\nv1.9.0-rc.1\n",
		"log --pretty=format:%s%n%b%x1e v1.2.3..HEAD": "feat: add reports\n\x1e",
		"diff --stat v1.2.3..HEAD":                    " report.go | 4 ++++\n",
		"diff --unified=3 v1.2.3..HEAD":               "diff --git a/report.go b/report.go\n",
		"show HEAD:CHANGELOG.md":                      "# Changelog\n\n## [Unreleased]\n\n- Added reports.\n",
	}}
	client := &nextChatClient{response: `{"bump":"minor","reason":"The release adds reporting."}`}
	builder := nextTestBuilder(executor, client, []byte("schema_version: 1\nscheme: semver\n"))

	output := executeNextCommand(t, builder, "--format", "json")

	require.JSONEq(t, `{
		"contract":"mprlab.version-decision/v1",
		"scheme":"semver",
		"source_commit":"abc123",
		"boundary_tag":"v1.2.3",
		"previous_version":"v1.2.3",
		"next_version":"v1.3.0",
		"bump":"minor",
		"deterministic_floor":"minor",
		"reason":"The release adds reporting.",
		"evidence_sha256":"`+strings.Repeat("0", 64)+`"
	}`, replaceDigest(output))
	require.Equal(t, 1, client.calls)
}

func TestNextCommandStartsSemVerWithoutLLM(t *testing.T) {
	executor := &nextGitExecutor{responses: map[string]string{
		"rev-parse --show-toplevel": "/repo\n",
		"rev-parse HEAD":            "abc123\n",
		"tag --list":                "docs-archive\n",
	}}
	client := &nextChatClient{}
	builder := nextTestBuilder(executor, client, []byte("schema_version: 1\nscheme: semver\n"))

	output := executeNextCommand(t, builder)

	require.Equal(t, "v1.0.0\n", output)
	require.Zero(t, client.calls)
}

func TestNextCommandExcludesRetiredTags(t *testing.T) {
	executor := &nextGitExecutor{responses: map[string]string{
		"rev-parse --show-toplevel": "/repo\n",
		"rev-parse HEAD":            "abc123\n",
		"tag --list":                "v1.2.3\nv2.0.0\n",
		"log --pretty=format:%s%n%b%x1e v1.2.3..HEAD": "fix: preserve behavior\n\x1e",
		"diff --stat v1.2.3..HEAD":                    " report.go | 1 +\n",
		"diff --unified=3 v1.2.3..HEAD":               "diff --git a/report.go b/report.go\n",
		"show HEAD:CHANGELOG.md":                      "# Changelog\n\n## [Unreleased]\n\n- Fixed reports.\n",
	}}
	client := &nextChatClient{response: `{"bump":"patch","reason":"The release fixes reporting."}`}
	builder := nextTestBuilder(executor, client, []byte("schema_version: 1\nscheme: semver\n"))

	output := executeNextCommand(t, builder, "--format", "json", "--exclude-tag", "v2.0.0")

	require.Contains(t, output, `"boundary_tag":"v1.2.3"`)
	require.Contains(t, output, `"next_version":"v1.2.4"`)
}

func TestNextCommandSelectsCalVerWithoutLLM(t *testing.T) {
	executor := &nextGitExecutor{responses: map[string]string{
		"rev-parse --show-toplevel": "/repo\n",
		"rev-parse HEAD":            "abc123\n",
		"tag --list":                "26.808.235959\n",
	}}
	client := &nextChatClient{}
	builder := nextTestBuilder(executor, client, []byte("schema_version: 1\nscheme: calver\n"))

	output := executeNextCommand(t, builder, "--format", "json", "--release-timestamp", "2026-08-09T00:00:00Z")

	require.JSONEq(t, `{
		"contract":"mprlab.version-decision/v1",
		"scheme":"calver",
		"source_commit":"abc123",
		"boundary_tag":"26.808.235959",
		"previous_version":"26.808.235959",
		"next_version":"26.809.0",
		"reason":"The release version is the canonical UTC CalVer for the supplied release timestamp.",
		"release_timestamp":"2026-08-09T00:00:00Z"
	}`, output)
	require.Zero(t, client.calls)
}

func nextTestBuilder(executor *nextGitExecutor, client *nextChatClient, configuration []byte) NextCommandBuilder {
	return NextCommandBuilder{
		GitExecutor: executor,
		WorkingDirectoryProvider: func() (string, error) {
			return "/repo", nil
		},
		ReadFile: func(string) ([]byte, error) {
			return configuration, nil
		},
		ConfigurationProvider: func() NextConfiguration {
			return NextConfiguration{
				ConnectionProfiles: llmclient.ConnectionProfiles{
					OpenAI:   llmclient.OpenAIConnectionProfile{Priority: 1, BaseURL: "https://api.openai.com/v1", Credential: "key", Model: "model"},
					LLMProxy: llmclient.LLMProxyConnectionProfile{Priority: 2, BaseURL: "https://llm-proxy.example", Credential: "key", Provider: "provider", Model: "model"},
				},
				TimeoutSeconds: 30,
			}
		},
		ClientFactory: func(llmclient.Config) (llm.ChatClient, error) {
			return client, nil
		},
		Now: func() time.Time {
			return time.Date(2026, time.August, 9, 0, 0, 0, 0, time.UTC)
		},
	}
}

func executeNextCommand(t *testing.T, builder NextCommandBuilder, arguments ...string) string {
	t.Helper()
	command, buildError := builder.Build()
	require.NoError(t, buildError)
	output := &bytes.Buffer{}
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs(arguments)
	command.SetContext(context.Background())
	require.NoError(t, command.Execute())
	return output.String()
}

func replaceDigest(value string) string {
	start := strings.Index(value, `"evidence_sha256":"`)
	if start < 0 {
		start = strings.Index(value, `"evidence_sha256": "`)
	}
	if start < 0 {
		return value
	}
	digestStart := strings.Index(value[start:], ":") + start + 1
	for digestStart < len(value) && (value[digestStart] == ' ' || value[digestStart] == '"') {
		digestStart++
	}
	digestEnd := strings.Index(value[digestStart:], `"`) + digestStart
	return value[:digestStart] + strings.Repeat("0", 64) + value[digestEnd:]
}

type nextGitExecutor struct {
	responses map[string]string
}

func (executor *nextGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{StandardOutput: executor.responses[strings.Join(details.Arguments, " ")]}, nil
}

func (executor *nextGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type nextChatClient struct {
	response string
	calls    int
}

func (client *nextChatClient) Chat(context.Context, llm.ChatRequest) (string, error) {
	client.calls++
	return client.response, nil
}
