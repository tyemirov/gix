package gitrepo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/gitrepo"
)

const (
	testRepositoryPathConstant                = "/tmp/repo"
	testBranchNameConstant                    = "feature/example"
	testStartPointConstant                    = "origin/main"
	testRemoteNameConstant                    = "origin"
	testRemoteURLConstant                     = "git@github.com:owner/example.git"
	testCleanWorktreeCaseNameConstant         = "clean"
	testDirtyWorktreeCaseNameConstant         = "dirty"
	testWorktreeErrorCaseNameConstant         = "error"
	testValidationCaseNameConstant            = "validation"
	testCheckoutSuccessCaseNameConstant       = "checkout_success"
	testCheckoutErrorCaseNameConstant         = "checkout_error"
	testCreateBranchSuccessCaseNameConstant   = "create_branch_success"
	testCreateBranchWithStartCaseNameConstant = "create_branch_start"
	testCreateBranchErrorCaseNameConstant     = "create_branch_error"
	testDeleteBranchForcedCaseNameConstant    = "delete_branch_forced"
	testDeleteBranchStandardCaseNameConstant  = "delete_branch_standard"
	testDeleteBranchErrorCaseNameConstant     = "delete_branch_error"
	testCurrentBranchSuccessCaseNameConstant  = "current_branch_success"
	testCurrentBranchErrorCaseNameConstant    = "current_branch_error"
	testGetRemoteSuccessCaseNameConstant      = "get_remote_success"
	testGetRemoteErrorCaseNameConstant        = "get_remote_error"
	testSetRemoteSuccessCaseNameConstant      = "set_remote_success"
	testSetRemoteErrorCaseNameConstant        = "set_remote_error"
	testParseRemoteSuccessCaseNameConstant    = "parse_remote_success"
	testParseRemoteErrorCaseNameConstant      = "parse_remote_error"
	testFormatRemoteSuccessCaseNameConstant   = "format_remote_success"
	testFormatRemoteErrorCaseNameConstant     = "format_remote_error"
)

type stubGitExecutor struct {
	executeFunc     func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error)
	recordedDetails []execshell.CommandDetails
}

func (executor *stubGitExecutor) ExecuteGit(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.recordedDetails = append(executor.recordedDetails, details)
	if executor.executeFunc != nil {
		return executor.executeFunc(executionContext, details)
	}
	return execshell.ExecutionResult{}, nil
}

func TestNewRepositoryManagerValidation(testInstance *testing.T) {
	testInstance.Run(testValidationCaseNameConstant, func(testInstance *testing.T) {
		manager, creationError := gitrepo.NewRepositoryManager(nil)
		require.Error(testInstance, creationError)
		require.ErrorIs(testInstance, creationError, gitrepo.ErrGitExecutorNotConfigured)
		require.Nil(testInstance, manager)
	})
}

func TestCheckCleanWorktree(testInstance *testing.T) {
	testCases := []struct {
		name        string
		executor    *stubGitExecutor
		expected    bool
		expectError bool
		errorType   any
	}{
		{
			name: testCleanWorktreeCaseNameConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{StandardOutput: ""}, nil
			}},
			expected: true,
		},
		{
			name: testDirtyWorktreeCaseNameConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{StandardOutput: " M file.txt"}, nil
			}},
			expected: false,
		},
		{
			name: testWorktreeErrorCaseNameConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandExecutionError{Command: execshell.ShellCommand{Name: execshell.CommandGit}, Cause: errors.New("failed")}
			}},
			expectError: true,
			errorType:   gitrepo.RepositoryOperationError{},
		},
		{
			name:        testValidationCaseNameConstant,
			executor:    &stubGitExecutor{},
			expectError: true,
			errorType:   gitrepo.InvalidRepositoryInputError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			manager, creationError := gitrepo.NewRepositoryManager(testCase.executor)
			require.NoError(testInstance, creationError)

			clean, checkError := manager.CheckCleanWorktree(context.Background(), func() string {
				if testCase.name == testValidationCaseNameConstant {
					return ""
				}
				return testRepositoryPathConstant
			}())

			if testCase.expectError {
				require.Error(testInstance, checkError)
				require.IsType(testInstance, testCase.errorType, checkError)
			} else {
				require.NoError(testInstance, checkError)
				require.Equal(testInstance, testCase.expected, clean)
				require.Len(testInstance, testCase.executor.recordedDetails, 1)
			}
		})
	}
}

