package syncflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/v5/internal/execshell"
)

const strictSyncPreflightTestCommit = "0123456789abcdef0123456789abcdef01234567"

type strictSyncPreflightExecutor struct {
	commands                 []execshell.CommandDetails
	worktreeOutput           string
	worktreeErr              error
	worktreeRepairErr        error
	commonDirectoryResponses map[string][]strictSyncPreflightResponse
	gitPathOutput            string
	gitPathErr               error
	verificationOutput       string
	verificationErr          error
	unmergedOutput           string
	unmergedErr              error
}

type strictSyncPreflightResponse struct {
	Output string
	Error  error
}

func (executor *strictSyncPreflightExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	command := strings.Join(details.Arguments, " ")
	switch {
	case strings.HasPrefix(command, "worktree repair "):
		return execshell.ExecutionResult{}, executor.worktreeRepairErr
	case command == "worktree list --porcelain":
		return execshell.ExecutionResult{StandardOutput: executor.worktreeOutput}, executor.worktreeErr
	case command == "rev-parse --git-common-dir":
		responses := executor.commonDirectoryResponses[details.WorkingDirectory]
		if len(responses) == 0 {
			return execshell.ExecutionResult{}, fmt.Errorf("unexpected common-directory inspection at %s", details.WorkingDirectory)
		}
		response := responses[0]
		executor.commonDirectoryResponses[details.WorkingDirectory] = responses[1:]
		return execshell.ExecutionResult{StandardOutput: response.Output}, response.Error
	case command == "rev-parse --git-path REVERT_HEAD":
		return execshell.ExecutionResult{StandardOutput: executor.gitPathOutput}, executor.gitPathErr
	case strings.HasPrefix(command, "rev-parse --git-path "):
		administrativeName := details.Arguments[len(details.Arguments)-1]
		return execshell.ExecutionResult{StandardOutput: filepath.Join(details.WorkingDirectory, ".git", administrativeName) + "\n"}, nil
	case strings.HasPrefix(command, "rev-parse --verify --quiet --end-of-options "):
		return execshell.ExecutionResult{StandardOutput: executor.verificationOutput}, executor.verificationErr
	case command == "diff --name-only --diff-filter=U":
		return execshell.ExecutionResult{StandardOutput: executor.unmergedOutput}, executor.unmergedErr
	default:
		return execshell.ExecutionResult{}, fmt.Errorf("unexpected Git command: %s", command)
	}
}

func TestStrictSyncUnmergedIndexPreflight(testInstance *testing.T) {
	repositoryPath := testInstance.TempDir()
	executor := &strictSyncPreflightExecutor{unmergedOutput: "README.md\n"}

	preflightErr := ensureWorktreeHasNoOperatorOwnedUnmergedIndex(context.Background(), executor, repositoryPath)

	require.Error(testInstance, preflightErr)
	require.Contains(testInstance, preflightErr.Error(), "operator-owned Git unmerged index")
	require.Contains(testInstance, preflightErr.Error(), strictSyncUnmergedIndexGuidance)
	require.Equal(testInstance, []string{"diff", "--name-only", "--diff-filter=U"}, executor.commands[0].Arguments)
	require.Equal(testInstance, repositoryPath, executor.commands[0].WorkingDirectory)
}

