package packages

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/ghcr"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/utils"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	rootutils "github.com/tyemirov/gix/internal/utils/roots"
	"github.com/tyemirov/gix/internal/workflow"
	"github.com/tyemirov/gix/pkg/taskrunner"
)

const (
	packagesDeleteCommandUseConstant                    = "repo-packages-delete"
	packagesDeleteCommandShortDescriptionConstant       = "Delete GHCR versions outside retention"
	packagesDeleteCommandLongDescriptionConstant        = "repo-packages-delete preserves the newest requested GitHub Container Registry versions and deletes all older versions."
	unexpectedArgumentsErrorMessageConstant             = "repo-packages-delete does not accept positional arguments"
	packageFlagNameConstant                             = "package"
	packageFlagDescriptionConstant                      = "Container package name in GHCR"
	keepFlagNameConstant                                = "keep"
	keepFlagDescriptionConstant                         = "Number of newest package versions to preserve"
	keepFlagRequiredErrorMessageConstant                = "packages delete requires --keep with a positive version count"
	baseURLConfigurationMissingErrorMessageConstant     = "packages delete configuration requires base_url"
	credentialConfigurationMissingErrorMessageConstant  = "packages delete configuration requires credential"
	workingDirectoryResolutionErrorTemplateConstant     = "unable to determine working directory: %w"
	workingDirectoryEmptyErrorMessageConstant           = "working directory not provided"
	gitExecutorResolutionErrorTemplateConstant          = "unable to resolve git executor: %w"
	gitRepositoryManagerResolutionErrorTemplateConstant = "unable to resolve repository manager: %w"
	gitHubResolverResolutionErrorTemplateConstant       = "unable to resolve github metadata resolver: %w"
	repositoryDiscoveryErrorTemplateConstant            = "unable to discover repositories: %w"
	repositoryDiscoveryFailedMessageConstant            = "Failed to discover repositories"
	repositoryRootsLogFieldNameConstant                 = "repository_roots"
	repositoryPathLogFieldNameConstant                  = "repository_path"
	repositoryMetadataFailedMessageConstant             = "Failed to resolve repository metadata"
	ownerRepoSeparatorConstant                          = "/"
)

// LoggerProvider supplies a zap logger instance.
type LoggerProvider func() *zap.Logger

// ConfigurationProvider returns the current packages configuration.
type ConfigurationProvider func() Configuration

// RetentionServiceResolver creates retention executors for the command.
type RetentionServiceResolver interface {
	Resolve(logger *zap.Logger) (RetentionExecutor, error)
}

// CommandBuilder assembles the repo-packages-delete command.
type CommandBuilder struct {
	LoggerProvider             LoggerProvider
	ConfigurationProvider      ConfigurationProvider
	ServiceResolver            RetentionServiceResolver
	HTTPClient                 ghcr.HTTPClient
	GitExecutor                shared.GitExecutor
	RepositoryManager          shared.GitRepositoryManager
	GitHubResolver             shared.GitHubMetadataResolver
	RepositoryMetadataResolver RepositoryMetadataResolver
	WorkingDirectoryResolver   WorkingDirectoryResolver
	RepositoryDiscoverer       shared.RepositoryDiscoverer
	TaskRunnerFactory          func(workflow.Dependencies) TaskRunnerExecutor
}

// WorkingDirectoryResolver resolves the directory containing the active repository.
type WorkingDirectoryResolver func() (string, error)

type commandExecutionOptions struct {
	PackageNameOverride string
	BaseURL             string
	Credential          string
	RepositoryRoots     []string
	Keep                ghcr.KeepCount
}

// Build constructs the repo-packages-delete command with retention functionality.
func (builder *CommandBuilder) Build() (*cobra.Command, error) {
	deleteCommand := &cobra.Command{
		Use:   packagesDeleteCommandUseConstant,
		Short: packagesDeleteCommandShortDescriptionConstant,
		Long:  packagesDeleteCommandLongDescriptionConstant,
		RunE:  builder.runDelete,
	}

	deleteCommand.Flags().String(packageFlagNameConstant, "", packageFlagDescriptionConstant)
	deleteCommand.Flags().Int(keepFlagNameConstant, 0, keepFlagDescriptionConstant)

	return deleteCommand, nil
}

