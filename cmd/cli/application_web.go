package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/tyemirov/gix/internal/execshell"
	reposdeps "github.com/tyemirov/gix/internal/repos/dependencies"
	"github.com/tyemirov/gix/internal/repos/shared"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/web"
)

const (
	webInterfaceUnavailableMessageConstant      = "web mode is unavailable from the web interface"
	webLaunchModeCurrentRepositoryConstant      = "current_repo"
	webLaunchModeDiscoveredRepositoriesConstant = "discovered_repositories"
	webNoRepositoriesErrorTemplateConstant      = "no Git repositories found beneath %s"
	webRepositoryIDTemplateConstant             = "repo-%03d"
	webRepositoryPathRequiredErrorConstant      = "repository path is required"
	webHelpCommandPathConstant                  = applicationNameConstant + " help"
	webCompletionCommandPathConstant            = applicationNameConstant + " completion"
	webCompletionBashCommandPathConstant        = webCompletionCommandPathConstant + " bash"
	webCompletionFishCommandPathConstant        = webCompletionCommandPathConstant + " fish"
	webCompletionPowershellCommandPathConstant  = webCompletionCommandPathConstant + " powershell"
	webCompletionZshCommandPathConstant         = webCompletionCommandPathConstant + " zsh"
	gitForEachRefSubcommandConstant             = "for-each-ref"
	gitBranchCatalogFormatArgumentConstant      = "--format=%(HEAD)|%(refname:short)|%(upstream:short)"
	gitBranchCatalogReferenceArgumentConstant   = "refs/heads/"
	gitRevParseSubcommandConstant               = "rev-parse"
	gitShowTopLevelArgumentConstant             = "--show-toplevel"
	gitSymbolicRefSubcommandConstant            = "symbolic-ref"
	gitSymbolicRefQuietArgumentConstant         = "--quiet"
	gitShortArgumentConstant                    = "--short"
	gitOriginHeadReferenceConstant              = "refs/remotes/origin/HEAD"
	gitOriginPrefixConstant                     = "origin/"
)

var nonActionableCommandPaths = map[string]struct{}{
	applicationNameConstant: {},
	applicationNameConstant + " " + repoFolderNamespaceUseNameConstant:                                              {},
	applicationNameConstant + " " + repoRemoteNamespaceUseNameConstant:                                              {},
	applicationNameConstant + " " + repoPullRequestsNamespaceUseNameConstant:                                        {},
	applicationNameConstant + " " + repoPackagesNamespaceUseNameConstant:                                            {},
	applicationNameConstant + " " + repoFilesNamespaceUseNameConstant:                                               {},
	applicationNameConstant + " " + messageNamespaceUseNameConstant:                                                 {},
	applicationNameConstant + " " + workflowCommandOperationNameConstant:                                            {},
	applicationNameConstant + " " + repoRemoteNamespaceUseNameConstant + " " + updateProtocolCommandUseNameConstant: {},
	applicationNameConstant + " " + repoFilesNamespaceUseNameConstant + " " + filesReplaceCommandUseNameConstant:    {},
	applicationNameConstant + " " + repoFilesNamespaceUseNameConstant + " " + filesAddCommandUseNameConstant:        {},
	applicationNameConstant + " " + repoFilesNamespaceUseNameConstant + " " + removeCommandUseNameConstant:          {},
	applicationNameConstant + " " + repoReleaseCommandUseNameConstant + " " + releaseRetagCommandUseNameConstant:    {},
	applicationNameConstant + " " + messageNamespaceUseNameConstant + " " + commitMessageUseNameConstant:            {},
	applicationNameConstant + " " + messageNamespaceUseNameConstant + " " + changelogMessageUseNameConstant:         {},
	applicationNameConstant + " " + repoReleaseCommandUseNameConstant:                                               {},
	webHelpCommandPathConstant:                 {},
	webCompletionCommandPathConstant:           {},
	webCompletionBashCommandPathConstant:       {},
	webCompletionFishCommandPathConstant:       {},
	webCompletionPowershellCommandPathConstant: {},
	webCompletionZshCommandPathConstant:        {},
}

type webRunner func(context.Context, web.ServerOptions) error

