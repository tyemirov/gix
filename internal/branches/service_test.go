package branches_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	branches "github.com/tyemirov/gix/internal/branches"
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	testRemoteNameConstant                 = "origin"
	testWorkingDirectoryConstant           = "/tmp/worktree"
	testPullRequestLimitConstant           = 50
	gitCommandLabelConstant                = "git"
	githubCommandLabelConstant             = "gh"
	commandKeySeparatorConstant            = " "
	commandKeyTemplateConstant             = "%s%s%s"
	remoteBranchOutputTemplateConstant     = "%s\trefs/heads/%s\n"
	remoteCommitPlaceholderConstant        = "1111111111111111111111111111111111111111"
	subtestNameTemplateConstant            = "%02d_%s"
	expectedLogMessageTemplateConstant     = "expected log message %s"
	unexpectedLogMessageTemplateConstant   = "unexpected log message %s"
	unexpectedCommandTemplateConstant      = "unexpected %s command: %s"
	deletingRemoteLogMessageConstant       = "Deleting remote branch"
	deletingLocalLogMessageConstant        = "Deleting local branch"
	skippingMissingLogMessageConstant      = "Skipping branch (already gone)"
	deletionDeclinedLogMessageConstant     = "Skipping branch deletion (user declined)"
	pullRequestJSONFieldNameConstant       = "headRefName"
	gitListRemoteSubcommandConstant        = "ls-remote"
	gitHeadsFlagConstant                   = "--heads"
	gitPushSubcommandConstant              = "push"
	gitDeleteFlagConstant                  = "--delete"
	gitBranchSubcommandConstant            = "branch"
	gitForceDeleteFlagConstant             = "-D"
	githubPullRequestSubcommandConstant    = "pr"
	githubListSubcommandConstant           = "list"
	githubStateFlagConstant                = "--state"
	githubClosedStateConstant              = "closed"
	githubJSONFlagConstant                 = "--json"
	githubLimitFlagConstant                = "--limit"
	remoteNameErrorMessageConstant         = "remote name must be provided"
	limitValidationErrorMessageConstant    = "pull request limit must be greater than zero"
	executorNotConfiguredMessageConstant   = "command executor not configured"
	remoteListFailureMessageConstant       = "ls failure"
	pullRequestListFailureMessageConstant  = "gh failure"
	invalidJSONPayloadConstant             = "{invalid"
	remoteListErrorContainsConstant        = "unable to list remote branches"
	pullRequestListErrorContainsConstant   = "unable to list closed pull requests"
	pullRequestDecodeErrorContainsConstant = "unable to decode pull request response"
)

type stubBranchPrompter struct {
	responses       []shared.ConfirmationResult
	errors          []error
	prompts         []string
	defaultResponse shared.ConfirmationResult
	defaultError    error
	index           int
}

func (prompter *stubBranchPrompter) Confirm(prompt string) (shared.ConfirmationResult, error) {
	prompter.prompts = append(prompter.prompts, prompt)
	result := prompter.defaultResponse
	if prompter.index < len(prompter.responses) {
		result = prompter.responses[prompter.index]
	}
	err := prompter.defaultError
	if prompter.index < len(prompter.errors) {
		err = prompter.errors[prompter.index]
	}
	prompter.index++
	return result, err
}

type fakeCommandExecutor struct {
	responses           map[string]fakeCommandResponse
	repositoryResponses map[string]map[string]fakeCommandResponse
	executedCommands    []executedCommandRecord
}

type fakeCommandResponse struct {
	result execshell.ExecutionResult
	err    error
}

type executedCommandRecord struct {
	key              string
	toolName         string
	arguments        []string
	workingDirectory string
}

func (executor *fakeCommandExecutor) ExecuteGit(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return executor.executeCommand(gitCommandLabelConstant, details)
}

func (executor *fakeCommandExecutor) ExecuteGitHubCLI(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return executor.executeCommand(githubCommandLabelConstant, details)
}

