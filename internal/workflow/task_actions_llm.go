package workflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tyemirov/gix/internal/changelog"
	"github.com/tyemirov/gix/internal/commitmsg"
	"github.com/tyemirov/gix/pkg/llm"
)

const (
	taskActionCommitMessage    = "commit.message.generate"
	taskActionChangelog        = "changelog.message.generate"
	commitOptionDiffSource     = "diff_source"
	commitOptionMaxTokens      = "max_tokens"
	commitOptionTemperature    = "temperature"
	commitOptionClient         = "client"
	changelogOptionVersion     = "version"
	changelogOptionRelease     = "release_date"
	changelogOptionSinceRef    = "since_reference"
	changelogOptionSinceDate   = "since_date"
	changelogOptionMaxTokens   = "max_tokens"
	changelogOptionTemperature = "temperature"
	changelogOptionClient      = "client"
	taskActionCaptureOptionKey = "capture_as"
)

func init() {
	RegisterTaskAction(taskActionCommitMessage, handleCommitMessageAction)
	RegisterTaskAction(taskActionChangelog, handleChangelogAction)
}

func handleCommitMessageAction(ctx context.Context, environment *Environment, repository *RepositoryState, parameters map[string]any) error {
	if environment == nil || repository == nil {
		return nil
	}

	reader := newOptionReader(parameters)

	diffSourceValue, _, diffErr := reader.stringValue(commitOptionDiffSource)
	if diffErr != nil {
		return diffErr
	}

	diffSource, parseErr := parseCommitDiffSource(diffSourceValue)
	if parseErr != nil {
		return parseErr
	}

	maxTokens, maxTokensErr := readIntOption(parameters, commitOptionMaxTokens)
	if maxTokensErr != nil {
		return maxTokensErr
	}

	temperature, temperatureErr := readOptionalFloatOption(parameters, commitOptionTemperature)
	if temperatureErr != nil {
		return temperatureErr
	}

	client, clientErr := extractCommitClient(parameters)
	if clientErr != nil {
		return clientErr
	}

	generator := commitmsg.Generator{
		GitExecutor: environment.GitExecutor,
		Client:      client,
		Logger:      environment.Logger,
	}

	result, generateErr := generator.Generate(ctx, commitmsg.Options{
		RepositoryPath: repository.Path,
		Source:         diffSource,
		MaxTokens:      maxTokens,
		Temperature:    temperature,
	})
	if generateErr != nil {
		return generateErr
	}

	output := environment.Output
	if output == nil {
		output = io.Discard
	}

	fmt.Fprintln(output, result.Message)
	return captureActionOutput(environment, parameters, result.Message)
}

func handleChangelogAction(ctx context.Context, environment *Environment, repository *RepositoryState, parameters map[string]any) error {
	if environment == nil || repository == nil {
		return nil
	}

	reader := newOptionReader(parameters)

	version, _, versionErr := reader.stringValue(changelogOptionVersion)
	if versionErr != nil {
		return versionErr
	}

	releaseDate, _, releaseErr := reader.stringValue(changelogOptionRelease)
	if releaseErr != nil {
		return releaseErr
	}

	sinceReference, _, sinceRefErr := reader.stringValue(changelogOptionSinceRef)
	if sinceRefErr != nil {
		return sinceRefErr
	}

	sinceDate, sinceDateErr := extractSinceDate(parameters)
	if sinceDateErr != nil {
		return sinceDateErr
	}

	maxTokens, maxTokensErr := readIntOption(parameters, changelogOptionMaxTokens)
	if maxTokensErr != nil {
		return maxTokensErr
	}

	temperature, temperatureErr := readOptionalFloatOption(parameters, changelogOptionTemperature)
	if temperatureErr != nil {
		return temperatureErr
	}

	client, clientErr := extractChangelogClient(parameters)
	if clientErr != nil {
		return clientErr
	}

	generator := changelog.Generator{
		GitExecutor: environment.GitExecutor,
		Client:      client,
		Logger:      environment.Logger,
	}

	options := changelog.Options{
		RepositoryPath: repository.Path,
		Version:        version,
		ReleaseDate:    releaseDate,
		SinceReference: sinceReference,
		SinceDate:      sinceDate,
		MaxTokens:      maxTokens,
		Temperature:    temperature,
	}

	result, generateErr := generator.Generate(ctx, options)
	if generateErr != nil {
		if errors.Is(generateErr, changelog.ErrNoChanges) {
			writer := environment.Errors
			if writer == nil {
				writer = environment.Output
			}
			if writer == nil {
				writer = io.Discard
			}
			fmt.Fprintln(writer, generateErr.Error())
			return nil
		}
		return generateErr
	}

	output := environment.Output
	if output == nil {
		output = io.Discard
	}

	fmt.Fprintln(output, result.Section)
	return captureActionOutput(environment, parameters, result.Section)
}