func TestCheckoutBranch(testInstance *testing.T) {
	testCases := []struct {
		name        string
		executor    *stubGitExecutor
		expectError bool
		errorType   any
	}{
		{
			name: testCheckoutSuccessCaseNameConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, nil
			}},
		},
		{
			name: testCheckoutErrorCaseNameConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandExecutionError{Command: execshell.ShellCommand{Name: execshell.CommandGit}, Cause: errors.New("failed")}
			}},
			expectError: true,
			errorType:   gitrepo.RepositoryOperationError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			manager, creationError := gitrepo.NewRepositoryManager(testCase.executor)
			require.NoError(testInstance, creationError)

			executionError := manager.CheckoutBranch(context.Background(), testRepositoryPathConstant, testBranchNameConstant)
			if testCase.expectError {
				require.Error(testInstance, executionError)
				require.IsType(testInstance, testCase.errorType, executionError)
			} else {
				require.NoError(testInstance, executionError)
				require.Len(testInstance, testCase.executor.recordedDetails, 1)
				require.Contains(testInstance, testCase.executor.recordedDetails[0].Arguments, testBranchNameConstant)
			}
		})
	}
}

func TestCreateBranch(testInstance *testing.T) {
	testCases := []struct {
		name        string
		branchName  string
		startPoint  string
		executor    *stubGitExecutor
		expectError bool
		errorType   any
		verify      func(testInstance *testing.T, executor *stubGitExecutor)
	}{
		{
			name:       testCreateBranchSuccessCaseNameConstant,
			branchName: testBranchNameConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, nil
			}},
			verify: func(testInstance *testing.T, executor *stubGitExecutor) {
				require.Len(testInstance, executor.recordedDetails, 1)
				require.Contains(testInstance, executor.recordedDetails[0].Arguments, testBranchNameConstant)
			},
		},
		{
			name:       testCreateBranchWithStartCaseNameConstant,
			branchName: testBranchNameConstant,
			startPoint: testStartPointConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, nil
			}},
			verify: func(testInstance *testing.T, executor *stubGitExecutor) {
				require.Len(testInstance, executor.recordedDetails, 1)
				require.Contains(testInstance, executor.recordedDetails[0].Arguments, testStartPointConstant)
			},
		},
		{
			name:       testCreateBranchErrorCaseNameConstant,
			branchName: testBranchNameConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandExecutionError{Command: execshell.ShellCommand{Name: execshell.CommandGit}, Cause: errors.New("failed")}
			}},
			expectError: true,
			errorType:   gitrepo.RepositoryOperationError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			manager, creationError := gitrepo.NewRepositoryManager(testCase.executor)
			require.NoError(testInstance, creationError)

			executionError := manager.CreateBranch(context.Background(), testRepositoryPathConstant, testCase.branchName, testCase.startPoint)
			if testCase.expectError {
				require.Error(testInstance, executionError)
				require.IsType(testInstance, testCase.errorType, executionError)
			} else {
				require.NoError(testInstance, executionError)
				require.NotNil(testInstance, testCase.verify)
				testCase.verify(testInstance, testCase.executor)
			}
		})
	}
}

