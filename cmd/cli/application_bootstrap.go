package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/zap"

	changelogcmd "github.com/tyemirov/gix/v5/cmd/cli/changelog"
	commitcmd "github.com/tyemirov/gix/v5/cmd/cli/commit"
	"github.com/tyemirov/gix/v5/cmd/cli/repos"
	releasecmd "github.com/tyemirov/gix/v5/cmd/cli/repos/release"
	semvercmd "github.com/tyemirov/gix/v5/cmd/cli/semver"
	workflowcmd "github.com/tyemirov/gix/v5/cmd/cli/workflow"
	"github.com/tyemirov/gix/v5/internal/audit"
	"github.com/tyemirov/gix/v5/internal/branches"
	syncflowcmd "github.com/tyemirov/gix/v5/internal/branches/syncflow"
	"github.com/tyemirov/gix/v5/internal/githubauth"
	"github.com/tyemirov/gix/v5/internal/llmclient"
	"github.com/tyemirov/gix/v5/internal/migrate"
	"github.com/tyemirov/gix/v5/internal/packages"
	reposdeps "github.com/tyemirov/gix/v5/internal/repos/dependencies"
	"github.com/tyemirov/gix/v5/internal/utils"
	flagutils "github.com/tyemirov/gix/v5/internal/utils/flags"
	"github.com/tyemirov/gix/v5/internal/version"
	workflowpkg "github.com/tyemirov/gix/v5/internal/workflow"
	"github.com/tyemirov/utils/llm"
)

var commandOperationRequirements = map[string][]string{
	auditOperationNameConstant:           {auditOperationNameConstant},
	packagesDeleteCommandPathKeyConstant: {packagesDeleteOperationNameConstant},
	pullRequestsDeleteCommandPathKeyConstant: {
		branchCleanupOperationNameConstant,
	},
	folderRenameCommandPathKeyConstant:    {reposRenameOperationNameConstant},
	remoteCanonicalCommandPathKeyConstant: {reposRemotesOperationNameConstant},
	remoteProtocolCommandPathKeyConstant:  {reposProtocolOperationNameConstant},
	repoReleaseCommandUseNameConstant:     {repoReleaseOperationNameConstant},
	releaseRetagCommandPathKeyConstant:    {repoReleaseOperationNameConstant},
	filesRemoveCommandPathKeyConstant:     {repoHistoryOperationNameConstant},
	filesReplaceCommandPathKeyConstant:    {repoFilesReplaceOperationNameConstant},
	filesAddCommandPathKeyConstant:        {repoFilesAddOperationNameConstant},
	workflowCommandOperationNameConstant:  {workflowCommandOperationNameConstant},
	defaultCommandUseNameConstant:         {defaultOperationNameConstant},
	branchSyncTopLevelUseNameConstant:     {branchSyncOperationNameConstant},
	commitMessageCommandPathKeyConstant:   {commitMessageOperationNameConstant},
	changelogMessageCommandPathKeyConstant: {
		changelogMessageOperationNameConstant,
	},
	semverDecisionCommandPathKeyConstant: {
		changelogMessageOperationNameConstant,
	},
}

var requiredOperationConfigurationNames = collectRequiredOperationConfigurationNames()

var systemConfigurationFilePath = "/etc/gix/config.yml"

type loggerOutputsFactory interface {
	CreateLoggerOutputs(utils.LogLevel, utils.LogFormat) (utils.LoggerOutputs, error)
}

// Application wires the Cobra root command, configuration loader, and structured logger.
type Application struct {
	rootCommand                       *cobra.Command
	configurationLoader               *utils.ConfigurationLoader
	loggerFactory                     loggerOutputsFactory
	logger                            *zap.Logger
	consoleLogger                     *zap.Logger
	configuration                     ApplicationConfiguration
	configurationMetadata             utils.LoadedConfiguration
	configurationFilePath             string
	logLevelFlagValue                 string
	logFormatFlagValue                string
	commandContextAccessor            utils.CommandContextAccessor
	operationConfigurations           OperationConfigurations
	configuredOperationConfigurations OperationConfigurations
	rootFlagValues                    *flagutils.RootFlagValues
	configurationInitializationForced bool
	versionFlag                       bool
	webFlagValue                      bool
	webBindFlagValue                  string
	webPortFlagValue                  string
	versionResolver                   func(context.Context) string
	versionExitEnabled                bool
	exitFunction                      func(int)
	webRunner                         webRunner
	llmClientFactory                  llmclient.ClientFactory
}

