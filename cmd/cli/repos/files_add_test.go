package repos_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repos "github.com/tyemirov/gix/cmd/cli/repos"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	rootutils "github.com/tyemirov/gix/internal/utils/roots"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	filesAddDryRunFlag        = "--" + flagutils.DryRunFlagName
	filesAddAssumeYesFlag     = "--" + flagutils.AssumeYesFlagName
	filesAddRootFlag          = "--" + flagutils.DefaultRootFlagName
	filesAddPathFlag          = "--path"
	filesAddContentFlag       = "--content"
	filesAddContentFileFlag   = "--content-file"
	filesAddModeFlag          = "--mode"
	filesAddPermissionsFlag   = "--permissions"
	filesAddRequireCleanFlag  = "--require-clean"
	filesAddBranchFlag        = "--branch"
	filesAddStartPointFlag    = "--start-point"
	filesAddPushFlag          = "--push"
	filesAddRemoteFlag        = "--remote"
	filesAddCommitMessageFlag = "--commit-message"
	filesAddConfiguredRoot    = "/tmp/file-add-config-root"
	filesAddCliRoot           = "/tmp/file-add-cli-root"
)

type filesAddRecordingTaskRunner struct {
	roots          []string
	definitions    []workflow.TaskDefinition
	runtimeOptions workflow.RuntimeOptions
}

func (runner *filesAddRecordingTaskRunner) Run(_ context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) error {
	runner.roots = append([]string{}, roots...)
	runner.definitions = append([]workflow.TaskDefinition{}, definitions...)
	runner.runtimeOptions = options
	return nil
}

func TestFilesAddCommandUsesConfigurationDefaults(t *testing.T) {
	t.Parallel()

	taskRunner := &filesAddRecordingTaskRunner{}
	builder := repos.FilesAddCommandBuilder{
		LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
		Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{filesAddConfiguredRoot}},
		GitExecutor:                  &fakeGitExecutor{},
		GitManager:                   &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:                   fakeFileSystem{},
		HumanReadableLoggingProvider: func() bool { return false },
		ConfigurationProvider: func() repos.AddConfiguration {
			return repos.AddConfiguration{
				DryRun:          true,
				AssumeYes:       true,
				RepositoryRoots: []string{filesAddConfiguredRoot},
				Path:            "docs/POLICY.md",
				Content:         "Example",
				Mode:            "skip-if-exists",
				Permissions:     "0640",
				RequireClean:    true,
				Branch:          "docs/add",
				StartPoint:      "main",
				Push:            true,
				PushRemote:      "origin",
				CommitMessage:   "docs: add POLICY",
			}
		},
		TaskRunnerFactory: func(workflow.Dependencies) repos.TaskRunnerExecutor { return taskRunner },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindGlobalReplaceFlags(command)

	command.SetContext(context.Background())
	require.NoError(t, command.Execute())

	require.Equal(t, []string{filesAddConfiguredRoot}, taskRunner.roots)
	require.True(t, taskRunner.runtimeOptions.DryRun)
	require.True(t, taskRunner.runtimeOptions.AssumeYes)
	require.True(t, taskRunner.runtimeOptions.CaptureInitialWorktreeStatus)
	require.Len(t, taskRunner.definitions, 1)

	definition := taskRunner.definitions[0]
	require.Equal(t, "Add repository file docs/POLICY.md", definition.Name)
	require.True(t, definition.EnsureClean)
	require.Equal(t, "docs/add", definition.Branch.NameTemplate)
	require.Equal(t, "main", definition.Branch.StartPointTemplate)
	require.Equal(t, "origin", definition.Branch.PushRemote)
	require.Len(t, definition.Files, 1)

	fileDefinition := definition.Files[0]
	require.Equal(t, "docs/POLICY.md", fileDefinition.PathTemplate)
	require.Equal(t, "Example", fileDefinition.ContentTemplate)
	require.Equal(t, workflow.TaskFileModeSkipIfExists, fileDefinition.Mode)
	require.Equal(t, "docs: add POLICY", definition.Commit.MessageTemplate)
}

