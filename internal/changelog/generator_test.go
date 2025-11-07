package changelog

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/pkg/llm"
)

func TestBuildRequestUsesLatestTagBaseline(t *testing.T) {
	executor := &stubGitExecutor{
		responses: map[string]stubResponse{
			"describe --tags --abbrev=0": {
				output: "v0.2.0\n",
			},
			"log --no-merges --date=short --pretty=format:%h %ad %an %s --max-count=200 v0.2.0..HEAD": {
				output: "abc123 2025-10-01 Alice Add feature support\n",
			},
			"diff --stat v0.2.0..HEAD": {
				output: " internal/app.go | 10 +++++-----\n",
			},
			"diff --unified=3 v0.2.0..HEAD": {
				output: "diff --git a/internal/app.go b/internal/app.go\n@@\n-func old()\n+func new()\n",
			},
		},
	}
	client := &stubChatClient{}
	generator := Generator{GitExecutor: executor, Client: client}

	request, err := generator.BuildRequest(context.Background(), Options{
		RepositoryPath: "/tmp/repo",
		Version:        "v0.3.0",
		ReleaseDate:    "2025-10-08",
	})
	require.NoError(t, err)

	require.Len(t, request.Messages, 2)
	systemMessage := request.Messages[0].Content
	userMessage := request.Messages[1].Content

	require.Contains(t, systemMessage, "Markdown changelog")
	require.Contains(t, systemMessage, "Features ✨")
	require.Contains(t, userMessage, "Release version: v0.3.0")
	require.Contains(t, userMessage, "Release date: 2025-10-08")
	require.Contains(t, userMessage, "changes since tag v0.2.0")
	require.Contains(t, userMessage, "abc123 2025-10-01")
	require.Contains(t, userMessage, "internal/app.go | 10")
	require.Contains(t, userMessage, "diff --git a/internal/app.go")
	require.Equal(t, defaultMaxTokens, request.MaxTokens)

	expectedCommands := [][]string{
		{"describe", "--tags", "--abbrev=0"},
		{"log", "--no-merges", "--date=short", "--pretty=format:%h %ad %an %s", "--max-count=200", "v0.2.0..HEAD"},
		{"diff", "--stat", "v0.2.0..HEAD"},
		{"diff", "--unified=3", "v0.2.0..HEAD"},
	}
	require.Equal(t, expectedCommands, executor.calls)
}

func TestBuildRequestSinceDateBaseline(t *testing.T) {
	dateValue := time.Date(2025, 10, 1, 15, 0, 0, 0, time.UTC)
	executor := &stubGitExecutor{
		responses: map[string]stubResponse{
			"rev-list --max-count=1 --before=2025-10-01T15:00:00Z HEAD": {
				output: "deadbeef\n",
			},
			"log --no-merges --date=short --pretty=format:%h %ad %an %s --max-count=200 deadbeef..HEAD": {
				output: "feed123 2025-10-07 Bob Fix regression\n",
			},
			"diff --stat deadbeef..HEAD": {
				output: " pkg/service.go | 4 ++--\n",
			},
			"diff --unified=3 deadbeef..HEAD": {
				output: "diff --git a/pkg/service.go b/pkg/service.go\n@@\n-return 1\n+return 2\n",
			},
		},
	}
	generator := Generator{GitExecutor: executor, Client: &stubChatClient{}}

	request, err := generator.BuildRequest(context.Background(), Options{
		RepositoryPath: "/tmp/repo",
		Version:        "v0.3.1",
		SinceDate:      &dateValue,
	})
	require.NoError(t, err)

	require.Contains(t, request.Messages[1].Content, "changes since 2025-10-01T15:00:00Z")
	require.Contains(t, request.Messages[1].Content, "feed123 2025-10-07 Bob Fix regression")

	expectedCommands := [][]string{
		{"rev-list", "--max-count=1", "--before=2025-10-01T15:00:00Z", "HEAD"},
		{"log", "--no-merges", "--date=short", "--pretty=format:%h %ad %an %s", "--max-count=200", "deadbeef..HEAD"},
		{"diff", "--stat", "deadbeef..HEAD"},
		{"diff", "--unified=3", "deadbeef..HEAD"},
	}
	require.Equal(t, expectedCommands, executor.calls)
}

func TestBuildRequestUsesProvidedReference(t *testing.T) {
	executor := &stubGitExecutor{
		responses: map[string]stubResponse{
			"log --no-merges --date=short --pretty=format:%h %ad %an %s --max-count=200 v0.2.1..HEAD": {
				output: "f1e2d3 2025-10-10 Carol Improve docs\n",
			},
			"diff --stat v0.2.1..HEAD": {
				output: " docs/guide.md | 1 +\n",
			},
			"diff --unified=3 v0.2.1..HEAD": {
				output: "diff --git a/docs/guide.md b/docs/guide.md\n@@\n-Old\n+New\n",
			},
		},
	}
	generator := Generator{GitExecutor: executor, Client: &stubChatClient{}}

	request, err := generator.BuildRequest(context.Background(), Options{
		RepositoryPath: "/tmp/repo",
		Version:        "v0.3.2",
		SinceReference: "v0.2.1",
	})
	require.NoError(t, err)

	require.Contains(t, request.Messages[1].Content, "changes since v0.2.1")
	require.NotContains(t, strings.Join(flattenCommandCalls(executor.calls), " "), "describe")
}

