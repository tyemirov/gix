package release

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	repocli "github.com/temirov/gix/cmd/cli/repos"
	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/utils"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
)

type stubGitExecutor struct {
	recorded []execshell.CommandDetails
}

func (executor *stubGitExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.recorded = append(executor.recorded, details)
	return execshell.ExecutionResult{}, nil
}

func (executor *stubGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func TestCommandBuilds(t *testing.T) {
	builder := CommandBuilder{}
	command, err := builder.Build()
	require.NoError(t, err)
	require.IsType(t, &cobra.Command{}, command)
	require.Equal(t, commandUsageTemplate, strings.TrimSpace(command.Use))
	require.NotEmpty(t, strings.TrimSpace(command.Example))
}

func TestCommandRequiresTagArgument(t *testing.T) {
	builder := CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() CommandConfiguration {
			return CommandConfiguration{}
		},
		GitExecutor: &stubGitExecutor{},
	}
	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	require.Error(t, command.RunE(command, []string{}))
	require.Error(t, command.RunE(command, []string{"   "}))
}

func TestCommandRunsAcrossRoots(t *testing.T) {
	executor := &stubGitExecutor{}
	root := t.TempDir()
	recording := &releaseRecordingExecutor{}
	preset := loadReleaseTagPreset(t)
	builder := CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() CommandConfiguration {
			return CommandConfiguration{RepositoryRoots: []string{root}, RemoteName: "origin", Message: "Ship it"}
		},
		GitExecutor:          executor,
		PresetCatalogFactory: func() workflowcmd.PresetCatalog { return &fakePresetCatalog{configuration: preset, found: true} },
		WorkflowExecutorFactory: func(nodes []*workflow.OperationNode, _ workflow.Dependencies) repocli.WorkflowExecutor {
			require.Len(t, nodes, 1)
			taskOp, ok := nodes[0].Operation.(*workflow.TaskOperation)
			require.True(t, ok)
			defs := taskOp.Definitions()
			require.Len(t, defs, 1)
			actionDefs := defs[0].Actions
			require.Len(t, actionDefs, 1)
			action := actionDefs[0]
			require.Equal(t, "repo.release.tag", action.Type)
			require.Equal(t, "v1.2.3", action.Options["tag"])
			require.Equal(t, "Ship it", action.Options["message"])
			require.Equal(t, "origin", action.Options["remote"])
			return recording
		},
	}
	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

	contextAccessor := utils.NewCommandContextAccessor()
	command.SetContext(contextAccessor.WithExecutionFlags(context.Background(), utils.ExecutionFlags{}))

	require.NoError(t, command.RunE(command, []string{"v1.2.3"}))
	require.Equal(t, []string{root}, recording.roots)
	require.False(t, recording.options.AssumeYes)
}

func TestRetagCommandRequiresMappings(t *testing.T) {
	builder := RetagCommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() CommandConfiguration {
			return CommandConfiguration{}
		},
		GitExecutor: &stubGitExecutor{},
	}
	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	require.Error(t, command.RunE(command, nil))
}

func TestRetagCommandBuildsMappings(t *testing.T) {
	root := t.TempDir()
	recording := &releaseRecordingExecutor{}
	preset := loadReleaseRetagPreset(t)
	builder := RetagCommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		ConfigurationProvider: func() CommandConfiguration {
			return CommandConfiguration{RepositoryRoots: []string{root}, RemoteName: "origin", Message: "Retag {{tag}} -> {{target}}"}
		},
		GitExecutor:          &stubGitExecutor{},
		PresetCatalogFactory: func() workflowcmd.PresetCatalog { return &fakePresetCatalog{configuration: preset, found: true} },
		WorkflowExecutorFactory: func(nodes []*workflow.OperationNode, _ workflow.Dependencies) repocli.WorkflowExecutor {
			require.Len(t, nodes, 1)
			taskOp, ok := nodes[0].Operation.(*workflow.TaskOperation)
			require.True(t, ok)
			defs := taskOp.Definitions()
			require.Len(t, defs, 1)
			actionDefs := defs[0].Actions
			require.Len(t, actionDefs, 1)
			action := actionDefs[0]
			require.Equal(t, "repo.release.retag", action.Type)
			mappings, ok := action.Options["mappings"].([]any)
			require.True(t, ok)
			require.Len(t, mappings, 2)
			first, ok := mappings[0].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "v1.0.0", first["tag"])
			require.Equal(t, "main", first["target"])
			require.Equal(t, "Retag v1.0.0 -> main", first["message"])
			require.Equal(t, "origin", action.Options["remote"])
			return recording
		},
	}
	command, err := builder.Build()
	require.NoError(t, err)
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	flagutils.BindExecutionFlags(command, flagutils.ExecutionDefaults{}, flagutils.ExecutionFlagDefinitions{
		AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Shorthand: flagutils.AssumeYesFlagShorthand, Enabled: true},
	})

	args := []string{
		"--" + retagMappingFlagName, "v1.0.0=main",
		"--" + retagMappingFlagName, "v1.1.0=feature",
	}
	command.SetArgs(args)
	command.SetContext(context.Background())
	require.NoError(t, command.Execute())

	require.Equal(t, []string{root}, recording.roots)
}