func TestFilesAddCommandFlagOverrides(t *testing.T) {
	t.Parallel()

	taskRunner := &filesAddRecordingTaskRunner{}
	fs := fakeFileSystem{files: map[string]string{"content.txt": "From file"}}
	builder := repos.FilesAddCommandBuilder{
		LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
		Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{filesAddCliRoot}},
		GitExecutor:                  &fakeGitExecutor{},
		GitManager:                   &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:                   fs,
		HumanReadableLoggingProvider: func() bool { return false },
		ConfigurationProvider: func() repos.AddConfiguration {
			return repos.AddConfiguration{RepositoryRoots: []string{filesAddConfiguredRoot}}
		},
		TaskRunnerFactory: func(workflow.Dependencies) repos.TaskRunnerExecutor { return taskRunner },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindGlobalReplaceFlags(command)

	args := flagutils.NormalizeToggleArguments([]string{
		filesAddDryRunFlag,
		filesAddAssumeYesFlag,
		filesAddRootFlag, filesAddCliRoot,
		filesAddPathFlag, "docs/POLICY.md",
		filesAddContentFileFlag, "content.txt",
		filesAddModeFlag, "overwrite",
		filesAddPermissionsFlag, "0600",
		filesAddRequireCleanFlag,
		filesAddBranchFlag, "automation/docs",
		filesAddStartPointFlag, "develop",
		filesAddPushFlag, "no",
		filesAddRemoteFlag, "upstream",
		filesAddCommitMessageFlag, "docs: seed policy",
	})

	command.SetContext(context.Background())
	command.SetArgs(args)

	require.NoError(t, command.Execute())
	require.Equal(t, []string{filesAddCliRoot}, taskRunner.roots)
	require.True(t, taskRunner.runtimeOptions.DryRun)
	require.True(t, taskRunner.runtimeOptions.AssumeYes)

	definition := taskRunner.definitions[0]
	require.True(t, definition.EnsureClean)
	require.Equal(t, "automation/docs", definition.Branch.NameTemplate)
	require.Equal(t, "develop", definition.Branch.StartPointTemplate)
	require.Equal(t, "", definition.Branch.PushRemote)
	require.Len(t, definition.Files, 1)

	fileDefinition := definition.Files[0]
	require.Equal(t, "docs/POLICY.md", fileDefinition.PathTemplate)
	require.Equal(t, "From file", fileDefinition.ContentTemplate)
	require.Equal(t, workflow.TaskFileModeOverwrite, fileDefinition.Mode)
	require.Equal(t, os.FileMode(0o600), fileDefinition.Permissions)
	require.Equal(t, "docs: seed policy", definition.Commit.MessageTemplate)
}

func TestFilesAddCommandRequiresContent(t *testing.T) {
	t.Parallel()

	taskRunner := &filesAddRecordingTaskRunner{}
	builder := repos.FilesAddCommandBuilder{
		LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
		Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{filesAddConfiguredRoot}},
		GitExecutor:                  &fakeGitExecutor{},
		GitManager:                   &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true},
		FileSystem:                   fakeFileSystem{},
		HumanReadableLoggingProvider: func() bool { return false },
		ConfigurationProvider: func() repos.AddConfiguration {
			return repos.AddConfiguration{}
		},
		TaskRunnerFactory: func(workflow.Dependencies) repos.TaskRunnerExecutor { return taskRunner },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindGlobalReplaceFlags(command)

	command.SetContext(context.Background())
	command.SetArgs([]string{"--path", "docs/policy.md"})

	err := command.Execute()
	require.Error(t, err)
}

func TestFilesAddCommandRejectsPositionalRoots(t *testing.T) {
	t.Parallel()

	taskRunner := &filesAddRecordingTaskRunner{}
	builder := repos.FilesAddCommandBuilder{
		LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
		Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{filesAddConfiguredRoot}},
		GitExecutor:                  &fakeGitExecutor{},
		GitManager:                   &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:                   fakeFileSystem{},
		HumanReadableLoggingProvider: func() bool { return false },
		ConfigurationProvider: func() repos.AddConfiguration {
			return repos.AddConfiguration{
				RepositoryRoots: []string{filesAddConfiguredRoot},
				Path:            "docs/POLICY.md",
				Content:         "Example",
			}
		},
		TaskRunnerFactory: func(workflow.Dependencies) repos.TaskRunnerExecutor { return taskRunner },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindGlobalReplaceFlags(command)

	command.SetContext(context.Background())
	command.SetArgs([]string{"./explicit-root"})

	err := command.Execute()
	require.Error(t, err)
	require.Equal(t, rootutils.PositionalRootsUnsupportedMessage(), err.Error())
	require.Empty(t, taskRunner.roots)
}
