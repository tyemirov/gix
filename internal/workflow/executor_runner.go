package workflow

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/temirov/gix/internal/repos/shared"
	"go.uber.org/zap"
)

type stageExecutionResult struct {
	stageOutcomes     []StageOutcome
	operationOutcomes map[string]OperationOutcome
	failures          []recordedOperationFailure
}

func runOperationStages(
	ctx context.Context,
	stages []OperationStage,
	environment *Environment,
	state *State,
	reporter shared.SummaryReporter,
	logger *zap.Logger,
) stageExecutionResult {
	if stagesAreRepositoryScoped(stages) {
		return runRepositoryScopedStages(ctx, stages, environment, state, reporter, logger)
	}

	result := stageExecutionResult{
		stageOutcomes:     make([]StageOutcome, 0, len(stages)),
		operationOutcomes: make(map[string]OperationOutcome),
	}

	var failureMu sync.Mutex
	var failures []recordedOperationFailure

	var resultMu sync.Mutex

	for stageIndex := range stages {
		stage := stages[stageIndex]
		if len(stage.Operations) == 0 {
			continue
		}

		stageStart := time.Now()
		var waitGroup sync.WaitGroup

		for _, node := range stage.Operations {
			if node == nil || node.Operation == nil {
				continue
			}

			waitGroup.Add(1)
			go func(operationNode *OperationNode) {
				defer waitGroup.Done()

				operation := operationNode.Operation
				operationName := strings.TrimSpace(operationNode.Name)
				if len(operationName) == 0 {
					operationName = strings.TrimSpace(operation.Name())
				}
				if len(operationName) == 0 {
					operationName = "operation"
				}

				startTime := time.Now()
				executeError := operation.Execute(ctx, environment, state)
				executionDuration := time.Since(startTime)
				if reporter != nil {
					reporter.RecordOperationDuration(operationName, executionDuration)
				}

				outcome := OperationOutcome{
					Name:     operationName,
					Duration: executionDuration,
					Failed:   executeError != nil,
					Error:    executeError,
				}

				resultMu.Lock()
				result.operationOutcomes[operationNode.Name] = outcome
				resultMu.Unlock()

				if executeError == nil {
					if reporter != nil {
						reporter.RecordEvent(shared.EventCodeWorkflowOperationSuccess, shared.EventLevelInfo)
					}
					return
				}

				if reporter != nil {
					reporter.RecordEvent(shared.EventCodeWorkflowOperationFailure, shared.EventLevelError)
				}

				subErrors := collectOperationErrors(executeError)
				if len(subErrors) == 0 {
					subErrors = []error{executeError}
				}

				failureMu.Lock()
				for _, failureErr := range subErrors {
					formatted := formatOperationFailure(environment, failureErr, operation.Name())
					if !logRepositoryOperationError(environment, failureErr) {
						if environment != nil && environment.Errors != nil {
							fmt.Fprintln(environment.Errors, formatted)
						}
					}
					failures = append(failures, recordedOperationFailure{
						name:    operation.Name(),
						err:     failureErr,
						message: formatted,
					})
				}
				failureMu.Unlock()
			}(node)
		}

		waitGroup.Wait()

		stageDuration := time.Since(stageStart)
		stageOutcome := StageOutcome{
			Index:    stageIndex,
			Duration: stageDuration,
			Operations: func() []string {
				names := make([]string, 0, len(stage.Operations))
				for _, node := range stage.Operations {
					if node == nil {
						continue
					}
					names = append(names, node.Name)
				}
				return names
			}(),
		}

		if reporter != nil {
			reporter.RecordStageDuration(fmt.Sprintf("stage-%d", stageIndex+1), stageDuration)
		}

		if logger != nil {
			logger.Info(
				"workflow_stage_complete",
				zap.Int("stage_index", stageIndex),
				zap.Duration("duration", stageDuration),
			)
		}

		result.stageOutcomes = append(result.stageOutcomes, stageOutcome)
	}

	result.failures = failures
	return result
}

