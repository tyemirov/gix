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
	"sync"

	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
	repoerrors "github.com/tyemirov/gix/internal/repos/errors"
	"github.com/tyemirov/gix/internal/repos/shared"
	pathutils "github.com/tyemirov/gix/internal/utils/path"
)

const (
	workflowExecutorDependenciesMessage = "workflow executor requires repository discovery, git, and GitHub dependencies"
	workflowExecutorMissingRootsMessage = "workflow executor requires at least one repository root"
	workflowRepositoryLoadErrorTemplate = "failed to inspect repositories: %w"
)

// Dependencies configures shared collaborators for workflow execution.
type Dependencies struct {
	Logger               *zap.Logger
	RepositoryDiscoverer shared.RepositoryDiscoverer
	GitExecutor          shared.GitExecutor
	RepositoryManager    *gitrepo.RepositoryManager
	GitHubClient         *githubcli.Client
	FileSystem           shared.FileSystem
	Prompter             shared.ConfirmationPrompter
	Output               io.Writer
	Errors               io.Writer
}

// RuntimeOptions captures user-provided execution modifiers.
type RuntimeOptions struct {
	DryRun                               bool
	AssumeYes                            bool
	IncludeNestedRepositories            bool
	ProcessRepositoriesByDescendingDepth bool
	CaptureInitialWorktreeStatus         bool
	// SkipRepositoryMetadata disables GitHub metadata resolution during repository inspections.
	SkipRepositoryMetadata bool
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
func (executor *Executor) Execute(executionContext context.Context, roots []string, runtimeOptions RuntimeOptions) error {
	requireGitHubClient := !runtimeOptions.SkipRepositoryMetadata
	if executor.dependencies.RepositoryDiscoverer == nil || executor.dependencies.GitExecutor == nil || executor.dependencies.RepositoryManager == nil || (requireGitHubClient && executor.dependencies.GitHubClient == nil) {
		return errors.New(workflowExecutorDependenciesMessage)
	}

	sanitizerConfiguration := pathutils.RepositoryPathSanitizerConfiguration{PruneNestedPaths: !runtimeOptions.IncludeNestedRepositories}
	repositoryPathSanitizer := pathutils.NewRepositoryPathSanitizerWithConfiguration(nil, sanitizerConfiguration)
	sanitizedRoots := repositoryPathSanitizer.Sanitize(roots)
	if len(sanitizedRoots) == 0 {
		return errors.New(workflowExecutorMissingRootsMessage)
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
		return fmt.Errorf(workflowRepositoryLoadErrorTemplate, inspectionError)
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
	reporter := shared.NewStructuredReporter(
		executor.dependencies.Output,
		executor.dependencies.Errors,
		shared.WithRepositoryHeaders(true),
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
		DryRun:            runtimeOptions.DryRun,
		Variables:         NewVariableStore(),
	}
	environment.State = state

	var (
		failureMu sync.Mutex
		failures  []recordedOperationFailure
	)

	for _, repository := range repositoryStates {
		if repository == nil {
			continue
		}
		identifier := strings.TrimSpace(repository.Inspection.FinalOwnerRepo)
		if identifier == "" {
			identifier = strings.TrimSpace(repository.Inspection.CanonicalOwnerRepo)
		}
		reporter.RecordRepository(identifier, repository.Path)
	}

	stages, planError := planOperationStages(executor.nodes)
	if planError != nil {
		return planError
	}

	for stageIndex := range stages {
		stage := stages[stageIndex]
		if len(stage.Operations) == 0 {
			continue
		}

		var waitGroup sync.WaitGroup

		for _, node := range stage.Operations {
			if node == nil || node.Operation == nil {
				continue
			}

			waitGroup.Add(1)
			go func(operation Operation) {
				defer waitGroup.Done()
				if executeError := operation.Execute(executionContext, environment, state); executeError != nil {
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
						failures = append(failures, recordedOperationFailure{name: operation.Name(), err: failureErr, message: formatted})
					}
					failureMu.Unlock()
				}
			}(node.Operation)
		}

		waitGroup.Wait()
	}

	if reporter != nil {
		reporter.PrintSummary()
	}

	if len(failures) == 0 {
		return nil
	}

	distinctErrors := make([]error, 0, len(failures))
	for _, failure := range failures {
		distinctErrors = append(distinctErrors, failure.err)
	}

	message := failures[0].message
	if len(failures) > 1 {
		message = fmt.Sprintf("%s (and %d more failures)", message, len(failures)-1)
	}

	return operationFailureError{
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
