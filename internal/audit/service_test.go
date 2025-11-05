package audit_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubcli"
)

const currentDirectoryRelativePathConstant = "."

type stubDiscoverer struct {
	repositories []string
}

func (discoverer stubDiscoverer) DiscoverRepositories(roots []string) ([]string, error) {
	return discoverer.repositories, nil
}

type stubGitExecutor struct {
	outputs                  map[string]execshell.ExecutionResult
	panicOnUnexpectedCommand bool
}

func (executor stubGitExecutor) ExecuteGit(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	key := strings.Join(details.Arguments, " ")
	if result, found := executor.outputs[key]; found {
		return result, nil
	}
	if executor.panicOnUnexpectedCommand {
		panic(fmt.Sprintf("unexpected git command: %s", key))
	}
	return execshell.ExecutionResult{}, fmt.Errorf("unexpected git command: %s", key)
}

func (executor stubGitExecutor) ExecuteGitHubCLI(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	key := strings.Join(details.Arguments, " ")
	return execshell.ExecutionResult{}, fmt.Errorf("unexpected github command: %s", key)
}

type stubGitManager struct {
	cleanWorktree       bool
	branchName          string
	remoteURL           string
	panicOnBranchLookup bool
}

func (manager stubGitManager) CheckCleanWorktree(ctx context.Context, repositoryPath string) (bool, error) {
	return manager.cleanWorktree, nil
}

func (manager stubGitManager) WorktreeStatus(ctx context.Context, repositoryPath string) ([]string, error) {
	if manager.cleanWorktree {
		return nil, nil
	}
	return []string{" M placeholder.txt"}, nil
}

func (manager stubGitManager) GetCurrentBranch(ctx context.Context, repositoryPath string) (string, error) {
	if manager.panicOnBranchLookup {
		panic("GetCurrentBranch should not be called during minimal inspection")
	}
	return manager.branchName, nil
}

func (manager stubGitManager) GetRemoteURL(ctx context.Context, repositoryPath string, remoteName string) (string, error) {
	return manager.remoteURL, nil
}

func (manager stubGitManager) SetRemoteURL(ctx context.Context, repositoryPath string, remoteName string, remoteURL string) error {
	return nil
}

type stubGitHubResolver struct {
	metadata githubcli.RepositoryMetadata
	err      error
}

func (resolver stubGitHubResolver) ResolveRepoMetadata(ctx context.Context, repository string) (githubcli.RepositoryMetadata, error) {
	if resolver.err != nil {
		return githubcli.RepositoryMetadata{}, resolver.err
	}
	return resolver.metadata, nil
}

type pathSensitiveGitExecutor struct {
	repositoryPath string
}

func (executor pathSensitiveGitExecutor) ExecuteGit(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	key := strings.Join(details.Arguments, " ")
	if key == "rev-parse --is-inside-work-tree" {
		if filepath.Clean(details.WorkingDirectory) == filepath.Clean(executor.repositoryPath) {
			return execshell.ExecutionResult{StandardOutput: "true"}, nil
		}
		return execshell.ExecutionResult{}, fmt.Errorf("unexpected git command: %s", key)
	}
	return execshell.ExecutionResult{}, fmt.Errorf("unexpected git command: %s", key)
}

func (executor pathSensitiveGitExecutor) ExecuteGitHubCLI(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, fmt.Errorf("unexpected github command: %s", strings.Join(details.Arguments, " "))
}

