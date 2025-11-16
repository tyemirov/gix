package repos_test

import (
	"context"
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
	replaceAssumeYesFlag    = "--" + flagutils.AssumeYesFlagName
	replaceRootFlag         = "--" + flagutils.DefaultRootFlagName
	replacePatternFlag      = "--pattern"
	replaceFindFlag         = "--find"
	replaceReplaceFlag      = "--replace"
	replaceCommandFlag      = "--command"
	replaceRequireCleanFlag = "--require-clean"
	replaceBranchFlag       = "--branch"
	replaceRequirePathFlag  = "--require-path"
	replaceConfiguredRoot   = "/tmp/replace-config-root"
	replaceCliRoot          = "/tmp/replace-cli-root"
)

type replaceRecordingExecutor struct {
	roots   []string
	options workflow.RuntimeOptions
}

func (executor *replaceRecordingExecutor) Execute(_ context.Context, roots []string, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	executor.roots = append([]string{}, roots...)
	executor.options = options
	return workflow.ExecutionOutcome{}, nil
}

func TestReplaceCommandUsesConfigurationDefaults(t *testing.T) {
	t.Parallel()

	presetConfig := loadFilesReplacePreset(t)
	recording := &replaceRecordingExecutor{}
	builder := repos.ReplaceCommandBuilder{
		LoggerProvider: func() *zap.Logger {
			return zap.NewNop()
		},
		Discoverer:  &fakeRepositoryDiscoverer{repositories: []string{replaceConfiguredRoot}},
		GitExecutor: &fakeGitExecutor{},
		GitManager:  &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:  filesystem.OSFileSystem{},
		HumanReadableLoggingProvider: func() bool {
			return false
		},
		ConfigurationProvider: func() repos.ReplaceConfiguration {
			return repos.ReplaceConfiguration{
				AssumeYes:       true,
				RepositoryRoots: []string{replaceConfiguredRoot},
				Patterns:        []string{"*.md"},
				Find:            "foo",
				Replace:         "bar",
				Command:         "go fmt ./...",
				RequireClean:    true,
				Branch:          "main",
				RequirePaths:    []string{"go.mod"},
			}
		},
		PresetCatalogFactory: func() workflowcmd.PresetCatalog {
			return &fakePresetCatalog{configuration: presetConfig, found: true}
		},
		WorkflowExecutorFactory: func(nodes []*workflow.OperationNode, _ workflow.Dependencies) repos.WorkflowExecutor {
			require.Len(t, nodes, 1)
			taskOperation, ok := nodes[0].Operation.(*workflow.TaskOperation)
			require.True(t, ok)
			definitions := taskOperation.Definitions()
			require.Len(t, definitions, 1)
			task := definitions[0]
			require.Len(t, task.Actions, 1)
			action := task.Actions[0]
			require.Equal(t, "repo.files.replace", action.Type)

			options := action.Options
			require.Equal(t, "foo", options["find"])
			require.Equal(t, "bar", options["replace"])
			require.Equal(t, "*.md", options["pattern"])
			require.Equal(t, []string{"go", "fmt", "./..."}, options["command"])

			safeguards, ok := options["safeguards"].(map[string]any)
			require.True(t, ok)
			require.True(t, safeguards["require_clean"].(bool))
			require.Equal(t, "main", safeguards["branch"])
			require.ElementsMatch(t, []string{filepath.Clean("go.mod")}, safeguards["paths"].([]string))
			return recording
		},
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)

	bindGlobalReplaceFlags(command)

	command.SetContext(context.Background())
	command.SetArgs(nil)

	runError := command.Execute()
	require.NoError(t, runError)

	require.Equal(t, []string{replaceConfiguredRoot}, recording.roots)
	require.True(t, recording.options.AssumeYes)
}

