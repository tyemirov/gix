package workflow_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/utils"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	rootutils "github.com/temirov/gix/internal/utils/roots"
	workflowpkg "github.com/temirov/gix/internal/workflow"
)

const (
	workflowConfigFileNameConstant          = "config.yaml"
	workflowConfigContentConstant           = "operations:\n  - command: [\"workflow\"]\n    with:\n      roots:\n        - .\nworkflow:\n  - step:\n      command: [\"audit\", \"report\"]\n"
	workflowApplyTasksConfigContentConstant = `
workflow:
  - step:
      command: ["tasks", "apply"]
      with:
        tasks:
          - name: Add Notes
            files:
              - path: NOTES.md
                content: "Repository: {{ .Repository.Name }}"
`
	workflowConfiguredRootConstant = "/tmp/workflow-config-root"
	workflowCliRootConstant        = "/tmp/workflow-cli-root"
	workflowRootsFlagConstant      = "--" + flagutils.DefaultRootFlagName
	workflowUsageSnippet           = "Usage:"
)

var workflowMissingRootsErrorMessage = rootutils.MissingRootsMessage()

func TestWorkflowCommandConfigurationPrecedence(testInstance *testing.T) {
	testCases := []struct {
		name                 string
		configuration        workflowcmd.CommandConfiguration
		additionalArgs       []string
		expectedRoots        []string
		expectPlanMessage    bool
		expectExecutionError bool
		expectedErrorMessage string
	}{
		{
			name: "configuration_applies_without_flags",
			configuration: workflowcmd.CommandConfiguration{
				Roots: []string{workflowConfiguredRootConstant},
			},
			additionalArgs:       []string{},
			expectedRoots:        []string{workflowConfiguredRootConstant},
			expectPlanMessage:    true,
			expectExecutionError: false,
		},
		{
			name: "flags_override_configuration",
			configuration: workflowcmd.CommandConfiguration{
				Roots: []string{workflowConfiguredRootConstant},
			},
			additionalArgs: []string{
				workflowRootsFlagConstant,
				workflowCliRootConstant,
			},
			expectedRoots:        []string{workflowCliRootConstant},
			expectPlanMessage:    true,
			expectExecutionError: false,
		},
		{
			name: "flag_disables_require_clean_with_no_literal",
			configuration: workflowcmd.CommandConfiguration{
				Roots:        []string{workflowConfiguredRootConstant},
				RequireClean: true,
			},
			additionalArgs: []string{
				workflowRootsFlagConstant,
				workflowConfiguredRootConstant,
				"--require-clean",
				"no",
			},
			expectedRoots:        []string{workflowConfiguredRootConstant},
			expectPlanMessage:    false,
			expectExecutionError: false,
		},
		{
			name:                 "error_when_roots_missing",
			configuration:        workflowcmd.CommandConfiguration{},
			additionalArgs:       []string{},
			expectedRoots:        nil,
			expectPlanMessage:    false,
			expectExecutionError: true,
			expectedErrorMessage: workflowMissingRootsErrorMessage,
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			tempDirectory := subtest.TempDir()
			configPath := filepath.Join(tempDirectory, workflowConfigFileNameConstant)
			writeError := os.WriteFile(configPath, []byte(workflowConfigContentConstant), 0o644)
			require.NoError(subtest, writeError)

			discoverer := &fakeWorkflowDiscoverer{}
			executor := &fakeWorkflowGitExecutor{}
			runner := &recordingTaskRunner{}

			builder := workflowcmd.CommandBuilder{
				LoggerProvider: func() *zap.Logger { return zap.NewNop() },
				Discoverer:     discoverer,
				GitExecutor:    executor,
				ConfigurationProvider: func() workflowcmd.CommandConfiguration {
					return testCase.configuration
				},
				TaskRunnerFactory: func(workflowpkg.Dependencies) workflowcmd.TaskRunnerExecutor {
					return runner
				},
			}

			command, buildError := builder.Build()
			require.NoError(subtest, buildError)
			bindGlobalWorkflowFlags(command)
			flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})

			var outputBuffer bytes.Buffer
			var errorBuffer bytes.Buffer
			command.SetOut(&outputBuffer)
			command.SetErr(&errorBuffer)
			command.SetContext(context.Background())

			arguments := append([]string{configPath}, testCase.additionalArgs...)
			normalizedArguments := flagutils.NormalizeToggleArguments(arguments)
			command.SetArgs(normalizedArguments)

			executionError := command.Execute()

			if testCase.expectExecutionError {
				require.Error(subtest, executionError)
				require.EqualError(subtest, executionError, testCase.expectedErrorMessage)
				require.Nil(subtest, discoverer.receivedRoots)
				require.Equal(subtest, 0, runner.invocations)

				outputText := outputBuffer.String()
				require.Contains(subtest, outputText, workflowUsageSnippet)
				return
			}

			require.NoError(subtest, executionError)

			require.Equal(subtest, 1, runner.invocations)
			require.Equal(subtest, testCase.expectedRoots, runner.roots)
			require.NotEmpty(subtest, runner.definitions)
			require.Equal(subtest, "audit.report", runner.definitions[0].Actions[0].Type)
		})
	}
}

