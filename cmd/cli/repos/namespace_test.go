package repos_test

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repos "github.com/temirov/gix/cmd/cli/repos"
	"github.com/temirov/gix/internal/repos/filesystem"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
)

type namespaceRecordingTaskRunner struct {
	roots       []string
	definitions []workflow.TaskDefinition
	options     workflow.RuntimeOptions
}

func (runner *namespaceRecordingTaskRunner) Run(_ context.Context, roots []string, definitions []workflow.TaskDefinition, options workflow.RuntimeOptions) error {
	runner.roots = append([]string{}, roots...)
	runner.definitions = append([]workflow.TaskDefinition{}, definitions...)
	runner.options = options
	return nil
}

func TestNamespaceCommandUsesConfigurationDefaults(t *testing.T) {
	t.Parallel()

	taskRunner := &namespaceRecordingTaskRunner{}
	builder := repos.NamespaceCommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     &fakeRepositoryDiscoverer{repositories: []string{"/tmp/cfg-root"}},
		GitExecutor:    &fakeGitExecutor{},
		GitManager:     &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:     filesystem.OSFileSystem{},
		ConfigurationProvider: func() repos.NamespaceConfiguration {
			return repos.NamespaceConfiguration{
				DryRun:          true,
				AssumeYes:       true,
				RepositoryRoots: []string{"/tmp/cfg-root"},
				OldPrefix:       "github.com/old/org",
				NewPrefix:       "github.com/new/org",
				Push:            true,
				Remote:          "origin",
				BranchPrefix:    "ns",
				Safeguards:      map[string]any{"require_clean": true},
			}
		},
		TaskRunnerFactory: func(workflow.Dependencies) repos.TaskRunnerExecutor { return taskRunner },
	}

	command, err := builder.Build()
	require.NoError(t, err)
	bindGlobalNamespaceFlags(command)

	command.SetContext(context.Background())
	command.SetArgs(nil)

	runErr := command.Execute()
	require.NoError(t, runErr)

	require.Equal(t, []string{"/tmp/cfg-root"}, taskRunner.roots)
	require.True(t, taskRunner.options.DryRun)
	require.True(t, taskRunner.options.AssumeYes)
	require.Len(t, taskRunner.definitions, 1)

	action := taskRunner.definitions[0].Actions[0]
	require.Equal(t, "repo.namespace.rewrite", action.Type)
	require.Equal(t, "github.com/old/org", action.Options["old"])
	require.Equal(t, "github.com/new/org", action.Options["new"])
	require.Equal(t, true, action.Options["push"])
	require.Equal(t, "ns", action.Options["branch_prefix"])
	require.Equal(t, map[string]any{"require_clean": true}, taskRunner.definitions[0].Safeguards)
}

func TestNamespaceCommandFlagOverrides(t *testing.T) {
	t.Parallel()

	taskRunner := &namespaceRecordingTaskRunner{}
	builder := repos.NamespaceCommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     &fakeRepositoryDiscoverer{repositories: []string{"/tmp/cli-root"}},
		GitExecutor:    &fakeGitExecutor{},
		GitManager:     &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:     filesystem.OSFileSystem{},
		ConfigurationProvider: func() repos.NamespaceConfiguration {
			return repos.NamespaceConfiguration{}
		},
		TaskRunnerFactory: func(workflow.Dependencies) repos.TaskRunnerExecutor { return taskRunner },
	}

	command, err := builder.Build()
	require.NoError(t, err)
	bindGlobalNamespaceFlags(command)

	args := flagutils.NormalizeToggleArguments([]string{
		"--dry-run",
		"--" + flagutils.AssumeYesFlagName,
		"--" + flagutils.DefaultRootFlagName, "/tmp/cli-root",
		"--old", "github.com/cli/old",
		"--new", "github.com/cli/new",
		"--branch-prefix", "rewrite",
		"--remote", "upstream",
		"--commit-message", "chore: rewrite",
		"--push=false",
	})
	command.SetContext(context.Background())
	command.SetArgs(args)

	runErr := command.Execute()
	require.NoError(t, runErr)

	require.Equal(t, []string{"/tmp/cli-root"}, taskRunner.roots)
	require.True(t, taskRunner.options.DryRun)
	require.True(t, taskRunner.options.AssumeYes)
	action := taskRunner.definitions[0].Actions[0]
	require.Equal(t, "github.com/cli/old", action.Options["old"])
	require.Equal(t, "github.com/cli/new", action.Options["new"])
	require.Equal(t, false, action.Options["push"])
	require.Equal(t, "rewrite", action.Options["branch_prefix"])
	require.Equal(t, "upstream", action.Options["remote"])
	require.Equal(t, "chore: rewrite", action.Options["commit_message"])
}

func TestNamespaceCommandRequiresPrefixes(t *testing.T) {
	t.Parallel()

	taskRunner := &namespaceRecordingTaskRunner{}
	builder := repos.NamespaceCommandBuilder{
		LoggerProvider:        func() *zap.Logger { return zap.NewNop() },
		Discoverer:            &fakeRepositoryDiscoverer{repositories: []string{"/tmp/root"}},
		GitExecutor:           &fakeGitExecutor{},
		GitManager:            &fakeGitRepositoryManager{cleanWorktree: true, cleanWorktreeSet: true, currentBranch: "main"},
		FileSystem:            filesystem.OSFileSystem{},
		ConfigurationProvider: func() repos.NamespaceConfiguration { return repos.NamespaceConfiguration{} },
		TaskRunnerFactory:     func(workflow.Dependencies) repos.TaskRunnerExecutor { return taskRunner },
	}

	command, err := builder.Build()
	require.NoError(t, err)
	bindGlobalNamespaceFlags(command)

	command.SetContext(context.Background())
	command.SetArgs(nil)

	runErr := command.Execute()
	require.Error(t, runErr)
}

func bindGlobalNamespaceFlags(command *cobra.Command) {
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Name: flagutils.DefaultRootFlagName, Usage: flagutils.DefaultRootFlagUsage, Enabled: true, Persistent: false})
	flagutils.BindExecutionFlags(command, flagutils.ExecutionDefaults{}, flagutils.ExecutionFlagDefinitions{
		DryRun:    flagutils.ExecutionFlagDefinition{Name: flagutils.DryRunFlagName, Usage: flagutils.DryRunFlagUsage, Enabled: true},
		AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Enabled: true},
	})
}