func TestDeleteBranch(testInstance *testing.T) {
	testCases := []struct {
		name        string
		forceDelete bool
		executor    *stubGitExecutor
		expectError bool
		errorType   any
	}{
		{
			name:        testDeleteBranchForcedCaseNameConstant,
			forceDelete: true,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, nil
			}},
		},
		{
			name: testDeleteBranchStandardCaseNameConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, nil
			}},
		},
		{
			name: testDeleteBranchErrorCaseNameConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandExecutionError{Command: execshell.ShellCommand{Name: execshell.CommandGit}, Cause: errors.New("failed")}
			}},
			expectError: true,
			errorType:   gitrepo.RepositoryOperationError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			manager, creationError := gitrepo.NewRepositoryManager(testCase.executor)
			require.NoError(testInstance, creationError)

			executionError := manager.DeleteBranch(context.Background(), testRepositoryPathConstant, testBranchNameConstant, testCase.forceDelete)
			if testCase.expectError {
				require.Error(testInstance, executionError)
				require.IsType(testInstance, testCase.errorType, executionError)
			} else {
				require.NoError(testInstance, executionError)
				require.Len(testInstance, testCase.executor.recordedDetails, 1)
				if testCase.forceDelete {
					require.Contains(testInstance, testCase.executor.recordedDetails[0].Arguments, "--force")
				}
			}
		})
	}
}

func TestGetCurrentBranch(testInstance *testing.T) {
	testCases := []struct {
		name        string
		executor    *stubGitExecutor
		expectError bool
		errorType   any
		expected    string
	}{
		{
			name: testCurrentBranchSuccessCaseNameConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{StandardOutput: "main\n"}, nil
			}},
			expected: "main",
		},
		{
			name: testCurrentBranchErrorCaseNameConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandExecutionError{Command: execshell.ShellCommand{Name: execshell.CommandGit}, Cause: errors.New("failed")}
			}},
			expectError: true,
			errorType:   gitrepo.RepositoryOperationError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			manager, creationError := gitrepo.NewRepositoryManager(testCase.executor)
			require.NoError(testInstance, creationError)

			branchName, executionError := manager.GetCurrentBranch(context.Background(), testRepositoryPathConstant)
			if testCase.expectError {
				require.Error(testInstance, executionError)
				require.IsType(testInstance, testCase.errorType, executionError)
			} else {
				require.NoError(testInstance, executionError)
				require.Equal(testInstance, testCase.expected, branchName)
			}
		})
	}
}

func TestGetRemoteURL(testInstance *testing.T) {
	testCases := []struct {
		name        string
		executor    *stubGitExecutor
		expectError bool
		errorType   any
		expected    string
	}{
		{
			name: testGetRemoteSuccessCaseNameConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{StandardOutput: testRemoteURLConstant + "\n"}, nil
			}},
			expected: testRemoteURLConstant,
		},
		{
			name: testGetRemoteErrorCaseNameConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandExecutionError{Command: execshell.ShellCommand{Name: execshell.CommandGit}, Cause: errors.New("failed")}
			}},
			expectError: true,
			errorType:   gitrepo.RepositoryOperationError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			manager, creationError := gitrepo.NewRepositoryManager(testCase.executor)
			require.NoError(testInstance, creationError)

			remoteURL, executionError := manager.GetRemoteURL(context.Background(), testRepositoryPathConstant, testRemoteNameConstant)
			if testCase.expectError {
				require.Error(testInstance, executionError)
				require.IsType(testInstance, testCase.errorType, executionError)
			} else {
				require.NoError(testInstance, executionError)
				require.Equal(testInstance, testCase.expected, remoteURL)
			}
		})
	}
}

