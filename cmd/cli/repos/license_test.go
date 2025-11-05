package repos_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repos "github.com/tyemirov/gix/cmd/cli/repos"
	"github.com/tyemirov/gix/internal/repos/filesystem"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	testLicenseTemplateFlag      = "template"
	testLicenseContentFlag       = "content"
	testLicenseTargetFlag        = "target"
	testLicenseModeFlag          = "mode"
	testLicenseBranchFlag        = "branch"
	testLicenseStartPointFlag    = "start-point"
	testLicenseRemoteFlag        = "remote"
	testLicenseCommitMessageFlag = "commit-message"
	testLicenseRequireCleanFlag  = "require-clean"
)

type licenseRecordingTaskRunner struct {
	roots          []string
	definitions    []workflow.TaskDefinition
	runtimeOptions workflow.RuntimeOptions
}

func (runner *licenseRecordingTaskRunner) Run(_ context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) error {
	runner.roots = append([]string{}, roots...)
	runner.definitions = append([]workflow.TaskDefinition{}, definitions...)
	runner.runtimeOptions = options
	return nil
}

func TestLicenseCommandUsesConfigurationDefaults(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "LICENSE.txt")
	require.NoError(t, os.WriteFile(templatePath, []byte("Example License\n"), 0o644))

	taskRunner := &licenseRecordingTaskRunner{}
	builder := repos.LicenseCommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return zap.NewNop()
		},
		Discoverer:  &fakeRepositoryDiscoverer{repositories: []string{"/tmp/license-config-root"}},
		GitExecutor: &fakeGitExecutor{},
		GitManager:  &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:  filesystem.OSFileSystem{},
		HumanReadableLoggingProvider: func() bool {
			return false
		},
		ConfigurationProvider: func() repos.LicenseConfiguration {
			return repos.LicenseConfiguration{
				DryRun:          true,
				AssumeYes:       true,
				RepositoryRoots: []string{"/tmp/license-config-root"},
				TemplatePath:    templatePath,
				TargetPath:      "LICENSE",
				Mode:            "skip-if-exists",
				RequireClean:    true,
				Branch:          "license/{{ .Repository.Name }}",
				StartPoint:      "{{ .Repository.DefaultBranch }}",
				PushRemote:      "origin",
				CommitMessage:   "docs: add license",
			}
		},
		TaskRunnerFactory: func(workflow.Dependencies) repos.TaskRunnerExecutor {
			return taskRunner
		},
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)

	bindGlobalLicenseFlags(command)
	command.SetContext(context.Background())
	command.SetArgs(nil)

	runError := command.Execute()
	require.NoError(t, runError)

	require.Equal(t, []string{"/tmp/license-config-root"}, taskRunner.roots)
	require.True(t, taskRunner.runtimeOptions.DryRun)
	require.True(t, taskRunner.runtimeOptions.AssumeYes)
	require.True(t, taskRunner.runtimeOptions.CaptureInitialWorktreeStatus)

	require.Len(t, taskRunner.definitions, 1)
	definition := taskRunner.definitions[0]
	require.True(t, definition.EnsureClean)
	require.Len(t, definition.Files, 1)

	fileDefinition := definition.Files[0]
	require.Equal(t, "LICENSE", fileDefinition.PathTemplate)
	require.Equal(t, "Example License\n", fileDefinition.ContentTemplate)
	require.Equal(t, workflow.TaskFileModeSkipIfExists, fileDefinition.Mode)

	require.Equal(t, "license/{{ .Repository.Name }}", definition.Branch.NameTemplate)
	require.Equal(t, "{{ .Repository.DefaultBranch }}", definition.Branch.StartPointTemplate)
	require.Equal(t, "origin", definition.Branch.PushRemote)
	require.Equal(t, "docs: add license", definition.Commit.MessageTemplate)
}

