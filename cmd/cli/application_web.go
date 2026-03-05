package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/tyemirov/gix/internal/execshell"
	reposdeps "github.com/tyemirov/gix/internal/repos/dependencies"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/web"
)

const (
	webInterfaceUnavailableMessageConstant    = "web mode is unavailable from the web interface"
	gitForEachRefSubcommandConstant           = "for-each-ref"
	gitBranchCatalogFormatArgumentConstant    = "--format=%(HEAD)|%(refname:short)|%(upstream:short)"
	gitBranchCatalogReferenceArgumentConstant = "refs/heads/"
)

type webRunner func(context.Context, web.ServerOptions) error

type webLaunchConfiguration struct {
	port int
}

func newWebLaunchConfiguration(rawValue string) (webLaunchConfiguration, error) {
	trimmedValue := strings.TrimSpace(rawValue)
	if len(trimmedValue) == 0 {
		trimmedValue = webDefaultPortConstant
	}

	portValue, parseError := strconv.Atoi(trimmedValue)
	if parseError != nil {
		return webLaunchConfiguration{}, fmt.Errorf(webPortInvalidTemplateConstant, rawValue)
	}
	if portValue < 1 || portValue > 65535 {
		return webLaunchConfiguration{}, fmt.Errorf(webPortRangeTemplateConstant, portValue)
	}

	return webLaunchConfiguration{port: portValue}, nil
}

func (configuration webLaunchConfiguration) listenAddress() string {
	return fmt.Sprintf(webAddressTemplateConstant, webListenHostConstant, configuration.port)
}

func (configuration webLaunchConfiguration) launchURL() string {
	return fmt.Sprintf(webLaunchURLTemplateConstant, webListenHostConstant, configuration.port)
}

func (application *Application) handleWebLaunch(command *cobra.Command) (bool, error) {
	launchConfiguration, requested, configurationError := application.resolveWebLaunchConfiguration(command)
	if configurationError != nil {
		return true, configurationError
	}
	if !requested {
		return false, nil
	}

	outputWriter := io.Writer(nil)
	executionContext := context.Background()
	if command != nil {
		outputWriter = command.OutOrStdout()
		executionContext = command.Context()
	}
	if executionContext == nil {
		executionContext = context.Background()
	}
	if outputWriter != nil {
		if _, writeError := fmt.Fprintf(outputWriter, webLaunchMessageTemplateConstant, launchConfiguration.launchURL()); writeError != nil {
			return true, writeError
		}
	}

	return true, application.webRunner(executionContext, web.ServerOptions{
		Address:  launchConfiguration.listenAddress(),
		Branches: application.branchCatalog(executionContext),
		Catalog:  application.commandCatalog(),
		Execute:  application.newWebCommandExecutor(),
	})
}

func (application *Application) resolveWebLaunchConfiguration(command *cobra.Command) (webLaunchConfiguration, bool, error) {
	if command == nil {
		return webLaunchConfiguration{}, false, nil
	}

	rawValue, changed, flagError := flagutils.StringFlag(command, webFlagNameConstant)
	switch {
	case flagError == nil:
	case errors.Is(flagError, flagutils.ErrFlagNotDefined):
		return webLaunchConfiguration{}, false, nil
	default:
		return webLaunchConfiguration{}, false, flagError
	}

	if !changed && len(strings.TrimSpace(rawValue)) == 0 {
		return webLaunchConfiguration{}, false, nil
	}

	launchConfiguration, configurationError := newWebLaunchConfiguration(rawValue)
	if configurationError != nil {
		return webLaunchConfiguration{}, true, configurationError
	}
	return launchConfiguration, true, nil
}

func (application *Application) launchWebInterface(executionContext context.Context, options web.ServerOptions) error {
	return web.Run(executionContext, options)
}

func (application *Application) newWebCommandExecutor() web.CommandExecutor {
	return func(executionContext context.Context, arguments []string, standardInput io.Reader, standardOutput io.Writer, standardError io.Writer) error {
		nestedApplication := NewApplication()
		nestedApplication.versionResolver = application.versionResolver
		nestedApplication.webRunner = func(context.Context, web.ServerOptions) error {
			return errors.New(webInterfaceUnavailableMessageConstant)
		}

		return nestedApplication.ExecuteWithOptions(ExecutionOptions{
			Arguments:      application.webInheritedArguments(arguments),
			Context:        executionContext,
			StandardInput:  standardInput,
			StandardOutput: standardOutput,
			StandardError:  standardError,
			ExitOnVersion:  false,
		})
	}
}

