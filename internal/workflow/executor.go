package workflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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

	auditService := audit.NewService(
		executor.dependencies.RepositoryDiscoverer,
		executor.dependencies.RepositoryManager,
		executor.dependencies.GitExecutor,
		executor.dependencies.GitHubClient,
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
		existingRepositories[state.Path] = struct{}{}
	}

	for _, sanitizedRoot := range sanitizedRoots {
		if _, alreadyPresent := existingRepositories[sanitizedRoot]; alreadyPresent {
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
		Logger:            executor.dependencies.Logger,
		DryRun:            runtimeOptions.DryRun,
	}
	environment.State = state

	stages, planError := planOperationStages(executor.nodes)
	if planError != nil {
		return planError
	}

	for stageIndex := range stages {
		stage := stages[stageIndex]
		if len(stage.Operations) == 0 {
			continue
		}

		stageContext, cancelStage := context.WithCancel(executionContext)
		var stageError error
		var failedOperation Operation
		var once sync.Once
		var waitGroup sync.WaitGroup

		for _, node := range stage.Operations {
			if node == nil || node.Operation == nil {
				continue
			}

			waitGroup.Add(1)
			go func(operation Operation) {
				defer waitGroup.Done()
				if executeError := operation.Execute(stageContext, environment, state); executeError != nil {
					once.Do(func() {
						stageError = executeError
						failedOperation = operation
						cancelStage()
					})
				}
			}(node.Operation)
		}

		waitGroup.Wait()
		cancelStage()

		if stageError != nil {
			operationName := ""
			if failedOperation != nil {
				operationName = failedOperation.Name()
			}
			failureMessage := formatOperationFailure(environment, stageError, operationName)
			return operationFailureError{message: failureMessage, cause: stageError}
		}
	}

	return nil

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
