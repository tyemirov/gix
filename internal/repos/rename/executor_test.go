package rename_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	repoerrors "github.com/tyemirov/gix/internal/repos/errors"
	"github.com/tyemirov/gix/internal/repos/rename"
	"github.com/tyemirov/gix/internal/repos/shared"
)

type stubFileSystem struct {
	existingPaths      map[string]bool
	renamedPairs       [][2]string
	createdDirectories []string
	absBase            string
	absError           error
	renameError        error
	fileContents       map[string][]byte
}

func (fileSystem *stubFileSystem) Stat(path string) (fs.FileInfo, error) {
	if fileSystem.existingPaths == nil {
		fileSystem.existingPaths = map[string]bool{}
	}
	if fileSystem.existingPaths[path] {
		return stubFileInfo{}, nil
	}
	return nil, errors.New("not exists")
}

func (fileSystem *stubFileSystem) Rename(oldPath string, newPath string) error {
	if fileSystem.renameError != nil {
		return fileSystem.renameError
	}
	fileSystem.renamedPairs = append(fileSystem.renamedPairs, [2]string{oldPath, newPath})
	if fileSystem.existingPaths == nil {
		fileSystem.existingPaths = map[string]bool{}
	}
	fileSystem.existingPaths[newPath] = true
	delete(fileSystem.existingPaths, oldPath)
	return nil
}

func (fileSystem *stubFileSystem) Abs(path string) (string, error) {
	if fileSystem.absError != nil {
		return "", fileSystem.absError
	}
	if len(fileSystem.absBase) == 0 {
		return path, nil
	}
	return filepath.Join(fileSystem.absBase, filepath.Base(path)), nil
}

func (fileSystem *stubFileSystem) MkdirAll(path string, permissions fs.FileMode) error {
	if fileSystem.existingPaths == nil {
		fileSystem.existingPaths = map[string]bool{}
	}
	fileSystem.createdDirectories = append(fileSystem.createdDirectories, path)
	fileSystem.existingPaths[path] = true
	return nil
}

func (fileSystem *stubFileSystem) ReadFile(path string) ([]byte, error) {
	if fileSystem.fileContents == nil {
		fileSystem.fileContents = map[string][]byte{}
	}
	if contents, exists := fileSystem.fileContents[path]; exists {
		duplicate := make([]byte, len(contents))
		copy(duplicate, contents)
		return duplicate, nil
	}
	return nil, errors.New("not exists")
}

func (fileSystem *stubFileSystem) WriteFile(path string, data []byte, permissions fs.FileMode) error {
	if fileSystem.existingPaths == nil {
		fileSystem.existingPaths = map[string]bool{}
	}
	if fileSystem.fileContents == nil {
		fileSystem.fileContents = map[string][]byte{}
	}
	duplicate := make([]byte, len(data))
	copy(duplicate, data)
	fileSystem.fileContents[path] = duplicate
	fileSystem.existingPaths[path] = true
	return nil
}

type stubFileInfo struct{}