// NewApplication assembles a fully wired CLI application instance.
func NewApplication() *Application {
	application := &Application{
		loggerFactory:          utils.NewLoggerFactory(),
		logger:                 zap.NewNop(),
		consoleLogger:          zap.NewNop(),
		commandContextAccessor: utils.NewCommandContextAccessor(),
	}
	application.versionResolver = application.resolveVersion
	application.versionExitEnabled = true
	application.exitFunction = os.Exit
	application.webRunner = application.launchWebInterface
	application.llmClientFactory = func(configuration llmclient.Config) (llm.ChatClient, error) {
		return llmclient.NewFactory(configuration)
	}

	application.configurationLoader = utils.NewConfigurationLoader()

	cobraCommand := &cobra.Command{
		Use:           applicationNameConstant,
		Short:         applicationShortDescriptionConstant,
		Long:          applicationLongDescriptionConstant,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(command *cobra.Command, arguments []string) error {
			if command != nil && command.Name() == configurationInitializationFlagNameConstant {
				return nil
			}
			if initializationError := application.initializeConfiguration(command); initializationError != nil {
				return initializationError
			}

			versionRequested := application.versionFlag
			if command != nil {
				if flagValue, flagChanged, flagError := flagutils.BoolFlag(command, versionFlagNameConstant); flagError == nil && flagChanged {
					versionRequested = flagValue
				}
			}

			if versionRequested {
				if versionError := application.printVersion(command.Context(), command.OutOrStdout()); versionError != nil {
					return versionError
				}
				if application.versionExitEnabled {
					application.exitFunction(0)
				}
				return errVersionHandled
			}

			return nil
		},
		RunE: func(command *cobra.Command, arguments []string) error {
			return application.runRootCommand(command, arguments)
		},
	}

	cobraCommand.SetContext(context.Background())
	cobraCommand.PersistentFlags().StringVar(&application.configurationFilePath, configFileFlagNameConstant, "", configFileFlagUsageConstant)
	cobraCommand.PersistentFlags().StringVar(&application.logLevelFlagValue, logLevelFlagNameConstant, "", logLevelFlagUsageConstant)
	cobraCommand.PersistentFlags().StringVar(&application.logFormatFlagValue, logFormatFlagNameConstant, "", logFormatFlagUsageConstant)
	cobraCommand.PersistentFlags().BoolVar(&application.webFlagValue, webFlagNameConstant, false, webFlagUsageConstant)
	cobraCommand.PersistentFlags().StringVar(&application.webBindFlagValue, webBindFlagNameConstant, "", webBindFlagUsageConstant)
	cobraCommand.PersistentFlags().StringVar(&application.webPortFlagValue, webPortFlagNameConstant, "", webPortFlagUsageConstant)
	application.rootFlagValues = flagutils.BindRootFlags(
		cobraCommand,
		flagutils.RootFlagValues{},
		flagutils.RootFlagDefinition{Name: flagutils.DefaultRootFlagName, Usage: flagutils.DefaultRootFlagUsage, Enabled: true, Persistent: true},
	)

	flagutils.BindExecutionFlags(
		cobraCommand,
		flagutils.ExecutionDefaults{},
		flagutils.ExecutionFlagDefinitions{
			AssumeYes: flagutils.ExecutionFlagDefinition{Name: flagutils.AssumeYesFlagName, Usage: flagutils.AssumeYesFlagUsage, Shorthand: flagutils.AssumeYesFlagShorthand, Enabled: true},
		},
	)

	cobraCommand.PersistentFlags().String(flagutils.RemoteFlagName, "", flagutils.RemoteFlagUsage)

	flagutils.AddToggleFlag(
		cobraCommand.PersistentFlags(),
		&application.versionFlag,
		versionFlagNameConstant,
		"",
		false,
		versionFlagUsageConstant,
	)

	versionCommand := &cobra.Command{
		Use:           versionCommandUseNameConstant,
		Short:         versionCommandShortDescriptionConstant,
		Long:          versionCommandLongDescriptionConstant,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(command *cobra.Command, arguments []string) error {
			return application.printVersion(command.Context(), command.OutOrStdout())
		},
	}
	cobraCommand.AddCommand(application.configurationInitializationCommand())
	cobraCommand.AddCommand(versionCommand)

	application.registerCommands(cobraCommand)

	application.rootCommand = cobraCommand

	return application
}

func (application *Application) configurationInitializationCommand() *cobra.Command {
	command := &cobra.Command{
		Use:           configurationInitializationFlagNameConstant,
		Short:         configurationInitializationCommandShortDescriptionConstant,
		Long:          configurationInitializationCommandLongDescriptionConstant,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(command *cobra.Command, arguments []string) error {
			return application.runConfigurationInitializationCommand(command)
		},
	}

	flagutils.AddToggleFlag(
		command.Flags(),
		nil,
		configurationInitializationSystemFlagNameConstant,
		"",
		false,
		configurationInitializationSystemFlagUsageConstant,
	)
	flagutils.AddToggleFlag(
		command.Flags(),
		&application.configurationInitializationForced,
		configurationInitializationForceFlagNameConstant,
		"",
		false,
		configurationInitializationForceFlagUsageConstant,
	)

	return command
}

// Execute runs the configured Cobra command hierarchy and ensures logger flushing.
func (application *Application) Execute() error {
	executionContext, stopSignalNotifications := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopSignalNotifications()

	return application.ExecuteWithOptions(ExecutionOptions{
		Arguments:      os.Args[1:],
		Context:        executionContext,
		StandardInput:  os.Stdin,
		StandardOutput: os.Stdout,
		StandardError:  os.Stderr,
		ExitOnVersion:  true,
	})
}

// Execute builds a fresh application instance and executes the root command hierarchy.
func Execute() error {
	return NewApplication().Execute()
}

func normalizeWebArguments(arguments []string) []string {
	if len(arguments) == 0 {
		return nil
	}

	normalizedArguments := make([]string, 0, len(arguments))
	flagPrefix := "--" + webFlagNameConstant
	for index := 0; index < len(arguments); index++ {
		currentArgument := arguments[index]
		normalizedArguments = append(normalizedArguments, currentArgument)
		if currentArgument != flagPrefix {
			continue
		}

		nextIndex := index + 1
		if nextIndex >= len(arguments) || strings.HasPrefix(arguments[nextIndex], "-") {
			continue
		}

		normalizedArguments = append(normalizedArguments, "--")
		normalizedArguments = append(normalizedArguments, arguments[nextIndex:]...)
		return normalizedArguments
	}
	return normalizedArguments
}

