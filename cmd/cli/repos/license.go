package repos

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	licenseUseConstant             = "repo-license-apply"
	licenseShortDescription        = "Distribute a license file across repositories"
	licenseLongDescription         = "repo-license-apply writes (or updates) a license file in each repository, staging commits on dedicated branches and pushing when remotes are available."
	licenseTemplateFlagName        = "template"
	licenseContentFlagName         = "content"
	licenseTargetFlagName          = "target"
	licenseModeFlagName            = "mode"
	licenseRequireCleanFlagName    = "require-clean"
	licenseBranchFlagName          = "branch"
	licenseStartPointFlagName      = "start-point"
	licenseRemoteFlagName          = "remote"
	licenseCommitMessageFlagName   = "commit-message"
	licenseTaskName                = "Distribute license file"
	defaultLicensePermissions      = 0o644
	relativePathValidationErrorMsg = "license target path must be a relative path"
)

// LicenseCommandBuilder assembles the repo-license-apply command.
type LicenseCommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() LicenseConfiguration
	TaskRunnerFactory            func(workflow.Dependencies) TaskRunnerExecutor
}

// Build constructs the repo-license-apply command.
func (builder *LicenseCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   licenseUseConstant,
		Short: licenseShortDescription,
		Long:  licenseLongDescription,
		RunE:  builder.run,
	}

	command.Flags().String(licenseTemplateFlagName, "", "Path to a license template file (required when --content is not provided)")
	command.Flags().String(licenseContentFlagName, "", "Inline license content (overrides --template when provided)")
	command.Flags().String(licenseTargetFlagName, "", "Relative path for the license file (default LICENSE)")
	command.Flags().String(licenseModeFlagName, "", "File handling mode: overwrite or skip-if-exists")
	flagutils.AddToggleFlag(command.Flags(), nil, licenseRequireCleanFlagName, "", true, "Skip repositories with dirty working trees")
	command.Flags().String(licenseBranchFlagName, "", "Branch name template for license commits (Go text/template over repository metadata)")
	command.Flags().String(licenseStartPointFlagName, "", "Start point template for the license branch (defaults to the repository default branch)")
	command.Flags().String(licenseRemoteFlagName, "", "Remote to push the license branch to (default origin)")
	command.Flags().String(licenseCommitMessageFlagName, "", "Commit message template for the license change")

	return command, nil
}

func (builder *LicenseCommandBuilder) run(command *cobra.Command, arguments []string) error {
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

	requireClean := configuration.RequireClean
	if command != nil {
		flagValue, flagChanged, flagError := flagutils.BoolFlag(command, licenseRequireCleanFlagName)
		if flagError != nil && !errors.Is(flagError, flagutils.ErrFlagNotDefined) {
			return flagError
		}
		if flagChanged {
			requireClean = flagValue
		}
	}

	templatePath := configuration.TemplatePath
	if command != nil && command.Flags().Changed(licenseTemplateFlagName) {
		value, err := command.Flags().GetString(licenseTemplateFlagName)
		if err != nil {
			return err
		}
		templatePath = strings.TrimSpace(value)
	}

	content := configuration.Content
	if command != nil && command.Flags().Changed(licenseContentFlagName) {
		value, err := command.Flags().GetString(licenseContentFlagName)
		if err != nil {
			return err
		}
		content = value
	}

	targetPath := configuration.TargetPath
	if command != nil && command.Flags().Changed(licenseTargetFlagName) {
		value, err := command.Flags().GetString(licenseTargetFlagName)
		if err != nil {
			return err
		}
		targetPath = strings.TrimSpace(value)
	}

	modeValue := configuration.Mode
	if command != nil && command.Flags().Changed(licenseModeFlagName) {
		value, err := command.Flags().GetString(licenseModeFlagName)
		if err != nil {
			return err
		}
		modeValue = strings.TrimSpace(value)
	}

	branchTemplate := configuration.Branch
	if command != nil && command.Flags().Changed(licenseBranchFlagName) {
		value, err := command.Flags().GetString(licenseBranchFlagName)
		if err != nil {
			return err
		}
		branchTemplate = strings.TrimSpace(value)
	}

	startPointTemplate := configuration.StartPoint
	if command != nil && command.Flags().Changed(licenseStartPointFlagName) {
		value, err := command.Flags().GetString(licenseStartPointFlagName)
		if err != nil {
			return err
		}
		startPointTemplate = strings.TrimSpace(value)
	}

	pushRemote := configuration.PushRemote
	if command != nil && command.Flags().Changed(licenseRemoteFlagName) {
		value, err := command.Flags().GetString(licenseRemoteFlagName)
		if err != nil {
			return err
		}
		pushRemote = strings.TrimSpace(value)
	}

	commitMessage := configuration.CommitMessage
	if command != nil && command.Flags().Changed(licenseCommitMessageFlagName) {
		value, err := command.Flags().GetString(licenseCommitMessageFlagName)
		if err != nil {
			return err
		}
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

	var repositoryManager *gitrepo.RepositoryManager
	if concrete, ok := gitManager.(*gitrepo.RepositoryManager); ok {
		repositoryManager = concrete
	} else {
		constructed, constructedErr := gitrepo.NewRepositoryManager(gitExecutor)
		if constructedErr != nil {
			return constructedErr
		}
		repositoryManager = constructed
	}

	repositoryDiscoverer := dependencies.ResolveRepositoryDiscoverer(builder.Discoverer)
	fileSystem := dependencies.ResolveFileSystem(builder.FileSystem)

	githubClient, githubClientError := githubcli.NewClient(gitExecutor)
	if githubClientError != nil {
		return githubClientError
	}

	trimmedContent := content
	if len(strings.TrimSpace(trimmedContent)) == 0 {
		if len(strings.TrimSpace(templatePath)) == 0 {
			if command != nil {
				_ = command.Help()
			}
			return fmt.Errorf("license distribution requires --%s or --%s", licenseTemplateFlagName, licenseContentFlagName)
		}
		data, readError := fileSystem.ReadFile(templatePath)
		if readError != nil {
			return readError
		}
		trimmedContent = string(data)
	}

	if filepath.IsAbs(targetPath) {
		return fmt.Errorf(relativePathValidationErrorMsg)
	}
	cleanedTarget := filepath.Clean(targetPath)
	if cleanedTarget == "." {
		return fmt.Errorf(relativePathValidationErrorMsg)
	}
	targetPath = cleanedTarget

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
	fileDefinition := workflow.TaskFileDefinition{
		PathTemplate:    targetPath,
		ContentTemplate: trimmedContent,
		Mode:            workflow.ParseTaskFileMode(modeValue),
		Permissions:     defaultLicensePermissions,
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        licenseTaskName,
		EnsureClean: requireClean,
		Branch: workflow.TaskBranchDefinition{
			NameTemplate:       branchTemplate,
			StartPointTemplate: startPointTemplate,
			PushRemote:         pushRemote,
		},
		Files: []workflow.TaskFileDefinition{fileDefinition},
		Commit: workflow.TaskCommitDefinition{
			MessageTemplate: commitMessage,
		},
	}

	runtimeOptions := workflow.RuntimeOptions{
		DryRun:                       dryRun,
		AssumeYes:                    assumeYes,
		CaptureInitialWorktreeStatus: requireClean,
	}

	return taskRunner.Run(command.Context(), roots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
}

func (builder *LicenseCommandBuilder) resolveConfiguration() LicenseConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultToolsConfiguration().License
	}
	return builder.ConfigurationProvider().Sanitize()
}