func bindGlobalWorkflowFlags(command *cobra.Command) {
	flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Enabled: true})
	flagutils.BindExecutionFlags(command, flagutils.ExecutionDefaults{}, flagutils.ExecutionFlagDefinitions{
		AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Shorthand: flagutils.AssumeYesFlagShorthand, Enabled: true},
	})
	command.PersistentFlags().String(flagutils.RemoteFlagName, "", flagutils.RemoteFlagUsage)
	command.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		contextAccessor := utils.NewCommandContextAccessor()
		executionFlags := utils.ExecutionFlags{}
		if assumeYesValue, assumeYesChanged, assumeYesError := flagutils.BoolFlag(cmd, flagutils.AssumeYesFlagName); assumeYesError == nil {
			executionFlags.AssumeYes = assumeYesValue
			executionFlags.AssumeYesSet = assumeYesChanged
		}
		if remoteValue, remoteChanged, remoteError := flagutils.StringFlag(cmd, flagutils.RemoteFlagName); remoteError == nil {
			executionFlags.Remote = strings.TrimSpace(remoteValue)
			executionFlags.RemoteSet = remoteChanged && len(strings.TrimSpace(remoteValue)) > 0
		}
		updatedContext := contextAccessor.WithExecutionFlags(cmd.Context(), executionFlags)
		cmd.SetContext(updatedContext)
		return nil
	}
}

func TestWorkflowCommandRunsPreset(testInstance *testing.T) {
	discoverer := &fakeWorkflowDiscoverer{}
	executor := &fakeWorkflowGitExecutor{}
	runner := &recordingTaskRunner{}
	presetCatalog := &fakePresetCatalog{
		metadata: []workflowcmd.PresetMetadata{
			{Name: "license", Description: "License audit workflow"},
		},
		configurations: map[string]workflowpkg.Configuration{
			"license": {
				Steps: []workflowpkg.StepConfiguration{
					{Command: []string{"audit", "report"}},
				},
			},
		},
	}

	builder := workflowcmd.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     discoverer,
		GitExecutor:    executor,
		ConfigurationProvider: func() workflowcmd.CommandConfiguration {
			return workflowcmd.CommandConfiguration{
				Roots: []string{workflowConfiguredRootConstant},
			}
		},
		TaskRunnerFactory: func(workflowpkg.Dependencies) workflowcmd.TaskRunnerExecutor {
			return runner
		},
		PresetCatalogFactory: func() workflowcmd.PresetCatalog {
			return presetCatalog
		},
	}

	command, buildError := builder.Build()
	require.NoError(testInstance, buildError)
	bindGlobalWorkflowFlags(command)

	var outputBuffer bytes.Buffer
	command.SetOut(&outputBuffer)
	command.SetErr(&outputBuffer)
	command.SetContext(context.Background())
	command.SetArgs([]string{"license"})

	require.NoError(testInstance, command.Execute())
	require.Equal(testInstance, 1, runner.invocations)
	require.Len(testInstance, runner.definitions, 1)
	require.Equal(testInstance, "audit.report", runner.definitions[0].Actions[0].Type)
}

func TestWorkflowCommandListsPresets(testInstance *testing.T) {
	builder := workflowcmd.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		PresetCatalogFactory: func() workflowcmd.PresetCatalog {
			return &fakePresetCatalog{
				metadata: []workflowcmd.PresetMetadata{
					{Name: "license", Description: "License audit workflow"},
					{Name: "namespace"},
				},
			}
		},
	}

	command, buildError := builder.Build()
	require.NoError(testInstance, buildError)
	bindGlobalWorkflowFlags(command)

	var outputBuffer bytes.Buffer
	command.SetOut(&outputBuffer)
	command.SetErr(&outputBuffer)
	command.SetContext(context.Background())
	command.SetArgs([]string{"--list-presets"})

	require.NoError(testInstance, command.Execute())
	output := outputBuffer.String()
	require.Contains(testInstance, output, "Embedded workflows")
	require.Contains(testInstance, output, "license")
	require.Contains(testInstance, output, "namespace")
}

