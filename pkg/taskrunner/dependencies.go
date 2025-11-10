package taskrunner

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/gitrepo"
	"github.com/temirov/gix/internal/repos/dependencies"
	"github.com/temirov/gix/internal/repos/prompt"
	"github.com/temirov/gix/internal/repos/shared"
	"github.com/temirov/gix/internal/workflow"
)

// DependenciesConfig captures providers required to build workflow dependencies.
type DependenciesConfig struct {
	LoggerProvider               func() *zap.Logger
	HumanReadableLoggingProvider func() bool
	RepositoryDiscoverer         shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitRepositoryManager         shared.GitRepositoryManager
	GitHubResolver               shared.GitHubMetadataResolver
	FileSystem                   shared.FileSystem
	PrompterFactory              func(*cobra.Command) shared.ConfirmationPrompter
}

// DependenciesOptions allows per-command overrides when resolving workflow dependencies.
type DependenciesOptions struct {
	Command            *cobra.Command
	Output             io.Writer
	Errors             io.Writer
	Prompter           shared.ConfirmationPrompter
	DisablePrompter    bool
	SkipGitHubResolver bool
}

// DependenciesResult exposes resolved collaborators along with their workflow wrapper.
type DependenciesResult struct {
	Workflow             workflow.Dependencies
	GitExecutor          shared.GitExecutor
	RepositoryManager    shared.GitRepositoryManager
	GitHubResolver       shared.GitHubMetadataResolver
	RepositoryDiscoverer shared.RepositoryDiscoverer
	FileSystem           shared.FileSystem
}

// BuildDependencies resolves git, GitHub, filesystem, and prompting collaborators for workflow execution.
func BuildDependencies(config DependenciesConfig, options DependenciesOptions) (DependenciesResult, error) {
	logger := resolveLogger(config.LoggerProvider)
	humanReadable := false
	if config.HumanReadableLoggingProvider != nil {
		humanReadable = config.HumanReadableLoggingProvider()
	}

	gitExecutor, executorError := dependencies.ResolveGitExecutor(config.GitExecutor, logger, humanReadable)
	if executorError != nil {
		return DependenciesResult{}, fmt.Errorf("taskrunner.dependencies.git_executor: %w", executorError)
	}

	resolvedManager, managerError := dependencies.ResolveGitRepositoryManager(config.GitRepositoryManager, gitExecutor)
	if managerError != nil {
		return DependenciesResult{}, fmt.Errorf("taskrunner.dependencies.git_manager: %w", managerError)
	}

	var repositoryManager *gitrepo.RepositoryManager
	if typedManager, ok := resolvedManager.(*gitrepo.RepositoryManager); ok && typedManager != nil {
		repositoryManager = typedManager
	} else {
		constructedManager, constructedManagerError := gitrepo.NewRepositoryManager(gitExecutor)
		if constructedManagerError != nil {
			return DependenciesResult{}, fmt.Errorf("taskrunner.dependencies.git_manager_construct: %w", constructedManagerError)
		}
		repositoryManager = constructedManager
		resolvedManager = repositoryManager
	}

	resolvedGitHubResolver := config.GitHubResolver
	var githubClient *githubcli.Client
	if options.SkipGitHubResolver {
		if typedClient, ok := resolvedGitHubResolver.(*githubcli.Client); ok && typedClient != nil {
			githubClient = typedClient
		}
	} else {
		var resolverError error
		resolvedGitHubResolver, resolverError = dependencies.ResolveGitHubResolver(config.GitHubResolver, gitExecutor)
		if resolverError != nil {
			return DependenciesResult{}, fmt.Errorf("taskrunner.dependencies.github_resolver: %w", resolverError)
		}

		if typedClient, ok := resolvedGitHubResolver.(*githubcli.Client); ok && typedClient != nil {
			githubClient = typedClient
		} else {
			constructedClient, constructedClientError := githubcli.NewClient(gitExecutor)
			if constructedClientError != nil {
				return DependenciesResult{}, fmt.Errorf("taskrunner.dependencies.github_client: %w", constructedClientError)
			}
			githubClient = constructedClient
			resolvedGitHubResolver = githubClient
		}
	}

	repositoryDiscoverer := dependencies.ResolveRepositoryDiscoverer(config.RepositoryDiscoverer)
	fileSystem := dependencies.ResolveFileSystem(config.FileSystem)

	outputWriter := resolveWriter(options.Output, options.Command, true)
	errorWriter := resolveWriter(options.Errors, options.Command, false)
	prompter := resolvePrompter(config.PrompterFactory, options)

	workflowDependencies := workflow.Dependencies{
		Logger:               logger,
		RepositoryDiscoverer: repositoryDiscoverer,
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         githubClient,
		FileSystem:           fileSystem,
		Prompter:             prompter,
		Output:               outputWriter,
		Errors:               errorWriter,
		HumanReadableLogging: humanReadable,
	}

	return DependenciesResult{
		Workflow:             workflowDependencies,
		GitExecutor:          gitExecutor,
		RepositoryManager:    resolvedManager,
		GitHubResolver:       resolvedGitHubResolver,
		RepositoryDiscoverer: repositoryDiscoverer,
		FileSystem:           fileSystem,
	}, nil
}

func resolveLogger(provider func() *zap.Logger) *zap.Logger {
	if provider == nil {
		return zap.NewNop()
	}
	logger := provider()
	if logger == nil {
		return zap.NewNop()
	}
	return logger
}

func resolveWriter(provided io.Writer, command *cobra.Command, useStdout bool) io.Writer {
	if provided != nil {
		return provided
	}
	if command != nil {
		if useStdout {
			if writer := command.OutOrStdout(); writer != nil && writer != io.Discard {
				return writer
			}
		} else {
			if writer := command.ErrOrStderr(); writer != nil && writer != io.Discard {
				return writer
			}
		}
	}
	if useStdout {
		return os.Stdout
	}
	return os.Stderr
}

func resolvePrompter(factory func(*cobra.Command) shared.ConfirmationPrompter, options DependenciesOptions) shared.ConfirmationPrompter {
	if options.DisablePrompter {
		return nil
	}
	if options.Prompter != nil {
		return options.Prompter
	}
	if factory != nil {
		if prompter := factory(options.Command); prompter != nil {
			return prompter
		}
	}

	var inputReader io.Reader = os.Stdin
	var outputWriter io.Writer = os.Stdout
	if options.Command != nil {
		inputReader = options.Command.InOrStdin()
		outputWriter = options.Command.OutOrStdout()
	}
	return prompt.NewIOConfirmationPrompter(inputReader, outputWriter)
}
