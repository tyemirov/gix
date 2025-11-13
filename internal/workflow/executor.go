package workflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/temirov/gix/internal/audit"
	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/gitrepo"
	repoerrors "github.com/temirov/gix/internal/repos/errors"
	"github.com/temirov/gix/internal/repos/shared"
	pathutils "github.com/temirov/gix/internal/utils/path"
)

const (
	workflowExecutorDependenciesMessage = "workflow executor requires repository discovery, git, and GitHub dependencies"
	workflowExecutorMissingRootsMessage = "workflow executor requires at least one repository root"
	workflowRepositoryLoadErrorTemplate = "failed to inspect repositories: %w"
)

// Dependencies configures shared collaborators for workflow execution.
type Dependencies struct {
	Logger                 *zap.Logger
	RepositoryDiscoverer   shared.RepositoryDiscoverer
	GitExecutor            shared.GitExecutor
	RepositoryManager      *gitrepo.RepositoryManager
	GitHubClient           *githubcli.Client
	FileSystem             shared.FileSystem
	Prompter               shared.ConfirmationPrompter
	Output                 io.Writer
	Errors                 io.Writer
	HumanReadableLogging   bool
	DisableWorkflowLogging bool
}

// RuntimeOptions captures user-provided execution modifiers.
type RuntimeOptions struct {
	AssumeYes                            bool
	IncludeNestedRepositories            bool
	ProcessRepositoriesByDescendingDepth bool
	CaptureInitialWorktreeStatus         bool
	// SkipRepositoryMetadata disables GitHub metadata resolution during repository inspections.
	SkipRepositoryMetadata bool
	Variables              map[string]string
}

// Executor coordinates workflow operation execution.
type Executor struct {
	nodes        []*OperationNode
	dependencies Dependencies
}

type operationFailureError struct {
	message string
	cause   error
}

func (failure operationFailureError) Error() string {
	return failure.message
}

func (failure operationFailureError) Unwrap() error {
	return failure.cause
}

type recordedOperationFailure struct {
	name    string
	err     error
	message string
}

func collectOperationErrors(err error) []error {
	if err == nil {
		return nil
	}

	type multiUnwrapper interface{ Unwrap() []error }
	type singleUnwrapper interface{ Unwrap() error }

	if _, ok := err.(repoerrors.OperationError); ok {
		return []error{err}
	}

	if multi, ok := err.(multiUnwrapper); ok {
		children := multi.Unwrap()
		results := make([]error, 0, len(children))
		for _, child := range children {
			results = append(results, collectOperationErrors(child)...)
		}
		return results
	}

	if single, ok := err.(singleUnwrapper); ok {
		child := single.Unwrap()
		if child != nil {
			return collectOperationErrors(child)
		}
	}

	return []error{err}
}

// NewExecutor constructs an Executor instance from the provided operations, preserving sequential execution semantics.
func NewExecutor(operations []Operation, dependencies Dependencies) *Executor {
	nodes := make([]*OperationNode, 0, len(operations))
	var previousName string

	for operationIndex := range operations {
		operation := operations[operationIndex]
		if operation == nil {
			continue
		}

		baseName := strings.TrimSpace(operation.Name())
		if len(baseName) == 0 {
			baseName = "step"
		}
		stepName := fmt.Sprintf("%s-%d", baseName, operationIndex+1)

		node := &OperationNode{
			Name:      stepName,
			Operation: operation,
		}
		if len(previousName) > 0 {
			node.Dependencies = []string{previousName}
		}

		nodes = append(nodes, node)
		previousName = stepName
	}

	return NewExecutorFromNodes(nodes, dependencies)
}

// NewExecutorFromNodes constructs an Executor using pre-built operation nodes.
func NewExecutorFromNodes(nodes []*OperationNode, dependencies Dependencies) *Executor {
	cloned := make([]*OperationNode, len(nodes))
	copy(cloned, nodes)
	return &Executor{nodes: cloned, dependencies: dependencies}
}

