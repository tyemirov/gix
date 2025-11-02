package repos

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/gitrepo"
	"github.com/temirov/gix/internal/repos/dependencies"
	"github.com/temirov/gix/internal/repos/shared"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
)

const (
	namespaceUseConstant               = "repo-namespace-rewrite"
	namespaceShortDescription          = "Rewrite Go module namespaces across repositories"
	namespaceLongDescription           = "repo-namespace-rewrite updates go.mod and Go imports to replace an old module namespace with a new one."
	namespaceOldFlagName               = "old"
	namespaceNewFlagName               = "new"
	namespaceBranchPrefixFlagName      = "branch-prefix"
	namespaceBranchPrefixOptionKeyName = "branch_prefix"
	namespaceRemoteFlagName            = "remote"
	namespacePushFlagName              = "push"
	namespaceCommitFlagName            = "commit-message"
	namespaceCommitOptionKeyName       = "commit_message"
	namespaceActionType                = "repo.namespace.rewrite"
)

// NamespaceCommandBuilder assembles the namespace rewrite command.
type NamespaceCommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
	PrompterFactory              PrompterFactory
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() NamespaceConfiguration
	TaskRunnerFactory            func(workflow.Dependencies) TaskRunnerExecutor
}

// Build constructs the namespace rewrite Cobra command.
func (builder *NamespaceCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   namespaceUseConstant,
		Short: namespaceShortDescription,
		Long:  namespaceLongDescription,
		Args:  cobra.NoArgs,
		RunE:  builder.run,
	}

	command.Flags().String(namespaceOldFlagName, "", "Old module namespace prefix (required) e.g. github.com/old/org")
	command.Flags().String(namespaceNewFlagName, "", "New module namespace prefix (required) e.g. github.com/new/org")
	command.Flags().String(namespaceBranchPrefixFlagName, "", "Branch name prefix to use when creating rewrite branches")
	command.Flags().String(namespaceRemoteFlagName, "", "Remote to push rewritten branches to (default origin)")
	command.Flags().Bool(namespacePushFlagName, true, "Push rewritten branches to the remote")
	command.Flags().String(namespaceCommitFlagName, "", "Override commit message for namespace rewrite")

	return command, nil
}

