package repos

import (
	"fmt"
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
	License   LicenseConfiguration   `mapstructure:"license"`
	Add       AddConfiguration       `mapstructure:"add"`
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

// LicenseConfiguration describes configuration values for license distribution.
type LicenseConfiguration struct {
	DryRun          bool     `mapstructure:"dry_run"`
	AssumeYes       bool     `mapstructure:"assume_yes"`
	RepositoryRoots []string `mapstructure:"roots"`
	TemplatePath    string   `mapstructure:"template_path"`
	Content         string   `mapstructure:"content"`
	TargetPath      string   `mapstructure:"target_path"`
	Mode            string   `mapstructure:"mode"`
	RequireClean    bool     `mapstructure:"require_clean"`
	Branch          string   `mapstructure:"branch"`
	StartPoint      string   `mapstructure:"start_point"`
	PushRemote      string   `mapstructure:"push_remote"`
	CommitMessage   string   `mapstructure:"commit_message"`
}

// AddConfiguration describes configuration values for repo files add.
type AddConfiguration struct {
	DryRun          bool     `mapstructure:"dry_run"`
	AssumeYes       bool     `mapstructure:"assume_yes"`
	RepositoryRoots []string `mapstructure:"roots"`
	Path            string   `mapstructure:"path"`
	Content         string   `mapstructure:"content"`
	ContentFile     string   `mapstructure:"content_file"`
	Mode            string   `mapstructure:"mode"`
	Permissions     string   `mapstructure:"permissions"`
	RequireClean    bool     `mapstructure:"require_clean"`
	Branch          string   `mapstructure:"branch"`
	StartPoint      string   `mapstructure:"start_point"`
	Push            bool     `mapstructure:"push"`
	PushRemote      string   `mapstructure:"push_remote"`
	CommitMessage   string   `mapstructure:"commit_message"`
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
			Remote:          "origin",
			BranchPrefix:    "namespace-rewrite",
			CommitMessage:   "",
			Safeguards:      nil,
		},
		License: LicenseConfiguration{
			DryRun:          false,
			AssumeYes:       false,
			RepositoryRoots: nil,
			TemplatePath:    "",
			Content:         "",
			TargetPath:      "LICENSE",
			Mode:            "skip-if-exists",
			RequireClean:    true,
			Branch:          "license/{{ .Repository.Name }}",
			StartPoint:      "",
			PushRemote:      "origin",
			CommitMessage:   "",
		},
		Add: AddConfiguration{
			DryRun:          false,
			AssumeYes:       false,
			RepositoryRoots: nil,
			Path:            "",
			Content:         "",
			ContentFile:     "",
			Mode:            "skip-if-exists",
			Permissions:     "0644",
			RequireClean:    true,
			Branch:          "docs/add/{{ .Repository.Name }}",
			StartPoint:      "",
			Push:            true,
			PushRemote:      "origin",
			CommitMessage:   "",
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

// sanitize normalizes license configuration values.
func (configuration LicenseConfiguration) sanitize() LicenseConfiguration {
	sanitized := configuration
	sanitized.RepositoryRoots = rootutils.SanitizeConfigured(configuration.RepositoryRoots)
	sanitized.TemplatePath = strings.TrimSpace(configuration.TemplatePath)
	sanitized.Content = configuration.Content

	targetPath := strings.TrimSpace(configuration.TargetPath)
	if len(targetPath) == 0 {
		targetPath = "LICENSE"
	}
	cleanedTarget := filepath.Clean(targetPath)
	if cleanedTarget == "." {
		cleanedTarget = "LICENSE"
	}
	sanitized.TargetPath = cleanedTarget

	modeValue := strings.TrimSpace(strings.ToLower(configuration.Mode))
	if len(modeValue) == 0 {
		modeValue = "skip-if-exists"
	}
	sanitized.Mode = modeValue

	sanitized.RequireClean = configuration.RequireClean

	if len(strings.TrimSpace(configuration.Branch)) == 0 {
		sanitized.Branch = "license/{{ .Repository.Name }}"
	} else {
		sanitized.Branch = strings.TrimSpace(configuration.Branch)
	}

	sanitized.StartPoint = strings.TrimSpace(configuration.StartPoint)

	if len(strings.TrimSpace(configuration.PushRemote)) == 0 {
		sanitized.PushRemote = "origin"
	} else {
		sanitized.PushRemote = strings.TrimSpace(configuration.PushRemote)
	}

	commitMessage := strings.TrimSpace(configuration.CommitMessage)
	if len(commitMessage) == 0 {
		commitMessage = fmt.Sprintf("docs: add %s", sanitized.TargetPath)
	}
	sanitized.CommitMessage = commitMessage

	return sanitized
}

// Sanitize normalizes license configuration values.
func (configuration LicenseConfiguration) Sanitize() LicenseConfiguration {
	return configuration.sanitize()
}

// sanitize normalizes add configuration values.
func (configuration AddConfiguration) sanitize() AddConfiguration {
	sanitized := configuration
	sanitized.RepositoryRoots = rootutils.SanitizeConfigured(configuration.RepositoryRoots)
	sanitized.Path = strings.TrimSpace(configuration.Path)
	sanitized.Content = configuration.Content
	sanitized.ContentFile = strings.TrimSpace(configuration.ContentFile)

	modeValue := strings.TrimSpace(strings.ToLower(configuration.Mode))
	if len(modeValue) == 0 {
		modeValue = "skip-if-exists"
	}
	sanitized.Mode = modeValue

	sanitized.RequireClean = configuration.RequireClean

	if len(strings.TrimSpace(configuration.Branch)) == 0 {
		sanitized.Branch = "docs/add/{{ .Repository.Name }}"
	} else {
		sanitized.Branch = strings.TrimSpace(configuration.Branch)
	}

	sanitized.StartPoint = strings.TrimSpace(configuration.StartPoint)
	sanitized.Push = configuration.Push

	if len(strings.TrimSpace(configuration.PushRemote)) == 0 {
		sanitized.PushRemote = "origin"
	} else {
		sanitized.PushRemote = strings.TrimSpace(configuration.PushRemote)
	}

	if len(strings.TrimSpace(configuration.CommitMessage)) == 0 && len(sanitized.Path) > 0 {
		sanitized.CommitMessage = fmt.Sprintf("docs: add %s", sanitized.Path)
	} else {
		sanitized.CommitMessage = strings.TrimSpace(configuration.CommitMessage)
	}

	sanitized.Permissions = strings.TrimSpace(configuration.Permissions)
	if len(sanitized.Permissions) == 0 {
		sanitized.Permissions = "0644"
	}

	return sanitized
}

// Sanitize normalizes add configuration values.
func (configuration AddConfiguration) Sanitize() AddConfiguration {
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