func (application *Application) initializeConfiguration(command *cobra.Command) error {
	configurationFilePath, pathError := application.resolveConfigurationFile(command)
	if pathError != nil {
		return pathError
	}
	application.configurationFilePath = configurationFilePath

	application.configuration = ApplicationConfiguration{}
	loadedConfiguration, loadError := application.configurationLoader.LoadConfiguration(application.configurationFilePath, &application.configuration)
	if loadError != nil {
		return fmt.Errorf(configurationLoadErrorTemplateConstant, loadError)
	}

	application.configurationMetadata = loadedConfiguration
	if llmConfigurationError := application.configuration.LLM.validateConnections(); llmConfigurationError != nil {
		return fmt.Errorf("invalid llm configuration: %w", llmConfigurationError)
	}
	operationConfigurations, configurationBuildError := newOperationConfigurations(application.configuration.Operations)
	if configurationBuildError != nil {
		return configurationBuildError
	}
	application.configuredOperationConfigurations = operationConfigurations.Clone()
	application.operationConfigurations = operationConfigurations

	if schemaError := application.validateOperationConfigurationSchemas(); schemaError != nil {
		return schemaError
	}
	if workflowSchemaError := application.validateWorkflowConfigurationSchema(); workflowSchemaError != nil {
		return workflowSchemaError
	}
	if validationError := application.validateOperationConfigurations(command); validationError != nil {
		return validationError
	}

	if application.persistentFlagChanged(command, logLevelFlagNameConstant) {
		application.configuration.Common.LogLevel = application.logLevelFlagValue
	}

	if application.persistentFlagChanged(command, logFormatFlagNameConstant) {
		application.configuration.Common.LogFormat = application.logFormatFlagValue
	}

	loggerOutputs, loggerCreationError := application.loggerFactory.CreateLoggerOutputs(
		utils.LogLevel(application.configuration.Common.LogLevel),
		utils.LogFormat(application.configuration.Common.LogFormat),
	)
	if loggerCreationError != nil {
		return fmt.Errorf(loggerCreationErrorTemplateConstant, loggerCreationError)
	}

	application.logger = loggerOutputs.DiagnosticLogger
	if application.logger == nil {
		application.logger = zap.NewNop()
	}

	application.consoleLogger = loggerOutputs.ConsoleLogger
	if application.consoleLogger == nil {
		application.consoleLogger = zap.NewNop()
	}

	application.logConfigurationInitialization()
	if command != nil {
		updatedContext := application.commandContextAccessor.WithConfigurationFilePath(
			command.Context(),
			application.configurationMetadata.ConfigFileUsed,
		)

		executionFlags := application.collectExecutionFlags(command)
		updatedContext = application.commandContextAccessor.WithExecutionFlags(updatedContext, executionFlags)
		updatedContext = application.commandContextAccessor.WithLogLevel(updatedContext, application.configuration.Common.LogLevel)
		updatedContext = githubauth.WithCredential(updatedContext, application.configuration.GitHub.Credential)

		updatedContext = application.commandContextAccessor.WithBranchContext(updatedContext, utils.BranchContext{RequireClean: true})

		command.SetContext(updatedContext)
		if rootCommand := command.Root(); rootCommand != nil {
			rootCommand.SetContext(updatedContext)
		}
	}

	return nil
}

func (application *Application) resolveConfigurationFile(command *cobra.Command) (string, error) {
	explicitPath := strings.TrimSpace(application.configurationFilePath)
	if explicitPath != "" {
		if filepath.Ext(explicitPath) != "."+configurationTypeConstant {
			return "", fmt.Errorf(configurationExtensionErrorTemplateConstant, explicitPath)
		}
		if _, statError := os.Stat(explicitPath); statError != nil {
			return "", fmt.Errorf(configurationLoadErrorTemplateConstant, statError)
		}
		return explicitPath, nil
	}

	if _, statError := os.Stat(systemConfigurationFilePath); statError == nil {
		return systemConfigurationFilePath, nil
	} else if !errors.Is(statError, os.ErrNotExist) {
		return "", fmt.Errorf(configurationLoadErrorTemplateConstant, statError)
	}

	userConfigurationPath, userPathError := application.userConfigurationFilePath()
	if userPathError != nil {
		return "", userPathError
	}
	if _, statError := os.Stat(userConfigurationPath); statError == nil {
		return userConfigurationPath, nil
	} else if !errors.Is(statError, os.ErrNotExist) {
		return "", fmt.Errorf(configurationLoadErrorTemplateConstant, statError)
	}

	accepted, promptError := application.offerUserConfigurationCreation(command, userConfigurationPath)
	if promptError != nil {
		return "", promptError
	}
	if !accepted {
		return "", errors.New(configurationMissingDeclinedErrorConstant)
	}
	configurationContent, _ := EmbeddedDefaultConfiguration()
	initializationPlan := configurationInitializationPlan{
		DirectoryPath: filepath.Dir(userConfigurationPath),
		FilePath:      userConfigurationPath,
	}
	if writeError := application.writeConfigurationFile(initializationPlan, configurationContent); writeError != nil {
		return "", writeError
	}
	return userConfigurationPath, nil
}

func (application *Application) userConfigurationFilePath() (string, error) {
	userHomeDirectoryPath, userHomeDirectoryError := os.UserHomeDir()
	if userHomeDirectoryError != nil {
		return "", fmt.Errorf(configurationInitializationHomeDirectoryErrorTemplateConstant, userHomeDirectoryError)
	}
	trimmedHomeDirectoryPath := strings.TrimSpace(userHomeDirectoryPath)
	if trimmedHomeDirectoryPath == "" {
		return "", fmt.Errorf(
			configurationInitializationHomeDirectoryErrorTemplateConstant,
			errors.New(configurationInitializationHomeDirectoryEmptyErrorConstant),
		)
	}
	return filepath.Join(trimmedHomeDirectoryPath, userConfigurationDirectoryNameConstant, configurationFileNameConstant), nil
}

func (application *Application) offerUserConfigurationCreation(command *cobra.Command, configurationFilePath string) (bool, error) {
	if command == nil {
		return false, errors.New(configurationMissingDeclinedErrorConstant)
	}
	if assumeYes, changed, flagError := flagutils.BoolFlag(command, flagutils.AssumeYesFlagName); flagError == nil && changed && assumeYes {
		return true, nil
	}
	if _, writeError := fmt.Fprintf(command.OutOrStdout(), configurationMissingPromptTemplateConstant, configurationFilePath); writeError != nil {
		return false, writeError
	}
	response, readError := bufio.NewReader(command.InOrStdin()).ReadString('\n')
	if readError != nil && !errors.Is(readError, io.EOF) {
		return false, readError
	}
	normalizedResponse := strings.ToLower(strings.TrimSpace(response))
	return normalizedResponse == "y" || normalizedResponse == "yes", nil
}

