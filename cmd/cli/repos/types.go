package repos

import (
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/temirov/gix/internal/repos/shared"
)

// LoggerProvider yields a zap logger for command execution.
type LoggerProvider func() *zap.Logger

// PrompterFactory creates confirmation prompters scoped to a Cobra command.
type PrompterFactory func(*cobra.Command) shared.ConfirmationPrompter
