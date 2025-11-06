package workflow

import (
	"context"
	"io"

	"go.uber.org/zap"

	"github.com/temirov/gix/internal/audit"
	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/gitrepo"
	"github.com/temirov/gix/internal/repos/shared"
)

// Operation coordinates a single workflow step across repositories.
type Operation interface {
	Name() string
	Execute(executionContext context.Context, environment *Environment, state *State) error
}

// Environment exposes shared dependencies for workflow operations.
type Environment struct {
	AuditService        *audit.Service
	GitExecutor         shared.GitExecutor
	RepositoryManager   *gitrepo.RepositoryManager
	GitHubClient        *githubcli.Client
	FileSystem          shared.FileSystem
	Prompter            shared.ConfirmationPrompter
	PromptState         *PromptState
	Output              io.Writer
	Errors              io.Writer
	Reporter            shared.SummaryReporter
	Logger              *zap.Logger
	Variables           *VariableStore
	State               *State
	auditReportExecuted bool
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
