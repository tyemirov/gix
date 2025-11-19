package workflow

import (
	"context"
	"io"
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
