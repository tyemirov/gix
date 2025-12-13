package workflow

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/tyemirov/gix/internal/repos/shared"
)

const defaultWorkflowParallelism = 1

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
	repositoryParallelism int,
) stageExecutionResult {
	if state == nil || len(state.Repositories) == 0 {
		return stageExecutionResult{
			stageOutcomes:     make([]StageOutcome, 0),
			operationOutcomes: make(map[string]OperationOutcome),
		}
	}

	workItems := buildRepositoryWorkItems(state.Repositories)
	if len(workItems) == 0 {
		return stageExecutionResult{
			stageOutcomes:     make([]StageOutcome, 0),
			operationOutcomes: make(map[string]OperationOutcome),
		}
	}

	result := stageExecutionResult{
		stageOutcomes:     make([]StageOutcome, 0),
		operationOutcomes: make(map[string]OperationOutcome),
	}

	originalState := environment.State
	defer func() {
		environment.State = originalState
	}()

	repositoryParallelism = sanitizeRepositoryParallelism(repositoryParallelism, len(workItems))
	stageCounter := 0
	repositoryStages := make([]OperationStage, 0)
	repositoryStageIndices := make([]int, 0)

	for stageIndex := range stages {
		stage := stages[stageIndex]
		if len(stage.Operations) == 0 {
			continue
		}

		if stageIsRepositoryScoped(stage) {
			repositoryStages = append(repositoryStages, stage)
			repositoryStageIndices = append(repositoryStageIndices, stageIndex)
			continue
		}

		if len(repositoryStages) > 0 {
			repositoryResults := runRepositoryPipelines(
				ctx,
				repositoryStages,
				repositoryStageIndices,
				environment,
				workItems,
				reporter,
				repositoryParallelism,
			)
			for _, pipelineResult := range repositoryResults {
				for _, outcome := range pipelineResult.stageOutcomes {
					if outcome == nil {
						continue
					}
					outcome.Index = stageCounter
					result.stageOutcomes = append(result.stageOutcomes, *outcome)
					stageCounter++
				}
				for key, operationOutcome := range pipelineResult.operationOutcomes {
					result.operationOutcomes[key] = operationOutcome
				}
				result.failures = append(result.failures, pipelineResult.failures...)
			}
			repositoryStages = repositoryStages[:0]
			repositoryStageIndices = repositoryStageIndices[:0]
		}

		environment.State = state
		outcome, operationResults, stageFailures := executeGlobalStage(
			ctx,
			stage,
			environment,
			state,
			reporter,
			stageIndex,
		)
		if outcome != nil {
			outcome.Index = stageCounter
			result.stageOutcomes = append(result.stageOutcomes, *outcome)
			stageCounter++
		}
		for key, opOutcome := range operationResults {
			result.operationOutcomes[key] = opOutcome
		}
		result.failures = append(result.failures, stageFailures...)
	}

	if len(repositoryStages) > 0 {
		repositoryResults := runRepositoryPipelines(
			ctx,
			repositoryStages,
			repositoryStageIndices,
			environment,
			workItems,
			reporter,
			repositoryParallelism,
		)
		for _, pipelineResult := range repositoryResults {
			for _, outcome := range pipelineResult.stageOutcomes {
				if outcome == nil {
					continue
				}
				outcome.Index = stageCounter
				result.stageOutcomes = append(result.stageOutcomes, *outcome)
				stageCounter++
			}
			for key, operationOutcome := range pipelineResult.operationOutcomes {
				result.operationOutcomes[key] = operationOutcome
			}
			result.failures = append(result.failures, pipelineResult.failures...)
		}
	}

	return result
}

type repositoryPipelineResult struct {
	stageOutcomes     []*StageOutcome
	operationOutcomes map[string]OperationOutcome
	failures          []recordedOperationFailure
}

func runRepositoryPipelines(
	ctx context.Context,
	stages []OperationStage,
	stageIndices []int,
	environment *Environment,
	workItems []repositoryWorkItem,
	reporter shared.SummaryReporter,
	repositoryParallelism int,
) []repositoryPipelineResult {
	if len(stages) == 0 || len(workItems) == 0 {
		return nil
	}

	results := make([]repositoryPipelineResult, len(workItems))
	workerCount := sanitizeRepositoryParallelism(repositoryParallelism, len(workItems))
	workChannel := make(chan repositoryWorkAssignment)
	var wg sync.WaitGroup

	for workerIndex := 0; workerIndex < workerCount; workerIndex++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for assignment := range workChannel {
				repoEnvironment := cloneEnvironmentForRepository(environment, assignment.item.state)
				repoResult := repositoryPipelineResult{
					stageOutcomes:     make([]*StageOutcome, 0, len(stages)),
					operationOutcomes: make(map[string]OperationOutcome),
					failures:          make([]recordedOperationFailure, 0),
				}
				repositoryFailed := false

				for stagePosition := range stages {
					if repositoryFailed {
						break
					}

					stage := stages[stagePosition]
					stageOutcome, operationResults, stageFailures, stageFailed := executeRepositoryStageForRepository(
						ctx,
						stage,
						assignment.item.repository,
						assignment.item.label,
						assignment.item.state,
						repoEnvironment,
						reporter,
						stageIndices[stagePosition],
					)

					if stageOutcome != nil {
						repoResult.stageOutcomes = append(repoResult.stageOutcomes, stageOutcome)
					}
					for key, operationOutcome := range operationResults {
						repoResult.operationOutcomes[key] = operationOutcome
					}
					repoResult.failures = append(repoResult.failures, stageFailures...)

					if stageFailed {
						repositoryFailed = true
					}
				}

				results[assignment.index] = repoResult
			}
		}()
	}

	for repoIndex := range workItems {
		workChannel <- repositoryWorkAssignment{
			index: repoIndex,
			item:  workItems[repoIndex],
		}
	}
	close(workChannel)
	wg.Wait()

	return results
}

