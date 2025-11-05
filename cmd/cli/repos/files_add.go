package repos

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	filesAddUseConstant             = "repo-files-add"
	filesAddShortDescription        = "Add or seed repository files"
	filesAddLongDescription         = "repo-files-add writes a file to each repository, optionally creating commits on dedicated branches and pushing the changes."
	filesAddPathFlagName            = "path"
	filesAddContentFlagName         = "content"
	filesAddContentFileFlagName     = "content-file"
	filesAddModeFlagName            = "mode"
	filesAddPermissionsFlagName     = "permissions"
	filesAddRequireCleanFlagName    = "require-clean"
	filesAddBranchFlagName          = "branch"
	filesAddStartPointFlagName      = "start-point"
	filesAddPushFlagName            = "push"
	filesAddRemoteFlagName          = "remote"
	filesAddCommitMessageFlagName   = "commit-message"
	filesAddMissingPathError        = "file add requires --path"
	filesAddMissingContentError     = "file add requires --content or --content-file"
	filesAddConflictingContentError = "--content and --content-file cannot both be provided"
	filesAddTaskNameTemplate        = "Add repository file %s"
	filesAddDefaultCommitTemplate   = "docs: add %s"
)

// FilesAddCommandBuilder assembles the repo-files-add command.
type FilesAddCommandBuilder struct {
	LoggerProvider               LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() AddConfiguration
	TaskRunnerFactory            func(workflow.Dependencies) TaskRunnerExecutor
}

// Build constructs the repo-files-add command.
func (builder *FilesAddCommandBuilder) Build() (*cobra.Command, error) {
	command := &cobra.Command{
		Use:   filesAddUseConstant,
		Short: filesAddShortDescription,
		Long:  filesAddLongDescription,
		RunE:  builder.run,
	}

	command.Flags().String(filesAddPathFlagName, "", "Relative path to write within each repository (required)")
	command.Flags().String(filesAddContentFlagName, "", "Inline file content")
	command.Flags().String(filesAddContentFileFlagName, "", "Path to a file whose contents should be written")
	command.Flags().String(filesAddModeFlagName, "", "Write mode: overwrite or skip-if-exists (default skip-if-exists)")
	command.Flags().String(filesAddPermissionsFlagName, "", "File permissions in octal (default 0644)")
	flagutils.AddToggleFlag(command.Flags(), nil, filesAddRequireCleanFlagName, "", true, "Require a clean working tree before applying changes")
	command.Flags().String(filesAddBranchFlagName, "", "Branch name template to use when creating commits")
	command.Flags().String(filesAddStartPointFlagName, "", "Start point template for the new branch (defaults to the repository default branch)")
	flagutils.AddToggleFlag(command.Flags(), nil, filesAddPushFlagName, "", true, "Push the branch to the configured remote after committing")
	command.Flags().String(filesAddRemoteFlagName, "", "Remote to push the branch to (default origin)")
	command.Flags().String(filesAddCommitMessageFlagName, "", "Commit message template for the file addition")

	return command, nil
}

