package workflow

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temirov/gix/internal/workflow"
)

func TestApplyVariableOverridesIgnoresNonHistoryActions(t *testing.T) {
	configuration := &workflow.Configuration{
		Steps: []workflow.StepConfiguration{
			{
				Command: []string{"tasks", "apply"},
				Options: map[string]any{
					"tasks": []any{
						map[string]any{
							"actions": []any{
								map[string]any{
									"type": "repo.namespace.rewrite",
									"options": map[string]any{
										"push": "{{ .Environment.namespace_push }}",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	variables := map[string]string{"owner": "example"}

	applyVariableOverrides(configuration, variables)

	options := configuration.Steps[0].Options["tasks"].([]any)[0].(map[string]any)["actions"].([]any)[0].(map[string]any)["options"].(map[string]any)
	pushValue, isString := options["push"].(string)
	require.True(t, isString)
	require.Equal(t, "{{ .Environment.namespace_push }}", pushValue)
}

func TestApplyVariableOverridesUpdatesHistoryActionsWhenProvided(t *testing.T) {
	configuration := &workflow.Configuration{
		Steps: []workflow.StepConfiguration{
			{
				Command: []string{"tasks", "apply"},
				Options: map[string]any{
					"tasks": []any{
						map[string]any{
							"actions": []any{
								map[string]any{
									"type": "repo.history.purge",
									"options": map[string]any{
										"push":    true,
										"restore": true,
										"paths":   []any{"secret"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	variables := map[string]string{
		"push":  "false",
		"paths": "public.txt",
	}

	applyVariableOverrides(configuration, variables)

	options := configuration.Steps[0].Options["tasks"].([]any)[0].(map[string]any)["actions"].([]any)[0].(map[string]any)["options"].(map[string]any)
	pushValue, isBool := options["push"].(bool)
	require.True(t, isBool)
	require.False(t, pushValue)
	require.Equal(t, []string{"public.txt"}, options["paths"])
}