// InitializeForCommand prepares application state for the provided command name without executing command logic.
func (application *Application) InitializeForCommand(commandUse string) error {
	command := &cobra.Command{Use: commandUse}
	return application.initializeConfiguration(command)
}

// ConfigFileUsed returns the configuration file path used during initialization.
func (application *Application) ConfigFileUsed() string {
	return application.configurationMetadata.ConfigFileUsed
}

func (application *Application) humanReadableLoggingEnabled() bool {
	logFormatValue := strings.TrimSpace(application.configuration.Common.LogFormat)
	return strings.EqualFold(logFormatValue, string(utils.LogFormatConsole))
}

func (application *Application) logConfigurationInitialization() {
	if !strings.EqualFold(strings.TrimSpace(application.configuration.Common.LogLevel), string(utils.LogLevelDebug)) {
		return
	}

	if application.humanReadableLoggingEnabled() {
		bannerMessage := fmt.Sprintf(
			configurationInitializedConsoleTemplateConstant,
			configurationInitializedMessageConstant,
			application.configuration.Common.LogLevel,
			application.configuration.Common.LogFormat,
			application.configurationMetadata.ConfigFileUsed,
		)
		application.consoleLogger.Debug(bannerMessage)
		return
	}

	application.logger.Debug(
		configurationInitializedMessageConstant,
		zap.String(configurationLogLevelFieldConstant, application.configuration.Common.LogLevel),
		zap.String(configurationLogFormatFieldConstant, application.configuration.Common.LogFormat),
		zap.String(configurationFileFieldConstant, application.configurationMetadata.ConfigFileUsed),
	)
}

func (application *Application) collectExecutionFlags(command *cobra.Command) utils.ExecutionFlags {
	executionFlags := utils.ExecutionFlags{}
	if command == nil {
		return executionFlags
	}

	if assumeYesValue, assumeYesChanged, assumeYesError := flagutils.BoolFlag(command, flagutils.AssumeYesFlagName); assumeYesError == nil {
		executionFlags.AssumeYes = assumeYesValue
		executionFlags.AssumeYesSet = assumeYesChanged
	}

	if remoteValue, remoteChanged, remoteError := flagutils.StringFlag(command, flagutils.RemoteFlagName); remoteError == nil {
		trimmedRemote := strings.TrimSpace(remoteValue)
		executionFlags.Remote = trimmedRemote
		executionFlags.RemoteSet = remoteChanged && len(trimmedRemote) > 0
	}

	return executionFlags
}

func (application *Application) auditCommandConfiguration() audit.CommandConfiguration {
	var configuration audit.CommandConfiguration
	application.decodeOperationConfiguration(auditOperationNameConstant, &configuration)
	if strings.EqualFold(application.configuration.Common.LogLevel, string(utils.LogLevelDebug)) {
		configuration.Debug = true
	}
	return configuration
}

func (application *Application) packagesConfiguration() packages.Configuration {
	configuration := packages.DefaultConfiguration()
	application.decodeOperationConfiguration(packagesDeleteOperationNameConstant, &configuration.Delete)

	return configuration
}

func (application *Application) branchCleanupConfiguration() branches.CommandConfiguration {
	configuration := branches.DefaultCommandConfiguration()
	application.decodeOperationConfiguration(branchCleanupOperationNameConstant, &configuration)

	options, optionsExist := application.lookupOperationOptions(branchCleanupOperationNameConstant)
	if !optionsExist || !optionExists(options, assumeYesOptionKeyConstant) {
		configuration.AssumeYes = application.configuration.Common.AssumeYes
	}

	return configuration
}

func (application *Application) branchSyncConfiguration() syncflowcmd.CommandConfiguration {
	configuration := syncflowcmd.DefaultCommandConfiguration()
	configuration.CommitMessage.Effort = application.configuration.LLM.Effort
	configuration.CommitMessage.TimeoutSeconds = application.configuration.LLM.TimeoutSeconds
	configuration.CommitMessage.ConnectionProfiles = application.configuration.LLM.connectionProfiles()
	application.decodeBranchSyncOperationConfiguration(branchSyncOperationNameConstant, &configuration)

	options, optionsExist := application.lookupOperationOptions(branchSyncOperationNameConstant)
	if !optionsExist || !optionExists(options, requireCleanOptionKeyConstant) {
		configuration.RequireClean = application.configuration.Common.RequireClean
	}

	return configuration.Sanitize()
}

func (application *Application) repoReleaseConfiguration() releasecmd.CommandConfiguration {
	configuration := releasecmd.DefaultCommandConfiguration()
	application.decodeOperationConfiguration(repoReleaseOperationNameConstant, &configuration)
	return configuration.Sanitize()
}

func (application *Application) resolveVersion(executionContext context.Context) string {
	dependencies := version.Dependencies{}
	gitExecutor, executorError := reposdeps.ResolveGitExecutor(nil, application.logger, application.humanReadableLoggingEnabled())
	if executorError == nil {
		dependencies.GitExecutor = gitExecutor
	}

	resolved := version.Detect(executionContext, dependencies)
	trimmed := strings.TrimSpace(resolved)
	if len(trimmed) == 0 {
		return resolved
	}
	return trimmed
}

func (application *Application) printVersion(executionContext context.Context, output io.Writer) error {
	if output == nil {
		output = io.Discard
	}
	versionString := application.versionResolver(executionContext)
	_, writeError := fmt.Fprintf(output, versionOutputTemplateConstant, versionString)
	return writeError
}

func (application *Application) reposRenameConfiguration() repos.RenameConfiguration {
	configuration := repos.DefaultToolsConfiguration().Rename
	application.decodeOperationConfiguration(reposRenameOperationNameConstant, &configuration)

	options, optionsExist := application.lookupOperationOptions(reposRenameOperationNameConstant)
	if !optionsExist || !optionExists(options, assumeYesOptionKeyConstant) {
		configuration.AssumeYes = application.configuration.Common.AssumeYes
	}
	if !optionsExist || !optionExists(options, requireCleanOptionKeyConstant) {
		configuration.RequireCleanWorktree = application.configuration.Common.RequireClean
	}

	return configuration
}

