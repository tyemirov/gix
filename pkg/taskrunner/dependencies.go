package taskrunner

import (
	"errors"
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

var (
	errOutputWriterMissing = errors.New("taskrunner.dependencies.output_missing")
	errErrorWriterMissing  = errors.New("taskrunner.dependencies.errors_missing")
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

// GitExecutionEnvironment captures executor-level collaborators.
type GitExecutionEnvironment struct {
	Logger                    *zap.Logger
	HumanReadableLogging      bool
	GitExecutor               shared.GitExecutor
	WorkflowRepositoryManager *gitrepo.RepositoryManager
	ResolvedRepositoryManager shared.GitRepositoryManager
	GitHubResolver            shared.GitHubMetadataResolver
	GitHubClient              *githubcli.Client
}

// RepositoryEnvironment exposes repository discovery and filesystem access.
type RepositoryEnvironment struct {
	Discoverer shared.RepositoryDiscoverer
	FileSystem shared.FileSystem
}

// PromptEnvironment captures terminal IO + prompt plumbing.
type PromptEnvironment struct {
	Prompter shared.ConfirmationPrompter
	Output   io.Writer
	Errors   io.Writer
}

// BuildDependencies resolves git, GitHub, filesystem, and prompting collaborators for workflow execution.
func BuildDependencies(config DependenciesConfig, options DependenciesOptions) (DependenciesResult, error) {
	gitEnvironment, gitEnvironmentError := NewGitExecutionEnvironment(config, options)
	if gitEnvironmentError != nil {
		return DependenciesResult{}, gitEnvironmentError
	}

	repositoryEnvironment := NewRepositoryEnvironment(config)

	promptEnvironment, promptEnvironmentError := NewPromptEnvironment(config, options)
	if promptEnvironmentError != nil {
		return DependenciesResult{}, promptEnvironmentError
	}

	workflowDependencies := workflow.Dependencies{
		Logger:               gitEnvironment.Logger,
		RepositoryDiscoverer: repositoryEnvironment.Discoverer,
		GitExecutor:          gitEnvironment.GitExecutor,
		RepositoryManager:    gitEnvironment.WorkflowRepositoryManager,
		GitHubClient:         gitEnvironment.GitHubClient,
		FileSystem:           repositoryEnvironment.FileSystem,
		Prompter:             promptEnvironment.Prompter,
		Output:               promptEnvironment.Output,
		Errors:               promptEnvironment.Errors,
		HumanReadableLogging: gitEnvironment.HumanReadableLogging,
	}

	return DependenciesResult{
		Workflow:             workflowDependencies,
		GitExecutor:          gitEnvironment.GitExecutor,
		RepositoryManager:    gitEnvironment.ResolvedRepositoryManager,
		GitHubResolver:       gitEnvironment.GitHubResolver,
		RepositoryDiscoverer: repositoryEnvironment.Discoverer,
		FileSystem:           repositoryEnvironment.FileSystem,
	}, nil
}

// NewGitExecutionEnvironment constructs the logger, git executor, and manager stack.
func NewGitExecutionEnvironment(config DependenciesConfig, options DependenciesOptions) (GitExecutionEnvironment, error) {
	logger := resolveLogger(config.LoggerProvider)
	humanReadable := false
	if config.HumanReadableLoggingProvider != nil {
		humanReadable = config.HumanReadableLoggingProvider()
	}

	gitExecutor, executorError := dependencies.ResolveGitExecutor(config.GitExecutor, logger, humanReadable)
	if executorError != nil {
		return GitExecutionEnvironment{}, fmt.Errorf("taskrunner.dependencies.git_executor: %w", executorError)
	}

	resolvedManager, managerError := dependencies.ResolveGitRepositoryManager(config.GitRepositoryManager, gitExecutor)
	if managerError != nil {
		return GitExecutionEnvironment{}, fmt.Errorf("taskrunner.dependencies.git_manager: %w", managerError)
	}

	var repositoryManager *gitrepo.RepositoryManager
	if typedManager, ok := resolvedManager.(*gitrepo.RepositoryManager); ok && typedManager != nil {
		repositoryManager = typedManager
	} else {
		constructedManager, constructedManagerError := gitrepo.NewRepositoryManager(gitExecutor)
		if constructedManagerError != nil {
			return GitExecutionEnvironment{}, fmt.Errorf("taskrunner.dependencies.git_manager_construct: %w", constructedManagerError)
		}
		repositoryManager = constructedManager
		resolvedManager = repositoryManager
	}

	var resolvedGitHubResolver shared.GitHubMetadataResolver
	var githubClient *githubcli.Client
	if options.SkipGitHubResolver {
		resolvedGitHubResolver = config.GitHubResolver
		if typedClient, ok := resolvedGitHubResolver.(*githubcli.Client); ok && typedClient != nil {
			githubClient = typedClient
		}
	} else {
		var resolverError error
		resolvedGitHubResolver, resolverError = dependencies.ResolveGitHubResolver(config.GitHubResolver, gitExecutor)
		if resolverError != nil {
			return GitExecutionEnvironment{}, fmt.Errorf("taskrunner.dependencies.github_resolver: %w", resolverError)
		}
		if typedClient, ok := resolvedGitHubResolver.(*githubcli.Client); ok && typedClient != nil {
			githubClient = typedClient
		}
	}

	return GitExecutionEnvironment{
		Logger:                    logger,
		HumanReadableLogging:      humanReadable,
		GitExecutor:               gitExecutor,
		WorkflowRepositoryManager: repositoryManager,
		ResolvedRepositoryManager: resolvedManager,
		GitHubResolver:            resolvedGitHubResolver,
		GitHubClient:              githubClient,
	}, nil
}

// NewRepositoryEnvironment wires repository discoverer and filesystem dependencies.
func NewRepositoryEnvironment(config DependenciesConfig) RepositoryEnvironment {
	return RepositoryEnvironment{
		Discoverer: dependencies.ResolveRepositoryDiscoverer(config.RepositoryDiscoverer),
		FileSystem: dependencies.ResolveFileSystem(config.FileSystem),
	}
}

// NewPromptEnvironment captures prompt + IO wiring, failing when writers are missing.
func NewPromptEnvironment(config DependenciesConfig, options DependenciesOptions) (PromptEnvironment, error) {
	outputWriter := options.Output
	if outputWriter == nil && options.Command != nil {
		outputWriter = options.Command.OutOrStdout()
	}
	if outputWriter == nil {
		return PromptEnvironment{}, errOutputWriterMissing
	}

	errorWriter := options.Errors
	if errorWriter == nil && options.Command != nil {
		errorWriter = options.Command.ErrOrStderr()
	}
	if errorWriter == nil {
		return PromptEnvironment{}, errErrorWriterMissing
	}

	options.Output = outputWriter
	options.Errors = errorWriter
	prompter := resolvePrompter(config.PrompterFactory, options)

	return PromptEnvironment{
		Prompter: prompter,
		Output:   outputWriter,
		Errors:   errorWriter,
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
	if options.Command != nil {
		inputReader = options.Command.InOrStdin()
	}

	outputWriter := options.Output
	if outputWriter == nil {
		outputWriter = os.Stdout
	}

	return prompt.NewIOConfirmationPrompter(inputReader, outputWriter)
}
