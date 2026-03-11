package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/web"
	"github.com/tyemirov/gix/internal/workflow"
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
		Arguments:     []string{"--web", "--port", "18080"},
		Context:       context.Background(),
		ExitOnVersion: false,
	})

	require.NoError(t, executionError)
	require.Equal(t, "127.0.0.1:18080", capturedAddress)
}

func TestExecuteWithOptionsRejectsLegacyWebPositionalPort(t *testing.T) {
	application := NewApplication()

	executionError := application.ExecuteWithOptions(ExecutionOptions{
		Arguments:     []string{"--web", "18080"},
		Context:       context.Background(),
		ExitOnVersion: false,
	})

	require.EqualError(t, executionError, webPositionalArgumentsRequirePortFlagConstant)
}

func TestExecuteWithOptionsLaunchesWebRunnerWithBindAndPortFlags(t *testing.T) {
	application := NewApplication()

	capturedAddress := ""
	application.webRunner = func(_ context.Context, options web.ServerOptions) error {
		capturedAddress = options.Address
		return nil
	}

	var standardOutput bytes.Buffer
	executionError := application.ExecuteWithOptions(ExecutionOptions{
		Arguments:      []string{"--web", "--bind", "0.0.0.0", "--port", "8081"},
		Context:        context.Background(),
		StandardOutput: &standardOutput,
		ExitOnVersion:  false,
	})

	require.NoError(t, executionError)
	require.Equal(t, "0.0.0.0:8081", capturedAddress)
	require.Contains(t, standardOutput.String(), "http://0.0.0.0:8081")
}

func TestExecuteWithOptionsLaunchesWebRunnerWithExplicitRoots(t *testing.T) {
	rootPath := t.TempDir()
	targetRootPath := filepath.Join(rootPath, "fleet")
	firstRepositoryPath := createTestRepository(t, filepath.Join(targetRootPath, "alpha"))
	secondRepositoryPath := createTestRepository(t, filepath.Join(targetRootPath, "nested", "beta"))
	createTestRepository(t, filepath.Join(rootPath, "outside", "ignored"))

	capturedCatalog := web.RepositoryCatalog{}
	withWorkingDirectory(t, rootPath, func() {
		application := NewApplication()
		application.webRunner = func(_ context.Context, options web.ServerOptions) error {
			capturedCatalog = options.Repositories
			return nil
		}

		executionError := application.ExecuteWithOptions(ExecutionOptions{
			Arguments:     []string{"--web", "--roots", targetRootPath},
			Context:       context.Background(),
			ExitOnVersion: false,
		})

		require.NoError(t, executionError)
	})

	require.Equal(t, webLaunchModeConfiguredRootsConstant, capturedCatalog.LaunchMode)
	require.Equal(t, canonicalPath(t, targetRootPath), canonicalPath(t, capturedCatalog.LaunchPath))
	require.Len(t, capturedCatalog.LaunchRoots, 1)
	require.Equal(t, []string{canonicalPath(t, targetRootPath)}, []string{canonicalPath(t, capturedCatalog.LaunchRoots[0])})
	require.Equal(t, canonicalPath(t, targetRootPath), canonicalPath(t, capturedCatalog.ExplorerRoot))
	require.Len(t, capturedCatalog.Repositories, 2)
	require.Equal(t, canonicalPath(t, firstRepositoryPath), canonicalPath(t, capturedCatalog.Repositories[0].Path))
	require.Equal(t, canonicalPath(t, secondRepositoryPath), canonicalPath(t, capturedCatalog.Repositories[1].Path))
}