func (builder *CommandBuilder) runDelete(command *cobra.Command, arguments []string) error {
	if len(arguments) > 0 {
		return errors.New(unexpectedArgumentsErrorMessageConstant)
	}

	logger := builder.resolveLogger()
	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

	executionOptions, optionsError := builder.parseCommandOptions(command, arguments, executionFlags, executionFlagsAvailable)
	if optionsError != nil {
		return optionsError
	}

	retentionService, serviceError := builder.resolveRetentionService(logger, executionOptions.BaseURL)
	if serviceError != nil {
		return serviceError
	}

	dependencyResult, dependencyError := taskrunner.BuildDependencies(
		taskrunner.DependenciesConfig{
			LoggerProvider:       func() *zap.Logger { return logger },
			RepositoryDiscoverer: builder.RepositoryDiscoverer,
			GitExecutor:          builder.GitExecutor,
			GitRepositoryManager: builder.RepositoryManager,
			GitHubResolver:       builder.GitHubResolver,
			FileSystem:           nil,
		},
		taskrunner.DependenciesOptions{
			Command:         command,
			Output:          command.OutOrStdout(),
			Errors:          command.ErrOrStderr(),
			DisablePrompter: true,
		},
	)
	if dependencyError != nil {
		return dependencyError
	}

	repositoryMetadataResolver, metadataResolverError := builder.resolveRepositoryMetadataResolver(
		logger,
		dependencyResult.RepositoryManager,
		dependencyResult.GitHubResolver,
	)
	if metadataResolverError != nil {
		return metadataResolverError
	}

	taskRunner := resolveTaskRunner(builder.TaskRunnerFactory, dependencyResult.Workflow)

	actionOptions := map[string]any{
		"service":           retentionService,
		"metadata_resolver": repositoryMetadataResolver,
		"credential":        executionOptions.Credential,
		"package_override":  executionOptions.PackageNameOverride,
		"keep_count":        executionOptions.Keep,
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        "Apply package version retention",
		EnsureClean: false,
		Actions: []workflow.TaskActionDefinition{
			{Type: taskActionPackagesRetention, Options: actionOptions},
		},
	}

	runtimeOptions := workflow.RuntimeOptions{AssumeYes: executionFlags.AssumeYes}

	_, runErr := taskRunner.Run(command.Context(), executionOptions.RepositoryRoots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
	return runErr
}

func (builder *CommandBuilder) parseCommandOptions(command *cobra.Command, arguments []string, executionFlags utils.ExecutionFlags, executionFlagsAvailable bool) (commandExecutionOptions, error) {
	configuration := builder.resolveConfiguration()

	packageFlagValue, packageFlagError := command.Flags().GetString(packageFlagNameConstant)
	if packageFlagError != nil {
		return commandExecutionOptions{}, packageFlagError
	}
	if !command.Flags().Changed(keepFlagNameConstant) {
		return commandExecutionOptions{}, errors.New(keepFlagRequiredErrorMessageConstant)
	}
	keepValue, keepFlagError := command.Flags().GetInt(keepFlagNameConstant)
	if keepFlagError != nil {
		return commandExecutionOptions{}, keepFlagError
	}
	keepCount, keepCountError := ghcr.NewKeepCount(keepValue)
	if keepCountError != nil {
		return commandExecutionOptions{}, keepCountError
	}

	packageValue := selectOptionalStringValue(packageFlagValue, configuration.Delete.PackageName)
	baseURL := strings.TrimSpace(configuration.Delete.BaseURL)
	if baseURL == "" {
		return commandExecutionOptions{}, errors.New(baseURLConfigurationMissingErrorMessageConstant)
	}
	credential := strings.TrimSpace(configuration.Delete.Credential)
	if credential == "" {
		return commandExecutionOptions{}, errors.New(credentialConfigurationMissingErrorMessageConstant)
	}

	repositoryRoots, rootsError := rootutils.Resolve(command, arguments, configuration.Delete.RepositoryRoots)
	if rootsError != nil {
		return commandExecutionOptions{}, rootsError
	}

	executionOptions := commandExecutionOptions{
		PackageNameOverride: packageValue,
		BaseURL:             baseURL,
		Credential:          credential,
		RepositoryRoots:     repositoryRoots,
		Keep:                keepCount,
	}

	return executionOptions, nil
}

func (builder *CommandBuilder) resolveLogger() *zap.Logger {
	if builder.LoggerProvider == nil {
		return zap.NewNop()
	}

	logger := builder.LoggerProvider()
	if logger == nil {
		return zap.NewNop()
	}

	return logger
}

func (builder *CommandBuilder) resolveConfiguration() Configuration {
	configuration := DefaultConfiguration()
	if builder.ConfigurationProvider != nil {
		configuration = builder.ConfigurationProvider()
	}

	return configuration.Sanitize()
}

func (builder *CommandBuilder) resolveRetentionService(logger *zap.Logger, baseURL string) (RetentionExecutor, error) {
	if builder.ServiceResolver != nil {
		return builder.ServiceResolver.Resolve(logger)
	}

	defaultResolver := &DefaultRetentionServiceResolver{
		HTTPClient: builder.HTTPClient,
		ServiceConfiguration: ghcr.ServiceConfiguration{
			BaseURL: strings.TrimSpace(baseURL),
		},
	}

	return defaultResolver.Resolve(logger)
}

func selectOptionalStringValue(flagValue string, configurationValue string) string {
	trimmedFlagValue := strings.TrimSpace(flagValue)
	if len(trimmedFlagValue) > 0 {
		return trimmedFlagValue
	}

	return strings.TrimSpace(configurationValue)
}

func (builder *CommandBuilder) resolveRepositoryMetadataResolver(
	logger *zap.Logger,
	repositoryManager shared.GitRepositoryManager,
	githubResolver shared.GitHubMetadataResolver,
) (RepositoryMetadataResolver, error) {
	if builder.RepositoryMetadataResolver != nil {
		return builder.RepositoryMetadataResolver, nil
	}

	return &DefaultRepositoryMetadataResolver{
		RepositoryManager: repositoryManager,
		GitHubResolver:    githubResolver,
	}, nil
}
