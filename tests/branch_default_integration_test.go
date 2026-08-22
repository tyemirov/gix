package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/githubauth"
)

const (
	branchDefaultIntegrationTimeout            = 20 * time.Second
	branchDefaultGitExecutable                 = "git"
	branchDefaultInitialBranch                 = "main"
	branchDefaultTargetBranch                  = "master"
	branchDefaultInitialCommitMessage          = "initial commit"
	branchDefaultMergeCommitMessage            = "merge master into main"
	branchDefaultWorkflowRelativePath          = ".github/workflows/ci.yml"
	branchDefaultWorkflowTemplate              = "name: CI\non:\n  push:\n    branches:\n      - %s\n"
	branchDefaultStubExecutableName            = "gh"
	branchDefaultGitWrapperExecutableName      = "git"
	branchDefaultParentRemoteRepository        = "example/parent"
	branchDefaultChildRemoteRepository         = "example/remote-child"
	branchDefaultDirtyRemoteRepository         = "example/dirty-default"
	branchDefaultParentRemoteURL               = "https://github.com/" + branchDefaultParentRemoteRepository + ".git"
	branchDefaultChildRemoteURL                = "https://github.com/" + branchDefaultChildRemoteRepository + ".git"
	branchDefaultDirtyRemoteURL                = "https://github.com/" + branchDefaultDirtyRemoteRepository + ".git"
	branchDefaultPromotionRemoteRepository     = "example/promotion"
	branchDefaultPromotionRemoteURL            = "https://github.com/" + branchDefaultPromotionRemoteRepository + ".git"
	branchDefaultPullRequestsStateFile         = "pull-requests.json"
	branchDefaultClosedPullRequestsLogFile     = "closed-pull-requests.log"
	branchDefaultGitIgnoreContents             = "tools/\n"
	branchDefaultStubStateDirectoryEnvironment = "BRANCH_DEFAULT_STATE_DIR"
	branchDefaultPagesStateEnvironment         = "BRANCH_DEFAULT_PAGES_STATE"
	branchDefaultPagesStateAbsent              = "absent"
	branchDefaultPagesStateFailure             = "failure"
	branchDefaultStubDefaultBranchPlaceholder  = "main"
	branchDefaultUserName                      = "Branch Default Tester"
	branchDefaultUserEmail                     = "default-command@example.com"
	branchDefaultWorkflowCommitMessageTemplate = "add workflow for %s"
)

func TestBranchDefaultHandlesNestedRepositoriesWithMixedRemotes(testInstance *testing.T) {
	testInstance.Helper()

	workspaceDirectory := testInstance.TempDir()
	parentRepositoryPath := filepath.Join(workspaceDirectory, "parent-remote")
	remoteChildPath := filepath.Join(parentRepositoryPath, "tools", "remote-child")
	localChildPath := filepath.Join(parentRepositoryPath, "tools", "local-child")

	initializeRepositoryWithFiles(
		testInstance,
		parentRepositoryPath,
		branchDefaultParentRemoteURL,
		map[string]string{
			"README.md":                       "parent repository\n",
			".gitignore":                      branchDefaultGitIgnoreContents,
			branchDefaultWorkflowRelativePath: fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch),
		},
	)

	initializeRepositoryWithFiles(
		testInstance,
		remoteChildPath,
		branchDefaultChildRemoteURL,
		map[string]string{
			"README.md":                       filepath.Base(remoteChildPath) + "\n",
			branchDefaultWorkflowRelativePath: fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch),
		},
	)

	initializeRepositoryWithFiles(
		testInstance,
		localChildPath,
		"",
		map[string]string{
			"README.md":                       filepath.Base(localChildPath) + "\n",
			branchDefaultWorkflowRelativePath: fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch),
		},
	)

	stateDirectory := testInstance.TempDir()
	initializeStubStateFile(testInstance, stateDirectory, branchDefaultParentRemoteRepository, branchDefaultInitialBranch)
	initializeStubStateFile(testInstance, stateDirectory, branchDefaultChildRemoteRepository, branchDefaultInitialBranch)
	testInstance.Logf("state directory: %s", stateDirectory)

	stubDirectory := filepath.Join(testInstance.TempDir(), "bin")
	require.NoError(testInstance, os.MkdirAll(stubDirectory, 0o755))
	testInstance.Logf("stub directory: %s", stubDirectory)

	githubStubPath := filepath.Join(stubDirectory, branchDefaultStubExecutableName)
	require.NoError(testInstance, os.WriteFile(githubStubPath, []byte(buildBranchDefaultStubScript(stateDirectory)), 0o755))

	realGitBinary, lookupError := exec.LookPath(branchDefaultGitExecutable)
	require.NoError(testInstance, lookupError)

	gitWrapperPath := filepath.Join(stubDirectory, branchDefaultGitWrapperExecutableName)
	require.NoError(testInstance, os.WriteFile(gitWrapperPath, []byte(buildBranchDefaultGitWrapper(realGitBinary, stateDirectory)), 0o755))

	repositoryRoot := integrationRepositoryRoot(testInstance)
	commandArguments := []string{
		"run",
		".",
		"--log-level",
		"error",
		"default",
		branchDefaultTargetBranch,
		"--roots",
		parentRepositoryPath,
		"--yes",
	}

	extendedPath := stubDirectory + string(os.PathListSeparator) + os.Getenv(pathEnvironmentVariableNameConstant)
	commandOptions := integrationCommandOptions{
		PathVariable: extendedPath,
		EnvironmentOverrides: map[string]string{
			branchDefaultStubStateDirectoryEnvironment: stateDirectory,
			branchDefaultPagesStateEnvironment:         branchDefaultPagesStateAbsent,
			githubauth.EnvGitHubToken:                  "test-token",
			githubauth.EnvGitHubCLIToken:               "test-token",
			githubauth.EnvGitHubAPIToken:               "test-token",
		},
	}

	output := runIntegrationCommand(
		testInstance,
		repositoryRoot,
		commandOptions,
		branchDefaultIntegrationTimeout,
		commandArguments,
	)
	testInstance.Logf("branch default output:\n%s", output)

	require.Contains(testInstance, output, fmt.Sprintf("WORKFLOW-DEFAULT: %s (main → master)", parentRepositoryPath))
	require.NotContains(testInstance, output, "PAGES-SKIP")
	require.NotContains(testInstance, output, remoteChildPath)
	require.NotContains(testInstance, output, localChildPath)
	require.NotContains(testInstance, strings.ToLower(output), "default branch update failed")

	assertRepositoryHead(testInstance, parentRepositoryPath, branchDefaultTargetBranch)
	assertRepositoryHead(testInstance, remoteChildPath, branchDefaultInitialBranch)
	assertRepositoryHead(testInstance, localChildPath, branchDefaultInitialBranch)

	assertStateFileBranch(testInstance, stateDirectory, branchDefaultParentRemoteRepository, branchDefaultTargetBranch)
	assertStateFileBranch(testInstance, stateDirectory, branchDefaultChildRemoteRepository, branchDefaultInitialBranch)
}

