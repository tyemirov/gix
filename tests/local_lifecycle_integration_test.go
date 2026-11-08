package tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type localRuntimeFixture struct {
	root, state, config string
	port                int
}

func newLocalRuntimeFixture(t *testing.T) localRuntimeFixture {
	t.Helper()
	root := createGitRepository(t, gitRepositoryOptions{InitialBranch: "master"})
	root, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	state, err := os.MkdirTemp("", "gix-local-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(state)) })
	return localRuntimeFixture{root, state, writeCanonicalIntegrationConfiguration(t), reserveFreeLoopbackPort(t)}
}

func (fixture localRuntimeFixture) make(t *testing.T, operation string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "make", "--no-print-directory", operation,
		"GIX_LOCAL_ROOT="+fixture.root, "GIX_LOCAL_STATE_DIR="+fixture.state,
		"GIX_LOCAL_CONFIG="+fixture.config, fmt.Sprintf("GIX_LOCAL_PORT=%d", fixture.port))
	command.Dir = releaseRepositoryRoot(t)
	command.Env = buildCommandEnvironment(integrationCommandOptions{})
	output, err := command.CombinedOutput()
	return string(output), err
}

func TestLocalLifecycleMakeTargets(t *testing.T) {
	first := newLocalRuntimeFixture(t)
	second := newLocalRuntimeFixture(t)
	for _, fixture := range []localRuntimeFixture{first, second} {
		t.Cleanup(func() { _, _ = fixture.make(t, "down") })
		output, err := fixture.make(t, "down")
		require.NoError(t, err, output)
		writeReleaseFixtureFile(t, filepath.Join(fixture.root, "keep.txt"), "application data\n")
		writeReleaseFixtureFile(t, filepath.Join(fixture.state, "keep.txt"), "operator data\n")
		output, err = fixture.make(t, "up")
		require.NoError(t, err, output)
		address := fmt.Sprintf("http://127.0.0.1:%d", fixture.port)
		require.Contains(t, output, address)
		require.Contains(t, readHTTPBody(t, address+"/"), "app.js")
		require.Contains(t, readHTTPBody(t, address+"/api/repos"), fixture.root)
		output, err = fixture.make(t, "up")
		require.NoError(t, err, output)
		require.Contains(t, output, address)
	}
	conflicting := first
	conflicting.port = reserveFreeLoopbackPort(t)
	output, err := conflicting.make(t, "up")
	require.Error(t, err, output)
	require.Contains(t, output, "different settings")
	for _, fixture := range []localRuntimeFixture{first, second} {
		output, err := fixture.make(t, "down")
		require.NoError(t, err, output)
		client := &http.Client{Timeout: time.Second}
		response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", fixture.port))
		if response != nil {
			response.Body.Close()
		}
		require.Error(t, err, "down must stop the selected application before it returns")
		output, err = fixture.make(t, "down")
		require.NoError(t, err, output)
		for path, content := range map[string]string{
			filepath.Join(fixture.root, "keep.txt"):  "application data\n",
			filepath.Join(fixture.state, "keep.txt"): "operator data\n",
		} {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, content, string(data))
		}
		if fixture == first {
			require.Contains(t, readHTTPBody(t, fmt.Sprintf("http://127.0.0.1:%d/", second.port)), "app.js")
		}
	}
	output, err = first.make(t, "up")
	require.NoError(t, err, output)
	output, err = first.make(t, "down")
	require.NoError(t, err, output)
}

func TestLocalLifecyclePreservesStartupFailures(t *testing.T) {
	for _, scenario := range []string{"occupied-port", "invalid-port", "missing-config"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := newLocalRuntimeFixture(t)
			t.Cleanup(func() { _, _ = fixture.make(t, "down") })
			switch scenario {
			case "occupied-port":
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					fmt.Fprint(w, "unrelated service")
				}))
				defer server.Close()
				_, err := fmt.Sscanf(server.URL, "http://127.0.0.1:%d", &fixture.port)
				require.NoError(t, err)
				output, err := fixture.make(t, "up")
				require.Error(t, err, output)
				require.Contains(t, readHTTPBody(t, server.URL), "unrelated service")
				output, err = fixture.make(t, "down")
				require.NoError(t, err, output)
				require.Contains(t, readHTTPBody(t, server.URL), "unrelated service")
				return
			case "invalid-port":
				fixture.port = 0
			case "missing-config":
				fixture.config = filepath.Join(fixture.state, "missing.yml")
			}
			output, err := fixture.make(t, "up")
			require.Error(t, err, output)
			require.NotEmpty(t, output)
		})
	}
}
