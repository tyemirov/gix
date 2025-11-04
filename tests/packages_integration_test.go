package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	packagesIntegrationOwnerConstant                    = "integration-org"
	packagesIntegrationPackageConstant                  = "tooling"
	packagesIntegrationTokenEnvNameConstant             = "GITHUB_PACKAGES_TOKEN"
	packagesIntegrationTokenValueConstant               = "packages-token-value"
	packagesIntegrationBaseURLEnvironmentNameConstant   = "GIX_REPO_PACKAGES_PURGE_BASE_URL"
	packagesIntegrationConfigFileNameConstant           = "config.yaml"
	packagesIntegrationConfigTemplateConstant           = "common:\n  log_level: error\noperations:\n  - command: [\"repo\", \"packages\", \"delete\"]\n    with:\n%s      dry_run: %t\n      roots:\n        - %s\nworkflow: []\n"
	packagesIntegrationPackageLineTemplateConstant      = "      package: %s\n"
	packagesIntegrationSubtestNameTemplateConstant      = "%d_%s"
	packagesIntegrationRunSubcommandConstant            = "run"
	packagesIntegrationModulePathConstant               = "."
	packagesIntegrationConfigFlagTemplateConstant       = "--config=%s"
	packagesIntegrationRepoNamespaceCommand             = "repo"
	packagesIntegrationPackagesNamespaceCommand         = "packages"
	packagesIntegrationDeleteActionCommand              = "delete"
	packagesIntegrationCommandTimeout                   = 10 * time.Second
	packagesIntegrationExpectedPageSizeConstant         = 100
	packagesIntegrationTaggedVersionIDConstant          = 101
	packagesIntegrationFirstUntaggedVersionIDConstant   = 202
	packagesIntegrationSecondUntaggedVersionIDConstant  = 303
	packagesIntegrationVersionsResponseTemplateConstant = `[
{"id":%d,"metadata":{"container":{"tags":["stable"]}}},
{"id":%d,"metadata":{"container":{"tags":[]}}},
{"id":%d,"metadata":{"container":{"tags":[]}}}
]`
	packagesIntegrationVersionsPathTemplateConstant  = "/orgs/%s/packages/container/%s/versions"
	packagesIntegrationDeletePathTemplateConstant    = "/orgs/%s/packages/container/%s/versions/%d"
	packagesIntegrationAuthorizationTemplateConstant = "Bearer %s"
	packagesIntegrationGitExecutableConstant         = "git"
	packagesIntegrationOriginRemoteNameConstant      = "origin"
	packagesIntegrationOriginURLTemplateConstant     = "https://github.com/%s/%s.git"
	packagesIntegrationStubExecutableNameConstant    = "gh"
	packagesIntegrationStubScriptTemplateConstant    = "#!/bin/sh\nif [ \"$1\" = \"repo\" ] && [ \"$2\" = \"view\" ]; then\n  cat <<'EOF'\n{\"nameWithOwner\":\"%s/%s\",\"defaultBranchRef\":{\"name\":\"main\"},\"description\":\"\",\"isInOrganization\":true}\nEOF\n  exit 0\nfi\nexit 0\n"
)

type packagesIntegrationListRequest struct {
	path    string
	page    int
	perPage int
}

type packagesIntegrationDeleteRequest struct {
	path      string
	versionID int64
}

type packagesIntegrationServer struct {
	mutex                sync.Mutex
	pageOnePayload       string
	listRequests         []packagesIntegrationListRequest
	deleteRequests       []packagesIntegrationDeleteRequest
	authorizationHeaders []string
}

func newPackagesIntegrationServer(pageOnePayload string) *packagesIntegrationServer {
	return &packagesIntegrationServer{pageOnePayload: pageOnePayload}
}

