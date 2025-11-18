package rename

import (
	"errors"
	"strings"

	"github.com/temirov/gix/internal/repos/shared"
)

var (
	// ErrDesiredFolderMissing indicates that no target folder was provided.
	ErrDesiredFolderMissing = errors.New("rename desired folder missing")
)

// Options configures a rename execution.
type Options struct {
	repositoryPath          shared.RepositoryPath
	desiredFolderName       string
	cleanPolicy             shared.CleanWorktreePolicy
	confirmationPolicy      shared.ConfirmationPolicy
	includeOwner            bool
	ensureParentDirectories bool
}

// OptionsDefinition captures the raw inputs for Options.
type OptionsDefinition struct {
	RepositoryPath          shared.RepositoryPath
	DesiredFolderName       string
	CleanPolicy             shared.CleanWorktreePolicy
	ConfirmationPolicy      shared.ConfirmationPolicy
	IncludeOwner            bool
	EnsureParentDirectories bool
}

// NewOptions validates and constructs rename options.
func NewOptions(definition OptionsDefinition) (Options, error) {
	trimmed := strings.TrimSpace(definition.DesiredFolderName)
	if len(trimmed) == 0 {
		return Options{}, ErrDesiredFolderMissing
	}

	return Options{
		repositoryPath:          definition.RepositoryPath,
		desiredFolderName:       trimmed,
		cleanPolicy:             definition.CleanPolicy,
		confirmationPolicy:      definition.ConfirmationPolicy,
		includeOwner:            definition.IncludeOwner,
		ensureParentDirectories: definition.EnsureParentDirectories,
	}, nil
}

// DesiredFolderName returns the normalized rename target.
func (options Options) DesiredFolderName() string {
	return options.desiredFolderName
}
