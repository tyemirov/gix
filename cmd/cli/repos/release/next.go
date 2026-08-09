package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/v5/internal/execshell"
	"github.com/tyemirov/gix/v5/internal/llmclient"
	"github.com/tyemirov/gix/v5/internal/releaseversion"
	"github.com/tyemirov/gix/v5/internal/repos/shared"
	"github.com/tyemirov/gix/v5/internal/semverdecision"
	"github.com/tyemirov/gix/v5/pkg/taskrunner"
)

const (
	nextCommandUse              = "next"
	nextCommandShortDescription = "Select the next release version"
	nextFormatFlagName          = "format"
	nextTimestampFlagName       = "release-timestamp"
	nextExcludeTagFlagName      = "exclude-tag"
)

// NextConfiguration contains the LLM policy used for established SemVer histories.
type NextConfiguration struct {
	LLMProxy           llmclient.LLMProxySelection
	Effort             string
	MaxTokens          int
	TimeoutSeconds     int
	ConnectionProfiles llmclient.ConnectionProfiles
}

// Sanitize normalizes the configured SemVer decision policy.
func (configuration NextConfiguration) Sanitize() NextConfiguration {
	sanitized := configuration
	sanitized.LLMProxy.Provider = strings.TrimSpace(configuration.LLMProxy.Provider)
	sanitized.LLMProxy.Model = strings.TrimSpace(configuration.LLMProxy.Model)
	sanitized.Effort = strings.TrimSpace(configuration.Effort)
	if sanitized.MaxTokens < 0 {
		sanitized.MaxTokens = 0
	}
	return sanitized
}

// VersionDecision is the canonical machine-readable successor contract.
type VersionDecision struct {
	Contract           string                `json:"contract"`
	Scheme             releaseversion.Scheme `json:"scheme"`
	SourceCommit       string                `json:"source_commit"`
	BoundaryTag        string                `json:"boundary_tag,omitempty"`
	PreviousVersion    string                `json:"previous_version,omitempty"`
	NextVersion        string                `json:"next_version"`
	Bump               releaseversion.Bump   `json:"bump,omitempty"`
	DeterministicFloor releaseversion.Bump   `json:"deterministic_floor,omitempty"`
	Reason             string                `json:"reason"`
	EvidenceSHA256     string                `json:"evidence_sha256,omitempty"`
	ReleaseTimestamp   string                `json:"release_timestamp,omitempty"`
}

// NextCommandBuilder assembles the release successor command.
type NextCommandBuilder struct {
	LoggerProvider               func() *zap.Logger
	HumanReadableLoggingProvider func() bool
	GitExecutor                  shared.GitExecutor
	ConfigurationProvider        func() NextConfiguration
	ClientFactory                llmclient.ClientFactory
	WorkingDirectoryProvider     func() (string, error)
	ReadFile                     func(string) ([]byte, error)
	Now                          func() time.Time
}

// Build constructs gix release next.
func (builder NextCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   nextCommandUse,
		Short: nextCommandShortDescription,
		Args:  cobra.NoArgs,
		RunE:  builder.run,
	}
	command.Flags().String(nextFormatFlagName, "text", "Output format: text or json")
	command.Flags().String(nextTimestampFlagName, "", "Release timestamp for CalVer in RFC3339 format")
	command.Flags().StringSlice(nextExcludeTagFlagName, nil, "Exclude a local tag from successor selection (repeatable)")
	return command, nil
}

func (builder NextCommandBuilder) run(command *cobra.Command, _ []string) error {
	format, formatError := command.Flags().GetString(nextFormatFlagName)
	if formatError != nil {
		return formatError
	}
	if format != "text" && format != "json" {
		return fmt.Errorf("release next format must be text or json: %q", format)
	}

	decision, decisionError := builder.decide(command)
	if decisionError != nil {
		return decisionError
	}
	if format == "json" {
		return json.NewEncoder(command.OutOrStdout()).Encode(decision)
	}
	_, writeError := fmt.Fprintln(command.OutOrStdout(), decision.NextVersion)
	return writeError
}

