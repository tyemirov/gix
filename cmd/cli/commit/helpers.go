package commit

import (
	"os"
	"strings"

	"go.uber.org/zap"

	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
)

// LoggerProvider yields a zap logger for command execution.
type LoggerProvider func() *zap.Logger

// TaskRunnerExecutor represents a workflow runner.
type TaskRunnerExecutor = taskrunner.Executor

// TaskRunnerFactory constructs task runners.
type TaskRunnerFactory = taskrunner.Factory

func resolveLogger(provider LoggerProvider) *zap.Logger {
	if provider == nil {
		return zap.NewNop()
	}
	logger := provider()
	if logger == nil {
		return zap.NewNop()
	}
	return logger
}

func lookupEnvironmentValue(name string) (string, bool) {
	value, ok := os.LookupEnv(name)
	return strings.TrimSpace(value), ok
}

func resolveTaskRunner(factory TaskRunnerFactory, dependencies workflow.Dependencies) TaskRunnerExecutor {
	return taskrunner.Resolve(factory, dependencies)
}
