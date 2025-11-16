package workflow_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/repos/shared"
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
			gitExecutor := &fakeWorkflowGitExecutor{}
			executorRecorder := &recordingExecutor{}

			builder := workflowcmd.CommandBuilder{
				LoggerProvider: func() *zap.Logger { return zap.NewNop() },
				Discoverer:     discoverer,
				GitExecutor:    gitExecutor,
				ConfigurationProvider: func() workflowcmd.CommandConfiguration {
					return testCase.configuration
				},
				OperationExecutorFactory: func(nodes []*workflowpkg.OperationNode, dependencies workflowpkg.Dependencies) workflowcmd.OperationExecutor {
					executorRecorder.captureOperations(nodes)
					return executorRecorder
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
				require.Equal(subtest, 0, executorRecorder.invocations)

				outputText := outputBuffer.String()
				require.Contains(subtest, outputText, workflowUsageSnippet)
				return
			}

			require.NoError(subtest, executionError)

			require.Equal(subtest, 1, executorRecorder.invocations)
			require.Equal(subtest, testCase.expectedRoots, executorRecorder.roots)
			require.NotEmpty(subtest, executorRecorder.operations)
			require.Equal(subtest, "audit report", executorRecorder.operations[0])
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
	gitExecutor := &fakeWorkflowGitExecutor{}
	executorRecorder := &recordingExecutor{}
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
		GitExecutor:    gitExecutor,
		ConfigurationProvider: func() workflowcmd.CommandConfiguration {
			return workflowcmd.CommandConfiguration{
				Roots: []string{workflowConfiguredRootConstant},
			}
		},
		OperationExecutorFactory: func(nodes []*workflowpkg.OperationNode, dependencies workflowpkg.Dependencies) workflowcmd.OperationExecutor {
			executorRecorder.captureOperations(nodes)
			return executorRecorder
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
	require.Equal(testInstance, 1, executorRecorder.invocations)
	require.Len(testInstance, executorRecorder.operations, 1)
	require.Equal(testInstance, "audit report", executorRecorder.operations[0])
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
	executorRecorder := &recordingExecutor{}

	builder := workflowcmd.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     discoverer,
		GitExecutor:    executor,
		ConfigurationProvider: func() workflowcmd.CommandConfiguration {
			return workflowcmd.CommandConfiguration{
				Roots: []string{workflowConfiguredRootConstant},
			}
		},
		OperationExecutorFactory: func(nodes []*workflowpkg.OperationNode, dependencies workflowpkg.Dependencies) workflowcmd.OperationExecutor {
			executorRecorder.captureOperations(nodes)
			return executorRecorder
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
	require.Equal(testInstance, "apache", executorRecorder.runtimeOptions.Variables["template"])
	require.Equal(testInstance, "demo", executorRecorder.runtimeOptions.Variables["scope"])
}

func TestWorkflowCommandAppliesOwnerVariableToCanonicalRemote(testInstance *testing.T) {
	discoverer := &fakeWorkflowDiscoverer{}
	executor := &fakeWorkflowGitExecutor{}
	executorRecorder := &recordingExecutor{}
	ownerPreset := workflowpkg.Configuration{
		Steps: []workflowpkg.StepConfiguration{
			{
				Command: []string{"remote", "update-to-canonical"},
				Options: map[string]any{"owner": ""},
			},
		},
	}
	var capturedNodes []*workflowpkg.OperationNode

	builder := workflowcmd.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     discoverer,
		GitExecutor:    executor,
		ConfigurationProvider: func() workflowcmd.CommandConfiguration {
			return workflowcmd.CommandConfiguration{Roots: []string{workflowConfiguredRootConstant}}
		},
		OperationExecutorFactory: func(nodes []*workflowpkg.OperationNode, dependencies workflowpkg.Dependencies) workflowcmd.OperationExecutor {
			capturedNodes = nodes
			executorRecorder.captureOperations(nodes)
			return executorRecorder
		},
		PresetCatalogFactory: func() workflowcmd.PresetCatalog {
			return &fakePresetCatalog{configurations: map[string]workflowpkg.Configuration{"remote-update-to-canonical": ownerPreset}}
		},
	}

	command, buildError := builder.Build()
	require.NoError(testInstance, buildError)
	bindGlobalWorkflowFlags(command)

	command.SetArgs([]string{"remote-update-to-canonical", "--roots", workflowConfiguredRootConstant, "--var", "owner=canonical"})
	command.SetContext(context.Background())

	require.NoError(testInstance, command.Execute())
	require.NotNil(testInstance, capturedNodes)
	found := false
	for index := range capturedNodes {
		node := capturedNodes[index]
		canonicalOperation, castSucceeded := node.Operation.(*workflowpkg.CanonicalRemoteOperation)
		if !castSucceeded {
			continue
		}
		found = true
		require.Equal(testInstance, "canonical", canonicalOperation.OwnerConstraint)
	}
	require.True(testInstance, found, "expected canonical remote operation to be built")
}

func TestWorkflowCommandAppliesProtocolVariablesToPreset(testInstance *testing.T) {
	discoverer := &fakeWorkflowDiscoverer{}
	executor := &fakeWorkflowGitExecutor{}
	executorRecorder := &recordingExecutor{}
	protocolPreset := workflowpkg.Configuration{
		Steps: []workflowpkg.StepConfiguration{
			{
				Command: []string{"remote", "update-protocol"},
				Options: map[string]any{"from": "", "to": ""},
			},
		},
	}
	var capturedNodes []*workflowpkg.OperationNode

	builder := workflowcmd.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     discoverer,
		GitExecutor:    executor,
		ConfigurationProvider: func() workflowcmd.CommandConfiguration {
			return workflowcmd.CommandConfiguration{Roots: []string{workflowConfiguredRootConstant}}
		},
		OperationExecutorFactory: func(nodes []*workflowpkg.OperationNode, dependencies workflowpkg.Dependencies) workflowcmd.OperationExecutor {
			capturedNodes = nodes
			executorRecorder.captureOperations(nodes)
			return executorRecorder
		},
		PresetCatalogFactory: func() workflowcmd.PresetCatalog {
			return &fakePresetCatalog{configurations: map[string]workflowpkg.Configuration{"remote-update-protocol": protocolPreset}}
		},
	}

	command, buildError := builder.Build()
	require.NoError(testInstance, buildError)
	bindGlobalWorkflowFlags(command)

	command.SetArgs([]string{"remote-update-protocol", "--roots", workflowConfiguredRootConstant, "--var", "from=https", "--var", "to=ssh"})
	command.SetContext(context.Background())

	require.NoError(testInstance, command.Execute())
	require.NotNil(testInstance, capturedNodes)
	found := false
	for index := range capturedNodes {
		node := capturedNodes[index]
		protocolOperation, castSucceeded := node.Operation.(*workflowpkg.ProtocolConversionOperation)
		if !castSucceeded {
			continue
		}
		found = true
		require.Equal(testInstance, shared.RemoteProtocolHTTPS, protocolOperation.FromProtocol)
		require.Equal(testInstance, shared.RemoteProtocolSSH, protocolOperation.ToProtocol)
	}
	require.True(testInstance, found, "expected protocol conversion operation to be built")
}

