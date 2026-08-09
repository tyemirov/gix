package workflow

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/utils/llm"
)

func TestHandleChangelogActionSwallowsNoChangesErrors(t *testing.T) {
	t.Parallel()

	executor := &noopGitExecutor{}
	outputBuffer := &bytes.Buffer{}
	errorsBuffer := &bytes.Buffer{}

	environment := &Environment{
		GitExecutor: executor,
		Output:      outputBuffer,
		Errors:      errorsBuffer,
	}

	repository := &RepositoryState{
		Path: t.TempDir(),
		Inspection: audit.RepositoryInspection{
			FinalOwnerRepo: "owner/repo",
		},
	}

	parameters := map[string]any{
		changelogOptionVersion:   "v1.2.3",
		changelogOptionClient:    &changelogStubChatClient{},
		changelogOptionMaxTokens: 256,
	}

	err := handleChangelogAction(context.Background(), environment, repository, parameters)
	require.NoError(t, err)
	require.Equal(t, "", outputBuffer.String())
	require.Equal(t, "no changes detected for changelog generation\n", errorsBuffer.String())
	require.False(t, environment.sharedState != nil && environment.sharedState.auditReportExecuted, "changelog action should not mutate audit state")
}

type changelogStubChatClient struct{}

func (changelogStubChatClient) Chat(ctx context.Context, request llm.ChatRequest) (string, error) {
	return "", nil
}
