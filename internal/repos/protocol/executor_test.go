package protocol_test

import (
	"bytes"
	"context"
	stdErrors "errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	repoerrors "github.com/temirov/gix/internal/repos/errors"
	"github.com/temirov/gix/internal/repos/protocol"
	"github.com/temirov/gix/internal/repos/shared"
)

type stubGitManager struct {
	currentURL string
	setURLs    []string
	getError   error
	setError   error
}

func (manager *stubGitManager) CheckCleanWorktree(ctx context.Context, repositoryPath string) (bool, error) {
	return true, nil
}

func (manager *stubGitManager) GetCurrentBranch(ctx context.Context, repositoryPath string) (string, error) {
	return "main", nil
}

func (manager *stubGitManager) GetRemoteURL(ctx context.Context, repositoryPath string, remoteName string) (string, error) {
	if manager.getError != nil {
		return "", manager.getError
	}
	return manager.currentURL, nil
}

func (manager *stubGitManager) SetRemoteURL(ctx context.Context, repositoryPath string, remoteName string, remoteURL string) error {
	if manager.setError != nil {
		return manager.setError
	}
	manager.setURLs = append(manager.setURLs, remoteURL)
	return nil
}

type stubPrompter struct {
	result          shared.ConfirmationResult
	callError       error
	recordedPrompts []string
}

func (prompter *stubPrompter) Confirm(prompt string) (shared.ConfirmationResult, error) {
	if prompter == nil {
		return shared.ConfirmationResult{}, nil
	}
	prompter.recordedPrompts = append(prompter.recordedPrompts, prompt)
	if prompter.callError != nil {
		return shared.ConfirmationResult{}, prompter.callError
	}
	return prompter.result, nil
}

const (
	protocolTestRepositoryPath         = "/tmp/project"
	protocolTestOriginOwnerRepo        = "origin/example"
	protocolTestCanonicalOwnerRepo     = "canonical/example"
	protocolTestOriginURL              = "https://github.com/origin/example.git"
	protocolTestGitOriginURL           = "git@github.com:origin/example.git"
	protocolTestTargetURL              = "ssh://git@github.com/canonical/example.git"
	protocolTestOriginTargetURL        = "ssh://git@github.com/origin/example.git"
	protocolTestTargetDisplayURL       = "git@github.com:canonical/example.git"
	protocolTestOriginTargetDisplayURL = "git@github.com:origin/example.git"
	protocolTestPlanMessage            = "PLAN-CONVERT: %s origin %s → %s\n"
	protocolTestDeclinedMessage        = "CONVERT-SKIP: user declined for %s\n"
	protocolTestSuccessMessage         = "CONVERT-DONE: %s origin now %s\n"
)

