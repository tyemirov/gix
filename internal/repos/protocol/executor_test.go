package protocol_test

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
	"github.com/tyemirov/gix/internal/repos/protocol"
	"github.com/tyemirov/gix/internal/repos/shared"
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

func (manager *stubGitManager) WorktreeStatus(ctx context.Context, repositoryPath string) ([]string, error) {
	return nil, nil
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
)

type eventExpectation struct {
	code   string
	assert func(*testing.T, map[string]string)
}

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
		expectedError     repoerrors.Sentinel
		expectedUpdates   int
		expectedTargetURL string
		expectPromptCall  bool
		expectedEvents    []eventExpectation
		expectNoEvents    bool
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
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeProtocolSkip,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, "owner_missing", event["reason"])
						require.Equal(t, protocolTestRepositoryPath, event["path"])
					},
				},
			},
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
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeProtocolSkip,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, "protocol_mismatch", event["reason"])
						require.Equal(t, protocolTestRepositoryPath, event["path"])
						require.Equal(t, "git", event["current_protocol"])
						require.Equal(t, "https", event["from_protocol"])
						require.Equal(t, "ssh", event["target_protocol"])
					},
				},
			},
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
			expectPromptCall: true,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeProtocolDeclined,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, protocolTestRepositoryPath, event["path"])
						require.Equal(t, "user_declined", event["reason"])
					},
				},
			},
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
			expectedUpdates:   1,
			expectedTargetURL: protocolTestTargetURL,
			expectPromptCall:  true,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeProtocolUpdate,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, protocolTestRepositoryPath, event["path"])
						require.Equal(t, protocolTestTargetDisplayURL, event["target_url"])
						require.Equal(t, string(shared.RemoteProtocolSSH), event["target_protocol"])
					},
				},
			},
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
			expectedUpdates:   1,
			expectedTargetURL: protocolTestTargetURL,
			expectPromptCall:  true,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeProtocolUpdate,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, protocolTestRepositoryPath, event["path"])
						require.Equal(t, protocolTestTargetDisplayURL, event["target_url"])
					},
				},
			},
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
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeProtocolSkip,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, "confirmation_error", event["reason"])
						require.Equal(t, protocolTestRepositoryPath, event["path"])
					},
				},
			},
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
			expectedUpdates:   1,
			expectedTargetURL: protocolTestTargetURL,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeProtocolUpdate,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, protocolTestRepositoryPath, event["path"])
						require.Equal(t, protocolTestTargetDisplayURL, event["target_url"])
					},
				},
			},
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
			expectedUpdates:   1,
			expectedTargetURL: protocolTestOriginTargetURL,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeProtocolUpdate,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, protocolTestRepositoryPath, event["path"])
						require.Equal(t, protocolTestOriginTargetDisplayURL, event["target_url"])
					},
				},
			},
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
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeProtocolSkip,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, "unknown_protocol", event["reason"])
						require.Equal(t, protocolTestRepositoryPath, event["path"])
					},
				},
			},
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

			executor := protocol.NewExecutor(protocol.Dependencies{
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
				require.Equal(testingInstance, repoerrors.OperationProtocolConvert, operationError.Operation())
				require.Equal(testingInstance, protocolTestRepositoryPath, operationError.Subject())
			} else {
				require.NoError(testingInstance, executionError)
			}

			events := parseStructuredEvents(outputBuffer.String())
			if testCase.expectNoEvents {
				require.Empty(testingInstance, events)
			} else {
				require.Len(testingInstance, events, len(testCase.expectedEvents))
				for _, expectation := range testCase.expectedEvents {
					event := requireEventByCode(testingInstance, events, expectation.code)
					if expectation.assert != nil {
						expectation.assert(testingInstance, event)
					}
				}
			}

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
		Reporter: shared.NewStructuredReporter(
			outputBuffer,
			outputBuffer,
			shared.WithRepositoryHeaders(false),
		),
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
	events := parseStructuredEvents(outputBuffer.String())
	require.Len(t, events, 1)
	event := requireEventByCode(t, events, shared.EventCodeProtocolDeclined)
	require.Equal(t, protocolTestRepositoryPath, event["path"])
	require.Equal(t, "user_declined", event["reason"])
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

func cloneOwnerRepository(value shared.OwnerRepository) *shared.OwnerRepository {
	clone := value
	return &clone
}
