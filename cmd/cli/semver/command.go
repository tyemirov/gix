package semver

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/v5/internal/llmclient"
	"github.com/tyemirov/gix/v5/internal/repos/shared"
	"github.com/tyemirov/gix/v5/internal/semverdecision"
	"github.com/tyemirov/gix/v5/pkg/taskrunner"
)

const (
	commandUseName          = "semver <boundary>"
	commandShortDescription = "Decide the next SemVer level"
)

// ClientFactory builds one configured LLM connection.
type ClientFactory = llmclient.ClientFactory

// LoggerProvider provides command logging.
type LoggerProvider func() *zap.Logger

// Configuration contains the fixed LLM policy for release decisions.
type Configuration struct {
	LLMProxy           llmclient.LLMProxySelection
	Effort             string
	MaxTokens          int
	TimeoutSeconds     int
	ConnectionProfiles llmclient.ConnectionProfiles
}

// Sanitize normalizes the configured decision policy.
func (configuration Configuration) Sanitize() Configuration {
	sanitized := configuration
	sanitized.LLMProxy.Provider = strings.TrimSpace(configuration.LLMProxy.Provider)
	sanitized.LLMProxy.Model = strings.TrimSpace(configuration.LLMProxy.Model)
	sanitized.Effort = strings.TrimSpace(configuration.Effort)
	if sanitized.MaxTokens < 0 {
		sanitized.MaxTokens = 0
	}
	return sanitized
}

// CommandBuilder assembles the internal release decision node.
type CommandBuilder struct {
	LoggerProvider               LoggerProvider
	HumanReadableLoggingProvider func() bool
	GitExecutor                  shared.GitExecutor
	ConfigurationProvider        func() Configuration
	ClientFactory                ClientFactory
}

// Build constructs the hidden zero-choice decision command used by make release.
func (builder CommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:    commandUseName,
		Short:  commandShortDescription,
		Args:   cobra.ExactArgs(1),
		Hidden: true,
		RunE:   builder.run,
	}
	return command, nil
}

func (builder CommandBuilder) run(command *cobra.Command, arguments []string) error {
	boundary := strings.TrimSpace(arguments[0])
	if boundary == "" {
		return errors.New("semver decision boundary cannot be empty")
	}

	configuration := Configuration{}
	if builder.ConfigurationProvider != nil {
		configuration = builder.ConfigurationProvider().Sanitize()
	}

	dependencies, dependencyError := taskrunner.BuildDependencies(
		taskrunner.DependenciesConfig{
			LoggerProvider:               builder.LoggerProvider,
			HumanReadableLoggingProvider: builder.HumanReadableLoggingProvider,
			GitExecutor:                  builder.GitExecutor,
		},
		taskrunner.DependenciesOptions{
			Command:            command,
			Output:             command.OutOrStdout(),
			Errors:             command.ErrOrStderr(),
			DisablePrompter:    true,
			SkipGitHubResolver: true,
		},
	)
	if dependencyError != nil {
		return dependencyError
	}

	client, clientError := llmclient.NewPrioritizedFactory(
		configuration.ConnectionProfiles,
		configuration.LLMProxy,
		llmclient.RuntimeConfig{
			Effort:              configuration.Effort,
			MaxCompletionTokens: configuration.MaxTokens,
			RequestTimeout:      time.Duration(configuration.TimeoutSeconds) * time.Second,
		},
		builder.ClientFactory,
	)
	if clientError != nil {
		return clientError
	}

	result, decisionError := (semverdecision.Generator{
		GitExecutor: dependencies.GitExecutor,
		Client:      client,
	}).Generate(command.Context(), semverdecision.Options{
		RepositoryPath: ".",
		SinceReference: boundary,
		MaxTokens:      configuration.MaxTokens,
	})
	if decisionError != nil {
		return decisionError
	}

	return json.NewEncoder(command.OutOrStdout()).Encode(struct {
		Bump               semverdecision.Bump `json:"bump"`
		Reason             string              `json:"reason"`
		DeterministicFloor semverdecision.Bump `json:"deterministic_floor"`
	}{
		Bump:               result.Bump,
		Reason:             result.Reason,
		DeterministicFloor: result.DeterministicFloor,
	})
}
