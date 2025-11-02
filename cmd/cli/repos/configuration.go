package repos

import (
	"path/filepath"
	"strings"

	rootutils "github.com/temirov/gix/internal/utils/roots"
)

// ToolsConfiguration captures repository command configuration sections.
type ToolsConfiguration struct {
	Remotes   RemotesConfiguration   `mapstructure:"remotes"`
	Protocol  ProtocolConfiguration  `mapstructure:"protocol"`
	Rename    RenameConfiguration    `mapstructure:"rename"`
	Remove    RemoveConfiguration    `mapstructure:"remove"`
	Replace   ReplaceConfiguration   `mapstructure:"replace"`
	Namespace NamespaceConfiguration `mapstructure:"namespace"`
}

// RemotesConfiguration describes configuration values for repo-remote-update.
type RemotesConfiguration struct {
	DryRun          bool     `mapstructure:"dry_run"`
	AssumeYes       bool     `mapstructure:"assume_yes"`
	Owner           string   `mapstructure:"owner"`
	RepositoryRoots []string `mapstructure:"roots"`
}

// ProtocolConfiguration describes configuration values for repo-protocol-convert.
type ProtocolConfiguration struct {
	DryRun          bool     `mapstructure:"dry_run"`
	AssumeYes       bool     `mapstructure:"assume_yes"`
	RepositoryRoots []string `mapstructure:"roots"`
	FromProtocol    string   `mapstructure:"from"`
	ToProtocol      string   `mapstructure:"to"`
}

// RenameConfiguration describes configuration values for repo-folders-rename.
type RenameConfiguration struct {
	DryRun               bool     `mapstructure:"dry_run"`
	AssumeYes            bool     `mapstructure:"assume_yes"`
	RequireCleanWorktree bool     `mapstructure:"require_clean"`
	RepositoryRoots      []string `mapstructure:"roots"`
	IncludeOwner         bool     `mapstructure:"include_owner"`
}

// RemoveConfiguration describes configuration values for repo history removal.
type RemoveConfiguration struct {
	DryRun          bool     `mapstructure:"dry_run"`
	AssumeYes       bool     `mapstructure:"assume_yes"`
	RepositoryRoots []string `mapstructure:"roots"`
	Remote          string   `mapstructure:"remote"`
	Push            bool     `mapstructure:"push"`
	Restore         bool     `mapstructure:"restore"`
	PushMissing     bool     `mapstructure:"push_missing"`
}

// ReplaceConfiguration describes configuration values for repo files replacement.
type ReplaceConfiguration struct {
	DryRun          bool     `mapstructure:"dry_run"`
	AssumeYes       bool     `mapstructure:"assume_yes"`
	RepositoryRoots []string `mapstructure:"roots"`
	Patterns        []string `mapstructure:"patterns"`
	Find            string   `mapstructure:"find"`
	Replace         string   `mapstructure:"replace"`
	Command         string   `mapstructure:"command"`
	RequireClean    bool     `mapstructure:"require_clean"`
	Branch          string   `mapstructure:"branch"`
	RequirePaths    []string `mapstructure:"paths"`
}

// NamespaceConfiguration describes configuration values for namespace rewrite.
type NamespaceConfiguration struct {
	DryRun          bool           `mapstructure:"dry_run"`
	AssumeYes       bool           `mapstructure:"assume_yes"`
	RepositoryRoots []string       `mapstructure:"roots"`
	OldPrefix       string         `mapstructure:"old"`
	NewPrefix       string         `mapstructure:"new"`
	Push            bool           `mapstructure:"push"`
	Remote          string         `mapstructure:"remote"`
	BranchPrefix    string         `mapstructure:"branch_prefix"`
	CommitMessage   string         `mapstructure:"commit_message"`
	Safeguards      map[string]any `mapstructure:"safeguards"`
}

// DefaultToolsConfiguration returns baseline configuration values for repository commands.
func DefaultToolsConfiguration() ToolsConfiguration {
	return ToolsConfiguration{
		Remotes: RemotesConfiguration{
			DryRun:          false,
			AssumeYes:       false,
			Owner:           "",
			RepositoryRoots: nil,
		},
		Protocol: ProtocolConfiguration{
			DryRun:          false,
			AssumeYes:       false,
			RepositoryRoots: nil,
			FromProtocol:    "",
			ToProtocol:      "",
		},
		Rename: RenameConfiguration{
			DryRun:               false,
			AssumeYes:            false,
			RequireCleanWorktree: false,
			RepositoryRoots:      nil,
			IncludeOwner:         false,
		},
		Remove: RemoveConfiguration{
			DryRun:          false,
			AssumeYes:       false,
			RepositoryRoots: nil,
			Remote:          "",
			Push:            true,
			Restore:         true,
			PushMissing:     false,
		},
		Replace: ReplaceConfiguration{
			DryRun:          false,
			AssumeYes:       false,
			RepositoryRoots: nil,
			Patterns:        nil,
			Find:            "",
			Replace:         "",
			Command:         "",
			RequireClean:    false,
			Branch:          "",
			RequirePaths:    nil,
		},
		Namespace: NamespaceConfiguration{
			DryRun:          false,
			AssumeYes:       false,
			RepositoryRoots: nil,
			OldPrefix:       "",
			NewPrefix:       "",
			Push:            true,
			Remote:          "origin",
			BranchPrefix:    "namespace-rewrite",
			CommitMessage:   "",
			Safeguards:      nil,
		},
	}
}

