package utils

import (
	"context"
	"strings"
)

const (
	configurationFilePathContextKeyConstant = commandContextKey("configurationFilePath")
	repositoryContextKeyConstant            = commandContextKey("repositoryContext")
	branchContextKeyConstant                = commandContextKey("branchContext")
	executionFlagsContextKeyConstant        = commandContextKey("executionFlags")
	logLevelContextKeyConstant              = commandContextKey("logLevel")
)

type commandContextKey string

// RepositoryContext describes repository-level execution context.
type RepositoryContext struct {
	Owner string
	Name  string
}

// BranchContext describes branch-level execution context.
type BranchContext struct {
	Name         string
	RequireClean bool
}

// ExecutionFlags captures standardized execution modifiers derived from CLI flags.
type ExecutionFlags struct {
	AssumeYes    bool
	AssumeYesSet bool
	Remote       string
	RemoteSet    bool
}

// CommandContextAccessor manages values stored in command execution contexts.
type CommandContextAccessor struct{}

// NewCommandContextAccessor constructs a CommandContextAccessor instance.
func NewCommandContextAccessor() CommandContextAccessor {
	return CommandContextAccessor{}
}

// WithConfigurationFilePath attaches the configuration file path to the provided context.
func (accessor CommandContextAccessor) WithConfigurationFilePath(parentContext context.Context, configurationFilePath string) context.Context {
	if parentContext == nil {
		parentContext = context.Background()
	}
	return context.WithValue(parentContext, configurationFilePathContextKeyConstant, configurationFilePath)
}

// WithRepositoryContext attaches repository context details to the provided context when values are present.
func (accessor CommandContextAccessor) WithRepositoryContext(parentContext context.Context, repository RepositoryContext) context.Context {
	if parentContext == nil {
		parentContext = context.Background()
	}
	normalizedOwner := strings.TrimSpace(repository.Owner)
	normalizedName := strings.TrimSpace(repository.Name)
	if len(normalizedOwner) == 0 && len(normalizedName) == 0 {
		return parentContext
	}
	normalized := RepositoryContext{Owner: normalizedOwner, Name: normalizedName}
	return context.WithValue(parentContext, repositoryContextKeyConstant, normalized)
}

// WithBranchContext attaches branch context details to the provided context when values are present.
func (accessor CommandContextAccessor) WithBranchContext(parentContext context.Context, branch BranchContext) context.Context {
	if parentContext == nil {
		parentContext = context.Background()
	}
	normalizedName := strings.TrimSpace(branch.Name)
	if len(normalizedName) == 0 && !branch.RequireClean {
		return parentContext
	}
	normalized := BranchContext{Name: normalizedName, RequireClean: branch.RequireClean}
	return context.WithValue(parentContext, branchContextKeyConstant, normalized)
}

// WithExecutionFlags attaches execution flag values to the provided context.
func (accessor CommandContextAccessor) WithExecutionFlags(parentContext context.Context, flags ExecutionFlags) context.Context {
	if parentContext == nil {
		parentContext = context.Background()
	}
	return context.WithValue(parentContext, executionFlagsContextKeyConstant, flags)
}

// WithLogLevel attaches the effective log level to the provided context.
func (accessor CommandContextAccessor) WithLogLevel(parentContext context.Context, logLevel string) context.Context {
	if parentContext == nil {
		parentContext = context.Background()
	}
	trimmedLogLevel := strings.TrimSpace(logLevel)
	if len(trimmedLogLevel) == 0 {
		return parentContext
	}
	return context.WithValue(parentContext, logLevelContextKeyConstant, trimmedLogLevel)
}

// ConfigurationFilePath extracts the configuration file path from the provided context.
func (accessor CommandContextAccessor) ConfigurationFilePath(executionContext context.Context) (string, bool) {
	if executionContext == nil {
		return "", false
	}
	configurationFilePath, configurationFilePathAvailable := executionContext.Value(configurationFilePathContextKeyConstant).(string)
	if !configurationFilePathAvailable {
		return "", false
	}
	return configurationFilePath, true
}

// RepositoryContext extracts repository context details from the provided execution context.
func (accessor CommandContextAccessor) RepositoryContext(executionContext context.Context) (RepositoryContext, bool) {
	if executionContext == nil {
		return RepositoryContext{}, false
	}
	value, valueAvailable := executionContext.Value(repositoryContextKeyConstant).(RepositoryContext)
	if !valueAvailable {
		return RepositoryContext{}, false
	}
	return value, true
}

// BranchContext extracts branch context details from the provided execution context.
func (accessor CommandContextAccessor) BranchContext(executionContext context.Context) (BranchContext, bool) {
	if executionContext == nil {
		return BranchContext{}, false
	}
	value, valueAvailable := executionContext.Value(branchContextKeyConstant).(BranchContext)
	if !valueAvailable {
		return BranchContext{}, false
	}
	return value, true
}

// ExecutionFlags extracts execution flag values from the provided context.
func (accessor CommandContextAccessor) ExecutionFlags(executionContext context.Context) (ExecutionFlags, bool) {
	if executionContext == nil {
		return ExecutionFlags{}, false
	}
	value, valueAvailable := executionContext.Value(executionFlagsContextKeyConstant).(ExecutionFlags)
	if !valueAvailable {
		return ExecutionFlags{}, false
	}
	return value, true
}

// LogLevel extracts the effective log level from the provided context.
func (accessor CommandContextAccessor) LogLevel(executionContext context.Context) (string, bool) {
	if executionContext == nil {
		return "", false
	}
	value, valueAvailable := executionContext.Value(logLevelContextKeyConstant).(string)
	if !valueAvailable {
		return "", false
	}
	return value, true
}
