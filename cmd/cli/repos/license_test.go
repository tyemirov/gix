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

	repos "github.com/temirov/gix/cmd/cli/repos"
	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/repos/filesystem"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
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

type recordingWorkflowExecutor struct {
	roots   []string
	options workflow.RuntimeOptions
}

func (executor *recordingWorkflowExecutor) Execute(_ context.Context, roots []string, options workflow.RuntimeOptions) error {
	executor.roots = append([]string{}, roots...)
	executor.options = options
	return nil
}

type stubPresetCatalog struct {
	configuration workflow.Configuration
}

func (stub stubPresetCatalog) List() []workflowcmd.PresetMetadata {
	return nil
}

func (stub stubPresetCatalog) Load(name string) (workflow.Configuration, bool, error) {
	return stub.configuration, true, nil
}

func TestLicenseCommandUsesConfigurationDefaults(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	templatePath := filepath.Join(tempDir, "LICENSE.txt")
	require.NoError(t, os.WriteFile(templatePath, []byte("Example License\n"), 0o644))

	executor := &recordingWorkflowExecutor{}
	builder := repos.LicenseCommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     &fakeRepositoryDiscoverer{repositories: []string{"/tmp/license-config-root"}},
		GitExecutor:    &fakeGitExecutor{},
		GitManager:     &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:     filesystem.OSFileSystem{},
		HumanReadableLoggingProvider: func() bool {
			return false
		},
		ConfigurationProvider: func() repos.LicenseConfiguration {
			return repos.LicenseConfiguration{
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
		PresetCatalogFactory: func() workflowcmd.PresetCatalog {
			return stubPresetCatalog{configuration: minimalLicenseWorkflow()}
		},
		ExecutorFactory: func(nodes []*workflow.OperationNode, deps workflow.Dependencies) repos.WorkflowExecutor {
			require.NotEmpty(t, nodes)
			require.NotNil(t, deps.RepositoryManager)
			return executor
		},
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)

	bindGlobalLicenseFlags(command)
	command.SetContext(context.Background())
	command.SetArgs(nil)

	runError := command.Execute()
	require.NoError(t, runError)

	require.Equal(t, []string{"/tmp/license-config-root"}, executor.roots)
	require.True(t, executor.options.AssumeYes)
	require.True(t, executor.options.CaptureInitialWorktreeStatus)
	require.Equal(t, "Example License\n", executor.options.Variables["license_content"])
	require.Equal(t, "LICENSE", executor.options.Variables["license_target"])
	require.Equal(t, "skip-if-exists", executor.options.Variables["license_mode"])
	require.Equal(t, "license/{{ .Repository.Name }}", executor.options.Variables["license_branch"])
	require.Equal(t, "{{ .Repository.DefaultBranch }}", executor.options.Variables["license_start_point"])
	require.Equal(t, "origin", executor.options.Variables["license_remote"])
	require.Equal(t, "docs: add license", executor.options.Variables["license_commit_message"])
	require.Equal(t, "true", executor.options.Variables["license_require_clean"])
}

func TestLicenseCommandFlagOverrides(t *testing.T) {
	t.Parallel()

	executor := &recordingWorkflowExecutor{}
	builder := repos.LicenseCommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     &fakeRepositoryDiscoverer{repositories: []string{"/tmp/license-cli-root"}},
		GitExecutor:    &fakeGitExecutor{},
		GitManager:     &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:     filesystem.OSFileSystem{},
		ConfigurationProvider: func() repos.LicenseConfiguration {
			return repos.LicenseConfiguration{RepositoryRoots: []string{"/tmp/license-config-root"}}
		},
		PresetCatalogFactory: func() workflowcmd.PresetCatalog {
			return stubPresetCatalog{configuration: minimalLicenseWorkflow()}
		},
		ExecutorFactory: func(nodes []*workflow.OperationNode, deps workflow.Dependencies) repos.WorkflowExecutor {
			return executor
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

	require.Equal(t, []string{cliRoot}, executor.roots)
	require.True(t, executor.options.AssumeYes)
	require.True(t, executor.options.CaptureInitialWorktreeStatus)
	require.Equal(t, "Inline License", executor.options.Variables["license_content"])
	require.Equal(t, "COPYING", executor.options.Variables["license_target"])
	require.Equal(t, "skip-if-exists", executor.options.Variables["license_mode"])
	require.Equal(t, "docs/{{ .Repository.Name }}/license", executor.options.Variables["license_branch"])
	require.Equal(t, "main", executor.options.Variables["license_start_point"])
	require.Equal(t, "upstream", executor.options.Variables["license_remote"])
	require.Equal(t, "docs: add COPYING", executor.options.Variables["license_commit_message"])
	require.Equal(t, "true", executor.options.Variables["license_require_clean"])
}

func TestLicenseCommandRequiresTemplateOrContent(t *testing.T) {
	t.Parallel()

	executor := &recordingWorkflowExecutor{}
	builder := repos.LicenseCommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     &fakeRepositoryDiscoverer{repositories: []string{"/tmp/license-config-root"}},
		GitExecutor:    &fakeGitExecutor{},
		GitManager:     &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:     filesystem.OSFileSystem{},
		ConfigurationProvider: func() repos.LicenseConfiguration {
			return repos.LicenseConfiguration{RepositoryRoots: []string{"/tmp/license-config-root"}}
		},
		PresetCatalogFactory: func() workflowcmd.PresetCatalog {
			return stubPresetCatalog{configuration: minimalLicenseWorkflow()}
		},
		ExecutorFactory: func(nodes []*workflow.OperationNode, deps workflow.Dependencies) repos.WorkflowExecutor {
			return executor
		},
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)

	bindGlobalLicenseFlags(command)
	command.SetContext(context.Background())
	command.SetArgs(nil)

	runError := command.Execute()
	require.Error(t, runError)
	require.Contains(t, strings.ToLower(runError.Error()), testLicenseTemplateFlag)
	require.Empty(t, executor.roots)
}

func minimalLicenseWorkflow() workflow.Configuration {
	return workflow.Configuration{
		Steps: []workflow.StepConfiguration{
			{
				Command: []string{"audit", "report"},
			},
		},
	}
}

func bindGlobalLicenseFlags(command *cobra.Command) {
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	flagutils.BindExecutionFlags(command, flagutils.ExecutionDefaults{}, flagutils.ExecutionFlagDefinitions{
		AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Shorthand: flagutils.AssumeYesFlagShorthand, Enabled: true},
	})
}
