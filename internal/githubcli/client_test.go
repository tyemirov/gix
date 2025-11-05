package githubcli_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubcli"
)

const (
	testRepositoryIdentifierConstant                     = "owner/example"
	testBaseBranchConstant                               = "main"
	testPullRequestTitleConstant                         = "Example"
	testPullRequestHeadConstant                          = "feature/example"
	testPagesSourceBranchConstant                        = "gh-pages"
	testPagesSourcePathConstant                          = "/docs"
	testTargetBranchConstant                             = "master"
	testResolveSuccessCaseNameConstant                   = "resolve_success"
	testResolveDecodeFailureCaseNameConstant             = "resolve_decode_failure"
	testResolveCommandFailureCaseNameConstant            = "resolve_command_failure"
	testResolveInputFailureCaseNameConstant              = "resolve_input_failure"
	testListSuccessCaseNameConstant                      = "list_success"
	testListDecodeFailureCaseNameConstant                = "list_decode_failure"
	testListCommandFailureCaseNameConstant               = "list_command_failure"
	testListRepositoryValidationCaseNameConstant         = "list_repository_validation"
	testListBaseValidationCaseNameConstant               = "list_base_validation"
	testListStateValidationCaseNameConstant              = "list_state_validation"
	testPagesSuccessCaseNameConstant                     = "pages_success"
	testPagesCommandFailureCaseNameConstant              = "pages_command_failure"
	testPagesRepositoryValidationCaseNameConstant        = "pages_repository_validation"
	testPagesSourceBranchValidationCaseNameConstant      = "pages_source_branch_validation"
	testGetPagesSuccessCaseNameConstant                  = "get_pages_success"
	testGetPagesNullCaseNameConstant                     = "get_pages_null"
	testGetPagesDecodeFailureCaseNameConstant            = "get_pages_decode_failure"
	testGetPagesCommandFailureCaseNameConstant           = "get_pages_command_failure"
	testGetPagesValidationCaseNameConstant               = "get_pages_validation"
	testDefaultBranchSuccessCaseNameConstant             = "default_branch_success"
	testDefaultBranchCommandFailureCaseNameConstant      = "default_branch_command_failure"
	testDefaultBranchValidationCaseNameConstant          = "default_branch_validation"
	testUpdatePullRequestSuccessCaseNameConstant         = "update_pull_request_success"
	testUpdatePullRequestCommandFailureCaseNameConstant  = "update_pull_request_command_failure"
	testUpdatePullRequestValidationCaseNameConstant      = "update_pull_request_validation"
	testCreatePullRequestSuccessCaseNameConstant         = "create_pull_request_success"
	testCreatePullRequestCommandFailureCaseNameConstant  = "create_pull_request_command_failure"
	testCreatePullRequestValidationCaseNameConstant      = "create_pull_request_validation"
	testBranchProtectionProtectedCaseNameConstant        = "branch_protection_protected"
	testBranchProtectionUnprotectedCaseNameConstant      = "branch_protection_unprotected"
	testBranchProtectionUnexpectedStatusCaseNameConstant = "branch_protection_unexpected_status"
	testBranchProtectionCommandFailureCaseNameConstant   = "branch_protection_command_failure"
	testBranchProtectionValidationCaseNameConstant       = "branch_protection_validation"
	testHTTPNotFoundStandardErrorMessageConstant         = "gh: Not Found (HTTP 404)"
	testHTTPForbiddenStandardErrorMessageConstant        = "gh: Forbidden (HTTP 403)"
)

type stubGitHubExecutor struct {
	executeFunc     func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error)
	recordedDetails []execshell.CommandDetails
}

func (executor *stubGitHubExecutor) ExecuteGitHubCLI(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.recordedDetails = append(executor.recordedDetails, details)
	if executor.executeFunc != nil {
		return executor.executeFunc(executionContext, details)
	}
	return execshell.ExecutionResult{}, nil
}

