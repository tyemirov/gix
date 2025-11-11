package release

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	repocli "github.com/temirov/gix/cmd/cli/repos"
	"github.com/temirov/gix/internal/repos/shared"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	rootutils "github.com/temirov/gix/internal/utils/roots"
	"github.com/temirov/gix/internal/workflow"
	"github.com/temirov/gix/pkg/taskrunner"
)

const (
	retagCommandUseTemplate      = "retag --map <tag=target> [--map <tag=target>...]"
	retagCommandShortDescription = "Retag existing releases to new commits"
	retagCommandLongDescription  = "retag deletes and recreates existing annotated tags so they point to the provided commits, then force-pushes the updated tags to the configured remote."
	retagCommandAlias            = "fix"
	retagMappingFlagName         = "map"
	retagMappingFlagUsage        = "Mapping of tag=target (repeatable)"
	retagMessageTemplateFlagName = "message-template"
	retagMessageTemplateUsage    = "Optional template for retag messages (placeholders: {{tag}}, {{target}})"
)

// RetagCommandBuilder assembles the repo release retag command.
type RetagCommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() CommandConfiguration
	TaskRunnerFactory            func(workflow.Dependencies) repocli.TaskRunnerExecutor
}

// Build constructs the retag Cobra command.
func (builder *RetagCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   retagCommandUseTemplate,
		Short: retagCommandShortDescription,
		Long:  retagCommandLongDescription,
		Args:  cobra.NoArgs,
		RunE:  builder.run,
	}

	command.Flags().StringSlice(retagMappingFlagName, nil, retagMappingFlagUsage)
	command.Flags().String(retagMessageTemplateFlagName, "", retagMessageTemplateUsage)

	return command, nil
}

func (builder *RetagCommandBuilder) run(command *cobra.Command, _ []string) error {
	configuration := builder.resolveConfiguration()

	executionFlags, executionFlagsAvailable := flagutils.ResolveExecutionFlags(command)

	messageTemplate := configuration.Message
	if command != nil && command.Flags().Changed(retagMessageTemplateFlagName) {
		templateValue, templateError := command.Flags().GetString(retagMessageTemplateFlagName)
		if templateError != nil {
			return templateError
		}
		messageTemplate = strings.TrimSpace(templateValue)
	}

	mappingValues, mappingError := command.Flags().GetStringSlice(retagMappingFlagName)
	if mappingError != nil {
		return mappingError
	}
	if len(mappingValues) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return errors.New("retag requires at least one --map <tag=target> entry")
	}

	repositoryRoots, rootsError := rootutils.Resolve(command, nil, configuration.RepositoryRoots)
	if rootsError != nil {
		return rootsError
	}

	dependencyResult, dependencyError := taskrunner.BuildDependencies(
		taskrunner.DependenciesConfig{
			LoggerProvider:               builder.LoggerProvider,
			HumanReadableLoggingProvider: builder.HumanReadableLoggingProvider,
			RepositoryDiscoverer:         builder.Discoverer,
			GitExecutor:                  builder.GitExecutor,
			GitRepositoryManager:         builder.GitManager,
			FileSystem:                   builder.FileSystem,
		},
		taskrunner.DependenciesOptions{
			Command:         command,
			DisablePrompter: true,
		},
	)
	if dependencyError != nil {
		return dependencyError
	}

	taskRunner := repocli.ResolveTaskRunner(builder.TaskRunnerFactory, dependencyResult.Workflow)

	mappings := make([]any, 0, len(mappingValues))
	for _, raw := range mappingValues {
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			return errors.New("mapping values must follow tag=target syntax")
		}
		tag := strings.TrimSpace(parts[0])
		target := strings.TrimSpace(parts[1])
		if len(tag) == 0 || len(target) == 0 {
			return errors.New("mapping values must contain non-empty tag and target")
		}

		entry := map[string]any{
			"tag":    tag,
			"target": target,
		}
		if len(messageTemplate) > 0 {
			message := strings.ReplaceAll(messageTemplate, "{{tag}}", tag)
			message = strings.ReplaceAll(message, "{{target}}", target)
			entry["message"] = message
		}
		mappings = append(mappings, entry)
	}

	remoteName := configuration.RemoteName
	if executionFlagsAvailable && executionFlags.RemoteSet {
		override := strings.TrimSpace(executionFlags.Remote)
		if len(override) > 0 {
			remoteName = override
		}
	}

	actionOptions := map[string]any{
		"mappings": mappings,
	}
	if len(remoteName) > 0 {
		actionOptions["remote"] = remoteName
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        "Retag release tags",
		EnsureClean: false,
		Actions: []workflow.TaskActionDefinition{
			{Type: "repo.release.retag", Options: actionOptions},
		},
		Commit: workflow.TaskCommitDefinition{},
	}

	assumeYes := false
	if executionFlagsAvailable && executionFlags.AssumeYesSet {
		assumeYes = executionFlags.AssumeYes
	}

	runtimeOptions := workflow.RuntimeOptions{AssumeYes: assumeYes}

	_, runErr := taskRunner.Run(command.Context(), repositoryRoots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
	return runErr
}

func (builder *RetagCommandBuilder) resolveConfiguration() CommandConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultCommandConfiguration()
	}
	return builder.ConfigurationProvider().Sanitize()
}