func (application *Application) webInheritedArguments(arguments []string) []string {
	inheritedArguments := make([]string, 0, len(arguments)+6)

	configurationFilePath := strings.TrimSpace(application.configurationMetadata.ConfigFileUsed)
	if len(configurationFilePath) > 0 && !commandArgumentsContainFlag(arguments, configFileFlagNameConstant) {
		inheritedArguments = append(inheritedArguments, "--"+configFileFlagNameConstant, configurationFilePath)
	}

	logLevelValue := strings.TrimSpace(application.logLevelFlagValue)
	if len(logLevelValue) > 0 && !commandArgumentsContainFlag(arguments, logLevelFlagNameConstant) {
		inheritedArguments = append(inheritedArguments, "--"+logLevelFlagNameConstant, logLevelValue)
	}

	logFormatValue := strings.TrimSpace(application.logFormatFlagValue)
	if len(logFormatValue) > 0 && !commandArgumentsContainFlag(arguments, logFormatFlagNameConstant) {
		inheritedArguments = append(inheritedArguments, "--"+logFormatFlagNameConstant, logFormatValue)
	}

	return append(inheritedArguments, arguments...)
}

func commandArgumentsContainFlag(arguments []string, flagName string) bool {
	if len(flagName) == 0 {
		return false
	}

	fullName := "--" + flagName
	prefixedName := fullName + "="
	for _, argument := range arguments {
		if argument == fullName || strings.HasPrefix(argument, prefixedName) {
			return true
		}
	}

	return false
}

func (application *Application) branchCatalog(executionContext context.Context) web.BranchCatalog {
	workingDirectory, workingDirectoryError := os.Getwd()
	if workingDirectoryError != nil {
		return web.BranchCatalog{Error: workingDirectoryError.Error()}
	}

	gitExecutor, executorError := reposdeps.ResolveGitExecutor(nil, application.logger, application.humanReadableLoggingEnabled())
	if executorError != nil {
		return web.BranchCatalog{
			RepositoryPath: workingDirectory,
			Error:          executorError.Error(),
		}
	}

	branchResult, branchError := gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitForEachRefSubcommandConstant, gitBranchCatalogFormatArgumentConstant, gitBranchCatalogReferenceArgumentConstant},
		WorkingDirectory: workingDirectory,
	})
	if branchError != nil {
		return web.BranchCatalog{
			RepositoryPath: workingDirectory,
			Error:          branchError.Error(),
		}
	}

	return web.BranchCatalog{
		RepositoryPath: workingDirectory,
		Branches:       parseBranchCatalog(branchResult.StandardOutput),
	}
}

func parseBranchCatalog(rawOutput string) []web.BranchDescriptor {
	trimmedOutput := strings.TrimSpace(rawOutput)
	if len(trimmedOutput) == 0 {
		return nil
	}

	branchDescriptors := make([]web.BranchDescriptor, 0)
	for _, line := range strings.Split(trimmedOutput, "\n") {
		trimmedLine := strings.TrimSpace(line)
		if len(trimmedLine) == 0 {
			continue
		}

		parts := strings.SplitN(trimmedLine, "|", 3)
		if len(parts) < 2 {
			continue
		}

		branchName := strings.TrimSpace(parts[1])
		if len(branchName) == 0 {
			continue
		}

		branchDescriptor := web.BranchDescriptor{
			Name:    branchName,
			Current: strings.TrimSpace(parts[0]) == "*",
		}
		if len(parts) == 3 {
			branchDescriptor.Upstream = strings.TrimSpace(parts[2])
		}

		branchDescriptors = append(branchDescriptors, branchDescriptor)
	}

	sort.Slice(branchDescriptors, func(leftIndex int, rightIndex int) bool {
		leftBranch := branchDescriptors[leftIndex]
		rightBranch := branchDescriptors[rightIndex]
		if leftBranch.Current != rightBranch.Current {
			return leftBranch.Current
		}
		return leftBranch.Name < rightBranch.Name
	})

	return branchDescriptors
}

func (application *Application) commandCatalog() web.CommandCatalog {
	commandDescriptors := collectCommandCatalogEntries(application.rootCommand)
	return web.CommandCatalog{
		Application: applicationNameConstant,
		Commands:    commandDescriptors,
	}
}