func TestNewClientValidation(testInstance *testing.T) {
	testInstance.Run("nil_executor", func(testInstance *testing.T) {
		client, creationError := githubcli.NewClient(nil)
		require.Error(testInstance, creationError)
		require.ErrorIs(testInstance, creationError, githubcli.ErrExecutorNotConfigured)
		require.Nil(testInstance, client)
	})
}

func TestResolveRepoMetadata(testInstance *testing.T) {
	testCases := []struct {
		name        string
		repository  string
		executor    *stubGitHubExecutor
		expectError bool
		errorType   any
		verify      func(testInstance *testing.T, metadata githubcli.RepositoryMetadata, executor *stubGitHubExecutor)
	}{
		{
			name:       testResolveSuccessCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			executor: &stubGitHubExecutor{
				executeFunc: func(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
					return execshell.ExecutionResult{StandardOutput: `{"nameWithOwner":"owner/example","description":"Example repo","defaultBranchRef":{"name":"main"},"isInOrganization":true}`}, nil
				},
			},
			verify: func(testInstance *testing.T, metadata githubcli.RepositoryMetadata, executor *stubGitHubExecutor) {
				require.Equal(testInstance, "owner/example", metadata.NameWithOwner)
				require.Equal(testInstance, "Example repo", metadata.Description)
				require.Equal(testInstance, "main", metadata.DefaultBranch)
				require.True(testInstance, metadata.IsInOrganization)
				require.Len(testInstance, executor.recordedDetails, 1)
				require.Contains(testInstance, executor.recordedDetails[0].Arguments, testRepositoryIdentifierConstant)
			},
		},
		{
			name:       testResolveDecodeFailureCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{StandardOutput: "not-json"}, nil
			}},
			expectError: true,
			errorType:   githubcli.ResponseDecodingError{},
		},
		{
			name:       testResolveCommandFailureCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandFailedError{Command: execshell.ShellCommand{Name: execshell.CommandGitHub}, Result: execshell.ExecutionResult{ExitCode: 1}}
			}},
			expectError: true,
			errorType:   githubcli.OperationError{},
		},
		{
			name:        testResolveInputFailureCaseNameConstant,
			repository:  "  ",
			executor:    &stubGitHubExecutor{},
			expectError: true,
			errorType:   githubcli.InvalidInputError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			client, creationError := githubcli.NewClient(testCase.executor)
			require.NoError(testInstance, creationError)

			metadata, resolutionError := client.ResolveRepoMetadata(context.Background(), testCase.repository)
			if testCase.expectError {
				require.Error(testInstance, resolutionError)
				require.IsType(testInstance, testCase.errorType, resolutionError)
			} else {
				require.NoError(testInstance, resolutionError)
				require.NotNil(testInstance, testCase.verify)
				testCase.verify(testInstance, metadata, testCase.executor)
			}
		})
	}
}

