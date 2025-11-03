package remotes_test

import (
	"bytes"
	"context"
	stdErrors "errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	repoerrors "github.com/temirov/gix/internal/repos/errors"
	"github.com/temirov/gix/internal/repos/remotes"
	"github.com/temirov/gix/internal/repos/shared"
)

type stubGitManager struct {
	urlsSet  []string
	setError error
}

func (manager *stubGitManager) CheckCleanWorktree(ctx context.Context, repositoryPath string) (bool, error) {
	return true, nil
}

func (manager *stubGitManager) GetCurrentBranch(ctx context.Context, repositoryPath string) (string, error) {
	return "main", nil
}

func (manager *stubGitManager) GetRemoteURL(ctx context.Context, repositoryPath string, remoteName string) (string, error) {
	return "", nil
}

func (manager *stubGitManager) SetRemoteURL(ctx context.Context, repositoryPath string, remoteName string, remoteURL string) error {
	if manager.setError != nil {
		return manager.setError
	}
	manager.urlsSet = append(manager.urlsSet, remoteURL)
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
	remotesTestRepositoryPath         = "/tmp/project"
	remotesTestCurrentOriginURL       = "https://github.com/origin/example.git"
	remotesTestOriginOwnerRepository  = "origin/example"
	remotesTestCanonicalOwnerRepo     = "canonical/example"
	remotesTestCanonicalURL           = "https://github.com/canonical/example.git"
	remotesTestCanonicalSSHURL        = "ssh://git@github.com/canonical/example.git"
	remotesTestCanonicalSSHDisplayURL = "git@github.com:canonical/example.git"
	remotesTestPlanMessage            = "PLAN-UPDATE-REMOTE: %s origin %s → %s\n"
	remotesTestDeclinedMessage        = "UPDATE-REMOTE-SKIP: user declined for %s\n"
	remotesTestSuccessMessage         = "UPDATE-REMOTE-DONE: %s origin now %s\n"
)