func TestExecuteWithOptionsLaunchesWebRunnerWithRelativeExplicitRoots(t *testing.T) {
	rootPath := t.TempDir()
	targetRootPath := filepath.Join(rootPath, "fleet")
	currentRepositoryPath := createTestRepository(t, filepath.Join(targetRootPath, "gix"))
	siblingRepositoryPath := createTestRepository(t, filepath.Join(targetRootPath, "alpha"))
	nestedWorkingDirectory := filepath.Join(currentRepositoryPath, "internal")
	require.NoError(t, os.MkdirAll(nestedWorkingDirectory, 0o755))

	capturedCatalog := web.RepositoryCatalog{}
	withWorkingDirectory(t, nestedWorkingDirectory, func() {
		application := NewApplication()
		application.webRunner = func(_ context.Context, options web.ServerOptions) error {
			capturedCatalog = options.Repositories
			return nil
		}

		executionError := application.ExecuteWithOptions(ExecutionOptions{
			Arguments:     []string{"--web", "--roots", "../.."},
			Context:       context.Background(),
			ExitOnVersion: false,
		})

		require.NoError(t, executionError)
	})

	require.Equal(t, webLaunchModeConfiguredRootsConstant, capturedCatalog.LaunchMode)
	require.Equal(t, canonicalPath(t, targetRootPath), canonicalPath(t, capturedCatalog.LaunchPath))
	require.Len(t, capturedCatalog.LaunchRoots, 1)
	require.Equal(t, canonicalPath(t, targetRootPath), canonicalPath(t, capturedCatalog.LaunchRoots[0]))
	require.Len(t, capturedCatalog.Repositories, 2)
	require.Equal(t, canonicalPath(t, siblingRepositoryPath), canonicalPath(t, capturedCatalog.Repositories[0].Path))
	require.Equal(t, canonicalPath(t, currentRepositoryPath), canonicalPath(t, capturedCatalog.Repositories[1].Path))
	require.False(t, capturedCatalog.Repositories[0].ContextCurrent)
	require.False(t, capturedCatalog.Repositories[1].ContextCurrent)
	require.Empty(t, capturedCatalog.SelectedRepositoryID)
}

func TestExecuteWithOptionsRejectsWebNetworkFlagsWithoutWeb(t *testing.T) {
	application := NewApplication()

	executionError := application.ExecuteWithOptions(ExecutionOptions{
		Arguments:     []string{"--bind", "0.0.0.0", "--port", "8081"},
		Context:       context.Background(),
		ExitOnVersion: false,
	})

	require.EqualError(t, executionError, webNetworkFlagsRequireWebConstant)
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
	rootPath := t.TempDir()
	repositoryPath := createTestRepository(t, filepath.Join(rootPath, "workspace", "example"))
	nestedRepositoryPath := createTestRepository(t, filepath.Join(repositoryPath, "plugins", "other"))
	createTestBranch(t, repositoryPath, "feature/demo")

	nestedWorkingDirectory := filepath.Join(repositoryPath, "internal")
	require.NoError(t, os.MkdirAll(nestedWorkingDirectory, 0o755))
	withWorkingDirectory(t, nestedWorkingDirectory, func() {
		application := NewApplication()
		catalog := application.repositoryCatalog(context.Background(), nil)

		require.Equal(t, "current_repo", catalog.LaunchMode)
		require.Equal(t, canonicalPath(t, nestedWorkingDirectory), canonicalPath(t, catalog.LaunchPath))
		require.Equal(t, canonicalPath(t, repositoryPath), canonicalPath(t, catalog.ExplorerRoot))
		require.Len(t, catalog.Repositories, 2)
		require.Equal(t, canonicalPath(t, repositoryPath), canonicalPath(t, catalog.Repositories[0].Path))
		require.Equal(t, canonicalPath(t, nestedRepositoryPath), canonicalPath(t, catalog.Repositories[1].Path))
		require.Equal(t, "feature/demo", catalog.Repositories[0].CurrentBranch)
		require.True(t, catalog.Repositories[0].ContextCurrent)
		require.False(t, catalog.Repositories[1].ContextCurrent)
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
		catalog := application.repositoryCatalog(context.Background(), nil)

		require.Equal(t, "discovered_repositories", catalog.LaunchMode)
		require.Equal(t, canonicalPath(t, rootPath), canonicalPath(t, catalog.LaunchPath))
		require.Equal(t, canonicalPath(t, rootPath), canonicalPath(t, catalog.ExplorerRoot))
		require.Len(t, catalog.Repositories, 2)
		require.Equal(t, canonicalPath(t, firstRepository), canonicalPath(t, catalog.Repositories[0].Path))
		require.Equal(t, canonicalPath(t, secondRepository), canonicalPath(t, catalog.Repositories[1].Path))
		require.False(t, catalog.Repositories[0].ContextCurrent)
		require.False(t, catalog.Repositories[1].ContextCurrent)
		require.Empty(t, catalog.SelectedRepositoryID)
	})
}