func (builder NextCommandBuilder) decide(command *cobra.Command) (VersionDecision, error) {
	workingDirectoryProvider := builder.WorkingDirectoryProvider
	if workingDirectoryProvider == nil {
		workingDirectoryProvider = os.Getwd
	}
	workingDirectory, workingDirectoryError := workingDirectoryProvider()
	if workingDirectoryError != nil {
		return VersionDecision{}, fmt.Errorf("resolve release repository working directory: %w", workingDirectoryError)
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
		return VersionDecision{}, dependencyError
	}

	repositoryRoot, rootError := executeGit(command.Context(), dependencies.GitExecutor, workingDirectory, "rev-parse", "--show-toplevel")
	if rootError != nil {
		return VersionDecision{}, fmt.Errorf("resolve release repository root: %w", rootError)
	}
	repositoryRoot = strings.TrimSpace(repositoryRoot)
	if repositoryRoot == "" {
		return VersionDecision{}, errors.New("release repository root is empty")
	}

	readFile := builder.ReadFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	configurationData, readError := readFile(filepath.Join(repositoryRoot, releaseversion.ConfigurationPath))
	if readError != nil {
		return VersionDecision{}, fmt.Errorf("read %s: %w", releaseversion.ConfigurationPath, readError)
	}
	releaseConfiguration, configurationError := releaseversion.ParseConfiguration(configurationData)
	if configurationError != nil {
		return VersionDecision{}, configurationError
	}

	sourceCommit, commitError := executeGit(command.Context(), dependencies.GitExecutor, repositoryRoot, "rev-parse", "HEAD")
	if commitError != nil {
		return VersionDecision{}, fmt.Errorf("resolve release source commit: %w", commitError)
	}
	sourceCommit = strings.TrimSpace(sourceCommit)
	if sourceCommit == "" {
		return VersionDecision{}, errors.New("release source commit is empty")
	}
	tagOutput, tagError := executeGit(command.Context(), dependencies.GitExecutor, repositoryRoot, "tag", "--list")
	if tagError != nil {
		return VersionDecision{}, fmt.Errorf("list release tags: %w", tagError)
	}
	excludedTags, excludedTagsError := command.Flags().GetStringSlice(nextExcludeTagFlagName)
	if excludedTagsError != nil {
		return VersionDecision{}, excludedTagsError
	}
	excluded := make(map[string]struct{}, len(excludedTags))
	for _, tag := range excludedTags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			return VersionDecision{}, errors.New("release next excluded tag cannot be empty")
		}
		excluded[trimmed] = struct{}{}
	}
	tags := make([]string, 0)
	for _, tag := range strings.Fields(tagOutput) {
		if _, isExcluded := excluded[tag]; !isExcluded {
			tags = append(tags, tag)
		}
	}

	switch releaseConfiguration.Scheme {
	case releaseversion.SchemeSemVer:
		return builder.decideSemVer(command, dependencies.GitExecutor, repositoryRoot, sourceCommit, tags)
	case releaseversion.SchemeCalVer:
		return builder.decideCalVer(command, sourceCommit, tags)
	default:
		return VersionDecision{}, fmt.Errorf("unsupported release scheme %q", releaseConfiguration.Scheme)
	}
}

func (builder NextCommandBuilder) decideSemVer(command *cobra.Command, gitExecutor shared.GitExecutor, repositoryRoot string, sourceCommit string, tags []string) (VersionDecision, error) {
	previous := releaseversion.LatestSemVer(tags)
	decision := VersionDecision{
		Contract:        releaseversion.ContractVersion,
		Scheme:          releaseversion.SchemeSemVer,
		SourceCommit:    sourceCommit,
		BoundaryTag:     previous,
		PreviousVersion: previous,
	}
	if previous == "" {
		decision.NextVersion = "v1.0.0"
		decision.Reason = "The repository has no canonical SemVer release, so the release starts at v1.0.0."
		return decision, nil
	}

	configuration := NextConfiguration{}
	if builder.ConfigurationProvider != nil {
		configuration = builder.ConfigurationProvider().Sanitize()
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
		return VersionDecision{}, clientError
	}
	semverResult, semverError := (semverdecision.Generator{
		GitExecutor: gitExecutor,
		Client:      client,
	}).Generate(command.Context(), semverdecision.Options{
		RepositoryPath: repositoryRoot,
		SinceReference: previous,
		MaxTokens:      configuration.MaxTokens,
	})
	if semverError != nil {
		return VersionDecision{}, semverError
	}
	next, nextError := releaseversion.NextSemVer(previous, semverResult.Bump)
	if nextError != nil {
		return VersionDecision{}, nextError
	}
	decision.NextVersion = next
	decision.Bump = semverResult.Bump
	decision.DeterministicFloor = semverResult.DeterministicFloor
	decision.Reason = semverResult.Reason
	decision.EvidenceSHA256 = semverResult.EvidenceSHA256
	return decision, nil
}

func (builder NextCommandBuilder) decideCalVer(command *cobra.Command, sourceCommit string, tags []string) (VersionDecision, error) {
	releaseTime := time.Time{}
	rawTimestamp, timestampFlagError := command.Flags().GetString(nextTimestampFlagName)
	if timestampFlagError != nil {
		return VersionDecision{}, timestampFlagError
	}
	if strings.TrimSpace(rawTimestamp) == "" {
		now := builder.Now
		if now == nil {
			now = time.Now
		}
		releaseTime = now()
	} else {
		parsedTimestamp, parseError := parseReleaseTimestamp(rawTimestamp)
		if parseError != nil {
			return VersionDecision{}, parseError
		}
		releaseTime = parsedTimestamp
	}
	previous := releaseversion.LatestCalVer(tags)
	next, nextError := releaseversion.NextCalVer(previous, releaseTime)
	if nextError != nil {
		return VersionDecision{}, nextError
	}
	return VersionDecision{
		Contract:         releaseversion.ContractVersion,
		Scheme:           releaseversion.SchemeCalVer,
		SourceCommit:     sourceCommit,
		BoundaryTag:      previous,
		PreviousVersion:  previous,
		NextVersion:      next,
		Reason:           "The release version is the canonical UTC CalVer for the supplied release timestamp.",
		ReleaseTimestamp: releaseTime.UTC().Format(time.RFC3339),
	}, nil
}

func executeGit(ctx context.Context, executor shared.GitExecutor, workingDirectory string, arguments ...string) (string, error) {
	result, executionError := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        arguments,
		WorkingDirectory: workingDirectory,
	})
	if executionError != nil {
		return "", executionError
	}
	return result.StandardOutput, nil
}

func parseReleaseTimestamp(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05-0700"} {
		if parsed, parseError := time.Parse(layout, strings.TrimSpace(value)); parseError == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("release timestamp must use RFC3339: %q", value)
}