func runRepositoryScopedStages(
	ctx context.Context,
	stages []OperationStage,
	environment *Environment,
	state *State,
	reporter shared.SummaryReporter,
	logger *zap.Logger,
) stageExecutionResult {
	result := stageExecutionResult{
		stageOutcomes:     make([]StageOutcome, 0, len(stages)),
		operationOutcomes: make(map[string]OperationOutcome),
	}

	if state == nil || len(state.Repositories) == 0 {
		return result
	}

	var failures []recordedOperationFailure
	stageCounter := 0

	for _, repository := range state.Repositories {
		if repository == nil {
			continue
		}
		repoLabel := repositoryLabel(repository)

		for stageIndex := range stages {
			stage := stages[stageIndex]
			if len(stage.Operations) == 0 {
				continue
			}

			stageStart := time.Now()
			stageOperationNames := make([]string, 0, len(stage.Operations))

			for _, node := range stage.Operations {
				if node == nil || node.Operation == nil {
					continue
				}

				repositoryOperation, scoped := node.Operation.(RepositoryScopedOperation)
				if !scoped {
					continue
				}

				operationName := strings.TrimSpace(node.Name)
				if len(operationName) == 0 {
					operationName = strings.TrimSpace(node.Operation.Name())
				}
				if len(operationName) == 0 {
					operationName = "operation"
				}
				compositeName := fmt.Sprintf("%s:%s", repoLabel, operationName)
				stageOperationNames = append(stageOperationNames, compositeName)

				startTime := time.Now()
				executeError := repositoryOperation.ExecuteForRepository(ctx, environment, repository)
				executionDuration := time.Since(startTime)

				if reporter != nil {
					reporter.RecordOperationDuration(compositeName, executionDuration)
					if executeError == nil {
						reporter.RecordEvent(shared.EventCodeWorkflowOperationSuccess, shared.EventLevelInfo)
					} else {
						reporter.RecordEvent(shared.EventCodeWorkflowOperationFailure, shared.EventLevelError)
					}
				}

				result.operationOutcomes[compositeName] = OperationOutcome{
					Name:     compositeName,
					Duration: executionDuration,
					Failed:   executeError != nil,
					Error:    executeError,
				}

				if executeError == nil {
					continue
				}

				subErrors := collectOperationErrors(executeError)
				if len(subErrors) == 0 {
					subErrors = []error{executeError}
				}

				for _, failureErr := range subErrors {
					formatted := formatOperationFailure(environment, failureErr, node.Operation.Name())
					if !logRepositoryOperationError(environment, failureErr) {
						if environment != nil && environment.Errors != nil {
							fmt.Fprintln(environment.Errors, formatted)
						}
					}
					failures = append(failures, recordedOperationFailure{
						name:    node.Operation.Name(),
						err:     failureErr,
						message: formatted,
					})
				}
			}

			stageDuration := time.Since(stageStart)
			if reporter != nil {
				reporter.RecordStageDuration(fmt.Sprintf("%s-stage-%d", repoLabel, stageIndex+1), stageDuration)
			}
			if logger != nil {
				logger.Info(
					"workflow_stage_complete",
					zap.Int("stage_index", stageCounter),
					zap.Duration("duration", stageDuration),
					zap.String("repository", repoLabel),
				)
			}

			result.stageOutcomes = append(result.stageOutcomes, StageOutcome{
				Index:      stageCounter,
				Duration:   stageDuration,
				Operations: stageOperationNames,
			})
			stageCounter++
		}
	}

	result.failures = failures
	return result
}

func stagesAreRepositoryScoped(stages []OperationStage) bool {
	for _, stage := range stages {
		for _, node := range stage.Operations {
			if node == nil || node.Operation == nil {
				continue
			}
			if _, scoped := node.Operation.(RepositoryScopedOperation); !scoped {
				return false
			}
		}
	}
	return len(stages) > 0
}

func repositoryLabel(repository *RepositoryState) string {
	if repository == nil {
		return "repository"
	}
	identifier := strings.TrimSpace(repository.Inspection.FinalOwnerRepo)
	if identifier != "" {
		return identifier
	}
	if trimmedPath := strings.TrimSpace(repository.Path); trimmedPath != "" {
		return trimmedPath
	}
	return "repository"
}