// Execute orchestrates workflow operations across discovered repositories.
func (executor *Executor) Execute(executionContext context.Context, roots []string, runtimeOptions RuntimeOptions) (ExecutionOutcome, error) {
	outcome := ExecutionOutcome{
		StartTime: time.Now(),
	}

	requireGitHubClient := !runtimeOptions.SkipRepositoryMetadata
	if executor.dependencies.RepositoryDiscoverer == nil || executor.dependencies.GitExecutor == nil || executor.dependencies.RepositoryManager == nil || (requireGitHubClient && executor.dependencies.GitHubClient == nil) {
		return outcome, errors.New(workflowExecutorDependenciesMessage)
	}

	sanitizerConfiguration := pathutils.RepositoryPathSanitizerConfiguration{PruneNestedPaths: !runtimeOptions.IncludeNestedRepositories}
	repositoryPathSanitizer := pathutils.NewRepositoryPathSanitizerWithConfiguration(nil, sanitizerConfiguration)
	sanitizedRoots := repositoryPathSanitizer.Sanitize(roots)
	if len(sanitizedRoots) == 0 {
		return outcome, errors.New(workflowExecutorMissingRootsMessage)
	}

	var metadataResolver audit.GitHubMetadataResolver
	if !runtimeOptions.SkipRepositoryMetadata {
		metadataResolver = executor.dependencies.GitHubClient
	}

	auditService := audit.NewService(
		executor.dependencies.RepositoryDiscoverer,
		executor.dependencies.RepositoryManager,
		executor.dependencies.GitExecutor,
		metadataResolver,
		executor.dependencies.Output,
		executor.dependencies.Errors,
	)

	inspections, inspectionError := auditService.DiscoverInspections(executionContext, sanitizedRoots, false, false, audit.InspectionDepthFull)
	if inspectionError != nil {
		return outcome, fmt.Errorf(workflowRepositoryLoadErrorTemplate, inspectionError)
	}

	repositoryStates := make([]*RepositoryState, 0, len(inspections))
	existingRepositories := make(map[string]struct{})
	for inspectionIndex := range inspections {
		state := NewRepositoryState(inspections[inspectionIndex])
		state.PathDepth = repositoryPathDepth(state.Path)
		repositoryStates = append(repositoryStates, state)
		existingRepositories[canonicalRepositoryIdentifier(state.Path)] = struct{}{}
	}

	for _, sanitizedRoot := range sanitizedRoots {
		canonicalRoot := canonicalRepositoryIdentifier(sanitizedRoot)
		if _, alreadyPresent := existingRepositories[canonicalRoot]; alreadyPresent {
			continue
		}
		if executor.dependencies.GitExecutor != nil {
			commandDetails := execshell.CommandDetails{
				Arguments:        []string{"rev-parse", "--is-inside-work-tree"},
				WorkingDirectory: sanitizedRoot,
			}
			result, gitError := executor.dependencies.GitExecutor.ExecuteGit(executionContext, commandDetails)
			if gitError != nil || strings.TrimSpace(result.StandardOutput) != "true" {
				continue
			}
		}
		inspection := audit.RepositoryInspection{Path: sanitizedRoot, FolderName: filepath.Base(sanitizedRoot), IsGitRepository: true}
		state := NewRepositoryState(inspection)
		state.PathDepth = repositoryPathDepth(state.Path)
		repositoryStates = append(repositoryStates, state)
		existingRepositories[canonicalRoot] = struct{}{}
	}

	if runtimeOptions.IncludeNestedRepositories {
		markNestedRepositoryAncestors(repositoryStates)
	}

	if runtimeOptions.CaptureInitialWorktreeStatus {
		captureInitialCleanStatuses(executionContext, executor.dependencies.RepositoryManager, repositoryStates)
	}

	outcome.RepositoryCount = len(repositoryStates)

	if runtimeOptions.ProcessRepositoriesByDescendingDepth {
		sort.SliceStable(repositoryStates, func(firstIndex int, secondIndex int) bool {
			first := repositoryStates[firstIndex]
			second := repositoryStates[secondIndex]
			if first.PathDepth == second.PathDepth {
				return first.Path < second.Path
			}
			return first.PathDepth > second.PathDepth
		})
	}

	promptState := NewPromptState(runtimeOptions.AssumeYes)
	dispatchingPrompter := newPromptDispatcher(executor.dependencies.Prompter, promptState)

	state := &State{Roots: sanitizedRoots, Repositories: repositoryStates}
	reporterOutput := executor.dependencies.Output
	errorWriter := executor.dependencies.Errors
	if errorWriter == nil {
		errorWriter = reporterOutput
	}
	if executor.dependencies.Errors != nil {
		reporterOutput = executor.dependencies.Errors
	}
	if !executor.dependencies.HumanReadableLogging || executor.dependencies.DisableWorkflowLogging {
		reporterOutput = io.Discard
	}
	if executor.dependencies.DisableWorkflowLogging {
		errorWriter = io.Discard
	}
	reporter := shared.NewStructuredReporter(
		reporterOutput,
		errorWriter,
		shared.WithRepositoryHeaders(executor.dependencies.HumanReadableLogging),
	)
	environment := &Environment{
		AuditService:      auditService,
		GitExecutor:       executor.dependencies.GitExecutor,
		RepositoryManager: executor.dependencies.RepositoryManager,
		GitHubClient:      executor.dependencies.GitHubClient,
		FileSystem:        executor.dependencies.FileSystem,
		Prompter:          dispatchingPrompter,
		PromptState:       promptState,
		Output:            executor.dependencies.Output,
		Errors:            executor.dependencies.Errors,
		Reporter:          reporter,
		Logger:            executor.dependencies.Logger,
		Variables:         NewVariableStore(),
	}
	environment.State = state
	if environment.Variables != nil {
		if runID, err := NewVariableName("workflow_run_id"); err == nil {
			environment.Variables.Seed(runID, time.Now().UTC().Format("20060102T150405"))
		}
	}
	if len(runtimeOptions.Variables) > 0 && environment.Variables != nil {
		for key, value := range runtimeOptions.Variables {
			environment.Variables.Seed(VariableName(key), value)
		}
	}

	for _, repository := range repositoryStates {
		if repository == nil {
			continue
		}
		identifier := strings.TrimSpace(repository.Inspection.FinalOwnerRepo)
		if identifier == "" {
			identifier = strings.TrimSpace(repository.Inspection.CanonicalOwnerRepo)
		}
		if reporter != nil {
			reporter.RecordRepository(identifier, repository.Path)
		}
	}

	stages, planError := planOperationStages(executor.nodes)
	if planError != nil {
		return outcome, planError
	}

	stageResults := runOperationStages(executionContext, stages, environment, state, reporter, executor.dependencies.Logger)
	stageFailures := stageResults.failures

	outcome.StageOutcomes = append(outcome.StageOutcomes, stageResults.stageOutcomes...)

	orderedOperationOutcomes := make([]OperationOutcome, 0, len(stageResults.operationOutcomes))
	for _, stage := range stageResults.stageOutcomes {
		for _, operationName := range stage.Operations {
			if result, exists := stageResults.operationOutcomes[operationName]; exists {
				orderedOperationOutcomes = append(orderedOperationOutcomes, result)
			}
		}
	}
	outcome.OperationOutcomes = orderedOperationOutcomes

	if reporter != nil {
		summaryData := reporter.SummaryData()
		outcome.ReporterSummaryData = summaryData
		if executor.dependencies.Logger != nil {
			executor.dependencies.Logger.Info("workflow_summary", zap.Any("summary", summaryData))
		}
		reporter.PrintSummary()
	}

	outcome.EndTime = time.Now()
	outcome.Duration = outcome.EndTime.Sub(outcome.StartTime)

	if len(stageFailures) == 0 {
		return outcome, nil
	}

	distinctErrors := make([]error, 0, len(stageFailures))
	failuresExport := make([]OperationFailure, 0, len(stageFailures))
	for _, failure := range stageFailures {
		distinctErrors = append(distinctErrors, failure.err)
		failuresExport = append(failuresExport, OperationFailure{
			Name:    failure.name,
			Error:   failure.err,
			Message: failure.message,
		})
	}
	outcome.Failures = failuresExport

	message := stageFailures[0].message
	if len(stageFailures) > 1 {
		message = fmt.Sprintf("%s (and %d more failures)", message, len(stageFailures)-1)
	}

	return outcome, operationFailureError{
		message: message,
		cause:   errors.Join(distinctErrors...),
	}
}

