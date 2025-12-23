package workflow

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/licenses"
	"github.com/tyemirov/gix/internal/workflow"
)

func TestApplyVariableOverridesIgnoresNonHistoryActions(testInstance *testing.T) {
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

	require.NoError(testInstance, applyVariableOverrides(configuration, variables))

	options := configuration.Steps[0].Options["tasks"].([]any)[0].(map[string]any)["actions"].([]any)[0].(map[string]any)["options"].(map[string]any)
	pushValue, isString := options["push"].(string)
	require.True(testInstance, isString)
	require.Equal(testInstance, "{{ .Environment.namespace_push }}", pushValue)
}

func TestApplyVariableOverridesUpdatesHistoryActionsWhenProvided(testInstance *testing.T) {
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

	require.NoError(testInstance, applyVariableOverrides(configuration, variables))

	options := configuration.Steps[0].Options["tasks"].([]any)[0].(map[string]any)["actions"].([]any)[0].(map[string]any)["options"].(map[string]any)
	pushValue, isBool := options["push"].(bool)
	require.True(testInstance, isBool)
	require.False(testInstance, pushValue)
	require.Equal(testInstance, []string{"public.txt"}, options["paths"])
}

func TestApplyVariableOverridesAppliesEmbeddedLicenseTemplate(testInstance *testing.T) {
	configuration := &workflow.Configuration{
		Steps: []workflow.StepConfiguration{
			{
				Command: []string{"tasks", "apply"},
				Options: map[string]any{
					"tasks": []any{
						map[string]any{
							"name": licenseTaskNameConstant,
							"files": []any{
								map[string]any{
									"path":        "{{ if .Environment.license_target }}{{ .Environment.license_target }}{{ else }}LICENSE{{ end }}",
									"content":     "{{ .Environment.license_content }}",
									"mode":        "{{ if .Environment.license_mode }}{{ .Environment.license_mode }}{{ else }}overwrite{{ end }}",
									"permissions": 420,
								},
							},
						},
					},
				},
			},
		},
	}

	variables := map[string]string{
		licenses.VariableTemplateAlias: licenses.TemplateNameBSL,
	}

	require.NoError(testInstance, applyVariableOverrides(configuration, variables))

	files := configuration.Steps[0].Options["tasks"].([]any)[0].(map[string]any)["files"].([]any)
	require.Len(testInstance, files, 2)

	primaryFile := files[0].(map[string]any)
	primaryContent, _ := primaryFile["content"].(string)
	require.True(testInstance, strings.Contains(primaryContent, licenses.VariableChangeDate))

	commercialFound := false
	for _, fileValue := range files {
		fileEntry := fileValue.(map[string]any)
		pathValue, _ := fileEntry["path"].(string)
		if strings.Contains(pathValue, licenses.OutputCommercialFileName) {
			commercialFound = true
		}
	}
	require.True(testInstance, commercialFound)
}
