package roots

import (
	"errors"

	"github.com/spf13/cobra"

	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	pathutils "github.com/tyemirov/gix/internal/utils/path"
)

const (
	missingRootsErrorMessage          = "no repository roots provided; specify --roots or configure defaults"
	positionalRootsUnsupportedMessage = "repository roots must be provided using --roots"
)

var sanitizer = pathutils.NewRepositoryPathSanitizerWithConfiguration(nil, pathutils.RepositoryPathSanitizerConfiguration{ExcludeBooleanLiteralCandidates: true, PruneNestedPaths: true})

// MissingRootsError returns the canonical error message when no roots are supplied.
func MissingRootsError() error {
	return errors.New(missingRootsErrorMessage)
}

// PositionalRootsUnsupportedError returns the canonical error when positional roots are supplied.
func PositionalRootsUnsupportedError() error {
	return errors.New(positionalRootsUnsupportedMessage)
}

// Resolve determines the repository roots for a command, enforcing --roots usage.
func Resolve(command *cobra.Command, positional []string, configured []string) ([]string, error) {
	if len(sanitizer.Sanitize(positional)) > 0 {
		if command != nil {
			_ = command.Help()
		}
		return nil, PositionalRootsUnsupportedError()
	}

	flagRoots, flagError := FlagValues(command)
	if flagError != nil {
		return nil, flagError
	}
	if len(flagRoots) > 0 {
		return flagRoots, nil
	}

	configuredRoots := sanitizer.Sanitize(configured)
	if len(configuredRoots) > 0 {
		return configuredRoots, nil
	}

	if command != nil {
		_ = command.Help()
	}
	return nil, MissingRootsError()
}

// FlagValues returns sanitized root values from the command flag set.
func FlagValues(command *cobra.Command) ([]string, error) {
	if command == nil {
		return nil, nil
	}
	values, err := command.Flags().GetStringSlice(flagutils.DefaultRootFlagName)
	if err != nil {
		return nil, err
	}
	return sanitizer.Sanitize(values), nil
}

// SanitizeConfigured normalizes configured root values.
func SanitizeConfigured(configured []string) []string {
	return sanitizer.Sanitize(configured)
}

// MissingRootsMessage exposes the canonical missing-roots error text.
func MissingRootsMessage() string {
	return missingRootsErrorMessage
}

// PositionalRootsUnsupportedMessage exposes the canonical positional-roots error text.
func PositionalRootsUnsupportedMessage() string {
	return positionalRootsUnsupportedMessage
}