func canonicalRepositoryIdentifier(path string) string {
	cleaned := filepath.Clean(path)

	absolutePath := cleaned
	if abs, err := filepath.Abs(cleaned); err == nil {
		absolutePath = filepath.Clean(abs)
	}

	resolvedPath := absolutePath
	if evaluated, err := filepath.EvalSymlinks(absolutePath); err == nil && len(strings.TrimSpace(evaluated)) > 0 {
		resolvedPath = filepath.Clean(evaluated)
	}

	if runtime.GOOS == "windows" {
		resolvedPath = strings.ToLower(resolvedPath)
	}

	return resolvedPath
}

func repositoryPathDepth(path string) int {
	cleaned := filepath.Clean(path)
	if len(cleaned) == 0 || cleaned == "." {
		return 0
	}
	normalized := filepath.ToSlash(cleaned)
	return strings.Count(normalized, "/")
}

func markNestedRepositoryAncestors(repositories []*RepositoryState) {
	for ancestorIndex := range repositories {
		ancestorPath := repositories[ancestorIndex].Path
		for candidateIndex := range repositories {
			if ancestorIndex == candidateIndex {
				continue
			}
			descendantPath := repositories[candidateIndex].Path
			if isAncestorPath(ancestorPath, descendantPath) {
				repositories[ancestorIndex].HasNestedRepositories = true
				break
			}
		}
	}
}