func (executor *fakeCommandExecutor) executeCommand(toolName string, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	if executor.responses == nil {
		executor.responses = map[string]fakeCommandResponse{}
	}

	commandKey := buildCommandKey(toolName, details.Arguments)
	executor.executedCommands = append(executor.executedCommands, executedCommandRecord{
		key:              commandKey,
		toolName:         toolName,
		arguments:        append([]string{}, details.Arguments...),
		workingDirectory: details.WorkingDirectory,
	})

	if executor.repositoryResponses != nil {
		if repositoryResponseMap, found := executor.repositoryResponses[details.WorkingDirectory]; found {
			if response, found := repositoryResponseMap[commandKey]; found {
				if response.err != nil {
					return execshell.ExecutionResult{}, response.err
				}
				return response.result, nil
			}
		}
	}

	if response, found := executor.responses[commandKey]; found {
		if response.err != nil {
			return execshell.ExecutionResult{}, response.err
		}
		return response.result, nil
	}

	return execshell.ExecutionResult{}, fmt.Errorf(unexpectedCommandTemplateConstant, toolName, commandKey)
}

func buildCommandKey(toolName string, arguments []string) string {
	return fmt.Sprintf(commandKeyTemplateConstant, toolName, commandKeySeparatorConstant, strings.Join(arguments, commandKeySeparatorConstant))
}

func registerResponse(executor *fakeCommandExecutor, toolName string, arguments []string, result execshell.ExecutionResult, commandError error) {
	if executor.responses == nil {
		executor.responses = map[string]fakeCommandResponse{}
	}
	executor.responses[buildCommandKey(toolName, arguments)] = fakeCommandResponse{result: result, err: commandError}
}

func buildRemoteOutput(branchNames []string) string {
	var builder strings.Builder
	for branchIndex := range branchNames {
		builder.WriteString(fmt.Sprintf(remoteBranchOutputTemplateConstant, remoteCommitPlaceholderConstant, branchNames[branchIndex]))
	}
	return builder.String()
}

func buildPullRequestJSON(branchNames []string) (string, error) {
	type pullRequestPayload struct {
		HeadRefName string `json:"headRefName"`
	}

	payload := make([]pullRequestPayload, 0, len(branchNames))
	for branchIndex := range branchNames {
		payload = append(payload, pullRequestPayload{HeadRefName: branchNames[branchIndex]})
	}

	encodedBytes, encodingError := json.Marshal(payload)
	if encodingError != nil {
		return "", encodingError
	}
	return string(encodedBytes), nil
}

