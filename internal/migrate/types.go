package migrate

import (
	"context"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubcli"
)

const (
	requiredValueMessageConstant = "value required"
)

// CommandExecutor coordinates git and GitHub CLI invocations.
type CommandExecutor interface {
	ExecuteGit(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error)
	ExecuteGitHubCLI(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error)
}

// MigrationExecutor exposes the migration workflow execution contract.
type MigrationExecutor interface {
	Execute(executionContext context.Context, options MigrationOptions) (MigrationResult, error)
}

// GitHubOperations exposes the GitHub workflows required for migration.
type GitHubOperations interface {
	ResolveRepoMetadata(executionContext context.Context, repository string) (githubcli.RepositoryMetadata, error)
	GetPagesConfig(executionContext context.Context, repository string) (githubcli.PagesStatus, error)
	UpdatePagesConfig(executionContext context.Context, repository string, configuration githubcli.PagesConfiguration) error
	ListPullRequests(executionContext context.Context, repository string, options githubcli.PullRequestListOptions) ([]githubcli.PullRequest, error)
	UpdatePullRequestBase(executionContext context.Context, repository string, pullRequestNumber int, baseBranch string) error
	SetDefaultBranch(executionContext context.Context, repository string, branchName string) error
	CheckBranchProtection(executionContext context.Context, repository string, branchName string) (bool, error)
}

// BranchName describes a git branch identifier.
type BranchName string

// Supported branch name constants.
const (
	BranchMain   BranchName = BranchName("main")
	BranchMaster BranchName = BranchName("master")
)