func TestBranchDefaultStopsBeforeDefaultBranchUpdateWhenPagesLookupFails(testInstance *testing.T) {
	workspaceDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(workspaceDirectory, "pages-failure")
	initializeRepositoryWithFiles(
		testInstance,
		repositoryPath,
		branchDefaultPromotionRemoteURL,
		map[string]string{
			"README.md":                       "Pages failure repository\n",
			branchDefaultWorkflowRelativePath: fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch),
		},
	)

	stateDirectory := testInstance.TempDir()
	initializeStubStateFile(testInstance, stateDirectory, branchDefaultPromotionRemoteRepository, branchDefaultInitialBranch)
	stubDirectory := filepath.Join(testInstance.TempDir(), "bin")
	require.NoError(testInstance, os.MkdirAll(stubDirectory, 0o755))
	require.NoError(testInstance, os.WriteFile(
		filepath.Join(stubDirectory, branchDefaultStubExecutableName),
		[]byte(buildBranchDefaultStubScript(stateDirectory)),
		0o755,
	))
	realGitBinary, lookupError := exec.LookPath(branchDefaultGitExecutable)
	require.NoError(testInstance, lookupError)
	require.NoError(testInstance, os.WriteFile(
		filepath.Join(stubDirectory, branchDefaultGitWrapperExecutableName),
		[]byte(buildBranchDefaultGitWrapper(realGitBinary, stateDirectory)),
		0o755,
	))

	output, runError := runFailingIntegrationCommand(
		testInstance,
		integrationRepositoryRoot(testInstance),
		integrationCommandOptions{
			PathVariable: stubDirectory + string(os.PathListSeparator) + os.Getenv(pathEnvironmentVariableNameConstant),
			EnvironmentOverrides: map[string]string{
				branchDefaultStubStateDirectoryEnvironment: stateDirectory,
				branchDefaultPagesStateEnvironment:         branchDefaultPagesStateFailure,
				githubauth.EnvGitHubToken:                  "test-token",
				githubauth.EnvGitHubCLIToken:               "test-token",
				githubauth.EnvGitHubAPIToken:               "test-token",
			},
		},
		branchDefaultIntegrationTimeout,
		[]string{
			"run",
			".",
			"--log-level",
			"error",
			"default",
			branchDefaultTargetBranch,
			"--roots",
			repositoryPath,
			"--yes",
		},
	)
	require.Error(testInstance, runError)
	require.Contains(testInstance, output, "GitHub Pages update failed")
	require.Contains(testInstance, output, "HTTP 403")
	require.NotContains(testInstance, output, "WORKFLOW-DEFAULT:")
	assertStateFileBranch(testInstance, stateDirectory, branchDefaultPromotionRemoteRepository, branchDefaultInitialBranch)

	githubLog := readTextFile(testInstance, filepath.Join(stateDirectory, "gh.log"))
	require.Contains(testInstance, githubLog, "api repos/"+branchDefaultPromotionRemoteRepository+"/pages -X GET")
	require.NotContains(testInstance, githubLog, "api repos/"+branchDefaultPromotionRemoteRepository+" -X PATCH")
}

func TestBranchDefaultClosesPullRequestFromPromotedBranch(testInstance *testing.T) {
	workspaceDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(workspaceDirectory, "promotion")
	initializeRepositoryWithFiles(
		testInstance,
		repositoryPath,
		branchDefaultPromotionRemoteURL,
		map[string]string{
			"README.md":                       "promotion repository\n",
			branchDefaultWorkflowRelativePath: fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch),
		},
	)

	stateDirectory := testInstance.TempDir()
	initializeStubStateFile(testInstance, stateDirectory, branchDefaultPromotionRemoteRepository, branchDefaultInitialBranch)
	require.NoError(testInstance, os.WriteFile(
		filepath.Join(stateDirectory, branchDefaultPullRequestsStateFile),
		[]byte(`[{"number":1,"title":"Promote master","headRefName":"master","headRefOid":"target-commit","headRepository":{"nameWithOwner":"example/promotion"},"baseRefName":"main"}]`),
		0o644,
	))

	stubDirectory := filepath.Join(testInstance.TempDir(), "bin")
	require.NoError(testInstance, os.MkdirAll(stubDirectory, 0o755))
	require.NoError(testInstance, os.WriteFile(
		filepath.Join(stubDirectory, branchDefaultStubExecutableName),
		[]byte(buildBranchDefaultStubScript(stateDirectory)),
		0o755,
	))
	realGitBinary, lookupError := exec.LookPath(branchDefaultGitExecutable)
	require.NoError(testInstance, lookupError)
	require.NoError(testInstance, os.WriteFile(
		filepath.Join(stubDirectory, branchDefaultGitWrapperExecutableName),
		[]byte(buildBranchDefaultGitWrapper(realGitBinary, stateDirectory)),
		0o755,
	))
	verifiedSourceCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", branchDefaultInitialBranch))

	output := runIntegrationCommand(
		testInstance,
		integrationRepositoryRoot(testInstance),
		integrationCommandOptions{
			PathVariable: stubDirectory + string(os.PathListSeparator) + os.Getenv(pathEnvironmentVariableNameConstant),
			EnvironmentOverrides: map[string]string{
				branchDefaultStubStateDirectoryEnvironment: stateDirectory,
				githubauth.EnvGitHubToken:                  "test-token",
				githubauth.EnvGitHubCLIToken:               "test-token",
				githubauth.EnvGitHubAPIToken:               "test-token",
			},
		},
		branchDefaultIntegrationTimeout,
		[]string{
			"run",
			".",
			"--log-level",
			"error",
			"default",
			branchDefaultTargetBranch,
			"--roots",
			repositoryPath,
			"--yes",
		},
	)

	require.Contains(testInstance, output, fmt.Sprintf(
		"WORKFLOW-DEFAULT: %s (main → master) safe_to_delete=true source_deleted=true",
		repositoryPath,
	))
	require.NotContains(testInstance, output, "PR-RETARGET-SKIP")
	githubLog := readTextFile(testInstance, filepath.Join(stateDirectory, "gh.log"))
	require.Contains(testInstance, githubLog, "pr close 1 --repo "+branchDefaultPromotionRemoteRepository)
	require.NotContains(testInstance, githubLog, "pr edit 1")
	require.Equal(testInstance, "1", strings.TrimSpace(readTextFile(
		testInstance,
		filepath.Join(stateDirectory, branchDefaultClosedPullRequestsLogFile),
	)))
	require.Equal(testInstance, "[]", strings.TrimSpace(readTextFile(
		testInstance,
		filepath.Join(stateDirectory, branchDefaultPullRequestsStateFile),
	)))
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--list", branchDefaultInitialBranch)))
	gitLog := readTextFile(testInstance, filepath.Join(stateDirectory, "git.log"))
	require.Contains(testInstance, gitLog, "push\n--force-with-lease=refs/heads/"+branchDefaultInitialBranch+":"+verifiedSourceCommit+"\norigin\n--delete\n"+branchDefaultInitialBranch+"\n")
}

