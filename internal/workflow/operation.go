package workflow

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/shared"
)

// Operation coordinates a single workflow step across repositories.
type Operation interface {
	Name() string
	Execute(executionContext context.Context, environment *Environment, state *State) error
}

// RepositoryScopedOperation executes work for a single repository at a time.
type RepositoryScopedOperation interface {
	Operation
	ExecuteForRepository(executionContext context.Context, environment *Environment, repository *RepositoryState) error
	IsRepositoryScoped() bool
}

// Environment exposes shared dependencies for workflow operations.
type Environment struct {
	AuditService      *audit.Service
	GitExecutor       shared.GitExecutor
	RepositoryManager *gitrepo.RepositoryManager
	GitHubClient      *githubcli.Client
	FileSystem        shared.FileSystem
	Prompter          shared.ConfirmationPrompter
	PromptState       *PromptState
	Output            io.Writer
	Errors            io.Writer
	Reporter          shared.SummaryReporter
	Logger            *zap.Logger
	Variables         *VariableStore
	State             *State
	sharedState       *environmentSharedState
	suppressHeaders   bool
}

type environmentSharedState struct {
	mutex               sync.Mutex
	auditReportExecuted bool
	lastRepositoryKey   string
	capturedKinds       map[string]map[string]CaptureKind
	capturedValues      map[string]map[string]string
}

func (environment *Environment) ensureSharedState() {
	if environment == nil {
		return
	}
	if environment.sharedState == nil {
		environment.sharedState = &environmentSharedState{}
	}
}

func (environment *Environment) auditReportHasExecuted() bool {
	if environment == nil {
		return false
	}
	environment.ensureSharedState()
	environment.sharedState.mutex.Lock()
	defer environment.sharedState.mutex.Unlock()
	return environment.sharedState.auditReportExecuted
}

func (environment *Environment) markAuditReportExecuted() {
	if environment == nil {
		return
	}
	environment.ensureSharedState()
	environment.sharedState.mutex.Lock()
	environment.sharedState.auditReportExecuted = true
	environment.sharedState.mutex.Unlock()
}

func (environment *Environment) repositoryKey() string {
	if environment == nil || environment.State == nil || len(environment.State.Repositories) == 0 {
		return ""
	}
	repository := environment.State.Repositories[0]
	if repository == nil {
		return ""
	}
	return strings.TrimSpace(repository.Path)
}

func (environment *Environment) capturedKindMapLocked(repositoryKey string) map[string]CaptureKind {
	if environment.sharedState.capturedKinds == nil {
		environment.sharedState.capturedKinds = make(map[string]map[string]CaptureKind)
	}
	kindMap, exists := environment.sharedState.capturedKinds[repositoryKey]
	if !exists {
		kindMap = make(map[string]CaptureKind)
		environment.sharedState.capturedKinds[repositoryKey] = kindMap
	}
	return kindMap
}

func (environment *Environment) capturedValueMapLocked(repositoryKey string) map[string]string {
	if environment.sharedState.capturedValues == nil {
		environment.sharedState.capturedValues = make(map[string]map[string]string)
	}
	valueMap, exists := environment.sharedState.capturedValues[repositoryKey]
	if !exists {
		valueMap = make(map[string]string)
		environment.sharedState.capturedValues[repositoryKey] = valueMap
	}
	return valueMap
}

// RecordCaptureKind remembers the kind used for a captured workflow variable.
func (environment *Environment) RecordCaptureKind(name VariableName, kind CaptureKind) {
	if environment == nil {
		return
	}
	environment.ensureSharedState()
	repositoryKey := environment.repositoryKey()
	environment.sharedState.mutex.Lock()
	defer environment.sharedState.mutex.Unlock()
	kindMap := environment.capturedKindMapLocked(repositoryKey)
	environment.capturedValueMapLocked(repositoryKey)
	kindMap[string(name)] = kind
}

// CaptureKindForVariable reports the capture kind previously recorded for the variable.
func (environment *Environment) CaptureKindForVariable(name VariableName) (CaptureKind, bool) {
	if environment == nil {
		return "", false
	}
	environment.ensureSharedState()
	repositoryKey := environment.repositoryKey()
	environment.sharedState.mutex.Lock()
	defer environment.sharedState.mutex.Unlock()
	if environment.sharedState.capturedKinds == nil {
		return "", false
	}
	kindMap, exists := environment.sharedState.capturedKinds[repositoryKey]
	if !exists {
		return "", false
	}
	kind, exists := kindMap[string(name)]
	return kind, exists
}

// StoreCaptureValue persists the captured value under the shared namespace.
func (environment *Environment) StoreCaptureValue(name VariableName, value string, overwrite bool) {
	if environment == nil {
		return
	}
	environment.ensureSharedState()
	repositoryKey := environment.repositoryKey()
	environment.sharedState.mutex.Lock()
	defer environment.sharedState.mutex.Unlock()
	valueMap := environment.capturedValueMapLocked(repositoryKey)
	key := string(name)
	if _, exists := valueMap[key]; exists && !overwrite {
		return
	}
	trimmedValue := strings.TrimSpace(value)
	valueMap[key] = trimmedValue
	if environment.Variables != nil {
		environment.Variables.Set(name, trimmedValue)
		environment.Variables.Set(VariableName(fmt.Sprintf("Captured.%s", key)), trimmedValue)
	}
}

// CapturedValue returns the previously stored capture value.
func (environment *Environment) CapturedValue(name VariableName) (string, bool) {
	if environment == nil {
		return "", false
	}
	environment.ensureSharedState()
	repositoryKey := environment.repositoryKey()
	environment.sharedState.mutex.Lock()
	defer environment.sharedState.mutex.Unlock()
	if environment.sharedState.capturedValues == nil {
		return "", false
	}
	valueMap, exists := environment.sharedState.capturedValues[repositoryKey]
	if !exists {
		return "", false
	}
	value, exists := valueMap[string(name)]
	return value, exists
}

// OperationDefaults captures fallback behaviors shared across operations.
type OperationDefaults struct {
	RequireClean bool
}

// ApplyDefaults configures operations with shared fallback options when not explicitly set.
func ApplyDefaults(nodes []*OperationNode, defaults OperationDefaults) {
	for nodeIndex := range nodes {
		if nodes[nodeIndex] == nil || nodes[nodeIndex].Operation == nil {
			continue
		}
		renameOperation, isRename := nodes[nodeIndex].Operation.(*RenameOperation)
		if !isRename {
			continue
		}
		renameOperation.ApplyRequireCleanDefault(defaults.RequireClean)
	}
}
