package workflow

import (
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/temirov/gix/internal/repos/prompt"
	"github.com/temirov/gix/internal/repos/shared"
	workflowpkg "github.com/temirov/gix/internal/workflow"
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

func resolvePrompter(factory PrompterFactory, command *cobra.Command) shared.ConfirmationPrompter {
	if factory != nil {
		prompter := factory(command)
		if prompter != nil {
			return prompter
		}
	}
	return prompt.NewIOConfirmationPrompter(command.InOrStdin(), command.OutOrStdout())
}

func displayCommandHelp(command *cobra.Command) error {
	if command == nil {
		return nil
	}
	return command.Help()
}

func resolveTaskRunner(factory TaskRunnerFactory, dependencies workflowpkg.Dependencies) TaskRunnerExecutor {
	return taskrunner.Resolve(factory, dependencies)
}
