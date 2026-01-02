package tests

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/branches"
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
)

const (
	integrationRemoteDirectoryNameConstant            = "remote.git"
	integrationLocalDirectoryNameConstant             = "workspace"
	integrationGitExecutableNameConstant              = "git"
	integrationGHExecutableNameConstant               = "gh"
	integrationFakeGHDirectoryNameConstant            = "fake_gh"
	integrationInitialFileNameConstant                = "initial.txt"
	integrationInitialFileContentsConstant            = "initial commit contents\n"
	integrationUpdatedFileContentsConstant            = "updated contents\n"
	integrationInitialCommitMessageConstant           = "Initial commit"
	integrationFeatureDeleteCommitMessageConstant     = "Feature delete changes"
	integrationFeatureSkipCommitMessageConstant       = "Feature skip changes"
	integrationFeatureRemoteOnlyCommitMessageConstant = "Feature remote-only changes"
	integrationUserNameConstant                       = "Integration Tester"
	integrationUserEmailConstant                      = "tester@example.com"
	integrationMainBranchNameConstant                 = "main"
	integrationFeatureDeleteBranchConstant            = "feature/delete"
	integrationFeatureSkipBranchConstant              = "feature/skip"
	integrationFeatureMissingBranchConstant           = "feature/missing"
	integrationFeatureRemoteOnlyBranchConstant        = "feature/remote-only"
	integrationRemoteNameConstant                     = "origin"
	integrationPullRequestLimitConstant               = 100
	prCleanupCommandTimeoutConstant                   = 10 * time.Second
	integrationCommandRemoteFlagConstant              = "--remote"
	integrationCommandLimitFlagConstant               = "--limit"
	integrationRootFlagConstant                       = "--" + flagutils.DefaultRootFlagName
	integrationFakeGHPayloadConstant                  = "[{\"headRefName\":\"feature/delete\"},{\"headRefName\":\"feature/missing\"},{\"headRefName\":\"feature/remote-only\"}]"
	integrationEmptyGHPayloadConstant                 = "[]"
	integrationFakeGHScriptTemplateConstant           = "#!/bin/sh\ncat <<'JSON'\n%s\nJSON\n"
	integrationExpectationMessageTemplateConstant     = "expected branch state: %s"
	prCleanupSubtestNameTemplateConstant              = "%d_%s"
	prCleanupOutputDeletedTokenConstant               = "deleted="
	prCleanupOutputClosedZeroTokenConstant            = "closed=0"
	prCleanupOutputFailedZeroTokenConstant            = "failed=0"
)

type automaticConfirmationPrompter struct{}

func (automaticConfirmationPrompter) Confirm(string) (shared.ConfirmationResult, error) {
	return shared.ConfirmationResult{Confirmed: true}, nil
}

