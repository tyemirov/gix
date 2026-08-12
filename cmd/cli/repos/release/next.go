package release

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/llmclient"
	"github.com/tyemirov/gix/internal/releaseversion"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/semverdecision"
	"github.com/tyemirov/gix/pkg/taskrunner"
)

const (
	nextCommandUse              = "next <semver|calver>"
	nextCommandShortDescription = "Select the next release version"
	nextFormatFlagName          = "format"
	nextTimestampFlagName       = "release-timestamp"
	nextExcludeTagFlagName      = "exclude-tag"
	nextFixedMajorFlagName      = "fixed-major"
	nextPreviousOutputFlagName  = "previous-release-output"
	nextCandidateOutputFlagName = "candidate-release-output"
	releaseOutputTransitionV1   = "mprlab.release-output-transition/v1"
	artifactSuccessorReason     = "The sealed release output changed for the same source commit, so the release is compatible."
	artifactSuccessorTagError   = "release output transition requires the latest SemVer tag at the source commit"
)

var releaseOutputIdentityPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type releaseOutputTransition struct {
	Previous       string
	Candidate      string
	EvidenceSHA256 string
}

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
	Policy             releaseversion.Policy `json:"policy"`
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
	Now                          func() time.Time
}

// Build constructs gix release next.
func (builder NextCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   nextCommandUse,
		Short: nextCommandShortDescription,
		Args:  cobra.ArbitraryArgs,
		RunE:  builder.run,
	}
	command.Flags().String(nextFormatFlagName, "text", "Output format: text or json")
	command.Flags().String(nextTimestampFlagName, "", "Release timestamp for CalVer in RFC3339 format")
	command.Flags().StringSlice(nextExcludeTagFlagName, nil, "Exclude a local tag from successor selection (repeatable)")
	command.Flags().Int(nextFixedMajorFlagName, 0, "Keep SemVer releases on one positive major")
	command.Flags().String(nextPreviousOutputFlagName, "", "Set the previous sealed release output SHA-256 identity")
	command.Flags().String(nextCandidateOutputFlagName, "", "Set the candidate sealed release output SHA-256 identity")
	return command, nil
}

func (builder NextCommandBuilder) run(command *cobra.Command, arguments []string) error {
	policy, policyError := invocationPolicy(command, arguments)
	if policyError != nil {
		return policyError
	}
	format, formatError := command.Flags().GetString(nextFormatFlagName)
	if formatError != nil {
		return formatError
	}
	if format != "text" && format != "json" {
		return fmt.Errorf("release next format must be text or json: %q", format)
	}
	outputTransition, outputTransitionError := releaseOutputTransitionFromCommand(command, policy)
	if outputTransitionError != nil {
		return outputTransitionError
	}

	decision, decisionError := builder.decide(command, policy, outputTransition)
	if decisionError != nil {
		return decisionError
	}
	if format == "json" {
		return json.NewEncoder(command.OutOrStdout()).Encode(decision)
	}
	_, writeError := fmt.Fprintln(command.OutOrStdout(), decision.NextVersion)
	return writeError
}

func releaseOutputTransitionFromCommand(command *cobra.Command, policy releaseversion.Policy) (*releaseOutputTransition, error) {
	previousSet := command.Flags().Changed(nextPreviousOutputFlagName)
	candidateSet := command.Flags().Changed(nextCandidateOutputFlagName)
	if !previousSet && !candidateSet {
		return nil, nil
	}
	if policy.Scheme() != releaseversion.SchemeSemVer {
		return nil, errors.New("release output transition is valid only for semver policy")
	}
	if !previousSet || !candidateSet {
		return nil, errors.New("release output transition requires previous and candidate identities")
	}
	previous, previousError := command.Flags().GetString(nextPreviousOutputFlagName)
	if previousError != nil {
		return nil, previousError
	}
	candidate, candidateError := command.Flags().GetString(nextCandidateOutputFlagName)
	if candidateError != nil {
		return nil, candidateError
	}
	return newReleaseOutputTransition(previous, candidate)
}

func newReleaseOutputTransition(previous string, candidate string) (*releaseOutputTransition, error) {
	previous = strings.TrimSpace(previous)
	candidate = strings.TrimSpace(candidate)
	if !releaseOutputIdentityPattern.MatchString(previous) || !releaseOutputIdentityPattern.MatchString(candidate) {
		return nil, errors.New("release output transition identities must be canonical SHA-256 values")
	}
	if previous == candidate {
		return nil, errors.New("release output transition identities must differ")
	}
	evidence := sha256.Sum256([]byte(strings.Join([]string{
		releaseOutputTransitionV1,
		previous,
		candidate,
		"",
	}, "\n")))
	return &releaseOutputTransition{
		Previous:       previous,
		Candidate:      candidate,
		EvidenceSHA256: fmt.Sprintf("%x", evidence),
	}, nil
}

