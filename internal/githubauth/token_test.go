package githubauth_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/githubauth"
)

func TestResolveTokenUsesConcreteInputsOnly(t *testing.T) {
	t.Setenv(githubauth.EnvGitHubCLIToken, "ambient-token")

	ambientToken, ambientAvailable := githubauth.ResolveToken(context.Background(), nil)
	require.False(t, ambientAvailable)
	require.Empty(t, ambientToken)

	executionContext := githubauth.WithCredential(context.Background(), "configured-token")
	configuredToken, configuredAvailable := githubauth.ResolveToken(executionContext, nil)
	require.True(t, configuredAvailable)
	require.Equal(t, "configured-token", configuredToken)

	explicitToken, explicitAvailable := githubauth.ResolveToken(executionContext, map[string]string{
		githubauth.EnvGitHubCLIToken: "command-token",
	})
	require.True(t, explicitAvailable)
	require.Equal(t, "command-token", explicitToken)
}
