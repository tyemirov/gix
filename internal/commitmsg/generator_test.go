package commitmsg

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/pkg/llm"
)

func TestBuildRequestStagedDiff(t *testing.T) {
	executor := &stubGitExecutor{
		responses: map[string]string{
			"status --short":                   " M internal/app.go\n?? README.md\n",
			"diff --unified=3 --cached --stat": " internal/app.go | 10 +++++-----\n 1 file changed, 5 insertions(+), 5 deletions(-)\n",
			"diff --unified=3 --cached":        "diff --git a/internal/app.go b/internal/app.go\n@@ -1,2 +1,2 @@\n-func old() {}\n+func updated() {}\n",
		},
	}
	client := &stubChatClient{}
	generator := Generator{GitExecutor: executor, Client: client}

	request, err := generator.BuildRequest(context.Background(), Options{RepositoryPath: "/tmp/repo"})
	require.NoError(t, err)

	require.Len(t, request.Messages, 2)
	require.Equal(t, "system", request.Messages[0].Role)
	require.Contains(t, request.Messages[1].Content, "Repository: repo")
	require.Contains(t, request.Messages[1].Content, "internal/app.go | 10 +++++-----")
	require.Contains(t, request.Messages[1].Content, "func updated()")
	require.Contains(t, request.Messages[1].Content, "Diff source: STAGED")
	require.Equal(t, defaultMaxTokens, request.MaxTokens)

	expectedCommands := [][]string{
		{"status", "--short"},
		{"diff", "--unified=3", "--cached", "--stat"},
		{"diff", "--unified=3", "--cached"},
	}
	require.Equal(t, expectedCommands, executor.calls)
}

func TestBuildRequestWorktreeDiff(t *testing.T) {
	executor := &stubGitExecutor{
		responses: map[string]string{
			"status --short":          " M file.txt\n",
			"diff --unified=3 --stat": " file.txt | 2 +-",
			"diff --unified=3":        "diff --git a/file.txt b/file.txt\n@@\n-old\n+new\n",
		},
	}
	generator := Generator{GitExecutor: executor, Client: &stubChatClient{}}

	request, err := generator.BuildRequest(context.Background(), Options{RepositoryPath: "/tmp/repo", Source: DiffSourceWorktree, MaxTokens: 80})
	require.NoError(t, err)
	require.Contains(t, request.Messages[1].Content, "Diff source: WORKTREE")
	require.Equal(t, 80, request.MaxTokens)

	expectedCommands := [][]string{
		{"status", "--short"},
		{"diff", "--unified=3", "--stat"},
		{"diff", "--unified=3"},
	}
	require.Equal(t, expectedCommands, executor.calls)
}

func TestBuildRequestNoChangesReturnsError(t *testing.T) {
	executor := &stubGitExecutor{
		responses: map[string]string{
			"status --short":                   "\n",
			"diff --unified=3 --cached":        "\n",
			"diff --unified=3 --cached --stat": "\n",
		},
	}
	generator := Generator{GitExecutor: executor, Client: &stubChatClient{}}

	_, err := generator.BuildRequest(context.Background(), Options{RepositoryPath: "/tmp/repo"})
	require.ErrorIs(t, err, ErrNoChanges)
}

func TestGenerateCallsChatClient(t *testing.T) {
	executor := &stubGitExecutor{
		responses: map[string]string{
			"status --short":                   " M example.go\n",
			"diff --unified=3 --cached --stat": " example.go | 1 +-\n",
			"diff --unified=3 --cached":        "diff --git a/example.go b/example.go\n@@\n-func old()\n+func new()\n",
		},
	}
	client := &stubChatClient{response: "feat: add example"}
	generator := Generator{GitExecutor: executor, Client: client}

	result, err := generator.Generate(context.Background(), Options{RepositoryPath: "/tmp/repo"})
	require.NoError(t, err)
	require.Equal(t, "feat: add example", result.Message)
	require.NotNil(t, client.lastRequest)
	require.Contains(t, client.lastRequest.Messages[1].Content, "example.go | 1 +-")
}

func TestBuildRequestValidatesRepositoryPath(t *testing.T) {
	generator := Generator{GitExecutor: &stubGitExecutor{}, Client: &stubChatClient{}}
	_, err := generator.BuildRequest(context.Background(), Options{})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "repository path"))
}

type stubGitExecutor struct {
	responses map[string]string
	calls     [][]string
}

func (executor *stubGitExecutor) ExecuteGit(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	key := strings.Join(details.Arguments, " ")
	executor.calls = append(executor.calls, details.Arguments)
	value, ok := executor.responses[key]
	if !ok {
		return execshell.ExecutionResult{}, nil
	}
	return execshell.ExecutionResult{StandardOutput: value}, nil
}

func (executor *stubGitExecutor) ExecuteGitHubCLI(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, errors.New("not implemented")
}

type stubChatClient struct {
	lastRequest llm.ChatRequest
	response    string
	err         error
}

func (client *stubChatClient) Chat(ctx context.Context, request llm.ChatRequest) (string, error) {
	client.lastRequest = request
	if client.err != nil {
		return "", client.err
	}
	return client.response, nil
}