func TestWorkflowCommandLoadsVariablesFromFile(testInstance *testing.T) {
	tempDirectory := testInstance.TempDir()
	configPath := filepath.Join(tempDirectory, workflowConfigFileNameConstant)
	require.NoError(testInstance, os.WriteFile(configPath, []byte(workflowConfigContentConstant), 0o644))

	varFilePath := filepath.Join(tempDirectory, "vars.yaml")
	require.NoError(testInstance, os.WriteFile(varFilePath, []byte("branch: feature/license\nmode: overwrite\n"), 0o644))

	discoverer := &fakeWorkflowDiscoverer{}
	executor := &fakeWorkflowGitExecutor{}
	executorRecorder := &recordingExecutor{}

	builder := workflowcmd.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     discoverer,
		GitExecutor:    executor,
		ConfigurationProvider: func() workflowcmd.CommandConfiguration {
			return workflowcmd.CommandConfiguration{
				Roots: []string{workflowConfiguredRootConstant},
			}
		},
		OperationExecutorFactory: func(nodes []*workflowpkg.OperationNode, dependencies workflowpkg.Dependencies) workflowcmd.OperationExecutor {
			executorRecorder.captureOperations(nodes)
			return executorRecorder
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
	require.Equal(testInstance, "feature/license", executorRecorder.runtimeOptions.Variables["branch"])
	require.Equal(testInstance, "overwrite", executorRecorder.runtimeOptions.Variables["mode"])
}

func TestWorkflowCommandPrintsStageSummary(testInstance *testing.T) {
	tempDirectory := testInstance.TempDir()
	configPath := filepath.Join(tempDirectory, workflowConfigFileNameConstant)
	require.NoError(testInstance, os.WriteFile(configPath, []byte(workflowConfigContentConstant), 0o644))

	executorRecorder := &recordingExecutor{
		outcome: workflowpkg.ExecutionOutcome{
			RepositoryCount: 2,
			Duration:        1500 * time.Millisecond,
			StageOutcomes: []workflowpkg.StageOutcome{
				{Index: 0, Operations: []string{"audit.report-1", "tasks.apply-1"}, Duration: 500 * time.Millisecond},
				{Index: 1, Operations: []string{"tasks.apply-2"}, Duration: 250 * time.Millisecond},
			},
		},
	}

	builder := workflowcmd.CommandBuilder{
		LoggerProvider: func() *zap.Logger { return zap.NewNop() },
		Discoverer:     &fakeWorkflowDiscoverer{},
		GitExecutor:    &fakeWorkflowGitExecutor{},
		ConfigurationProvider: func() workflowcmd.CommandConfiguration {
			return workflowcmd.CommandConfiguration{Roots: []string{workflowConfiguredRootConstant}}
		},
		OperationExecutorFactory: func(nodes []*workflowpkg.OperationNode, dependencies workflowpkg.Dependencies) workflowcmd.OperationExecutor {
			executorRecorder.captureOperations(nodes)
			return executorRecorder
		},
	}

	command, buildError := builder.Build()
	require.NoError(testInstance, buildError)
	bindGlobalWorkflowFlags(command)

	var outputBuffer bytes.Buffer
	var errorBuffer bytes.Buffer
	command.SetOut(&outputBuffer)
	command.SetErr(&errorBuffer)
	command.SetContext(context.Background())
	command.SetArgs([]string{configPath})

	require.NoError(testInstance, command.Execute())
	errorText := strings.TrimSpace(errorBuffer.String())
	require.Equal(testInstance, "", errorText)
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

type recordingExecutor struct {
	roots          []string
	runtimeOptions workflowpkg.RuntimeOptions
	operations     []string
	invocations    int
	outcome        workflowpkg.ExecutionOutcome
	executeError   error
}

func (executor *recordingExecutor) captureOperations(nodes []*workflowpkg.OperationNode) {
	executor.operations = executor.operations[:0]
	for _, node := range nodes {
		if node == nil || node.Operation == nil {
			continue
		}
		executor.operations = append(executor.operations, node.Operation.Name())
	}
}

func (executor *recordingExecutor) Execute(_ context.Context, roots []string, options workflowpkg.RuntimeOptions) (workflowpkg.ExecutionOutcome, error) {
	executor.invocations++
	executor.roots = append([]string{}, roots...)
	executor.runtimeOptions = options
	return executor.outcome, executor.executeError
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