func TestListPullRequests(testInstance *testing.T) {
	testCases := []struct {
		name        string
		repository  string
		options     githubcli.PullRequestListOptions
		executor    *stubGitHubExecutor
		expectError bool
		errorType   any
		verify      func(testInstance *testing.T, pullRequests []githubcli.PullRequest, executor *stubGitHubExecutor)
	}{
		{
			name:       testListSuccessCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			options: githubcli.PullRequestListOptions{
				State:       githubcli.PullRequestStateOpen,
				BaseBranch:  testBaseBranchConstant,
				ResultLimit: 50,
			},
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{StandardOutput: `[{"number":42,"title":"Example","headRefName":"feature/example"}]`}, nil
			}},
			verify: func(testInstance *testing.T, pullRequests []githubcli.PullRequest, executor *stubGitHubExecutor) {
				require.Len(testInstance, pullRequests, 1)
				require.Equal(testInstance, 42, pullRequests[0].Number)
				require.Equal(testInstance, testPullRequestTitleConstant, pullRequests[0].Title)
				require.Equal(testInstance, testPullRequestHeadConstant, pullRequests[0].HeadRefName)
				require.Len(testInstance, executor.recordedDetails, 1)
				require.Contains(testInstance, executor.recordedDetails[0].Arguments, testRepositoryIdentifierConstant)
			},
		},
		{
			name:       testListDecodeFailureCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			options:    githubcli.PullRequestListOptions{State: githubcli.PullRequestStateOpen, BaseBranch: testBaseBranchConstant},
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{StandardOutput: "not-json"}, nil
			}},
			expectError: true,
			errorType:   githubcli.ResponseDecodingError{},
		},
		{
			name:       testListCommandFailureCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			options:    githubcli.PullRequestListOptions{State: githubcli.PullRequestStateClosed, BaseBranch: testBaseBranchConstant},
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandExecutionError{Command: execshell.ShellCommand{Name: execshell.CommandGitHub}, Cause: errors.New("failed")}
			}},
			expectError: true,
			errorType:   githubcli.OperationError{},
		},
		{
			name:        testListRepositoryValidationCaseNameConstant,
			repository:  "",
			options:     githubcli.PullRequestListOptions{State: githubcli.PullRequestStateOpen, BaseBranch: testBaseBranchConstant},
			executor:    &stubGitHubExecutor{},
			expectError: true,
			errorType:   githubcli.InvalidInputError{},
		},
		{
			name:        testListBaseValidationCaseNameConstant,
			repository:  testRepositoryIdentifierConstant,
			options:     githubcli.PullRequestListOptions{State: githubcli.PullRequestStateOpen, BaseBranch: " "},
			executor:    &stubGitHubExecutor{},
			expectError: true,
			errorType:   githubcli.InvalidInputError{},
		},
		{
			name:        testListStateValidationCaseNameConstant,
			repository:  testRepositoryIdentifierConstant,
			options:     githubcli.PullRequestListOptions{BaseBranch: testBaseBranchConstant},
			executor:    &stubGitHubExecutor{},
			expectError: true,
			errorType:   githubcli.InvalidInputError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			client, creationError := githubcli.NewClient(testCase.executor)
			require.NoError(testInstance, creationError)

			pullRequests, listError := client.ListPullRequests(context.Background(), testCase.repository, testCase.options)
			if testCase.expectError {
				require.Error(testInstance, listError)
				require.IsType(testInstance, testCase.errorType, listError)
			} else {
				require.NoError(testInstance, listError)
				require.NotNil(testInstance, testCase.verify)
				testCase.verify(testInstance, pullRequests, testCase.executor)
			}
		})
	}
}

