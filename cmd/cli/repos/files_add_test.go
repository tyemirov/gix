package repos_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repos "github.com/temirov/gix/cmd/cli/repos"
	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	rootutils "github.com/temirov/gix/internal/utils/roots"
	"github.com/temirov/gix/internal/workflow"
)

const (
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

func TestFilesAddCommandUsesConfigurationDefaults(t *testing.T) {
	t.Parallel()

	presetConfig := loadFilesAddPreset(t)
	recording := &filesAddRecordingExecutor{}
	builder := repos.FilesAddCommandBuilder{
		LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
		Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{filesAddConfiguredRoot}},
		GitExecutor:                  &fakeGitExecutor{},
		GitManager:                   &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:                   fakeFileSystem{},
		HumanReadableLoggingProvider: func() bool { return false },
		ConfigurationProvider: func() repos.AddConfiguration {
			return repos.AddConfiguration{
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
		PresetCatalogFactory: func() workflowcmd.PresetCatalog { return &fakePresetCatalog{configuration: presetConfig, found: true} },
		WorkflowExecutorFactory: func(nodes []*workflow.OperationNode, _ workflow.Dependencies) workflowcmd.OperationExecutor {
			require.Len(t, nodes, 1)
			taskOp, ok := nodes[0].Operation.(*workflow.TaskOperation)
			require.True(t, ok)
			definitions := taskOp.Definitions()
			require.Len(t, definitions, 1)
			task := definitions[0]
			require.Equal(t, "Add repository file docs/POLICY.md", task.Name)
			require.True(t, task.EnsureClean)
			require.Equal(t, "docs/add", task.Branch.NameTemplate)
			require.Equal(t, "main", task.Branch.StartPointTemplate)
			require.Equal(t, "origin", task.Branch.PushRemote)
			require.Len(t, task.Files, 1)
			fileDef := task.Files[0]
			require.Equal(t, "docs/POLICY.md", fileDef.PathTemplate)
			require.Equal(t, "Example", fileDef.ContentTemplate)
			require.Equal(t, workflow.TaskFileModeSkipIfExists, fileDef.Mode)
			require.Equal(t, "docs: add POLICY", task.Commit.MessageTemplate)
			return recording
		},
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindGlobalReplaceFlags(command)

	command.SetContext(context.Background())
	require.NoError(t, command.Execute())

	require.Equal(t, []string{filesAddConfiguredRoot}, recording.roots)
	require.True(t, recording.options.AssumeYes)
	require.True(t, recording.options.CaptureInitialWorktreeStatus)
}

func TestFilesAddCommandFlagOverrides(t *testing.T) {
	t.Parallel()

	presetConfig := loadFilesAddPreset(t)
	recording := &filesAddRecordingExecutor{}
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
		PresetCatalogFactory: func() workflowcmd.PresetCatalog { return &fakePresetCatalog{configuration: presetConfig, found: true} },
		WorkflowExecutorFactory: func(nodes []*workflow.OperationNode, _ workflow.Dependencies) workflowcmd.OperationExecutor {
			require.Len(t, nodes, 1)
			taskOp, ok := nodes[0].Operation.(*workflow.TaskOperation)
			require.True(t, ok)
			definitions := taskOp.Definitions()
			require.Len(t, definitions, 1)
			task := definitions[0]
			require.True(t, task.EnsureClean)
			require.Equal(t, "automation/docs", task.Branch.NameTemplate)
			require.Equal(t, "develop", task.Branch.StartPointTemplate)
			require.Equal(t, "origin", task.Branch.PushRemote)
			stepNames := make([]string, 0, len(task.Steps))
			for _, step := range task.Steps {
				stepNames = append(stepNames, string(step))
			}
			require.Equal(t, []string{"branch.prepare", "files.apply", "git.stage-commit"}, stepNames)
			require.Len(t, task.Files, 1)

			fileDef := task.Files[0]
			require.Equal(t, "docs/POLICY.md", fileDef.PathTemplate)
			require.Equal(t, "From file", fileDef.ContentTemplate)
			require.Equal(t, workflow.TaskFileModeOverwrite, fileDef.Mode)
			require.Equal(t, os.FileMode(0o600), fileDef.Permissions)
			require.Equal(t, "docs: seed policy", task.Commit.MessageTemplate)
			return recording
		},
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindGlobalReplaceFlags(command)

	args := flagutils.NormalizeToggleArguments([]string{
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
	require.Equal(t, []string{filesAddCliRoot}, recording.roots)
	require.True(t, recording.options.AssumeYes)
}

func TestFilesAddCommandRequiresContent(t *testing.T) {
	t.Parallel()

	presetConfig := loadFilesAddPreset(t)
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
		PresetCatalogFactory: func() workflowcmd.PresetCatalog { return &fakePresetCatalog{configuration: presetConfig, found: true} },
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

	presetConfig := loadFilesAddPreset(t)
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
		PresetCatalogFactory: func() workflowcmd.PresetCatalog { return &fakePresetCatalog{configuration: presetConfig, found: true} },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)
	bindGlobalReplaceFlags(command)

	command.SetContext(context.Background())
	command.SetArgs([]string{"./explicit-root"})

	err := command.Execute()
	require.Error(t, err)
	require.Equal(t, rootutils.PositionalRootsUnsupportedMessage(), err.Error())
}

func TestFilesAddCommandPresetErrorsSurface(t *testing.T) {
	t.Parallel()

	presetConfig := loadFilesAddPreset(t)

	testCases := []struct {
		name      string
		catalog   fakePresetCatalog
		expectErr string
	}{
		{
			name:      "missing_preset",
			catalog:   fakePresetCatalog{found: false},
			expectErr: "files-add preset not found",
		},
		{
			name:      "load_error",
			catalog:   fakePresetCatalog{found: true, loadError: errors.New("boom")},
			expectErr: "unable to load files-add preset: boom",
		},
		{
			name: "build_error",
			catalog: fakePresetCatalog{
				found: true,
				configuration: workflow.Configuration{
					Steps: []workflow.StepConfiguration{
						{Command: []string{"unknown"}},
					},
				},
			},
			expectErr: "unable to build files-add workflow: unsupported workflow command: unknown",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(subtest *testing.T) {
			catalog := testCase.catalog
			if len(catalog.configuration.Steps) == 0 && catalog.found && catalog.loadError == nil && testCase.name != "build_error" {
				catalog.configuration = presetConfig
			}

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
				PresetCatalogFactory: func() workflowcmd.PresetCatalog { return &catalog },
			}

			command, buildError := builder.Build()
			require.NoError(subtest, buildError)
			bindGlobalReplaceFlags(command)
			command.SetContext(context.Background())
			err := command.Execute()
			require.EqualError(subtest, err, testCase.expectErr)
		})
	}
}

type filesAddRecordingExecutor struct {
	roots   []string
	options workflow.RuntimeOptions
}

func (executor *filesAddRecordingExecutor) Execute(_ context.Context, roots []string, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	executor.roots = append([]string{}, roots...)
	executor.options = options
	return workflow.ExecutionOutcome{}, nil
}

func loadFilesAddPreset(testingInstance testing.TB) workflow.Configuration {
	presetContent := []byte(`workflow:
  - step:
      name: files-add
      command: ["tasks", "apply"]
      with:
        tasks:
          - name: Add repository file
            ensure_clean: true
            branch:
              name: ""
              start_point: ""
              push_remote: ""
            files:
              - path: ""
                content: ""
                mode: skip-if-exists
                permissions: 420
            commit:
              message: ""
`)
	configuration, err := workflow.ParseConfiguration(presetContent)
	require.NoError(testingInstance, err)
	return configuration
}