func (application *Application) reposRemotesConfiguration() repos.RemotesConfiguration {
	configuration := repos.DefaultToolsConfiguration().Remotes
	application.decodeOperationConfiguration(reposRemotesOperationNameConstant, &configuration)

	options, optionsExist := application.lookupOperationOptions(reposRemotesOperationNameConstant)
	if !optionsExist || !optionExists(options, assumeYesOptionKeyConstant) {
		configuration.AssumeYes = application.configuration.Common.AssumeYes
	}

	return configuration
}

func (application *Application) reposProtocolConfiguration() repos.ProtocolConfiguration {
	configuration := repos.DefaultToolsConfiguration().Protocol
	application.decodeOperationConfiguration(reposProtocolOperationNameConstant, &configuration)

	options, optionsExist := application.lookupOperationOptions(reposProtocolOperationNameConstant)
	if !optionsExist || !optionExists(options, assumeYesOptionKeyConstant) {
		configuration.AssumeYes = application.configuration.Common.AssumeYes
	}

	return configuration
}

func (application *Application) reposRemoveConfiguration() repos.RemoveConfiguration {
	configuration := repos.DefaultToolsConfiguration().Remove
	application.decodeOperationConfiguration(repoHistoryOperationNameConstant, &configuration)

	options, optionsExist := application.lookupOperationOptions(repoHistoryOperationNameConstant)
	if !optionsExist || !optionExists(options, assumeYesOptionKeyConstant) {
		configuration.AssumeYes = application.configuration.Common.AssumeYes
	}

	return configuration.Sanitize()
}

func (application *Application) reposReplaceConfiguration() repos.ReplaceConfiguration {
	configuration := repos.DefaultToolsConfiguration().Replace
	application.decodeOperationConfiguration(repoFilesReplaceOperationNameConstant, &configuration)

	options, optionsExist := application.lookupOperationOptions(repoFilesReplaceOperationNameConstant)
	if !optionsExist || !optionExists(options, assumeYesOptionKeyConstant) {
		configuration.AssumeYes = application.configuration.Common.AssumeYes
	}

	return configuration.Sanitize()
}

func (application *Application) reposFilesAddConfiguration() repos.AddConfiguration {
	configuration := repos.DefaultToolsConfiguration().Add
	application.decodeOperationConfiguration(repoFilesAddOperationNameConstant, &configuration)

	options, optionsExist := application.lookupOperationOptions(repoFilesAddOperationNameConstant)
	if !optionsExist || !optionExists(options, assumeYesOptionKeyConstant) {
		configuration.AssumeYes = application.configuration.Common.AssumeYes
	}

	return configuration.Sanitize()
}

func (application *Application) workflowCommandConfiguration() workflowcmd.CommandConfiguration {
	configuration := workflowcmd.DefaultCommandConfiguration()
	application.decodeOperationConfiguration(workflowCommandOperationNameConstant, &configuration)

	options, optionsExist := application.lookupOperationOptions(workflowCommandOperationNameConstant)
	if !optionsExist || !optionExists(options, assumeYesOptionKeyConstant) {
		configuration.AssumeYes = application.configuration.Common.AssumeYes
	}
	if !optionsExist || !optionExists(options, requireCleanOptionKeyConstant) {
		configuration.RequireClean = application.configuration.Common.RequireClean
	}
	configuration.ConnectionProfiles = application.configuration.LLM.connectionProfiles()
	configuredWorkflow := application.configuredWorkflow()
	configuration.ConfiguredWorkflow = &configuredWorkflow

	return configuration
}

func (application *Application) configuredWorkflow() workflowpkg.Configuration {
	steps := make([]workflowpkg.StepConfiguration, 0, len(application.configuration.Workflow))
	for _, wrappedStep := range application.configuration.Workflow {
		steps = append(steps, wrappedStep.Step)
	}
	return workflowpkg.Configuration{Steps: steps}
}

func (application *Application) changelogMessageConfiguration() changelogcmd.MessageConfiguration {
	configuration := changelogcmd.DefaultMessageConfiguration()
	application.applyGlobalLLMDefaultsToChangelogMessageConfiguration(&configuration)
	application.decodeChangelogMessageOperationConfigurationIfPresent(application.configuredOperationConfigurations, changelogMessageConfigurationKeyConstant, &configuration)
	application.decodeChangelogMessageOperationConfigurationIfPresent(application.configuredOperationConfigurations, changelogMessageOperationNameConstant, &configuration)
	configuration.ConnectionProfiles = application.configuration.LLM.connectionProfiles()
	return configuration.Sanitize()
}

func (application *Application) semverDecisionConfiguration() semvercmd.Configuration {
	changelogConfiguration := application.changelogMessageConfiguration()
	return semvercmd.Configuration{
		LLMProxy:           changelogConfiguration.LLMProxy,
		Effort:             changelogConfiguration.Effort,
		MaxTokens:          changelogConfiguration.MaxTokens,
		TimeoutSeconds:     changelogConfiguration.TimeoutSeconds,
		ConnectionProfiles: changelogConfiguration.ConnectionProfiles,
	}.Sanitize()
}

func (application *Application) commitMessageConfiguration() commitcmd.MessageConfiguration {
	configuration := commitcmd.DefaultMessageConfiguration()
	application.applyGlobalLLMDefaultsToCommitMessageConfiguration(&configuration)
	application.decodeCommitMessageOperationConfigurationIfPresent(application.configuredOperationConfigurations, commitMessageConfigurationKeyConstant, &configuration)
	application.decodeCommitMessageOperationConfigurationIfPresent(application.configuredOperationConfigurations, commitMessageOperationNameConstant, &configuration)
	configuration.ConnectionProfiles = application.configuration.LLM.connectionProfiles()
	return configuration.Sanitize()
}