func TestServiceCleanupScenarios(testInstance *testing.T) {
	testCases := []struct {
		name                  string
		remoteBranches        []string
		pullRequestBranches   []string
		options               branches.CleanupOptions
		expectedCommandKeys   []string
		expectedLogMessages   []string
		unexpectedLogMessages []string
		prompter              *stubBranchPrompter
		expectedPrompts       []string
	}{
		{
			name:                "deletes_remote_and_local_branches",
			remoteBranches:      []string{"feature/delete"},
			pullRequestBranches: []string{"feature/delete"},
			options: branches.CleanupOptions{
				RemoteName:       testRemoteNameConstant,
				PullRequestLimit: testPullRequestLimitConstant,
				WorkingDirectory: testWorkingDirectoryConstant,
			},
			expectedCommandKeys: []string{
				buildCommandKey(gitCommandLabelConstant, []string{gitListRemoteSubcommandConstant, gitHeadsFlagConstant, testRemoteNameConstant}),
				buildCommandKey(githubCommandLabelConstant, []string{
					githubPullRequestSubcommandConstant,
					githubListSubcommandConstant,
					githubStateFlagConstant,
					githubClosedStateConstant,
					githubJSONFlagConstant,
					pullRequestJSONFieldNameConstant,
					githubLimitFlagConstant,
					strconv.Itoa(testPullRequestLimitConstant),
				}),
				buildCommandKey(gitCommandLabelConstant, []string{gitPushSubcommandConstant, testRemoteNameConstant, gitDeleteFlagConstant, "feature/delete"}),
				buildCommandKey(gitCommandLabelConstant, []string{gitBranchSubcommandConstant, gitForceDeleteFlagConstant, "feature/delete"}),
			},
			expectedLogMessages:   []string{deletingRemoteLogMessageConstant, deletingLocalLogMessageConstant},
			unexpectedLogMessages: []string{skippingMissingLogMessageConstant},
		},
		{
			name:                "skips_missing_remote_branch",
			remoteBranches:      []string{},
			pullRequestBranches: []string{"feature/missing"},
			options: branches.CleanupOptions{
				RemoteName:       testRemoteNameConstant,
				PullRequestLimit: testPullRequestLimitConstant,
				WorkingDirectory: testWorkingDirectoryConstant,
			},
			expectedCommandKeys: []string{
				buildCommandKey(gitCommandLabelConstant, []string{gitListRemoteSubcommandConstant, gitHeadsFlagConstant, testRemoteNameConstant}),
				buildCommandKey(githubCommandLabelConstant, []string{
					githubPullRequestSubcommandConstant,
					githubListSubcommandConstant,
					githubStateFlagConstant,
					githubClosedStateConstant,
					githubJSONFlagConstant,
					pullRequestJSONFieldNameConstant,
					githubLimitFlagConstant,
					strconv.Itoa(testPullRequestLimitConstant),
				}),
			},
			expectedLogMessages:   []string{skippingMissingLogMessageConstant},
			unexpectedLogMessages: []string{deletingRemoteLogMessageConstant, deletingLocalLogMessageConstant},
		},
		{
			name:                "user_declines_branch_deletion",
			remoteBranches:      []string{"feature/user-decline"},
			pullRequestBranches: []string{"feature/user-decline"},
			options: branches.CleanupOptions{
				RemoteName:       testRemoteNameConstant,
				PullRequestLimit: testPullRequestLimitConstant,
				WorkingDirectory: testWorkingDirectoryConstant,
			},
			expectedCommandKeys: []string{
				buildCommandKey(gitCommandLabelConstant, []string{gitListRemoteSubcommandConstant, gitHeadsFlagConstant, testRemoteNameConstant}),
				buildCommandKey(githubCommandLabelConstant, []string{
					githubPullRequestSubcommandConstant,
					githubListSubcommandConstant,
					githubStateFlagConstant,
					githubClosedStateConstant,
					githubJSONFlagConstant,
					pullRequestJSONFieldNameConstant,
					githubLimitFlagConstant,
					strconv.Itoa(testPullRequestLimitConstant),
				}),
			},
			expectedLogMessages:   []string{deletionDeclinedLogMessageConstant},
			unexpectedLogMessages: []string{deletingRemoteLogMessageConstant, deletingLocalLogMessageConstant},
			prompter:              &stubBranchPrompter{defaultResponse: shared.ConfirmationResult{Confirmed: false}},
			expectedPrompts:       []string{fmt.Sprintf("Delete pull request branch '%s' from remote '%s' and the local repository? [y/N] ", "feature/user-decline", testRemoteNameConstant)},
		},
		{
			name:                "duplicates_are_processed_once",
			remoteBranches:      []string{"feature/duplicate"},
			pullRequestBranches: []string{"feature/duplicate", "feature/duplicate"},
			options: branches.CleanupOptions{
				RemoteName:       testRemoteNameConstant,
				PullRequestLimit: testPullRequestLimitConstant,
				WorkingDirectory: testWorkingDirectoryConstant,
			},
			expectedCommandKeys: []string{
				buildCommandKey(gitCommandLabelConstant, []string{gitListRemoteSubcommandConstant, gitHeadsFlagConstant, testRemoteNameConstant}),
				buildCommandKey(githubCommandLabelConstant, []string{
					githubPullRequestSubcommandConstant,
					githubListSubcommandConstant,
					githubStateFlagConstant,
					githubClosedStateConstant,
					githubJSONFlagConstant,
					pullRequestJSONFieldNameConstant,
					githubLimitFlagConstant,
					strconv.Itoa(testPullRequestLimitConstant),
				}),
				buildCommandKey(gitCommandLabelConstant, []string{gitPushSubcommandConstant, testRemoteNameConstant, gitDeleteFlagConstant, "feature/duplicate"}),
				buildCommandKey(gitCommandLabelConstant, []string{gitBranchSubcommandConstant, gitForceDeleteFlagConstant, "feature/duplicate"}),
			},
			expectedLogMessages:   []string{deletingRemoteLogMessageConstant, deletingLocalLogMessageConstant},
			unexpectedLogMessages: []string{skippingMissingLogMessageConstant},
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		testInstance.Run(fmt.Sprintf(subtestNameTemplateConstant, testCaseIndex, testCase.name), func(testInstance *testing.T) {
			fakeExecutorInstance := &fakeCommandExecutor{}

			remoteOutput := buildRemoteOutput(testCase.remoteBranches)
			pullRequestJSON, jsonError := buildPullRequestJSON(testCase.pullRequestBranches)
			require.NoError(testInstance, jsonError)

			gitListArguments := []string{gitListRemoteSubcommandConstant, gitHeadsFlagConstant, testCase.options.RemoteName}
			registerResponse(fakeExecutorInstance, gitCommandLabelConstant, gitListArguments, execshell.ExecutionResult{StandardOutput: remoteOutput, ExitCode: 0}, nil)

			githubListArguments := []string{
				githubPullRequestSubcommandConstant,
				githubListSubcommandConstant,
				githubStateFlagConstant,
				githubClosedStateConstant,
				githubJSONFlagConstant,
				pullRequestJSONFieldNameConstant,
				githubLimitFlagConstant,
				strconv.Itoa(testCase.options.PullRequestLimit),
			}
			registerResponse(fakeExecutorInstance, githubCommandLabelConstant, githubListArguments, execshell.ExecutionResult{StandardOutput: pullRequestJSON, ExitCode: 0}, nil)

			for branchIndex := range testCase.pullRequestBranches {
				branchName := testCase.pullRequestBranches[branchIndex]
				if len(branchName) == 0 {
					continue
				}
				registerResponse(fakeExecutorInstance, gitCommandLabelConstant, []string{gitPushSubcommandConstant, testCase.options.RemoteName, gitDeleteFlagConstant, branchName}, execshell.ExecutionResult{ExitCode: 0}, nil)
				registerResponse(fakeExecutorInstance, gitCommandLabelConstant, []string{gitBranchSubcommandConstant, gitForceDeleteFlagConstant, branchName}, execshell.ExecutionResult{ExitCode: 0}, nil)
			}

			logCore, observedLogs := observer.New(zap.DebugLevel)
			logger := zap.New(logCore)

			confirmationPrompter := testCase.prompter
			if confirmationPrompter == nil {
				confirmationPrompter = &stubBranchPrompter{defaultResponse: shared.ConfirmationResult{Confirmed: true}}
			}

			service, serviceError := branches.NewService(logger, fakeExecutorInstance, confirmationPrompter)
			require.NoError(testInstance, serviceError)

			cleanupError := service.Cleanup(context.Background(), testCase.options)
			require.NoError(testInstance, cleanupError)

			if testCase.expectedPrompts != nil {
				require.Equal(testInstance, testCase.expectedPrompts, confirmationPrompter.prompts)
			}

			actualCommandKeys := make([]string, 0, len(fakeExecutorInstance.executedCommands))
			for commandIndex := range fakeExecutorInstance.executedCommands {
				actualCommandKeys = append(actualCommandKeys, fakeExecutorInstance.executedCommands[commandIndex].key)
				require.Equal(testInstance, testCase.options.WorkingDirectory, fakeExecutorInstance.executedCommands[commandIndex].workingDirectory)
			}
			require.Equal(testInstance, testCase.expectedCommandKeys, actualCommandKeys)

			loggedEntries := observedLogs.All()
			for expectedIndex := range testCase.expectedLogMessages {
				expectedMessage := testCase.expectedLogMessages[expectedIndex]
				require.True(testInstance, containsLogMessage(loggedEntries, expectedMessage), fmt.Sprintf(expectedLogMessageTemplateConstant, expectedMessage))
			}

			for unexpectedIndex := range testCase.unexpectedLogMessages {
				unexpectedMessage := testCase.unexpectedLogMessages[unexpectedIndex]
				require.False(testInstance, containsLogMessage(loggedEntries, unexpectedMessage), fmt.Sprintf(unexpectedLogMessageTemplateConstant, unexpectedMessage))
			}
		})
	}
}

func containsLogMessage(entries []observer.LoggedEntry, message string) bool {
	for entryIndex := range entries {
		if entries[entryIndex].Message == message {
			return true
		}
	}
	return false
}

func TestServiceCleanupFailures(testInstance *testing.T) {
	testCases := []struct {
		name              string
		configureExecutor func(*fakeCommandExecutor)
		options           branches.CleanupOptions
		expectedError     string
	}{
		{
			name: "remote_name_required",
			configureExecutor: func(executor *fakeCommandExecutor) {
			},
			options: branches.CleanupOptions{
				RemoteName:       "",
				PullRequestLimit: testPullRequestLimitConstant,
			},
			expectedError: remoteNameErrorMessageConstant,
		},
		{
			name: "limit_must_be_positive",
			configureExecutor: func(executor *fakeCommandExecutor) {
			},
			options: branches.CleanupOptions{
				RemoteName:       testRemoteNameConstant,
				PullRequestLimit: 0,
			},
			expectedError: limitValidationErrorMessageConstant,
		},
		{
			name: "remote_listing_failure",
			configureExecutor: func(executor *fakeCommandExecutor) {
				failingArguments := []string{gitListRemoteSubcommandConstant, gitHeadsFlagConstant, testRemoteNameConstant}
				registerResponse(executor, gitCommandLabelConstant, failingArguments, execshell.ExecutionResult{}, errors.New(remoteListFailureMessageConstant))
			},
			options: branches.CleanupOptions{
				RemoteName:       testRemoteNameConstant,
				PullRequestLimit: testPullRequestLimitConstant,
			},
			expectedError: remoteListErrorContainsConstant,
		},
		{
			name: "pull_request_listing_failure",
			configureExecutor: func(executor *fakeCommandExecutor) {
				gitArguments := []string{gitListRemoteSubcommandConstant, gitHeadsFlagConstant, testRemoteNameConstant}
				registerResponse(executor, gitCommandLabelConstant, gitArguments, execshell.ExecutionResult{ExitCode: 0}, nil)

				ghArguments := []string{
					githubPullRequestSubcommandConstant,
					githubListSubcommandConstant,
					githubStateFlagConstant,
					githubClosedStateConstant,
					githubJSONFlagConstant,
					pullRequestJSONFieldNameConstant,
					githubLimitFlagConstant,
					strconv.Itoa(testPullRequestLimitConstant),
				}
				registerResponse(executor, githubCommandLabelConstant, ghArguments, execshell.ExecutionResult{}, errors.New(pullRequestListFailureMessageConstant))
			},
			options: branches.CleanupOptions{
				RemoteName:       testRemoteNameConstant,
				PullRequestLimit: testPullRequestLimitConstant,
			},
			expectedError: pullRequestListErrorContainsConstant,
		},
		{
			name: "pull_request_decoding_failure",
			configureExecutor: func(executor *fakeCommandExecutor) {
				gitArguments := []string{gitListRemoteSubcommandConstant, gitHeadsFlagConstant, testRemoteNameConstant}
				registerResponse(executor, gitCommandLabelConstant, gitArguments, execshell.ExecutionResult{ExitCode: 0}, nil)

				ghArguments := []string{
					githubPullRequestSubcommandConstant,
					githubListSubcommandConstant,
					githubStateFlagConstant,
					githubClosedStateConstant,
					githubJSONFlagConstant,
					pullRequestJSONFieldNameConstant,
					githubLimitFlagConstant,
					strconv.Itoa(testPullRequestLimitConstant),
				}
				registerResponse(executor, githubCommandLabelConstant, ghArguments, execshell.ExecutionResult{StandardOutput: invalidJSONPayloadConstant, ExitCode: 0}, nil)
			},
			options: branches.CleanupOptions{
				RemoteName:       testRemoteNameConstant,
				PullRequestLimit: testPullRequestLimitConstant,
			},
			expectedError: pullRequestDecodeErrorContainsConstant,
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		testInstance.Run(fmt.Sprintf(subtestNameTemplateConstant, testCaseIndex, testCase.name), func(testInstance *testing.T) {
			fakeExecutorInstance := &fakeCommandExecutor{}
			testCase.configureExecutor(fakeExecutorInstance)

			logCore, _ := observer.New(zap.DebugLevel)
			logger := zap.New(logCore)

			service, serviceError := branches.NewService(logger, fakeExecutorInstance, nil)
			require.NoError(testInstance, serviceError)

			cleanupError := service.Cleanup(context.Background(), testCase.options)
			require.Error(testInstance, cleanupError)
			require.Contains(testInstance, cleanupError.Error(), testCase.expectedError)
		})
	}
}

func TestNewServiceRequiresExecutor(testInstance *testing.T) {
	service, serviceError := branches.NewService(zap.NewNop(), nil, nil)
	require.Error(testInstance, serviceError)
	require.Nil(testInstance, service)
	require.EqualError(testInstance, serviceError, executorNotConfiguredMessageConstant)
}