func TestServiceRunBehaviors(testInstance *testing.T) {
	testCases := []struct {
		name                 string
		options              audit.CommandOptions
		discoverer           audit.RepositoryDiscoverer
		executorOutputs      map[string]execshell.ExecutionResult
		gitManager           audit.GitRepositoryManager
		githubResolver       audit.GitHubMetadataResolver
		expectedOutput       string
		expectedError        string
		panicOnUnexpectedGit bool
	}{
		{
			name: "audit_report_full_depth",
			options: audit.CommandOptions{
				Roots:           []string{"/tmp/example"},
				InspectionDepth: audit.InspectionDepthFull,
			},
			discoverer: stubDiscoverer{repositories: []string{"/tmp/example"}},
			executorOutputs: map[string]execshell.ExecutionResult{
				"rev-parse --is-inside-work-tree": {StandardOutput: "true"},
			},
			gitManager: stubGitManager{
				cleanWorktree: true,
				branchName:    "main",
				remoteURL:     "https://github.com/origin/example.git",
			},
			githubResolver: stubGitHubResolver{
				metadata: githubcli.RepositoryMetadata{
					NameWithOwner: "canonical/example",
					DefaultBranch: "main",
				},
			},
			expectedOutput: "folder_name,final_github_repo,name_matches,remote_default_branch,local_branch,in_sync,remote_protocol,origin_matches_canonical\nexample,canonical/example,yes,main,main,n/a,https,no\n",
			expectedError:  "",
		},
		{
			name: "audit_report_minimal_depth",
			options: audit.CommandOptions{
				Roots:           []string{"/tmp/example"},
				InspectionDepth: audit.InspectionDepthMinimal,
			},
			discoverer: stubDiscoverer{repositories: []string{"/tmp/example"}},
			executorOutputs: map[string]execshell.ExecutionResult{
				"rev-parse --is-inside-work-tree": {StandardOutput: "true"},
			},
			gitManager: stubGitManager{
				cleanWorktree:       true,
				branchName:          "feature",
				remoteURL:           "https://github.com/origin/example.git",
				panicOnBranchLookup: true,
			},
			githubResolver: stubGitHubResolver{
				metadata: githubcli.RepositoryMetadata{
					NameWithOwner: "canonical/example",
					DefaultBranch: "main",
				},
			},
			expectedOutput:       "folder_name,final_github_repo,name_matches,remote_default_branch,local_branch,in_sync,remote_protocol,origin_matches_canonical\nexample,canonical/example,yes,main,,n/a,https,no\n",
			expectedError:        "",
			panicOnUnexpectedGit: true,
		},
		{
			name: "audit_debug_full_depth",
			options: audit.CommandOptions{
				DebugOutput:     true,
				Roots:           []string{"/tmp/example"},
				InspectionDepth: audit.InspectionDepthFull,
			},
			discoverer: stubDiscoverer{repositories: []string{"/tmp/example"}},
			executorOutputs: map[string]execshell.ExecutionResult{
				"rev-parse --is-inside-work-tree": {StandardOutput: "true"},
			},
			gitManager: stubGitManager{
				cleanWorktree: true,
				branchName:    "main",
				remoteURL:     "https://github.com/origin/example.git",
			},
			githubResolver: stubGitHubResolver{
				metadata: githubcli.RepositoryMetadata{
					NameWithOwner: "canonical/example",
					DefaultBranch: "main",
				},
			},
			expectedOutput: "folder_name,final_github_repo,name_matches,remote_default_branch,local_branch,in_sync,remote_protocol,origin_matches_canonical\nexample,canonical/example,yes,main,main,n/a,https,no\n",
			expectedError:  "DEBUG: discovered 1 candidate repos under: /tmp/example\nDEBUG: checking /tmp/example\n",
		},
		{
			name: "metadata_skipped_when_resolver_missing",
			options: audit.CommandOptions{
				Roots:           []string{"/tmp/example"},
				InspectionDepth: audit.InspectionDepthMinimal,
			},
			discoverer: stubDiscoverer{repositories: []string{"/tmp/example"}},
			executorOutputs: map[string]execshell.ExecutionResult{
				"rev-parse --is-inside-work-tree": {StandardOutput: "true"},
				"ls-remote --symref origin HEAD":  {StandardOutput: "ref: refs/heads/main\tHEAD\n"},
			},
			gitManager: stubGitManager{
				cleanWorktree: true,
				branchName:    "main",
				remoteURL:     "https://github.com/origin/example.git",
			},
			expectedOutput: "folder_name,final_github_repo,name_matches,remote_default_branch,local_branch,in_sync,remote_protocol,origin_matches_canonical\nexample,origin/example,yes,main,,n/a,https,n/a\n",
			expectedError:  "",
		},
		{
			name: "ignored_nested_repositories_removed",
			options: audit.CommandOptions{
				Roots:           []string{"/tmp/example"},
				InspectionDepth: audit.InspectionDepthMinimal,
			},
			discoverer: stubDiscoverer{repositories: []string{"/tmp/example", "/tmp/example/tools/licenser"}},
			executorOutputs: map[string]execshell.ExecutionResult{
				"check-ignore --stdin":            {StandardOutput: "tools/licenser\n"},
				"rev-parse --is-inside-work-tree": {StandardOutput: "true"},
			},
			gitManager: stubGitManager{
				cleanWorktree: true,
				branchName:    "main",
				remoteURL:     "https://github.com/origin/example.git",
			},
			githubResolver: stubGitHubResolver{
				metadata: githubcli.RepositoryMetadata{
					NameWithOwner: "canonical/example",
					DefaultBranch: "main",
				},
			},
			expectedOutput: "folder_name,final_github_repo,name_matches,remote_default_branch,local_branch,in_sync,remote_protocol,origin_matches_canonical\nexample,canonical/example,yes,main,,n/a,https,no\n",
			expectedError:  "",
		},
	}

	for testCaseIndex, testCase := range testCases {
		testInstance.Run(fmt.Sprintf("%d_%s", testCaseIndex, testCase.name), func(testInstance *testing.T) {
			outputBuffer := &bytes.Buffer{}
			errorBuffer := &bytes.Buffer{}

			service := audit.NewService(
				testCase.discoverer,
				testCase.gitManager,
				stubGitExecutor{outputs: testCase.executorOutputs, panicOnUnexpectedCommand: testCase.panicOnUnexpectedGit},
				testCase.githubResolver,
				outputBuffer,
				errorBuffer,
			)

			runError := service.Run(context.Background(), testCase.options)
			require.NoError(testInstance, runError)
			require.Equal(testInstance, testCase.expectedOutput, outputBuffer.String())
			require.Equal(testInstance, testCase.expectedError, errorBuffer.String())
		})
	}
}