func extractCommitClient(options map[string]any) (llm.ChatClient, error) {
	rawClient, ok := options[commitOptionClient]
	if !ok {
		return nil, errors.New("commit message action requires client option")
	}
	switch typed := rawClient.(type) {
	case llm.ChatClient:
		return typed, nil
	case *TaskLLMClientConfiguration:
		if typed == nil {
			return nil, errors.New("commit message action received invalid client configuration")
		}
		client, clientErr := typed.Client()
		if clientErr != nil {
			return nil, clientErr
		}
		return client, nil
	default:
		return nil, errors.New("commit message action received invalid client option")
	}
}

func extractChangelogClient(options map[string]any) (llm.ChatClient, error) {
	rawClient, ok := options[changelogOptionClient]
	if !ok {
		return nil, errors.New("changelog action requires client option")
	}
	switch typed := rawClient.(type) {
	case llm.ChatClient:
		return typed, nil
	case *TaskLLMClientConfiguration:
		if typed == nil {
			return nil, errors.New("changelog action received invalid client configuration")
		}
		client, clientErr := typed.Client()
		if clientErr != nil {
			return nil, clientErr
		}
		return client, nil
	default:
		return nil, errors.New("changelog action received invalid client option")
	}
}

func captureActionOutput(environment *Environment, parameters map[string]any, value string) error {
	if environment == nil {
		return nil
	}

	reader := newOptionReader(parameters)
	variableName, exists, nameErr := reader.stringValue(taskActionCaptureOptionKey)
	if nameErr != nil {
		return nameErr
	}
	if !exists || variableName == "" {
		return nil
	}
	if environment.Variables == nil {
		return errors.New("workflow variable store not configured")
	}
	normalizedName, normalizedErr := NewVariableName(variableName)
	if normalizedErr != nil {
		return normalizedErr
	}
	environment.Variables.Set(normalizedName, strings.TrimSpace(value))
	return nil
}

func parseCommitDiffSource(value string) (commitmsg.DiffSource, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(commitmsg.DiffSourceStaged):
		return commitmsg.DiffSourceStaged, nil
	case string(commitmsg.DiffSourceWorktree):
		return commitmsg.DiffSourceWorktree, nil
	default:
		return "", fmt.Errorf("unsupported diff source %q", value)
	}
}

func readIntOption(options map[string]any, key string) (int, error) {
	raw, exists := options[key]
	if !exists {
		return 0, nil
	}
	switch typed := raw.(type) {
	case int:
		return typed, nil
	case int32:
		return int(typed), nil
	case int64:
		return int(typed), nil
	case float64:
		return int(typed), nil
	default:
		return 0, fmt.Errorf("%s must be an integer", key)
	}
}

func readOptionalFloatOption(options map[string]any, key string) (*float64, error) {
	raw, exists := options[key]
	if !exists || raw == nil {
		return nil, nil
	}
	switch typed := raw.(type) {
	case float32:
		value := float64(typed)
		return &value, nil
	case float64:
		return &typed, nil
	default:
		return nil, fmt.Errorf("%s must be a float", key)
	}
}

func extractSinceDate(options map[string]any) (*time.Time, error) {
	raw, exists := options[changelogOptionSinceDate]
	if !exists || raw == nil {
		return nil, nil
	}
	typed, ok := raw.(*time.Time)
	if !ok {
		return nil, errors.New("changelog action expects since_date to be a *time.Time")
	}
	return typed, nil
}
