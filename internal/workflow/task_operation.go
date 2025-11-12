package workflow

import (
	"context"
	"errors"
	"strings"

	"github.com/temirov/gix/internal/repos/shared"
)

// TaskOperation executes declarative repository tasks (file mutations, commits, and PRs).
type TaskOperation struct {
	tasks            []TaskDefinition
	llmConfiguration *TaskLLMClientConfiguration
}

// Definitions returns a copy of the task definitions associated with the operation.
func (operation *TaskOperation) Definitions() []TaskDefinition {
	if operation == nil || len(operation.tasks) == 0 {
		return nil
	}
	definitions := make([]TaskDefinition, len(operation.tasks))
	copy(definitions, operation.tasks)
	return definitions
}

func (operation *TaskOperation) attachLLMConfiguration() {
	if operation == nil || operation.llmConfiguration == nil {
		return
	}

	for taskIndex := range operation.tasks {
		actionDefinitions := operation.tasks[taskIndex].Actions
		for actionIndex := range actionDefinitions {
			action := &actionDefinitions[actionIndex]
			normalizedType := strings.ToLower(strings.TrimSpace(action.Type))
			switch normalizedType {
			case taskActionCommitMessage:
				if _, exists := action.Options[commitOptionClient]; !exists {
					action.Options[commitOptionClient] = operation.llmConfiguration
				}
			case taskActionChangelog:
				if _, exists := action.Options[changelogOptionClient]; !exists {
					action.Options[changelogOptionClient] = operation.llmConfiguration
				}
			}
		}
	}
}

// Name identifies the workflow command handled by this operation.
func (operation *TaskOperation) Name() string {
	return commandTasksApplyKey
}

// Execute runs the configured tasks across repositories.
func (operation *TaskOperation) Execute(executionContext context.Context, environment *Environment, state *State) error {
	if operation == nil || environment == nil || state == nil {
		return nil
	}

	seen := make(map[string]struct{}, len(state.Repositories))

	var executionErrors []error

	for _, repository := range state.Repositories {
		if repository == nil {
			continue
		}
		pathKey := strings.TrimSpace(repository.Path)
		if len(pathKey) > 0 {
			if _, exists := seen[pathKey]; exists {
				continue
			}
			seen[pathKey] = struct{}{}
		}
		for _, task := range operation.tasks {
			if err := operation.executeTask(executionContext, environment, repository, task); err != nil {
				executionErrors = append(executionErrors, err)
			}
		}
	}

	if len(executionErrors) > 0 {
		return errors.Join(executionErrors...)
	}
	return nil
}

func (operation *TaskOperation) executeTask(executionContext context.Context, environment *Environment, repository *RepositoryState, task TaskDefinition) error {
	if environment == nil || repository == nil {
		return nil
	}

	if len(task.Safeguards) > 0 {
		pass, reason, evalErr := EvaluateSafeguards(executionContext, environment, repository, task.Safeguards)
		if evalErr != nil {
			return evalErr
		}
		if !pass {
			environment.ReportRepositoryEvent(repository, shared.EventLevelWarn, shared.EventCodeTaskSkip, reason, nil)
			return nil
		}
	}

	var variableSnapshot map[string]string
	if environment.Variables != nil {
		variableSnapshot = environment.Variables.Snapshot()
	}

	templateData := buildTaskTemplateData(repository, task, variableSnapshot)

	planner := newTaskPlanner(task, templateData)
	plan, planError := planner.BuildPlan(environment, repository)
	if planError != nil {
		return planError
	}
	plan.variables = variableSnapshot

	executor := newTaskExecutor(environment, repository, plan)
	return executor.Execute(executionContext)
}
