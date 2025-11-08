package changelog

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/temirov/gix/internal/repos/shared"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	rootutils "github.com/temirov/gix/internal/utils/roots"
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/llm"
	"github.com/temirov/gix/pkg/taskrunner"
)

const (
	messageCommandUseName          = "message"
	messageCommandShortDescription = "Generate a changelog section from git history"
	messageCommandAlias            = "msg"
	versionFlagName                = "version"
	versionFlagUsage               = "Release version label to include in the changelog heading"
	releaseDateFlagName            = "release-date"
	releaseDateFlagUsage           = "Release date to include in the changelog heading (YYYY-MM-DD)"
	sinceReferenceFlagName         = "since-tag"
	sinceReferenceFlagUsage        = "Tag or commit to compare against when collecting changes"
	sinceDateFlagName              = "since-date"
	sinceDateFlagUsage             = "Timestamp boundary (RFC3339 or YYYY-MM-DD) for changes; conflicts with --since-tag"
	maxTokensFlagName              = "max-tokens"
	maxTokensFlagUsage             = "Override the maximum completion tokens"
	temperatureFlagName            = "temperature"
	temperatureFlagUsage           = "Override the sampling temperature (0-2)"
	modelFlagName                  = "model"
	modelFlagUsage                 = "Override the model identifier"
	baseURLFlagName                = "base-url"
	baseURLFlagUsage               = "Override the LLM base URL"
	apiKeyEnvFlagName              = "api-key-env"
	apiKeyEnvFlagUsage             = "Environment variable providing the LLM API key"
	timeoutFlagName                = "timeout-seconds"
	timeoutFlagUsage               = "Override the LLM request timeout in seconds"

	taskTypeChangelogMessage       = "changelog.message.generate"
	taskOptionChangelogVersion     = "version"
	taskOptionChangelogReleaseDate = "release_date"
	taskOptionChangelogSinceRef    = "since_reference"
	taskOptionChangelogSinceDate   = "since_date"
	taskOptionChangelogMaxTokens   = "max_tokens"
	taskOptionChangelogTemperature = "temperature"
	taskOptionChangelogClient      = "client"
)

// ClientFactory builds chat clients from configuration.
type ClientFactory func(config llm.Config) (llm.ChatClient, error)

// MessageCommandBuilder assembles the changelog message command.
type MessageCommandBuilder struct {
	LoggerProvider               LoggerProvider
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	GitHubResolver               shared.GitHubMetadataResolver
	Discoverer                   shared.RepositoryDiscoverer
	FileSystem                   shared.FileSystem
	ConfigurationProvider        func() MessageConfiguration
	HumanReadableLoggingProvider func() bool
	ClientFactory                ClientFactory
	TaskRunnerFactory            taskrunner.Factory
}

// Build constructs the changelog message command.
func (builder *MessageCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   messageCommandUseName,
		Short: messageCommandShortDescription,
		Args:  cobra.NoArgs,
		RunE:  builder.run,
	}

	command.Flags().String(versionFlagName, "", versionFlagUsage)
	command.Flags().String(releaseDateFlagName, "", releaseDateFlagUsage)
	command.Flags().String(sinceReferenceFlagName, "", sinceReferenceFlagUsage)
	command.Flags().String(sinceDateFlagName, "", sinceDateFlagUsage)
	command.Flags().Int(maxTokensFlagName, 0, maxTokensFlagUsage)
	command.Flags().Float64(temperatureFlagName, 0, temperatureFlagUsage)
	command.Flags().String(modelFlagName, "", modelFlagUsage)
	command.Flags().String(baseURLFlagName, "", baseURLFlagUsage)
	command.Flags().String(apiKeyEnvFlagName, "", apiKeyEnvFlagUsage)
	command.Flags().Int(timeoutFlagName, 0, timeoutFlagUsage)

	command.Aliases = append(command.Aliases, messageCommandAlias)

	return command, nil
}