func TestExecutorBehaviors(t *testing.T) {
	repositoryPath, repositoryPathError := shared.NewRepositoryPath(protocolTestRepositoryPath)
	require.NoError(t, repositoryPathError)

	originOwnerRepository, originOwnerRepositoryError := shared.NewOwnerRepository(protocolTestOriginOwnerRepo)
	require.NoError(t, originOwnerRepositoryError)

	canonicalOwnerRepository, canonicalOwnerRepositoryError := shared.NewOwnerRepository(protocolTestCanonicalOwnerRepo)
	require.NoError(t, canonicalOwnerRepositoryError)

	testCases := []struct {
		name              string
		options           protocol.Options
		gitManager        *stubGitManager
		prompter          shared.ConfirmationPrompter
		expectedOutput    string
		expectedError     repoerrors.Sentinel
		expectedUpdates   int
		expectedTargetURL string
		expectPromptCall  bool
	}{
		{
			name: "owner_repo_missing",
			options: protocol.Options{
				RepositoryPath:           repositoryPath,
				OriginOwnerRepository:    nil,
				CanonicalOwnerRepository: nil,
				CurrentProtocol:          shared.RemoteProtocolHTTPS,
				TargetProtocol:           shared.RemoteProtocolSSH,
			},
			gitManager:    &stubGitManager{currentURL: protocolTestOriginURL},
			expectedError: repoerrors.ErrOriginOwnerMissing,
		},
		{
			name: "current_protocol_mismatch_skips",
			options: protocol.Options{
				RepositoryPath:           repositoryPath,
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				CurrentProtocol:          shared.RemoteProtocolHTTPS,
				TargetProtocol:           shared.RemoteProtocolSSH,
			},
			gitManager: &stubGitManager{currentURL: protocolTestGitOriginURL},
		},
		{
			name: "dry_run_plan",
			options: protocol.Options{
				RepositoryPath:           repositoryPath,
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				CurrentProtocol:          shared.RemoteProtocolHTTPS,
				TargetProtocol:           shared.RemoteProtocolSSH,
				DryRun:                   true,
			},
			gitManager: &stubGitManager{currentURL: protocolTestOriginURL},
			expectedOutput: fmt.Sprintf(
				protocolTestPlanMessage,
				protocolTestRepositoryPath,
				protocolTestOriginURL,
				protocolTestTargetDisplayURL,
			),
		},
		{
			name: "prompter_declines",
			options: protocol.Options{
				RepositoryPath:           repositoryPath,
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				CurrentProtocol:          shared.RemoteProtocolHTTPS,
				TargetProtocol:           shared.RemoteProtocolSSH,
			},
			gitManager:       &stubGitManager{currentURL: protocolTestOriginURL},
			prompter:         &stubPrompter{result: shared.ConfirmationResult{Confirmed: false}},
			expectedOutput:   fmt.Sprintf(protocolTestDeclinedMessage, protocolTestRepositoryPath),
			expectPromptCall: true,
		},
		{
			name: "prompter_accepts_once",
			options: protocol.Options{
				RepositoryPath:           repositoryPath,
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				CurrentProtocol:          shared.RemoteProtocolHTTPS,
				TargetProtocol:           shared.RemoteProtocolSSH,
			},
			gitManager:        &stubGitManager{currentURL: protocolTestOriginURL},
			prompter:          &stubPrompter{result: shared.ConfirmationResult{Confirmed: true}},
			expectedOutput:    fmt.Sprintf(protocolTestSuccessMessage, protocolTestRepositoryPath, protocolTestTargetDisplayURL),
			expectedUpdates:   1,
			expectedTargetURL: protocolTestTargetURL,
			expectPromptCall:  true,
		},
		{
			name: "prompter_accepts_all",
			options: protocol.Options{
				RepositoryPath:           repositoryPath,
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				CurrentProtocol:          shared.RemoteProtocolHTTPS,
				TargetProtocol:           shared.RemoteProtocolSSH,
			},
			gitManager:        &stubGitManager{currentURL: protocolTestOriginURL},
			prompter:          &stubPrompter{result: shared.ConfirmationResult{Confirmed: true, ApplyToAll: true}},
			expectedOutput:    fmt.Sprintf(protocolTestSuccessMessage, protocolTestRepositoryPath, protocolTestTargetDisplayURL),
			expectedUpdates:   1,
			expectedTargetURL: protocolTestTargetURL,
			expectPromptCall:  true,
		},
		{
			name: "prompter_error",
			options: protocol.Options{
				RepositoryPath:           repositoryPath,
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				CurrentProtocol:          shared.RemoteProtocolHTTPS,
				TargetProtocol:           shared.RemoteProtocolSSH,
			},
			gitManager:       &stubGitManager{currentURL: protocolTestOriginURL},
			prompter:         &stubPrompter{callError: fmt.Errorf("prompt failure")},
			expectedError:    repoerrors.ErrUserConfirmationFailed,
			expectPromptCall: true,
		},
		{
			name: "assume_yes_updates_without_prompt",
			options: protocol.Options{
				RepositoryPath:           repositoryPath,
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				CurrentProtocol:          shared.RemoteProtocolHTTPS,
				TargetProtocol:           shared.RemoteProtocolSSH,
				ConfirmationPolicy:       shared.ConfirmationAssumeYes,
			},
			gitManager:        &stubGitManager{currentURL: protocolTestOriginURL},
			expectedOutput:    fmt.Sprintf(protocolTestSuccessMessage, protocolTestRepositoryPath, protocolTestTargetDisplayURL),
			expectedUpdates:   1,
			expectedTargetURL: protocolTestTargetURL,
		},
		{
			name: "origin_owner_fallback_when_canonical_missing",
			options: protocol.Options{
				RepositoryPath:           repositoryPath,
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: nil,
				CurrentProtocol:          shared.RemoteProtocolHTTPS,
				TargetProtocol:           shared.RemoteProtocolSSH,
				ConfirmationPolicy:       shared.ConfirmationAssumeYes,
			},
			gitManager:        &stubGitManager{currentURL: protocolTestOriginURL},
			expectedOutput:    fmt.Sprintf(protocolTestSuccessMessage, protocolTestRepositoryPath, protocolTestOriginTargetDisplayURL),
			expectedUpdates:   1,
			expectedTargetURL: protocolTestOriginTargetURL,
		},
		{
			name: "unknown_target_protocol_errors",
			options: protocol.Options{
				RepositoryPath:           repositoryPath,
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				CurrentProtocol:          shared.RemoteProtocolHTTPS,
				TargetProtocol:           shared.RemoteProtocolOther,
				ConfirmationPolicy:       shared.ConfirmationAssumeYes,
			},
			gitManager:    &stubGitManager{currentURL: protocolTestOriginURL},
			expectedError: repoerrors.ErrUnknownProtocol,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(testingInstance *testing.T) {
			outputBuffer := &bytes.Buffer{}

			executor := protocol.NewExecutor(protocol.Dependencies{
				GitManager: testCase.gitManager,
				Prompter:   testCase.prompter,
				Reporter:   shared.NewWriterReporter(outputBuffer),
			})

			executionError := executor.Execute(context.Background(), testCase.options)

			if testCase.expectedError != "" {
				require.Error(testingInstance, executionError)
				require.True(testingInstance, stdErrors.Is(executionError, testCase.expectedError))

				var operationError repoerrors.OperationError
				require.True(testingInstance, stdErrors.As(executionError, &operationError))
				require.Equal(testingInstance, repoerrors.OperationProtocolConvert, operationError.Operation())
				require.Equal(testingInstance, protocolTestRepositoryPath, operationError.Subject())
			} else {
				require.NoError(testingInstance, executionError)
			}

			require.Equal(testingInstance, testCase.expectedOutput, outputBuffer.String())

			if testCase.gitManager != nil {
				require.Len(testingInstance, testCase.gitManager.setURLs, testCase.expectedUpdates)
				if testCase.expectedTargetURL != "" && testCase.expectedUpdates > 0 {
					require.Equal(testingInstance, testCase.expectedTargetURL, testCase.gitManager.setURLs[0])
				}
			}

			if prompter, ok := testCase.prompter.(*stubPrompter); ok {
				if testCase.expectPromptCall {
					require.NotEmpty(testingInstance, prompter.recordedPrompts)
				} else {
					require.Empty(testingInstance, prompter.recordedPrompts)
				}
			}
		})
	}
}

