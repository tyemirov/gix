package workflow

import (
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/temirov/gix/internal/repos/shared"
	"github.com/temirov/gix/pkg/taskrunner"
)

// LoggerProvider yields a zap logger for command execution.
type LoggerProvider func() *zap.Logger

// PrompterFactory constructs confirmation prompters scoped to a command.
type PrompterFactory func(*cobra.Command) shared.ConfirmationPrompter

// TaskRunnerExecutor represents a workflow runner.
type TaskRunnerExecutor = taskrunner.Executor

// TaskRunnerFactory constructs workflow runners.
type TaskRunnerFactory = taskrunner.Factory

func displayCommandHelp(command *cobra.Command) error {
	if command == nil {
		return nil
	}
	return command.Help()
}
