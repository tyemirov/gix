package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
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
	require.Equal(t, web.CommandTargetRequirementNone, versionCommand.Target.Repository)

	messageNamespace := findCatalogCommand(catalog, "gix message")
	require.NotNil(t, messageNamespace)
	require.False(t, messageNamespace.Actionable)

	workflowCommand := findCatalogCommand(catalog, "gix workflow")
	require.NotNil(t, workflowCommand)
	require.False(t, workflowCommand.Actionable)
	require.Equal(t, web.CommandTargetRequirementRequired, workflowCommand.Target.Repository)
	require.True(t, workflowCommand.Target.SupportsBatch)

	defaultCommand := findCatalogCommand(catalog, "gix default")
	require.NotNil(t, defaultCommand)
	require.True(t, defaultCommand.Actionable)
	require.Equal(t, web.CommandTargetRequirementRequired, defaultCommand.Target.Repository)
	require.Equal(t, web.CommandTargetRequirementRequired, defaultCommand.Target.Ref)
	require.True(t, defaultCommand.Target.SupportsBatch)

	protocolCommand := findCatalogCommand(catalog, "gix remote update-protocol")
	require.NotNil(t, protocolCommand)
	require.False(t, protocolCommand.Actionable)

	retagCommand := findCatalogCommand(catalog, "gix release retag")
	require.NotNil(t, retagCommand)
	require.False(t, retagCommand.Actionable)

	releaseCommand := findCatalogCommand(catalog, "gix release")
	require.NotNil(t, releaseCommand)
	require.False(t, releaseCommand.Actionable)

	commitMessageCommand := findCatalogCommand(catalog, "gix message commit")
	require.NotNil(t, commitMessageCommand)
	require.False(t, commitMessageCommand.Actionable)

	changelogMessageCommand := findCatalogCommand(catalog, "gix message changelog")
	require.NotNil(t, changelogMessageCommand)
	require.False(t, changelogMessageCommand.Actionable)

	filesReplaceCommand := findCatalogCommand(catalog, "gix files replace")
	require.NotNil(t, filesReplaceCommand)
	require.False(t, filesReplaceCommand.Actionable)
	require.Equal(t, "files", filesReplaceCommand.Target.Group)
	require.Equal(t, web.CommandTargetRequirementRequired, filesReplaceCommand.Target.Repository)
	require.Equal(t, web.CommandTargetRequirementOptional, filesReplaceCommand.Target.Ref)
	require.Equal(t, web.CommandTargetRequirementRequired, filesReplaceCommand.Target.Path)
	require.True(t, filesReplaceCommand.Target.SupportsBatch)
	require.Equal(t, "files_replace", filesReplaceCommand.Target.DraftTemplate)

	filesAddCommand := findCatalogCommand(catalog, "gix files add")
	require.NotNil(t, filesAddCommand)
	require.False(t, filesAddCommand.Actionable)
	require.Equal(t, "files", filesAddCommand.Target.Group)
	require.Equal(t, web.CommandTargetRequirementRequired, filesAddCommand.Target.Repository)
	require.Equal(t, web.CommandTargetRequirementNone, filesAddCommand.Target.Ref)
	require.Equal(t, web.CommandTargetRequirementRequired, filesAddCommand.Target.Path)
	require.True(t, filesAddCommand.Target.SupportsBatch)
	require.Equal(t, "files_add", filesAddCommand.Target.DraftTemplate)

	filesRemoveCommand := findCatalogCommand(catalog, "gix files rm")
	require.NotNil(t, filesRemoveCommand)
	require.False(t, filesRemoveCommand.Actionable)
	require.Equal(t, "files", filesRemoveCommand.Target.Group)
	require.Equal(t, web.CommandTargetRequirementRequired, filesRemoveCommand.Target.Repository)
	require.Equal(t, web.CommandTargetRequirementNone, filesRemoveCommand.Target.Ref)
	require.Equal(t, web.CommandTargetRequirementRequired, filesRemoveCommand.Target.Path)
	require.True(t, filesRemoveCommand.Target.SupportsBatch)
	require.Equal(t, "files_remove", filesRemoveCommand.Target.DraftTemplate)

	renameCommand := findCatalogCommand(catalog, "gix folder rename")
	require.NotNil(t, renameCommand)
	require.True(t, renameCommand.Actionable)
	require.Equal(t, "repository", renameCommand.Target.Group)
}