func (executor *strictSyncPreflightExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func TestStrictSyncRevertPreflight(testInstance *testing.T) {
	testCases := []struct {
		Name                 string
		CreateRevertHead     bool
		RevertHeadContents   string
		CreatePathDirectory  bool
		PathError            error
		VerificationOutput   string
		VerificationError    error
		ExpectedPresent      bool
		ExpectedCommandCount int
		ExpectedErrorSamples []string
	}{
		{
			Name:                 "ActiveRevert",
			CreateRevertHead:     true,
			RevertHeadContents:   strictSyncPreflightTestCommit + "\n",
			VerificationOutput:   strictSyncPreflightTestCommit + "\n",
			ExpectedPresent:      true,
			ExpectedCommandCount: 2,
		},
		{
			Name:                 "NoActiveRevert",
			ExpectedCommandCount: 1,
		},
		{
			Name: "PathInspectionFailure",
			PathError: execshell.CommandFailedError{
				Result: execshell.ExecutionResult{
					ExitCode:      128,
					StandardError: "fatal: cannot inspect repository state",
				},
			},
			ExpectedCommandCount: 1,
			ExpectedErrorSamples: []string{
				"cannot inspect repository state",
			},
		},
		{
			Name:                 "UnreadableRevertHead",
			CreatePathDirectory:  true,
			ExpectedCommandCount: 1,
			ExpectedErrorSamples: []string{
				"unexpected filesystem kind",
			},
		},
		{
			Name:               "InvalidRevertHead",
			CreateRevertHead:   true,
			RevertHeadContents: "not-an-object\n",
			VerificationError: execshell.CommandFailedError{
				Result: execshell.ExecutionResult{ExitCode: 1},
			},
			ExpectedCommandCount: 2,
			ExpectedErrorSamples: []string{
				"validate REVERT_HEAD",
			},
		},
		{
			Name:                 "NonCanonicalRevertHead",
			CreateRevertHead:     true,
			RevertHeadContents:   "HEAD\n",
			VerificationOutput:   strictSyncPreflightTestCommit + "\n",
			ExpectedCommandCount: 2,
			ExpectedErrorSamples: []string{
				"canonical commit identifier",
			},
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		testInstance.Run(testCase.Name, func(testInstance *testing.T) {
			repositoryPath := testInstance.TempDir()
			revertHeadPath := filepath.Join(repositoryPath, ".git", "REVERT_HEAD")
			require.NoError(testInstance, os.MkdirAll(filepath.Dir(revertHeadPath), 0o755))
			if testCase.CreatePathDirectory {
				require.NoError(testInstance, os.Mkdir(revertHeadPath, 0o755))
			}
			if testCase.CreateRevertHead {
				require.NoError(testInstance, os.WriteFile(revertHeadPath, []byte(testCase.RevertHeadContents), 0o600))
			}

			executor := &strictSyncPreflightExecutor{
				gitPathOutput:      ".git/REVERT_HEAD\n",
				gitPathErr:         testCase.PathError,
				verificationOutput: testCase.VerificationOutput,
				verificationErr:    testCase.VerificationError,
			}

			operationPresent, preflightErr := inspectStrictSyncGitOperation(
				context.Background(),
				executor,
				repositoryPath,
				strictSyncGitOperationDescriptor{
					Kind:               "revert",
					AdministrativePath: gitRevertHeadReferenceConstant,
					CommitFile:         true,
				},
			)

			if len(testCase.ExpectedErrorSamples) == 0 {
				require.NoError(testInstance, preflightErr)
				require.Equal(testInstance, testCase.ExpectedPresent, operationPresent)
			} else {
				require.Error(testInstance, preflightErr)
				for sampleIndex := range testCase.ExpectedErrorSamples {
					require.Contains(testInstance, preflightErr.Error(), testCase.ExpectedErrorSamples[sampleIndex])
				}
			}
			require.Len(testInstance, executor.commands, testCase.ExpectedCommandCount)
			require.Equal(
				testInstance,
				[]string{"rev-parse", "--git-path", "REVERT_HEAD"},
				executor.commands[0].Arguments,
			)
			require.Equal(testInstance, repositoryPath, executor.commands[0].WorkingDirectory)
			if testCase.ExpectedCommandCount == 2 {
				require.Equal(
					testInstance,
					[]string{
						"rev-parse",
						"--verify",
						"--quiet",
						"--end-of-options",
						strings.TrimSpace(testCase.RevertHeadContents) + "^{commit}",
					},
					executor.commands[1].Arguments,
				)
				require.Equal(testInstance, repositoryPath, executor.commands[1].WorkingDirectory)
			}
		})
	}
}

func TestStrictSyncRevertPreflightInspectsEveryRegisteredWorktree(testInstance *testing.T) {
	repositoryPath := testInstance.TempDir()
	siblingPath := testInstance.TempDir()
	siblingRevertHeadPath := filepath.Join(siblingPath, ".git", "REVERT_HEAD")
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(siblingRevertHeadPath), 0o755))
	require.NoError(testInstance, os.WriteFile(siblingRevertHeadPath, []byte(strictSyncPreflightTestCommit+"\n"), 0o600))

	executor := &strictSyncPreflightExecutor{
		worktreeOutput: fmt.Sprintf(
			"worktree %s\nHEAD %s\nbranch refs/heads/master\n\nworktree %s\nHEAD %s\nbranch refs/heads/feature/revert\n",
			repositoryPath,
			strictSyncPreflightTestCommit,
			siblingPath,
			strictSyncPreflightTestCommit,
		),
		commonDirectoryResponses: map[string][]strictSyncPreflightResponse{
			repositoryPath: {{Output: filepath.Join(repositoryPath, ".git") + "\n"}},
			siblingPath:    {{Output: filepath.Join(repositoryPath, ".git") + "\n"}},
		},
		gitPathOutput:      ".git/REVERT_HEAD\n",
		verificationOutput: strictSyncPreflightTestCommit + "\n",
	}

	_, preflightErr := buildStrictSyncPlan(
		context.Background(),
		executor,
		repositoryPath,
		"feature/revert",
	)

	require.Error(testInstance, preflightErr)
	require.Contains(testInstance, preflightErr.Error(), siblingPath)
	require.Contains(testInstance, preflightErr.Error(), "operator-owned Git revert is in progress")
	require.Equal(testInstance, []string{"worktree", "list", "--porcelain"}, executor.commands[0].Arguments)
	require.Equal(testInstance, repositoryPath, executor.commands[0].WorkingDirectory)
	require.Equal(testInstance, []string{"rev-parse", "--git-common-dir"}, executor.commands[1].Arguments)
	require.Equal(testInstance, repositoryPath, executor.commands[1].WorkingDirectory)
	require.Equal(testInstance, []string{"rev-parse", "--git-common-dir"}, executor.commands[2].Arguments)
	require.Equal(testInstance, siblingPath, executor.commands[2].WorkingDirectory)
	require.Greater(testInstance, len(executor.commands), 6)
	require.Equal(testInstance, siblingPath, executor.commands[len(executor.commands)-2].WorkingDirectory)
	require.Equal(testInstance, siblingPath, executor.commands[len(executor.commands)-1].WorkingDirectory)
}