func TestUpdatePagesConfig(testInstance *testing.T) {
	testCases := []struct {
		name          string
		repository    string
		configuration githubcli.PagesConfiguration
		executor      *stubGitHubExecutor
		expectError   bool
		errorType     any
		verify        func(testInstance *testing.T, executor *stubGitHubExecutor)
	}{
		{
			name:          testPagesSuccessCaseNameConstant,
			repository:    testRepositoryIdentifierConstant,
			configuration: githubcli.PagesConfiguration{SourceBranch: testPagesSourceBranchConstant, SourcePath: testPagesSourcePathConstant},
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, nil
			}},
			verify: func(testInstance *testing.T, executor *stubGitHubExecutor) {
				require.Len(testInstance, executor.recordedDetails, 1)
				require.Equal(testInstance, fmt.Sprintf("repos/%s/pages", testRepositoryIdentifierConstant), executor.recordedDetails[0].Arguments[1])
				require.NotEmpty(testInstance, executor.recordedDetails[0].StandardInput)
			},
		},
		{
			name:          testPagesCommandFailureCaseNameConstant,
			repository:    testRepositoryIdentifierConstant,
			configuration: githubcli.PagesConfiguration{SourceBranch: testPagesSourceBranchConstant},
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandExecutionError{Command: execshell.ShellCommand{Name: execshell.CommandGitHub}, Cause: errors.New("failed")}
			}},
			expectError: true,
			errorType:   githubcli.OperationError{},
		},
		{
			name:          testPagesRepositoryValidationCaseNameConstant,
			repository:    " ",
			configuration: githubcli.PagesConfiguration{SourceBranch: testPagesSourceBranchConstant},
			executor:      &stubGitHubExecutor{},
			expectError:   true,
			errorType:     githubcli.InvalidInputError{},
		},
		{
			name:          testPagesSourceBranchValidationCaseNameConstant,
			repository:    testRepositoryIdentifierConstant,
			configuration: githubcli.PagesConfiguration{SourceBranch: " "},
			executor:      &stubGitHubExecutor{},
			expectError:   true,
			errorType:     githubcli.InvalidInputError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			client, creationError := githubcli.NewClient(testCase.executor)
			require.NoError(testInstance, creationError)

			executionError := client.UpdatePagesConfig(context.Background(), testCase.repository, testCase.configuration)
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

func TestGetPagesConfig(testInstance *testing.T) {
	testCases := []struct {
		name        string
		repository  string
		executor    *stubGitHubExecutor
		expectError bool
		errorType   any
		verify      func(testInstance *testing.T, status githubcli.PagesStatus, executor *stubGitHubExecutor)
	}{
		{
			name:       testGetPagesSuccessCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{StandardOutput: `{"build_type":"legacy","source":{"branch":"main","path":"/"}}`}, nil
			}},
			verify: func(testInstance *testing.T, status githubcli.PagesStatus, executor *stubGitHubExecutor) {
				require.True(testInstance, status.Enabled)
				require.Equal(testInstance, githubcli.PagesBuildTypeLegacy, status.BuildType)
				require.Equal(testInstance, "main", status.SourceBranch)
				require.Equal(testInstance, "/", status.SourcePath)
				require.Len(testInstance, executor.recordedDetails, 1)
			},
		},
		{
			name:       testGetPagesNullCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{StandardOutput: "null"}, nil
			}},
			verify: func(testInstance *testing.T, status githubcli.PagesStatus, executor *stubGitHubExecutor) {
				require.False(testInstance, status.Enabled)
				require.Len(testInstance, executor.recordedDetails, 1)
			},
		},
		{
			name:       testGetPagesDecodeFailureCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{StandardOutput: "{"}, nil
			}},
			expectError: true,
			errorType:   githubcli.ResponseDecodingError{},
		},
		{
			name:       testGetPagesCommandFailureCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandExecutionError{Command: execshell.ShellCommand{Name: execshell.CommandGitHub}, Cause: errors.New("failed")}
			}},
			expectError: true,
			errorType:   githubcli.OperationError{},
		},
		{
			name:        testGetPagesValidationCaseNameConstant,
			repository:  " ",
			executor:    &stubGitHubExecutor{},
			expectError: true,
			errorType:   githubcli.InvalidInputError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			client, creationError := githubcli.NewClient(testCase.executor)
			require.NoError(testInstance, creationError)

			status, getError := client.GetPagesConfig(context.Background(), testCase.repository)
			if testCase.expectError {
				require.Error(testInstance, getError)
				require.IsType(testInstance, testCase.errorType, getError)
			} else {
				require.NoError(testInstance, getError)
				require.NotNil(testInstance, testCase.verify)
				testCase.verify(testInstance, status, testCase.executor)
			}
		})
	}
}

