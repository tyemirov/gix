package web

import (
	"context"
	"io"
)

// CommandExecutor executes one gix command with explicit arguments and I/O streams.
type CommandExecutor func(context.Context, []string, io.Reader, io.Writer, io.Writer) error

// BranchCatalogLoader resolves branch metadata for one repository descriptor.
type BranchCatalogLoader func(context.Context, RepositoryDescriptor) BranchCatalog

// AuditInspector resolves typed audit rows for explicit roots.
type AuditInspector func(context.Context, AuditInspectionRequest) AuditInspectionResponse

// AuditChangeExecutor applies queued audit changes.
type AuditChangeExecutor func(context.Context, AuditChangeApplyRequest) AuditChangeApplyResponse

// ServerOptions configures the local web server.
type ServerOptions struct {
	Address           string
	Repositories      RepositoryCatalog
	Catalog           CommandCatalog
	LoadBranches      BranchCatalogLoader
	Execute           CommandExecutor
	InspectAudit      AuditInspector
	ApplyAuditChanges AuditChangeExecutor
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

// CommandTargetRequirement describes whether a target axis is unused, optional, or required.
type CommandTargetRequirement string

const (
	CommandTargetRequirementNone     CommandTargetRequirement = "none"
	CommandTargetRequirementOptional CommandTargetRequirement = "optional"
	CommandTargetRequirementRequired CommandTargetRequirement = "required"
)

// CommandTargetDescriptor captures how a command maps onto repository, ref, and path targets.
type CommandTargetDescriptor struct {
	Group         string                   `json:"group,omitempty"`
	Repository    CommandTargetRequirement `json:"repository"`
	Ref           CommandTargetRequirement `json:"ref"`
	Path          CommandTargetRequirement `json:"path"`
	SupportsBatch bool                     `json:"supports_batch"`
	DraftTemplate string                   `json:"draft_template,omitempty"`
}

// CommandDescriptor captures one command available through the web interface.
type CommandDescriptor struct {
	Path        string                  `json:"path"`
	Use         string                  `json:"use"`
	Name        string                  `json:"name"`
	Short       string                  `json:"short,omitempty"`
	Long        string                  `json:"long,omitempty"`
	Example     string                  `json:"example,omitempty"`
	Aliases     []string                `json:"aliases,omitempty"`
	Runnable    bool                    `json:"runnable"`
	Actionable  bool                    `json:"actionable"`
	Target      CommandTargetDescriptor `json:"target"`
	Flags       []FlagDescriptor        `json:"flags,omitempty"`
	Subcommands []string                `json:"subcommands,omitempty"`
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

// AuditInspectionRequest captures web-driven audit inputs.
type AuditInspectionRequest struct {
	Roots      []string `json:"roots"`
	IncludeAll bool     `json:"include_all"`
}

// AuditInspectionResponse returns typed audit rows to the browser.
type AuditInspectionResponse struct {
	Roots []string             `json:"roots,omitempty"`
	Rows  []AuditInspectionRow `json:"rows,omitempty"`
	Error string               `json:"error,omitempty"`
}

// AuditInspectionRow captures one typed audit result row.
type AuditInspectionRow struct {
	Path                   string `json:"path"`
	FolderName             string `json:"folder_name"`
	IsGitRepository        bool   `json:"is_git_repository"`
	FinalGitHubRepository  string `json:"final_github_repo"`
	OriginRemoteStatus     string `json:"origin_remote_status"`
	NameMatches            string `json:"name_matches"`
	RemoteDefaultBranch    string `json:"remote_default_branch"`
	LocalBranch            string `json:"local_branch"`
	InSync                 string `json:"in_sync"`
	RemoteProtocol         string `json:"remote_protocol"`
	OriginMatchesCanonical string `json:"origin_matches_canonical"`
	WorktreeDirty          string `json:"worktree_dirty"`
	DirtyFiles             string `json:"dirty_files"`
}

// AuditChangeKind identifies one queued audit remediation.
type AuditChangeKind string

const (
	AuditChangeKindRenameFolder          AuditChangeKind = "rename_folder"
	AuditChangeKindUpdateCanonical       AuditChangeKind = "update_remote_canonical"
	AuditChangeKindConvertProtocol       AuditChangeKind = "convert_protocol"
	AuditChangeKindSyncWithRemote        AuditChangeKind = "sync_with_remote"
	AuditChangeKindDeleteFolder          AuditChangeKind = "delete_folder"
	AuditChangeSyncStrategyRequireClean  string          = "require_clean"
	AuditChangeSyncStrategyStashChanges  string          = "stash_changes"
	AuditChangeSyncStrategyCommitChanges string          = "commit_changes"
)

// AuditQueuedChange captures one queued audit remediation from the browser.
type AuditQueuedChange struct {
	ID             string          `json:"id"`
	Kind           AuditChangeKind `json:"kind"`
	Path           string          `json:"path"`
	IncludeOwner   bool            `json:"include_owner,omitempty"`
	RequireClean   bool            `json:"require_clean,omitempty"`
	SourceProtocol string          `json:"source_protocol,omitempty"`
	TargetProtocol string          `json:"target_protocol,omitempty"`
	SyncStrategy   string          `json:"sync_strategy,omitempty"`
	ConfirmDelete  bool            `json:"confirm_delete,omitempty"`
}

// AuditChangeApplyRequest captures one queued apply request.
type AuditChangeApplyRequest struct {
	Changes []AuditQueuedChange `json:"changes"`
}

// AuditChangeApplyResult reports the result of one queued change.
type AuditChangeApplyResult struct {
	ID      string          `json:"id"`
	Kind    AuditChangeKind `json:"kind"`
	Path    string          `json:"path"`
	Status  string          `json:"status"`
	Message string          `json:"message,omitempty"`
	Stdout  string          `json:"stdout,omitempty"`
	Stderr  string          `json:"stderr,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// AuditChangeApplyResponse returns per-change execution results.
type AuditChangeApplyResponse struct {
	Results []AuditChangeApplyResult `json:"results,omitempty"`
	Error   string                   `json:"error,omitempty"`
}
