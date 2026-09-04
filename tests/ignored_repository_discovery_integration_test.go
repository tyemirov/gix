package tests

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuditAndWorkflowIgnoreRepositoriesWithinIgnoredCheckout(testInstance *testing.T) {
	workingDirectory, directoryError := os.Getwd()
	require.NoError(testInstance, directoryError)
	repositoryRoot := filepath.Dir(workingDirectory)
	binaryPath := buildIntegrationBinary(testInstance, repositoryRoot)
	stubPath := buildStubbedExecutablePath(testInstance, auditIntegrationStubExecutableName, auditIntegrationStubScript)

	for _, dependencyKind := range []string{"repository", "submodule"} {
		testInstance.Run(dependencyKind, func(subtest *testing.T) {
			fixtureRoot := subtest.TempDir()
			applicationPath := createGitRepository(subtest, gitRepositoryOptions{
				Path:      filepath.Join(fixtureRoot, "application"),
				RemoteURL: auditIntegrationOriginURL,
			})
			writeFile(subtest, filepath.Join(applicationPath, ".gitignore"), "build/cache/\n")
			ignoredPath := createGitRepository(subtest, gitRepositoryOptions{
				Path:      filepath.Join(applicationPath, "build", "cache"),
				RemoteURL: auditIntegrationOriginURL,
			})
			dependencyPath := filepath.Join(ignoredPath, "libs", "ignored-dependency")
			if dependencyKind == "submodule" {
				dependencySource := createGitRepository(subtest, gitRepositoryOptions{
					Path: filepath.Join(fixtureRoot, "dependency-source"),
				})
				configureGitIdentity(subtest, dependencySource)
				writeFile(subtest, filepath.Join(dependencySource, "README.md"), "Dependency fixture.\n")
				runGit(subtest, dependencySource, "add", "README.md")
				runGit(subtest, dependencySource, "commit", "-m", "Initialize dependency fixture")
				runGit(subtest, ignoredPath, "-c", "protocol.file.allow=always", "submodule", "add", dependencySource, "libs/ignored-dependency")
				runGit(subtest, dependencyPath, "remote", "set-url", "origin", auditIntegrationOriginURL)
			} else {
				createGitRepository(subtest, gitRepositoryOptions{
					Path:      dependencyPath,
					RemoteURL: auditIntegrationOriginURL,
				})
			}
			createGitRepository(subtest, gitRepositoryOptions{
				Path:      filepath.Join(dependencyPath, "ignored-grandchild"),
				RemoteURL: auditIntegrationOriginURL,
			})
			visiblePath := createGitRepository(subtest, gitRepositoryOptions{
				Path:      filepath.Join(applicationPath, "build", "cache-visible"),
				RemoteURL: auditIntegrationOriginURL,
			})
			createGitRepository(subtest, gitRepositoryOptions{
				Path:      filepath.Join(visiblePath, "visible-dependency"),
				RemoteURL: auditIntegrationOriginURL,
			})
			workflowPath := filepath.Join(fixtureRoot, "audit.yaml")
			writeFile(subtest, workflowPath, "workflow:\n  - step:\n      command: ['audit', 'report']\n      with:\n        format: csv\n")

			commands := map[string][]string{
				"audit":    {"audit", "--roots", applicationPath, "--format", "csv"},
				"workflow": {"workflow", workflowPath, "--roots", applicationPath},
			}
			for commandName, arguments := range commands {
				subtest.Run(commandName, func(commandTest *testing.T) {
					output, commandError := runBinaryIntegrationCommand(
						commandTest, binaryPath, repositoryRoot,
						map[string]string{pathEnvironmentVariableNameConstant: stubPath},
						auditIntegrationTimeout,
						append([]string{"--log-level", "error"}, arguments...),
					)
					require.NoError(commandTest, commandError, output)
					filteredOutput := filterStructuredOutput(output)
					csvStart := strings.Index(filteredOutput, auditIntegrationCSVHeaderConstant)
					require.NotEqual(commandTest, -1, csvStart, filteredOutput)
					reader := csv.NewReader(strings.NewReader(filteredOutput[csvStart:]))
					records, parseError := reader.ReadAll()
					require.NoError(commandTest, parseError, filteredOutput)
					var folders []string
					for _, record := range records[1:] {
						folders = append(folders, record[0])
					}
					require.ElementsMatch(commandTest, []string{
						"application",
						"build/cache-visible",
						"build/cache-visible/visible-dependency",
					}, folders)
				})
			}
		})
	}
}