func (server *packagesIntegrationServer) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
	server.mutex.Lock()
	server.authorizationHeaders = append(server.authorizationHeaders, request.Header.Get("Authorization"))
	server.mutex.Unlock()

	switch request.Method {
	case http.MethodGet:
		pageValue := request.URL.Query().Get("page")
		perPageValue := request.URL.Query().Get("per_page")
		pageNumber, pageParseError := strconv.Atoi(pageValue)
		if pageParseError != nil {
			responseWriter.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(responseWriter, "invalid page: %v", pageParseError)
			return
		}

		perPageNumber, perPageParseError := strconv.Atoi(perPageValue)
		if perPageParseError != nil {
			responseWriter.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(responseWriter, "invalid per_page: %v", perPageParseError)
			return
		}

		listRequest := packagesIntegrationListRequest{
			path:    request.URL.Path,
			page:    pageNumber,
			perPage: perPageNumber,
		}

		server.mutex.Lock()
		server.listRequests = append(server.listRequests, listRequest)
		server.mutex.Unlock()

		responseWriter.Header().Set("Content-Type", "application/json")
		if pageNumber == 1 {
			_, _ = fmt.Fprint(responseWriter, server.pageOnePayload)
			return
		}

		_, _ = fmt.Fprint(responseWriter, "[]")
	case http.MethodDelete:
		pathSegments := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
		if len(pathSegments) == 0 {
			responseWriter.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprint(responseWriter, "missing identifier")
			return
		}

		identifierText := pathSegments[len(pathSegments)-1]
		versionID, parseError := strconv.ParseInt(identifierText, 10, 64)
		if parseError != nil {
			responseWriter.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintf(responseWriter, "invalid version identifier: %v", parseError)
			return
		}

		deleteRequest := packagesIntegrationDeleteRequest{
			path:      request.URL.Path,
			versionID: versionID,
		}

		server.mutex.Lock()
		server.deleteRequests = append(server.deleteRequests, deleteRequest)
		server.mutex.Unlock()

		responseWriter.WriteHeader(http.StatusNoContent)
	default:
		responseWriter.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (server *packagesIntegrationServer) snapshotListRequests() []packagesIntegrationListRequest {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	requests := make([]packagesIntegrationListRequest, len(server.listRequests))
	copy(requests, server.listRequests)
	return requests
}

func (server *packagesIntegrationServer) snapshotDeleteRequests() []packagesIntegrationDeleteRequest {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	requests := make([]packagesIntegrationDeleteRequest, len(server.deleteRequests))
	copy(requests, server.deleteRequests)
	return requests
}

func (server *packagesIntegrationServer) snapshotAuthorizationHeaders() []string {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	headers := make([]string, len(server.authorizationHeaders))
	copy(headers, server.authorizationHeaders)
	return headers
}

func TestPackagesCommandIntegration(testInstance *testing.T) {
	workingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)
	repositoryRoot := filepath.Dir(workingDirectory)

	pageOnePayload := fmt.Sprintf(
		packagesIntegrationVersionsResponseTemplateConstant,
		packagesIntegrationTaggedVersionIDConstant,
		packagesIntegrationFirstUntaggedVersionIDConstant,
		packagesIntegrationSecondUntaggedVersionIDConstant,
	)

	testCases := []struct {
		name              string
		dryRun            bool
		packageOverride   string
		expectedDeleteIDs []int64
	}{
		{
			name:              "purge_deletes_untagged_versions",
			dryRun:            false,
			packageOverride:   packagesIntegrationPackageConstant,
			expectedDeleteIDs: []int64{packagesIntegrationFirstUntaggedVersionIDConstant, packagesIntegrationSecondUntaggedVersionIDConstant},
		},
		{
			name:              "dry_run_skips_deletion_with_derived_package",
			dryRun:            true,
			packageOverride:   "",
			expectedDeleteIDs: nil,
		},
	}

	for testCaseIndex, testCase := range testCases {
		subtestName := fmt.Sprintf(packagesIntegrationSubtestNameTemplateConstant, testCaseIndex, testCase.name)
		testInstance.Run(subtestName, func(subtest *testing.T) {
			serverState := newPackagesIntegrationServer(pageOnePayload)
			server := httptest.NewServer(serverState)
			defer server.Close()

			repositoryName := filepath.Base(repositoryRoot)
			cleanupRemote := configurePackagesIntegrationRemote(subtest, repositoryRoot, packagesIntegrationOwnerConstant, repositoryName)
			defer cleanupRemote()

			stubDirectory := createPackagesIntegrationStub(subtest, packagesIntegrationOwnerConstant, repositoryName)

			configDirectory := subtest.TempDir()
			configPath := filepath.Join(configDirectory, packagesIntegrationConfigFileNameConstant)
			packageBlock := ""
			expectedPackageName := repositoryName
			trimmedOverride := strings.TrimSpace(testCase.packageOverride)
			if len(trimmedOverride) > 0 {
				packageBlock = fmt.Sprintf(packagesIntegrationPackageLineTemplateConstant, trimmedOverride)
				expectedPackageName = trimmedOverride
			}

			configContent := fmt.Sprintf(
				packagesIntegrationConfigTemplateConstant,
				packageBlock,
				testCase.dryRun,
				repositoryRoot,
			)

			writeError := os.WriteFile(configPath, []byte(configContent), 0o600)
			require.NoError(subtest, writeError)

			subtest.Setenv(packagesIntegrationTokenEnvNameConstant, packagesIntegrationTokenValueConstant)
			subtest.Setenv(packagesIntegrationBaseURLEnvironmentNameConstant, server.URL)

			arguments := []string{
				packagesIntegrationRunSubcommandConstant,
				packagesIntegrationModulePathConstant,
				fmt.Sprintf(packagesIntegrationConfigFlagTemplateConstant, configPath),
				packagesIntegrationRepoNamespaceCommand,
				packagesIntegrationPackagesNamespaceCommand,
				packagesIntegrationDeleteActionCommand,
			}

			pathVariable := os.Getenv("PATH")
			extendedPath := fmt.Sprintf("%s%c%s", stubDirectory, os.PathListSeparator, pathVariable)
			commandOptions := integrationCommandOptions{PathVariable: extendedPath}
			_ = runIntegrationCommand(subtest, repositoryRoot, commandOptions, packagesIntegrationCommandTimeout, arguments)

			listRequests := serverState.snapshotListRequests()
			require.GreaterOrEqual(subtest, len(listRequests), 2)

			expectedVersionsPath := fmt.Sprintf(
				packagesIntegrationVersionsPathTemplateConstant,
				packagesIntegrationOwnerConstant,
				expectedPackageName,
			)

			require.Equal(subtest, expectedVersionsPath, listRequests[0].path)
			require.Equal(subtest, 1, listRequests[0].page)
			require.Equal(subtest, packagesIntegrationExpectedPageSizeConstant, listRequests[0].perPage)

			require.Equal(subtest, expectedVersionsPath, listRequests[1].path)
			require.Equal(subtest, 2, listRequests[1].page)
			require.Equal(subtest, packagesIntegrationExpectedPageSizeConstant, listRequests[1].perPage)

			deleteRequests := serverState.snapshotDeleteRequests()
			if len(testCase.expectedDeleteIDs) == 0 {
				require.Empty(subtest, deleteRequests)
			} else {
				require.GreaterOrEqual(subtest, len(deleteRequests), len(testCase.expectedDeleteIDs))
				for deleteIndex, expectedIdentifier := range testCase.expectedDeleteIDs {
					deleteRequest := deleteRequests[deleteIndex]
					require.Equal(subtest, expectedIdentifier, deleteRequest.versionID)
					expectedDeletePath := fmt.Sprintf(
						packagesIntegrationDeletePathTemplateConstant,
						packagesIntegrationOwnerConstant,
						expectedPackageName,
						expectedIdentifier,
					)
					require.Equal(subtest, expectedDeletePath, deleteRequest.path)
				}
			}

			authorizationHeaders := serverState.snapshotAuthorizationHeaders()
			expectedAuthorization := fmt.Sprintf(packagesIntegrationAuthorizationTemplateConstant, packagesIntegrationTokenValueConstant)
			expectedHeaderCount := len(listRequests) + len(deleteRequests)
			require.Len(subtest, authorizationHeaders, expectedHeaderCount)
			for _, headerValue := range authorizationHeaders {
				require.Equal(subtest, expectedAuthorization, headerValue)
			}
		})
	}
}