func TestExecutorPromptsAdvertiseApplyAll(t *testing.T) {
	commandPrompter := &stubPrompter{result: shared.ConfirmationResult{Confirmed: false}}
	gitManager := &stubGitManager{currentURL: protocolTestOriginURL}
	outputBuffer := &bytes.Buffer{}
	dependencies := protocol.Dependencies{
		GitManager: gitManager,
		Prompter:   commandPrompter,
		Reporter:   shared.NewWriterReporter(outputBuffer),
	}
	repositoryPath, repositoryPathError := shared.NewRepositoryPath(protocolTestRepositoryPath)
	require.NoError(t, repositoryPathError)

	originOwnerRepository, originOwnerRepositoryError := shared.NewOwnerRepository(protocolTestOriginOwnerRepo)
	require.NoError(t, originOwnerRepositoryError)

	canonicalOwnerRepository, canonicalOwnerRepositoryError := shared.NewOwnerRepository(protocolTestCanonicalOwnerRepo)
	require.NoError(t, canonicalOwnerRepositoryError)
	options := protocol.Options{
		RepositoryPath:           repositoryPath,
		OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
		CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
		CurrentProtocol:          shared.RemoteProtocolHTTPS,
		TargetProtocol:           shared.RemoteProtocolSSH,
	}
	executor := protocol.NewExecutor(dependencies)
	executionError := executor.Execute(context.Background(), options)
	require.NoError(t, executionError)
	require.Equal(
		t,
		[]string{fmt.Sprintf("Convert 'origin' in '%s' (%s → %s)? [a/N/y] ", protocolTestRepositoryPath, shared.RemoteProtocolHTTPS, shared.RemoteProtocolSSH)},
		commandPrompter.recordedPrompts,
	)
	require.Equal(t, fmt.Sprintf(protocolTestDeclinedMessage, protocolTestRepositoryPath), outputBuffer.String())
}

func cloneOwnerRepository(value shared.OwnerRepository) *shared.OwnerRepository {
	clone := value
	return &clone
}