func (application *Application) defaultCommandConfiguration() migrate.CommandConfiguration {
	configuration := migrate.DefaultCommandConfiguration()
	application.decodeOperationConfiguration(defaultOperationNameConstant, &configuration)
	if strings.EqualFold(application.configuration.Common.LogLevel, string(utils.LogLevelDebug)) {
		configuration.EnableDebugLogging = true
	}
	return configuration
}

func (application *Application) decodeOperationConfiguration(operationName string, target any) {
	application.decodeOperationConfigurationFrom(application.operationConfigurations, operationName, target)
}

func (application *Application) decodeOperationConfigurationFrom(configurations OperationConfigurations, operationName string, target any) {
	if decodeError := configurations.decode(operationName, target); decodeError != nil {
		if application.logger == nil {
			return
		}
		application.logger.Warn(
			operationDecodeErrorMessageConstant,
			zap.String(operationNameLogFieldConstant, operationName),
			zap.Error(decodeError),
		)
	}
}

func (application *Application) decodeOperationConfigurationIfPresentFrom(configurations OperationConfigurations, operationName string, target any) (map[string]any, bool) {
	options, exists := application.lookupOperationOptionsFrom(configurations, operationName)
	if !exists {
		return nil, false
	}
	application.decodeOperationConfigurationFrom(configurations, operationName, target)
	return options, true
}

func (application *Application) lookupOperationOptions(operationName string) (map[string]any, bool) {
	return application.lookupOperationOptionsFrom(application.operationConfigurations, operationName)
}

func (application *Application) lookupOperationOptionsFrom(configurations OperationConfigurations, operationName string) (map[string]any, bool) {
	options, lookupError := configurations.Lookup(operationName)
	if lookupError != nil {
		return nil, false
	}
	return options, true
}

func (application *Application) decodeBranchSyncOperationConfiguration(operationName string, target *syncflowcmd.CommandConfiguration) {
	options, exists := application.decodeOperationConfigurationIfPresentFrom(application.operationConfigurations, operationName, target)
	if !exists {
		return
	}
	resetBranchSyncCommitMessageProviderDefault(options, target)
}

func (application *Application) decodeCommitMessageOperationConfigurationIfPresent(configurations OperationConfigurations, operationName string, target *commitcmd.MessageConfiguration) {
	options, exists := application.decodeOperationConfigurationIfPresentFrom(configurations, operationName, target)
	if !exists {
		return
	}
	resetCommitMessageProviderDefault(options, target)
}

func (application *Application) decodeChangelogMessageOperationConfigurationIfPresent(configurations OperationConfigurations, operationName string, target *changelogcmd.MessageConfiguration) {
	options, exists := application.decodeOperationConfigurationIfPresentFrom(configurations, operationName, target)
	if !exists {
		return
	}
	resetChangelogMessageProviderDefault(options, target)
}

func (application *Application) applyGlobalLLMDefaultsToCommitMessageConfiguration(target *commitcmd.MessageConfiguration) {
	if target == nil {
		return
	}
	configuration := application.configuration.LLM
	if configuration.Effort != "" {
		target.Effort = configuration.Effort
	}
	if configuration.TimeoutSeconds > 0 {
		target.TimeoutSeconds = configuration.TimeoutSeconds
	}
}

func (application *Application) applyGlobalLLMDefaultsToChangelogMessageConfiguration(target *changelogcmd.MessageConfiguration) {
	if target == nil {
		return
	}
	configuration := application.configuration.LLM
	if configuration.Effort != "" {
		target.Effort = configuration.Effort
	}
	if configuration.TimeoutSeconds > 0 {
		target.TimeoutSeconds = configuration.TimeoutSeconds
	}
}

func resetCommitMessageProviderDefault(options map[string]any, target *commitcmd.MessageConfiguration) {
	if target == nil {
		return
	}
	proxyOptions, exists := optionMap(options, llmProxyOptionKeyConstant)
	if !exists || !optionExists(proxyOptions, llmProviderOptionKeyConstant) {
		return
	}
	if !optionExists(proxyOptions, llmModelOptionKeyConstant) {
		target.LLMProxy.Model = ""
	}
}

func resetChangelogMessageProviderDefault(options map[string]any, target *changelogcmd.MessageConfiguration) {
	if target == nil {
		return
	}
	proxyOptions, exists := optionMap(options, llmProxyOptionKeyConstant)
	if !exists || !optionExists(proxyOptions, llmProviderOptionKeyConstant) {
		return
	}
	if !optionExists(proxyOptions, llmModelOptionKeyConstant) {
		target.LLMProxy.Model = ""
	}
}

func resetBranchSyncCommitMessageProviderDefault(options map[string]any, target *syncflowcmd.CommandConfiguration) {
	if target == nil {
		return
	}
	commitMessageOptions, exists := optionMap(options, syncCommitMessageOptionKeyConstant)
	if !exists {
		return
	}
	proxyOptions, exists := optionMap(commitMessageOptions, llmProxyOptionKeyConstant)
	if !exists || !optionExists(proxyOptions, llmProviderOptionKeyConstant) {
		return
	}
	if !optionExists(proxyOptions, llmModelOptionKeyConstant) {
		target.CommitMessage.LLMProxy.Model = ""
	}
}

func optionMap(options map[string]any, optionKey string) (map[string]any, bool) {
	optionValue, exists := optionValue(options, optionKey)
	if !exists {
		return nil, false
	}
	switch typedValue := optionValue.(type) {
	case map[string]any:
		return typedValue, true
	case map[interface{}]interface{}:
		converted := make(map[string]any, len(typedValue))
		for key, value := range typedValue {
			keyString, keyOK := key.(string)
			if !keyOK {
				continue
			}
			converted[keyString] = value
		}
		return converted, true
	default:
		return nil, false
	}
}