func (builder *MessageCommandBuilder) run(command *cobra.Command, arguments []string) error {
	configuration := builder.resolveConfiguration()

	executionFlags, _ := flagutils.ResolveExecutionFlags(command)

	repositoryPath, rootError := selectRepositoryRoot(command, configuration)
	if rootError != nil {
		return rootError
	}

	version := configuration.Version
	if command != nil {
		if flagValue, flagError := command.Flags().GetString(versionFlagName); flagError == nil && command.Flags().Changed(versionFlagName) {
			version = strings.TrimSpace(flagValue)
		}
	}

	releaseDate := configuration.ReleaseDate
	if command != nil {
		if flagValue, flagError := command.Flags().GetString(releaseDateFlagName); flagError == nil && command.Flags().Changed(releaseDateFlagName) {
			releaseDate = strings.TrimSpace(flagValue)
		}
	}

	sinceReference := configuration.SinceReference
	if command != nil {
		if flagValue, flagError := command.Flags().GetString(sinceReferenceFlagName); flagError == nil && command.Flags().Changed(sinceReferenceFlagName) {
			sinceReference = strings.TrimSpace(flagValue)
		}
	}

	sinceDateValue := configuration.SinceDate
	if command != nil {
		if flagValue, flagError := command.Flags().GetString(sinceDateFlagName); flagError == nil && command.Flags().Changed(sinceDateFlagName) {
			sinceDateValue = strings.TrimSpace(flagValue)
		}
	}

	if sinceReference != "" && sinceDateValue != "" {
		return errors.New("only one of --since-tag or --since-date may be provided")
	}

	var sinceDate *time.Time
	if sinceDateValue != "" {
		parsedSinceDate, parseError := parseSinceDate(sinceDateValue)
		if parseError != nil {
			return parseError
		}
		sinceDate = parsedSinceDate
	}

	maxTokens, maxTokensError := resolveMaxTokens(command, configuration)
	if maxTokensError != nil {
		return maxTokensError
	}

	temperaturePointer, temperatureError := resolveTemperature(command, configuration)
	if temperatureError != nil {
		return temperatureError
	}

	modelIdentifier := configuration.Model
	if command != nil {
		if flagValue, flagError := command.Flags().GetString(modelFlagName); flagError == nil && command.Flags().Changed(modelFlagName) {
			modelIdentifier = strings.TrimSpace(flagValue)
		}
	}
	if modelIdentifier == "" {
		return errors.New("model identifier must be provided via configuration or --model")
	}

	baseURL := configuration.BaseURL
	if command != nil {
		if flagValue, flagError := command.Flags().GetString(baseURLFlagName); flagError == nil && command.Flags().Changed(baseURLFlagName) {
			baseURL = strings.TrimSpace(flagValue)
		}
	}

	apiKeyEnv := configuration.APIKeyEnv
	if command != nil {
		if flagValue, flagError := command.Flags().GetString(apiKeyEnvFlagName); flagError == nil && command.Flags().Changed(apiKeyEnvFlagName) {
			apiKeyEnv = strings.TrimSpace(flagValue)
		}
	}
	if apiKeyEnv == "" {
		apiKeyEnv = defaultAPIKeyEnvironment
	}
	apiKey, apiKeyPresent := lookupEnvironmentValue(apiKeyEnv)
	if !apiKeyPresent || apiKey == "" {
		return fmt.Errorf("environment variable %s must be set with an API key", apiKeyEnv)
	}

	timeoutDuration := time.Duration(configuration.TimeoutSeconds) * time.Second
	if command != nil {
		if flagValue, flagError := command.Flags().GetInt(timeoutFlagName); flagError == nil && command.Flags().Changed(timeoutFlagName) {
			if flagValue <= 0 {
				return errors.New("timeout-seconds must be positive")
			}
			timeoutDuration = time.Duration(flagValue) * time.Second
		}
	}

	dependencyResult, dependencyError := taskrunner.BuildDependencies(
		taskrunner.DependenciesConfig{
			LoggerProvider:               builder.LoggerProvider,
			HumanReadableLoggingProvider: builder.HumanReadableLoggingProvider,
			RepositoryDiscoverer:         builder.Discoverer,
			GitExecutor:                  builder.GitExecutor,
			GitRepositoryManager:         builder.GitManager,
			GitHubResolver:               builder.GitHubResolver,
			FileSystem:                   builder.FileSystem,
		},
		taskrunner.DependenciesOptions{
			Command:         command,
			DisablePrompter: true,
		},
	)
	if dependencyError != nil {
		return dependencyError
	}
	taskDependencies := dependencyResult.Workflow

	clientFactory := builder.ClientFactory
	if clientFactory == nil {
		clientFactory = func(config llm.Config) (llm.ChatClient, error) {
			return llm.NewFactory(config)
		}
	}

	client, clientError := clientFactory(llm.Config{
		BaseURL:             baseURL,
		APIKey:              apiKey,
		Model:               modelIdentifier,
		MaxCompletionTokens: configuration.MaxTokens,
		Temperature:         configuration.Temperature,
		RequestTimeout:      timeoutDuration,
	})
	if clientError != nil {
		return clientError
	}

	taskDependencies.Output = command.OutOrStdout()
	taskDependencies.Errors = command.ErrOrStderr()

	taskRunner := taskrunner.Resolve(builder.TaskRunnerFactory, taskDependencies)

	actionOptions := map[string]any{
		taskOptionChangelogVersion:     version,
		taskOptionChangelogReleaseDate: releaseDate,
		taskOptionChangelogSinceRef:    sinceReference,
		taskOptionChangelogMaxTokens:   maxTokens,
		taskOptionChangelogClient:      client,
	}
	if sinceDate != nil {
		actionOptions[taskOptionChangelogSinceDate] = sinceDate
	}
	if temperaturePointer != nil {
		actionOptions[taskOptionChangelogTemperature] = *temperaturePointer
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        "Generate changelog section",
		EnsureClean: false,
		Actions: []workflow.TaskActionDefinition{
			{Type: taskTypeChangelogMessage, Options: actionOptions},
		},
	}

	runtimeOptions := workflow.RuntimeOptions{AssumeYes: executionFlags.AssumeYes}

	return taskRunner.Run(
		command.Context(),
		[]string{repositoryPath},
		[]workflow.TaskDefinition{taskDefinition},
		runtimeOptions,
	)
}

