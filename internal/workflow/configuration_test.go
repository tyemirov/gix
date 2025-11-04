package workflow_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	configurationTestFileName               = "workflow.yaml"
	configurationAnchoredSequenceCaseName   = "sequence with anchored operation defaults"
	configurationInlineSequenceCaseName     = "sequence with inline operation"
	configurationInvalidWorkflowMappingCase = "workflow mapping is rejected"
	configurationOptionFromKey              = "from"
	configurationOptionToKey                = "to"
	configurationOptionRequireClean         = "require_clean"
	configurationOptionIncludeOwnerKey      = "include_owner"
	configurationOptionOwnerKey             = "owner"
	anchoredWorkflowConfigurationTemplate   = `operations:
  - &protocol_conversion_step
    operation: convert-protocol
    with:
      from: https
      to: ssh
workflow:
  - step: *protocol_conversion_step
`
	inlineWorkflowConfiguration = `workflow:
  - step:
      operation: update-canonical-remote
`
	invalidWorkflowMappingConfiguration = `workflow:
  steps: []
`
)

func TestBuildOperations(testInstance *testing.T) {
	testCases := []struct {
		name                  string
		configuration         workflow.Configuration
		expectedOperationType workflow.OperationType
		assertFunc            func(*testing.T, *workflow.OperationNode)
	}{
		{
			name: "builds protocol conversion operation",
			configuration: workflow.Configuration{
				Steps: []workflow.StepConfiguration{
					{
						Operation: workflow.OperationTypeProtocolConversion,
						Options: map[string]any{
							configurationOptionFromKey: string(shared.RemoteProtocolHTTPS),
							configurationOptionToKey:   string(shared.RemoteProtocolSSH),
						},
					},
				},
			},
			expectedOperationType: workflow.OperationTypeProtocolConversion,
			assertFunc: func(testingInstance *testing.T, node *workflow.OperationNode) {
				require.NotNil(testingInstance, node)
				protocolConversionOperation, castSucceeded := node.Operation.(*workflow.ProtocolConversionOperation)
				require.True(testingInstance, castSucceeded)
				require.Equal(testingInstance, shared.RemoteProtocolHTTPS, protocolConversionOperation.FromProtocol)
				require.Equal(testingInstance, shared.RemoteProtocolSSH, protocolConversionOperation.ToProtocol)
			},
		},
		{
			name: "builds canonical remote operation",
			configuration: workflow.Configuration{
				Steps: []workflow.StepConfiguration{
					{
						Operation: workflow.OperationTypeCanonicalRemote,
						Options: map[string]any{
							configurationOptionOwnerKey: "  canonical  ",
						},
					},
				},
			},
			expectedOperationType: workflow.OperationTypeCanonicalRemote,
			assertFunc: func(testingInstance *testing.T, node *workflow.OperationNode) {
				require.NotNil(testingInstance, node)
				canonicalOperation, castSucceeded := node.Operation.(*workflow.CanonicalRemoteOperation)
				require.True(testingInstance, castSucceeded)
				require.Equal(testingInstance, "canonical", canonicalOperation.OwnerConstraint)
			},
		},
		{
			name: "builds rename operation with defaults",
			configuration: workflow.Configuration{
				Steps: []workflow.StepConfiguration{
					{Operation: workflow.OperationTypeRenameDirectories},
				},
			},
			expectedOperationType: workflow.OperationTypeRenameDirectories,
			assertFunc: func(testingInstance *testing.T, node *workflow.OperationNode) {
				require.NotNil(testingInstance, node)
				renameOperation, castSucceeded := node.Operation.(*workflow.RenameOperation)
				require.True(testingInstance, castSucceeded)
				require.False(testingInstance, renameOperation.RequireCleanWorktree)
				require.False(testingInstance, renameOperation.IncludeOwner)
			},
		},
		{
			name: "builds rename operation with include owner",
			configuration: workflow.Configuration{
				Steps: []workflow.StepConfiguration{
					{
						Operation: workflow.OperationTypeRenameDirectories,
						Options: map[string]any{
							configurationOptionRequireClean:    true,
							configurationOptionIncludeOwnerKey: true,
						},
					},
				},
			},
			expectedOperationType: workflow.OperationTypeRenameDirectories,
			assertFunc: func(testingInstance *testing.T, node *workflow.OperationNode) {
				require.NotNil(testingInstance, node)
				renameOperation, castSucceeded := node.Operation.(*workflow.RenameOperation)
				require.True(testingInstance, castSucceeded)
				require.True(testingInstance, renameOperation.RequireCleanWorktree)
				require.True(testingInstance, renameOperation.IncludeOwner)
			},
		},
		{
			name: "builds task operation",
			configuration: workflow.Configuration{
				Steps: []workflow.StepConfiguration{
					{
						Operation: workflow.OperationTypeApplyTasks,
						Options: map[string]any{
							"tasks": []any{
								map[string]any{
									"name": "add-agents",
									"files": []any{
										map[string]any{
											"path":    "AGENTS.md",
											"content": "example",
										},
									},
								},
							},
						},
					},
				},
			},
			expectedOperationType: workflow.OperationTypeApplyTasks,
			assertFunc: func(testingInstance *testing.T, node *workflow.OperationNode) {
				require.NotNil(testingInstance, node)
				require.IsType(testingInstance, &workflow.TaskOperation{}, node.Operation)
			},
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		testInstance.Run(testCase.name, func(testingInstance *testing.T) {
			nodes, buildError := workflow.BuildOperations(testCase.configuration)
			require.NoError(testingInstance, buildError)
			require.Len(testingInstance, nodes, 1)
			require.Equal(testingInstance, string(testCase.expectedOperationType), nodes[0].Operation.Name())
			testCase.assertFunc(testingInstance, nodes[0])
		})
	}
}

func TestBuildOperationsMissingOperation(testInstance *testing.T) {
	configuration := workflow.Configuration{
		Steps: []workflow.StepConfiguration{{}},
	}

	_, buildError := workflow.BuildOperations(configuration)
	require.Error(testInstance, buildError)
	require.ErrorContains(testInstance, buildError, "workflow step missing operation name")
}

func TestBuildOperationsApplyTasksValidation(testInstance *testing.T) {
	configuration := workflow.Configuration{
		Steps: []workflow.StepConfiguration{
			{Operation: workflow.OperationTypeApplyTasks},
		},
	}

	_, buildError := workflow.BuildOperations(configuration)
	require.Error(testInstance, buildError)
	require.ErrorContains(testInstance, buildError, "apply-tasks step requires at least one task entry")
}

func TestLoadConfiguration(testInstance *testing.T) {
	testCases := []struct {
		name              string
		contents          string
		expectError       bool
		expectedOperation workflow.OperationType
		expectedOptions   map[string]any
	}{
		{
			name:              configurationAnchoredSequenceCaseName,
			contents:          anchoredWorkflowConfigurationTemplate,
			expectError:       false,
			expectedOperation: workflow.OperationTypeProtocolConversion,
			expectedOptions: map[string]any{
				configurationOptionFromKey: "https",
				configurationOptionToKey:   "ssh",
			},
		},
		{
			name:              configurationInlineSequenceCaseName,
			contents:          inlineWorkflowConfiguration,
			expectError:       false,
			expectedOperation: workflow.OperationTypeCanonicalRemote,
			expectedOptions:   nil,
		},
		{
			name:        configurationInvalidWorkflowMappingCase,
			contents:    invalidWorkflowMappingConfiguration,
			expectError: true,
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		testInstance.Run(testCase.name, func(testingInstance *testing.T) {
			tempDirectory := testingInstance.TempDir()
			configurationPath := filepath.Join(tempDirectory, configurationTestFileName)
			require.NoError(testingInstance, os.WriteFile(configurationPath, []byte(testCase.contents), 0o644))

			configuration, loadError := workflow.LoadConfiguration(configurationPath)
			if testCase.expectError {
				require.Error(testingInstance, loadError)
				return
			}

			require.NoError(testingInstance, loadError)
			require.Len(testingInstance, configuration.Steps, 1)
			require.Equal(testingInstance, testCase.expectedOperation, configuration.Steps[0].Operation)
			require.Equal(testingInstance, testCase.expectedOptions, configuration.Steps[0].Options)
		})
	}
}

func TestLoadConfigurationMissingOperation(testInstance *testing.T) {
	tempDirectory := testInstance.TempDir()
	configurationPath := filepath.Join(tempDirectory, configurationTestFileName)
	require.NoError(testInstance, os.WriteFile(configurationPath, []byte("workflow:\n  - {}\n"), 0o644))

	_, loadError := workflow.LoadConfiguration(configurationPath)
	require.Error(testInstance, loadError)
	require.ErrorContains(testInstance, loadError, "workflow step missing operation name")
}