func TestServiceRunNormalizesRepositoryPaths(testInstance *testing.T) {
	testInstance.Helper()

	workingDirectory, workingDirectoryError := os.Getwd()
	require.NoError(testInstance, workingDirectoryError)

	normalizedWorkingDirectory := filepath.Clean(workingDirectory)
	repositoryFolderName := filepath.Base(normalizedWorkingDirectory)

	outputBuffer := &bytes.Buffer{}
	errorBuffer := &bytes.Buffer{}

	service := audit.NewService(
		stubDiscoverer{repositories: []string{currentDirectoryRelativePathConstant}},
		stubGitManager{
			cleanWorktree:       true,
			branchName:          "main",
			remoteURL:           "https://github.com/origin/example.git",
			panicOnBranchLookup: true,
		},
		stubGitExecutor{
			outputs: map[string]execshell.ExecutionResult{
				"rev-parse --is-inside-work-tree": {StandardOutput: "true"},
			},
			panicOnUnexpectedCommand: true,
		},
		stubGitHubResolver{
			metadata: githubcli.RepositoryMetadata{
				NameWithOwner: "canonical/example",
				DefaultBranch: "main",
			},
		},
		outputBuffer,
		errorBuffer,
	)

	options := audit.CommandOptions{
		Roots:           []string{currentDirectoryRelativePathConstant},
		InspectionDepth: audit.InspectionDepthMinimal,
		DebugOutput:     true,
	}

	runError := service.Run(context.Background(), options)
	require.NoError(testInstance, runError)

	expectedNameMatches := "no"
	if repositoryFolderName == "example" {
		expectedNameMatches = "yes"
	}

	expectedCSVOutput := fmt.Sprintf(
		"folder_name,final_github_repo,name_matches,remote_default_branch,local_branch,in_sync,remote_protocol,origin_matches_canonical\n%s,canonical/example,%s,main,,n/a,https,no\n",
		repositoryFolderName,
		expectedNameMatches,
	)
	expectedDebugOutput := fmt.Sprintf(
		"DEBUG: discovered 1 candidate repos under: %s\nDEBUG: checking %s\n",
		currentDirectoryRelativePathConstant,
		normalizedWorkingDirectory,
	)

	require.Equal(testInstance, expectedCSVOutput, outputBuffer.String())
	require.Equal(testInstance, expectedDebugOutput, errorBuffer.String())
}