func TestRepositoryCatalogUsesExplicitLaunchRoots(t *testing.T) {
	rootPath := t.TempDir()
	firstRootPath := filepath.Join(rootPath, "fleet", "alpha")
	secondRootPath := filepath.Join(rootPath, "fleet", "beta")
	firstRepositoryPath := createTestRepository(t, filepath.Join(firstRootPath, "example"))
	secondRepositoryPath := createTestRepository(t, filepath.Join(secondRootPath, "other"))
	createTestRepository(t, filepath.Join(rootPath, "ignored", "skip"))
	nestedWorkingDirectory := filepath.Join(secondRepositoryPath, "internal")
	require.NoError(t, os.MkdirAll(nestedWorkingDirectory, 0o755))

	withWorkingDirectory(t, nestedWorkingDirectory, func() {
		application := NewApplication()
		catalog := application.repositoryCatalog(context.Background(), []string{firstRootPath, secondRootPath})

		require.Equal(t, webLaunchModeConfiguredRootsConstant, catalog.LaunchMode)
		require.Equal(t, canonicalPath(t, filepath.Join(rootPath, "fleet")), canonicalPath(t, catalog.LaunchPath))
		require.Len(t, catalog.LaunchRoots, 2)
		require.Equal(t, []string{canonicalPath(t, firstRootPath), canonicalPath(t, secondRootPath)}, []string{
			canonicalPath(t, catalog.LaunchRoots[0]),
			canonicalPath(t, catalog.LaunchRoots[1]),
		})
		require.Equal(t, canonicalPath(t, filepath.Join(rootPath, "fleet")), canonicalPath(t, catalog.ExplorerRoot))
		require.Len(t, catalog.Repositories, 2)
		require.Equal(t, canonicalPath(t, firstRepositoryPath), canonicalPath(t, catalog.Repositories[0].Path))
		require.Equal(t, canonicalPath(t, secondRepositoryPath), canonicalPath(t, catalog.Repositories[1].Path))
		require.False(t, catalog.Repositories[0].ContextCurrent)
		require.False(t, catalog.Repositories[1].ContextCurrent)
		require.Empty(t, catalog.SelectedRepositoryID)
	})
}

