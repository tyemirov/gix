package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	result := stageExecutionResult{
		stageOutcomes:     make([]StageOutcome, 0),
		operationOutcomes: make(map[string]OperationOutcome),
	}

	if state == nil || len(state.Repositories) == 0 {
		return result
	}

	originalState := environment.State
	defer func() {
		environment.State = originalState
	}()

	var failures []recordedOperationFailure
	stageCounter := 0
	globalStageExecuted := make([]bool, len(stages))
	seenRepositories := make(map[string]struct{}, len(state.Repositories))
	repositoryOrderIndex := 0

	for _, repository := range state.Repositories {
		if repository == nil {
			continue
		}

		repositoryPath := strings.TrimSpace(repository.Path)
		if repositoryPath != "" {
			if _, exists := seenRepositories[repositoryPath]; exists {
				continue
			}
			seenRepositories[repositoryPath] = struct{}{}
		}

		repoLabel := repositoryLabel(repository)
		repoState := &State{Repositories: []*RepositoryState{repository}}
		environment.State = repoState

		for stageIndex := range stages {
			stage := stages[stageIndex]
			if len(stage.Operations) == 0 {
				continue
			}

			if !stageIsRepositoryScoped(stage) {
				if repositoryOrderIndex == 0 && !globalStageExecuted[stageIndex] {
					environment.State = state
					outcome, stageFailures := executeGlobalStage(
						ctx,
						stage,
						environment,
						state,
						reporter,
						logger,
						stageIndex,
						&result,
						stageCounter,
					)
					if outcome != nil {
						result.stageOutcomes = append(result.stageOutcomes, *outcome)
						stageCounter++
					}
					failures = append(failures, stageFailures...)
					globalStageExecuted[stageIndex] = true
					environment.State = repoState
				}
				continue
			}

			stageOutcome, stageFailures, stageFailed := executeRepositoryStageForRepository(
				ctx,
				stage,
				repository,
				repoLabel,
				repoState,
				environment,
				reporter,
				logger,
				stageIndex,
				stageCounter,
				&result,
			)

			if stageOutcome != nil {
				result.stageOutcomes = append(result.stageOutcomes, *stageOutcome)
				stageCounter++
			}
			failures = append(failures, stageFailures...)
			if stageFailed {
				// stop executing further stages for this repository
				break
			}
		}

		repositoryOrderIndex++
	}

	result.failures = failures
	return result
}

func executeRepositoryStageForRepository(
	ctx context.Context,
	stage OperationStage,
	repository *RepositoryState,
	repoLabel string,
	repoState *State,
	environment *Environment,
	reporter shared.SummaryReporter,
	logger *zap.Logger,
	stageIndex int,
	stageCounter int,
	result *stageExecutionResult,
) (*StageOutcome, []recordedOperationFailure, bool) {
	if repository == nil || repoState == nil {
		return nil, nil, false
	}

	stageStart := time.Now()
	stageOperationNames := make([]string, 0, len(stage.Operations))
	var failures []recordedOperationFailure
	repositoryFailed := false

	for _, node := range stage.Operations {
		if node == nil || node.Operation == nil {
			continue
		}

		repoOperation, ok := node.Operation.(RepositoryScopedOperation)
		if !ok || !repoOperation.IsRepositoryScoped() {
			continue
		}

		environment.State = repoState

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
		executeError := repoOperation.ExecuteForRepository(ctx, environment, repository)
		executionDuration := time.Since(startTime)

		if reporter != nil {
			reporter.RecordOperationDuration(compositeName, executionDuration)
			if executeError == nil {
				reporter.RecordEvent(shared.EventCodeWorkflowOperationSuccess, shared.EventLevelInfo)
			} else {
				reporter.RecordEvent(shared.EventCodeWorkflowOperationFailure, shared.EventLevelError)
			}
		}

		result.operationOutcomes[fmt.Sprintf("%s@%s", node.Name, repoLabel)] = OperationOutcome{
			Name:     compositeName,
			Duration: executionDuration,
			Failed:   executeError != nil,
			Error:    executeError,
		}

		if executeError == nil {
			continue
		}

		if errors.Is(executeError, errRepositorySkipped) {
			repositoryFailed = true
			break
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
		repositoryFailed = true
	}

	if len(stageOperationNames) == 0 {
		return nil, failures, repositoryFailed
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

	return &StageOutcome{
		Index:      stageCounter,
		Duration:   stageDuration,
		Operations: stageOperationNames,
	}, failures, repositoryFailed
}

func executeGlobalStage(
	ctx context.Context,
	stage OperationStage,
	environment *Environment,
	state *State,
	reporter shared.SummaryReporter,
	logger *zap.Logger,
	stageIndex int,
	result *stageExecutionResult,
	stageCounter int,
) (*StageOutcome, []recordedOperationFailure) {
	environment.State = state

	stageStart := time.Now()
	stageOperationNames := make([]string, 0, len(stage.Operations))
	var failures []recordedOperationFailure

	for _, node := range stage.Operations {
		if node == nil || node.Operation == nil {
			continue
		}

		operationName := strings.TrimSpace(node.Name)
		if len(operationName) == 0 {
			operationName = strings.TrimSpace(node.Operation.Name())
		}
		if len(operationName) == 0 {
			operationName = "operation"
		}
		stageOperationNames = append(stageOperationNames, operationName)

		startTime := time.Now()
		executeError := node.Operation.Execute(ctx, environment, state)
		executionDuration := time.Since(startTime)

		if reporter != nil {
			reporter.RecordOperationDuration(operationName, executionDuration)
			if executeError == nil {
				reporter.RecordEvent(shared.EventCodeWorkflowOperationSuccess, shared.EventLevelInfo)
			} else {
				reporter.RecordEvent(shared.EventCodeWorkflowOperationFailure, shared.EventLevelError)
			}
		}

		result.operationOutcomes[node.Name] = OperationOutcome{
			Name:     operationName,
			Duration: executionDuration,
			Failed:   executeError != nil,
			Error:    executeError,
		}

		if executeError == nil {
			continue
		}

		if errors.Is(executeError, errRepositorySkipped) {
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
		reporter.RecordStageDuration(fmt.Sprintf("stage-%d", stageIndex+1), stageDuration)
	}
	if logger != nil {
		logger.Info(
			"workflow_stage_complete",
			zap.Int("stage_index", stageCounter),
			zap.Duration("duration", stageDuration),
		)
	}

	return &StageOutcome{
		Index:      stageCounter,
		Duration:   stageDuration,
		Operations: stageOperationNames,
	}, failures
}

func stageIsRepositoryScoped(stage OperationStage) bool {
	if len(stage.Operations) == 0 {
		return false
	}
	for _, node := range stage.Operations {
		if node == nil || node.Operation == nil {
			continue
		}
		repoOperation, ok := node.Operation.(RepositoryScopedOperation)
		if !ok || !repoOperation.IsRepositoryScoped() {
			return false
		}
	}
	return true
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
