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
	repositoryScoped bool
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

// IsRepositoryScoped reports whether the operation should execute per repository.
func (operation *TaskOperation) IsRepositoryScoped() bool {
	if operation == nil {
		return false
	}
	return operation.repositoryScoped
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
		if err := operation.ExecuteForRepository(executionContext, environment, repository); err != nil {
			executionErrors = append(executionErrors, err)
		}
	}

	if len(executionErrors) > 0 {
		return errors.Join(executionErrors...)
	}
	return nil
}

// ExecuteForRepository runs the configured tasks against a single repository.
func (operation *TaskOperation) ExecuteForRepository(executionContext context.Context, environment *Environment, repository *RepositoryState) error {
	if operation == nil || environment == nil || repository == nil {
		return nil
	}

	var executionErrors []error
	for _, task := range operation.tasks {
		if err := operation.executeTask(executionContext, environment, repository, task); err != nil {
			if errors.Is(err, errRepositorySkipped) {
				return err
			}
			executionErrors = append(executionErrors, err)
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

	hardSafeguards, softSafeguards := splitSafeguardSets(task.Safeguards, safeguardDefaultHardStop)
	if len(hardSafeguards) > 0 {
		pass, reason, evalErr := EvaluateSafeguards(executionContext, environment, repository, hardSafeguards)
		if evalErr != nil {
			return evalErr
		}
		if !pass {
			environment.ReportRepositoryEvent(repository, shared.EventLevelWarn, shared.EventCodeTaskSkip, reason, nil)
			return repositorySkipError{reason: reason}
		}
	}

	if len(softSafeguards) > 0 {
		pass, reason, evalErr := EvaluateSafeguards(executionContext, environment, repository, softSafeguards)
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

func isRepositoryScopedTaskOperation(tasks []TaskDefinition) bool {
	for _, task := range tasks {
		if taskDefinitionIsRepositoryScoped(task) {
			return true
		}
	}
	return false
}

func taskDefinitionIsRepositoryScoped(task TaskDefinition) bool {
	if len(task.Files) > 0 {
		return true
	}
	if len(strings.TrimSpace(task.Branch.NameTemplate)) > 0 || len(strings.TrimSpace(task.Branch.StartPointTemplate)) > 0 {
		return true
	}
	if len(strings.TrimSpace(task.Commit.MessageTemplate)) > 0 {
		return true
	}
	if task.PullRequest != nil {
		return true
	}
	for _, action := range task.Actions {
		if !isGlobalTaskAction(action.Type) {
			return true
		}
	}
	return false
}

func isGlobalTaskAction(actionType string) bool {
	switch strings.ToLower(strings.TrimSpace(actionType)) {
	case taskActionAuditReport:
		return true
	default:
		return false
	}
}