func TestReleaseCommandPresetErrorsSurface(t *testing.T) {
	preset := loadReleaseTagPreset(t)

	testCases := []struct {
		name      string
		catalog   fakePresetCatalog
		expectErr string
	}{
		{
			name:      "missing_preset",
			catalog:   fakePresetCatalog{found: false},
			expectErr: releasePresetMissingMsg,
		},
		{
			name:      "load_error",
			catalog:   fakePresetCatalog{found: true, loadError: errors.New("boom")},
			expectErr: "unable to load release-tag preset: boom",
		},
		{
			name: "build_error",
			catalog: fakePresetCatalog{
				found: true,
				configuration: workflow.Configuration{
					Steps: []workflow.StepConfiguration{{Command: []string{"unknown"}}},
				},
			},
			expectErr: "unable to build release-tag workflow: unsupported workflow command: unknown",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(subtest *testing.T) {
			catalog := testCase.catalog
			if len(catalog.configuration.Steps) == 0 && catalog.found && catalog.loadError == nil && testCase.name != "build_error" {
				catalog.configuration = preset
			}

			builder := CommandBuilder{
				LoggerProvider:        func() *zap.Logger { return zap.NewNop() },
				ConfigurationProvider: func() CommandConfiguration { return CommandConfiguration{RepositoryRoots: []string{t.TempDir()}} },
				GitExecutor:           &stubGitExecutor{},
				PresetCatalogFactory:  func() workflowcmd.PresetCatalog { return &catalog },
			}
			command, err := builder.Build()
			require.NoError(subtest, err)
			flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			executionErr := command.RunE(command, []string{"v1.0.0"})
			require.EqualError(subtest, executionErr, testCase.expectErr)
		})
	}
}

func TestRetagCommandPresetErrorsSurface(t *testing.T) {
	preset := loadReleaseRetagPreset(t)

	testCases := []struct {
		name      string
		catalog   fakePresetCatalog
		expectErr string
	}{
		{
			name:      "missing_preset",
			catalog:   fakePresetCatalog{found: false},
			expectErr: retagPresetMissingMessage,
		},
		{
			name:      "load_error",
			catalog:   fakePresetCatalog{found: true, loadError: errors.New("boom")},
			expectErr: "unable to load release-retag preset: boom",
		},
		{
			name: "build_error",
			catalog: fakePresetCatalog{
				found: true,
				configuration: workflow.Configuration{
					Steps: []workflow.StepConfiguration{{Command: []string{"unknown"}}},
				},
			},
			expectErr: "unable to build release-retag workflow: unsupported workflow command: unknown",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(subtest *testing.T) {
			catalog := testCase.catalog
			if len(catalog.configuration.Steps) == 0 && catalog.found && catalog.loadError == nil && testCase.name != "build_error" {
				catalog.configuration = preset
			}

			builder := RetagCommandBuilder{
				LoggerProvider:        func() *zap.Logger { return zap.NewNop() },
				ConfigurationProvider: func() CommandConfiguration { return CommandConfiguration{RepositoryRoots: []string{t.TempDir()}} },
				GitExecutor:           &stubGitExecutor{},
				PresetCatalogFactory:  func() workflowcmd.PresetCatalog { return &catalog },
			}
			command, err := builder.Build()
			require.NoError(subtest, err)
			flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
			command.SetOut(&bytes.Buffer{})
			command.SetErr(&bytes.Buffer{})
			command.SetArgs([]string{"--map", "v1.0.0=main"})
			err = command.Execute()
			require.EqualError(subtest, err, testCase.expectErr)
		})
	}
}

type fakePresetCatalog struct {
	configuration workflow.Configuration
	found         bool
	loadError     error
}

func (catalog *fakePresetCatalog) List() []workflowcmd.PresetMetadata {
	return nil
}

func (catalog *fakePresetCatalog) Load(name string) (workflow.Configuration, bool, error) {
	if catalog == nil {
		return workflow.Configuration{}, false, nil
	}
	if catalog.loadError != nil {
		return workflow.Configuration{}, true, catalog.loadError
	}
	if !catalog.found {
		return workflow.Configuration{}, false, nil
	}
	return catalog.configuration, true, nil
}

type releaseRecordingExecutor struct {
	roots   []string
	options workflow.RuntimeOptions
}

func (executor *releaseRecordingExecutor) Execute(_ context.Context, roots []string, options workflow.RuntimeOptions) (workflow.ExecutionOutcome, error) {
	executor.roots = append([]string{}, roots...)
	executor.options = options
	return workflow.ExecutionOutcome{}, nil
}

func loadReleaseTagPreset(testingInstance testing.TB) workflow.Configuration {
	presetContent := []byte(`workflow:
  - step:
      name: release-tag
      command: ["tasks", "apply"]
      with:
        tasks:
          - name: Create release tag
            ensure_clean: false
            actions:
              - type: repo.release.tag
                options:
                  tag: ""
                  message: ""
                  remote: ""
`)
	configuration, err := workflow.ParseConfiguration(presetContent)
	require.NoError(testingInstance, err)
	return configuration
}

func loadReleaseRetagPreset(testingInstance testing.TB) workflow.Configuration {
	presetContent := []byte(`workflow:
  - step:
      name: release-retag
      command: ["tasks", "apply"]
      with:
        tasks:
          - name: Retag release tags
            ensure_clean: false
            actions:
              - type: repo.release.retag
                options:
                  mappings: []
                  remote: ""
`)
	configuration, err := workflow.ParseConfiguration(presetContent)
	require.NoError(testingInstance, err)
	return configuration
}
