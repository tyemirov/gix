package repos_test

import (
	"context"
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
	replaceDryRunFlag       = "--" + flagutils.DryRunFlagName
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

type replaceRecordingTaskRunner struct {
	roots          []string
	definitions    []workflow.TaskDefinition
	runtimeOptions workflow.RuntimeOptions
}

func (runner *replaceRecordingTaskRunner) Run(_ context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) error {
	runner.roots = append([]string{}, roots...)
	runner.definitions = append([]workflow.TaskDefinition{}, definitions...)
	runner.runtimeOptions = options
	return nil
}

func TestReplaceCommandUsesConfigurationDefaults(t *testing.T) {
	t.Parallel()

	taskRunner := &replaceRecordingTaskRunner{}
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
				DryRun:          true,
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
		TaskRunnerFactory: func(workflow.Dependencies) repos.TaskRunnerExecutor {
			return taskRunner
		},
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)

	bindGlobalReplaceFlags(command)

	command.SetContext(context.Background())
	command.SetArgs(nil)

	runError := command.Execute()
	require.NoError(t, runError)

	require.Equal(t, []string{replaceConfiguredRoot}, taskRunner.roots)
	require.True(t, taskRunner.runtimeOptions.DryRun)
	require.True(t, taskRunner.runtimeOptions.AssumeYes)
	require.Len(t, taskRunner.definitions, 1)

	action := taskRunner.definitions[0].Actions[0]
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
}

func TestReplaceCommandFlagOverrides(t *testing.T) {
	t.Parallel()

	taskRunner := &replaceRecordingTaskRunner{}
	builder := repos.ReplaceCommandBuilder{
		LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
		Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{replaceCliRoot}},
		GitExecutor:                  &fakeGitExecutor{},
		GitManager:                   &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "master"},
		FileSystem:                   filesystem.OSFileSystem{},
		HumanReadableLoggingProvider: func() bool { return false },
		ConfigurationProvider: func() repos.ReplaceConfiguration {
			return repos.ReplaceConfiguration{
				DryRun:          false,
				AssumeYes:       false,
				RepositoryRoots: []string{replaceConfiguredRoot},
				Find:            "config",
			}
		},
		TaskRunnerFactory: func(workflow.Dependencies) repos.TaskRunnerExecutor { return taskRunner },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)

	bindGlobalReplaceFlags(command)

	args := flagutils.NormalizeToggleArguments([]string{
		replaceDryRunFlag,
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

	require.Equal(t, []string{replaceCliRoot}, taskRunner.roots)
	require.True(t, taskRunner.runtimeOptions.DryRun)
	require.True(t, taskRunner.runtimeOptions.AssumeYes)

	require.Len(t, taskRunner.definitions, 1)
	action := taskRunner.definitions[0].Actions[0]
	require.Equal(t, "repo.files.replace", action.Type)

	require.Equal(t, []string{"old", "new"}, []string{action.Options["find"].(string), action.Options["replace"].(string)})

	patternList, ok := action.Options["patterns"].([]string)
	require.True(t, ok)
	require.ElementsMatch(t, []string{"*.go", "cmd/**/*.go"}, patternList)

	commandArgs := action.Options["command"].([]string)
	require.Equal(t, []string{"go", "test", "./..."}, commandArgs)

	safeguards := action.Options["safeguards"].(map[string]any)
	require.True(t, safeguards["require_clean"].(bool))
	require.Equal(t, "master", safeguards["branch"])
	require.Equal(t, []string{filepath.Clean("go.mod")}, safeguards["paths"].([]string))
}

func TestReplaceCommandRequiresFind(t *testing.T) {
	t.Parallel()

	taskRunner := &replaceRecordingTaskRunner{}
	builder := repos.ReplaceCommandBuilder{
		LoggerProvider:               func() *zap.Logger { return zap.NewNop() },
		Discoverer:                   &fakeRepositoryDiscoverer{repositories: []string{replaceConfiguredRoot}},
		GitExecutor:                  &fakeGitExecutor{},
		GitManager:                   &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true},
		FileSystem:                   filesystem.OSFileSystem{},
		HumanReadableLoggingProvider: func() bool { return false },
		ConfigurationProvider:        func() repos.ReplaceConfiguration { return repos.ReplaceConfiguration{} },
		TaskRunnerFactory:            func(workflow.Dependencies) repos.TaskRunnerExecutor { return taskRunner },
	}

	command, buildError := builder.Build()
	require.NoError(t, buildError)

	bindGlobalReplaceFlags(command)

	command.SetContext(context.Background())
	command.SetArgs(nil)

	err := command.Execute()
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "requires --find")
	require.Empty(t, taskRunner.definitions)
}

func bindGlobalReplaceFlags(command *cobra.Command) {
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	flagutils.BindExecutionFlags(command, flagutils.ExecutionDefaults{}, flagutils.ExecutionFlagDefinitions{
		DryRun:    flagutils.ExecutionFlagDefinition{Name: flagutils.DryRunFlagName, Usage: flagutils.DryRunFlagUsage, Enabled: true},
		AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Shorthand: flagutils.AssumeYesFlagShorthand, Enabled: true},
	})
}