func TestServiceRunIncludesAllFolders(testInstance *testing.T) {
	testInstance.Helper()

	rootDirectory := testInstance.TempDir()
	gitRepositoryFolderName := "git-project"
	gitRepositoryPath := filepath.Join(rootDirectory, gitRepositoryFolderName)
	require.NoError(testInstance, os.MkdirAll(gitRepositoryPath, 0o755))
	nonRepositoryFolderName := "notes"
	nonRepositoryPath := filepath.Join(rootDirectory, nonRepositoryFolderName)
	require.NoError(testInstance, os.MkdirAll(nonRepositoryPath, 0o755))
	nestedNonRepositoryFolderName := "drafts"
	nestedNonRepositoryPath := filepath.Join(nonRepositoryPath, nestedNonRepositoryFolderName)
	require.NoError(testInstance, os.MkdirAll(nestedNonRepositoryPath, 0o755))

	outputBuffer := &bytes.Buffer{}
	service := audit.NewService(
		stubDiscoverer{repositories: []string{gitRepositoryPath}},
		stubGitManager{
			cleanWorktree:       true,
			branchName:          "main",
			remoteURL:           "https://github.com/origin/example.git",
			panicOnBranchLookup: true,
		},
		pathSensitiveGitExecutor{repositoryPath: gitRepositoryPath},
		stubGitHubResolver{
			metadata: githubcli.RepositoryMetadata{
				NameWithOwner: "canonical/example",
				DefaultBranch: "main",
			},
		},
		outputBuffer,
		&bytes.Buffer{},
	)

	options := audit.CommandOptions{
		Roots:             []string{rootDirectory},
		InspectionDepth:   audit.InspectionDepthMinimal,
		IncludeAllFolders: true,
	}

	runError := service.Run(context.Background(), options)
	require.NoError(testInstance, runError)

	expectedOutput := fmt.Sprintf(
		"folder_name,final_github_repo,name_matches,remote_default_branch,local_branch,in_sync,remote_protocol,origin_matches_canonical\n"+
			"%s,canonical/example,no,main,,n/a,https,no\n"+
			"%s,n/a,n/a,n/a,n/a,n/a,n/a,n/a\n",
		gitRepositoryFolderName,
		nonRepositoryFolderName,
	)
	require.Equal(testInstance, expectedOutput, outputBuffer.String())
	require.NotContains(testInstance, outputBuffer.String(), nestedNonRepositoryFolderName)
}

func TestServiceRunUsesRelativeFolderNames(testInstance *testing.T) {
	testInstance.Helper()

	rootDirectory := testInstance.TempDir()
	relativeFolderPath := filepath.Join("team", "git-project")
	gitRepositoryPath := filepath.Join(rootDirectory, relativeFolderPath)
	require.NoError(testInstance, os.MkdirAll(gitRepositoryPath, 0o755))

	outputBuffer := &bytes.Buffer{}
	service := audit.NewService(
		stubDiscoverer{repositories: []string{gitRepositoryPath}},
		stubGitManager{
			cleanWorktree:       true,
			branchName:          "main",
			remoteURL:           "https://github.com/origin/git-project.git",
			panicOnBranchLookup: true,
		},
		pathSensitiveGitExecutor{repositoryPath: gitRepositoryPath},
		stubGitHubResolver{
			metadata: githubcli.RepositoryMetadata{
				NameWithOwner: "canonical/git-project",
				DefaultBranch: "main",
			},
		},
		outputBuffer,
		&bytes.Buffer{},
	)

	options := audit.CommandOptions{
		Roots:           []string{rootDirectory},
		InspectionDepth: audit.InspectionDepthMinimal,
	}

	runError := service.Run(context.Background(), options)
	require.NoError(testInstance, runError)

	expectedOutput := fmt.Sprintf(
		"folder_name,final_github_repo,name_matches,remote_default_branch,local_branch,in_sync,remote_protocol,origin_matches_canonical\n%s,canonical/git-project,yes,main,,n/a,https,no\n",
		filepath.ToSlash(relativeFolderPath),
	)
	require.Equal(testInstance, expectedOutput, outputBuffer.String())
}