func TestSetRemoteURL(testInstance *testing.T) {
	testCases := []struct {
		name        string
		executor    *stubGitExecutor
		expectError bool
		errorType   any
	}{
		{
			name: testSetRemoteSuccessCaseNameConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, nil
			}},
		},
		{
			name: testSetRemoteErrorCaseNameConstant,
			executor: &stubGitExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandExecutionError{Command: execshell.ShellCommand{Name: execshell.CommandGit}, Cause: errors.New("failed")}
			}},
			expectError: true,
			errorType:   gitrepo.RepositoryOperationError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			manager, creationError := gitrepo.NewRepositoryManager(testCase.executor)
			require.NoError(testInstance, creationError)

			executionError := manager.SetRemoteURL(context.Background(), testRepositoryPathConstant, testRemoteNameConstant, testRemoteURLConstant)
			if testCase.expectError {
				require.Error(testInstance, executionError)
				require.IsType(testInstance, testCase.errorType, executionError)
			} else {
				require.NoError(testInstance, executionError)
				require.Len(testInstance, testCase.executor.recordedDetails, 1)
				require.Contains(testInstance, testCase.executor.recordedDetails[0].Arguments, testRemoteURLConstant)
			}
		})
	}
}

func TestParseRemoteURL(testInstance *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expected    gitrepo.RemoteURL
		expectError bool
	}{
		{
			name:     testParseRemoteSuccessCaseNameConstant,
			input:    "git@github.com:owner/example.git",
			expected: gitrepo.RemoteURL{Protocol: gitrepo.RemoteProtocolSSH, Host: "github.com", Owner: "owner", Repository: "example"},
		},
		{
			name:     "ssh_scheme_prefix",
			input:    "ssh://git@github.com/owner/example.git",
			expected: gitrepo.RemoteURL{Protocol: gitrepo.RemoteProtocolSSH, Host: "github.com", Owner: "owner", Repository: "example"},
		},
		{
			name:     "https_protocol",
			input:    "https://github.com/owner/example.git",
			expected: gitrepo.RemoteURL{Protocol: gitrepo.RemoteProtocolHTTPS, Host: "github.com", Owner: "owner", Repository: "example"},
		},
		{
			name:        testParseRemoteErrorCaseNameConstant,
			input:       "invalid",
			expectError: true,
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			remoteURL, parseError := gitrepo.ParseRemoteURL(testCase.input)
			if testCase.expectError {
				require.Error(testInstance, parseError)
				require.IsType(testInstance, gitrepo.RemoteURLParseError{}, parseError)
			} else {
				require.NoError(testInstance, parseError)
				require.Equal(testInstance, testCase.expected, remoteURL)
			}
		})
	}
}

func TestFormatRemoteURL(testInstance *testing.T) {
	testCases := []struct {
		name        string
		input       gitrepo.RemoteURL
		expected    string
		expectError bool
		errorType   any
	}{
		{
			name:     testFormatRemoteSuccessCaseNameConstant,
			input:    gitrepo.RemoteURL{Protocol: gitrepo.RemoteProtocolSSH, Host: "github.com", Owner: "owner", Repository: "example"},
			expected: "git@github.com:owner/example.git",
		},
		{
			name:     "https_protocol",
			input:    gitrepo.RemoteURL{Protocol: gitrepo.RemoteProtocolHTTPS, Host: "github.com", Owner: "owner", Repository: "example"},
			expected: "https://github.com/owner/example.git",
		},
		{
			name:        testFormatRemoteErrorCaseNameConstant,
			input:       gitrepo.RemoteURL{Protocol: gitrepo.RemoteProtocolSSH, Host: "", Owner: "owner", Repository: "example"},
			expectError: true,
			errorType:   gitrepo.RemoteURLParseError{},
		},
		{
			name:        "unsupported_protocol",
			input:       gitrepo.RemoteURL{Protocol: gitrepo.RemoteProtocol("git"), Host: "github.com", Owner: "owner", Repository: "example"},
			expectError: true,
			errorType:   gitrepo.UnsupportedProtocolError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			formatted, formatError := gitrepo.FormatRemoteURL(testCase.input)
			if testCase.expectError {
				require.Error(testInstance, formatError)
				require.IsType(testInstance, testCase.errorType, formatError)
			} else {
				require.NoError(testInstance, formatError)
				require.Equal(testInstance, testCase.expected, formatted)
			}
		})
	}
}