func (builder *MessageCommandBuilder) resolveConfiguration() MessageConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultMessageConfiguration()
	}
	return builder.ConfigurationProvider().Sanitize()
}

func selectRepositoryRoot(command *cobra.Command, configuration MessageConfiguration) (string, error) {
	flagRoots, flagError := rootutils.FlagValues(command)
	if flagError != nil {
		return "", flagError
	}
	flagRoots = rootutils.SanitizeConfigured(flagRoots)
	configurationRoots := rootutils.SanitizeConfigured(configuration.Roots)

	var roots []string
	switch {
	case len(flagRoots) > 0:
		roots = flagRoots
	case len(configurationRoots) > 0:
		roots = configurationRoots
	default:
		roots = []string{"."}
	}

	if len(roots) != 1 {
		return "", fmt.Errorf("changelog message command requires exactly one repository root (received %d)", len(roots))
	}

	trimmed := strings.TrimSpace(roots[0])
	if trimmed == "" {
		return "", errors.New("repository root cannot be empty")
	}
	return trimmed, nil
}

func resolveMaxTokens(command *cobra.Command, configuration MessageConfiguration) (int, error) {
	maxTokens := configuration.MaxTokens
	if command != nil {
		if flagValue, flagError := command.Flags().GetInt(maxTokensFlagName); flagError == nil && command.Flags().Changed(maxTokensFlagName) {
			if flagValue < 0 {
				return 0, errors.New("max-tokens must be zero or positive")
			}
			maxTokens = flagValue
		}
	}
	return maxTokens, nil
}

func resolveTemperature(command *cobra.Command, configuration MessageConfiguration) (*float64, error) {
	if command != nil {
		if flagValue, flagError := command.Flags().GetFloat64(temperatureFlagName); flagError == nil && command.Flags().Changed(temperatureFlagName) {
			if flagValue < 0 {
				return nil, errors.New("temperature cannot be negative")
			}
			return &flagValue, nil
		}
	}
	if configuration.Temperature > 0 {
		value := configuration.Temperature
		return &value, nil
	}
	return nil, nil
}

func parseSinceDate(value string) (*time.Time, error) {
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02",
	}
	for _, layout := range formats {
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("unable to parse since-date %q; expected RFC3339 or YYYY-MM-DD", value)
}
