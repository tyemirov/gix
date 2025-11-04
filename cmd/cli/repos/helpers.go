package repos

import (
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/repos/prompt"
	"github.com/tyemirov/gix/internal/repos/shared"
	rootutils "github.com/tyemirov/gix/internal/utils/roots"
)

// LoggerProvider yields a zap logger for command execution.
type LoggerProvider func() *zap.Logger

// PrompterFactory creates confirmation prompters scoped to a Cobra command.
type PrompterFactory func(*cobra.Command) shared.ConfirmationPrompter

func requireRepositoryRoots(command *cobra.Command, arguments []string, configuredRoots []string) ([]string, error) {
	roots, resolveError := rootutils.Resolve(command, arguments, configuredRoots)
	if resolveError != nil {
		return nil, resolveError
	}
	return roots, nil
}

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

// cascadingConfirmationPrompter forwards confirmations while tracking apply-to-all decisions.
type cascadingConfirmationPrompter struct {
	basePrompter shared.ConfirmationPrompter
	assumeYes    bool
}

func newCascadingConfirmationPrompter(base shared.ConfirmationPrompter, initialAssumeYes bool) *cascadingConfirmationPrompter {
	return &cascadingConfirmationPrompter{basePrompter: base, assumeYes: initialAssumeYes}
}

func (prompter *cascadingConfirmationPrompter) Confirm(prompt string) (shared.ConfirmationResult, error) {
	if prompter.basePrompter == nil {
		return shared.ConfirmationResult{}, nil
	}
	result, err := prompter.basePrompter.Confirm(prompt)
	if err != nil {
		return shared.ConfirmationResult{}, err
	}
	if result.ApplyToAll {
		prompter.assumeYes = true
	}
	return result, nil
}

func (prompter *cascadingConfirmationPrompter) AssumeYes() bool {
	return prompter.assumeYes
}

func displayCommandHelp(command *cobra.Command) error {
	if command == nil {
		return nil
	}
	return command.Help()
}