type execshellGitExecutor = shared.GitExecutor
type webGitRepositoryManager = shared.GitRepositoryManager

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

	repositoryCatalog := application.repositoryCatalog(executionContext)

	return true, application.webRunner(executionContext, web.ServerOptions{
		Address:      launchConfiguration.listenAddress(),
		Repositories: repositoryCatalog,
		Catalog:      application.commandCatalog(),
		LoadBranches: application.loadRepositoryBranches,
		Execute:      application.newWebCommandExecutor(),
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

func (application *Application) repositoryCatalog(executionContext context.Context) web.RepositoryCatalog {
	workingDirectory, workingDirectoryError := os.Getwd()
	if workingDirectoryError != nil {
		return web.RepositoryCatalog{Error: workingDirectoryError.Error()}
	}

	gitExecutor, repositoryManager, dependencyError := application.webGitDependencies()
	if dependencyError != nil {
		return web.RepositoryCatalog{
			LaunchPath: workingDirectory,
			Error:      dependencyError.Error(),
		}
	}

	currentRepositoryRoot, currentRepositoryAvailable := application.currentRepositoryRoot(executionContext, gitExecutor, workingDirectory)
	if currentRepositoryAvailable {
		repositoryDescriptor := application.inspectRepository(executionContext, currentRepositoryRoot, 1, true, gitExecutor, repositoryManager)
		return web.RepositoryCatalog{
			LaunchPath:           workingDirectory,
			LaunchMode:           webLaunchModeCurrentRepositoryConstant,
			SelectedRepositoryID: repositoryDescriptor.ID,
			Repositories:         []web.RepositoryDescriptor{repositoryDescriptor},
		}
	}

	discoverer := reposdeps.ResolveRepositoryDiscoverer(nil)
	discoveredRepositories, discoverError := discoverer.DiscoverRepositories([]string{workingDirectory})
	if discoverError != nil {
		return web.RepositoryCatalog{
			LaunchPath: workingDirectory,
			Error:      discoverError.Error(),
		}
	}

	if len(discoveredRepositories) == 0 {
		return web.RepositoryCatalog{
			LaunchPath: workingDirectory,
			LaunchMode: webLaunchModeDiscoveredRepositoriesConstant,
			Error:      fmt.Sprintf(webNoRepositoriesErrorTemplateConstant, workingDirectory),
		}
	}

	repositoryDescriptors := make([]web.RepositoryDescriptor, 0, len(discoveredRepositories))
	for repositoryIndex, repositoryPath := range discoveredRepositories {
		repositoryDescriptors = append(
			repositoryDescriptors,
			application.inspectRepository(executionContext, repositoryPath, repositoryIndex+1, false, gitExecutor, repositoryManager),
		)
	}

	selectedRepositoryID := ""
	if len(repositoryDescriptors) > 0 {
		selectedRepositoryID = repositoryDescriptors[0].ID
	}

	return web.RepositoryCatalog{
		LaunchPath:           workingDirectory,
		LaunchMode:           webLaunchModeDiscoveredRepositoriesConstant,
		SelectedRepositoryID: selectedRepositoryID,
		Repositories:         repositoryDescriptors,
	}
}

func (application *Application) loadRepositoryBranches(executionContext context.Context, repository web.RepositoryDescriptor) web.BranchCatalog {
	gitExecutor, _, dependencyError := application.webGitDependencies()
	if dependencyError != nil {
		return web.BranchCatalog{
			RepositoryID:   repository.ID,
			RepositoryPath: repository.Path,
			Error:          dependencyError.Error(),
		}
	}

	return loadRepositoryBranches(executionContext, repository, gitExecutor)
}

func loadRepositoryBranches(executionContext context.Context, repository web.RepositoryDescriptor, gitExecutor execshellGitExecutor) web.BranchCatalog {
	trimmedRepositoryPath := strings.TrimSpace(repository.Path)
	if len(trimmedRepositoryPath) == 0 {
		return web.BranchCatalog{
			RepositoryID: repository.ID,
			Error:        webRepositoryPathRequiredErrorConstant,
		}
	}

	branchResult, branchError := gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitForEachRefSubcommandConstant, gitBranchCatalogFormatArgumentConstant, gitBranchCatalogReferenceArgumentConstant},
		WorkingDirectory: trimmedRepositoryPath,
	})
	if branchError != nil {
		return web.BranchCatalog{
			RepositoryID:   repository.ID,
			RepositoryPath: trimmedRepositoryPath,
			Error:          branchError.Error(),
		}
	}

	return web.BranchCatalog{
		RepositoryID:   repository.ID,
		RepositoryPath: trimmedRepositoryPath,
		Branches:       parseBranchCatalog(branchResult.StandardOutput),
	}
}