func TestExecutorBehaviors(t *testing.T) {
	repositoryPath, repositoryPathError := shared.NewRepositoryPath(remotesTestRepositoryPath)
	require.NoError(t, repositoryPathError)

	currentOriginURL, currentOriginURLError := shared.NewRemoteURL(remotesTestCurrentOriginURL)
	require.NoError(t, currentOriginURLError)

	originOwnerRepository, originOwnerRepositoryError := shared.NewOwnerRepository(remotesTestOriginOwnerRepository)
	require.NoError(t, originOwnerRepositoryError)

	canonicalOwnerRepository, canonicalOwnerRepositoryError := shared.NewOwnerRepository(remotesTestCanonicalOwnerRepo)
	require.NoError(t, canonicalOwnerRepositoryError)

	testCases := []struct {
		name             string
		options          remotes.Options
		gitManager       *stubGitManager
		prompter         shared.ConfirmationPrompter
		expectedOutput   string
		expectedError    repoerrors.Sentinel
		expectedUpdates  int
		expectedSetURL   string
		expectPromptCall bool
	}{
		{
			name: "skip_missing_origin",
			options: remotes.Options{
				RepositoryPath:           repositoryPath,
				OriginOwnerRepository:    nil,
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				RemoteProtocol:           shared.RemoteProtocolHTTPS,
			},
			gitManager:      &stubGitManager{},
			expectedOutput:  fmt.Sprintf("UPDATE-REMOTE-SKIP: %s (error: could not parse origin owner/repo)\n", remotesTestRepositoryPath),
			expectedError:   repoerrors.ErrOriginOwnerMissing,
			expectedUpdates: 0,
		},
		{
			name: "canonical_missing_returns_error",
			options: remotes.Options{
				RepositoryPath:           repositoryPath,
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: nil,
				RemoteProtocol:           shared.RemoteProtocolHTTPS,
			},
			gitManager:     &stubGitManager{},
			expectedOutput: fmt.Sprintf("UPDATE-REMOTE-SKIP: %s (no upstream: no canonical redirect found)\n", remotesTestRepositoryPath),
			expectedError:  repoerrors.ErrCanonicalOwnerMissing,
		},
		{
			name: "dry_run_plan",
			options: remotes.Options{
				RepositoryPath:           repositoryPath,
				CurrentOriginURL:         cloneRemoteURL(currentOriginURL),
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				RemoteProtocol:           shared.RemoteProtocolHTTPS,
				DryRun:                   true,
			},
			gitManager: &stubGitManager{},
			expectedOutput: fmt.Sprintf(
				remotesTestPlanMessage,
				remotesTestRepositoryPath,
				remotesTestCurrentOriginURL,
				remotesTestCanonicalURL,
			),
		},
		{
			name: "dry_run_plan_ssh_formats_display",
			options: remotes.Options{
				RepositoryPath:           repositoryPath,
				CurrentOriginURL:         cloneRemoteURL(currentOriginURL),
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				RemoteProtocol:           shared.RemoteProtocolSSH,
				DryRun:                   true,
			},
			gitManager: &stubGitManager{},
			expectedOutput: fmt.Sprintf(
				remotesTestPlanMessage,
				remotesTestRepositoryPath,
				remotesTestCurrentOriginURL,
				remotesTestCanonicalSSHDisplayURL,
			),
		},
		{
			name: "prompter_declines",
			options: remotes.Options{
				RepositoryPath:           repositoryPath,
				CurrentOriginURL:         cloneRemoteURL(currentOriginURL),
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				RemoteProtocol:           shared.RemoteProtocolHTTPS,
			},
			gitManager:       &stubGitManager{},
			prompter:         &stubPrompter{result: shared.ConfirmationResult{Confirmed: false}},
			expectedOutput:   fmt.Sprintf(remotesTestDeclinedMessage, remotesTestRepositoryPath),
			expectPromptCall: true,
		},
		{
			name: "prompter_accepts_once",
			options: remotes.Options{
				RepositoryPath:           repositoryPath,
				CurrentOriginURL:         cloneRemoteURL(currentOriginURL),
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				RemoteProtocol:           shared.RemoteProtocolHTTPS,
			},
			gitManager:       &stubGitManager{},
			prompter:         &stubPrompter{result: shared.ConfirmationResult{Confirmed: true}},
			expectedOutput:   fmt.Sprintf(remotesTestSuccessMessage, remotesTestRepositoryPath, remotesTestCanonicalURL),
			expectedUpdates:  1,
			expectPromptCall: true,
		},
		{
			name: "prompter_accepts_all",
			options: remotes.Options{
				RepositoryPath:           repositoryPath,
				CurrentOriginURL:         cloneRemoteURL(currentOriginURL),
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				RemoteProtocol:           shared.RemoteProtocolHTTPS,
			},
			gitManager:       &stubGitManager{},
			prompter:         &stubPrompter{result: shared.ConfirmationResult{Confirmed: true, ApplyToAll: true}},
			expectedOutput:   fmt.Sprintf(remotesTestSuccessMessage, remotesTestRepositoryPath, remotesTestCanonicalURL),
			expectedUpdates:  1,
			expectPromptCall: true,
		},
		{
			name: "prompter_error_returns_contextual_error",
			options: remotes.Options{
				RepositoryPath:           repositoryPath,
				CurrentOriginURL:         cloneRemoteURL(currentOriginURL),
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				RemoteProtocol:           shared.RemoteProtocolHTTPS,
			},
			gitManager:       &stubGitManager{},
			prompter:         &stubPrompter{callError: fmt.Errorf("prompt failed")},
			expectedOutput:   fmt.Sprintf("UPDATE-REMOTE-SKIP: %s (error: could not construct target URL)\n", remotesTestRepositoryPath),
			expectedError:    repoerrors.ErrUserConfirmationFailed,
			expectPromptCall: true,
		},
		{
			name: "assume_yes_updates_without_prompt",
			options: remotes.Options{
				RepositoryPath:           repositoryPath,
				CurrentOriginURL:         cloneRemoteURL(currentOriginURL),
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				RemoteProtocol:           shared.RemoteProtocolHTTPS,
				ConfirmationPolicy:       shared.ConfirmationAssumeYes,
			},
			gitManager:      &stubGitManager{},
			expectedOutput:  fmt.Sprintf(remotesTestSuccessMessage, remotesTestRepositoryPath, remotesTestCanonicalURL),
			expectedUpdates: 1,
		},
		{
			name: "assume_yes_updates_ssh_formats_display",
			options: remotes.Options{
				RepositoryPath:           repositoryPath,
				CurrentOriginURL:         cloneRemoteURL(currentOriginURL),
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				RemoteProtocol:           shared.RemoteProtocolSSH,
				ConfirmationPolicy:       shared.ConfirmationAssumeYes,
			},
			gitManager:      &stubGitManager{},
			expectedOutput:  fmt.Sprintf(remotesTestSuccessMessage, remotesTestRepositoryPath, remotesTestCanonicalSSHDisplayURL),
			expectedUpdates: 1,
			expectedSetURL:  remotesTestCanonicalSSHURL,
		},
		{
			name: "remote_update_failure_returns_error",
			options: remotes.Options{
				RepositoryPath:           repositoryPath,
				CurrentOriginURL:         cloneRemoteURL(currentOriginURL),
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				RemoteProtocol:           shared.RemoteProtocolHTTPS,
			},
			gitManager:     &stubGitManager{setError: fmt.Errorf("update failed")},
			expectedOutput: fmt.Sprintf("UPDATE-REMOTE-SKIP: %s (error: failed to set origin URL)\n", remotesTestRepositoryPath),
			expectedError:  repoerrors.ErrRemoteUpdateFailed,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(testingInstance *testing.T) {
			outputBuffer := &bytes.Buffer{}

			executor := remotes.NewExecutor(remotes.Dependencies{
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
				require.Equal(testingInstance, repoerrors.OperationCanonicalRemote, operationError.Operation())
				require.Equal(testingInstance, remotesTestRepositoryPath, operationError.Subject())
			} else {
				require.NoError(testingInstance, executionError)
			}

			require.Equal(testingInstance, testCase.expectedOutput, outputBuffer.String())

			if testCase.gitManager != nil {
				require.Len(testingInstance, testCase.gitManager.urlsSet, testCase.expectedUpdates)
				if testCase.expectedSetURL != "" && testCase.expectedUpdates > 0 {
					require.Equal(testingInstance, testCase.expectedSetURL, testCase.gitManager.urlsSet[0])
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

func cloneOwnerRepository(value shared.OwnerRepository) *shared.OwnerRepository {
	clone := value
	return &clone
}

func cloneRemoteURL(value shared.RemoteURL) *shared.RemoteURL {
	clone := value
	return &clone
}