func TestWebDirectoryBrowserShowsOnlyRepositoryBranches(t *testing.T) {
	rootPath := t.TempDir()
	containerPath := filepath.Join(rootPath, "Folder C")
	require.NoError(t, os.MkdirAll(filepath.Join(containerPath, "Folder A"), 0o755))
	repositoryPath := createTestRepository(t, filepath.Join(containerPath, "Folder B"))
	createTestBranch(t, repositoryPath, "feature/demo")

	application := NewApplication()
	directoryBrowser := application.newWebDirectoryBrowser()

	type expectedFolder struct {
		name           string
		path           string
		repositoryPath string
		currentBranch  string
	}

	testCases := []struct {
		name            string
		browsePath      string
		expectedFolders []expectedFolder
	}{
		{
			name:       "ancestor folder remains visible when it contains a repository descendant",
			browsePath: rootPath,
			expectedFolders: []expectedFolder{
				{
					name: "Folder C",
					path: containerPath,
				},
			},
		},
		{
			name:       "non repository siblings stay hidden beneath visible ancestor folder",
			browsePath: containerPath,
			expectedFolders: []expectedFolder{
				{
					name:           "Folder B",
					path:           repositoryPath,
					repositoryPath: repositoryPath,
					currentBranch:  "feature/demo",
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			listing := directoryBrowser(context.Background(), testCase.browsePath)

			require.Empty(t, listing.Error)
			require.Equal(t, canonicalPath(t, testCase.browsePath), canonicalPath(t, listing.Path))
			require.Len(t, listing.Folders, len(testCase.expectedFolders))

			for folderIndex, expectedFolder := range testCase.expectedFolders {
				actualFolder := listing.Folders[folderIndex]
				require.Equal(t, expectedFolder.name, actualFolder.Name)
				require.Equal(t, canonicalPath(t, expectedFolder.path), canonicalPath(t, actualFolder.Path))

				if len(expectedFolder.repositoryPath) == 0 {
					require.Nil(t, actualFolder.Repository)
					continue
				}

				require.NotNil(t, actualFolder.Repository)
				require.Equal(t, dynamicWebRepositoryID(expectedFolder.repositoryPath), actualFolder.Repository.ID)
				require.Equal(t, expectedFolder.name, actualFolder.Repository.Name)
				require.Equal(t, canonicalPath(t, expectedFolder.repositoryPath), canonicalPath(t, actualFolder.Repository.Path))
				require.Equal(t, expectedFolder.currentBranch, actualFolder.Repository.CurrentBranch)
				require.False(t, actualFolder.Repository.ContextCurrent)
			}
		})
	}
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

func TestWebServerLoadsBranchesForDynamicallyDiscoveredRepository(t *testing.T) {
	repositoryPath := createTestRepository(t, filepath.Join(t.TempDir(), "workspace", "example"))
	createTestBranch(t, repositoryPath, "feature/demo")

	application := NewApplication()
	server, serverError := web.NewServer(web.ServerOptions{
		Address:           "127.0.0.1:8080",
		Repositories:      web.RepositoryCatalog{},
		Catalog:           application.commandCatalog(),
		LoadBranches:      application.loadRepositoryBranches,
		BrowseDirectories: application.newWebDirectoryBrowser(),
		Execute:           application.newWebCommandExecutor(),
		InspectAudit: func(_ context.Context, _ web.AuditInspectionRequest) web.AuditInspectionResponse {
			return web.AuditInspectionResponse{}
		},
		ApplyAuditChanges: func(_ context.Context, _ web.AuditChangeApplyRequest) web.AuditChangeApplyResponse {
			return web.AuditChangeApplyResponse{}
		},
		LoadWorkflowPrimitives: application.newWebWorkflowPrimitiveCatalogLoader(),
		ApplyWorkflowActions: func(_ context.Context, request web.WorkflowPrimitiveApplyRequest) web.WorkflowPrimitiveApplyResponse {
			return web.WorkflowPrimitiveApplyResponse{
				Results: []web.WorkflowPrimitiveApplyResult{
					{
						ID:             request.Actions[0].ID,
						RepositoryPath: request.Actions[0].RepositoryPath,
						PrimitiveID:    request.Actions[0].PrimitiveID,
						Status:         webAuditChangeStatusSucceededConstant,
						Message:        "Applied Replace in files",
					},
				},
			}
		},
	})
	require.NoError(t, serverError)

	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	repositoryID := dynamicWebRepositoryID(repositoryPath)
	branchesResponse, branchesError := http.Get(httpServer.URL + "/api/repos/" + url.PathEscape(repositoryID) + "/branches?path=" + url.QueryEscape(repositoryPath))
	require.NoError(t, branchesError)
	defer branchesResponse.Body.Close()
	require.Equal(t, http.StatusOK, branchesResponse.StatusCode)

	var branchCatalog web.BranchCatalog
	require.NoError(t, json.NewDecoder(branchesResponse.Body).Decode(&branchCatalog))
	require.Empty(t, branchCatalog.Error)
	require.Equal(t, repositoryID, branchCatalog.RepositoryID)
	require.Equal(t, canonicalPath(t, repositoryPath), canonicalPath(t, branchCatalog.RepositoryPath))
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
			ExplorerRoot:         "/tmp/example",
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
		BrowseDirectories: application.newWebDirectoryBrowser(),
		Execute:           application.newWebCommandExecutor(),
		InspectAudit: func(_ context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
			return web.AuditInspectionResponse{
				Roots: request.Roots,
				Rows: []web.AuditInspectionRow{
					{
						Path:                   "/tmp/custom/example",
						FolderName:             "example",
						IsGitRepository:        true,
						FinalGitHubRepository:  "canonical/example",
						OriginRemoteStatus:     "configured",
						NameMatches:            "yes",
						RemoteDefaultBranch:    "main",
						LocalBranch:            "main",
						InSync:                 "yes",
						RemoteProtocol:         "https",
						OriginMatchesCanonical: "yes",
						WorktreeDirty:          "no",
						DirtyFiles:             "",
					},
				},
			}
		},
		ApplyAuditChanges: func(_ context.Context, request web.AuditChangeApplyRequest) web.AuditChangeApplyResponse {
			return web.AuditChangeApplyResponse{
				Results: []web.AuditChangeApplyResult{
					{
						ID:      request.Changes[0].ID,
						Kind:    request.Changes[0].Kind,
						Path:    request.Changes[0].Path,
						Status:  "succeeded",
						Message: "queued change applied",
					},
				},
			}
		},
		LoadWorkflowPrimitives: application.newWebWorkflowPrimitiveCatalogLoader(),
		ApplyWorkflowActions:   application.newWebWorkflowPrimitiveExecutor(),
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
	require.Contains(t, indexDocument.String(), "Audit Workspace")
	require.Contains(t, indexDocument.String(), "id=\"workspace-layout\"")
	require.Contains(t, indexDocument.String(), "id=\"repo-sidebar\"")
	require.Contains(t, indexDocument.String(), "id=\"workspace-main\"")
	require.Contains(t, indexDocument.String(), "<h2>Repos</h2>")
	require.Contains(t, indexDocument.String(), "cdn.jsdelivr.net/npm/wunderbaum@0/dist/wunderbaum.min.css")
	require.Contains(t, indexDocument.String(), "<h3>Scope</h3>")
	require.Contains(t, indexDocument.String(), "<h3>Workflow Actions</h3>")
	require.NotContains(t, indexDocument.String(), "<h3>Refs</h3>")
	require.NotContains(t, indexDocument.String(), "<h3>Paths</h3>")
	require.NotContains(t, indexDocument.String(), "id=\"target-ref-select\"")
	require.NotContains(t, indexDocument.String(), "id=\"target-path-value\"")
	require.Contains(t, indexDocument.String(), "id=\"repo-tree\"")
	require.Contains(t, indexDocument.String(), "id=\"audit-selection-summary\"")
	require.Contains(t, indexDocument.String(), "id=\"audit-roots-input\"")
	require.Contains(t, indexDocument.String(), "Inspect or refresh audit table")
	require.Contains(t, indexDocument.String(), "id=\"audit-results-panel\"")
	require.Contains(t, indexDocument.String(), "id=\"audit-queue-panel\"")
	require.Contains(t, indexDocument.String(), "Pending Changes")
	require.Contains(t, indexDocument.String(), "Queue workflow action")
	require.Contains(t, indexDocument.String(), "Apply queued workflow actions")

	applicationScriptResponse, applicationScriptError := http.Get(httpServer.URL + "/assets/app.js")
	require.NoError(t, applicationScriptError)
	defer applicationScriptResponse.Body.Close()
	require.Equal(t, http.StatusOK, applicationScriptResponse.StatusCode)

	var applicationScript bytes.Buffer
	_, applicationScriptCopyError := applicationScript.ReadFrom(applicationScriptResponse.Body)
	require.NoError(t, applicationScriptCopyError)
	require.Contains(t, applicationScript.String(), "from \"https://cdn.jsdelivr.net/npm/wunderbaum@0/+esm\"")

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
	require.Equal(t, "/tmp/example", repositories.ExplorerRoot)
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

	auditBody := strings.NewReader(`{"roots":["/tmp/custom"],"include_all":true}`)
	auditResponse, auditError := http.Post(httpServer.URL+"/api/audit/inspect", "application/json", auditBody)
	require.NoError(t, auditError)
	defer auditResponse.Body.Close()
	require.Equal(t, http.StatusOK, auditResponse.StatusCode)

	var auditInspection web.AuditInspectionResponse
	require.NoError(t, json.NewDecoder(auditResponse.Body).Decode(&auditInspection))
	require.Equal(t, []string{"/tmp/custom"}, auditInspection.Roots)
	require.Len(t, auditInspection.Rows, 1)
	require.Equal(t, "/tmp/custom/example", auditInspection.Rows[0].Path)
	require.Equal(t, "configured", auditInspection.Rows[0].OriginRemoteStatus)

	applyBody := strings.NewReader(`{"changes":[{"id":"chg-001","kind":"rename_folder","path":"/tmp/custom/example","require_clean":true}]}`)
	applyResponse, applyError := http.Post(httpServer.URL+"/api/audit/apply", "application/json", applyBody)
	require.NoError(t, applyError)
	defer applyResponse.Body.Close()
	require.Equal(t, http.StatusOK, applyResponse.StatusCode)

	var applyInspection web.AuditChangeApplyResponse
	require.NoError(t, json.NewDecoder(applyResponse.Body).Decode(&applyInspection))
	require.Len(t, applyInspection.Results, 1)
	require.Equal(t, "chg-001", applyInspection.Results[0].ID)
	require.Equal(t, "succeeded", applyInspection.Results[0].Status)

	workflowCatalogResponse, workflowCatalogError := http.Get(httpServer.URL + "/api/workflow/primitives")
	require.NoError(t, workflowCatalogError)
	defer workflowCatalogResponse.Body.Close()
	require.Equal(t, http.StatusOK, workflowCatalogResponse.StatusCode)

	var workflowCatalog web.WorkflowPrimitiveCatalog
	require.NoError(t, json.NewDecoder(workflowCatalogResponse.Body).Decode(&workflowCatalog))
	require.NotEmpty(t, workflowCatalog.Primitives)
	require.Equal(t, webWorkflowPrimitiveCanonicalRemoteConstant, workflowCatalog.Primitives[0].ID)

	workflowApplyBody := strings.NewReader(`{"actions":[{"id":"wf-001","repository_path":"/tmp/example","primitive_id":"repo.files.replace","parameters":{"patterns":"README.md","find":"initial","replace":"updated"}}]}`)
	workflowApplyResponse, workflowApplyError := http.Post(httpServer.URL+"/api/workflow/apply", "application/json", workflowApplyBody)
	require.NoError(t, workflowApplyError)
	defer workflowApplyResponse.Body.Close()
	require.Equal(t, http.StatusOK, workflowApplyResponse.StatusCode)

	var workflowApplyInspection web.WorkflowPrimitiveApplyResponse
	require.NoError(t, json.NewDecoder(workflowApplyResponse.Body).Decode(&workflowApplyInspection))
	require.Len(t, workflowApplyInspection.Results, 1)
	require.Equal(t, "wf-001", workflowApplyInspection.Results[0].ID)
	require.Equal(t, webAuditChangeStatusSucceededConstant, workflowApplyInspection.Results[0].Status)

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

func TestWebAuditChangeExecutorRejectsRelativePaths(t *testing.T) {
	application := NewApplication()
	response := application.newWebAuditChangeExecutor()(context.Background(), web.AuditChangeApplyRequest{
		Changes: []web.AuditQueuedChange{
			{
				ID:            "chg-001",
				Kind:          web.AuditChangeKindDeleteFolder,
				Path:          "../sibling",
				ConfirmDelete: true,
			},
		},
	})

	require.Empty(t, response.Error)
	require.Len(t, response.Results, 1)
	require.Equal(t, "failed", response.Results[0].Status)
	require.Contains(t, response.Results[0].Error, "absolute")
}

func TestWebAuditChangeResultStatusMarksSkippedOutcomes(t *testing.T) {
	outcome := workflow.ExecutionOutcome{
		ReporterSummaryData: shared.SummaryData{
			StepOutcomeCounts: map[string]map[string]int{
				"audit-sync": {
					"skipped": 1,
				},
			},
		},
	}

	require.Equal(t, webAuditChangeStatusSkippedConstant, webAuditChangeResultStatus(outcome, nil))
	require.Equal(t, webAuditChangeStatusSucceededConstant, webAuditChangeResultStatus(workflow.ExecutionOutcome{}, nil))
	require.Equal(t, webAuditChangeStatusFailedConstant, webAuditChangeResultStatus(workflow.ExecutionOutcome{}, errors.New("boom")))
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