func TestBranchDefaultAlreadyTargetRemovesObsoleteReviewBase(testInstance *testing.T) {
	workspaceDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(workspaceDirectory, "already-default")
	initializeRepositoryWithFiles(
		testInstance,
		repositoryPath,
		branchDefaultPromotionRemoteURL,
		map[string]string{
			"README.md":                       "already default repository\n",
			branchDefaultWorkflowRelativePath: fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch),
		},
	)
	runGit(testInstance, repositoryPath, "config", "branch.master.gix-review-base", branchDefaultInitialBranch)

	stateDirectory := testInstance.TempDir()
	initializeStubStateFile(testInstance, stateDirectory, branchDefaultPromotionRemoteRepository, branchDefaultTargetBranch)
	output := runBranchDefaultMigration(testInstance, repositoryPath, stateDirectory)

	require.Contains(testInstance, output, fmt.Sprintf(
		"WORKFLOW-DEFAULT-SKIP: %s already defaults to master",
		repositoryPath,
	))
	assertBranchReviewBaseMissing(testInstance, repositoryPath, branchDefaultTargetBranch)
	githubLog := readTextFile(testInstance, filepath.Join(stateDirectory, "gh.log"))
	require.NotContains(testInstance, githubLog, "default_branch=")
}

func TestBranchDefaultAlreadyTargetIgnoresIncludedReviewBase(testInstance *testing.T) {
	workspaceDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(workspaceDirectory, "included-review-base")
	initializeRepositoryWithFiles(
		testInstance,
		repositoryPath,
		branchDefaultPromotionRemoteURL,
		map[string]string{
			"README.md":                       "included review base repository\n",
			branchDefaultWorkflowRelativePath: fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch),
		},
	)
	includePath := filepath.Join(testInstance.TempDir(), "review-base.inc")
	require.NoError(testInstance, os.WriteFile(includePath, []byte("[branch \"master\"]\n\tgix-review-base = main\n"), 0o600))
	runGit(testInstance, repositoryPath, "config", "--local", "include.path", includePath)
	require.Equal(testInstance, branchDefaultInitialBranch, strings.TrimSpace(runGit(
		testInstance,
		repositoryPath,
		"config",
		"--includes",
		"--get",
		"branch.master.gix-review-base",
	)))

	stateDirectory := testInstance.TempDir()
	initializeStubStateFile(testInstance, stateDirectory, branchDefaultPromotionRemoteRepository, branchDefaultTargetBranch)
	output := runBranchDefaultMigration(testInstance, repositoryPath, stateDirectory)

	require.Contains(testInstance, output, fmt.Sprintf(
		"WORKFLOW-DEFAULT-SKIP: %s already defaults to master",
		repositoryPath,
	))
	require.Equal(testInstance, branchDefaultInitialBranch, strings.TrimSpace(runGit(
		testInstance,
		repositoryPath,
		"config",
		"--includes",
		"--get",
		"branch.master.gix-review-base",
	)))
	assertBranchReviewBaseMissing(testInstance, repositoryPath, branchDefaultTargetBranch)
}

func TestBranchDefaultPreservesCaseDistinctReviewBaseBeforeMigration(testInstance *testing.T) {
	workspaceDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(workspaceDirectory, "case-distinct-default")
	initializeRepositoryWithFiles(
		testInstance,
		repositoryPath,
		branchDefaultPromotionRemoteURL,
		map[string]string{
			"README.md":                       "case-distinct default repository\n",
			branchDefaultWorkflowRelativePath: fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch),
		},
	)
	runGit(testInstance, repositoryPath, "config", "--local", "branch.master.gix-review-base", branchDefaultInitialBranch)
	require.NoError(testInstance, os.WriteFile(filepath.Join(repositoryPath, "README.md"), []byte("dirty case-distinct work\n"), 0o644))

	stateDirectory := testInstance.TempDir()
	initializeStubStateFile(testInstance, stateDirectory, branchDefaultPromotionRemoteRepository, "Master")
	output, runError := runFailingBranchDefaultMigration(testInstance, repositoryPath, stateDirectory)

	require.Error(testInstance, runError)
	require.Contains(testInstance, output, "repository worktree must be clean before migration")
	require.NotContains(testInstance, output, "WORKFLOW-DEFAULT-SKIP")
	require.Equal(
		testInstance,
		branchDefaultInitialBranch,
		strings.TrimSpace(runGit(testInstance, repositoryPath, "config", "--local", "--no-includes", "--get", "branch.master.gix-review-base")),
	)
}

