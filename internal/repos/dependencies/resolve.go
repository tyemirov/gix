package dependencies

import (
	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/discovery"
	"github.com/tyemirov/gix/internal/repos/filesystem"
	"github.com/tyemirov/gix/internal/repos/shared"
	"go.uber.org/zap"
)

// ResolveRepositoryDiscoverer returns the provided discoverer or a filesystem-backed default.
func ResolveRepositoryDiscoverer(existing shared.RepositoryDiscoverer) shared.RepositoryDiscoverer {
	if existing != nil {
		return existing
	}
	return discovery.NewFilesystemRepositoryDiscoverer()
}

// ResolveFileSystem returns the provided filesystem or an OS-backed default.
func ResolveFileSystem(existing shared.FileSystem) shared.FileSystem {
	if existing != nil {
		return existing
	}
	return filesystem.OSFileSystem{}
}

// ResolveGitExecutor returns the provided executor or constructs a shell-backed default.
func ResolveGitExecutor(existing shared.GitExecutor, logger *zap.Logger, humanReadableLogging bool) (shared.GitExecutor, error) {
	if existing != nil {
		return existing, nil
	}

	commandRunner := execshell.NewOSCommandRunner()
	shellExecutor, creationError := execshell.NewShellExecutor(logger, commandRunner, humanReadableLogging)
	if creationError != nil {
		return nil, creationError
	}
	return shellExecutor, nil
}

// ResolveGitRepositoryManager returns the provided repository manager or constructs one from the executor.
func ResolveGitRepositoryManager(existing shared.GitRepositoryManager, executor shared.GitExecutor) (shared.GitRepositoryManager, error) {
	if existing != nil {
		return existing, nil
	}
	return gitrepo.NewRepositoryManager(executor)
}

// ResolveGitHubResolver returns the provided resolver or creates a GitHub CLI-backed implementation.
func ResolveGitHubResolver(existing shared.GitHubMetadataResolver, executor shared.GitExecutor) (shared.GitHubMetadataResolver, error) {
	if existing != nil {
		return existing, nil
	}
	return githubcli.NewClient(executor)
}
