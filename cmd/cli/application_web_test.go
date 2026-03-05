package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/web"
)

func TestExecuteWithOptionsVersionFlagWritesToProvidedOutput(t *testing.T) {
	application := NewApplication()
	application.versionResolver = func(context.Context) string {
		return "v9.9.9"
	}

	exitCode := -1
	application.exitFunction = func(code int) {
		exitCode = code
	}

	var standardOutput bytes.Buffer
	var standardError bytes.Buffer
	executionError := application.ExecuteWithOptions(ExecutionOptions{
		Arguments:      []string{"--version"},
		Context:        context.Background(),
		StandardOutput: &standardOutput,
		StandardError:  &standardError,
		ExitOnVersion:  false,
	})

	require.NoError(t, executionError)
	require.Equal(t, "gix version: v9.9.9\n", standardOutput.String())
	require.Empty(t, standardError.String())
	require.Equal(t, -1, exitCode)
}

func TestExecuteWithOptionsLaunchesWebRunnerWithDefaultPort(t *testing.T) {
	application := NewApplication()

	capturedAddress := ""
	capturedCatalog := web.CommandCatalog{}
	application.webRunner = func(executionContext context.Context, options web.ServerOptions) error {
		capturedAddress = options.Address
		capturedCatalog = options.Catalog
		require.NotNil(t, options.Execute)
		require.NotNil(t, executionContext)
		return nil
	}

	var standardOutput bytes.Buffer
	executionError := application.ExecuteWithOptions(ExecutionOptions{
		Arguments:      []string{"--web"},
		Context:        context.Background(),
		StandardOutput: &standardOutput,
		ExitOnVersion:  false,
	})

	require.NoError(t, executionError)
	require.Equal(t, "127.0.0.1:8080", capturedAddress)
	require.Contains(t, standardOutput.String(), "http://127.0.0.1:8080")
	require.NotEmpty(t, capturedCatalog.Commands)
}

func TestExecuteWithOptionsLaunchesWebRunnerWithExplicitPort(t *testing.T) {
	application := NewApplication()

	capturedAddress := ""
	application.webRunner = func(_ context.Context, options web.ServerOptions) error {
		capturedAddress = options.Address
		return nil
	}

	executionError := application.ExecuteWithOptions(ExecutionOptions{
		Arguments:     []string{"--web", "18080"},
		Context:       context.Background(),
		ExitOnVersion: false,
	})

	require.NoError(t, executionError)
	require.Equal(t, "127.0.0.1:18080", capturedAddress)
}

func TestCommandCatalogMarksInactionableCommands(t *testing.T) {
	application := NewApplication()
	catalog := application.commandCatalog()

	versionCommand := findCatalogCommand(catalog, "gix version")
	require.NotNil(t, versionCommand)
	require.True(t, versionCommand.Actionable)

	messageNamespace := findCatalogCommand(catalog, "gix message")
	require.NotNil(t, messageNamespace)
	require.False(t, messageNamespace.Actionable)

	workflowCommand := findCatalogCommand(catalog, "gix workflow")
	require.NotNil(t, workflowCommand)
	require.False(t, workflowCommand.Actionable)

	defaultCommand := findCatalogCommand(catalog, "gix default")
	require.NotNil(t, defaultCommand)
	require.True(t, defaultCommand.Actionable)

	protocolCommand := findCatalogCommand(catalog, "gix remote update-protocol")
	require.NotNil(t, protocolCommand)
	require.False(t, protocolCommand.Actionable)

	retagCommand := findCatalogCommand(catalog, "gix release retag")
	require.NotNil(t, retagCommand)
	require.False(t, retagCommand.Actionable)

	renameCommand := findCatalogCommand(catalog, "gix folder rename")
	require.NotNil(t, renameCommand)
	require.True(t, renameCommand.Actionable)
}