func TestPullRequestCleanupIntegration(testInstance *testing.T) {
	temporaryRoot := testInstance.TempDir()
	remoteRepositoryPath := filepath.Join(temporaryRoot, integrationRemoteDirectoryNameConstant)
	localRepositoryPath := filepath.Join(temporaryRoot, integrationLocalDirectoryNameConstant)

	runGitCommand(testInstance, temporaryRoot, []string{integrationGitExecutableNameConstant, "init", "--bare", remoteRepositoryPath})

	runGitCommand(testInstance, temporaryRoot, []string{integrationGitExecutableNameConstant, "init", localRepositoryPath})
	configureLocalRepository(testInstance, localRepositoryPath)

	initialFilePath := filepath.Join(localRepositoryPath, integrationInitialFileNameConstant)
	writeFile(testInstance, initialFilePath, integrationInitialFileContentsConstant)

	runGitCommand(testInstance, localRepositoryPath, []string{integrationGitExecutableNameConstant, "add", integrationInitialFileNameConstant})
	runGitCommand(testInstance, localRepositoryPath, []string{integrationGitExecutableNameConstant, "commit", "-m", integrationInitialCommitMessageConstant})
	runGitCommand(testInstance, localRepositoryPath, []string{integrationGitExecutableNameConstant, "branch", "-M", integrationMainBranchNameConstant})
	runGitCommand(testInstance, localRepositoryPath, []string{integrationGitExecutableNameConstant, "remote", "add", integrationRemoteNameConstant, remoteRepositoryPath})
	runGitCommand(testInstance, localRepositoryPath, []string{integrationGitExecutableNameConstant, "push", "-u", integrationRemoteNameConstant, integrationMainBranchNameConstant})

	createFeatureBranch(testInstance, localRepositoryPath, integrationFeatureDeleteBranchConstant, integrationFeatureDeleteCommitMessageConstant, integrationUpdatedFileContentsConstant)
	runGitCommand(testInstance, localRepositoryPath, []string{integrationGitExecutableNameConstant, "push", integrationRemoteNameConstant, integrationFeatureDeleteBranchConstant})
	runGitCommand(testInstance, localRepositoryPath, []string{integrationGitExecutableNameConstant, "checkout", integrationMainBranchNameConstant})

	createFeatureBranch(testInstance, localRepositoryPath, integrationFeatureSkipBranchConstant, integrationFeatureSkipCommitMessageConstant, integrationUpdatedFileContentsConstant)
	runGitCommand(testInstance, localRepositoryPath, []string{integrationGitExecutableNameConstant, "push", integrationRemoteNameConstant, integrationFeatureSkipBranchConstant})
	runGitCommand(testInstance, localRepositoryPath, []string{integrationGitExecutableNameConstant, "checkout", integrationMainBranchNameConstant})

	createFeatureBranch(testInstance, localRepositoryPath, integrationFeatureRemoteOnlyBranchConstant, integrationFeatureRemoteOnlyCommitMessageConstant, integrationUpdatedFileContentsConstant)
	runGitCommand(testInstance, localRepositoryPath, []string{integrationGitExecutableNameConstant, "push", integrationRemoteNameConstant, integrationFeatureRemoteOnlyBranchConstant})
	runGitCommand(testInstance, localRepositoryPath, []string{integrationGitExecutableNameConstant, "checkout", integrationMainBranchNameConstant})
	runGitCommand(testInstance, localRepositoryPath, []string{integrationGitExecutableNameConstant, "branch", "-D", integrationFeatureRemoteOnlyBranchConstant})

	runGitCommand(testInstance, localRepositoryPath, []string{integrationGitExecutableNameConstant, "branch", integrationFeatureMissingBranchConstant})

	fakeGHDirectoryPath := filepath.Join(temporaryRoot, integrationFakeGHDirectoryNameConstant)
	require.NoError(testInstance, os.MkdirAll(fakeGHDirectoryPath, 0o755))
	fakeGHScriptPath := filepath.Join(fakeGHDirectoryPath, integrationGHExecutableNameConstant)
	scriptContents := fmt.Sprintf(integrationFakeGHScriptTemplateConstant, integrationFakeGHPayloadConstant)
	writeFile(testInstance, fakeGHScriptPath, scriptContents)
	require.NoError(testInstance, os.Chmod(fakeGHScriptPath, 0o755))

	originalPathVariable := os.Getenv("PATH")
	updatedPathVariable := fmt.Sprintf("%s%c%s", fakeGHDirectoryPath, os.PathListSeparator, originalPathVariable)
	require.NoError(testInstance, os.Setenv("PATH", updatedPathVariable))
	defer func() {
		require.NoError(testInstance, os.Setenv("PATH", originalPathVariable))
	}()

	commandRunner := execshell.NewOSCommandRunner()
	commandLogger := zap.NewNop()
	shellExecutor, executorError := execshell.NewShellExecutor(commandLogger, commandRunner, false)
	require.NoError(testInstance, executorError)

	cleanupCommandBuilder := branches.CommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return zap.NewNop()
		},
		GitExecutor: shellExecutor,
		PrompterFactory: func(*cobra.Command) shared.ConfirmationPrompter {
			return automaticConfirmationPrompter{}
		},
	}

	runCleanupCommand := func() string {
		cleanupCommand, buildError := cleanupCommandBuilder.Build()
		require.NoError(testInstance, buildError)

		outputBuffer := &bytes.Buffer{}
		errorBuffer := &bytes.Buffer{}
		cleanupCommand.SetOut(outputBuffer)
		cleanupCommand.SetErr(errorBuffer)

		flagutils.BindRootFlags(cleanupCommand, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

		cleanupCommand.SetContext(context.Background())
		cleanupCommand.SetArgs([]string{
			integrationCommandRemoteFlagConstant,
			integrationRemoteNameConstant,
			integrationCommandLimitFlagConstant,
			strconv.Itoa(integrationPullRequestLimitConstant),
			integrationRootFlagConstant,
			localRepositoryPath,
		})

		executionError := cleanupCommand.Execute()
		require.NoError(testInstance, executionError, errorBuffer.String())
		return strings.TrimSpace(outputBuffer.String())
	}

	initialOutput := runCleanupCommand()
	require.NotEmpty(testInstance, initialOutput)
	require.Contains(testInstance, initialOutput, localRepositoryPath)
	require.Contains(testInstance, initialOutput, prCleanupOutputDeletedTokenConstant)
	require.Contains(testInstance, initialOutput, prCleanupOutputFailedZeroTokenConstant)

	branchExpectations := []struct {
		name        string
		command     []string
		expectEmpty bool
		branchName  string
	}{
		{
			name:        "remote_deleted",
			command:     []string{integrationGitExecutableNameConstant, "ls-remote", "--heads", integrationRemoteNameConstant, integrationFeatureDeleteBranchConstant},
			expectEmpty: true,
			branchName:  integrationFeatureDeleteBranchConstant,
		},
		{
			name:        "remote_remote_only_deleted",
			command:     []string{integrationGitExecutableNameConstant, "ls-remote", "--heads", integrationRemoteNameConstant, integrationFeatureRemoteOnlyBranchConstant},
			expectEmpty: true,
			branchName:  integrationFeatureRemoteOnlyBranchConstant,
		},
		{
			name:        "remote_preserved",
			command:     []string{integrationGitExecutableNameConstant, "ls-remote", "--heads", integrationRemoteNameConstant, integrationFeatureSkipBranchConstant},
			expectEmpty: false,
			branchName:  integrationFeatureSkipBranchConstant,
		},
		{
			name:        "local_deleted",
			command:     []string{integrationGitExecutableNameConstant, "branch", "--list", integrationFeatureDeleteBranchConstant},
			expectEmpty: true,
			branchName:  integrationFeatureDeleteBranchConstant,
		},
		{
			name:        "local_remote_only_deleted",
			command:     []string{integrationGitExecutableNameConstant, "branch", "--list", integrationFeatureRemoteOnlyBranchConstant},
			expectEmpty: true,
			branchName:  integrationFeatureRemoteOnlyBranchConstant,
		},
		{
			name:        "local_missing_branch_retained",
			command:     []string{integrationGitExecutableNameConstant, "branch", "--list", integrationFeatureMissingBranchConstant},
			expectEmpty: false,
			branchName:  integrationFeatureMissingBranchConstant,
		},
	}

	for testCaseIndex, expectation := range branchExpectations {
		testInstance.Run(fmt.Sprintf(prCleanupSubtestNameTemplateConstant, testCaseIndex, expectation.name), func(subtest *testing.T) {
			commandOutput := runGitCommand(subtest, localRepositoryPath, expectation.command)
			trimmedOutput := strings.TrimSpace(commandOutput)
			message := fmt.Sprintf(integrationExpectationMessageTemplateConstant, expectation.branchName)
			if expectation.expectEmpty {
				require.Empty(subtest, trimmedOutput, message)
			} else {
				require.NotEmpty(subtest, trimmedOutput, message)
			}
		})
	}

	emptyPayloadScript := fmt.Sprintf(integrationFakeGHScriptTemplateConstant, integrationEmptyGHPayloadConstant)
	writeFile(testInstance, fakeGHScriptPath, emptyPayloadScript)

	emptyOutput := runCleanupCommand()
	require.NotEmpty(testInstance, emptyOutput)
	require.Contains(testInstance, emptyOutput, localRepositoryPath)
	require.Contains(testInstance, emptyOutput, prCleanupOutputClosedZeroTokenConstant)
}