func optionValue(options map[string]any, optionKey string) (any, bool) {
	if len(options) == 0 {
		return nil, false
	}
	normalizedOptionKey := strings.ToLower(strings.TrimSpace(optionKey))
	for candidateKey, candidateValue := range options {
		if strings.ToLower(strings.TrimSpace(candidateKey)) == normalizedOptionKey {
			return candidateValue, true
		}
	}
	return nil, false
}

func optionExists(options map[string]any, optionKey string) bool {
	if len(options) == 0 {
		return false
	}

	normalizedOptionKey := strings.ToLower(strings.TrimSpace(optionKey))
	for candidateKey := range options {
		if strings.ToLower(strings.TrimSpace(candidateKey)) == normalizedOptionKey {
			return true
		}
	}

	return false
}

func (application *Application) validateOperationConfigurations(command *cobra.Command) error {
	if len(application.configuration.Operations) == 0 {
		return nil
	}

	requiredOperations := application.operationsRequiredForCommand(command)
	if len(requiredOperations) == 0 {
		return nil
	}

	for operationIndex := range requiredOperations {
		operationName := requiredOperations[operationIndex]
		_, lookupError := application.operationConfigurations.Lookup(operationName)
		if lookupError == nil {
			continue
		}
		return lookupError
	}

	return nil
}

func (application *Application) validateOperationConfigurationSchemas() error {
	operationNames := make([]string, 0, len(application.operationConfigurations.entries))
	for operationName := range application.operationConfigurations.entries {
		operationNames = append(operationNames, operationName)
	}
	sort.Strings(operationNames)

	for _, operationName := range operationNames {
		target, supported := operationConfigurationSchemaTarget(operationName)
		if !supported {
			return InvalidOperationConfigurationError{
				OperationName: operationName,
				Cause:         fmt.Errorf(unsupportedOperationConfigurationTemplateConstant, operationName),
			}
		}
		if decodeError := application.operationConfigurations.decode(operationName, target); decodeError != nil {
			return InvalidOperationConfigurationError{
				OperationName: operationName,
				Cause:         decodeError,
			}
		}
	}

	return nil
}

func (application *Application) validateWorkflowConfigurationSchema() error {
	if len(application.configuration.Workflow) == 0 {
		return nil
	}
	if _, buildError := workflowpkg.BuildOperations(application.configuredWorkflow()); buildError != nil {
		return fmt.Errorf("invalid workflow configuration: %w", buildError)
	}
	return nil
}

func operationConfigurationSchemaTarget(operationName string) (any, bool) {
	switch operationName {
	case auditOperationNameConstant:
		return &audit.CommandConfiguration{}, true
	case packagesDeleteOperationNameConstant:
		return &packages.DeleteConfiguration{}, true
	case branchCleanupOperationNameConstant:
		return &branches.CommandConfiguration{}, true
	case reposRenameOperationNameConstant:
		return &repos.RenameConfiguration{}, true
	case reposRemotesOperationNameConstant:
		return &repos.RemotesConfiguration{}, true
	case reposProtocolOperationNameConstant:
		return &repos.ProtocolConfiguration{}, true
	case repoReleaseOperationNameConstant:
		return &releasecmd.CommandConfiguration{}, true
	case repoHistoryOperationNameConstant:
		return &repos.RemoveConfiguration{}, true
	case repoFilesReplaceOperationNameConstant:
		return &repos.ReplaceConfiguration{}, true
	case repoFilesAddOperationNameConstant:
		return &repos.AddConfiguration{}, true
	case workflowCommandOperationNameConstant:
		return &workflowcmd.CommandConfiguration{}, true
	case defaultOperationNameConstant:
		return &migrate.CommandConfiguration{}, true
	case branchSyncOperationNameConstant:
		return &syncflowcmd.CommandConfiguration{}, true
	case commitMessageOperationNameConstant:
		return &commitcmd.MessageConfiguration{}, true
	case changelogMessageOperationNameConstant:
		return &changelogcmd.MessageConfiguration{}, true
	default:
		return nil, false
	}
}

func (application *Application) operationsRequiredForCommand(command *cobra.Command) []string {
	if command == nil {
		return requiredOperationConfigurationNames
	}

	commandName := strings.TrimSpace(command.Name())
	if len(commandName) == 0 {
		return requiredOperationConfigurationNames
	}

	if requiredOperations, exists := commandOperationRequirements[commandName]; exists {
		return requiredOperations
	}

	if parentCommand := command.Parent(); parentCommand != nil {
		parentName := strings.TrimSpace(parentCommand.Name())
		if len(parentName) > 0 {
			compositeKey := parentName + "/" + commandName
			if requiredOperations, exists := commandOperationRequirements[compositeKey]; exists {
				return requiredOperations
			}
		}
		return application.operationsRequiredForCommand(parentCommand)
	}

	return nil
}
func (application *Application) runConfigurationInitializationCommand(command *cobra.Command) error {
	initializationScope, scopeError := application.configurationInitializationScopeForCommand(command)
	if scopeError != nil {
		return scopeError
	}

	forceEnabled, _, forceFlagError := flagutils.BoolFlag(command, configurationInitializationForceFlagNameConstant)
	if forceFlagError != nil {
		return forceFlagError
	}
	application.configurationInitializationForced = forceEnabled

	initializationPlan, planError := application.resolveConfigurationInitializationPlan(initializationScope)
	if planError != nil {
		return planError
	}

	configurationContent, _ := EmbeddedDefaultConfiguration()
	if len(configurationContent) == 0 {
		return errors.New(configurationInitializationContentUnavailableErrorConstant)
	}

	if writeError := application.writeConfigurationFile(initializationPlan, configurationContent); writeError != nil {
		return writeError
	}

	application.logger.Info(
		configurationInitializationSuccessMessageConstant,
		zap.String(configurationFileFieldConstant, initializationPlan.FilePath),
	)

	return nil
}

