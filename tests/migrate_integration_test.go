package tests

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
	migrate "github.com/tyemirov/gix/internal/migrate"
)

const (
	integrationRepositoryIdentifierConstant   = "integration/test"
	integrationWorkflowDirectoryConstant      = ".github/workflows"
	integrationWorkflowFileNameConstant       = "ci.yml"
	integrationWorkflowInitialContent         = "on:\n  push:\n    branches:\n      - main\n"
	integrationWorkflowCommitMessageConstant  = "CI: switch workflow branch filters to master"
	migrationRetainBranchCaseNameConstant     = "retain_source_branch_with_open_pr"
	migrationDeleteBranchCaseNameConstant     = "delete_source_branch_when_safe"
	migrationSkipDeletionCaseNameConstant     = "skip_deletion_when_not_safe"
	migrationSubtestNameTemplateConstant      = "%d_%s"
	gitExecutableNameConstant                 = "git"
	gitDirOptionConstant                      = "--git-dir"
	gitSymbolicRefCommandConstant             = "symbolic-ref"
	gitHeadReferenceNameConstant              = "HEAD"
	gitReferenceHeadsPrefixConstant           = "refs/heads"
	symbolicRefMissingRemotePathErrorConstant = "remote git directory path must be configured for symbolic-ref operation"
	symbolicRefFailureErrorTemplateConstant   = "failed to update remote HEAD symbolic reference: %w"
	wrappedErrorWithOutputTemplateConstant    = "%w: %s"
)

type recordingGitHubOperations struct {
	pagesStatus             githubcli.PagesStatus
	updatedPagesConfig      *githubcli.PagesConfiguration
	defaultBranchTarget     string
	pullRequests            []githubcli.PullRequest
	retargetedPullRequests  []int
	branchProtectionEnabled bool
	remoteGitDirectoryPath  string
	currentDefaultBranch    string
}

func (operations *recordingGitHubOperations) GetPagesConfig(_ context.Context, repository string) (githubcli.PagesStatus, error) {
	_ = repository
	return operations.pagesStatus, nil
}

func (operations *recordingGitHubOperations) UpdatePagesConfig(_ context.Context, repository string, configuration githubcli.PagesConfiguration) error {
	_ = repository
	operations.updatedPagesConfig = &configuration
	return nil
}

func (operations *recordingGitHubOperations) ListPullRequests(_ context.Context, repository string, options githubcli.PullRequestListOptions) ([]githubcli.PullRequest, error) {
	_ = repository
	_ = options
	return operations.pullRequests, nil
}

func (operations *recordingGitHubOperations) UpdatePullRequestBase(_ context.Context, repository string, pullRequestNumber int, baseBranch string) error {
	_ = repository
	_ = baseBranch
	operations.retargetedPullRequests = append(operations.retargetedPullRequests, pullRequestNumber)
	return nil
}

func (operations *recordingGitHubOperations) SetDefaultBranch(_ context.Context, repository string, branchName string) error {
	_ = repository
	operations.defaultBranchTarget = branchName
	operations.currentDefaultBranch = branchName
	if len(operations.remoteGitDirectoryPath) == 0 {
		return errors.New(symbolicRefMissingRemotePathErrorConstant)
	}

	branchReference := fmt.Sprintf("%s/%s", gitReferenceHeadsPrefixConstant, branchName)
	symbolicRefCommand := exec.Command(
		gitExecutableNameConstant,
		gitDirOptionConstant,
		operations.remoteGitDirectoryPath,
		gitSymbolicRefCommandConstant,
		gitHeadReferenceNameConstant,
		branchReference,
	)
	symbolicRefCommand.Env = buildGitCommandEnvironment(nil)
	symbolicRefOutput, symbolicRefError := symbolicRefCommand.CombinedOutput()
	if symbolicRefError != nil {
		trimmedOutput := strings.TrimSpace(string(symbolicRefOutput))
		if len(trimmedOutput) > 0 {
			symbolicRefError = fmt.Errorf(wrappedErrorWithOutputTemplateConstant, symbolicRefError, trimmedOutput)
		}
		return fmt.Errorf(symbolicRefFailureErrorTemplateConstant, symbolicRefError)
	}
	return nil
}

func (operations *recordingGitHubOperations) ResolveRepoMetadata(_ context.Context, repository string) (githubcli.RepositoryMetadata, error) {
	_ = repository
	defaultBranch := strings.TrimSpace(operations.currentDefaultBranch)
	if len(defaultBranch) == 0 {
		defaultBranch = "main"
	}
	return githubcli.RepositoryMetadata{NameWithOwner: repository, DefaultBranch: defaultBranch}, nil
}

func (operations *recordingGitHubOperations) CheckBranchProtection(_ context.Context, repository string, branchName string) (bool, error) {
	_ = repository
	_ = branchName
	return operations.branchProtectionEnabled, nil
}

