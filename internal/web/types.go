package web

import (
	"context"
	"io"
)

// CommandExecutor executes one gix command with explicit arguments and I/O streams.
type CommandExecutor func(context.Context, []string, io.Reader, io.Writer, io.Writer) error

// BranchCatalogLoader resolves branch metadata for one repository descriptor.
type BranchCatalogLoader func(context.Context, RepositoryDescriptor) BranchCatalog

// ServerOptions configures the local web server.
type ServerOptions struct {
	Address      string
	Repositories RepositoryCatalog
	Catalog      CommandCatalog
	LoadBranches BranchCatalogLoader
	Execute      CommandExecutor
}

// RepositoryCatalog describes the repositories visible to the web interface at launch time.
type RepositoryCatalog struct {
	LaunchPath           string                 `json:"launch_path,omitempty"`
	LaunchMode           string                 `json:"launch_mode,omitempty"`
	SelectedRepositoryID string                 `json:"selected_repository_id,omitempty"`
	Repositories         []RepositoryDescriptor `json:"repositories,omitempty"`
	Error                string                 `json:"error,omitempty"`
}

// RepositoryDescriptor captures one repository in the web interface explorer.
type RepositoryDescriptor struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Path           string `json:"path"`
	CurrentBranch  string `json:"current_branch,omitempty"`
	DefaultBranch  string `json:"default_branch,omitempty"`
	Dirty          bool   `json:"dirty"`
	ContextCurrent bool   `json:"context_current"`
	Error          string `json:"error,omitempty"`
}

// BranchCatalog describes the local repository branches visible to the web interface.
type BranchCatalog struct {
	RepositoryID   string             `json:"repository_id,omitempty"`
	RepositoryPath string             `json:"repository_path,omitempty"`
	Branches       []BranchDescriptor `json:"branches,omitempty"`
	Error          string             `json:"error,omitempty"`
}

// BranchDescriptor captures one local Git branch.
type BranchDescriptor struct {
	Name     string `json:"name"`
	Current  bool   `json:"current"`
	Upstream string `json:"upstream,omitempty"`
}

// CommandCatalog describes the CLI commands exposed through the web interface.
type CommandCatalog struct {
	Application string              `json:"application"`
	Commands    []CommandDescriptor `json:"commands"`
}

// CommandDescriptor captures one command available through the web interface.
type CommandDescriptor struct {
	Path        string           `json:"path"`
	Use         string           `json:"use"`
	Name        string           `json:"name"`
	Short       string           `json:"short,omitempty"`
	Long        string           `json:"long,omitempty"`
	Example     string           `json:"example,omitempty"`
	Aliases     []string         `json:"aliases,omitempty"`
	Runnable    bool             `json:"runnable"`
	Actionable  bool             `json:"actionable"`
	Flags       []FlagDescriptor `json:"flags,omitempty"`
	Subcommands []string         `json:"subcommands,omitempty"`
}

// FlagDescriptor captures the public metadata for one Cobra flag.
type FlagDescriptor struct {
	Name         string `json:"name"`
	Shorthand    string `json:"shorthand,omitempty"`
	Usage        string `json:"usage,omitempty"`
	Type         string `json:"type,omitempty"`
	Default      string `json:"default,omitempty"`
	NoOptDefault string `json:"no_opt_default,omitempty"`
	Required     bool   `json:"required"`
}