func TestWorkflowCommandPassesVariablesFromFlags(testInstance *testing.T) {
	tempDirectory := testInstance.TempDir()
	configPath := filepath.Join(tempDirectory, workflowConfigFileNameConstant)
	require.NoError(testInstance, os.WriteFile(configPath, []byte(workflowConfigContentConstant), 0o644))

	discoverer := &fakeWorkflowDiscoverer{}
	executor := &fakeWorkflowGitExecutor{}
	runner := &recordingTaskRunner{}

	builder := workflowcmd.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     discoverer,
		GitExecutor:    executor,
		ConfigurationProvider: func() workflowcmd.CommandConfiguration {
			return workflowcmd.CommandConfiguration{
				Roots: []string{workflowConfiguredRootConstant},
			}
		},
		TaskRunnerFactory: func(workflowpkg.Dependencies) workflowcmd.TaskRunnerExecutor {
			return runner
		},
	}

	command, buildError := builder.Build()
	require.NoError(testInstance, buildError)
	bindGlobalWorkflowFlags(command)

	var outputBuffer bytes.Buffer
	command.SetOut(&outputBuffer)
	command.SetErr(&outputBuffer)
	command.SetContext(context.Background())
	command.SetArgs([]string{configPath, "--var", "template=apache", "--var", "scope=demo"})

	require.NoError(testInstance, command.Execute())
	require.Equal(testInstance, "apache", runner.runtimeOptions.Variables["template"])
	require.Equal(testInstance, "demo", runner.runtimeOptions.Variables["scope"])
}

func TestWorkflowCommandLoadsVariablesFromFile(testInstance *testing.T) {
	tempDirectory := testInstance.TempDir()
	configPath := filepath.Join(tempDirectory, workflowConfigFileNameConstant)
	require.NoError(testInstance, os.WriteFile(configPath, []byte(workflowConfigContentConstant), 0o644))

	varFilePath := filepath.Join(tempDirectory, "vars.yaml")
	require.NoError(testInstance, os.WriteFile(varFilePath, []byte("branch: feature/license\nmode: overwrite\n"), 0o644))

	discoverer := &fakeWorkflowDiscoverer{}
	executor := &fakeWorkflowGitExecutor{}
	runner := &recordingTaskRunner{}

	builder := workflowcmd.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     discoverer,
		GitExecutor:    executor,
		ConfigurationProvider: func() workflowcmd.CommandConfiguration {
			return workflowcmd.CommandConfiguration{
				Roots: []string{workflowConfiguredRootConstant},
			}
		},
		TaskRunnerFactory: func(workflowpkg.Dependencies) workflowcmd.TaskRunnerExecutor {
			return runner
		},
	}

	command, buildError := builder.Build()
	require.NoError(testInstance, buildError)
	bindGlobalWorkflowFlags(command)

	var outputBuffer bytes.Buffer
	command.SetOut(&outputBuffer)
	command.SetErr(&outputBuffer)
	command.SetContext(context.Background())
	command.SetArgs([]string{configPath, "--var-file", varFilePath})

	require.NoError(testInstance, command.Execute())
	require.Equal(testInstance, "feature/license", runner.runtimeOptions.Variables["branch"])
	require.Equal(testInstance, "overwrite", runner.runtimeOptions.Variables["mode"])
}

type fakeWorkflowDiscoverer struct {
	receivedRoots []string
	repositories  []string
}

func (discoverer *fakeWorkflowDiscoverer) DiscoverRepositories(roots []string) ([]string, error) {
	discoverer.receivedRoots = append([]string{}, roots...)
	if len(discoverer.repositories) == 0 {
		return []string{}, nil
	}
	return append([]string{}, discoverer.repositories...), nil
}

type fakeWorkflowGitExecutor struct{}

func (executor *fakeWorkflowGitExecutor) ExecuteGit(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{StandardOutput: ""}, nil
}

func (executor *fakeWorkflowGitExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{StandardOutput: ""}, nil
}

type recordingTaskRunner struct {
	roots          []string
	definitions    []workflowpkg.TaskDefinition
	runtimeOptions workflowpkg.RuntimeOptions
	invocations    int
}

func (runner *recordingTaskRunner) Run(_ context.Context, roots []string, definitions []workflowpkg.TaskDefinition, options workflowpkg.RuntimeOptions) error {
	runner.invocations++
	runner.roots = append([]string{}, roots...)
	runner.definitions = append([]workflowpkg.TaskDefinition{}, definitions...)
	runner.runtimeOptions = options
	return nil
}

type fakePresetCatalog struct {
	metadata       []workflowcmd.PresetMetadata
	configurations map[string]workflowpkg.Configuration
}

func (catalog *fakePresetCatalog) List() []workflowcmd.PresetMetadata {
	if catalog == nil || len(catalog.metadata) == 0 {
		return nil
	}
	list := make([]workflowcmd.PresetMetadata, len(catalog.metadata))
	copy(list, catalog.metadata)
	return list
}

func (catalog *fakePresetCatalog) Load(name string) (workflowpkg.Configuration, bool, error) {
	if catalog == nil || len(catalog.configurations) == 0 {
		return workflowpkg.Configuration{}, false, nil
	}
	normalized := strings.ToLower(strings.TrimSpace(name))
	for key, configuration := range catalog.configurations {
		if strings.ToLower(key) == normalized {
			return configuration, true, nil
		}
	}
	return workflowpkg.Configuration{}, false, nil
}
