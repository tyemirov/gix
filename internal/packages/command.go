package packages

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/temirov/gix/internal/ghcr"
	"github.com/temirov/gix/internal/repos/shared"
	"github.com/temirov/gix/internal/utils"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	rootutils "github.com/temirov/gix/internal/utils/roots"
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
)

const (
	packagesPurgeCommandUseConstant                     = "repo-packages-purge"
	packagesPurgeCommandShortDescriptionConstant        = "Delete untagged GHCR versions"
	packagesPurgeCommandLongDescriptionConstant         = "repo-packages-purge removes untagged container versions from GitHub Container Registry."
	unexpectedArgumentsErrorMessageConstant             = "repo-packages-purge does not accept positional arguments"
	commandExecutionErrorTemplateConstant               = "repo-packages-purge failed: %w"
	packageFlagNameConstant                             = "package"
	packageFlagDescriptionConstant                      = "Container package name in GHCR"
	tokenSourceParseErrorTemplateConstant               = "invalid token source: %w"
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
	repositoryPurgeFailedMessageConstant                = "repo-packages-purge failed for repository"
	ownerRepoSeparatorConstant                          = "/"
)

// LoggerProvider supplies a zap logger instance.
type LoggerProvider func() *zap.Logger

// ConfigurationProvider returns the current packages configuration.
type ConfigurationProvider func() Configuration

// PurgeServiceResolver creates purge executors for the command.
type PurgeServiceResolver interface {
	Resolve(logger *zap.Logger) (PurgeExecutor, error)
}

// CommandBuilder assembles the repo-packages-purge command.
type CommandBuilder struct {
	LoggerProvider             LoggerProvider
	ConfigurationProvider      ConfigurationProvider
	ServiceResolver            PurgeServiceResolver
	HTTPClient                 ghcr.HTTPClient
	EnvironmentLookup          EnvironmentLookup
	FileReader                 FileReader
	TokenResolver              TokenResolver
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
	TokenSource         TokenSourceConfiguration
	RepositoryRoots     []string
}

// Build constructs the repo-packages-purge command with purge functionality.
func (builder *CommandBuilder) Build() (*cobra.Command, error) {
	purgeCommand := &cobra.Command{
		Use:   packagesPurgeCommandUseConstant,
		Short: packagesPurgeCommandShortDescriptionConstant,
		Long:  packagesPurgeCommandLongDescriptionConstant,
		RunE:  builder.runPurge,
	}

	purgeCommand.Flags().String(packageFlagNameConstant, "", packageFlagDescriptionConstant)

	return purgeCommand, nil
}

func (builder *CommandBuilder) runPurge(command *cobra.Command, arguments []string) error {
	if len(arguments) > 0 {
		return errors.New(unexpectedArgumentsErrorMessageConstant)
	}

	logger := builder.resolveLogger()
	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

	executionOptions, optionsError := builder.parseCommandOptions(command, arguments, executionFlags, executionFlagsAvailable)
	if optionsError != nil {
		return optionsError
	}

	purgeService, serviceError := builder.resolvePurgeService(logger)
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
		"service":           purgeService,
		"metadata_resolver": repositoryMetadataResolver,
		"token_source":      executionOptions.TokenSource,
		"package_override":  executionOptions.PackageNameOverride,
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        "Purge package versions",
		EnsureClean: false,
		Actions: []workflow.TaskActionDefinition{
			{Type: taskActionPackagesPurge, Options: actionOptions},
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
	packageValue := selectOptionalStringValue(packageFlagValue, configuration.Purge.PackageName)

	parsedTokenSource, tokenParseError := ParseTokenSource(defaultTokenSourceValueConstant)
	if tokenParseError != nil {
		return commandExecutionOptions{}, fmt.Errorf(tokenSourceParseErrorTemplateConstant, tokenParseError)
	}

	repositoryRoots, rootsError := rootutils.Resolve(command, arguments, configuration.Purge.RepositoryRoots)
	if rootsError != nil {
		return commandExecutionOptions{}, rootsError
	}

	executionOptions := commandExecutionOptions{
		PackageNameOverride: packageValue,
		TokenSource:         parsedTokenSource,
		RepositoryRoots:     repositoryRoots,
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

func (builder *CommandBuilder) resolvePurgeService(logger *zap.Logger) (PurgeExecutor, error) {
	if builder.ServiceResolver != nil {
		return builder.ServiceResolver.Resolve(logger)
	}

	defaultResolver := &DefaultPurgeServiceResolver{
		HTTPClient:        builder.HTTPClient,
		EnvironmentLookup: builder.EnvironmentLookup,
		FileReader:        builder.FileReader,
		TokenResolver:     builder.TokenResolver,
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
