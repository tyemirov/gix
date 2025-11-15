package workflow

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/temirov/gix/internal/repos/shared"
)

const defaultRepositoryParallelism = 4

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

	parallelism := sanitizeRepositoryParallelism(repositoryParallelism, len(workItems))
	if parallelism <= 1 {
		return runOperationStagesSequential(ctx, stages, environment, state, workItems, reporter)
	}
	return runOperationStagesConcurrent(ctx, stages, environment, state, workItems, reporter, parallelism)
}

func runOperationStagesSequential(
	ctx context.Context,
	stages []OperationStage,
	environment *Environment,
	state *State,
	workItems []repositoryWorkItem,
	reporter shared.SummaryReporter,
) stageExecutionResult {
	result := stageExecutionResult{
		stageOutcomes:     make([]StageOutcome, 0),
		operationOutcomes: make(map[string]OperationOutcome),
	}

	originalState := environment.State
	defer func() {
		environment.State = originalState
	}()

	var failures []recordedOperationFailure
	stageCounter := 0
	globalStageExecuted := make([]bool, len(stages))

	for repositoryOrderIndex, item := range workItems {
		environment.State = item.state

		for stageIndex := range stages {
			stage := stages[stageIndex]
			if len(stage.Operations) == 0 {
				continue
			}

			if !stageIsRepositoryScoped(stage) {
				if repositoryOrderIndex == 0 && !globalStageExecuted[stageIndex] {
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
					failures = append(failures, stageFailures...)
					globalStageExecuted[stageIndex] = true
					environment.State = item.state
				}
				continue
			}

			stageOutcome, operationResults, stageFailures, stageFailed := executeRepositoryStageForRepository(
				ctx,
				stage,
				item.repository,
				item.label,
				item.state,
				environment,
				reporter,
				stageIndex,
			)

			if stageOutcome != nil {
				stageOutcome.Index = stageCounter
				result.stageOutcomes = append(result.stageOutcomes, *stageOutcome)
				stageCounter++
			}
			for key, outcome := range operationResults {
				result.operationOutcomes[key] = outcome
			}
			failures = append(failures, stageFailures...)
			if stageFailed {
				break
			}
		}
	}

	result.failures = failures
	return result
}

func runOperationStagesConcurrent(
	ctx context.Context,
	stages []OperationStage,
	environment *Environment,
	state *State,
	workItems []repositoryWorkItem,
	reporter shared.SummaryReporter,
	repositoryParallelism int,
) stageExecutionResult {
	result := stageExecutionResult{
		stageOutcomes:     make([]StageOutcome, 0),
		operationOutcomes: make(map[string]OperationOutcome),
	}

	originalState := environment.State
	defer func() {
		environment.State = originalState
	}()

	repositoryFailed := make([]bool, len(workItems))
	repoStageResults := make([][]repositoryStageResult, len(workItems))
	for index := range repoStageResults {
		repoStageResults[index] = make([]repositoryStageResult, len(stages))
	}
	globalStageResults := make([]repositoryStageResult, len(stages))

	for stageIndex := range stages {
		stage := stages[stageIndex]
		if len(stage.Operations) == 0 {
			continue
		}

		if !stageIsRepositoryScoped(stage) {
			environment.State = state
			outcome, operationResults, stageFailures := executeGlobalStage(
				ctx,
				stage,
				environment,
				state,
				reporter,
				stageIndex,
			)
			globalStageResults[stageIndex] = repositoryStageResult{
				outcome:           outcome,
				operationOutcomes: operationResults,
				failures:          stageFailures,
			}
			continue
		}

		semaphore := make(chan struct{}, repositoryParallelism)
		resultChannel := make(chan parallelStageResult, len(workItems))
		launched := 0

		for repoIndex, item := range workItems {
			if repositoryFailed[repoIndex] {
				continue
			}
			launched++
			semaphore <- struct{}{}
			go func(repoIndex int, item repositoryWorkItem) {
				repoEnvironment := cloneEnvironmentForRepository(environment, item.state)
				stageOutcome, operationResults, stageFailures, stageFailed := executeRepositoryStageForRepository(
					ctx,
					stage,
					item.repository,
					item.label,
					item.state,
					repoEnvironment,
					reporter,
					stageIndex,
				)
				resultChannel <- parallelStageResult{
					repositoryIndex:   repoIndex,
					stageIndex:        stageIndex,
					stageOutcome:      stageOutcome,
					operationOutcomes: operationResults,
					failures:          stageFailures,
					stageFailed:       stageFailed,
				}
				<-semaphore
			}(repoIndex, item)
		}

		for processed := 0; processed < launched; processed++ {
			resultMessage := <-resultChannel
			repoStageResults[resultMessage.repositoryIndex][resultMessage.stageIndex] = repositoryStageResult{
				outcome:           resultMessage.stageOutcome,
				operationOutcomes: resultMessage.operationOutcomes,
				failures:          resultMessage.failures,
			}
			if resultMessage.stageFailed {
				repositoryFailed[resultMessage.repositoryIndex] = true
			}
		}
	}

	stageCounter := 0
	for repoIndex := range workItems {
		for stageIndex := range stages {
			stage := stages[stageIndex]
			if len(stage.Operations) == 0 {
				continue
			}

			if !stageIsRepositoryScoped(stage) {
				if repoIndex == 0 {
					record := globalStageResults[stageIndex]
					if record.outcome != nil {
						record.outcome.Index = stageCounter
						result.stageOutcomes = append(result.stageOutcomes, *record.outcome)
						for key, outcome := range record.operationOutcomes {
							result.operationOutcomes[key] = outcome
						}
						stageCounter++
					}
					result.failures = append(result.failures, record.failures...)
				}
				continue
			}

			record := repoStageResults[repoIndex][stageIndex]
			if record.outcome != nil {
				record.outcome.Index = stageCounter
				result.stageOutcomes = append(result.stageOutcomes, *record.outcome)
				for key, outcome := range record.operationOutcomes {
					result.operationOutcomes[key] = outcome
				}
				stageCounter++
			}
			result.failures = append(result.failures, record.failures...)
		}
	}

	return result
}

type repositoryStageResult struct {
	outcome           *StageOutcome
	operationOutcomes map[string]OperationOutcome
	failures          []recordedOperationFailure
}

type parallelStageResult struct {
	repositoryIndex   int
	stageIndex        int
	stageOutcome      *StageOutcome
	operationOutcomes map[string]OperationOutcome
	failures          []recordedOperationFailure
	stageFailed       bool
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
	return &clone
}

func sanitizeRepositoryParallelism(requested int, repositoryCount int) int {
	if repositoryCount <= 1 {
		return 1
	}
	parallelism := requested
	if parallelism <= 0 {
		parallelism = defaultRepositoryParallelism
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

		operationOutcomes[fmt.Sprintf("%s@%s", node.Name, repoLabel)] = OperationOutcome{
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