func invocationPolicy(command *cobra.Command, arguments []string) (releaseversion.Policy, error) {
	if len(arguments) != 1 {
		return releaseversion.NewPolicy("", 0)
	}
	fixedMajor, fixedMajorError := command.Flags().GetInt(nextFixedMajorFlagName)
	if fixedMajorError != nil {
		return releaseversion.Policy{}, fixedMajorError
	}
	if command.Flags().Changed(nextFixedMajorFlagName) && fixedMajor < 1 {
		return releaseversion.Policy{}, fmt.Errorf("release fixed major must be positive: %d", fixedMajor)
	}
	policy, policyError := releaseversion.NewPolicy(arguments[0], fixedMajor)
	if policyError != nil {
		return releaseversion.Policy{}, policyError
	}
	if policy.Scheme() == releaseversion.SchemeSemVer && command.Flags().Changed(nextTimestampFlagName) {
		return releaseversion.Policy{}, errors.New("release timestamp is valid only for calver policy")
	}
	return policy, nil
}

func (builder NextCommandBuilder) decide(command *cobra.Command, policy releaseversion.Policy, outputTransition *releaseOutputTransition) (VersionDecision, error) {
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

	var decision VersionDecision
	var decisionError error
	switch policy.Scheme() {
	case releaseversion.SchemeSemVer:
		decision, decisionError = builder.decideSemVer(command, dependencies.GitExecutor, repositoryRoot, sourceCommit, tags, policy, outputTransition)
	case releaseversion.SchemeCalVer:
		decision, decisionError = builder.decideCalVer(command, sourceCommit, tags, policy)
	default:
		return VersionDecision{}, fmt.Errorf("unsupported release scheme %q", policy.Scheme())
	}
	if decisionError != nil {
		return VersionDecision{}, decisionError
	}
	return decision, nil
}

func (builder NextCommandBuilder) decideSemVer(command *cobra.Command, gitExecutor shared.GitExecutor, repositoryRoot string, sourceCommit string, tags []string, policy releaseversion.Policy, outputTransition *releaseOutputTransition) (VersionDecision, error) {
	fixedMajor := policy.FixedMajor()
	previous := releaseversion.LatestSemVer(tags)
	if fixedMajor > 0 {
		previous = releaseversion.LatestFixedMajorSemVer(tags, fixedMajor)
	}
	decision := VersionDecision{
		Contract:        releaseversion.ContractVersion,
		Policy:          policy,
		SourceCommit:    sourceCommit,
		BoundaryTag:     previous,
		PreviousVersion: previous,
	}
	if previous == "" {
		if outputTransition != nil {
			return VersionDecision{}, errors.New(artifactSuccessorTagError)
		}
		initialMajor := 1
		if fixedMajor > 0 {
			initialMajor = fixedMajor
		}
		decision.NextVersion = fmt.Sprintf("v%d.0.0", initialMajor)
		decision.Reason = fmt.Sprintf("The repository has no canonical SemVer release, so the release starts at v%d.0.0.", initialMajor)
		return decision, nil
	}
	boundaryCommit, boundaryError := executeGit(command.Context(), gitExecutor, repositoryRoot, "rev-parse", "--verify", previous+"^{commit}")
	if boundaryError != nil {
		return VersionDecision{}, fmt.Errorf("resolve SemVer boundary %s: %w", previous, boundaryError)
	}
	boundaryCommit = strings.TrimSpace(boundaryCommit)
	if boundaryCommit == "" {
		return VersionDecision{}, fmt.Errorf("resolved SemVer boundary %s is empty", previous)
	}
	if _, ancestorError := executeGit(command.Context(), gitExecutor, repositoryRoot, "merge-base", "--is-ancestor", boundaryCommit, sourceCommit); ancestorError != nil {
		return VersionDecision{}, fmt.Errorf("SemVer boundary %s is not an ancestor of source commit %s: %w", previous, sourceCommit, ancestorError)
	}
	if outputTransition != nil {
		if boundaryCommit != sourceCommit {
			return VersionDecision{}, errors.New(artifactSuccessorTagError)
		}
		next, nextError := releaseversion.NextSemVer(previous, releaseversion.BumpPatch)
		if fixedMajor > 0 {
			next, nextError = releaseversion.NextFixedMajorSemVer(previous, releaseversion.BumpPatch, fixedMajor)
		}
		if nextError != nil {
			return VersionDecision{}, nextError
		}
		decision.NextVersion = next
		decision.Bump = releaseversion.BumpPatch
		decision.DeterministicFloor = releaseversion.BumpPatch
		decision.Reason = artifactSuccessorReason
		decision.EvidenceSHA256 = outputTransition.EvidenceSHA256
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
		RepositoryPath:  repositoryRoot,
		SinceReference:  boundaryCommit,
		SourceReference: sourceCommit,
		BoundaryLabel:   previous,
		MaxTokens:       configuration.MaxTokens,
		FixedMajor:      fixedMajor,
	})
	if semverError != nil {
		return VersionDecision{}, semverError
	}
	var next string
	var nextError error
	if fixedMajor > 0 {
		next, nextError = releaseversion.NextFixedMajorSemVer(previous, semverResult.Bump, fixedMajor)
	} else {
		next, nextError = releaseversion.NextSemVer(previous, semverResult.Bump)
	}
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

func (builder NextCommandBuilder) decideCalVer(command *cobra.Command, sourceCommit string, tags []string, policy releaseversion.Policy) (VersionDecision, error) {
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
		Policy:           policy,
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
