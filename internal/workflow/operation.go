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
	capturedKinds       map[string]CaptureKind
	capturedValues      map[string]string
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

// RecordCaptureKind remembers the kind used for a captured workflow variable.
func (environment *Environment) RecordCaptureKind(name VariableName, kind CaptureKind) {
	if environment == nil {
		return
	}
	environment.ensureSharedState()
	if environment.sharedState.capturedKinds == nil {
		environment.sharedState.capturedKinds = make(map[string]CaptureKind)
	}
	if environment.sharedState.capturedValues == nil {
		environment.sharedState.capturedValues = make(map[string]string)
	}
	environment.sharedState.capturedKinds[string(name)] = kind
}

// CaptureKindForVariable reports the capture kind previously recorded for the variable.
func (environment *Environment) CaptureKindForVariable(name VariableName) (CaptureKind, bool) {
	if environment == nil {
		return "", false
	}
	environment.ensureSharedState()
	if environment.sharedState.capturedKinds == nil {
		return "", false
	}
	kind, exists := environment.sharedState.capturedKinds[string(name)]
	return kind, exists
}

// StoreCaptureValue persists the captured value under the shared namespace.
func (environment *Environment) StoreCaptureValue(name VariableName, value string, overwrite bool) {
	if environment == nil {
		return
	}
	environment.ensureSharedState()
	if environment.sharedState.capturedValues == nil {
		environment.sharedState.capturedValues = make(map[string]string)
	}
	key := string(name)
	if _, exists := environment.sharedState.capturedValues[key]; exists && !overwrite {
		return
	}
	environment.sharedState.capturedValues[key] = strings.TrimSpace(value)
	if environment.Variables != nil {
		environment.Variables.Set(VariableName(fmt.Sprintf("Captured.%s", key)), value)
	}
}

// CapturedValue returns the previously stored capture value.
func (environment *Environment) CapturedValue(name VariableName) (string, bool) {
	if environment == nil {
		return "", false
	}
	environment.ensureSharedState()
	if environment.sharedState.capturedValues == nil {
		return "", false
	}
	value, exists := environment.sharedState.capturedValues[string(name)]
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