func TestStrictSyncPreflightRejectsWorktreeRepairFailure(testInstance *testing.T) {
	repositoryPath := testInstance.TempDir()
	siblingPath := testInstance.TempDir()
	missingGitDirectory := filepath.Join(testInstance.TempDir(), "missing", ".git", "worktrees", "sibling")
	require.NoError(
		testInstance,
		os.WriteFile(
			filepath.Join(siblingPath, ".git"),
			[]byte("gitdir: "+missingGitDirectory+"\n"),
			0o600,
		),
	)
	executor := &strictSyncPreflightExecutor{
		worktreeOutput: fmt.Sprintf(
			"worktree %s\nHEAD %s\nbranch refs/heads/master\n\nworktree %s\nHEAD %s\nbranch refs/heads/feature/repair\n",
			repositoryPath,
			strictSyncPreflightTestCommit,
			siblingPath,
			strictSyncPreflightTestCommit,
		),
		worktreeRepairErr: execshell.CommandFailedError{
			Result: execshell.ExecutionResult{
				ExitCode:      128,
				StandardError: "fatal: unable to repair linked worktree",
			},
		},
		commonDirectoryResponses: map[string][]strictSyncPreflightResponse{
			repositoryPath: {{Output: filepath.Join(repositoryPath, ".git") + "\n"}},
			siblingPath: {{
				Error: execshell.CommandFailedError{
					Result: execshell.ExecutionResult{
						ExitCode:      128,
						StandardError: "fatal: not a git repository",
					},
				},
			}},
		},
	}

	_, preflightErr := buildStrictSyncPlan(
		context.Background(),
		executor,
		repositoryPath,
		"master",
	)

	require.Error(testInstance, preflightErr)
	require.Contains(testInstance, preflightErr.Error(), "repair worktree metadata before strict sync")
	require.Contains(testInstance, preflightErr.Error(), repositoryPath)
	require.Contains(testInstance, preflightErr.Error(), "unable to repair linked worktree")
	require.Len(testInstance, executor.commands, 4)
	require.Equal(testInstance, []string{"worktree", "repair", siblingPath}, executor.commands[3].Arguments)
	require.Equal(testInstance, repositoryPath, executor.commands[3].WorkingDirectory)
}

func TestStrictSyncPreflightRejectsWorktreeOwnedByLiveCommonRepository(testInstance *testing.T) {
	repositoryPath := testInstance.TempDir()
	foreignRepositoryPath := testInstance.TempDir()
	siblingPath := testInstance.TempDir()
	executor := &strictSyncPreflightExecutor{
		worktreeOutput: fmt.Sprintf(
			"worktree %s\nHEAD %s\nbranch refs/heads/master\n\nworktree %s\nHEAD %s\nbranch refs/heads/feature/foreign\n",
			repositoryPath,
			strictSyncPreflightTestCommit,
			siblingPath,
			strictSyncPreflightTestCommit,
		),
		commonDirectoryResponses: map[string][]strictSyncPreflightResponse{
			repositoryPath: {{Output: filepath.Join(repositoryPath, ".git") + "\n"}},
			siblingPath:    {{Output: filepath.Join(foreignRepositoryPath, ".git") + "\n"}},
		},
	}

	_, preflightErr := buildStrictSyncPlan(
		context.Background(),
		executor,
		repositoryPath,
		"master",
	)

	require.Error(testInstance, preflightErr)
	require.Contains(testInstance, preflightErr.Error(), siblingPath)
	require.Contains(testInstance, preflightErr.Error(), filepath.Join(foreignRepositoryPath, ".git"))
	require.Contains(testInstance, preflightErr.Error(), filepath.Join(repositoryPath, ".git"))
	require.Contains(testInstance, preflightErr.Error(), "remove the conflicting registration explicitly")
	require.Len(testInstance, executor.commands, 3)
	require.Equal(testInstance, []string{"worktree", "list", "--porcelain"}, executor.commands[0].Arguments)
	require.Equal(testInstance, []string{"rev-parse", "--git-common-dir"}, executor.commands[1].Arguments)
	require.Equal(testInstance, []string{"rev-parse", "--git-common-dir"}, executor.commands[2].Arguments)
}