func TestGenerateReturnsSection(t *testing.T) {
	executor := &stubGitExecutor{
		responses: map[string]stubResponse{
			"describe --tags --abbrev=0": {
				output: "v0.2.0\n",
			},
			"log --no-merges --date=short --pretty=format:%h %ad %an %s --max-count=200 v0.2.0..HEAD": {
				output: "abc123 2025-10-01 Alice Add feature support\n",
			},
			"diff --stat v0.2.0..HEAD": {
				output: " internal/app.go | 10 +++++-----\n",
			},
			"diff --unified=3 v0.2.0..HEAD": {
				output: "diff --git a/internal/app.go b/internal/app.go\n",
			},
		},
	}
	client := &stubChatClient{response: "## [v0.3.0]\n\n### Features ✨\n- New feature\n"}
	generator := Generator{GitExecutor: executor, Client: client}

	result, err := generator.Generate(context.Background(), Options{
		RepositoryPath: "/tmp/repo",
		Version:        "v0.3.0",
	})
	require.NoError(t, err)
	require.Equal(t, "## [v0.3.0]\n\n### Features ✨\n- New feature", result.Section)
	require.NotNil(t, client.lastRequest)
}

func TestBuildRequestReturnsErrNoChanges(t *testing.T) {
	executor := &stubGitExecutor{
		responses: map[string]stubResponse{
			"describe --tags --abbrev=0": {
				output: "v0.2.0\n",
			},
			"log --no-merges --date=short --pretty=format:%h %ad %an %s --max-count=200 v0.2.0..HEAD": {
				output: "\n",
			},
			"diff --stat v0.2.0..HEAD": {
				output: "\n",
			},
			"diff --unified=3 v0.2.0..HEAD": {
				output: "\n",
			},
		},
	}
	generator := Generator{GitExecutor: executor, Client: &stubChatClient{}}

	_, err := generator.BuildRequest(context.Background(), Options{
		RepositoryPath: "/tmp/repo",
		Version:        "v0.3.0",
	})
	require.ErrorIs(t, err, ErrNoChanges)
}

func TestChangelogGenerateHandlesLLMResponses(t *testing.T) {
	testCases := []struct {
		name          string
		client        *stubChatClient
		expectedError string
		expectedSec   string
	}{
		{
			name:        "success trims whitespace",
			client:      &stubChatClient{response: "  ## [v0.3.0]\n- change  "},
			expectedSec: "## [v0.3.0]\n- change",
		},
		{
			name:          "empty response",
			client:        &stubChatClient{response: " \n "},
			expectedError: "empty changelog section",
		},
		{
			name:          "llm error surfaces",
			client:        &stubChatClient{err: errors.New("timeout")},
			expectedError: "timeout",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			executor := &stubGitExecutor{
				responses: map[string]stubResponse{
					"describe --tags --abbrev=0": {
						output: "v0.2.0\n",
					},
					"log --no-merges --date=short --pretty=format:%h %ad %an %s --max-count=200 v0.2.0..HEAD": {
						output: "abc123 2025-10-01 Alice Add feature support\n",
					},
					"diff --stat v0.2.0..HEAD": {
						output: " internal/app.go | 10 +++++-----\n",
					},
					"diff --unified=3 v0.2.0..HEAD": {
						output: "diff --git a/internal/app.go b/internal/app.go\n",
					},
				},
			}

			generator := Generator{
				GitExecutor: executor,
				Client:      tc.client,
			}

			result, err := generator.Generate(context.Background(), Options{
				RepositoryPath: "/tmp/repo",
				Version:        "v0.3.0",
			})
			if tc.expectedError != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedError)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedSec, result.Section)
		})
	}
}

type stubResponse struct {
	output string
	err    error
}

type stubGitExecutor struct {
	responses map[string]stubResponse
	calls     [][]string
}

func (executor *stubGitExecutor) ExecuteGit(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	key := strings.Join(details.Arguments, " ")
	executor.calls = append(executor.calls, details.Arguments)
	response, ok := executor.responses[key]
	if !ok {
		return execshell.ExecutionResult{}, nil
	}
	if response.err != nil {
		return execshell.ExecutionResult{}, response.err
	}
	return execshell.ExecutionResult{StandardOutput: response.output}, nil
}

func (executor *stubGitExecutor) ExecuteGitHubCLI(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

type stubChatClient struct {
	response    string
	err         error
	lastRequest *llm.ChatRequest
}

func (client *stubChatClient) Chat(ctx context.Context, request llm.ChatRequest) (string, error) {
	copy := request
	client.lastRequest = &copy
	if client.err != nil {
		return "", client.err
	}
	return client.response, nil
}

func flattenCommandCalls(calls [][]string) []string {
	flattened := make([]string, 0, len(calls))
	for _, call := range calls {
		flattened = append(flattened, strings.Join(call, " "))
	}
	return flattened
}