func collectCommandCatalogEntries(rootCommand *cobra.Command) []web.CommandDescriptor {
	if rootCommand == nil {
		return nil
	}

	commandDescriptors := make([]web.CommandDescriptor, 0)
	var collectCommands func(*cobra.Command)
	collectCommands = func(command *cobra.Command) {
		if command == nil || command.Hidden {
			return
		}

		commandDescriptors = append(commandDescriptors, buildCommandDescriptor(command))

		subcommands := visibleSubcommands(command)
		for _, subcommand := range subcommands {
			collectCommands(subcommand)
		}
	}
	collectCommands(rootCommand)

	sort.Slice(commandDescriptors, func(leftIndex int, rightIndex int) bool {
		return commandDescriptors[leftIndex].Path < commandDescriptors[rightIndex].Path
	})

	return commandDescriptors
}

func buildCommandDescriptor(command *cobra.Command) web.CommandDescriptor {
	visibleSubcommandNames := make([]string, 0)
	for _, subcommand := range visibleSubcommands(command) {
		visibleSubcommandNames = append(visibleSubcommandNames, strings.TrimSpace(subcommand.CommandPath()))
	}

	return web.CommandDescriptor{
		Path:        strings.TrimSpace(command.CommandPath()),
		Use:         strings.TrimSpace(command.UseLine()),
		Name:        strings.TrimSpace(command.Name()),
		Short:       strings.TrimSpace(command.Short),
		Long:        strings.TrimSpace(command.Long),
		Example:     strings.TrimSpace(command.Example),
		Aliases:     sortedStrings(command.Aliases),
		Runnable:    command.Runnable(),
		Flags:       collectFlagDescriptors(command),
		Subcommands: visibleSubcommandNames,
	}
}

func visibleSubcommands(command *cobra.Command) []*cobra.Command {
	if command == nil {
		return nil
	}

	visibleCommands := make([]*cobra.Command, 0)
	for _, subcommand := range command.Commands() {
		if subcommand == nil || subcommand.Hidden {
			continue
		}
		visibleCommands = append(visibleCommands, subcommand)
	}

	sort.Slice(visibleCommands, func(leftIndex int, rightIndex int) bool {
		return visibleCommands[leftIndex].CommandPath() < visibleCommands[rightIndex].CommandPath()
	})

	return visibleCommands
}

func collectFlagDescriptors(command *cobra.Command) []web.FlagDescriptor {
	if command == nil {
		return nil
	}

	flagDescriptorsByName := map[string]web.FlagDescriptor{}
	collectFromFlagSet := func(flagSet *pflag.FlagSet) {
		if flagSet == nil {
			return
		}
		flagSet.VisitAll(func(flag *pflag.Flag) {
			if flag == nil || flag.Hidden {
				return
			}
			flagDescriptorsByName[flag.Name] = web.FlagDescriptor{
				Name:         flag.Name,
				Shorthand:    flag.Shorthand,
				Usage:        strings.TrimSpace(flag.Usage),
				Type:         flag.Value.Type(),
				Default:      strings.TrimSpace(flag.DefValue),
				NoOptDefault: strings.TrimSpace(flag.NoOptDefVal),
				Required:     flagIsRequired(flag),
			}
		})
	}

	collectFromFlagSet(command.NonInheritedFlags())
	collectFromFlagSet(command.InheritedFlags())

	flagDescriptors := make([]web.FlagDescriptor, 0, len(flagDescriptorsByName))
	for _, flagDescriptor := range flagDescriptorsByName {
		flagDescriptors = append(flagDescriptors, flagDescriptor)
	}

	sort.Slice(flagDescriptors, func(leftIndex int, rightIndex int) bool {
		return flagDescriptors[leftIndex].Name < flagDescriptors[rightIndex].Name
	})

	return flagDescriptors
}

func flagIsRequired(flag *pflag.Flag) bool {
	if flag == nil {
		return false
	}
	if flag.Annotations == nil {
		return false
	}
	_, required := flag.Annotations[cobra.BashCompOneRequiredFlag]
	return required
}

func sortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	sanitizedValues := make([]string, 0, len(values))
	for _, value := range values {
		trimmedValue := strings.TrimSpace(value)
		if len(trimmedValue) == 0 {
			continue
		}
		sanitizedValues = append(sanitizedValues, trimmedValue)
	}

	sort.Strings(sanitizedValues)
	return sanitizedValues
}
