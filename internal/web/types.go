package web

import (
	"context"
	"io"
)

// CommandExecutor executes one gix command with explicit arguments and I/O streams.
type CommandExecutor func(context.Context, []string, io.Reader, io.Writer, io.Writer) error

// ServerOptions configures the local web server.
type ServerOptions struct {
	Address  string
	Branches BranchCatalog
	Catalog  CommandCatalog
	Execute  CommandExecutor
}

// BranchCatalog describes the local repository branches visible to the web interface.
type BranchCatalog struct {
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
