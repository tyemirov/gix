package flags

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/tyemirov/gix/internal/utils"
)

const boolFlagParseErrorTemplate = "unable to parse flag %q: %w"

// ErrFlagNotDefined indicates that the requested flag is not present on the command.
var ErrFlagNotDefined = errors.New("flag not defined")

func BoolFlag(command *cobra.Command, name string) (bool, bool, error) {
	flagSet, flag := locateFlag(command, name)
	if flag == nil {
		return false, false, ErrFlagNotDefined
	}
	value, err := flagSet.GetBool(name)
	if err == nil {
		return value, flag.Changed, nil
	}

	if flag.Value == nil {
		return false, false, err
	}

	parsedValue, parseError := parseToggleValue(flag.Value.String())
	if parseError != nil {
		return false, false, fmt.Errorf(boolFlagParseErrorTemplate, name, parseError)
	}

	return parsedValue, flag.Changed, nil
}

func StringFlag(command *cobra.Command, name string) (string, bool, error) {
	flagSet, flag := locateFlag(command, name)
	if flag == nil {
		return "", false, ErrFlagNotDefined
	}
	value, err := flagSet.GetString(name)
	if err != nil {
		return "", false, err
	}
	return value, flag.Changed, nil
}

func StringSliceFlag(command *cobra.Command, name string) ([]string, bool, error) {
	flagSet, flag := locateFlag(command, name)
	if flag == nil {
		return nil, false, ErrFlagNotDefined
	}
	values, err := flagSet.GetStringSlice(name)
	if err != nil {
		return nil, false, err
	}
	return values, flag.Changed, nil
}

func locateFlag(command *cobra.Command, name string) (*pflag.FlagSet, *pflag.Flag) {
	if command == nil {
		return nil, nil
	}

	candidateSets := []*pflag.FlagSet{
		command.Flags(),
		command.PersistentFlags(),
		command.InheritedFlags(),
	}

	if root := command.Root(); root != nil {
		candidateSets = append(candidateSets, root.PersistentFlags())
	}

	for _, set := range candidateSets {
		if set == nil {
			continue
		}
		if flag := set.Lookup(name); flag != nil {
			return set, flag
		}
	}

	return nil, nil
}

// CollectExecutionFlags inspects the command's flags to produce execution flag values.
func CollectExecutionFlags(command *cobra.Command) utils.ExecutionFlags {
	executionFlags := utils.ExecutionFlags{}
	if command == nil {
		return executionFlags
	}

	if dryRunValue, dryRunChanged, dryRunError := BoolFlag(command, DryRunFlagName); dryRunError == nil {
		executionFlags.DryRun = dryRunValue
		executionFlags.DryRunSet = dryRunChanged
	}

	if assumeYesValue, assumeYesChanged, assumeYesError := BoolFlag(command, AssumeYesFlagName); assumeYesError == nil {
		executionFlags.AssumeYes = assumeYesValue
		executionFlags.AssumeYesSet = assumeYesChanged
	}

	if remoteValue, remoteChanged, remoteError := StringFlag(command, RemoteFlagName); remoteError == nil {
		executionFlags.Remote = strings.TrimSpace(remoteValue)
		executionFlags.RemoteSet = remoteChanged
	}

	return executionFlags
}

// ResolveExecutionFlags returns execution flags from context or flag values, indicating whether any overrides are provided.
func ResolveExecutionFlags(command *cobra.Command) (utils.ExecutionFlags, bool) {
	contextAccessor := utils.NewCommandContextAccessor()
	if command != nil {
		if flags, available := contextAccessor.ExecutionFlags(command.Context()); available {
			return flags, true
		}
	}

	executionFlags := CollectExecutionFlags(command)
	available := executionFlags.DryRunSet || executionFlags.AssumeYesSet || executionFlags.RemoteSet
	return executionFlags, available
}
