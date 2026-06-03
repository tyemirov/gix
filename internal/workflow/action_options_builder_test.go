package workflow

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/gix/internal/repos/shared"
)

func TestActionOptionBuildersSerializeWorkflowContracts(testingInstance *testing.T) {
	testCases := []struct {
		name     string
		options  map[string]any
		expected map[string]any
	}{
		{
			name: "protocol conversion options",
			options: ProtocolConversionActionOptions{
				From: shared.RemoteProtocolHTTPS,
				To:   shared.RemoteProtocolSSH,
			}.Options(),
			expected: map[string]any{
				optionFromKeyConstant: string(shared.RemoteProtocolHTTPS),
				optionToKeyConstant:   string(shared.RemoteProtocolSSH),
			},
		},
		{
			name: "history purge options",
			options: HistoryPurgeActionOptions{
				Paths:       []string{"secret.txt", "nested/token.env"},
				RemoteName:  "origin",
				Push:        false,
				Restore:     true,
				PushMissing: true,
			}.Options(),
			expected: map[string]any{
				optionPathsKeyConstant:             []string{"secret.txt", "nested/token.env"},
				actionOptionRemoteKeyConstant:      "origin",
				actionOptionPushKeyConstant:        false,
				actionOptionRestoreKeyConstant:     true,
				actionOptionPushMissingKeyConstant: true,
			},
		},
		{
			name: "file replace options",
			options: FileReplaceActionOptions{
				Patterns: []string{"README.md", "docs/*.md"},
				Find:     "old",
				Replace:  "new",
				Command:  []string{"go", "test", "./..."},
			}.Options(),
			expected: map[string]any{
				fileReplacePatternsOptionKey: []string{"README.md", "docs/*.md"},
				fileReplaceFindOptionKey:     "old",
				fileReplaceReplaceOptionKey:  "new",
				fileReplaceCommandOptionKey:  []string{"go", "test", "./..."},
			},
		},
		{
			name: "audit report options",
			options: AuditReportActionOptions{
				IncludeAll: true,
				Debug:      true,
				Depth:      audit.InspectionDepthMinimal,
				OutputPath: "audit.csv",
			}.Options(),
			expected: map[string]any{
				actionOptionIncludeAllKeyConstant: true,
				actionOptionDebugKeyConstant:      true,
				actionOptionDepthKeyConstant:      string(audit.InspectionDepthMinimal),
				optionOutputPathKeyConstant:       "audit.csv",
			},
		},
	}

	for _, testCase := range testCases {
		testingInstance.Run(testCase.name, func(subtest *testing.T) {
			require.Equal(subtest, testCase.expected, testCase.options)
		})
	}
}

func TestFileReplaceSafeguardOptionsSerializeHardAndSoftSets(testingInstance *testing.T) {
	options := FileReplaceSafeguardOptions{
		RequireClean: true,
		Branch:       "main",
		Paths:        []string{"go.mod", "cmd/root.go"},
	}.Options()

	require.Equal(testingInstance, map[string]any{
		safeguardHardStopKeyConstant: map[string]any{
			optionRequireCleanKeyConstant: true,
		},
		safeguardSoftSkipKeyConstant: map[string]any{
			safeguardBranchKeyConstant: "main",
			optionPathsKeyConstant:     []string{"go.mod", "cmd/root.go"},
		},
	}, options)
}

func TestReleaseRetagActionOptionsSerializeTypedMappings(testingInstance *testing.T) {
	options := ReleaseRetagActionOptions{
		RemoteName: "origin",
		Mappings: []ReleaseRetagMappingOptions{
			{TagName: "v1.2.3", TargetReference: "main", Message: "Retag main"},
			{TagName: "v1.2.4", TargetReference: "release"},
		},
	}.Options()

	require.Equal(testingInstance, "origin", options[actionOptionRemoteKeyConstant])
	entries, exists, readError := newOptionReader(options).mapSlice(actionOptionMappingsKeyConstant)
	require.NoError(testingInstance, readError)
	require.True(testingInstance, exists)
	require.Equal(testingInstance, []map[string]any{
		{
			actionOptionTagKeyConstant:     "v1.2.3",
			actionOptionTargetKeyConstant:  "main",
			actionOptionMessageKeyConstant: "Retag main",
		},
		{
			actionOptionTagKeyConstant:    "v1.2.4",
			actionOptionTargetKeyConstant: "release",
		},
	}, entries)
}

func TestApplyHistoryPurgeActionOverridesMergesFirstHistoryAction(testingInstance *testing.T) {
	options := TasksApplyDefinition{
		Tasks: []TaskDefinition{
			{
				Name: "Mixed actions",
				Actions: []TaskActionDefinition{
					{
						Type: TaskActionNamespaceRewriteType,
						Options: map[string]any{
							actionOptionPushKeyConstant: "{{ .Environment.namespace_push }}",
						},
					},
					{
						Type: TaskActionHistoryPurgeType,
						Options: HistoryPurgeActionOptions{
							Paths:       []string{"secret.txt"},
							RemoteName:  "origin",
							Push:        true,
							Restore:     true,
							PushMissing: false,
						}.Options(),
					},
				},
			},
		},
	}.Options()

	pushOverride := false
	updated, applied, applyError := ApplyHistoryPurgeActionOverrides(options, HistoryPurgeActionOverrides{
		Paths: []string{"public.txt"},
		Push:  &pushOverride,
	})

	require.NoError(testingInstance, applyError)
	require.True(testingInstance, applied)

	tasks := updated[optionTasksKeyConstant].([]any)
	taskEntry := tasks[0].(map[string]any)
	actions := taskEntry[optionTaskActionsKeyConstant].([]any)
	namespaceAction := actions[0].(map[string]any)
	namespaceOptions := namespaceAction[optionTaskActionOptionsKeyConstant].(map[string]any)
	require.Equal(testingInstance, "{{ .Environment.namespace_push }}", namespaceOptions[actionOptionPushKeyConstant])

	historyAction := actions[1].(map[string]any)
	historyOptions := historyAction[optionTaskActionOptionsKeyConstant].(map[string]any)
	require.Equal(testingInstance, []string{"public.txt"}, historyOptions[optionPathsKeyConstant])
	require.Equal(testingInstance, "origin", historyOptions[actionOptionRemoteKeyConstant])
	require.Equal(testingInstance, false, historyOptions[actionOptionPushKeyConstant])
	require.Equal(testingInstance, true, historyOptions[actionOptionRestoreKeyConstant])
	require.Equal(testingInstance, false, historyOptions[actionOptionPushMissingKeyConstant])
}