func TestSetDefaultBranch(testInstance *testing.T) {
	testCases := []struct {
		name        string
		repository  string
		branchName  string
		executor    *stubGitHubExecutor
		expectError bool
		errorType   any
		verify      func(testInstance *testing.T, executor *stubGitHubExecutor)
	}{
		{
			name:       testDefaultBranchSuccessCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			branchName: testTargetBranchConstant,
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, nil
			}},
			verify: func(testInstance *testing.T, executor *stubGitHubExecutor) {
				require.Len(testInstance, executor.recordedDetails, 1)
				recorded := executor.recordedDetails[0]
				require.Contains(testInstance, recorded.Arguments, "PATCH")
				require.Contains(testInstance, recorded.Arguments, fmt.Sprintf("default_branch=%s", testTargetBranchConstant))
			},
		},
		{
			name:       testDefaultBranchCommandFailureCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			branchName: testTargetBranchConstant,
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandExecutionError{Command: execshell.ShellCommand{Name: execshell.CommandGitHub}, Cause: errors.New("failed")}
			}},
			expectError: true,
			errorType:   githubcli.OperationError{},
		},
		{
			name:        testDefaultBranchValidationCaseNameConstant,
			repository:  "",
			branchName:  " ",
			executor:    &stubGitHubExecutor{},
			expectError: true,
			errorType:   githubcli.InvalidInputError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			client, creationError := githubcli.NewClient(testCase.executor)
			require.NoError(testInstance, creationError)

			executionError := client.SetDefaultBranch(context.Background(), testCase.repository, testCase.branchName)
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

func TestUpdatePullRequestBase(testInstance *testing.T) {
	testCases := []struct {
		name        string
		repository  string
		number      int
		branchName  string
		executor    *stubGitHubExecutor
		expectError bool
		errorType   any
		verify      func(testInstance *testing.T, executor *stubGitHubExecutor)
	}{
		{
			name:       testUpdatePullRequestSuccessCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			number:     42,
			branchName: testTargetBranchConstant,
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, nil
			}},
			verify: func(testInstance *testing.T, executor *stubGitHubExecutor) {
				require.Len(testInstance, executor.recordedDetails, 1)
				recorded := executor.recordedDetails[0]
				require.Contains(testInstance, recorded.Arguments, "edit")
				require.Contains(testInstance, recorded.Arguments, strconv.Itoa(42))
				require.Contains(testInstance, recorded.Arguments, testTargetBranchConstant)
			},
		},
		{
			name:       testUpdatePullRequestCommandFailureCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			number:     7,
			branchName: testTargetBranchConstant,
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandExecutionError{Command: execshell.ShellCommand{Name: execshell.CommandGitHub}, Cause: errors.New("failed")}
			}},
			expectError: true,
			errorType:   githubcli.OperationError{},
		},
		{
			name:        testUpdatePullRequestValidationCaseNameConstant,
			repository:  "",
			number:      0,
			branchName:  " ",
			executor:    &stubGitHubExecutor{},
			expectError: true,
			errorType:   githubcli.InvalidInputError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			client, creationError := githubcli.NewClient(testCase.executor)
			require.NoError(testInstance, creationError)

			executionError := client.UpdatePullRequestBase(context.Background(), testCase.repository, testCase.number, testCase.branchName)
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

func TestCreatePullRequest(testInstance *testing.T) {
	testCases := []struct {
		name        string
		options     githubcli.PullRequestCreateOptions
		executor    *stubGitHubExecutor
		expectError bool
		errorType   any
		verify      func(testInstance *testing.T, executor *stubGitHubExecutor)
	}{
		{
			name:     testCreatePullRequestSuccessCaseNameConstant,
			executor: &stubGitHubExecutor{},
			options: githubcli.PullRequestCreateOptions{
				Repository: testRepositoryIdentifierConstant,
				Title:      testPullRequestTitleConstant,
				Body:       "Automated update",
				Base:       testBaseBranchConstant,
				Head:       testPullRequestHeadConstant,
				Draft:      true,
			},
			verify: func(testInstance *testing.T, executor *stubGitHubExecutor) {
				require.Len(testInstance, executor.recordedDetails, 1)
				expectedArguments := []string{
					"pr",
					"create",
					"--repo",
					testRepositoryIdentifierConstant,
					"--base",
					testBaseBranchConstant,
					"--head",
					testPullRequestHeadConstant,
					"--title",
					testPullRequestTitleConstant,
					"--body",
					"Automated update",
					"--draft",
				}
				require.Equal(testInstance, expectedArguments, executor.recordedDetails[0].Arguments)
			},
		},
		{
			name: testCreatePullRequestCommandFailureCaseNameConstant,
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandFailedError{Command: execshell.ShellCommand{Name: execshell.CommandGitHub}, Result: execshell.ExecutionResult{ExitCode: 1}}
			}},
			options: githubcli.PullRequestCreateOptions{
				Repository: testRepositoryIdentifierConstant,
				Title:      testPullRequestTitleConstant,
				Body:       "Automated update",
				Base:       testBaseBranchConstant,
				Head:       testPullRequestHeadConstant,
			},
			expectError: true,
			errorType:   githubcli.OperationError{},
		},
		{
			name:        testCreatePullRequestValidationCaseNameConstant,
			executor:    &stubGitHubExecutor{},
			options:     githubcli.PullRequestCreateOptions{},
			expectError: true,
			errorType:   githubcli.InvalidInputError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			client, creationError := githubcli.NewClient(testCase.executor)
			require.NoError(testInstance, creationError)

			executionError := client.CreatePullRequest(context.Background(), testCase.options)
			if testCase.expectError {
				require.Error(testInstance, executionError)
				require.IsType(testInstance, testCase.errorType, executionError)
			} else {
				require.NoError(testInstance, executionError)
				if testCase.verify != nil {
					testCase.verify(testInstance, testCase.executor)
				}
			}
		})
	}
}