func (builder *NamespaceCommandBuilder) run(command *cobra.Command, arguments []string) error {
	if command != nil {
		if command.OutOrStdout() == io.Discard {
			command.SetOut(os.Stdout)
		}
		if command.ErrOrStderr() == io.Discard {
			command.SetErr(os.Stderr)
		}
	}

	configuration := builder.resolveConfiguration()
	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

	dryRun := configuration.DryRun
	if executionFlagsAvailable && executionFlags.DryRunSet {
		dryRun = executionFlags.DryRun
	}

	assumeYes := configuration.AssumeYes
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

	oldPrefix := configuration.OldPrefix
	if command != nil && command.Flags().Changed(namespaceOldFlagName) {
		value, _ := command.Flags().GetString(namespaceOldFlagName)
		oldPrefix = strings.TrimSpace(value)
	}
	if len(oldPrefix) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return fmt.Errorf("namespace rewrite requires --%s", namespaceOldFlagName)
	}

	newPrefix := configuration.NewPrefix
	if command != nil && command.Flags().Changed(namespaceNewFlagName) {
		value, _ := command.Flags().GetString(namespaceNewFlagName)
		newPrefix = strings.TrimSpace(value)
	}
	if len(newPrefix) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return fmt.Errorf("namespace rewrite requires --%s", namespaceNewFlagName)
	}

	branchPrefix := configuration.BranchPrefix
	if command != nil && command.Flags().Changed(namespaceBranchPrefixFlagName) {
		value, _ := command.Flags().GetString(namespaceBranchPrefixFlagName)
		branchPrefix = strings.TrimSpace(value)
	}

	push := configuration.Push
	if command != nil && command.Flags().Changed(namespacePushFlagName) {
		value, _ := command.Flags().GetBool(namespacePushFlagName)
		push = value
	}

	remote := configuration.Remote
	if command != nil && command.Flags().Changed(namespaceRemoteFlagName) {
		value, _ := command.Flags().GetString(namespaceRemoteFlagName)
		remote = strings.TrimSpace(value)
	}

	commitMessage := configuration.CommitMessage
	if command != nil && command.Flags().Changed(namespaceCommitFlagName) {
		value, _ := command.Flags().GetString(namespaceCommitFlagName)
		commitMessage = strings.TrimSpace(value)
	}

	roots, rootsError := requireRepositoryRoots(command, arguments, configuration.RepositoryRoots)
	if rootsError != nil {
		return rootsError
	}

	logger := resolveLogger(builder.LoggerProvider)
	humanReadableLogging := false
	if builder.HumanReadableLoggingProvider != nil {
		humanReadableLogging = builder.HumanReadableLoggingProvider()
	}

	gitExecutor, executorError := dependencies.ResolveGitExecutor(builder.GitExecutor, logger, humanReadableLogging)
	if executorError != nil {
		return executorError
	}

	gitManager, managerError := dependencies.ResolveGitRepositoryManager(builder.GitManager, gitExecutor)
	if managerError != nil {
		return managerError
	}

	repositoryDiscoverer := dependencies.ResolveRepositoryDiscoverer(builder.Discoverer)
	fileSystem := dependencies.ResolveFileSystem(builder.FileSystem)
	githubClient, githubClientError := githubcli.NewClient(gitExecutor)
	if githubClientError != nil {
		return githubClientError
	}

	prompter := resolvePrompter(builder.PrompterFactory, command)
	trackingPrompter := newCascadingConfirmationPrompter(prompter, assumeYes)

	var repositoryManager *gitrepo.RepositoryManager
	if concrete, ok := gitManager.(*gitrepo.RepositoryManager); ok {
		repositoryManager = concrete
	} else {
		constructedManager, constructedErr := gitrepo.NewRepositoryManager(gitExecutor)
		if constructedErr != nil {
			return constructedErr
		}
		repositoryManager = constructedManager
	}

	dependenciesSet := workflow.Dependencies{
		Logger:               logger,
		RepositoryDiscoverer: repositoryDiscoverer,
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         githubClient,
		FileSystem:           fileSystem,
		Prompter:             trackingPrompter,
		Output:               command.OutOrStdout(),
		Errors:               command.ErrOrStderr(),
	}

	taskRunner := ResolveTaskRunner(builder.TaskRunnerFactory, dependenciesSet)

	actionOptions := map[string]any{
		namespaceOldFlagName:  oldPrefix,
		namespaceNewFlagName:  newPrefix,
		namespacePushFlagName: push,
	}
	if len(branchPrefix) > 0 {
		actionOptions[namespaceBranchPrefixOptionKeyName] = branchPrefix
		actionOptions[namespaceBranchPrefixFlagName] = branchPrefix
	}
	if len(remote) > 0 {
		actionOptions[namespaceRemoteFlagName] = remote
	}
	if len(commitMessage) > 0 {
		actionOptions[namespaceCommitOptionKeyName] = commitMessage
		actionOptions[namespaceCommitFlagName] = commitMessage
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        "Rewrite Go namespace",
		EnsureClean: false,
		Safeguards:  configuration.Safeguards,
		Actions: []workflow.TaskActionDefinition{
			{Type: namespaceActionType, Options: actionOptions},
		},
	}

	runtimeOptions := workflow.RuntimeOptions{
		DryRun:    dryRun,
		AssumeYes: trackingPrompter.AssumeYes(),
	}

	return taskRunner.Run(command.Context(), roots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
}

func (builder *NamespaceCommandBuilder) resolveConfiguration() NamespaceConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultToolsConfiguration().Namespace.Sanitize()
	}
	return builder.ConfigurationProvider().Sanitize()
}
