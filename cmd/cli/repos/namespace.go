package repos

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/repos/shared"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
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
	namespaceSafeguardsOptionKey       = "safeguards"
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
	PresetCatalogFactory         func() workflowcmd.PresetCatalog
	ExecutorFactory              func(nodes []*workflow.OperationNode, dependencies workflow.Dependencies) WorkflowExecutor
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
	command.Deprecated = "Use `gix workflow namespace --var old=github.com/old/org --var new=github.com/new/org` instead."

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

	assumeYes := configuration.AssumeYes
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

	oldPrefix := configuration.OldPrefix
	if command != nil && command.Flags().Changed(namespaceOldFlagName) {
		value, _ := command.Flags().GetString(namespaceOldFlagName)
		oldPrefix = strings.TrimSpace(value)
	}
	oldPrefix = strings.TrimSpace(oldPrefix)
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
	newPrefix = strings.TrimSpace(newPrefix)
	if len(newPrefix) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return fmt.Errorf("namespace rewrite requires --%s", namespaceNewFlagName)
	}

	branchPrefix := strings.TrimSpace(configuration.BranchPrefix)
	if command != nil && command.Flags().Changed(namespaceBranchPrefixFlagName) {
		value, _ := command.Flags().GetString(namespaceBranchPrefixFlagName)
		branchPrefix = strings.TrimSpace(value)
	}

	push := configuration.Push
	if command != nil && command.Flags().Changed(namespacePushFlagName) {
		value, _ := command.Flags().GetBool(namespacePushFlagName)
		push = value
	}

	remote := strings.TrimSpace(configuration.Remote)
	if command != nil && command.Flags().Changed(namespaceRemoteFlagName) {
		value, _ := command.Flags().GetString(namespaceRemoteFlagName)
		remote = strings.TrimSpace(value)
	}

	commitMessage := strings.TrimSpace(configuration.CommitMessage)
	if command != nil && command.Flags().Changed(namespaceCommitFlagName) {
		value, _ := command.Flags().GetString(namespaceCommitFlagName)
		commitMessage = strings.TrimSpace(value)
	}

	roots, rootsError := requireRepositoryRoots(command, arguments, configuration.RepositoryRoots)
	if rootsError != nil {
		return rootsError
	}

	dependencyResult, dependencyError := buildDependencies(
		command,
		dependencyInputs{
			LoggerProvider:               builder.LoggerProvider,
			HumanReadableLoggingProvider: builder.HumanReadableLoggingProvider,
			Discoverer:                   builder.Discoverer,
			GitExecutor:                  builder.GitExecutor,
			GitManager:                   builder.GitManager,
			FileSystem:                   builder.FileSystem,
			PrompterFactory:              builder.PrompterFactory,
		},
		taskrunner.DependenciesOptions{Command: command},
	)
	if dependencyError != nil {
		return dependencyError
	}

	workflowDependencies := dependencyResult.Workflow

	presetCatalog := builder.resolvePresetCatalog()
	presetConfiguration, presetFound, presetError := presetCatalog.Load("namespace")
	if presetError != nil {
		return fmt.Errorf("failed to load embedded namespace workflow: %w", presetError)
	}
	if !presetFound {
		return errors.New("embedded namespace workflow not available")
	}

	if len(configuration.Safeguards) > 0 {
		applyNamespaceSafeguards(&presetConfiguration, configuration.Safeguards)
	}

	nodes, operationsError := workflow.BuildOperations(presetConfiguration)
	if operationsError != nil {
		return fmt.Errorf("unable to build workflow operations: %w", operationsError)
	}

	executor := builder.resolveExecutor(nodes, workflowDependencies)

	variables := map[string]string{
		"namespace_old":  oldPrefix,
		"namespace_new":  newPrefix,
		"namespace_push": strconv.FormatBool(push),
	}
	if len(branchPrefix) > 0 {
		variables["namespace_branch_prefix"] = branchPrefix
	}
	if len(remote) > 0 {
		variables["namespace_remote"] = remote
	}
	if len(commitMessage) > 0 {
		variables["namespace_commit_message"] = commitMessage
	}

	runtimeOptions := workflow.RuntimeOptions{
		AssumeYes: assumeYes,
		Variables: variables,
	}

	if command != nil {
		fmt.Fprintln(command.ErrOrStderr(), "DEPRECATED: repo-namespace-rewrite will be removed; use `gix workflow namespace --var old=github.com/old/org --var new=github.com/new/org` instead.")
	}

	return executor.Execute(command.Context(), roots, runtimeOptions)
}

func (builder *NamespaceCommandBuilder) resolveConfiguration() NamespaceConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultToolsConfiguration().Namespace.Sanitize()
	}
	return builder.ConfigurationProvider().Sanitize()
}

func (builder *NamespaceCommandBuilder) resolvePresetCatalog() workflowcmd.PresetCatalog {
	if builder.PresetCatalogFactory != nil {
		if catalog := builder.PresetCatalogFactory(); catalog != nil {
			return catalog
		}
	}
	return workflowcmd.NewEmbeddedPresetCatalog()
}

func (builder *NamespaceCommandBuilder) resolveExecutor(nodes []*workflow.OperationNode, dependencies workflow.Dependencies) WorkflowExecutor {
	if builder.ExecutorFactory != nil {
		if executor := builder.ExecutorFactory(nodes, dependencies); executor != nil {
			return executor
		}
	}
	return workflow.NewExecutorFromNodes(nodes, dependencies)
}

func applyNamespaceSafeguards(configuration *workflow.Configuration, safeguards map[string]any) {
	if configuration == nil || len(safeguards) == 0 {
		return
	}

	for stepIndex := range configuration.Steps {
		step := &configuration.Steps[stepIndex]
		if step.Options == nil {
			continue
		}

		rawTasks, hasTasks := step.Options["tasks"]
		if !hasTasks {
			continue
		}

		taskEntries, ok := rawTasks.([]any)
		if !ok {
			continue
		}

		for taskIndex := range taskEntries {
			taskEntry, ok := taskEntries[taskIndex].(map[string]any)
			if !ok {
				continue
			}

			rawActions, hasActions := taskEntry["actions"]
			if !hasActions {
				continue
			}
			actionEntries, ok := rawActions.([]any)
			if !ok {
				continue
			}

			for actionIndex := range actionEntries {
				actionEntry, ok := actionEntries[actionIndex].(map[string]any)
				if !ok {
					continue
				}

				typeValue, _ := actionEntry["type"].(string)
				if !strings.EqualFold(strings.TrimSpace(typeValue), namespaceActionType) {
					continue
				}

				options, ok := actionEntry["options"].(map[string]any)
				if !ok || options == nil {
					options = map[string]any{}
					actionEntry["options"] = options
				}
				options[namespaceSafeguardsOptionKey] = cloneStringAnyMap(safeguards)
			}
		}
	}
}

func cloneStringAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = cloneArbitraryValue(value)
	}
	return cloned
}

func cloneArbitraryValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneStringAnyMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index := range typed {
			cloned[index] = cloneArbitraryValue(typed[index])
		}
		return cloned
	case []string:
		cloned := make([]string, len(typed))
		copy(cloned, typed)
		return cloned
	default:
		return typed
	}
}