func TestCheckBranchProtection(testInstance *testing.T) {
	testCases := []struct {
		name              string
		repository        string
		branchName        string
		executor          *stubGitHubExecutor
		expectedProtected bool
		expectError       bool
		errorType         any
	}{
		{
			name:       testBranchProtectionProtectedCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			branchName: testBaseBranchConstant,
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, nil
			}},
			expectedProtected: true,
		},
		{
			name:       testBranchProtectionUnprotectedCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			branchName: testBaseBranchConstant,
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandFailedError{Command: execshell.ShellCommand{Name: execshell.CommandGitHub}, Result: execshell.ExecutionResult{ExitCode: 1, StandardError: testHTTPNotFoundStandardErrorMessageConstant}}
			}},
			expectedProtected: false,
		},
		{
			name:       testBranchProtectionUnexpectedStatusCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			branchName: testBaseBranchConstant,
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandFailedError{Command: execshell.ShellCommand{Name: execshell.CommandGitHub}, Result: execshell.ExecutionResult{ExitCode: 1, StandardError: testHTTPForbiddenStandardErrorMessageConstant}}
			}},
			expectError: true,
			errorType:   githubcli.OperationError{},
		},
		{
			name:       testBranchProtectionCommandFailureCaseNameConstant,
			repository: testRepositoryIdentifierConstant,
			branchName: testBaseBranchConstant,
			executor: &stubGitHubExecutor{executeFunc: func(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
				return execshell.ExecutionResult{}, execshell.CommandExecutionError{Command: execshell.ShellCommand{Name: execshell.CommandGitHub}, Cause: errors.New("failed")}
			}},
			expectError: true,
			errorType:   githubcli.OperationError{},
		},
		{
			name:        testBranchProtectionValidationCaseNameConstant,
			repository:  "",
			branchName:  " ",
			executor:    &stubGitHubExecutor{},
			expectError: true,
			errorType:   githubcli.InvalidInputError{},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testInstance *testing.T) {
			client, creationError := githubcli.NewClient(testCase.executor)
			require.NoError(testInstance, creationError)

			protected, protectionError := client.CheckBranchProtection(context.Background(), testCase.repository, testCase.branchName)
			if testCase.expectError {
				require.Error(testInstance, protectionError)
				require.IsType(testInstance, testCase.errorType, protectionError)
			} else {
				require.NoError(testInstance, protectionError)
				require.Equal(testInstance, testCase.expectedProtected, protected)
			}
		})
	}
}