func isAncestorPath(potentialAncestor string, potentialDescendant string) bool {
	if len(strings.TrimSpace(potentialAncestor)) == 0 || len(strings.TrimSpace(potentialDescendant)) == 0 {
		return false
	}
	relativePath, relativeError := filepath.Rel(potentialAncestor, potentialDescendant)
	if relativeError != nil {
		return false
	}
	if relativePath == "." {
		return false
	}
	return !strings.HasPrefix(relativePath, "..")
}

func captureInitialCleanStatuses(executionContext context.Context, manager *gitrepo.RepositoryManager, repositories []*RepositoryState) {
	if manager == nil {
		return
	}
	for repositoryIndex := range repositories {
		repository := repositories[repositoryIndex]
		clean, cleanError := manager.CheckCleanWorktree(executionContext, repository.Path)
		if cleanError != nil {
			continue
		}
		repository.InitialCleanWorktree = clean
	}
}

func formatOperationFailure(environment *Environment, err error, operationName string) string {
	var operationError repoerrors.OperationError
	if errors.As(err, &operationError) {
		formatted := formatRepositoryOperationError(environment, operationError)
		return strings.TrimSuffix(formatted, "\n")
	}

	message := strings.TrimSpace(err.Error())
	if len(message) == 0 {
		if len(operationName) == 0 {
			return "operation failed"
		}
		return fmt.Sprintf("%s failed", operationName)
	}
	if len(operationName) == 0 {
		return message
	}

	lowerMessage := strings.ToLower(message)
	lowerOperation := strings.ToLower(operationName)
	if strings.HasPrefix(lowerMessage, lowerOperation) {
		return message
	}

	return fmt.Sprintf("%s: %s", operationName, message)
}
