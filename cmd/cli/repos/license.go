package repos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/githubcli"
	"github.com/temirov/gix/internal/gitrepo"
	"github.com/temirov/gix/internal/repos/dependencies"
	"github.com/temirov/gix/internal/repos/shared"
	"github.com/temirov/gix/internal/utils"
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
	defaultLicensePermissions      = 0o644
	relativePathValidationErrorMsg = "license target path must be a relative path"
)

type WorkflowExecutor interface {
	Execute(ctx context.Context, roots []string, options workflow.RuntimeOptions) error
}

// LicenseCommandBuilder assembles the repo-license-apply command.
type LicenseCommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() LicenseConfiguration
	PresetCatalogFactory         func() workflowcmd.PresetCatalog
	ExecutorFactory              func(nodes []*workflow.OperationNode, dependencies workflow.Dependencies) WorkflowExecutor
}

// Build constructs the repo-license-apply command.
func (builder *LicenseCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   licenseUseConstant,
		Short: licenseShortDescription,
		Long:  licenseLongDescription,
		RunE:  builder.run,
	}
	command.Deprecated = "Use `gix workflow license --var template=PATH --var branch=...` instead."

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

	trimmedContent := strings.TrimSpace(content)
	if len(trimmedContent) == 0 {
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
	if len(strings.TrimSpace(trimmedContent)) == 0 {
		return errors.New("license content cannot be empty")
	}

	if filepath.IsAbs(targetPath) {
		return errors.New(relativePathValidationErrorMsg)
	}
	cleanedTarget := filepath.Clean(strings.TrimSpace(targetPath))
	if cleanedTarget == "." {
		return errors.New(relativePathValidationErrorMsg)
	}
	targetPath = cleanedTarget

	variables := map[string]string{
		"license_content":       trimmedContent,
		"license_require_clean": strconv.FormatBool(requireClean),
	}

	if len(targetPath) > 0 {
		variables["license_target"] = targetPath
	}
	if trimmedMode := strings.TrimSpace(strings.ToLower(modeValue)); len(trimmedMode) > 0 {
		variables["license_mode"] = trimmedMode
	}
	if len(branchTemplate) > 0 {
		variables["license_branch"] = branchTemplate
	}
	if len(startPointTemplate) > 0 {
		variables["license_start_point"] = startPointTemplate
	}
	if len(pushRemote) > 0 {
		variables["license_remote"] = pushRemote
	}
	if len(commitMessage) > 0 {
		variables["license_commit_message"] = commitMessage
	}

	workflowDependencies := workflow.Dependencies{
		Logger:               logger,
		RepositoryDiscoverer: repositoryDiscoverer,
		GitExecutor:          gitExecutor,
		RepositoryManager:    repositoryManager,
		GitHubClient:         githubClient,
		FileSystem:           fileSystem,
		Output:               utils.NewFlushingWriter(command.OutOrStdout()),
		Errors:               utils.NewFlushingWriter(command.ErrOrStderr()),
	}

	presetCatalog := builder.resolvePresetCatalog()
	presetConfiguration, presetFound, presetError := presetCatalog.Load("license")
	if presetError != nil {
		return fmt.Errorf("failed to load embedded license workflow: %w", presetError)
	}
	if !presetFound {
		return errors.New("embedded license workflow not available")
	}

	nodes, operationsError := workflow.BuildOperations(presetConfiguration)
	if operationsError != nil {
		return fmt.Errorf("unable to build workflow operations: %w", operationsError)
	}

	executorInstance := builder.resolveExecutor(nodes, workflowDependencies)
	runtimeOptions := workflow.RuntimeOptions{
		AssumeYes:                    assumeYes,
		CaptureInitialWorktreeStatus: requireClean,
		Variables:                    variables,
	}

	if command != nil {
		fmt.Fprintln(command.ErrOrStderr(), "DEPRECATED: repo-license-apply will be removed; use `gix workflow license --var template=PATH --var branch=...` instead.")
	}

	return executorInstance.Execute(command.Context(), roots, runtimeOptions)
}

func (builder *LicenseCommandBuilder) resolveConfiguration() LicenseConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultToolsConfiguration().License
	}
	return builder.ConfigurationProvider().Sanitize()
}

func (builder *LicenseCommandBuilder) resolvePresetCatalog() workflowcmd.PresetCatalog {
	if builder.PresetCatalogFactory != nil {
		if catalog := builder.PresetCatalogFactory(); catalog != nil {
			return catalog
		}
	}
	return workflowcmd.NewEmbeddedPresetCatalog()
}

func (builder *LicenseCommandBuilder) resolveExecutor(nodes []*workflow.OperationNode, dependencies workflow.Dependencies) WorkflowExecutor {
	if builder.ExecutorFactory != nil {
		if executor := builder.ExecutorFactory(nodes, dependencies); executor != nil {
			return executor
		}
	}
	return workflow.NewExecutorFromNodes(nodes, dependencies)
}
