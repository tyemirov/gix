package commit

import (
	"go.uber.org/zap"

	"github.com/tyemirov/gix/pkg/taskrunner"
)

// LoggerProvider yields a zap logger for command execution.
type LoggerProvider func() *zap.Logger

// TaskRunnerExecutor represents a workflow runner.
type TaskRunnerExecutor = taskrunner.Executor

// TaskRunnerFactory constructs task runners.
type TaskRunnerFactory = taskrunner.Factory