// sanitize normalizes repository configuration values.
func (configuration RemotesConfiguration) sanitize() RemotesConfiguration {
	sanitized := configuration
	sanitized.RepositoryRoots = rootutils.SanitizeConfigured(configuration.RepositoryRoots)
	sanitized.Owner = strings.TrimSpace(configuration.Owner)
	return sanitized
}

// sanitize normalizes protocol configuration values.
func (configuration ProtocolConfiguration) sanitize() ProtocolConfiguration {
	sanitized := configuration
	sanitized.RepositoryRoots = rootutils.SanitizeConfigured(configuration.RepositoryRoots)
	sanitized.FromProtocol = strings.TrimSpace(configuration.FromProtocol)
	sanitized.ToProtocol = strings.TrimSpace(configuration.ToProtocol)
	return sanitized
}

// sanitize normalizes rename configuration values.
func (configuration RenameConfiguration) sanitize() RenameConfiguration {
	sanitized := configuration
	sanitized.RepositoryRoots = rootutils.SanitizeConfigured(configuration.RepositoryRoots)
	return sanitized
}

// sanitize normalizes remove configuration values.
func (configuration RemoveConfiguration) sanitize() RemoveConfiguration {
	sanitized := configuration
	sanitized.RepositoryRoots = rootutils.SanitizeConfigured(configuration.RepositoryRoots)
	sanitized.Remote = strings.TrimSpace(configuration.Remote)
	return sanitized
}

// Sanitize normalizes remove configuration values.
func (configuration RemoveConfiguration) Sanitize() RemoveConfiguration {
	return configuration.sanitize()
}

// sanitize normalizes replace configuration values.
func (configuration ReplaceConfiguration) sanitize() ReplaceConfiguration {
	sanitized := configuration
	sanitized.RepositoryRoots = rootutils.SanitizeConfigured(configuration.RepositoryRoots)
	sanitized.Patterns = sanitizeReplacementPatterns(configuration.Patterns)
	sanitized.Find = strings.TrimSpace(configuration.Find)
	sanitized.Replace = configuration.Replace
	sanitized.Command = strings.TrimSpace(configuration.Command)
	sanitized.Branch = strings.TrimSpace(configuration.Branch)
	sanitized.RequirePaths = sanitizeReplacementPaths(configuration.RequirePaths)
	return sanitized
}

// Sanitize normalizes replace configuration values.
func (configuration ReplaceConfiguration) Sanitize() ReplaceConfiguration {
	return configuration.sanitize()
}

// sanitize normalizes namespace configuration values.
func (configuration NamespaceConfiguration) sanitize() NamespaceConfiguration {
	sanitized := configuration
	sanitized.RepositoryRoots = rootutils.SanitizeConfigured(configuration.RepositoryRoots)
	sanitized.OldPrefix = strings.TrimSpace(configuration.OldPrefix)
	sanitized.NewPrefix = strings.TrimSpace(configuration.NewPrefix)
	sanitized.Remote = strings.TrimSpace(configuration.Remote)
	sanitized.BranchPrefix = strings.TrimSpace(configuration.BranchPrefix)
	sanitized.CommitMessage = strings.TrimSpace(configuration.CommitMessage)
	if len(configuration.Safeguards) > 0 {
		sanitized.Safeguards = make(map[string]any, len(configuration.Safeguards))
		for key, value := range configuration.Safeguards {
			sanitized.Safeguards[key] = value
		}
	} else {
		sanitized.Safeguards = nil
	}
	return sanitized
}

// Sanitize normalizes namespace configuration values.
func (configuration NamespaceConfiguration) Sanitize() NamespaceConfiguration {
	return configuration.sanitize()
}

func sanitizeReplacementPatterns(patterns []string) []string {
	sanitized := make([]string, 0, len(patterns))
	seen := map[string]struct{}{}
	for _, pattern := range patterns {
		trimmed := strings.TrimSpace(pattern)
		if len(trimmed) == 0 {
			continue
		}
		normalized := strings.TrimPrefix(trimmed, "./")
		if len(normalized) == 0 {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		sanitized = append(sanitized, normalized)
	}
	return sanitized
}

func sanitizeReplacementPaths(paths []string) []string {
	sanitized := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, pathValue := range paths {
		trimmed := strings.TrimSpace(pathValue)
		if len(trimmed) == 0 {
			continue
		}
		normalized := strings.TrimPrefix(trimmed, "./")
		if len(normalized) == 0 {
			continue
		}
		cleaned := filepath.Clean(normalized)
		if _, exists := seen[cleaned]; exists {
			continue
		}
		seen[cleaned] = struct{}{}
		sanitized = append(sanitized, cleaned)
	}
	return sanitized
}
