package syncflow

import (
	"strings"

	"github.com/tyemirov/gix/internal/llmclient"
	pathutils "github.com/tyemirov/gix/internal/utils/path"
)

var commandConfigurationRepositorySanitizer = pathutils.NewRepositoryPathSanitizerWithConfiguration(nil, pathutils.RepositoryPathSanitizerConfiguration{PruneNestedPaths: true})

const defaultCommitMessageTimeoutSeconds = 60

// CommitMessageConfiguration captures LLM settings for automatic worktree checkpoint commits and strict-sync merge resolution.
type CommitMessageConfiguration struct {
	LLMProxy           llmclient.LLMProxySelection  `mapstructure:"llm_proxy"`
	Effort             string                       `mapstructure:"effort"`
	MaxTokens          int                          `mapstructure:"max_completion_tokens"`
	TimeoutSeconds     int                          `mapstructure:"timeout_seconds"`
	ConnectionProfiles llmclient.ConnectionProfiles `mapstructure:"-"`
}

// PullRequestConfiguration captures optional PR metadata overrides for sync-created pull requests.
type PullRequestConfiguration struct {
	Title string `mapstructure:"title"`
	Body  string `mapstructure:"body"`
}

// CommandConfiguration captures configuration values for the sync command.
type CommandConfiguration struct {
	RepositoryRoots []string                   `mapstructure:"roots"`
	DefaultBranch   string                     `mapstructure:"branch"`
	RemoteName      string                     `mapstructure:"remote"`
	CreateIfMissing bool                       `mapstructure:"create_if_missing"`
	RequireClean    bool                       `mapstructure:"require_clean"`
	StashChanges    bool                       `mapstructure:"stash"`
	CommitChanges   bool                       `mapstructure:"commit"`
	CommitMessage   CommitMessageConfiguration `mapstructure:"commit_message"`
	PullRequest     PullRequestConfiguration   `mapstructure:"pull_request"`
}

// DefaultCommandConfiguration returns the baseline configuration for sync.
func DefaultCommandConfiguration() CommandConfiguration {
	return CommandConfiguration{
		CreateIfMissing: true,
		CommitMessage:   DefaultCommitMessageConfiguration(),
	}
}

// DefaultCommitMessageConfiguration returns baseline LLM settings for adoption commits and strict-sync merge resolution.
func DefaultCommitMessageConfiguration() CommitMessageConfiguration {
	return CommitMessageConfiguration{}
}

// Sanitize normalizes textual configuration values and repository roots.
func (configuration CommandConfiguration) Sanitize() CommandConfiguration {
	sanitized := configuration
	sanitized.RepositoryRoots = commandConfigurationRepositorySanitizer.Sanitize(configuration.RepositoryRoots)
	sanitized.DefaultBranch = strings.TrimSpace(configuration.DefaultBranch)
	sanitized.RemoteName = strings.TrimSpace(configuration.RemoteName)
	sanitized.CommitMessage = configuration.CommitMessage.Sanitize()
	sanitized.PullRequest = configuration.PullRequest.Sanitize()
	return sanitized
}

// Sanitize normalizes pull request configuration values.
func (configuration PullRequestConfiguration) Sanitize() PullRequestConfiguration {
	sanitized := configuration
	sanitized.Title = strings.TrimSpace(configuration.Title)
	sanitized.Body = strings.TrimSpace(configuration.Body)
	return sanitized
}

// Sanitize normalizes commit-message configuration values.
func (configuration CommitMessageConfiguration) Sanitize() CommitMessageConfiguration {
	sanitized := configuration

	sanitized.LLMProxy.Provider = strings.TrimSpace(configuration.LLMProxy.Provider)
	sanitized.LLMProxy.Model = strings.TrimSpace(configuration.LLMProxy.Model)

	if sanitized.MaxTokens < 0 {
		sanitized.MaxTokens = 0
	}

	return sanitized
}