func TestWebServerExecutesVersionCommand(t *testing.T) {
	application := NewApplication()
	application.versionResolver = func(context.Context) string {
		return "v4.5.6"
	}

	server, serverError := web.NewServer(web.ServerOptions{
		Address: "127.0.0.1:8080",
		Branches: web.BranchCatalog{
			RepositoryPath: "/tmp/example",
			Branches: []web.BranchDescriptor{
				{Name: "feature/demo", Current: true, Upstream: "origin/feature/demo"},
				{Name: "master", Current: false, Upstream: "origin/master"},
			},
		},
		Catalog: application.commandCatalog(),
		Execute: application.newWebCommandExecutor(),
	})
	require.NoError(t, serverError)

	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	indexResponse, indexError := http.Get(httpServer.URL)
	require.NoError(t, indexError)
	defer indexResponse.Body.Close()
	require.Equal(t, http.StatusOK, indexResponse.StatusCode)

	var indexDocument bytes.Buffer
	_, copyError := indexDocument.ReadFrom(indexResponse.Body)
	require.NoError(t, copyError)
	require.Contains(t, indexDocument.String(), "<title>gix Control Surface</title>")

	commandsResponse, commandsError := http.Get(httpServer.URL + "/api/commands")
	require.NoError(t, commandsError)
	defer commandsResponse.Body.Close()
	require.Equal(t, http.StatusOK, commandsResponse.StatusCode)

	var catalog web.CommandCatalog
	require.NoError(t, json.NewDecoder(commandsResponse.Body).Decode(&catalog))
	require.NotEmpty(t, catalog.Commands)
	require.True(t, catalogContainsCommand(catalog, "gix version"))

	branchesResponse, branchesError := http.Get(httpServer.URL + "/api/branches")
	require.NoError(t, branchesError)
	defer branchesResponse.Body.Close()
	require.Equal(t, http.StatusOK, branchesResponse.StatusCode)

	var branches web.BranchCatalog
	require.NoError(t, json.NewDecoder(branchesResponse.Body).Decode(&branches))
	require.Equal(t, "/tmp/example", branches.RepositoryPath)
	require.Len(t, branches.Branches, 2)
	require.Equal(t, "feature/demo", branches.Branches[0].Name)
	require.True(t, branches.Branches[0].Current)

	runBody := strings.NewReader(`{"arguments":["version"]}`)
	runResponse, runError := http.Post(httpServer.URL+"/api/runs", "application/json", runBody)
	require.NoError(t, runError)
	defer runResponse.Body.Close()
	require.Equal(t, http.StatusAccepted, runResponse.StatusCode)

	var createdRun web.RunSnapshot
	require.NoError(t, json.NewDecoder(runResponse.Body).Decode(&createdRun))
	require.NotEmpty(t, createdRun.ID)

	var finalRun web.RunSnapshot
	require.Eventually(t, func() bool {
		pollResponse, pollError := http.Get(httpServer.URL + "/api/runs/" + createdRun.ID)
		if pollError != nil {
			return false
		}
		defer pollResponse.Body.Close()
		if pollResponse.StatusCode != http.StatusOK {
			return false
		}
		if decodeError := json.NewDecoder(pollResponse.Body).Decode(&finalRun); decodeError != nil {
			return false
		}
		return finalRun.Status != "running"
	}, 5*time.Second, 100*time.Millisecond)

	require.Equal(t, "succeeded", finalRun.Status)
	require.Equal(t, 0, finalRun.ExitCode)
	require.Contains(t, finalRun.StandardOutput, "gix version: v4.5.6")
	require.Empty(t, finalRun.StandardError)
	require.Empty(t, finalRun.Error)
}

func catalogContainsCommand(catalog web.CommandCatalog, commandPath string) bool {
	return findCatalogCommand(catalog, commandPath) != nil
}

func findCatalogCommand(catalog web.CommandCatalog, commandPath string) *web.CommandDescriptor {
	for _, command := range catalog.Commands {
		if command.Path == commandPath {
			commandCopy := command
			return &commandCopy
		}
	}
	return nil
}
