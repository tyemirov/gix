package workflow

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	configurationLoadErrorTemplateConstant       = "failed to load workflow configuration: %w"
	configurationParseErrorTemplateConstant      = "failed to parse workflow configuration: %w"
	configurationPathRequiredMessageConstant     = "workflow configuration path must be provided"
	configurationEmptyStepsMessageConstant       = "workflow configuration must define at least one step"
	configurationCommandMissingMessageConstant   = "workflow step missing command path"
	configurationWorkflowSequenceMessageConstant = "workflow block must be defined as a sequence of steps"
)

// Configuration describes the ordered workflow steps loaded from YAML or JSON.
type Configuration struct {
	Steps []StepConfiguration
}

type workflowFile struct {
	Workflow []workflowStepWrapper `yaml:"workflow" json:"workflow"`
}

type workflowStepWrapper struct {
	Step StepConfiguration `yaml:"step" json:"step"`
}

// StepConfiguration associates a command path with declarative options.
type StepConfiguration struct {
	Name    string         `yaml:"name" json:"name"`
	After   []string       `yaml:"after" json:"after"`
	Command []string       `yaml:"command" json:"command"`
	Options map[string]any `yaml:"with" json:"with"`
}

// LoadConfiguration reads the workflow definition from disk and performs basic validation.
func LoadConfiguration(filePath string) (Configuration, error) {
	trimmedPath := strings.TrimSpace(filePath)
	if len(trimmedPath) == 0 {
		return Configuration{}, errors.New(configurationPathRequiredMessageConstant)
	}

	contentBytes, readError := os.ReadFile(trimmedPath)
	if readError != nil {
		return Configuration{}, fmt.Errorf(configurationLoadErrorTemplateConstant, readError)
	}

	return parseConfiguration(contentBytes)
}

// ParseConfiguration loads the workflow definition from raw bytes and validates its structure.
func ParseConfiguration(contentBytes []byte) (Configuration, error) {
	return parseConfiguration(contentBytes)
}

func parseConfiguration(contentBytes []byte) (Configuration, error) {
	var parsedWorkflow workflowFile
	if unmarshalError := yaml.Unmarshal(contentBytes, &parsedWorkflow); unmarshalError != nil {
		return Configuration{}, fmt.Errorf(configurationParseErrorTemplateConstant, unmarshalError)
	}

	if workflowError := ensureWorkflowSequence(contentBytes); workflowError != nil {
		return Configuration{}, fmt.Errorf(configurationParseErrorTemplateConstant, workflowError)
	}

	configuration := Configuration{Steps: make([]StepConfiguration, 0, len(parsedWorkflow.Workflow))}
	for index := range parsedWorkflow.Workflow {
		configuration.Steps = append(configuration.Steps, parsedWorkflow.Workflow[index].Step)
	}

	if len(configuration.Steps) == 0 {
		return Configuration{}, errors.New(configurationEmptyStepsMessageConstant)
	}

	for stepIndex := range configuration.Steps {
		configuration.Steps[stepIndex].Name = strings.TrimSpace(configuration.Steps[stepIndex].Name)
		configuration.Steps[stepIndex].Command = normalizeCommandParts(configuration.Steps[stepIndex].Command)
		if len(configuration.Steps[stepIndex].Command) == 0 {
			return Configuration{}, errors.New(configurationCommandMissingMessageConstant)
		}
	}

	return configuration, nil
}

func ensureWorkflowSequence(contentBytes []byte) error {
	var workflowWrapper struct {
		Workflow yaml.Node `yaml:"workflow" json:"workflow"`
	}

	if unmarshalError := yaml.Unmarshal(contentBytes, &workflowWrapper); unmarshalError != nil {
		return unmarshalError
	}

	if workflowWrapper.Workflow.Kind == 0 {
		return nil
	}

	switch workflowWrapper.Workflow.Kind {
	case yaml.SequenceNode:
		return nil
	default:
		return errors.New(configurationWorkflowSequenceMessageConstant)
	}
}
