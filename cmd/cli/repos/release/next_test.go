package release

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/llmclient"
	"github.com/tyemirov/utils/llm"
)

func TestNextCommandSelectsEstablishedSemVer(t *testing.T) {
	executor := &nextGitExecutor{responses: map[string]string{
		"rev-parse --show-toplevel":                                    "/repo\n",
		"rev-parse HEAD":                                               "abc123\n",
		"rev-parse --verify v1.2.3^{commit}":                           "base123\n",
		"merge-base --is-ancestor base123 abc123":                      "",
		"tag --list":                                                   "v1.2.3\nv1.9.0-rc.1\n",
		"log --pretty=format:%s%n%b%x1e base123..abc123":               "feat: add reports\n\x1e",
		"diff --stat base123..abc123":                                  " report.go | 4 ++++\n",
		"diff --unified=3 base123..abc123 -- . :(exclude)CHANGELOG.md": "diff --git a/report.go b/report.go\n",
		"diff --unified=0 base123..abc123 -- CHANGELOG.md":             "@@ -4,0 +5 @@\n+- Added reports.\n",
	}}
	client := &nextChatClient{response: `{"impact":"additive","public_contract":"CLI reporting","reason":"The release adds reporting."}`}
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
		"deterministic_floor":"patch",
		"reason":"The release adds reporting.",
		"evidence_sha256":"`+strings.Repeat("0", 64)+`"
	}`, replaceDigest(output))
	require.Equal(t, 2, client.calls)
	require.Contains(t, executor.calls, "rev-parse --verify v1.2.3^{commit}")
	require.Contains(t, executor.calls, "merge-base --is-ancestor base123 abc123")
	require.Contains(t, executor.calls, "log --pretty=format:%s%n%b%x1e base123..abc123")
	require.NotContains(t, executor.calls, "log --pretty=format:%s%n%b%x1e v1.2.3..HEAD")
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
		"rev-parse --show-toplevel":                                    "/repo\n",
		"rev-parse HEAD":                                               "abc123\n",
		"rev-parse --verify v1.2.3^{commit}":                           "base123\n",
		"merge-base --is-ancestor base123 abc123":                      "",
		"tag --list":                                                   "v1.2.3\nv2.0.0\n",
		"log --pretty=format:%s%n%b%x1e base123..abc123":               "fix: preserve behavior\n\x1e",
		"diff --stat base123..abc123":                                  " report.go | 1 +\n",
		"diff --unified=3 base123..abc123 -- . :(exclude)CHANGELOG.md": "diff --git a/report.go b/report.go\n",
		"diff --unified=0 base123..abc123 -- CHANGELOG.md":             "@@ -4,0 +5 @@\n+- Fixed reports.\n",
	}}
	client := &nextChatClient{response: `{"impact":"compatible","public_contract":"CLI reporting","reason":"The release fixes reporting."}`}
	builder := nextTestBuilder(executor, client, []byte("schema_version: 1\nscheme: semver\n"))

	output := executeNextCommand(t, builder, "--format", "json", "--exclude-tag", "v2.0.0")

	require.Contains(t, output, `"boundary_tag":"v1.2.3"`)
	require.Contains(t, output, `"next_version":"v1.2.4"`)
}

func TestNextCommandIgnoresInvalidHistoricalMajorTags(t *testing.T) {
	executor := &nextGitExecutor{responses: map[string]string{
		"rev-parse --show-toplevel":                                       "/repo\n",
		"rev-parse HEAD":                                                  "source600\n",
		"rev-parse --verify v1.1.25^{commit}":                             "base600\n",
		"merge-base --is-ancestor base600 source600":                      "",
		"tag --list":                                                      "v1.1.25\nv6.0.0\n",
		"log --pretty=format:%s%n%b%x1e base600..source600":               "fix: repair root installation\n\x1e",
		"diff --stat base600..source600":                                  " go.mod | 2 +-\n",
		"diff --unified=3 base600..source600 -- . :(exclude)CHANGELOG.md": "diff --git a/go.mod b/go.mod\n",
		"diff --unified=0 base600..source600 -- CHANGELOG.md":             "@@ -4,0 +5 @@\n+- Fixed root installation.\n",
	}}
	client := &nextChatClient{response: `{"impact":"compatible","public_contract":"root Go installation","reason":"The release repairs root installation."}`}
	builder := nextTestBuilder(executor, client, []byte("schema_version: 1\nscheme: semver\nsemver:\n  fixed_major: 1\n"))

	output := executeNextCommand(t, builder, "--format", "json")

	require.Contains(t, output, `"contract":"mprlab.version-decision/v1"`)
	require.Contains(t, output, `"previous_version":"v1.1.25"`)
	require.Contains(t, output, `"next_version":"v1.1.26"`)
	require.NotContains(t, output, "fixed_major")
	require.NotContains(t, output, "go_install")
}

func TestNextCommandMapsFixedMajorIncompatibilityToMinor(t *testing.T) {
	executor := &nextGitExecutor{responses: map[string]string{
		"rev-parse --show-toplevel":                                       "/repo\n",
		"rev-parse HEAD":                                                  "source601\n",
		"rev-parse --verify v1.1.26^{commit}":                             "base601\n",
		"merge-base --is-ancestor base601 source601":                      "",
		"tag --list":                                                      "v1.1.26\nv7.0.1\n",
		"log --pretty=format:%s%n%b%x1e base601..source601":               "change the release contract\n\x1e",
		"diff --stat base601..source601":                                  "README.md | 2 +-\n",
		"diff --unified=3 base601..source601 -- . :(exclude)CHANGELOG.md": "diff --git a/README.md b/README.md\n",
		"diff --unified=0 base601..source601 -- CHANGELOG.md":             "@@ -4,0 +5 @@\n+- Changed the release contract.\n",
	}}
	client := &nextChatClient{response: `{"impact":"incompatible","public_contract":"Gix release versions","reason":"The release changes the Gix version contract."}`}
	builder := nextTestBuilder(executor, client, []byte("schema_version: 1\nscheme: semver\nsemver:\n  fixed_major: 1\n"))

	output := executeNextCommand(t, builder, "--format", "json")

	require.Contains(t, output, `"previous_version":"v1.1.26"`)
	require.Contains(t, output, `"next_version":"v1.2.0"`)
	require.Contains(t, output, `"bump":"minor"`)
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
		ReadFile: func(path string) ([]byte, error) {
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
	calls     []string
}

func (executor *nextGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	key := strings.Join(details.Arguments, " ")
	executor.calls = append(executor.calls, key)
	return execshell.ExecutionResult{StandardOutput: executor.responses[key]}, nil
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