func (application *Application) webGitDependencies() (execshellGitExecutor, webGitRepositoryManager, error) {
	gitExecutor, executorError := reposdeps.ResolveGitExecutor(nil, application.logger, application.humanReadableLoggingEnabled())
	if executorError != nil {
		return nil, nil, executorError
	}

	repositoryManager, managerError := reposdeps.ResolveGitRepositoryManager(nil, gitExecutor)
	if managerError != nil {
		return nil, nil, managerError
	}

	return gitExecutor, repositoryManager, nil
}

func (application *Application) currentRepositoryRoot(executionContext context.Context, gitExecutor execshellGitExecutor, workingDirectory string) (string, bool) {
	repositoryRootResult, repositoryRootError := gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitRevParseSubcommandConstant, gitShowTopLevelArgumentConstant},
		WorkingDirectory: workingDirectory,
	})
	if repositoryRootError != nil {
		return "", false
	}

	trimmedRepositoryRoot := strings.TrimSpace(repositoryRootResult.StandardOutput)
	if len(trimmedRepositoryRoot) == 0 {
		return "", false
	}

	return trimmedRepositoryRoot, true
}

func (application *Application) inspectRepository(
	executionContext context.Context,
	repositoryPath string,
	repositoryIndex int,
	contextCurrent bool,
	gitExecutor execshellGitExecutor,
	repositoryManager webGitRepositoryManager,
) web.RepositoryDescriptor {
	descriptor := web.RepositoryDescriptor{
		ID:             fmt.Sprintf(webRepositoryIDTemplateConstant, repositoryIndex),
		Name:           filepath.Base(strings.TrimSpace(repositoryPath)),
		Path:           strings.TrimSpace(repositoryPath),
		ContextCurrent: contextCurrent,
	}

	currentBranch, currentBranchError := repositoryManager.GetCurrentBranch(executionContext, descriptor.Path)
	if currentBranchError == nil {
		descriptor.CurrentBranch = strings.TrimSpace(currentBranch)
	} else {
		descriptor.Error = currentBranchError.Error()
	}

	cleanWorktree, cleanWorktreeError := repositoryManager.CheckCleanWorktree(executionContext, descriptor.Path)
	if cleanWorktreeError == nil {
		descriptor.Dirty = !cleanWorktree
	} else if len(descriptor.Error) == 0 {
		descriptor.Error = cleanWorktreeError.Error()
	}

	defaultBranch, defaultBranchError := resolveRepositoryDefaultBranch(executionContext, gitExecutor, descriptor.Path)
	if defaultBranchError == nil {
		descriptor.DefaultBranch = defaultBranch
	} else if len(descriptor.Error) == 0 {
		descriptor.Error = defaultBranchError.Error()
	}

	return descriptor
}

func resolveRepositoryDefaultBranch(executionContext context.Context, gitExecutor execshellGitExecutor, repositoryPath string) (string, error) {
	defaultBranchResult, defaultBranchError := gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitSymbolicRefSubcommandConstant, gitSymbolicRefQuietArgumentConstant, gitShortArgumentConstant, gitOriginHeadReferenceConstant},
		WorkingDirectory: repositoryPath,
	})
	if defaultBranchError != nil {
		return "", defaultBranchError
	}

	trimmedDefaultBranch := strings.TrimSpace(defaultBranchResult.StandardOutput)
	trimmedDefaultBranch = strings.TrimPrefix(trimmedDefaultBranch, gitOriginPrefixConstant)

	return trimmedDefaultBranch, nil
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
		Actionable:  commandIsActionable(command),
		Flags:       collectFlagDescriptors(command),
		Subcommands: visibleSubcommandNames,
	}
}

func commandIsActionable(command *cobra.Command) bool {
	if command == nil || !command.Runnable() {
		return false
	}

	commandPath := strings.TrimSpace(command.CommandPath())
	if _, excluded := nonActionableCommandPaths[commandPath]; excluded {
		return false
	}

	return commandSupportsBareExecution(command)
}

func commandSupportsBareExecution(command *cobra.Command) bool {
	if command == nil {
		return false
	}

	if validationError := command.ValidateArgs([]string{}); validationError != nil {
		return false
	}

	if validationError := command.ValidateRequiredFlags(); validationError != nil {
		return false
	}

	if validationError := command.ValidateFlagGroups(); validationError != nil {
		return false
	}

	return true
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
