package release

import "go.uber.org/zap"

// LoggerProvider yields a zap logger for command execution.
type LoggerProvider func() *zap.Logger
