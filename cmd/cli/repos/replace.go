package repos

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/dependencies"
	"github.com/tyemirov/gix/internal/repos/shared"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	replaceUseConstant          = "repo-files-replace"
	replaceShortDescription     = "Perform string replacements across repository files"
	replaceLongDescription      = "repo-files-replace applies string substitutions to files matched by glob patterns, optionally enforcing safeguards and running a follow-up command."
	replacePatternFlagName      = "pattern"
	replaceFindFlagName         = "find"
	replaceReplaceFlagName      = "replace"
	replaceCommandFlagName      = "command"
	replaceRequireCleanFlagName = "require-clean"
	replaceBranchFlagName       = "branch"
	replaceRequirePathFlagName  = "require-path"
	replaceMissingFindError     = "replacement requires --find"
	replacementTaskName         = "Replace repository file content"
	replacementActionType       = "repo.files.replace"
)

// ReplaceCommandBuilder assembles the repo-files-replace command.
type ReplaceCommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() ReplaceConfiguration
	TaskRunnerFactory            func(workflow.Dependencies) TaskRunnerExecutor
}

// Build constructs the repo-files-replace command.
func (builder *ReplaceCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   replaceUseConstant,
		Short: replaceShortDescription,
		Long:  replaceLongDescription,
		RunE:  builder.run,
	}

	command.Flags().StringSlice(replacePatternFlagName, nil, "Glob pattern (repeatable) selecting files to update")
	command.Flags().String(replaceFindFlagName, "", "String to search for within matched files (required)")
	command.Flags().String(replaceReplaceFlagName, "", "String to substitute in place of the search string")
	command.Flags().String(replaceCommandFlagName, "", "Optional command to run after replacements (quoted, e.g. \"go fmt ./...\")")
	flagutils.AddToggleFlag(command.Flags(), nil, replaceRequireCleanFlagName, "", false, "Require a clean working tree before applying replacements")
	command.Flags().String(replaceBranchFlagName, "", "Require the repository to be on the specified branch before applying replacements")
	command.Flags().StringSlice(replaceRequirePathFlagName, nil, "Require the specified relative path to exist before applying replacements (repeatable)")

	return command, nil
}

func (builder *ReplaceCommandBuilder) run(command *cobra.Command, _ []string) error {
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

	patterns := append([]string{}, configuration.Patterns...)
	if command != nil && command.Flags().Changed(replacePatternFlagName) {
		flagPatterns, flagError := command.Flags().GetStringSlice(replacePatternFlagName)
		if flagError != nil {
			return flagError
		}
		patterns = sanitizeReplacementPatterns(flagPatterns)
	}

	findValue := strings.TrimSpace(configuration.Find)
	if command != nil && command.Flags().Changed(replaceFindFlagName) {
		flagValue, flagError := command.Flags().GetString(replaceFindFlagName)
		if flagError != nil {
			return flagError
		}
		findValue = strings.TrimSpace(flagValue)
	}
	if len(findValue) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return errors.New(replaceMissingFindError)
	}

	replaceValue := configuration.Replace
	if command != nil && command.Flags().Changed(replaceReplaceFlagName) {
		flagValue, flagError := command.Flags().GetString(replaceReplaceFlagName)
		if flagError != nil {
			return flagError
		}
		replaceValue = flagValue
	}

	commandValue := strings.TrimSpace(configuration.Command)
	if command != nil && command.Flags().Changed(replaceCommandFlagName) {
		flagValue, flagError := command.Flags().GetString(replaceCommandFlagName)
		if flagError != nil {
			return flagError
		}
		commandValue = strings.TrimSpace(flagValue)
	}
	commandArguments := sanitizeCommandArguments(strings.Fields(commandValue))

	requireClean := configuration.RequireClean
	if command != nil {
		flagValue, flagChanged, flagError := flagutils.BoolFlag(command, replaceRequireCleanFlagName)
		if flagError != nil && !errors.Is(flagError, flagutils.ErrFlagNotDefined) {
			return flagError
		}
		if flagChanged {
			requireClean = flagValue
		}
	}

	branchValue := strings.TrimSpace(configuration.Branch)
	if command != nil && command.Flags().Changed(replaceBranchFlagName) {
		flagValue, flagError := command.Flags().GetString(replaceBranchFlagName)
		if flagError != nil {
			return flagError
		}
		branchValue = strings.TrimSpace(flagValue)
	}

	requiredPaths := append([]string{}, configuration.RequirePaths...)
	if command != nil && command.Flags().Changed(replaceRequirePathFlagName) {
		flagPaths, flagError := command.Flags().GetStringSlice(replaceRequirePathFlagName)
		if flagError != nil {
			return flagError
		}
		requiredPaths = sanitizeReplacementPaths(flagPaths)
	}

	roots, rootsError := requireRepositoryRoots(command, nil, configuration.RepositoryRoots)
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

	var repositoryManager *gitrepo.RepositoryManager
	if concreteManager, ok := gitManager.(*gitrepo.RepositoryManager); ok {
		repositoryManager = concreteManager
	} else {
		constructed, err := gitrepo.NewRepositoryManager(gitExecutor)
		if err != nil {
			return err
		}
		repositoryManager = constructed
	}

	repositoryDiscoverer := dependencies.ResolveRepositoryDiscoverer(builder.Discoverer)
	fileSystem := dependencies.ResolveFileSystem(builder.FileSystem)

	githubClient, githubClientError := githubcli.NewClient(gitExecutor)
	if githubClientError != nil {
		return githubClientError
	}

	taskDependencies := workflow.Dependencies{
		Logger:               logger,
		RepositoryDiscoverer: repositoryDiscoverer,
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         githubClient,
		FileSystem:           fileSystem,
		Output:               command.OutOrStdout(),
		Errors:               command.ErrOrStderr(),
	}

	taskRunner := ResolveTaskRunner(builder.TaskRunnerFactory, taskDependencies)

	actionOptions := map[string]any{
		"find":    findValue,
		"replace": replaceValue,
	}

	if len(patterns) == 1 {
		actionOptions["pattern"] = patterns[0]
	} else if len(patterns) > 1 {
		actionOptions["patterns"] = patterns
	}

	if len(commandArguments) > 0 {
		actionOptions["command"] = commandArguments
	}

	safeguards := map[string]any{}
	if requireClean {
		safeguards["require_clean"] = true
	}
	if len(branchValue) > 0 {
		safeguards["branch"] = branchValue
	}
	if len(requiredPaths) > 0 {
		safeguards["paths"] = requiredPaths
	}
	if len(safeguards) > 0 {
		actionOptions["safeguards"] = safeguards
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        replacementTaskName,
		EnsureClean: false,
		Actions: []workflow.TaskActionDefinition{
			{Type: replacementActionType, Options: actionOptions},
		},
	}

	runtimeOptions := workflow.RuntimeOptions{
		DryRun:    dryRun,
		AssumeYes: assumeYes,
	}

	return taskRunner.Run(command.Context(), roots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
}

func (builder *ReplaceCommandBuilder) resolveConfiguration() ReplaceConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultToolsConfiguration().Replace
	}
	return builder.ConfigurationProvider().Sanitize()
}

func sanitizeCommandArguments(arguments []string) []string {
	sanitized := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		trimmed := strings.TrimSpace(argument)
		if len(trimmed) == 0 {
			continue
		}
		sanitized = append(sanitized, trimmed)
	}
	if len(sanitized) == 0 {
		return nil
	}
	return sanitized
}
