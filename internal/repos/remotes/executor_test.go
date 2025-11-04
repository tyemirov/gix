package remotes_test

import (
	"bufio"
	"bytes"
	"context"
	stdErrors "errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	repoerrors "github.com/tyemirov/gix/internal/repos/errors"
	"github.com/tyemirov/gix/internal/repos/remotes"
	"github.com/tyemirov/gix/internal/repos/shared"
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
)

type eventExpectation struct {
	code   string
	assert func(*testing.T, map[string]string)
}

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
		expectedError    repoerrors.Sentinel
		expectedUpdates  int
		expectedSetURL   string
		expectPromptCall bool
		expectedEvents   []eventExpectation
	}{
		{
			name: "skip_missing_origin",
			options: remotes.Options{
				RepositoryPath:           repositoryPath,
				OriginOwnerRepository:    nil,
				CanonicalOwnerRepository: cloneOwnerRepository(canonicalOwnerRepository),
				RemoteProtocol:           shared.RemoteProtocolHTTPS,
			},
			gitManager:    &stubGitManager{},
			expectedError: repoerrors.ErrOriginOwnerMissing,
			expectedEvents: []eventExpectation{{code: shared.EventCodeRemoteSkip, assert: func(t *testing.T, event map[string]string) {
				require.Equal(t, "origin_owner_missing", event["reason"])
				require.Equal(t, remotesTestRepositoryPath, event["path"])
			}}},
		},
		{
			name: "canonical_missing_returns_error",
			options: remotes.Options{
				RepositoryPath:           repositoryPath,
				OriginOwnerRepository:    cloneOwnerRepository(originOwnerRepository),
				CanonicalOwnerRepository: nil,
				RemoteProtocol:           shared.RemoteProtocolHTTPS,
			},
			gitManager:    &stubGitManager{},
			expectedError: repoerrors.ErrCanonicalOwnerMissing,
			expectedEvents: []eventExpectation{{
				code: shared.EventCodeRemoteSkip,
				assert: func(t *testing.T, event map[string]string) {
					require.Equal(t, "canonical_missing", event["reason"])
					require.Equal(t, remotesTestRepositoryPath, event["path"])
				},
			}},
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
			expectedEvents: []eventExpectation{{
				code: shared.EventCodeRemotePlan,
				assert: func(t *testing.T, event map[string]string) {
					require.Equal(t, remotesTestRepositoryPath, event["path"])
					require.Equal(t, remotesTestCanonicalURL, event["target_url"])
					require.Equal(t, string(shared.RemoteProtocolHTTPS), event["protocol"])
				},
			}},
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
			expectedEvents: []eventExpectation{{
				code: shared.EventCodeRemotePlan,
				assert: func(t *testing.T, event map[string]string) {
					require.Equal(t, remotesTestRepositoryPath, event["path"])
					require.Equal(t, remotesTestCanonicalSSHDisplayURL, event["target_url"])
					require.Equal(t, string(shared.RemoteProtocolSSH), event["protocol"])
				},
			}},
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
			expectPromptCall: true,
			expectedEvents: []eventExpectation{{
				code: shared.EventCodeRemoteDeclined,
				assert: func(t *testing.T, event map[string]string) {
					require.Equal(t, remotesTestRepositoryPath, event["path"])
					require.Equal(t, "user_declined", event["reason"])
				},
			}},
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
			expectedUpdates:  1,
			expectPromptCall: true,
			expectedEvents: []eventExpectation{{
				code: shared.EventCodeRemoteUpdate,
				assert: func(t *testing.T, event map[string]string) {
					require.Equal(t, remotesTestRepositoryPath, event["path"])
					require.Equal(t, remotesTestCanonicalURL, event["target_url"])
				},
			}},
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
			expectedUpdates:  1,
			expectPromptCall: true,
			expectedEvents: []eventExpectation{{
				code: shared.EventCodeRemoteUpdate,
				assert: func(t *testing.T, event map[string]string) {
					require.Equal(t, remotesTestRepositoryPath, event["path"])
					require.Equal(t, remotesTestCanonicalURL, event["target_url"])
				},
			}},
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
			expectedError:    repoerrors.ErrUserConfirmationFailed,
			expectPromptCall: true,
			expectedEvents: []eventExpectation{{
				code: shared.EventCodeRemoteSkip,
				assert: func(t *testing.T, event map[string]string) {
					require.Equal(t, "confirmation_error", event["reason"])
					require.Equal(t, remotesTestRepositoryPath, event["path"])
				},
			}},
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
			expectedUpdates: 1,
			expectedEvents: []eventExpectation{{
				code: shared.EventCodeRemoteUpdate,
				assert: func(t *testing.T, event map[string]string) {
					require.Equal(t, remotesTestRepositoryPath, event["path"])
					require.Equal(t, remotesTestCanonicalURL, event["target_url"])
				},
			}},
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
			expectedUpdates: 1,
			expectedSetURL:  remotesTestCanonicalSSHURL,
			expectedEvents: []eventExpectation{{
				code: shared.EventCodeRemoteUpdate,
				assert: func(t *testing.T, event map[string]string) {
					require.Equal(t, remotesTestRepositoryPath, event["path"])
					require.Equal(t, remotesTestCanonicalSSHDisplayURL, event["target_url"])
				},
			}},
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
			gitManager:    &stubGitManager{setError: fmt.Errorf("update failed")},
			expectedError: repoerrors.ErrRemoteUpdateFailed,
			expectedEvents: []eventExpectation{{
				code: shared.EventCodeRemoteSkip,
				assert: func(t *testing.T, event map[string]string) {
					require.Equal(t, "update_failed", event["reason"])
					require.Equal(t, remotesTestRepositoryPath, event["path"])
				},
			}},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(testingInstance *testing.T) {
			outputBuffer := &bytes.Buffer{}
			reporter := shared.NewStructuredReporter(
				outputBuffer,
				outputBuffer,
				shared.WithRepositoryHeaders(false),
			)

			executor := remotes.NewExecutor(remotes.Dependencies{
				GitManager: testCase.gitManager,
				Prompter:   testCase.prompter,
				Reporter:   reporter,
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

			events := parseStructuredEvents(outputBuffer.String())
			require.Len(testingInstance, events, len(testCase.expectedEvents))
			for _, expectation := range testCase.expectedEvents {
				event := requireEventByCode(testingInstance, events, expectation.code)
				if expectation.assert != nil {
					expectation.assert(testingInstance, event)
				}
			}

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

func parseStructuredEvents(output string) []map[string]string {
	scanner := bufio.NewScanner(strings.NewReader(output))
	var events []map[string]string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if len(line) == 0 {
			continue
		}
		if strings.HasPrefix(line, "Summary:") {
			continue
		}
		if strings.HasPrefix(line, "-- ") {
			continue
		}

		parts := strings.Split(line, " | ")
		machinePart := parts[len(parts)-1]
		fields := strings.Fields(machinePart)
		if len(fields) == 0 {
			continue
		}

		event := make(map[string]string, len(fields))
		for _, field := range fields {
			keyValue := strings.SplitN(field, "=", 2)
			if len(keyValue) != 2 {
				continue
			}
			event[keyValue[0]] = keyValue[1]
		}
		if len(event) > 0 {
			events = append(events, event)
		}
	}

	return events
}

func requireEventByCode(t *testing.T, events []map[string]string, code string) map[string]string {
	for _, event := range events {
		if event["event"] == code {
			return event
		}
	}
	require.Failf(t, "event not found", "expected event code %s in %+v", code, events)
	return nil
}