func configurePackagesIntegrationRemote(testInstance *testing.T, repositoryRoot string, owner string, repositoryName string) func() {
	testInstance.Helper()

	remoteURL := fmt.Sprintf(packagesIntegrationOriginURLTemplateConstant, owner, repositoryName)

	getURLCommand := exec.Command(packagesIntegrationGitExecutableConstant, "-C", repositoryRoot, "remote", "get-url", packagesIntegrationOriginRemoteNameConstant)
	getURLCommand.Env = buildGitCommandEnvironment(nil)
	outputBytes, getURLError := getURLCommand.CombinedOutput()
	remoteExists := getURLError == nil
	originalURL := strings.TrimSpace(string(outputBytes))

	var configureCommand *exec.Cmd
	if remoteExists {
		configureCommand = exec.Command(packagesIntegrationGitExecutableConstant, "-C", repositoryRoot, "remote", "set-url", packagesIntegrationOriginRemoteNameConstant, remoteURL)
	} else {
		configureCommand = exec.Command(packagesIntegrationGitExecutableConstant, "-C", repositoryRoot, "remote", "add", packagesIntegrationOriginRemoteNameConstant, remoteURL)
	}
	configureCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, configureCommand.Run())

	return func() {
		var cleanupCommand *exec.Cmd
		if remoteExists {
			cleanupCommand = exec.Command(packagesIntegrationGitExecutableConstant, "-C", repositoryRoot, "remote", "set-url", packagesIntegrationOriginRemoteNameConstant, originalURL)
		} else {
			cleanupCommand = exec.Command(packagesIntegrationGitExecutableConstant, "-C", repositoryRoot, "remote", "remove", packagesIntegrationOriginRemoteNameConstant)
		}
		cleanupCommand.Env = buildGitCommandEnvironment(nil)
		require.NoError(testInstance, cleanupCommand.Run())
	}
}

func createPackagesIntegrationStub(testInstance *testing.T, owner string, repositoryName string) string {
	testInstance.Helper()

	stubDirectory := testInstance.TempDir()
	stubPath := filepath.Join(stubDirectory, packagesIntegrationStubExecutableNameConstant)
	scriptContent := fmt.Sprintf(packagesIntegrationStubScriptTemplateConstant, owner, repositoryName)
	require.NoError(testInstance, os.WriteFile(stubPath, []byte(scriptContent), 0o755))
	return stubDirectory
}
