package repos

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	workflowcmd "github.com/temirov/gix/cmd/cli/workflow"
	"github.com/temirov/gix/internal/repos/shared"
	flagutils "github.com/temirov/gix/internal/utils/flags"
	"github.com/temirov/gix/internal/workflow"
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
	filesAddPresetName              = "files-add"
	filesAddPresetCommandKey        = "tasks apply"
	filesAddPresetMissingMessage    = "files-add preset not found"
	filesAddPresetLoadErrorTemplate = "unable to load files-add preset: %w"
	filesAddBuildWorkflowError      = "unable to build files-add workflow: %w"
)

// FilesAddCommandBuilder assembles the repo-files-add command.
type FilesAddCommandBuilder struct {
	LoggerProvider               workflowcmd.LoggerProvider
	Discoverer                   shared.RepositoryDiscoverer
	GitExecutor                  shared.GitExecutor
	GitManager                   shared.GitRepositoryManager
	FileSystem                   shared.FileSystem
	HumanReadableLoggingProvider func() bool
	ConfigurationProvider        func() AddConfiguration
	PresetCatalogFactory         func() workflowcmd.PresetCatalog
	WorkflowExecutorFactory      workflowcmd.OperationExecutorFactory
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

	presetCommand := builder.presetCommand()
	request := workflowcmd.PresetCommandRequest{
		Command:                 command,
		Arguments:               arguments,
		RootArguments:           arguments,
		ConfiguredAssumeYes:     configuration.AssumeYes,
		ConfiguredRoots:         configuration.RepositoryRoots,
		PresetName:              filesAddPresetName,
		PresetMissingMessage:    filesAddPresetMissingMessage,
		PresetLoadErrorTemplate: filesAddPresetLoadErrorTemplate,
		BuildErrorTemplate:      filesAddBuildWorkflowError,
		Configure: func(ctx workflowcmd.PresetCommandContext) (workflowcmd.PresetCommandResult, error) {
			fileContent := contentValue
			if len(strings.TrimSpace(contentFilePath)) > 0 {
				data, readError := ctx.Dependencies.FileSystem.ReadFile(contentFilePath)
				if readError != nil {
					return workflowcmd.PresetCommandResult{}, readError
				}
				fileContent = string(data)
			}

			resolvedRemote := remoteName
			if ctx.ExecutionFlagsAvailable && ctx.ExecutionFlags.RemoteSet {
				override := strings.TrimSpace(ctx.ExecutionFlags.Remote)
				if len(override) > 0 {
					resolvedRemote = override
				}
			}

			params := filesAddPresetOptions{
				Path:          targetPath,
				Content:       fileContent,
				Mode:          modeValue,
				Permissions:   os.FileMode(parsedPermissions),
				RequireClean:  requireClean,
				BranchName:    branchTemplate,
				StartPoint:    startPointTemplate,
				Push:          pushBranches,
				Remote:        resolvedRemote,
				CommitMessage: commitMessage,
			}

			for index := range ctx.Configuration.Steps {
				if workflow.CommandPathKey(ctx.Configuration.Steps[index].Command) != filesAddPresetCommandKey {
					continue
				}
				updateFilesAddPresetOptions(ctx.Configuration.Steps[index].Options, params)
			}

			runtimeOptions := ctx.RuntimeOptions()
			runtimeOptions.CaptureInitialWorktreeStatus = requireClean
			return workflowcmd.PresetCommandResult{
				Configuration:  ctx.Configuration,
				RuntimeOptions: runtimeOptions,
			}, nil
		},
	}

	return presetCommand.Execute(request)
}

func (builder *FilesAddCommandBuilder) resolveConfiguration() AddConfiguration {
	if builder.ConfigurationProvider == nil {
		return DefaultToolsConfiguration().Add
	}
	return builder.ConfigurationProvider().Sanitize()
}

func (builder *FilesAddCommandBuilder) resolvePresetCatalog() workflowcmd.PresetCatalog {
	if builder.PresetCatalogFactory != nil {
		if catalog := builder.PresetCatalogFactory(); catalog != nil {
			return catalog
		}
	}
	return workflowcmd.NewEmbeddedPresetCatalog()
}

func (builder *FilesAddCommandBuilder) presetCommand() workflowcmd.PresetCommand {
	return newPresetCommand(presetCommandDependencies{
		LoggerProvider:               builder.LoggerProvider,
		HumanReadableLoggingProvider: builder.HumanReadableLoggingProvider,
		Discoverer:                   builder.Discoverer,
		GitExecutor:                  builder.GitExecutor,
		GitManager:                   builder.GitManager,
		FileSystem:                   builder.FileSystem,
		PresetCatalogFactory:         builder.PresetCatalogFactory,
		WorkflowExecutorFactory:      builder.WorkflowExecutorFactory,
	})
}

type filesAddPresetOptions struct {
	Path          string
	Content       string
	Mode          string
	Permissions   os.FileMode
	RequireClean  bool
	BranchName    string
	StartPoint    string
	Push          bool
	Remote        string
	CommitMessage string
}

func updateFilesAddPresetOptions(options map[string]any, params filesAddPresetOptions) {
	if options == nil {
		return
	}
	tasksValue, ok := options["tasks"].([]any)
	if !ok || len(tasksValue) == 0 {
		return
	}
	taskEntry, ok := tasksValue[0].(map[string]any)
	if !ok {
		return
	}
	taskEntry["name"] = fmt.Sprintf(filesAddTaskNameTemplate, params.Path)
	taskEntry["ensure_clean"] = params.RequireClean

	branchEntry := map[string]any{
		"name":        params.BranchName,
		"start_point": params.StartPoint,
	}
	if params.Push {
		branchEntry["push_remote"] = params.Remote
	} else {
		taskEntry["steps"] = []any{"branch.prepare", "files.apply", "git.stage-commit"}
	}
	taskEntry["branch"] = branchEntry

	fileDefinition := map[string]any{
		"path":        params.Path,
		"content":     params.Content,
		"mode":        params.Mode,
		"permissions": int(params.Permissions),
	}
	taskEntry["files"] = []any{fileDefinition}

	commitEntry := map[string]any{
		"message": params.CommitMessage,
	}
	taskEntry["commit"] = commitEntry
	taskEntry["commit_message"] = params.CommitMessage

	tasksValue[0] = taskEntry
	options["tasks"] = tasksValue
}
