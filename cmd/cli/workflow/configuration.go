package workflow

import (
	"github.com/tyemirov/gix/v5/internal/llmclient"
	pathutils "github.com/tyemirov/gix/v5/internal/utils/path"
	workflowpkg "github.com/tyemirov/gix/v5/internal/workflow"
)

var workflowConfigurationRepositoryPathSanitizer = pathutils.NewRepositoryPathSanitizerWithConfiguration(nil, pathutils.RepositoryPathSanitizerConfiguration{PruneNestedPaths: true})

// CommandConfiguration captures configuration values for workflow.
type CommandConfiguration struct {
	Roots              []string                     `mapstructure:"roots"`
	AssumeYes          bool                         `mapstructure:"assume_yes"`
	RequireClean       bool                         `mapstructure:"require_clean"`
	WorkflowWorkers    int                          `mapstructure:"workflow_workers"`
	Variables          map[string]string            `mapstructure:"variables"`
	ConnectionProfiles llmclient.ConnectionProfiles `mapstructure:"-"`
	ConfiguredWorkflow *workflowpkg.Configuration   `mapstructure:"-"`
}

// DefaultCommandConfiguration provides default workflow command settings for workflow.
func DefaultCommandConfiguration() CommandConfiguration {
	return CommandConfiguration{
		AssumeYes:       false,
		RequireClean:    false,
		WorkflowWorkers: 1,
	}
}

// Sanitize normalizes configuration values.
func (configuration CommandConfiguration) Sanitize() CommandConfiguration {
	sanitized := configuration
	sanitized.Roots = workflowConfigurationRepositoryPathSanitizer.Sanitize(configuration.Roots)
	if sanitized.WorkflowWorkers < 1 {
		sanitized.WorkflowWorkers = 1
	}
	if configuration.ConfiguredWorkflow != nil {
		clonedWorkflow := cloneConfiguredWorkflow(*configuration.ConfiguredWorkflow)
		sanitized.ConfiguredWorkflow = &clonedWorkflow
	}
	return sanitized
}

func cloneConfiguredWorkflow(configuration workflowpkg.Configuration) workflowpkg.Configuration {
	clonedConfiguration := workflowpkg.Configuration{
		Steps: make([]workflowpkg.StepConfiguration, len(configuration.Steps)),
	}
	for stepIndex, step := range configuration.Steps {
		clonedConfiguration.Steps[stepIndex] = workflowpkg.StepConfiguration{
			Name:    step.Name,
			After:   append([]string(nil), step.After...),
			Command: append([]string(nil), step.Command...),
			Options: cloneWorkflowOptions(step.Options),
		}
	}
	return clonedConfiguration
}

func cloneWorkflowOptions(options map[string]any) map[string]any {
	if options == nil {
		return nil
	}
	clonedOptions := make(map[string]any, len(options))
	for optionName, optionValue := range options {
		clonedOptions[optionName] = cloneWorkflowOptionValue(optionValue)
	}
	return clonedOptions
}

func cloneWorkflowOptionValue(optionValue any) any {
	switch typedValue := optionValue.(type) {
	case map[string]any:
		return cloneWorkflowOptions(typedValue)
	case []any:
		clonedValues := make([]any, len(typedValue))
		for valueIndex, value := range typedValue {
			clonedValues[valueIndex] = cloneWorkflowOptionValue(value)
		}
		return clonedValues
	case []string:
		return append([]string(nil), typedValue...)
	case map[string]string:
		clonedValues := make(map[string]string, len(typedValue))
		for key, value := range typedValue {
			clonedValues[key] = value
		}
		return clonedValues
	default:
		return optionValue
	}
}