func configureLocalRepository(testInstance *testing.T, repositoryPath string) {
	runGitCommand(testInstance, repositoryPath, []string{integrationGitExecutableNameConstant, "config", "user.name", integrationUserNameConstant})
	runGitCommand(testInstance, repositoryPath, []string{integrationGitExecutableNameConstant, "config", "user.email", integrationUserEmailConstant})
}

func createFeatureBranch(testInstance *testing.T, repositoryPath string, branchName string, commitMessage string, fileContents string) {
	runGitCommand(testInstance, repositoryPath, []string{integrationGitExecutableNameConstant, "checkout", "-b", branchName})
	writeFile(testInstance, filepath.Join(repositoryPath, integrationInitialFileNameConstant), fileContents)
	runGitCommand(testInstance, repositoryPath, []string{integrationGitExecutableNameConstant, "commit", "-am", commitMessage})
}

func writeFile(testInstance *testing.T, filePath string, contents string) {
	require.NoError(testInstance, os.MkdirAll(filepath.Dir(filePath), 0o755))
	require.NoError(testInstance, os.WriteFile(filePath, []byte(contents), 0o644))
}

func runGitCommand(testInstance *testing.T, workingDirectory string, arguments []string) string {
	executionContext, cancelFunction := context.WithTimeout(context.Background(), prCleanupCommandTimeoutConstant)
	defer cancelFunction()

	command := exec.CommandContext(executionContext, arguments[0], arguments[1:]...)
	if len(workingDirectory) > 0 {
		command.Dir = workingDirectory
	}

	outputBytes, commandError := command.CombinedOutput()
	require.NoError(testInstance, commandError, string(outputBytes))
	return string(outputBytes)
}