func TestReplaceCommandFlagOverrides(t *testing.T) {
	t.Parallel()

	presetConfig := loadFilesReplacePreset(t)
	recording := &replaceRecordingExecutor{}
	builder := repos.ReplaceCommandBuilder{
		LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
		Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{replaceCliRoot}},
		GitExecutor:                  &fakeGitExecutor{},
		GitManager:                   &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "master"},
		FileSystem:                   filesystem.OSFileSystem{},
		HumanReadableLoggingProvider: func() bool { return false },
		ConfigurationProvider: func() repos.ReplaceConfiguration {
			return repos.ReplaceConfiguration{
				AssumeYes:       false,
				RepositoryRoots: []string{replaceConfiguredRoot},
				Find:            "config",
			}
		},
		PresetCatalogFactory: func() workflowcmd.PresetCatalog {
			return &fakePresetCatalog{configuration: presetConfig, found: true}
		},
		WorkflowExecutorFactory: func(nodes []*workflow.OperationNode, _ workflow.Dependencies) repos.WorkflowExecutor {
			require.Len(t, nodes, 1)
			taskOperation, ok := nodes[0].Operation.(*workflow.TaskOperation)
			require.True(t, ok)
			definitions := taskOperation.Definitions()
			require.Len(t, definitions, 1)
			task := definitions[0]
			require.Len(t, task.Actions, 1)
			action := task.Actions[0]

			require.Equal(t, "repo.files.replace", action.Type)
			require.Equal(t, "old", action.Options["find"])
			require.Equal(t, "new", action.Options["replace"])

			patternList, ok := action.Options["patterns"].([]string)
			require.True(t, ok)
			require.ElementsMatch(t, []string{"*.go", "cmd/**/*.go"}, patternList)

			commandArgs := action.Options["command"].([]string)
			require.Equal(t, []string{"go", "test", "./..."}, commandArgs)

			safeguards := action.Options["safeguards"].(map[string]any)
			require.True(t, safeguards["require_clean"].(bool))
			require.Equal(t, "master", safeguards["branch"])
			require.Equal(t, []string{filepath.Clean("go.mod")}, safeguards["paths"].([]string))

			return recording
		},
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)

	bindGlobalReplaceFlags(command)

	args := flagutils.NormalizeToggleArguments([]string{
		replaceAssumeYesFlag,
		replaceRootFlag, replaceCliRoot,
		replacePatternFlag, "*.go",
		replacePatternFlag, "cmd/**/*.go",
		replaceFindFlag, "old",
		replaceReplaceFlag, "new",
		replaceCommandFlag, "go test ./...",
		replaceRequireCleanFlag,
		replaceBranchFlag, "master",
		replaceRequirePathFlag, "go.mod",
	})
	command.SetContext(context.Background())
	command.SetArgs(args)

	runError := command.Execute()
	require.NoError(t, runError)

	require.Equal(t, []string{replaceCliRoot}, recording.roots)
	require.True(t, recording.options.AssumeYes)
}

func TestReplaceCommandRequiresFind(t *testing.T) {
	t.Parallel()

	builder := repos.ReplaceCommandBuilder{
		LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
		Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{replaceConfiguredRoot}},
		GitExecutor:                  &fakeGitExecutor{},
		GitManager:                   &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true},
		FileSystem:                   filesystem.OSFileSystem{},
		HumanReadableLoggingProvider: func() bool { return false },
		ConfigurationProvider:        func() repos.ReplaceConfiguration { return repos.ReplaceConfiguration{} },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)

	bindGlobalReplaceFlags(command)

	command.SetContext(context.Background())
	command.SetArgs(nil)

	err := command.Execute()
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "requires --find")
}

func bindGlobalReplaceFlags(command *cobra.Command) {
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	flagutils.BindExecutionFlags(command, flagutils.ExecutionDefaults{}, flagutils.ExecutionFlagDefinitions{
		AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Shorthand: flagutils.AssumeYesFlagShorthand, Enabled: true},
	})
}

func loadFilesReplacePreset(testInstance testing.TB) workflow.Configuration {
	testInstance.Helper()
	presetContent := []byte(`workflow:
  - step:
      name: files-replace
      command: ["tasks", "apply"]
      with:
        tasks:
          - name: Replace repository file content
            ensure_clean: false
            actions:
              - type: repo.files.replace
                options:
                  find: ""
                  replace: ""
                  pattern: ""
                  patterns: []
                  command: []
                  safeguards: {}
`)
	configuration, parseError := workflow.ParseConfiguration(presetContent)
	require.NoError(testInstance, parseError)
	return configuration
}