func (application *Application) configurationInitializationScopeForCommand(command *cobra.Command) (string, error) {
	systemEnabled, systemChanged, systemFlagError := flagutils.BoolFlag(command, configurationInitializationSystemFlagNameConstant)
	if systemFlagError != nil {
		return "", systemFlagError
	}
	if systemChanged && systemEnabled {
		return configurationInitializationScopeSystemConstant, nil
	}

	return configurationInitializationDefaultScopeConstant, nil
}

func (application *Application) resolveConfigurationInitializationPlan(initializationScope string) (configurationInitializationPlan, error) {
	normalizedScope := strings.ToLower(strings.TrimSpace(initializationScope))
	switch normalizedScope {
	case configurationInitializationScopeSystemConstant:
		return configurationInitializationPlan{
			DirectoryPath: filepath.Dir(systemConfigurationFilePath),
			FilePath:      systemConfigurationFilePath,
		}, nil
	case "", configurationInitializationScopeUserConstant:
		userConfigurationPath, userPathError := application.userConfigurationFilePath()
		if userPathError != nil {
			return configurationInitializationPlan{}, userPathError
		}
		return configurationInitializationPlan{
			DirectoryPath: filepath.Dir(userConfigurationPath),
			FilePath:      userConfigurationPath,
		}, nil
	default:
		trimmedScope := strings.TrimSpace(initializationScope)
		if len(trimmedScope) == 0 {
			trimmedScope = initializationScope
		}
		return configurationInitializationPlan{}, fmt.Errorf(configurationInitializationUnsupportedScopeTemplateConstant, trimmedScope)
	}
}

func (application *Application) writeConfigurationFile(initializationPlan configurationInitializationPlan, configurationContent []byte) error {
	if len(configurationContent) == 0 {
		return errors.New(configurationInitializationContentUnavailableErrorConstant)
	}

	directoryPath := strings.TrimSpace(initializationPlan.DirectoryPath)
	if len(directoryPath) == 0 {
		return fmt.Errorf(
			configurationInitializationDirectoryErrorTemplateConstant,
			initializationPlan.DirectoryPath,
			errors.New(configurationInitializationWorkingDirectoryEmptyErrorConstant),
		)
	}

	directoryInfo, directoryStatError := os.Stat(directoryPath)
	switch {
	case directoryStatError == nil:
		if !directoryInfo.IsDir() {
			return fmt.Errorf(configurationInitializationDirectoryConflictTemplateConstant, directoryPath)
		}
	case errors.Is(directoryStatError, os.ErrNotExist):
		if createError := os.MkdirAll(directoryPath, configurationDirectoryPermissionConstant); createError != nil {
			return fmt.Errorf(configurationInitializationDirectoryErrorTemplateConstant, directoryPath, createError)
		}
	default:
		return fmt.Errorf(configurationInitializationDirectoryErrorTemplateConstant, directoryPath, directoryStatError)
	}

	fileInfo, fileStatError := os.Stat(initializationPlan.FilePath)
	switch {
	case fileStatError == nil:
		if fileInfo.IsDir() {
			return fmt.Errorf(configurationInitializationExistingDirectoryTemplateConstant, initializationPlan.FilePath)
		}
		if !application.configurationInitializationForced {
			return fmt.Errorf(configurationInitializationExistingFileTemplateConstant, initializationPlan.FilePath)
		}
	case errors.Is(fileStatError, os.ErrNotExist):
	default:
		return fmt.Errorf(configurationInitializationWriteErrorTemplateConstant, initializationPlan.FilePath, fileStatError)
	}

	writeError := os.WriteFile(initializationPlan.FilePath, configurationContent, configurationFilePermissionConstant)
	if writeError != nil {
		return fmt.Errorf(configurationInitializationWriteErrorTemplateConstant, initializationPlan.FilePath, writeError)
	}

	return nil
}

func (application *Application) runRootCommand(command *cobra.Command, arguments []string) error {
	if application.logger == nil {
		return errors.New(loggerNotInitializedMessageConstant)
	}

	webLaunchHandled, webLaunchError := application.handleWebLaunch(command, arguments)
	if webLaunchError != nil {
		return webLaunchError
	}
	if webLaunchHandled {
		return nil
	}

	application.logger.Info(
		rootCommandInfoMessageConstant,
		zap.String(logFieldCommandNameConstant, command.Name()),
		zap.Int(logFieldArgumentCountConstant, len(arguments)),
	)

	application.logger.Debug(
		rootCommandDebugMessageConstant,
		zap.Strings(logFieldArgumentsConstant, arguments),
	)

	if len(arguments) == 0 {
		return command.Help()
	}

	return nil
}

func (application *Application) flushLogger() error {
	if syncError := application.syncLoggerInstance(application.logger); syncError != nil {
		return syncError
	}

	if syncError := application.syncLoggerInstance(application.consoleLogger); syncError != nil {
		return syncError
	}

	return nil
}

func (application *Application) syncLoggerInstance(logger *zap.Logger) error {
	if logger == nil {
		return nil
	}

	syncError := logger.Sync()
	switch {
	case syncError == nil:
		return nil
	case errors.Is(syncError, syscall.ENOTSUP):
		return nil
	case errors.Is(syncError, syscall.EINVAL):
		return nil
	case errors.Is(syncError, syscall.EBADF):
		return nil
	case errors.Is(syncError, syscall.ENOTTY):
		return nil
	default:
		return syncError
	}
}

func (application *Application) persistentFlagChanged(command *cobra.Command, flagName string) bool {
	if command == nil {
		return false
	}

	flagSetsToInspect := []*pflag.FlagSet{
		command.PersistentFlags(),
		command.InheritedFlags(),
	}

	rootCommand := command.Root()
	if rootCommand != nil {
		flagSetsToInspect = append(flagSetsToInspect, rootCommand.PersistentFlags())
	}

	for _, flagSet := range flagSetsToInspect {
		if flagSet == nil {
			continue
		}

		if flagSet.Changed(flagName) {
			return true
		}
	}

	return false
}