func TestBranchDefaultDeletesContentEquivalentMergedSource(testInstance *testing.T) {
	workspaceDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(workspaceDirectory, "merged-source")
	initializeRepositoryWithFiles(
		testInstance,
		repositoryPath,
		branchDefaultPromotionRemoteURL,
		map[string]string{
			"README.md":                       "merged source repository\n",
			branchDefaultWorkflowRelativePath: fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch),
		},
	)

	runGit(testInstance, repositoryPath, "checkout", "-b", branchDefaultTargetBranch)
	featurePath := filepath.Join(repositoryPath, "model.txt")
	require.NoError(testInstance, os.WriteFile(featurePath, []byte("model configuration\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", filepath.Base(featurePath))
	runGit(testInstance, repositoryPath, "commit", "-m", "add model configuration")
	runGit(testInstance, repositoryPath, "checkout", branchDefaultInitialBranch)
	runGit(testInstance, repositoryPath, "merge", "--no-ff", branchDefaultTargetBranch, "-m", branchDefaultMergeCommitMessage)
	runGit(testInstance, repositoryPath, "config", "branch.master.gix-review-base", branchDefaultInitialBranch)

	stateDirectory := testInstance.TempDir()
	initializeStubStateFile(testInstance, stateDirectory, branchDefaultPromotionRemoteRepository, branchDefaultInitialBranch)
	verifiedSourceCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", branchDefaultInitialBranch))
	output := runBranchDefaultMigration(testInstance, repositoryPath, stateDirectory)

	require.Contains(testInstance, output, fmt.Sprintf(
		"WORKFLOW-DEFAULT: %s (main → master) safe_to_delete=true source_deleted=true",
		repositoryPath,
	))
	assertBranchReviewBaseMissing(testInstance, repositoryPath, branchDefaultTargetBranch)
	require.Empty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--list", branchDefaultInitialBranch)))
	gitLog := readTextFile(testInstance, filepath.Join(stateDirectory, "git.log"))
	require.Contains(testInstance, gitLog, "fetch\n--no-tags\norigin\n+refs/heads/main:refs/remotes/origin/main\n+refs/heads/master:refs/remotes/origin/master\n")
	require.Contains(testInstance, gitLog, "push\n--force-with-lease=refs/heads/main:"+verifiedSourceCommit+"\norigin\n--delete\nmain\n")
}

func TestBranchDefaultRetainsSourceWithUnpreservedChanges(testInstance *testing.T) {
	workspaceDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(workspaceDirectory, "divergent-source")
	initializeRepositoryWithFiles(
		testInstance,
		repositoryPath,
		branchDefaultPromotionRemoteURL,
		map[string]string{
			"README.md":                       "divergent source repository\n",
			branchDefaultWorkflowRelativePath: fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch),
		},
	)

	runGit(testInstance, repositoryPath, "checkout", "-b", branchDefaultTargetBranch)
	targetPath := filepath.Join(repositoryPath, "target.txt")
	require.NoError(testInstance, os.WriteFile(targetPath, []byte("target change\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", filepath.Base(targetPath))
	runGit(testInstance, repositoryPath, "commit", "-m", "add target change")
	runGit(testInstance, repositoryPath, "checkout", branchDefaultInitialBranch)
	sourcePath := filepath.Join(repositoryPath, "source.txt")
	require.NoError(testInstance, os.WriteFile(sourcePath, []byte("source change\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", filepath.Base(sourcePath))
	runGit(testInstance, repositoryPath, "commit", "-m", "add source change")

	stateDirectory := testInstance.TempDir()
	initializeStubStateFile(testInstance, stateDirectory, branchDefaultPromotionRemoteRepository, branchDefaultInitialBranch)
	output := runBranchDefaultMigration(testInstance, repositoryPath, stateDirectory)

	require.Contains(testInstance, output, fmt.Sprintf(
		"WORKFLOW-DEFAULT: %s (main → master) safe_to_delete=false source_deleted=false",
		repositoryPath,
	))
	require.NotEmpty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--list", branchDefaultInitialBranch)))
	gitLog := readTextFile(testInstance, filepath.Join(stateDirectory, "git.log"))
	require.NotContains(testInstance, gitLog, "--delete\nmain\n")
}

func TestBranchDefaultRetainsRemoteSourceAheadOfStaleLocalBranch(testInstance *testing.T) {
	workspaceDirectory := testInstance.TempDir()
	remotePath := filepath.Join(workspaceDirectory, "remote.git")
	repositoryPath := filepath.Join(workspaceDirectory, "stale-checkout")
	runGitWithDir(testInstance, workspaceDirectory, "init", "--bare", remotePath)
	runGitWithDir(testInstance, workspaceDirectory, "init", "--initial-branch="+branchDefaultInitialBranch, repositoryPath)
	configureGitIdentity(testInstance, repositoryPath)

	readmePath := filepath.Join(repositoryPath, "README.md")
	workflowPath := filepath.Join(repositoryPath, branchDefaultWorkflowRelativePath)
	require.NoError(testInstance, os.WriteFile(readmePath, []byte("stale checkout repository\n"), 0o644))
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(workflowPath), 0o755))
	require.NoError(testInstance, os.WriteFile(workflowPath, []byte(fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch)), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md", branchDefaultWorkflowRelativePath)
	runGit(testInstance, repositoryPath, "commit", "-m", branchDefaultInitialCommitMessage)
	initialCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	runGit(testInstance, repositoryPath, "remote", "add", "origin", remotePath)
	runGit(testInstance, repositoryPath, "push", "-u", "origin", branchDefaultInitialBranch)
	runGit(testInstance, repositoryPath, "branch", branchDefaultTargetBranch, initialCommit)
	runGit(testInstance, repositoryPath, "push", "origin", branchDefaultTargetBranch)

	remoteOnlyPath := filepath.Join(repositoryPath, "remote-only.txt")
	require.NoError(testInstance, os.WriteFile(remoteOnlyPath, []byte("remote source change\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", filepath.Base(remoteOnlyPath))
	runGit(testInstance, repositoryPath, "commit", "-m", "add remote source change")
	remoteSourceCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	runGit(testInstance, repositoryPath, "push", "origin", branchDefaultInitialBranch)
	runGit(testInstance, repositoryPath, "checkout", branchDefaultTargetBranch)
	runGit(testInstance, repositoryPath, "branch", "-f", branchDefaultInitialBranch, initialCommit)
	runGit(testInstance, remotePath, "symbolic-ref", "HEAD", "refs/heads/"+branchDefaultInitialBranch)
	runGit(testInstance, repositoryPath, "remote", "set-url", "origin", branchDefaultPromotionRemoteURL)

	stateDirectory := testInstance.TempDir()
	initializeStubStateFile(testInstance, stateDirectory, branchDefaultPromotionRemoteRepository, branchDefaultInitialBranch)
	output := runBranchDefaultMigrationWithRemote(testInstance, repositoryPath, stateDirectory, remotePath)

	require.Contains(testInstance, output, fmt.Sprintf(
		"WORKFLOW-DEFAULT: %s (main → master) safe_to_delete=false source_deleted=false",
		repositoryPath,
	))
	require.Equal(testInstance, remoteSourceCommit, strings.TrimSpace(runGit(testInstance, remotePath, "rev-parse", "refs/heads/"+branchDefaultInitialBranch)))
	gitLog := readTextFile(testInstance, filepath.Join(stateDirectory, "git.log"))
	require.Contains(testInstance, gitLog, "fetch --no-tags origin +refs/heads/main:refs/remotes/origin/main +refs/heads/master:refs/remotes/origin/master")
	require.NotContains(testInstance, gitLog, "--delete "+branchDefaultInitialBranch)
}

func TestBranchDefaultRetainsRemoteSourceThatAdvancesBeforeDeletion(testInstance *testing.T) {
	workspaceDirectory := testInstance.TempDir()
	remotePath := filepath.Join(workspaceDirectory, "remote.git")
	repositoryPath := filepath.Join(workspaceDirectory, "source-race")
	runGitWithDir(testInstance, workspaceDirectory, "init", "--bare", remotePath)
	runGitWithDir(testInstance, workspaceDirectory, "init", "--initial-branch="+branchDefaultInitialBranch, repositoryPath)
	configureGitIdentity(testInstance, repositoryPath)

	readmePath := filepath.Join(repositoryPath, "README.md")
	workflowPath := filepath.Join(repositoryPath, branchDefaultWorkflowRelativePath)
	require.NoError(testInstance, os.WriteFile(readmePath, []byte("source race repository\n"), 0o644))
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(workflowPath), 0o755))
	require.NoError(testInstance, os.WriteFile(workflowPath, []byte(fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch)), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md", branchDefaultWorkflowRelativePath)
	runGit(testInstance, repositoryPath, "commit", "-m", branchDefaultInitialCommitMessage)
	verifiedSourceCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	runGit(testInstance, repositoryPath, "remote", "add", "origin", remotePath)
	runGit(testInstance, repositoryPath, "push", "-u", "origin", branchDefaultInitialBranch)
	runGit(testInstance, repositoryPath, "branch", branchDefaultTargetBranch, verifiedSourceCommit)
	runGit(testInstance, repositoryPath, "push", "origin", branchDefaultTargetBranch)

	concurrentPath := filepath.Join(repositoryPath, "concurrent.txt")
	require.NoError(testInstance, os.WriteFile(concurrentPath, []byte("concurrent source change\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", filepath.Base(concurrentPath))
	runGit(testInstance, repositoryPath, "commit", "-m", "add concurrent source change")
	advancedSourceCommit := strings.TrimSpace(runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	runGit(testInstance, repositoryPath, "push", "origin", "HEAD:refs/heads/source-race")
	runGit(testInstance, repositoryPath, "checkout", branchDefaultTargetBranch)
	runGit(testInstance, repositoryPath, "branch", "-f", branchDefaultInitialBranch, verifiedSourceCommit)
	runGit(testInstance, remotePath, "symbolic-ref", "HEAD", "refs/heads/"+branchDefaultInitialBranch)
	runGit(testInstance, repositoryPath, "remote", "set-url", "origin", branchDefaultPromotionRemoteURL)

	stateDirectory := testInstance.TempDir()
	initializeStubStateFile(testInstance, stateDirectory, branchDefaultPromotionRemoteRepository, branchDefaultInitialBranch)
	output := runBranchDefaultMigrationWithRemoteAdvance(testInstance, repositoryPath, stateDirectory, remotePath, advancedSourceCommit)

	require.Contains(testInstance, output, fmt.Sprintf(
		"WORKFLOW-DEFAULT: %s (main → master) safe_to_delete=true source_deleted=false",
		repositoryPath,
	))
	require.Contains(testInstance, output, "DELETE-SKIP:")
	require.Equal(testInstance, advancedSourceCommit, strings.TrimSpace(runGit(testInstance, remotePath, "rev-parse", "refs/heads/"+branchDefaultInitialBranch)))
	require.NotEmpty(testInstance, strings.TrimSpace(runGit(testInstance, repositoryPath, "branch", "--list", branchDefaultInitialBranch)))
	gitLog := readTextFile(testInstance, filepath.Join(stateDirectory, "git.log"))
	require.Contains(testInstance, gitLog, "push --force-with-lease=refs/heads/main:"+verifiedSourceCommit+" origin --delete main")
}

func TestBranchDefaultRetargetsSameNamedForkPullRequest(testInstance *testing.T) {
	workspaceDirectory := testInstance.TempDir()
	repositoryPath := filepath.Join(workspaceDirectory, "promotion")
	initializeRepositoryWithFiles(
		testInstance,
		repositoryPath,
		branchDefaultPromotionRemoteURL,
		map[string]string{
			"README.md":                       "promotion repository\n",
			branchDefaultWorkflowRelativePath: fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch),
		},
	)

	stateDirectory := testInstance.TempDir()
	initializeStubStateFile(testInstance, stateDirectory, branchDefaultPromotionRemoteRepository, branchDefaultInitialBranch)
	pullRequestState := `[{"number":2,"title":"Fork master","headRefName":"master","headRefOid":"fork-commit","headRepository":{"nameWithOwner":"contributor/promotion"},"baseRefName":"main"}]`
	require.NoError(testInstance, os.WriteFile(
		filepath.Join(stateDirectory, branchDefaultPullRequestsStateFile),
		[]byte(pullRequestState),
		0o644,
	))

	stubDirectory := filepath.Join(testInstance.TempDir(), "bin")
	require.NoError(testInstance, os.MkdirAll(stubDirectory, 0o755))
	require.NoError(testInstance, os.WriteFile(
		filepath.Join(stubDirectory, branchDefaultStubExecutableName),
		[]byte(buildBranchDefaultStubScript(stateDirectory)),
		0o755,
	))
	realGitBinary, lookupError := exec.LookPath(branchDefaultGitExecutable)
	require.NoError(testInstance, lookupError)
	require.NoError(testInstance, os.WriteFile(
		filepath.Join(stubDirectory, branchDefaultGitWrapperExecutableName),
		[]byte(buildBranchDefaultGitWrapper(realGitBinary, stateDirectory)),
		0o755,
	))

	output := runIntegrationCommand(
		testInstance,
		integrationRepositoryRoot(testInstance),
		integrationCommandOptions{
			PathVariable: stubDirectory + string(os.PathListSeparator) + os.Getenv(pathEnvironmentVariableNameConstant),
			EnvironmentOverrides: map[string]string{
				branchDefaultStubStateDirectoryEnvironment: stateDirectory,
				githubauth.EnvGitHubToken:                  "test-token",
				githubauth.EnvGitHubCLIToken:               "test-token",
				githubauth.EnvGitHubAPIToken:               "test-token",
			},
		},
		branchDefaultIntegrationTimeout,
		[]string{
			"run",
			".",
			"--log-level",
			"error",
			"default",
			branchDefaultTargetBranch,
			"--roots",
			repositoryPath,
			"--yes",
		},
	)

	require.Contains(testInstance, output, fmt.Sprintf(
		"WORKFLOW-DEFAULT: %s (main → master) safe_to_delete=false",
		repositoryPath,
	))
	require.NotContains(testInstance, output, "PR-CLOSE-SKIP")
	require.NotContains(testInstance, output, "PR-RETARGET-SKIP")
	githubLog := readTextFile(testInstance, filepath.Join(stateDirectory, "gh.log"))
	require.Contains(testInstance, githubLog, "pr edit 2 --repo "+branchDefaultPromotionRemoteRepository+" --base master")
	require.NotContains(testInstance, githubLog, "pr close 2")
	require.NoFileExists(testInstance, filepath.Join(stateDirectory, branchDefaultClosedPullRequestsLogFile))
	require.JSONEq(testInstance, pullRequestState, readTextFile(
		testInstance,
		filepath.Join(stateDirectory, branchDefaultPullRequestsStateFile),
	))
}

func TestBranchDefaultRejectsDirtyWorktreeBeforeLocalOrRemoteMutation(testInstance *testing.T) {
	workspaceDirectory := testInstance.TempDir()
	remotePath := filepath.Join(workspaceDirectory, "remote.git")
	repositoryPath := filepath.Join(workspaceDirectory, "repository")
	runGitWithDir(testInstance, workspaceDirectory, "init", "--bare", remotePath)
	runGitWithDir(testInstance, workspaceDirectory, "init", "--initial-branch="+branchDefaultInitialBranch, repositoryPath)
	configureGitIdentity(testInstance, repositoryPath)

	readmePath := filepath.Join(repositoryPath, "README.md")
	unstagedPath := filepath.Join(repositoryPath, "unstaged.txt")
	untrackedPath := filepath.Join(repositoryPath, "untracked.txt")
	workflowPath := filepath.Join(repositoryPath, branchDefaultWorkflowRelativePath)
	require.NoError(testInstance, os.WriteFile(readmePath, []byte("initial\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(unstagedPath, []byte("initial unstaged file\n"), 0o644))
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(workflowPath), 0o755))
	require.NoError(testInstance, os.WriteFile(workflowPath, []byte(fmt.Sprintf(branchDefaultWorkflowTemplate, branchDefaultInitialBranch)), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md", "unstaged.txt", branchDefaultWorkflowRelativePath)
	runGit(testInstance, repositoryPath, "commit", "-m", branchDefaultInitialCommitMessage)
	runGit(testInstance, repositoryPath, "remote", "add", "origin", remotePath)
	runGit(testInstance, repositoryPath, "push", "-u", "origin", branchDefaultInitialBranch)
	runGit(testInstance, remotePath, "symbolic-ref", "HEAD", "refs/heads/"+branchDefaultInitialBranch)
	runGit(testInstance, repositoryPath, "remote", "set-url", "origin", branchDefaultDirtyRemoteURL)

	require.NoError(testInstance, os.WriteFile(readmePath, []byte("staged change\n"), 0o644))
	runGit(testInstance, repositoryPath, "add", "README.md")
	require.NoError(testInstance, os.WriteFile(unstagedPath, []byte("unstaged change\n"), 0o644))
	require.NoError(testInstance, os.WriteFile(untrackedPath, []byte("untracked change\n"), 0o644))

	startingBranch := runGit(testInstance, repositoryPath, "symbolic-ref", "--short", "HEAD")
	startingCommit := runGit(testInstance, repositoryPath, "rev-parse", "HEAD")
	startingStatus := runGit(testInstance, repositoryPath, "status", "--porcelain=v1", "-z")
	startingIndex := runGit(testInstance, repositoryPath, "ls-files", "--stage", "-v", "-z")
	startingRepositoryRefs := runGit(testInstance, repositoryPath, "for-each-ref", "--format=%(refname) %(objectname)", "refs/")
	startingRemoteRefs := runGit(testInstance, remotePath, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads/")
	startingReadme := readTextFile(testInstance, readmePath)
	startingUnstaged := readTextFile(testInstance, unstagedPath)
	startingUntracked := readTextFile(testInstance, untrackedPath)
	startingWorkflow := readTextFile(testInstance, workflowPath)

	stateDirectory := testInstance.TempDir()
	initializeStubStateFile(testInstance, stateDirectory, branchDefaultDirtyRemoteRepository, branchDefaultInitialBranch)
	stubDirectory := filepath.Join(testInstance.TempDir(), "bin")
	require.NoError(testInstance, os.MkdirAll(stubDirectory, 0o755))
	realGitBinary, lookupError := exec.LookPath(branchDefaultGitExecutable)
	require.NoError(testInstance, lookupError)
	require.NoError(testInstance, os.WriteFile(filepath.Join(stubDirectory, branchDefaultStubExecutableName), []byte(buildBranchDefaultStubScript(stateDirectory)), 0o755))
	require.NoError(testInstance, os.WriteFile(filepath.Join(stubDirectory, branchDefaultGitWrapperExecutableName), []byte(buildBranchDefaultDelegatingGitWrapper(realGitBinary, stateDirectory, remotePath, "")), 0o755))

	output, runError := runFailingIntegrationCommand(
		testInstance,
		integrationRepositoryRoot(testInstance),
		integrationCommandOptions{
			PathVariable: stubDirectory + string(os.PathListSeparator) + os.Getenv(pathEnvironmentVariableNameConstant),
			EnvironmentOverrides: map[string]string{
				branchDefaultStubStateDirectoryEnvironment: stateDirectory,
				githubauth.EnvGitHubToken:                  "test-token",
				githubauth.EnvGitHubCLIToken:               "test-token",
				githubauth.EnvGitHubAPIToken:               "test-token",
			},
		},
		branchDefaultIntegrationTimeout,
		[]string{
			"run",
			".",
			"--log-level",
			"error",
			"default",
			branchDefaultTargetBranch,
			"--roots",
			repositoryPath,
			"--yes",
		},
	)
	require.Error(testInstance, runError)
	require.Contains(testInstance, output, "repository worktree must be clean before migration")

	require.Equal(testInstance, startingBranch, runGit(testInstance, repositoryPath, "symbolic-ref", "--short", "HEAD"))
	require.Equal(testInstance, startingCommit, runGit(testInstance, repositoryPath, "rev-parse", "HEAD"))
	require.Equal(testInstance, startingStatus, runGit(testInstance, repositoryPath, "status", "--porcelain=v1", "-z"))
	require.Equal(testInstance, startingIndex, runGit(testInstance, repositoryPath, "ls-files", "--stage", "-v", "-z"))
	require.Equal(testInstance, startingRepositoryRefs, runGit(testInstance, repositoryPath, "for-each-ref", "--format=%(refname) %(objectname)", "refs/"))
	require.Equal(testInstance, startingRemoteRefs, runGit(testInstance, remotePath, "for-each-ref", "--format=%(refname) %(objectname)", "refs/heads/"))
	require.Equal(testInstance, startingReadme, readTextFile(testInstance, readmePath))
	require.Equal(testInstance, startingUnstaged, readTextFile(testInstance, unstagedPath))
	require.Equal(testInstance, startingUntracked, readTextFile(testInstance, untrackedPath))
	require.Equal(testInstance, startingWorkflow, readTextFile(testInstance, workflowPath))
	assertStateFileBranch(testInstance, stateDirectory, branchDefaultDirtyRemoteRepository, branchDefaultInitialBranch)

	gitLog := readTextFile(testInstance, filepath.Join(stateDirectory, "git.log"))
	require.NotContains(testInstance, gitLog, "branch "+branchDefaultTargetBranch+" "+branchDefaultInitialBranch)
	require.NotContains(testInstance, gitLog, "checkout "+branchDefaultTargetBranch)
	require.NotContains(testInstance, gitLog, "push origin "+branchDefaultTargetBranch+":"+branchDefaultTargetBranch)
	require.NotContains(testInstance, gitLog, "commit ")

	githubLog := readTextFile(testInstance, filepath.Join(stateDirectory, "gh.log"))
	require.NotContains(testInstance, githubLog, " -X PUT ")
	require.NotContains(testInstance, githubLog, " -X PATCH ")
	require.NotContains(testInstance, githubLog, "pr edit ")
}

func initializeRepositoryWithFiles(testInstance *testing.T, repositoryPath string, remoteURL string, files map[string]string) {
	testInstance.Helper()

	require.NoError(testInstance, os.MkdirAll(repositoryPath, 0o755))

	initCommand := exec.Command(branchDefaultGitExecutable, "init", "-b", branchDefaultInitialBranch, repositoryPath)
	initCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, initCommand.Run())

	configNameCommand := exec.Command(branchDefaultGitExecutable, "-C", repositoryPath, "config", "user.name", branchDefaultUserName)
	configNameCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, configNameCommand.Run())

	configEmailCommand := exec.Command(branchDefaultGitExecutable, "-C", repositoryPath, "config", "user.email", branchDefaultUserEmail)
	configEmailCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, configEmailCommand.Run())

	for relativePath, contents := range files {
		fullPath := filepath.Join(repositoryPath, relativePath)
		parentDirectory := filepath.Dir(fullPath)
		require.NoError(testInstance, os.MkdirAll(parentDirectory, 0o755))
		require.NoError(testInstance, os.WriteFile(fullPath, []byte(contents), 0o644))
	}

	addCommand := exec.Command(branchDefaultGitExecutable, "-C", repositoryPath, "add", ".")
	addCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, addCommand.Run())

	commitMessage := branchDefaultInitialCommitMessage
	if remoteURL == "" {
		commitMessage = fmt.Sprintf(branchDefaultWorkflowCommitMessageTemplate, filepath.Base(repositoryPath))
	}

	commitCommand := exec.Command(branchDefaultGitExecutable, "-C", repositoryPath, "commit", "-m", commitMessage)
	commitCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, commitCommand.Run())

	if len(strings.TrimSpace(remoteURL)) == 0 {
		return
	}

	remoteAddCommand := exec.Command(branchDefaultGitExecutable, "-C", repositoryPath, "remote", "add", "origin", remoteURL)
	remoteAddCommand.Env = buildGitCommandEnvironment(nil)
	require.NoError(testInstance, remoteAddCommand.Run())
}

func runBranchDefaultMigration(testInstance *testing.T, repositoryPath string, stateDirectory string) string {
	testInstance.Helper()
	realGitBinary, lookupError := exec.LookPath(branchDefaultGitExecutable)
	require.NoError(testInstance, lookupError)
	return runBranchDefaultMigrationWithWrapper(
		testInstance,
		repositoryPath,
		stateDirectory,
		buildBranchDefaultGitWrapper(realGitBinary, stateDirectory),
	)
}

func runBranchDefaultMigrationWithRemote(testInstance *testing.T, repositoryPath string, stateDirectory string, remotePath string) string {
	testInstance.Helper()
	realGitBinary, lookupError := exec.LookPath(branchDefaultGitExecutable)
	require.NoError(testInstance, lookupError)
	return runBranchDefaultMigrationWithWrapper(
		testInstance,
		repositoryPath,
		stateDirectory,
		buildBranchDefaultDelegatingGitWrapper(realGitBinary, stateDirectory, remotePath, ""),
	)
}

func runBranchDefaultMigrationWithRemoteAdvance(testInstance *testing.T, repositoryPath string, stateDirectory string, remotePath string, advancedSourceCommit string) string {
	testInstance.Helper()
	realGitBinary, lookupError := exec.LookPath(branchDefaultGitExecutable)
	require.NoError(testInstance, lookupError)
	return runBranchDefaultMigrationWithWrapper(
		testInstance,
		repositoryPath,
		stateDirectory,
		buildBranchDefaultDelegatingGitWrapper(realGitBinary, stateDirectory, remotePath, advancedSourceCommit),
	)
}

func runBranchDefaultMigrationWithWrapper(testInstance *testing.T, repositoryPath string, stateDirectory string, gitWrapper string) string {
	testInstance.Helper()
	stubDirectory := filepath.Join(testInstance.TempDir(), "bin")
	require.NoError(testInstance, os.MkdirAll(stubDirectory, 0o755))
	require.NoError(testInstance, os.WriteFile(
		filepath.Join(stubDirectory, branchDefaultStubExecutableName),
		[]byte(buildBranchDefaultStubScript(stateDirectory)),
		0o755,
	))
	require.NoError(testInstance, os.WriteFile(
		filepath.Join(stubDirectory, branchDefaultGitWrapperExecutableName),
		[]byte(gitWrapper),
		0o755,
	))

	return runIntegrationCommand(
		testInstance,
		integrationRepositoryRoot(testInstance),
		integrationCommandOptions{
			PathVariable: stubDirectory + string(os.PathListSeparator) + os.Getenv(pathEnvironmentVariableNameConstant),
			EnvironmentOverrides: map[string]string{
				branchDefaultStubStateDirectoryEnvironment: stateDirectory,
				branchDefaultPagesStateEnvironment:         branchDefaultPagesStateAbsent,
				githubauth.EnvGitHubToken:                  "test-token",
				githubauth.EnvGitHubCLIToken:               "test-token",
				githubauth.EnvGitHubAPIToken:               "test-token",
			},
		},
		branchDefaultIntegrationTimeout,
		[]string{
			"run",
			".",
			"--log-level",
			"error",
			"default",
			branchDefaultTargetBranch,
			"--roots",
			repositoryPath,
			"--yes",
		},
	)
}

func runFailingBranchDefaultMigration(testInstance *testing.T, repositoryPath string, stateDirectory string) (string, error) {
	testInstance.Helper()
	stubDirectory := filepath.Join(testInstance.TempDir(), "bin")
	require.NoError(testInstance, os.MkdirAll(stubDirectory, 0o755))
	require.NoError(testInstance, os.WriteFile(
		filepath.Join(stubDirectory, branchDefaultStubExecutableName),
		[]byte(buildBranchDefaultStubScript(stateDirectory)),
		0o755,
	))
	realGitBinary, lookupError := exec.LookPath(branchDefaultGitExecutable)
	require.NoError(testInstance, lookupError)
	require.NoError(testInstance, os.WriteFile(
		filepath.Join(stubDirectory, branchDefaultGitWrapperExecutableName),
		[]byte(buildBranchDefaultGitWrapper(realGitBinary, stateDirectory)),
		0o755,
	))

	return runFailingIntegrationCommand(
		testInstance,
		integrationRepositoryRoot(testInstance),
		integrationCommandOptions{
			PathVariable: stubDirectory + string(os.PathListSeparator) + os.Getenv(pathEnvironmentVariableNameConstant),
			EnvironmentOverrides: map[string]string{
				branchDefaultStubStateDirectoryEnvironment: stateDirectory,
				branchDefaultPagesStateEnvironment:         branchDefaultPagesStateAbsent,
				githubauth.EnvGitHubToken:                  "test-token",
				githubauth.EnvGitHubCLIToken:               "test-token",
				githubauth.EnvGitHubAPIToken:               "test-token",
			},
		},
		branchDefaultIntegrationTimeout,
		[]string{
			"run",
			".",
			"--log-level",
			"error",
			"default",
			branchDefaultTargetBranch,
			"--roots",
			repositoryPath,
			"--yes",
		},
	)
}

func buildBranchDefaultStubScript(stateDirectory string) string {
	return fmt.Sprintf(`#!/bin/sh
STATE_DIR=%[1]q
DEFAULT_BRANCH=%[2]q
PULL_REQUESTS_PATH="$STATE_DIR/%[3]s"
CLOSED_PULL_REQUESTS_PATH="$STATE_DIR/%[4]s"
PAGES_STATE="${%[5]s:-configured}"

state_path() {
  repo="$1"
  key=$(echo "$repo" | sed 's#/#__#g')
  echo "$STATE_DIR/$key.txt"
}

log_command() {
  printf '%%s\n' "$*" >>"$STATE_DIR/gh.log"
}

log_command "$@"

ensure_state() {
  repo="$1"
  path=$(state_path "$repo")
  if [ ! -f "$path" ]; then
    printf '%%s\n' "$DEFAULT_BRANCH" >"$path"
  fi
}

read_state() {
  repo="$1"
  ensure_state "$repo"
  path=$(state_path "$repo")
  tr -d '\n' <"$path"
}

write_state() {
  repo="$1"
  branch="$2"
  path=$(state_path "$repo")
  printf '%%s\n' "$branch" >"$path"
}

if [ "$1" = "repo" ] && [ "$2" = "view" ]; then
  repo="$3"
  default_branch=$(read_state "$repo")
  printf '{"nameWithOwner":"%%s","defaultBranchRef":{"name":"%%s"},"description":""}\n' "$repo" "$default_branch"
  exit 0
fi

if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
	if [ -f "$PULL_REQUESTS_PATH" ]; then
		cat "$PULL_REQUESTS_PATH"
		exit 0
	fi
  echo '[]'
  exit 0
fi

if [ "$1" = "pr" ] && [ "$2" = "edit" ]; then
	if [ "$3" = "1" ]; then
		echo "GraphQL: There are no new commits between base branch 'master' and head branch 'master' (updatePullRequest)" >&2
		exit 1
	fi
	exit 0
fi

if [ "$1" = "pr" ] && [ "$2" = "close" ]; then
	printf '%%s\n' "$3" >>"$CLOSED_PULL_REQUESTS_PATH"
	printf '[]\n' >"$PULL_REQUESTS_PATH"
  exit 0
fi

if [ "$1" = "api" ]; then
  endpoint="$2"
  method="$4"

  case "$endpoint" in
    repos/*/pages)
      if [ "$method" = "GET" ]; then
        case "$PAGES_STATE" in
          configured)
            echo '{"build_type":"legacy","source":{"branch":"main","path":"/"}}'
            exit 0
            ;;
          absent)
            echo 'gh: Not Found (HTTP 404)' >&2
            exit 1
            ;;
          failure)
            echo 'gh: Forbidden (HTTP 403)' >&2
            exit 1
            ;;
          *)
            echo 'unexpected Pages state' >&2
            exit 2
            ;;
        esac
      fi
      if [ "$method" = "PUT" ]; then
        if [ "$PAGES_STATE" != "configured" ]; then
          echo 'unexpected Pages update' >&2
          exit 2
        fi
        exit 0
      fi
      ;;
    repos/*/branches/*/protection)
      echo 'gh: Not Found (HTTP 404)' >&2
      exit 1
      ;;
    repos/*)
      repo=${endpoint#repos/}
      writeBranch="$method"
      if [ "$writeBranch" = "PATCH" ]; then
        for argument in "$@"; do
          case "$argument" in
            default_branch=*)
              write_state "$repo" "${argument#default_branch=}"
              ;;
          esac
        done
      fi
      exit 0
      ;;
  esac
fi

exit 0
`, stateDirectory, branchDefaultStubDefaultBranchPlaceholder, branchDefaultPullRequestsStateFile, branchDefaultClosedPullRequestsLogFile, branchDefaultPagesStateEnvironment)
}

func buildBranchDefaultGitWrapper(realGitPath string, stateDirectory string) string {
	return fmt.Sprintf(`#!/bin/sh
REAL_GIT=%q
STATE_DIR=%q
printf '%%s\n' "$@" >>"$STATE_DIR/git.log"
if [ "$1" = "ls-remote" ]; then
  exit 0
fi
if [ "$1" = "fetch" ]; then
  for argument in "$@"; do
    case "$argument" in
      +refs/heads/*:refs/remotes/*)
        source_reference=${argument%%%%:*}
        source_reference=${source_reference#+}
        destination_reference=${argument#*:}
        source_commit=$("$REAL_GIT" rev-parse "$source_reference")
        "$REAL_GIT" update-ref "$destination_reference" "$source_commit"
        ;;
    esac
  done
  exit 0
fi
if [ "$1" = "push" ]; then
  exit 0
fi
exec "$REAL_GIT" "$@"
`, realGitPath, stateDirectory)
}

func buildBranchDefaultDelegatingGitWrapper(realGitPath string, stateDirectory string, remotePath string, advancedSourceCommit string) string {
	return fmt.Sprintf(`#!/bin/sh
REAL_GIT=%q
STATE_DIR=%q
REMOTE_PATH=%q
ADVANCED_SOURCE_COMMIT=%q
printf '%%s\n' "$*" >>"$STATE_DIR/git.log"
if [ "$1" = "ls-remote" ] && [ "$2" = "--heads" ] && [ "$3" = "origin" ]; then
  exec "$REAL_GIT" ls-remote --heads "$REMOTE_PATH" "$4"
fi
if [ "$1" = "ls-remote" ] && [ "$2" = "--symref" ] && [ "$3" = "origin" ]; then
  exec "$REAL_GIT" ls-remote --symref "$REMOTE_PATH" "$4"
fi
if [ "$1" = "fetch" ] && [ "$2" = "--no-tags" ] && [ "$3" = "origin" ]; then
  shift 3
  exec "$REAL_GIT" fetch --no-tags "$REMOTE_PATH" "$@"
fi
if [ "$1" = "push" ] && [ "$2" = "origin" ]; then
  shift 2
  exec "$REAL_GIT" push "$REMOTE_PATH" "$@"
fi
if [ "$1" = "push" ] && [ "$3" = "origin" ]; then
  lease="$2"
  shift 3
  if [ -n "$ADVANCED_SOURCE_COMMIT" ]; then
    "$REAL_GIT" --git-dir="$REMOTE_PATH" update-ref refs/heads/main "$ADVANCED_SOURCE_COMMIT"
  fi
  exec "$REAL_GIT" push "$lease" "$REMOTE_PATH" "$@"
fi
exec "$REAL_GIT" "$@"
`, realGitPath, stateDirectory, remotePath, advancedSourceCommit)
}

func initializeStubStateFile(testInstance *testing.T, stateDirectory string, repository string, branch string) {
	testInstance.Helper()
	key := strings.ReplaceAll(repository, "/", "__")
	statePath := filepath.Join(stateDirectory, key+".txt")
	require.NoError(testInstance, os.WriteFile(statePath, []byte(branch+"\n"), 0o644))
}

func assertRepositoryHead(testInstance *testing.T, repositoryPath string, expectedBranch string) {
	testInstance.Helper()
	command := exec.Command(branchDefaultGitExecutable, "-C", repositoryPath, "rev-parse", "--abbrev-ref", "HEAD")
	command.Env = buildGitCommandEnvironment(nil)
	output, err := command.CombinedOutput()
	require.NoError(testInstance, err, string(output))
	require.Equal(testInstance, expectedBranch, strings.TrimSpace(string(output)))
}

func assertStateFileBranch(testInstance *testing.T, stateDirectory string, repository string, expectedBranch string) {
	testInstance.Helper()
	key := strings.ReplaceAll(repository, "/", "__")
	statePath := filepath.Join(stateDirectory, key+".txt")
	content, readError := os.ReadFile(statePath)
	require.NoError(testInstance, readError)
	require.Equal(testInstance, expectedBranch, strings.TrimSpace(string(content)))
}

func assertBranchReviewBaseMissing(testInstance *testing.T, repositoryPath string, branchName string) {
	testInstance.Helper()
	command := exec.Command(branchDefaultGitExecutable, "-C", repositoryPath, "config", "--local", "--no-includes", "--get", "branch."+branchName+".gix-review-base")
	command.Env = buildGitCommandEnvironment(nil)
	output, commandError := command.CombinedOutput()
	require.Error(testInstance, commandError, string(output))
	require.Empty(testInstance, strings.TrimSpace(string(output)))
}
