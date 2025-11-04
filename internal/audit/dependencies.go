package audit

import "github.com/tyemirov/gix/internal/repos/shared"

// RepositoryDiscoverer finds git repositories rooted under the provided paths.
type RepositoryDiscoverer = shared.RepositoryDiscoverer

// GitExecutor exposes the subset of shell execution used by the audit command.
type GitExecutor = shared.GitExecutor

// GitRepositoryManager exposes repository-level git operations.
type GitRepositoryManager = shared.GitRepositoryManager

// GitHubMetadataResolver resolves canonical repository metadata via GitHub CLI.
type GitHubMetadataResolver = shared.GitHubMetadataResolver

// ConfirmationPrompter prompts users for confirmation during mutable operations.
type ConfirmationPrompter = shared.ConfirmationPrompter

// FileSystem provides filesystem operations required by the audit workflows.
type FileSystem = shared.FileSystem
