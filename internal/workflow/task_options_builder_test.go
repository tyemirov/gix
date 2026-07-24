package workflow

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/llmclient"
)

func TestTasksApplyDefinitionOptionsSerializesTaskDefinition(t *testing.T) {
	t.Helper()

	definition := TasksApplyDefinition{
		Tasks: []TaskDefinition{
			{
				Name: "Add repository file README.md",
				Branch: TaskBranchDefinition{
					NameTemplate:       "feature/{{ .Repository.Name }}",
					StartPointTemplate: "main",
					PushRemote:         "origin",
				},
				Files: []TaskFileDefinition{
					{
						PathTemplate:    "README.md",
						ContentTemplate: "hello world",
						Mode:            TaskFileModeOverwrite,
						Permissions:     0o640,
						Replacements: []TaskReplacementDefinition{
							{
								FromTemplate: "foo",
								ToTemplate:   "bar",
							},
						},
					},
				},
				Commit: TaskCommitDefinition{MessageTemplate: "docs: update README"},
				Safeguards: map[string]any{
					"hard_stop": map[string]any{"require_clean": true},
				},
				Steps: []TaskExecutionStep{
					TaskExecutionStepBranchPrepare,
					TaskExecutionStepFilesApply,
					TaskExecutionStepGitStageCommit,
				},
			},
		},
	}

	options := definition.Options()
	encodedTasks, ok := options[optionTasksKeyConstant].([]any)
	require.True(t, ok)
	require.Len(t, encodedTasks, 1)

	taskEntry, ok := encodedTasks[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "Add repository file README.md", taskEntry[optionTaskNameKeyConstant])
	require.Equal(t, "docs: update README", taskEntry[optionTaskCommitMessageKeyConstant])

	branchEntry, ok := taskEntry[optionTaskBranchKeyConstant].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "feature/{{ .Repository.Name }}", branchEntry[optionTaskBranchNameKeyConstant])
	require.Equal(t, "main", branchEntry[optionTaskBranchStartPointKeyConstant])
	require.Equal(t, "origin", branchEntry[optionTaskBranchPushRemoteKeyConstant])

	fileEntries, ok := taskEntry[optionTaskFilesKeyConstant].([]any)
	require.True(t, ok)
	require.Len(t, fileEntries, 1)

	fileEntry, ok := fileEntries[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "README.md", fileEntry[optionTaskFilePathKeyConstant])
	require.Equal(t, "hello world", fileEntry[optionTaskFileContentKeyConstant])
	require.Equal(t, string(TaskFileModeOverwrite), fileEntry[optionTaskFileModeKeyConstant])
	require.Equal(t, 0o640, fileEntry[optionTaskFilePermissionsKeyConstant])

	replacementEntries, ok := fileEntry[optionTaskFileReplacementsKeyConstant].([]any)
	require.True(t, ok)
	require.Len(t, replacementEntries, 1)
	replacementEntry, ok := replacementEntries[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "foo", replacementEntry[optionTaskReplacementFromKeyConstant])
	require.Equal(t, "bar", replacementEntry[optionTaskReplacementToKeyConstant])

	safeguards := taskEntry[optionTaskSafeguardsKeyConstant].(map[string]any)
	hardStop := safeguards["hard_stop"].(map[string]any)
	require.True(t, hardStop["require_clean"].(bool))

	steps, ok := taskEntry[optionTaskStepsKeyConstant].([]string)
	require.True(t, ok)
	require.Equal(t, []string{
		string(TaskExecutionStepBranchPrepare),
		string(TaskExecutionStepFilesApply),
		string(TaskExecutionStepGitStageCommit),
	}, steps)
}

func TestTasksApplyDefinitionOptionsSerializesLLMConfiguration(t *testing.T) {
	t.Helper()

	temperature := 0.2
	definition := TasksApplyDefinition{
		Tasks: []TaskDefinition{
			{
				Name: "Test task",
			},
		},
		LLM: &TaskLLMDefinition{
			LLMProxy: llmclient.LLMProxySelection{
				Provider: "deepseek",
				Model:    "gpt-test",
			},
			TimeoutSeconds:      30,
			MaxCompletionTokens: 512,
			Temperature:         &temperature,
		},
	}

	options := definition.Options()
	llmOptions, ok := options[optionTaskLLMKeyConstant].(map[string]any)
	require.True(t, ok)
	proxyOptions, ok := llmOptions[optionTaskLLMProxyKeyConstant].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "deepseek", proxyOptions[optionTaskLLMProviderKeyConstant])
	require.Equal(t, "gpt-test", proxyOptions[optionTaskLLMModelKeyConstant])
	require.Equal(t, 30, llmOptions[optionTaskLLMTimeoutKeyConstant])
	require.Equal(t, 512, llmOptions[optionTaskLLMMaxTokensKeyConstant])
	require.Equal(t, temperature, llmOptions[optionTaskLLMTemperatureKeyConstant])
}