func TestLicenseCommandFlagOverrides(t *testing.T) {
	t.Parallel()

	taskRunner := &licenseRecordingTaskRunner{}
	builder := repos.LicenseCommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return zap.NewNop()
		},
		Discoverer:  &fakeRepositoryDiscoverer{repositories: []string{"/tmp/license-cli-root"}},
		GitExecutor: &fakeGitExecutor{},
		GitManager:  &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:  filesystem.OSFileSystem{},
		HumanReadableLoggingProvider: func() bool {
			return false
		},
		ConfigurationProvider: func() repos.LicenseConfiguration {
			return repos.LicenseConfiguration{
				RepositoryRoots: []string{"/tmp/license-config-root"},
				RequireClean:    false,
			}
		},
		TaskRunnerFactory: func(workflow.Dependencies) repos.TaskRunnerExecutor {
			return taskRunner
		},
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)

	bindGlobalLicenseFlags(command)

	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "LICENSE.tpl")
	require.NoError(t, os.WriteFile(templatePath, []byte("License Template\n"), 0o644))

	cliRoot := "/tmp/license-cli-root"
	args := flagutils.NormalizeToggleArguments([]string{
		"--" + flagutils.DryRunFlagName,
		"--" + flagutils.AssumeYesFlagName,
		"--" + flagutils.DefaultRootFlagName, cliRoot,
		"--" + testLicenseTemplateFlag, templatePath,
		"--" + testLicenseContentFlag, "Inline License",
		"--" + testLicenseTargetFlag, "COPYING",
		"--" + testLicenseModeFlag, "skip-if-exists",
		"--" + testLicenseBranchFlag, "docs/{{ .Repository.Name }}/license",
		"--" + testLicenseStartPointFlag, "main",
		"--" + testLicenseRemoteFlag, "upstream",
		"--" + testLicenseCommitMessageFlag, "docs: add COPYING",
		"--" + testLicenseRequireCleanFlag,
	})

	command.SetContext(context.Background())
	command.SetArgs(args)

	runError := command.Execute()
	require.NoError(t, runError)

	require.Equal(t, []string{cliRoot}, taskRunner.roots)
	require.True(t, taskRunner.runtimeOptions.DryRun)
	require.True(t, taskRunner.runtimeOptions.AssumeYes)
	require.True(t, taskRunner.runtimeOptions.CaptureInitialWorktreeStatus)

	require.Len(t, taskRunner.definitions, 1)
	definition := taskRunner.definitions[0]
	require.True(t, definition.EnsureClean)
	require.Equal(t, "docs/{{ .Repository.Name }}/license", definition.Branch.NameTemplate)
	require.Equal(t, "main", definition.Branch.StartPointTemplate)
	require.Equal(t, "upstream", definition.Branch.PushRemote)

	require.Len(t, definition.Files, 1)
	fileDefinition := definition.Files[0]
	require.Equal(t, "COPYING", fileDefinition.PathTemplate)
	require.Equal(t, "Inline License", fileDefinition.ContentTemplate)
	require.Equal(t, workflow.TaskFileModeSkipIfExists, fileDefinition.Mode)
	require.Equal(t, "docs: add COPYING", definition.Commit.MessageTemplate)
}

func TestLicenseCommandRequiresTemplateOrContent(t *testing.T) {
	t.Parallel()

	taskRunner := &licenseRecordingTaskRunner{}
	builder := repos.LicenseCommandBuilder{
		LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
		Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{"/tmp/license-config-root"}},
		GitExecutor:                  &fakeGitExecutor{},
		GitManager:                   &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:                   filesystem.OSFileSystem{},
		HumanReadableLoggingProvider: func() bool { return false },
		ConfigurationProvider: func() repos.LicenseConfiguration {
			return repos.LicenseConfiguration{
				RepositoryRoots: []string{"/tmp/license-config-root"},
			}
		},
		TaskRunnerFactory: func(workflow.Dependencies) repos.TaskRunnerExecutor { return taskRunner },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)

	bindGlobalLicenseFlags(command)
	command.SetContext(context.Background())
	command.SetArgs(nil)

	err := command.Execute()
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), testLicenseTemplateFlag)
	require.Empty(t, taskRunner.definitions)
}

func bindGlobalLicenseFlags(command *cobra.Command) {
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	flagutils.BindExecutionFlags(command, flagutils.ExecutionDefaults{}, flagutils.ExecutionFlagDefinitions{
		DryRun:    flagutils.ExecutionFlagDefinition{Name: flagutils.DryRunFlagName, Usage: flagutils.DryRunFlagUsage, Enabled: true},
		AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Shorthand: flagutils.AssumeYesFlagShorthand, Enabled: true},
	})
}