func (builder *FilesAddCommandBuilder) run(command *cobra.Command, arguments []string) error {
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

	targetPath := strings.TrimSpace(configuration.Path)
	if command != nil && command.Flags().Changed(filesAddPathFlagName) {
		value, err := command.Flags().GetString(filesAddPathFlagName)
		if err != nil {
			return err
		}
		targetPath = strings.TrimSpace(value)
	}
	if len(targetPath) == 0 {
		if command != nil {
			_ = command.Help()
		}
		return errors.New(filesAddMissingPathError)
	}
	if filepath.IsAbs(targetPath) {
		return fmt.Errorf("path must be relative: %s", targetPath)
	}
	targetPath = filepath.Clean(targetPath)
	if targetPath == "." || len(targetPath) == 0 {
		return errors.New(filesAddMissingPathError)
	}

	contentValue := configuration.Content
	if command != nil && command.Flags().Changed(filesAddContentFlagName) {
		value, err := command.Flags().GetString(filesAddContentFlagName)
		if err != nil {
			return err
		}
		contentValue = value
	}

	contentFilePath := configuration.ContentFile
	if command != nil && command.Flags().Changed(filesAddContentFileFlagName) {
		value, err := command.Flags().GetString(filesAddContentFileFlagName)
		if err != nil {
			return err
		}
		contentFilePath = strings.TrimSpace(value)
	}

	if len(strings.TrimSpace(contentValue)) > 0 && len(strings.TrimSpace(contentFilePath)) > 0 {
		return errors.New(filesAddConflictingContentError)
	}
	if len(strings.TrimSpace(contentValue)) == 0 && len(strings.TrimSpace(contentFilePath)) == 0 {
		return errors.New(filesAddMissingContentError)
	}

	fileContent := contentValue
	fileSystem := dependencies.ResolveFileSystem(builder.FileSystem)
	if len(strings.TrimSpace(contentFilePath)) > 0 {
		data, readError := fileSystem.ReadFile(contentFilePath)
		if readError != nil {
			return readError
		}
		fileContent = string(data)
	}

	modeValue := configuration.Mode
	if command != nil && command.Flags().Changed(filesAddModeFlagName) {
		value, err := command.Flags().GetString(filesAddModeFlagName)
		if err != nil {
			return err
		}
		modeValue = strings.TrimSpace(value)
	}

	permissionsValue := configuration.Permissions
	if command != nil && command.Flags().Changed(filesAddPermissionsFlagName) {
		value, err := command.Flags().GetString(filesAddPermissionsFlagName)
		if err != nil {
			return err
		}
		permissionsValue = strings.TrimSpace(value)
	}
	if len(permissionsValue) == 0 {
		permissionsValue = "0644"
	}
	parsedPermissions, parseError := strconv.ParseUint(permissionsValue, 8, 32)
	if parseError != nil {
		return fmt.Errorf("invalid permissions %s", permissionsValue)
	}

	requireClean := configuration.RequireClean
	if command != nil {
		flagValue, flagChanged, flagError := flagutils.BoolFlag(command, filesAddRequireCleanFlagName)
		if flagError != nil && !errors.Is(flagError, flagutils.ErrFlagNotDefined) {
			return flagError
		}
		if flagChanged {
			requireClean = flagValue
		}
	}

	branchTemplate := configuration.Branch
	if command != nil && command.Flags().Changed(filesAddBranchFlagName) {
		value, err := command.Flags().GetString(filesAddBranchFlagName)
		if err != nil {
			return err
		}
		branchTemplate = strings.TrimSpace(value)
	}

	startPointTemplate := configuration.StartPoint
	if command != nil && command.Flags().Changed(filesAddStartPointFlagName) {
		value, err := command.Flags().GetString(filesAddStartPointFlagName)
		if err != nil {
			return err
		}
		startPointTemplate = strings.TrimSpace(value)
	}

	pushBranches := configuration.Push
	if command != nil {
		flagValue, flagChanged, flagError := flagutils.BoolFlag(command, filesAddPushFlagName)
		if flagError != nil && !errors.Is(flagError, flagutils.ErrFlagNotDefined) {
			return flagError
		}
		if flagChanged {
			pushBranches = flagValue
		}
	}

	remoteName := configuration.PushRemote
	if command != nil && command.Flags().Changed(filesAddRemoteFlagName) {
		value, err := command.Flags().GetString(filesAddRemoteFlagName)
		if err != nil {
			return err
		}
		remoteName = strings.TrimSpace(value)
	}
	if executionFlagsAvailable && executionFlags.RemoteSet {
		override := strings.TrimSpace(executionFlags.Remote)
		if len(override) > 0 {
			remoteName = override
		}
	}

	commitMessage := configuration.CommitMessage
	if command != nil && command.Flags().Changed(filesAddCommitMessageFlagName) {
		value, err := command.Flags().GetString(filesAddCommitMessageFlagName)
		if err != nil {
			return err
		}
		commitMessage = strings.TrimSpace(value)
	}
	if len(commitMessage) == 0 {
		commitMessage = fmt.Sprintf(filesAddDefaultCommitTemplate, targetPath)
	}

	roots, rootsError := requireRepositoryRoots(command, arguments, configuration.RepositoryRoots)
	if rootsError != nil {
		return rootsError
	}

	logger := resolveLogger(builder.LoggerProvider)
	humanReadable := false
	if builder.HumanReadableLoggingProvider != nil {
		humanReadable = builder.HumanReadableLoggingProvider()
	}

	gitExecutor, executorError := dependencies.ResolveGitExecutor(builder.GitExecutor, logger, humanReadable)
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
		constructedManager, err := gitrepo.NewRepositoryManager(gitExecutor)
		if err != nil {
			return err
		}
		repositoryManager = constructedManager
	}

	repositoryDiscoverer := dependencies.ResolveRepositoryDiscoverer(builder.Discoverer)

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

	fileDefinition := workflow.TaskFileDefinition{
		PathTemplate:    targetPath,
		ContentTemplate: fileContent,
		Mode:            workflow.ParseTaskFileMode(modeValue),
		Permissions:     os.FileMode(parsedPermissions),
	}

	taskDefinition := workflow.TaskDefinition{
		Name:        fmt.Sprintf(filesAddTaskNameTemplate, targetPath),
		EnsureClean: requireClean,
		Branch: workflow.TaskBranchDefinition{
			NameTemplate:       branchTemplate,
			StartPointTemplate: startPointTemplate,
			PushRemote:         remoteName,
		},
		Files: []workflow.TaskFileDefinition{fileDefinition},
		Commit: workflow.TaskCommitDefinition{
			MessageTemplate: commitMessage,
		},
	}

	if !pushBranches {
		taskDefinition.Branch.PushRemote = ""
	}

	runtimeOptions := workflow.RuntimeOptions{
		DryRun:                       dryRun,
		AssumeYes:                    assumeYes,
		CaptureInitialWorktreeStatus: requireClean,
	}

	return taskRunner.Run(command.Context(), roots, []workflow.TaskDefinition{taskDefinition}, runtimeOptions)
}

func (builder *FilesAddCommandBuilder) resolveConfiguration() AddConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultToolsConfiguration().Add
	}
	return builder.ConfigurationProvider().Sanitize()
}