func TestRepositoryCatalogUsesCurrentRepositoryContext(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))
	createTestBranch(t, repositoryPath, "feature/demo")

	nestedWorkingDirectory := filepath.Join(repositoryPath, "internal")
	require.NoError(t, os.MkdirAll(nestedWorkingDirectory, 0o755))
	withWorkingDirectory(t, nestedWorkingDirectory, func() {
		application := NewApplication()
		catalog := application.repositoryCatalog(context.Background())

		require.Equal(t, "current_repo", catalog.LaunchMode)
		require.Equal(t, canonicalPath(t, nestedWorkingDirectory), canonicalPath(t, catalog.LaunchPath))
		require.Len(t, catalog.Repositories, 1)
		require.Equal(t, canonicalPath(t, repositoryPath), canonicalPath(t, catalog.Repositories[0].Path))
		require.Equal(t, "feature/demo", catalog.Repositories[0].CurrentBranch)
		require.True(t, catalog.Repositories[0].ContextCurrent)
		require.Equal(t, catalog.Repositories[0].ID, catalog.SelectedRepositoryID)
	})
}

func TestRepositoryCatalogDiscoversRepositoriesBeneathWorkingDirectory(t *testing.T) {
	rootPath := t.TempDir()
	firstRepository := createTestRepository(t, filepath.Join(rootPath, "alpha"))
	secondRepository := createTestRepository(t, filepath.Join(rootPath, "nested", "beta"))
	createTestBranch(t, secondRepository, "feature/demo")

	withWorkingDirectory(t, rootPath, func() {
		application := NewApplication()
		catalog := application.repositoryCatalog(context.Background())

		require.Equal(t, "discovered_repositories", catalog.LaunchMode)
		require.Equal(t, canonicalPath(t, rootPath), canonicalPath(t, catalog.LaunchPath))
		require.Len(t, catalog.Repositories, 2)
		require.Equal(t, canonicalPath(t, firstRepository), canonicalPath(t, catalog.Repositories[0].Path))
		require.Equal(t, canonicalPath(t, secondRepository), canonicalPath(t, catalog.Repositories[1].Path))
		require.False(t, catalog.Repositories[0].ContextCurrent)
		require.False(t, catalog.Repositories[1].ContextCurrent)
		require.Equal(t, catalog.Repositories[0].ID, catalog.SelectedRepositoryID)
	})
}

func TestLoadRepositoryBranchesReturnsLocalBranches(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))
	createTestBranch(t, repositoryPath, "feature/demo")

	application := NewApplication()
	branchCatalog := application.loadRepositoryBranches(context.Background(), web.RepositoryDescriptor{
		ID:   "repo-001",
		Path: repositoryPath,
	})

	require.Empty(t, branchCatalog.Error)
	require.Equal(t, "repo-001", branchCatalog.RepositoryID)
	require.Equal(t, repositoryPath, branchCatalog.RepositoryPath)
	require.Len(t, branchCatalog.Branches, 2)
	require.Equal(t, "feature/demo", branchCatalog.Branches[0].Name)
	require.True(t, branchCatalog.Branches[0].Current)
	require.Equal(t, "master", branchCatalog.Branches[1].Name)
}