type repositoryWorkAssignment struct {
	index int
	item  repositoryWorkItem
}

type repositoryWorkItem struct {
	repository *RepositoryState
	state      *State
	label      string
}

func buildRepositoryWorkItems(repositories []*RepositoryState) []repositoryWorkItem {
	seen := make(map[string]struct{}, len(repositories))
	workItems := make([]repositoryWorkItem, 0, len(repositories))
	for _, repository := range repositories {
		if repository == nil {
			continue
		}
		repositoryPath := strings.TrimSpace(repository.Path)
		if repositoryPath != "" {
			if _, exists := seen[repositoryPath]; exists {
				continue
			}
			seen[repositoryPath] = struct{}{}
		}
		workItems = append(workItems, repositoryWorkItem{
			repository: repository,
			state:      &State{Repositories: []*RepositoryState{repository}},
			label:      repositoryLabel(repository),
		})
	}
	return workItems
}

func cloneEnvironmentForRepository(base *Environment, repoState *State) *Environment {
	if base == nil {
		return nil
	}
	clone := *base
	clone.State = repoState
	if base.Variables != nil {
		clone.Variables = base.Variables.Clone()
	}
	return &clone
}

func sanitizeRepositoryParallelism(requested int, repositoryCount int) int {
	if repositoryCount <= 1 {
		return 1
	}
	parallelism := requested
	if parallelism <= 0 {
		parallelism = defaultWorkflowParallelism
	}
	if parallelism <= 0 {
		parallelism = runtime.NumCPU()
	}
	if parallelism <= 0 {
		parallelism = 1
	}
	if parallelism > repositoryCount {
		parallelism = repositoryCount
	}
	return parallelism
}

func executeRepositoryStageForRepository(
	ctx context.Context,
	stage OperationStage,
	repository *RepositoryState,
	repoLabel string,
	repoState *State,
	environment *Environment,
	reporter shared.SummaryReporter,
	stageIndex int,
) (*StageOutcome, map[string]OperationOutcome, []recordedOperationFailure, bool) {
	if repository == nil || repoState == nil {
		return nil, nil, nil, false
	}

	stageStart := time.Now()
	stageOperationNames := make([]string, 0, len(stage.Operations))
	var failures []recordedOperationFailure
	repositoryFailed := false
	operationOutcomes := make(map[string]OperationOutcome)
	previousStepName := environment.currentStepName

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

		environment.currentStepName = operationName
		environment.beginStep(repository.Path, operationName)

		compositeName := fmt.Sprintf("%s:%s", repoLabel, operationName)
		stageOperationNames = append(stageOperationNames, compositeName)

		startTime := time.Now()
		executeError := repoOperation.ExecuteForRepository(ctx, environment, repository)
		skipRepository := errors.Is(executeError, errRepositorySkipped)
		environment.reportStepSummary(repository, operationName, executeError, skipRepository)
		if skipRepository {
			repositoryFailed = true
		}
		executionDuration := time.Since(startTime)

		if reporter != nil {
			reporter.RecordOperationDuration(compositeName, executionDuration)
			if executeError == nil || skipRepository {
				reporter.RecordEvent(shared.EventCodeWorkflowOperationSuccess, shared.EventLevelInfo)
			} else {
				reporter.RecordEvent(shared.EventCodeWorkflowOperationFailure, shared.EventLevelError)
			}
		}

		if skipRepository {
			executeError = nil
		}

		operationOutcomes[fmt.Sprintf("%s@%s", node.Name, repoLabel)] = OperationOutcome{
			Name:     compositeName,
			Duration: executionDuration,
			Failed:   executeError != nil,
			Error:    executeError,
		}

		if executeError == nil {
			if skipRepository {
				break
			}
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
		repositoryFailed = true
	}

	environment.currentStepName = previousStepName

	if len(stageOperationNames) == 0 {
		return nil, operationOutcomes, failures, repositoryFailed
	}

	stageDuration := time.Since(stageStart)
	if reporter != nil {
		reporter.RecordStageDuration(fmt.Sprintf("%s-stage-%d", repoLabel, stageIndex+1), stageDuration)
	}

	return &StageOutcome{
		Duration:   stageDuration,
		Operations: stageOperationNames,
	}, operationOutcomes, failures, repositoryFailed
}

func executeGlobalStage(
	ctx context.Context,
	stage OperationStage,
	environment *Environment,
	state *State,
	reporter shared.SummaryReporter,
	stageIndex int,
) (*StageOutcome, map[string]OperationOutcome, []recordedOperationFailure) {
	environment.State = state

	stageStart := time.Now()
	stageOperationNames := make([]string, 0, len(stage.Operations))
	var failures []recordedOperationFailure
	operationOutcomes := make(map[string]OperationOutcome)

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

		operationOutcomes[node.Name] = OperationOutcome{
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

	return &StageOutcome{
		Duration:   stageDuration,
		Operations: stageOperationNames,
	}, operationOutcomes, failures
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