func TestMigrationIntegration(testInstance *testing.T) {
	testCases := []struct {
		name                    string
		pullRequests            []githubcli.PullRequest
		branchProtectionEnabled bool
		deleteSourceBranch      bool
		expectSafe              bool
		expectedBlockingReasons []string
		expectedRetargeted      []int
		expectLocalBranch       bool
		expectRemoteBranch      bool
	}{
		{
			name:                    migrationRetainBranchCaseNameConstant,
			pullRequests:            []githubcli.PullRequest{{Number: 12}},
			branchProtectionEnabled: false,
			deleteSourceBranch:      false,
			expectSafe:              false,
			expectedBlockingReasons: []string{"open pull requests still target source branch"},
			expectedRetargeted:      []int{12},
			expectLocalBranch:       true,
			expectRemoteBranch:      true,
		},
		{
			name:                    migrationDeleteBranchCaseNameConstant,
			pullRequests:            nil,
			branchProtectionEnabled: false,
			deleteSourceBranch:      true,
			expectSafe:              true,
			expectedBlockingReasons: nil,
			expectedRetargeted:      nil,
			expectLocalBranch:       false,
			expectRemoteBranch:      false,
		},
		{
			name:                    migrationSkipDeletionCaseNameConstant,
			pullRequests:            []githubcli.PullRequest{{Number: 34}},
			branchProtectionEnabled: false,
			deleteSourceBranch:      true,
			expectSafe:              false,
			expectedBlockingReasons: []string{"open pull requests still target source branch"},
			expectedRetargeted:      []int{34},
			expectLocalBranch:       true,
			expectRemoteBranch:      true,
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		subtestName := fmt.Sprintf(migrationSubtestNameTemplateConstant, testCaseIndex, testCase.name)
		testInstance.Run(subtestName, func(subtest *testing.T) {
			testRoot := subtest.TempDir()
			repositoryDirectory := filepath.Join(testRoot, "repository")
			require.NoError(subtest, os.MkdirAll(repositoryDirectory, 0o755))

			remoteDirectory := filepath.Join(testRoot, "remote.git")
			initializeBareGitRepository(subtest, remoteDirectory)
			remotePath, remotePathError := filepath.Abs(remoteDirectory)
			require.NoError(subtest, remotePathError)

			runMigrationGitCommand(subtest, repositoryDirectory, "init")
			runMigrationGitCommand(subtest, repositoryDirectory, "config", "user.name", "Integration User")
			runMigrationGitCommand(subtest, repositoryDirectory, "config", "user.email", "integration@example.com")
			runMigrationGitCommand(subtest, repositoryDirectory, "checkout", "-b", "main")

			readmePath := filepath.Join(repositoryDirectory, "README.md")
			require.NoError(subtest, os.WriteFile(readmePath, []byte("hello\n"), 0o644))
			runMigrationGitCommand(subtest, repositoryDirectory, "add", "README.md")
			runMigrationGitCommand(subtest, repositoryDirectory, "commit", "-m", "initial commit")

			runMigrationGitCommand(subtest, repositoryDirectory, "branch", "master")
			runMigrationGitCommand(subtest, repositoryDirectory, "checkout", "master")

			workflowsDirectory := filepath.Join(repositoryDirectory, integrationWorkflowDirectoryConstant)
			require.NoError(subtest, os.MkdirAll(workflowsDirectory, 0o755))
			workflowPath := filepath.Join(workflowsDirectory, integrationWorkflowFileNameConstant)
			require.NoError(subtest, os.WriteFile(workflowPath, []byte(integrationWorkflowInitialContent), 0o644))
			runMigrationGitCommand(subtest, repositoryDirectory, "add", integrationWorkflowDirectoryConstant)
			runMigrationGitCommand(subtest, repositoryDirectory, "commit", "-m", "add workflow")

			runMigrationGitCommand(subtest, repositoryDirectory, "remote", "add", "origin", remotePath)
			runMigrationGitCommand(subtest, repositoryDirectory, "push", "origin", "main:main")

			logger := zap.NewNop()
			commandRunner := execshell.NewOSCommandRunner()
			executor, creationError := execshell.NewShellExecutor(logger, commandRunner, false)
			require.NoError(subtest, creationError)
			repositoryManager, managerError := gitrepo.NewRepositoryManager(executor)
			require.NoError(subtest, managerError)

			githubOperations := &recordingGitHubOperations{
				pagesStatus: githubcli.PagesStatus{
					Enabled:      true,
					BuildType:    githubcli.PagesBuildTypeLegacy,
					SourceBranch: "main",
					SourcePath:   "/docs",
				},
				pullRequests:            append([]githubcli.PullRequest{}, testCase.pullRequests...),
				branchProtectionEnabled: testCase.branchProtectionEnabled,
				remoteGitDirectoryPath:  remoteDirectory,
			}

			service, serviceError := migrate.NewService(migrate.ServiceDependencies{
				Logger:            logger,
				RepositoryManager: repositoryManager,
				GitHubClient:      githubOperations,
				GitExecutor:       executor,
			})
			require.NoError(subtest, serviceError)

			options := migrate.MigrationOptions{
				RepositoryPath:       repositoryDirectory,
				RepositoryRemoteName: "origin",
				RepositoryIdentifier: integrationRepositoryIdentifierConstant,
				WorkflowsDirectory:   integrationWorkflowDirectoryConstant,
				SourceBranch:         migrate.BranchMain,
				TargetBranch:         migrate.BranchMaster,
				PushUpdates:          false,
				DeleteSourceBranch:   testCase.deleteSourceBranch,
			}

			result, migrationError := service.Execute(context.Background(), options)
			require.NoError(subtest, migrationError)

			require.Len(subtest, result.WorkflowOutcome.UpdatedFiles, 1)
			require.Contains(subtest, result.WorkflowOutcome.UpdatedFiles[0], integrationWorkflowFileNameConstant)
			require.True(subtest, result.PagesConfigurationUpdated)
			require.True(subtest, result.DefaultBranchUpdated)
			require.ElementsMatch(subtest, testCase.expectedRetargeted, result.RetargetedPullRequests)
			require.Equal(subtest, testCase.expectSafe, result.SafetyStatus.SafeToDelete)
			if len(testCase.expectedBlockingReasons) > 0 {
				for _, expectedReason := range testCase.expectedBlockingReasons {
					require.Contains(subtest, result.SafetyStatus.BlockingReasons, expectedReason)
				}
			} else {
				require.Empty(subtest, result.SafetyStatus.BlockingReasons)
			}

			contentBytes, readError := os.ReadFile(workflowPath)
			require.NoError(subtest, readError)
			require.Contains(subtest, string(contentBytes), "- master")

			logOutput := runMigrationGitCommand(subtest, repositoryDirectory, "log", "-1", "--pretty=%s")
			require.Equal(subtest, integrationWorkflowCommitMessageConstant, logOutput)

			statusOutput := runMigrationGitCommand(subtest, repositoryDirectory, "status", "--porcelain")
			require.Equal(subtest, "", statusOutput)

			require.NotNil(subtest, githubOperations.updatedPagesConfig)
			require.Equal(subtest, string(migrate.BranchMaster), githubOperations.updatedPagesConfig.SourceBranch)
			require.ElementsMatch(subtest, testCase.expectedRetargeted, githubOperations.retargetedPullRequests)
			require.Equal(subtest, string(migrate.BranchMaster), githubOperations.defaultBranchTarget)

			require.Equal(subtest, testCase.expectLocalBranch, branchExists(subtest, repositoryDirectory, "main"))
			require.Equal(subtest, testCase.expectRemoteBranch, remoteBranchExists(subtest, remotePath, "main"))
		})
	}
}

func runMigrationGitCommand(testInstance *testing.T, repositoryPath string, arguments ...string) string {
	testInstance.Helper()
	command := exec.Command(gitExecutableNameConstant, arguments...)
	command.Dir = repositoryPath
	command.Env = buildGitCommandEnvironment(nil)
	outputBytes, commandError := command.CombinedOutput()
	require.NoError(testInstance, commandError, string(outputBytes))
	return string(bytes.TrimSpace(outputBytes))
}

func branchExists(testInstance *testing.T, repositoryPath string, branchName string) bool {
	testInstance.Helper()
	output := runMigrationGitCommand(testInstance, repositoryPath, "branch", "--list", branchName)
	return len(strings.TrimSpace(output)) > 0
}

func remoteBranchExists(testInstance *testing.T, remoteGitDirectory string, branchName string) bool {
	testInstance.Helper()
	command := exec.Command(gitExecutableNameConstant, gitDirOptionConstant, remoteGitDirectory, "branch", "--list", branchName)
	command.Env = buildGitCommandEnvironment(nil)
	outputBytes, commandError := command.CombinedOutput()
	require.NoError(testInstance, commandError, string(outputBytes))
	return len(strings.TrimSpace(string(bytes.TrimSpace(outputBytes)))) > 0
}

func initializeBareGitRepository(testInstance *testing.T, repositoryPath string) {
	testInstance.Helper()
	command := exec.Command(gitExecutableNameConstant, "init", "--bare", repositoryPath)
	command.Env = buildGitCommandEnvironment(nil)
	outputBytes, commandError := command.CombinedOutput()
	require.NoError(testInstance, commandError, string(outputBytes))
}