func TestWebServerExecutesVersionCommand(t *testing.T) {
	application := NewApplication()
	application.versionResolver = func(context.Context) string {
		return "v4.5.6"
	}

	server, serverError := web.NewServer(web.ServerOptions{
		Address: "127.0.0.1:8080",
		Repositories: web.RepositoryCatalog{
			LaunchPath:           "/tmp/example",
			LaunchMode:           "current_repo",
			SelectedRepositoryID: "repo-001",
			Repositories: []web.RepositoryDescriptor{
				{
					ID:             "repo-001",
					Name:           "example",
					Path:           "/tmp/example",
					CurrentBranch:  "feature/demo",
					DefaultBranch:  "master",
					Dirty:          false,
					ContextCurrent: true,
				},
			},
		},
		Catalog: application.commandCatalog(),
		LoadBranches: func(_ context.Context, repository web.RepositoryDescriptor) web.BranchCatalog {
			return web.BranchCatalog{
				RepositoryID:   repository.ID,
				RepositoryPath: repository.Path,
				Branches: []web.BranchDescriptor{
					{Name: "feature/demo", Current: true, Upstream: "origin/feature/demo"},
					{Name: "master", Current: false, Upstream: "origin/master"},
				},
			}
		},
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
	require.Contains(t, indexDocument.String(), "Build a target set first")
	require.Contains(t, indexDocument.String(), "<h2>Repos</h2>")
	require.Contains(t, indexDocument.String(), "<h3>Repos</h3>")
	require.Contains(t, indexDocument.String(), "<h3>Paths</h3>")
	require.Contains(t, indexDocument.String(), "<h3>Tasks</h3>")
	require.Contains(t, indexDocument.String(), "id=\"target-ref-select\"")
	require.Contains(t, indexDocument.String(), "Load audit snapshot")
	require.Contains(t, indexDocument.String(), "Load remote normalization")
	require.Contains(t, indexDocument.String(), "Load workflow command")

	commandsResponse, commandsError := http.Get(httpServer.URL + "/api/commands")
	require.NoError(t, commandsError)
	defer commandsResponse.Body.Close()
	require.Equal(t, http.StatusOK, commandsResponse.StatusCode)

	var catalog web.CommandCatalog
	require.NoError(t, json.NewDecoder(commandsResponse.Body).Decode(&catalog))
	require.NotEmpty(t, catalog.Commands)
	require.True(t, catalogContainsCommand(catalog, "gix version"))

	repositoriesResponse, repositoriesError := http.Get(httpServer.URL + "/api/repos")
	require.NoError(t, repositoriesError)
	defer repositoriesResponse.Body.Close()
	require.Equal(t, http.StatusOK, repositoriesResponse.StatusCode)

	var repositories web.RepositoryCatalog
	require.NoError(t, json.NewDecoder(repositoriesResponse.Body).Decode(&repositories))
	require.Equal(t, "/tmp/example", repositories.LaunchPath)
	require.Len(t, repositories.Repositories, 1)
	require.Equal(t, "repo-001", repositories.Repositories[0].ID)
	require.Equal(t, "feature/demo", repositories.Repositories[0].CurrentBranch)

	branchesResponse, branchesError := http.Get(httpServer.URL + "/api/repos/repo-001/branches")
	require.NoError(t, branchesError)
	defer branchesResponse.Body.Close()
	require.Equal(t, http.StatusOK, branchesResponse.StatusCode)

	var branches web.BranchCatalog
	require.NoError(t, json.NewDecoder(branchesResponse.Body).Decode(&branches))
	require.Equal(t, "repo-001", branches.RepositoryID)
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

func withWorkingDirectory(testingInstance *testing.T, workingDirectory string, callback func()) {
	testingInstance.Helper()

	previousWorkingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testingInstance, workingDirectoryError)
	require.NoError(testingInstance, os.Chdir(workingDirectory))
	defer func() {
		require.NoError(testingInstance, os.Chdir(previousWorkingDirectory))
	}()

	callback()
}

func createTestRepository(testingInstance *testing.T, repositoryPath string) string {
	testingInstance.Helper()

	require.NoError(testingInstance, os.MkdirAll(repositoryPath, 0o755))
	runGitCommand(testingInstance, "", "init", "-b", "master", repositoryPath)
	require.NoError(testingInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("initial\n"), 0o644))
	runGitCommand(testingInstance, repositoryPath, "add", "README.md")
	runGitCommand(testingInstance, repositoryPath, "commit", "-m", "initial commit")
	return repositoryPath
}

func createTestBranch(testingInstance *testing.T, repositoryPath string, branchName string) {
	testingInstance.Helper()
	runGitCommand(testingInstance, repositoryPath, "checkout", "-b", branchName)
}

func runGitCommand(testingInstance *testing.T, workingDirectory string, arguments ...string) {
	testingInstance.Helper()

	command := exec.Command("git", arguments...)
	command.Dir = workingDirectory
	command.Env = append(
		os.Environ(),
		"GIT_AUTHOR_NAME=gix-test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=gix-test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)

	output, commandError := command.CombinedOutput()
	require.NoError(testingInstance, commandError, string(output))
}

func canonicalPath(testingInstance *testing.T, path string) string {
	testingInstance.Helper()

	resolvedPath, resolveError := filepath.EvalSymlinks(path)
	require.NoError(testingInstance, resolveError)
	return resolvedPath
}