func (stubFileInfo) Name() string       { return "" }
func (stubFileInfo) Size() int64        { return 0 }
func (stubFileInfo) Mode() fs.FileMode  { return 0 }
func (stubFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (stubFileInfo) IsDir() bool        { return true }
func (stubFileInfo) Sys() any           { return nil }

type stubGitManager struct {
	clean        bool
	dirtyEntries []string
}

func (manager stubGitManager) CheckCleanWorktree(ctx context.Context, repositoryPath string) (bool, error) {
	return manager.clean, nil
}

func (manager stubGitManager) WorktreeStatus(ctx context.Context, repositoryPath string) ([]string, error) {
	if manager.clean {
		return nil, nil
	}
	if len(manager.dirtyEntries) > 0 {
		return manager.dirtyEntries, nil
	}
	return []string{" M README.md"}, nil
}

func (manager stubGitManager) GetCurrentBranch(ctx context.Context, repositoryPath string) (string, error) {
	return "", nil
}

func (manager stubGitManager) GetRemoteURL(ctx context.Context, repositoryPath string, remoteName string) (string, error) {
	return "", nil
}

func (manager stubGitManager) SetRemoteURL(ctx context.Context, repositoryPath string, remoteName string, remoteURL string) error {
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

type stubClock struct{}

func (stubClock) Now() time.Time { return time.Unix(0, 0) }

const (
	renameTestRootDirectory          = "/tmp"
	renameTestLegacyFolderPath       = "/tmp/legacy"
	renameTestProjectFolderPath      = "/tmp/project"
	renameTestTargetFolderPath       = "/tmp/example"
	renameTestDesiredFolderName      = "example"
	renameTestOwnerSegment           = "owner"
	renameTestOwnerDesiredFolderName = "owner/example"
	renameTestOwnerDirectoryPath     = "/tmp/owner"
)

type eventExpectation struct {
	code   string
	assert func(*testing.T, map[string]string)
}

func TestExecutorBehaviors(testInstance *testing.T) {
	legacyPath := mustRepositoryPath(testInstance, renameTestLegacyFolderPath)
	projectPath := mustRepositoryPath(testInstance, renameTestProjectFolderPath)
	testCases := []struct {
		name                       string
		options                    rename.Options
		fileSystem                 *stubFileSystem
		gitManager                 shared.GitRepositoryManager
		prompter                   shared.ConfirmationPrompter
		expectedError              repoerrors.Sentinel
		expectedRenames            int
		expectedCreatedDirectories []string
		expectedEvents             []eventExpectation
	}{
		{
			name: "dry_run_plan_ready",
			options: rename.Options{
				RepositoryPath:    legacyPath,
				DesiredFolderName: renameTestDesiredFolderName,
				DryRun:            true,
				CleanPolicy:       shared.CleanWorktreeRequired,
			},
			fileSystem: &stubFileSystem{
				existingPaths: map[string]bool{
					renameTestRootDirectory:    true,
					renameTestTargetFolderPath: false,
				},
			},
			gitManager:      stubGitManager{clean: true},
			expectedRenames: 0,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeFolderPlan,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, renameTestLegacyFolderPath, event["path"])
						require.Equal(t, renameTestTargetFolderPath, event["new_path"])
					},
				},
			},
		},
		{
			name: "dry_run_missing_parent_without_creation",
			options: rename.Options{
				RepositoryPath:          legacyPath,
				DesiredFolderName:       renameTestOwnerDesiredFolderName,
				DryRun:                  true,
				CleanPolicy:             shared.CleanWorktreeRequired,
				EnsureParentDirectories: false,
			},
			fileSystem: &stubFileSystem{
				existingPaths: map[string]bool{
					renameTestRootDirectory: true,
				},
			},
			gitManager:      stubGitManager{clean: true},
			expectedRenames: 0,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeFolderPlan,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, renameTestOwnerDirectoryPath, event["path"])
						require.Equal(t, "parent_missing", event["reason"])
					},
				},
			},
		},
		{
			name: "dry_run_missing_parent_with_creation",
			options: rename.Options{
				RepositoryPath:          legacyPath,
				DesiredFolderName:       renameTestOwnerDesiredFolderName,
				DryRun:                  true,
				CleanPolicy:             shared.CleanWorktreeRequired,
				EnsureParentDirectories: true,
			},
			fileSystem: &stubFileSystem{
				existingPaths: map[string]bool{
					renameTestRootDirectory: true,
				},
			},
			gitManager:      stubGitManager{clean: true},
			expectedRenames: 0,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeFolderPlan,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, renameTestLegacyFolderPath, event["path"])
						require.Equal(t, filepath.Join(renameTestRootDirectory, renameTestOwnerDesiredFolderName), event["new_path"])
					},
				},
			},
		},
		{
			name: "prompter_declines",
			options: rename.Options{
				RepositoryPath:    projectPath,
				DesiredFolderName: renameTestDesiredFolderName,
			},
			fileSystem: &stubFileSystem{
				existingPaths: map[string]bool{
					renameTestRootDirectory:     true,
					renameTestProjectFolderPath: true,
					renameTestTargetFolderPath:  false,
				},
			},
			gitManager:      stubGitManager{clean: true},
			prompter:        &stubPrompter{result: shared.ConfirmationResult{Confirmed: false}},
			expectedRenames: 0,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeFolderSkip,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, renameTestProjectFolderPath, event["path"])
						require.Equal(t, "user_declined", event["reason"])
					},
				},
			},
		},
		{
			name: "prompter_accepts_once",
			options: rename.Options{
				RepositoryPath:    projectPath,
				DesiredFolderName: renameTestDesiredFolderName,
			},
			fileSystem: &stubFileSystem{
				existingPaths: map[string]bool{
					renameTestRootDirectory:     true,
					renameTestProjectFolderPath: true,
				},
			},
			gitManager:      stubGitManager{clean: true},
			prompter:        &stubPrompter{result: shared.ConfirmationResult{Confirmed: true}},
			expectedRenames: 1,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeFolderRename,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, renameTestProjectFolderPath, event["path"])
						require.Equal(t, renameTestProjectFolderPath, event["old_path"])
						require.Equal(t, renameTestTargetFolderPath, event["new_path"])
					},
				},
			},
		},
		{
			name: "prompter_accepts_all",
			options: rename.Options{
				RepositoryPath:    projectPath,
				DesiredFolderName: renameTestDesiredFolderName,
			},
			fileSystem: &stubFileSystem{
				existingPaths: map[string]bool{
					renameTestRootDirectory:     true,
					renameTestProjectFolderPath: true,
				},
			},
			gitManager:      stubGitManager{clean: true},
			prompter:        &stubPrompter{result: shared.ConfirmationResult{Confirmed: true, ApplyToAll: true}},
			expectedRenames: 1,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeFolderRename,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, renameTestProjectFolderPath, event["path"])
						require.Equal(t, renameTestTargetFolderPath, event["new_path"])
					},
				},
			},
		},
		{
			name: "prompter_error",
			options: rename.Options{
				RepositoryPath:    projectPath,
				DesiredFolderName: renameTestDesiredFolderName,
			},
			fileSystem: &stubFileSystem{
				existingPaths: map[string]bool{
					renameTestRootDirectory:     true,
					renameTestProjectFolderPath: true,
				},
			},
			gitManager:      stubGitManager{clean: true},
			prompter:        &stubPrompter{callError: errors.New("read failure")},
			expectedError:   repoerrors.ErrUserConfirmationFailed,
			expectedRenames: 0,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeFolderError,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, renameTestProjectFolderPath, event["path"])
						require.Equal(t, "rename_failed", event["reason"])
					},
				},
			},
		},
		{
			name: "assume_yes_skips_prompt",
			options: rename.Options{
				RepositoryPath:     projectPath,
				DesiredFolderName:  renameTestDesiredFolderName,
				ConfirmationPolicy: shared.ConfirmationAssumeYes,
			},
			fileSystem: &stubFileSystem{
				existingPaths: map[string]bool{
					renameTestRootDirectory:     true,
					renameTestProjectFolderPath: true,
				},
			},
			gitManager:      stubGitManager{clean: true},
			expectedRenames: 1,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeFolderRename,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, renameTestProjectFolderPath, event["path"])
						require.Equal(t, renameTestTargetFolderPath, event["new_path"])
					},
				},
			},
		},
		{
			name: "skip_dirty_worktree",
			options: rename.Options{
				RepositoryPath:     projectPath,
				DesiredFolderName:  renameTestDesiredFolderName,
				CleanPolicy:        shared.CleanWorktreeRequired,
				ConfirmationPolicy: shared.ConfirmationAssumeYes,
			},
			fileSystem: &stubFileSystem{
				existingPaths: map[string]bool{
					renameTestRootDirectory:     true,
					renameTestProjectFolderPath: true,
				},
			},
			gitManager:      stubGitManager{clean: false, dirtyEntries: []string{" M README.md", "?? tmp.txt"}},
			expectedRenames: 0,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeFolderSkip,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, renameTestProjectFolderPath, event["path"])
						require.Equal(t, "dirty_worktree", event["reason"])
						require.Equal(t, "M README.md; ?? tmp.txt", event["dirty_entries"])
					},
				},
			},
		},
		{
			name: "already_normalized_skip",
			options: rename.Options{
				RepositoryPath:     projectPath,
				DesiredFolderName:  filepath.Base(renameTestProjectFolderPath),
				ConfirmationPolicy: shared.ConfirmationAssumeYes,
			},
			fileSystem: &stubFileSystem{
				existingPaths: map[string]bool{
					renameTestRootDirectory:     true,
					renameTestProjectFolderPath: true,
				},
			},
			gitManager:      stubGitManager{clean: true},
			expectedRenames: 0,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeFolderSkip,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, renameTestProjectFolderPath, event["path"])
						require.Equal(t, "already_normalized", event["reason"])
					},
				},
			},
		},
		{
			name: "execute_missing_parent_without_creation",
			options: rename.Options{
				RepositoryPath:          projectPath,
				DesiredFolderName:       renameTestOwnerDesiredFolderName,
				ConfirmationPolicy:      shared.ConfirmationAssumeYes,
				EnsureParentDirectories: false,
			},
			fileSystem: &stubFileSystem{
				existingPaths: map[string]bool{
					renameTestRootDirectory:     true,
					renameTestProjectFolderPath: true,
				},
			},
			gitManager:      stubGitManager{clean: true},
			expectedError:   repoerrors.ErrParentMissing,
			expectedRenames: 0,
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeFolderError,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, renameTestOwnerDirectoryPath, event["path"])
						require.Equal(t, "parent_missing", event["reason"])
					},
				},
			},
		},
		{
			name: "execute_missing_parent_with_creation",
			options: rename.Options{
				RepositoryPath:          projectPath,
				DesiredFolderName:       renameTestOwnerDesiredFolderName,
				ConfirmationPolicy:      shared.ConfirmationAssumeYes,
				EnsureParentDirectories: true,
			},
			fileSystem: &stubFileSystem{
				existingPaths: map[string]bool{
					renameTestRootDirectory:     true,
					renameTestProjectFolderPath: true,
				},
			},
			gitManager:                 stubGitManager{clean: true},
			expectedRenames:            1,
			expectedCreatedDirectories: []string{renameTestOwnerDirectoryPath},
			expectedEvents: []eventExpectation{
				{
					code: shared.EventCodeFolderRename,
					assert: func(t *testing.T, event map[string]string) {
						require.Equal(t, renameTestProjectFolderPath, event["path"])
						require.Equal(t, filepath.Join(renameTestRootDirectory, renameTestOwnerDesiredFolderName), event["new_path"])
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(testingInstance *testing.T) {
			outputBuffer := &bytes.Buffer{}
			reporter := shared.NewStructuredReporter(
				outputBuffer,
				outputBuffer,
				shared.WithRepositoryHeaders(false),
			)
			executor := rename.NewExecutor(rename.Dependencies{
				FileSystem: testCase.fileSystem,
				GitManager: testCase.gitManager,
				Prompter:   testCase.prompter,
				Clock:      stubClock{},
				Reporter:   reporter,
			})

			executionError := executor.Execute(context.Background(), testCase.options)
			if testCase.expectedError != "" {
				require.Error(testingInstance, executionError)
				require.True(testingInstance, errors.Is(executionError, testCase.expectedError))
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

			require.Len(testingInstance, testCase.fileSystem.renamedPairs, testCase.expectedRenames)
			require.Equal(testingInstance, testCase.expectedCreatedDirectories, testCase.fileSystem.createdDirectories)
		})
	}
}

func TestExecutorPromptsAdvertiseApplyAll(testInstance *testing.T) {
	commandPrompter := &stubPrompter{}
	fileSystem := &stubFileSystem{existingPaths: map[string]bool{
		renameTestRootDirectory:     true,
		renameTestProjectFolderPath: true,
		renameTestTargetFolderPath:  false,
	}}
	dependencies := rename.Dependencies{
		FileSystem: fileSystem,
		GitManager: stubGitManager{clean: true},
		Prompter:   commandPrompter,
		Reporter: shared.NewStructuredReporter(
			&bytes.Buffer{},
			&bytes.Buffer{},
			shared.WithRepositoryHeaders(false),
		),
	}
	projectPath := mustRepositoryPath(testInstance, renameTestProjectFolderPath)
	renamer := rename.NewExecutor(dependencies)
	executionError := renamer.Execute(context.Background(), rename.Options{RepositoryPath: projectPath, DesiredFolderName: renameTestDesiredFolderName})
	require.NoError(testInstance, executionError)
	require.Equal(testInstance, []string{fmt.Sprintf("Rename '%s' → '%s'? [a/N/y] ", renameTestProjectFolderPath, renameTestTargetFolderPath)}, commandPrompter.recordedPrompts)
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
		var lastKey string
		for _, field := range fields {
			keyValue := strings.SplitN(field, "=", 2)
			if len(keyValue) == 2 {
				key := keyValue[0]
				value := keyValue[1]
				event[key] = value
				lastKey = key
				continue
			}
			if len(field) == 0 || len(lastKey) == 0 {
				continue
			}
			event[lastKey] = event[lastKey] + " " + field
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

func mustRepositoryPath(testingInstance *testing.T, path string) shared.RepositoryPath {
	result, err := shared.NewRepositoryPath(path)
	require.NoError(testingInstance, err)
	return result
}
