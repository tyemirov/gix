package flags

import "github.com/spf13/cobra"

const (
	// DefaultRootFlagName exposes the shared repository root flag name.
	DefaultRootFlagName = "roots"
	// DefaultRootFlagUsage describes the shared repository root flag purpose.
	DefaultRootFlagUsage = "Repository roots to scan (repeatable; nested paths ignored)"
	// AssumeYesFlagName exposes the shared assume-yes flag name.
	AssumeYesFlagName = "yes"
	// AssumeYesFlagShorthand provides the shorthand for the assume-yes flag.
	AssumeYesFlagShorthand = "y"
	// AssumeYesFlagUsage describes the shared assume-yes flag purpose.
	AssumeYesFlagUsage = "Automatically confirm prompts"
	// RemoteFlagName exposes the shared remote flag name.
	RemoteFlagName = "remote"
	// RemoteFlagUsage describes the shared remote flag purpose.
	RemoteFlagUsage = "Remote name to target"
)

// RepositoryFlagDefinition captures configuration for repository context flags.
type RepositoryFlagDefinition struct {
	Name    string
	Usage   string
	Enabled bool
}

// RepositoryFlagDefinitions groups repository context flag definitions.
type RepositoryFlagDefinitions struct {
	Owner RepositoryFlagDefinition
	Name  RepositoryFlagDefinition
}

// RepositoryFlagValues stores repository context flag values.
type RepositoryFlagValues struct {
	Owner string
	Name  string
}

// BindRepositoryFlags attaches repository context flags to the provided command.
func BindRepositoryFlags(command *cobra.Command, defaults RepositoryFlagValues, definitions RepositoryFlagDefinitions) *RepositoryFlagValues {
	values := defaults
	if command == nil {
		return &values
	}

	persistentFlagSet := command.PersistentFlags()
	if definitions.Owner.Enabled && len(definitions.Owner.Name) > 0 {
		persistentFlagSet.StringVar(&values.Owner, definitions.Owner.Name, defaults.Owner, definitions.Owner.Usage)
	}
	if definitions.Name.Enabled && len(definitions.Name.Name) > 0 {
		persistentFlagSet.StringVar(&values.Name, definitions.Name.Name, defaults.Name, definitions.Name.Usage)
	}

	return &values
}

// RootFlagDefinition captures configuration for repository root flags.
type RootFlagDefinition struct {
	Name       string
	Usage      string
	Enabled    bool
	Persistent bool
}

// RootFlagValues stores repository root flag values.
type RootFlagValues struct {
	Roots []string
}

// BindRootFlags attaches standard repository root flags to the provided command.

func BindRootFlags(command *cobra.Command, defaults RootFlagValues, definition RootFlagDefinition) *RootFlagValues {
	values := RootFlagValues{Roots: append([]string{}, defaults.Roots...)}
	if command == nil {
		return &values
	}
	if !definition.Enabled {
		return &values
	}
	flagName := definition.Name
	if len(flagName) == 0 {
		flagName = DefaultRootFlagName
	}
	flagUsage := definition.Usage
	if len(flagUsage) == 0 {
		flagUsage = DefaultRootFlagUsage
	}

	targetSet := command.PersistentFlags()
	if !definition.Persistent {
		targetSet = command.Flags()
	}

	if targetSet.Lookup(flagName) == nil {
		targetSet.StringSliceVar(&values.Roots, flagName, values.Roots, flagUsage)
	}

	if definition.Persistent {
		if command.Flags().Lookup(flagName) == nil {
			if persistentFlag := targetSet.Lookup(flagName); persistentFlag != nil {
				command.Flags().AddFlag(persistentFlag)
			}
		}
	}
	return &values
}

// EnsureRemoteFlag guarantees the shared remote flag is available on the command.
func EnsureRemoteFlag(command *cobra.Command, defaultValue string, usage string) {
	if command == nil {
		return
	}

	persistentSet := command.PersistentFlags()
	if persistentSet.Lookup(RemoteFlagName) == nil {
		persistentSet.String(RemoteFlagName, defaultValue, usage)
	}

	if command.Flags().Lookup(RemoteFlagName) == nil {
		if remoteFlag := persistentSet.Lookup(RemoteFlagName); remoteFlag != nil {
			command.Flags().AddFlag(remoteFlag)
		}
	}
}
