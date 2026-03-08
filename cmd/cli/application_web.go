package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/gix/internal/execshell"
	reposdeps "github.com/tyemirov/gix/internal/repos/dependencies"
	"github.com/tyemirov/gix/internal/repos/shared"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	"github.com/tyemirov/gix/internal/web"
	"github.com/tyemirov/gix/internal/workflow"
	"github.com/tyemirov/gix/pkg/taskrunner"
)

const (
	webInterfaceUnavailableMessageConstant      = "web mode is unavailable from the web interface"
	webLaunchModeCurrentRepositoryConstant      = "current_repo"
	webLaunchModeDiscoveredRepositoriesConstant = "discovered_repositories"
	webNoRepositoriesErrorTemplateConstant      = "no Git repositories found beneath %s"
	webRepositoryIDTemplateConstant             = "repo-%03d"
	webRepositoryPathRequiredErrorConstant      = "repository path is required"
	webHelpCommandPathConstant                  = applicationNameConstant + " help"
	webCompletionCommandPathConstant            = applicationNameConstant + " completion"
	webCompletionBashCommandPathConstant        = webCompletionCommandPathConstant + " bash"
	webCompletionFishCommandPathConstant        = webCompletionCommandPathConstant + " fish"
	webCompletionPowershellCommandPathConstant  = webCompletionCommandPathConstant + " powershell"
	webCompletionZshCommandPathConstant         = webCompletionCommandPathConstant + " zsh"
	gitForEachRefSubcommandConstant             = "for-each-ref"
	gitBranchCatalogFormatArgumentConstant      = "--format=%(HEAD)|%(refname:short)|%(upstream:short)"
	gitBranchCatalogReferenceArgumentConstant   = "refs/heads/"
	gitRevParseSubcommandConstant               = "rev-parse"
	gitShowTopLevelArgumentConstant             = "--show-toplevel"
	gitSymbolicRefSubcommandConstant            = "symbolic-ref"
	gitSymbolicRefQuietArgumentConstant         = "--quiet"
	gitShortArgumentConstant                    = "--short"
	gitOriginHeadReferenceConstant              = "refs/remotes/origin/HEAD"
	gitOriginPrefixConstant                     = "origin/"
	webCommandGroupBranchConstant               = "branch"
	webCommandGroupRepositoryConstant           = "repository"
	webCommandGroupRemoteConstant               = "remote"
	webCommandGroupPullRequestsConstant         = "prs"
	webCommandGroupPackagesConstant             = "packages"
	webCommandGroupFilesConstant                = "files"
	webCommandGroupGeneralConstant              = "general"
	webDraftTemplateFilesAddConstant            = "files_add"
	webDraftTemplateFilesReplaceConstant        = "files_replace"
	webDraftTemplateFilesRemoveConstant         = "files_remove"
	webAuditQueuedChangesRequiredConstant       = "at least one queued audit change is required"
	webAuditChangePathRequiredConstant          = "audit change path is required"
	webAuditChangeDeleteRejectedConstant        = "delete_folder requires confirm_delete"
	webAuditChangeDeleteRootRejectedConstant    = "delete_folder does not allow deleting filesystem roots"
	webAuditChangeProtocolMissingConstant       = "convert_protocol requires source_protocol and target_protocol"
	webAuditChangeSyncStrategyTemplateConstant  = "unsupported sync strategy %q"
	webAuditChangeKindTemplateConstant          = "unsupported audit change kind %q"
	webAuditChangeStatusSucceededConstant       = "succeeded"
	webAuditChangeStatusFailedConstant          = "failed"
)

var nonActionableCommandPaths = map[string]struct{}{
	applicationNameConstant: {},
	applicationNameConstant + " " + repoFolderNamespaceUseNameConstant:                                              {},
	applicationNameConstant + " " + repoRemoteNamespaceUseNameConstant:                                              {},
	applicationNameConstant + " " + repoPullRequestsNamespaceUseNameConstant:                                        {},
	applicationNameConstant + " " + repoPackagesNamespaceUseNameConstant:                                            {},
	applicationNameConstant + " " + repoFilesNamespaceUseNameConstant:                                               {},
	applicationNameConstant + " " + messageNamespaceUseNameConstant:                                                 {},
	applicationNameConstant + " " + workflowCommandOperationNameConstant:                                            {},
	applicationNameConstant + " " + repoRemoteNamespaceUseNameConstant + " " + updateProtocolCommandUseNameConstant: {},
	applicationNameConstant + " " + repoFilesNamespaceUseNameConstant + " " + filesReplaceCommandUseNameConstant:    {},
	applicationNameConstant + " " + repoFilesNamespaceUseNameConstant + " " + filesAddCommandUseNameConstant:        {},
	applicationNameConstant + " " + repoFilesNamespaceUseNameConstant + " " + removeCommandUseNameConstant:          {},
	applicationNameConstant + " " + repoReleaseCommandUseNameConstant + " " + releaseRetagCommandUseNameConstant:    {},
	applicationNameConstant + " " + messageNamespaceUseNameConstant + " " + commitMessageUseNameConstant:            {},
	applicationNameConstant + " " + messageNamespaceUseNameConstant + " " + changelogMessageUseNameConstant:         {},
	applicationNameConstant + " " + repoReleaseCommandUseNameConstant:                                               {},
	webHelpCommandPathConstant:                 {},
	webCompletionCommandPathConstant:           {},
	webCompletionBashCommandPathConstant:       {},
	webCompletionFishCommandPathConstant:       {},
	webCompletionPowershellCommandPathConstant: {},
	webCompletionZshCommandPathConstant:        {},
}

type webRunner func(context.Context, web.ServerOptions) error

type execshellGitExecutor = shared.GitExecutor
type webGitRepositoryManager = shared.GitRepositoryManager
type webGitHubMetadataResolver = shared.GitHubMetadataResolver

type webLaunchConfiguration struct {
	port int
}

func newWebLaunchConfiguration(rawValue string) (webLaunchConfiguration, error) {
	trimmedValue := strings.TrimSpace(rawValue)
	if len(trimmedValue) == 0 {
		trimmedValue = webDefaultPortConstant
	}

	portValue, parseError := strconv.Atoi(trimmedValue)
	if parseError != nil {
		return webLaunchConfiguration{}, fmt.Errorf(webPortInvalidTemplateConstant, rawValue)
	}
	if portValue < 1 || portValue > 65535 {
		return webLaunchConfiguration{}, fmt.Errorf(webPortRangeTemplateConstant, portValue)
	}

	return webLaunchConfiguration{port: portValue}, nil
}

func (configuration webLaunchConfiguration) listenAddress() string {
	return fmt.Sprintf(webAddressTemplateConstant, webListenHostConstant, configuration.port)
}

func (configuration webLaunchConfiguration) launchURL() string {
	return fmt.Sprintf(webLaunchURLTemplateConstant, webListenHostConstant, configuration.port)
}

func (application *Application) handleWebLaunch(command *cobra.Command) (bool, error) {
	launchConfiguration, requested, configurationError := application.resolveWebLaunchConfiguration(command)
	if configurationError != nil {
		return true, configurationError
	}
	if !requested {
		return false, nil
	}

	outputWriter := io.Writer(nil)
	executionContext := context.Background()
	if command != nil {
		outputWriter = command.OutOrStdout()
		executionContext = command.Context()
	}
	if executionContext == nil {
		executionContext = context.Background()
	}
	if outputWriter != nil {
		if _, writeError := fmt.Fprintf(outputWriter, webLaunchMessageTemplateConstant, launchConfiguration.launchURL()); writeError != nil {
			return true, writeError
		}
	}

	repositoryCatalog := application.repositoryCatalog(executionContext)

	return true, application.webRunner(executionContext, web.ServerOptions{
		Address:           launchConfiguration.listenAddress(),
		Repositories:      repositoryCatalog,
		Catalog:           application.commandCatalog(),
		LoadBranches:      application.loadRepositoryBranches,
		Execute:           application.newWebCommandExecutor(),
		InspectAudit:      application.newWebAuditInspector(),
		ApplyAuditChanges: application.newWebAuditChangeExecutor(),
	})
}

func (application *Application) resolveWebLaunchConfiguration(command *cobra.Command) (webLaunchConfiguration, bool, error) {
	if command == nil {
		return webLaunchConfiguration{}, false, nil
	}

	rawValue, changed, flagError := flagutils.StringFlag(command, webFlagNameConstant)
	switch {
	case flagError == nil:
	case errors.Is(flagError, flagutils.ErrFlagNotDefined):
		return webLaunchConfiguration{}, false, nil
	default:
		return webLaunchConfiguration{}, false, flagError
	}

	if !changed && len(strings.TrimSpace(rawValue)) == 0 {
		return webLaunchConfiguration{}, false, nil
	}

	launchConfiguration, configurationError := newWebLaunchConfiguration(rawValue)
	if configurationError != nil {
		return webLaunchConfiguration{}, true, configurationError
	}
	return launchConfiguration, true, nil
}

func (application *Application) launchWebInterface(executionContext context.Context, options web.ServerOptions) error {
	return web.Run(executionContext, options)
}

func (application *Application) newWebCommandExecutor() web.CommandExecutor {
	return func(executionContext context.Context, arguments []string, standardInput io.Reader, standardOutput io.Writer, standardError io.Writer) error {
		nestedApplication := NewApplication()
		nestedApplication.versionResolver = application.versionResolver
		nestedApplication.webRunner = func(context.Context, web.ServerOptions) error {
			return errors.New(webInterfaceUnavailableMessageConstant)
		}

		return nestedApplication.ExecuteWithOptions(ExecutionOptions{
			Arguments:      application.webInheritedArguments(arguments),
			Context:        executionContext,
			StandardInput:  standardInput,
			StandardOutput: standardOutput,
			StandardError:  standardError,
			ExitOnVersion:  false,
		})
	}
}

func (application *Application) newWebAuditInspector() web.AuditInspector {
	return func(executionContext context.Context, request web.AuditInspectionRequest) web.AuditInspectionResponse {
		discoverer, gitExecutor, repositoryManager, githubResolver, dependencyError := application.webAuditDependencies()
		if dependencyError != nil {
			return web.AuditInspectionResponse{Error: dependencyError.Error()}
		}

		auditService := audit.NewService(
			discoverer,
			repositoryManager,
			gitExecutor,
			githubResolver,
			io.Discard,
			io.Discard,
		)

		inspections, inspectionError := auditService.DiscoverInspections(
			executionContext,
			append([]string(nil), request.Roots...),
			request.IncludeAll,
			false,
			audit.InspectionDepthFull,
		)
		if inspectionError != nil {
			return web.AuditInspectionResponse{Roots: append([]string(nil), request.Roots...), Error: inspectionError.Error()}
		}

		rows := make([]web.AuditInspectionRow, 0, len(inspections))
		for inspectionIndex := range inspections {
			rows = append(rows, mapAuditInspectionRow(inspections[inspectionIndex]))
		}

		return web.AuditInspectionResponse{
			Roots: append([]string(nil), request.Roots...),
			Rows:  rows,
		}
	}
}

func (application *Application) newWebAuditChangeExecutor() web.AuditChangeExecutor {
	return func(executionContext context.Context, request web.AuditChangeApplyRequest) web.AuditChangeApplyResponse {
		if len(request.Changes) == 0 {
			return web.AuditChangeApplyResponse{Error: webAuditQueuedChangesRequiredConstant}
		}

		changes := append([]web.AuditQueuedChange(nil), request.Changes...)
		slices.SortStableFunc(changes, func(left web.AuditQueuedChange, right web.AuditQueuedChange) int {
			return cmpWebAuditChangePriority(left.Kind, right.Kind)
		})

		results := make([]web.AuditChangeApplyResult, 0, len(changes))
		for changeIndex := range changes {
			results = append(results, application.applyWebAuditChange(executionContext, changes[changeIndex]))
		}

		return web.AuditChangeApplyResponse{Results: results}
	}
}

func (application *Application) webInheritedArguments(arguments []string) []string {
	inheritedArguments := make([]string, 0, len(arguments)+6)

	configurationFilePath := strings.TrimSpace(application.configurationMetadata.ConfigFileUsed)
	if len(configurationFilePath) > 0 && !commandArgumentsContainFlag(arguments, configFileFlagNameConstant) {
		inheritedArguments = append(inheritedArguments, "--"+configFileFlagNameConstant, configurationFilePath)
	}

	logLevelValue := strings.TrimSpace(application.logLevelFlagValue)
	if len(logLevelValue) > 0 && !commandArgumentsContainFlag(arguments, logLevelFlagNameConstant) {
		inheritedArguments = append(inheritedArguments, "--"+logLevelFlagNameConstant, logLevelValue)
	}

	logFormatValue := strings.TrimSpace(application.logFormatFlagValue)
	if len(logFormatValue) > 0 && !commandArgumentsContainFlag(arguments, logFormatFlagNameConstant) {
		inheritedArguments = append(inheritedArguments, "--"+logFormatFlagNameConstant, logFormatValue)
	}

	return append(inheritedArguments, arguments...)
}

func (application *Application) applyWebAuditChange(executionContext context.Context, change web.AuditQueuedChange) web.AuditChangeApplyResult {
	normalizedPath, pathError := normalizeWebAuditChangePath(change.Path)
	result := web.AuditChangeApplyResult{
		ID:   change.ID,
		Kind: change.Kind,
		Path: normalizedPath,
	}
	if pathError != nil {
		result.Status = webAuditChangeStatusFailedConstant
		result.Error = pathError.Error()
		return result
	}

	outputBuffer := &bytes.Buffer{}
	errorBuffer := &bytes.Buffer{}

	applyError := error(nil)
	message := ""

	switch change.Kind {
	case web.AuditChangeKindDeleteFolder:
		if !change.ConfirmDelete {
			applyError = errors.New(webAuditChangeDeleteRejectedConstant)
			break
		}
		if filepath.Dir(normalizedPath) == normalizedPath {
			applyError = errors.New(webAuditChangeDeleteRootRejectedConstant)
			break
		}
		if deleteError := os.RemoveAll(normalizedPath); deleteError != nil {
			applyError = deleteError
			break
		}
		_, _ = fmt.Fprintf(outputBuffer, "DELETED: %s\n", normalizedPath)
		message = "Folder deleted"
	default:
		dependencies, dependencyError := application.webTaskRunnerDependencies(outputBuffer, errorBuffer)
		if dependencyError != nil {
			applyError = dependencyError
			break
		}

		applyError = executeWebAuditWorkflowChange(executionContext, dependencies.Workflow, normalizedPath, change)
		message = webAuditChangeMessage(change.Kind)
	}

	result.Stdout = outputBuffer.String()
	result.Stderr = errorBuffer.String()
	if len(message) > 0 {
		result.Message = message
	}
	if applyError != nil {
		result.Status = webAuditChangeStatusFailedConstant
		result.Error = applyError.Error()
		return result
	}

	result.Status = webAuditChangeStatusSucceededConstant
	return result
}

func commandArgumentsContainFlag(arguments []string, flagName string) bool {
	if len(flagName) == 0 {
		return false
	}

	fullName := "--" + flagName
	prefixedName := fullName + "="
	for _, argument := range arguments {
		if argument == fullName || strings.HasPrefix(argument, prefixedName) {
			return true
		}
	}

	return false
}

func (application *Application) repositoryCatalog(executionContext context.Context) web.RepositoryCatalog {
	workingDirectory, workingDirectoryError := os.Getwd()
	if workingDirectoryError != nil {
		return web.RepositoryCatalog{Error: workingDirectoryError.Error()}
	}

	gitExecutor, repositoryManager, dependencyError := application.webGitDependencies()
	if dependencyError != nil {
		return web.RepositoryCatalog{
			LaunchPath: workingDirectory,
			Error:      dependencyError.Error(),
		}
	}

	currentRepositoryRoot, currentRepositoryAvailable := application.currentRepositoryRoot(executionContext, gitExecutor, workingDirectory)
	if currentRepositoryAvailable {
		repositoryDescriptor := application.inspectRepository(executionContext, currentRepositoryRoot, 1, true, gitExecutor, repositoryManager)
		return web.RepositoryCatalog{
			LaunchPath:           workingDirectory,
			LaunchMode:           webLaunchModeCurrentRepositoryConstant,
			SelectedRepositoryID: repositoryDescriptor.ID,
			Repositories:         []web.RepositoryDescriptor{repositoryDescriptor},
		}
	}

	discoverer := reposdeps.ResolveRepositoryDiscoverer(nil)
	discoveredRepositories, discoverError := discoverer.DiscoverRepositories([]string{workingDirectory})
	if discoverError != nil {
		return web.RepositoryCatalog{
			LaunchPath: workingDirectory,
			Error:      discoverError.Error(),
		}
	}

	if len(discoveredRepositories) == 0 {
		return web.RepositoryCatalog{
			LaunchPath: workingDirectory,
			LaunchMode: webLaunchModeDiscoveredRepositoriesConstant,
			Error:      fmt.Sprintf(webNoRepositoriesErrorTemplateConstant, workingDirectory),
		}
	}

	repositoryDescriptors := make([]web.RepositoryDescriptor, 0, len(discoveredRepositories))
	for repositoryIndex, repositoryPath := range discoveredRepositories {
		repositoryDescriptors = append(
			repositoryDescriptors,
			application.inspectRepository(executionContext, repositoryPath, repositoryIndex+1, false, gitExecutor, repositoryManager),
		)
	}

	selectedRepositoryID := ""
	if len(repositoryDescriptors) > 0 {
		selectedRepositoryID = repositoryDescriptors[0].ID
	}

	return web.RepositoryCatalog{
		LaunchPath:           workingDirectory,
		LaunchMode:           webLaunchModeDiscoveredRepositoriesConstant,
		SelectedRepositoryID: selectedRepositoryID,
		Repositories:         repositoryDescriptors,
	}
}

func (application *Application) loadRepositoryBranches(executionContext context.Context, repository web.RepositoryDescriptor) web.BranchCatalog {
	gitExecutor, _, dependencyError := application.webGitDependencies()
	if dependencyError != nil {
		return web.BranchCatalog{
			RepositoryID:   repository.ID,
			RepositoryPath: repository.Path,
			Error:          dependencyError.Error(),
		}
	}

	return loadRepositoryBranches(executionContext, repository, gitExecutor)
}

func mapAuditInspectionRow(inspection audit.RepositoryInspection) web.AuditInspectionRow {
	row := auditInspectionReportRow(inspection)
	return web.AuditInspectionRow{
		Path:                   inspection.Path,
		FolderName:             row.FolderName,
		IsGitRepository:        inspection.IsGitRepository,
		FinalGitHubRepository:  row.FinalRepository,
		OriginRemoteStatus:     string(row.OriginRemoteStatus),
		NameMatches:            string(row.NameMatches),
		RemoteDefaultBranch:    row.RemoteDefaultBranch,
		LocalBranch:            row.LocalBranch,
		InSync:                 string(row.InSync),
		RemoteProtocol:         string(row.RemoteProtocol),
		OriginMatchesCanonical: string(row.OriginMatchesCanonical),
		WorktreeDirty:          string(row.WorktreeDirty),
		DirtyFiles:             row.DirtyFiles,
	}
}

func executeWebAuditWorkflowChange(
	executionContext context.Context,
	dependencies workflow.Dependencies,
	repositoryPath string,
	change web.AuditQueuedChange,
) error {
	switch change.Kind {
	case web.AuditChangeKindRenameFolder:
		return executeWebAuditOperation(
			executionContext,
			dependencies,
			repositoryPath,
			&workflow.RenameOperation{
				RequireCleanWorktree: change.RequireClean,
				IncludeOwner:         change.IncludeOwner,
			},
		)
	case web.AuditChangeKindUpdateCanonical:
		return executeWebAuditOperation(
			executionContext,
			dependencies,
			repositoryPath,
			&workflow.CanonicalRemoteOperation{},
		)
	case web.AuditChangeKindConvertProtocol:
		sourceProtocol, sourceError := shared.ParseRemoteProtocol(change.SourceProtocol)
		if sourceError != nil {
			return errors.New(webAuditChangeProtocolMissingConstant)
		}
		targetProtocol, targetError := shared.ParseRemoteProtocol(change.TargetProtocol)
		if targetError != nil {
			return errors.New(webAuditChangeProtocolMissingConstant)
		}
		return executeWebAuditOperation(
			executionContext,
			dependencies,
			repositoryPath,
			&workflow.ProtocolConversionOperation{
				FromProtocol: sourceProtocol,
				ToProtocol:   targetProtocol,
			},
		)
	case web.AuditChangeKindSyncWithRemote:
		taskDefinition, taskDefinitionError := webAuditSyncTaskDefinition(change.SyncStrategy)
		if taskDefinitionError != nil {
			return taskDefinitionError
		}
		return executeWebAuditTasks(
			executionContext,
			dependencies,
			repositoryPath,
			[]workflow.TaskDefinition{taskDefinition},
		)
	default:
		return fmt.Errorf(webAuditChangeKindTemplateConstant, change.Kind)
	}
}

func executeWebAuditOperation(
	executionContext context.Context,
	dependencies workflow.Dependencies,
	repositoryPath string,
	operation workflow.Operation,
) error {
	executor := workflow.NewExecutor([]workflow.Operation{operation}, dependencies)
	_, executionError := executor.Execute(
		executionContext,
		[]string{repositoryPath},
		workflow.RuntimeOptions{
			AssumeYes:           true,
			WorkflowParallelism: 1,
		},
	)
	return executionError
}

func executeWebAuditTasks(
	executionContext context.Context,
	dependencies workflow.Dependencies,
	repositoryPath string,
	definitions []workflow.TaskDefinition,
) error {
	runner := workflow.NewTaskRunner(dependencies)
	_, executionError := runner.Run(
		executionContext,
		[]string{repositoryPath},
		definitions,
		workflow.RuntimeOptions{
			AssumeYes:           true,
			WorkflowParallelism: 1,
		},
	)
	return executionError
}

func webAuditSyncTaskDefinition(syncStrategy string) (workflow.TaskDefinition, error) {
	options := map[string]any{
		"refresh": true,
	}

	switch strings.TrimSpace(syncStrategy) {
	case "", web.AuditChangeSyncStrategyRequireClean:
		options["require_clean"] = true
	case web.AuditChangeSyncStrategyStashChanges:
		options["stash"] = true
	case web.AuditChangeSyncStrategyCommitChanges:
		options["commit"] = true
	default:
		return workflow.TaskDefinition{}, fmt.Errorf(webAuditChangeSyncStrategyTemplateConstant, syncStrategy)
	}

	return workflow.TaskDefinition{
		Name:  "audit-sync",
		Steps: []workflow.TaskExecutionStep{workflow.TaskExecutionStepCustomActions},
		Actions: []workflow.TaskActionDefinition{
			{
				Type:    "branch.change",
				Options: options,
			},
		},
	}, nil
}

func webAuditChangeMessage(kind web.AuditChangeKind) string {
	switch kind {
	case web.AuditChangeKindRenameFolder:
		return "Rename folder applied"
	case web.AuditChangeKindUpdateCanonical:
		return "Canonical remote updated"
	case web.AuditChangeKindConvertProtocol:
		return "Remote protocol updated"
	case web.AuditChangeKindSyncWithRemote:
		return "Repository synchronized with remote"
	case web.AuditChangeKindDeleteFolder:
		return "Folder deleted"
	default:
		return ""
	}
}

func cmpWebAuditChangePriority(left web.AuditChangeKind, right web.AuditChangeKind) int {
	return webAuditChangePriority(left) - webAuditChangePriority(right)
}

func webAuditChangePriority(kind web.AuditChangeKind) int {
	switch kind {
	case web.AuditChangeKindUpdateCanonical:
		return 10
	case web.AuditChangeKindConvertProtocol:
		return 20
	case web.AuditChangeKindSyncWithRemote:
		return 30
	case web.AuditChangeKindRenameFolder:
		return 40
	case web.AuditChangeKindDeleteFolder:
		return 50
	default:
		return 100
	}
}

func normalizeWebAuditChangePath(rawPath string) (string, error) {
	trimmedPath := strings.TrimSpace(rawPath)
	if len(trimmedPath) == 0 {
		return "", errors.New(webAuditChangePathRequiredConstant)
	}
	return filepath.Clean(trimmedPath), nil
}

func auditInspectionReportRow(inspection audit.RepositoryInspection) audit.AuditReportRow {
	finalRepository := strings.TrimSpace(inspection.CanonicalOwnerRepo)
	if len(finalRepository) == 0 {
		finalRepository = inspection.OriginOwnerRepo
	}

	nameMatches := audit.TernaryValueNotApplicable
	if inspection.IsGitRepository {
		nameMatches = audit.TernaryValueNo
		folderBaseName := filepath.Base(inspection.FolderName)
		if len(inspection.DesiredFolderName) > 0 && inspection.DesiredFolderName == folderBaseName {
			nameMatches = audit.TernaryValueYes
		}
	}

	remoteDefaultBranch := inspection.RemoteDefaultBranch
	localBranch := inspection.LocalBranch
	inSync := inspection.InSyncStatus
	originRemoteStatus := inspection.OriginRemoteStatus
	remoteProtocol := inspection.RemoteProtocol
	originMatches := inspection.OriginMatchesCanonical
	worktreeDirty := audit.TernaryValueNo
	dirtyFiles := ""

	if !inspection.IsGitRepository {
		finalRepository = string(audit.TernaryValueNotApplicable)
		originRemoteStatus = audit.OriginRemoteStatusNotApplicable
		remoteDefaultBranch = string(audit.TernaryValueNotApplicable)
		localBranch = string(audit.TernaryValueNotApplicable)
		inSync = audit.TernaryValueNotApplicable
		remoteProtocol = audit.RemoteProtocolType(string(audit.TernaryValueNotApplicable))
		originMatches = audit.TernaryValueNotApplicable
		worktreeDirty = audit.TernaryValueNotApplicable
	} else {
		if originRemoteStatus == audit.OriginRemoteStatusMissing {
			finalRepository = string(audit.TernaryValueNotApplicable)
			remoteProtocol = audit.RemoteProtocolType(string(audit.TernaryValueNotApplicable))
		}
		if len(inspection.WorktreeDirtyFiles) > 0 {
			worktreeDirty = audit.TernaryValueYes
			dirtyFiles = strings.Join(inspection.WorktreeDirtyFiles, "; ")
		}
	}

	return audit.AuditReportRow{
		FolderName:             inspection.FolderName,
		FinalRepository:        finalRepository,
		OriginRemoteStatus:     originRemoteStatus,
		NameMatches:            nameMatches,
		RemoteDefaultBranch:    remoteDefaultBranch,
		LocalBranch:            localBranch,
		InSync:                 inSync,
		RemoteProtocol:         remoteProtocol,
		OriginMatchesCanonical: originMatches,
		WorktreeDirty:          worktreeDirty,
		DirtyFiles:             dirtyFiles,
	}
}

func loadRepositoryBranches(executionContext context.Context, repository web.RepositoryDescriptor, gitExecutor execshellGitExecutor) web.BranchCatalog {
	trimmedRepositoryPath := strings.TrimSpace(repository.Path)
	if len(trimmedRepositoryPath) == 0 {
		return web.BranchCatalog{
			RepositoryID: repository.ID,
			Error:        webRepositoryPathRequiredErrorConstant,
		}
	}

	branchResult, branchError := gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitForEachRefSubcommandConstant, gitBranchCatalogFormatArgumentConstant, gitBranchCatalogReferenceArgumentConstant},
		WorkingDirectory: trimmedRepositoryPath,
	})
	if branchError != nil {
		return web.BranchCatalog{
			RepositoryID:   repository.ID,
			RepositoryPath: trimmedRepositoryPath,
			Error:          branchError.Error(),
		}
	}

	return web.BranchCatalog{
		RepositoryID:   repository.ID,
		RepositoryPath: trimmedRepositoryPath,
		Branches:       parseBranchCatalog(branchResult.StandardOutput),
	}
}

func (application *Application) webGitDependencies() (execshellGitExecutor, webGitRepositoryManager, error) {
	gitExecutor, executorError := reposdeps.ResolveGitExecutor(nil, application.logger, application.humanReadableLoggingEnabled())
	if executorError != nil {
		return nil, nil, executorError
	}

	repositoryManager, managerError := reposdeps.ResolveGitRepositoryManager(nil, gitExecutor)
	if managerError != nil {
		return nil, nil, managerError
	}

	return gitExecutor, repositoryManager, nil
}

func (application *Application) webTaskRunnerDependencies(outputWriter io.Writer, errorWriter io.Writer) (taskrunner.DependenciesResult, error) {
	return taskrunner.BuildDependencies(
		taskrunner.DependenciesConfig{
			LoggerProvider: func() *zap.Logger {
				return application.logger
			},
			HumanReadableLoggingProvider: application.humanReadableLoggingEnabled,
		},
		taskrunner.DependenciesOptions{
			Output:          outputWriter,
			Errors:          errorWriter,
			DisablePrompter: true,
		},
	)
}

func (application *Application) webAuditDependencies() (shared.RepositoryDiscoverer, execshellGitExecutor, webGitRepositoryManager, webGitHubMetadataResolver, error) {
	discoverer := reposdeps.ResolveRepositoryDiscoverer(nil)
	gitExecutor, repositoryManager, dependencyError := application.webGitDependencies()
	if dependencyError != nil {
		return nil, nil, nil, nil, dependencyError
	}

	githubResolver, resolverError := reposdeps.ResolveGitHubResolver(nil, gitExecutor)
	if resolverError != nil {
		return nil, nil, nil, nil, resolverError
	}

	return discoverer, gitExecutor, repositoryManager, githubResolver, nil
}

func (application *Application) currentRepositoryRoot(executionContext context.Context, gitExecutor execshellGitExecutor, workingDirectory string) (string, bool) {
	repositoryRootResult, repositoryRootError := gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitRevParseSubcommandConstant, gitShowTopLevelArgumentConstant},
		WorkingDirectory: workingDirectory,
	})
	if repositoryRootError != nil {
		return "", false
	}

	trimmedRepositoryRoot := strings.TrimSpace(repositoryRootResult.StandardOutput)
	if len(trimmedRepositoryRoot) == 0 {
		return "", false
	}

	return trimmedRepositoryRoot, true
}

func (application *Application) inspectRepository(
	executionContext context.Context,
	repositoryPath string,
	repositoryIndex int,
	contextCurrent bool,
	gitExecutor execshellGitExecutor,
	repositoryManager webGitRepositoryManager,
) web.RepositoryDescriptor {
	descriptor := web.RepositoryDescriptor{
		ID:             fmt.Sprintf(webRepositoryIDTemplateConstant, repositoryIndex),
		Name:           filepath.Base(strings.TrimSpace(repositoryPath)),
		Path:           strings.TrimSpace(repositoryPath),
		ContextCurrent: contextCurrent,
	}

	currentBranch, currentBranchError := repositoryManager.GetCurrentBranch(executionContext, descriptor.Path)
	if currentBranchError == nil {
		descriptor.CurrentBranch = strings.TrimSpace(currentBranch)
	} else {
		descriptor.Error = currentBranchError.Error()
	}

	cleanWorktree, cleanWorktreeError := repositoryManager.CheckCleanWorktree(executionContext, descriptor.Path)
	if cleanWorktreeError == nil {
		descriptor.Dirty = !cleanWorktree
	} else if len(descriptor.Error) == 0 {
		descriptor.Error = cleanWorktreeError.Error()
	}

	defaultBranch, defaultBranchError := resolveRepositoryDefaultBranch(executionContext, gitExecutor, descriptor.Path)
	if defaultBranchError == nil {
		descriptor.DefaultBranch = defaultBranch
	} else if len(descriptor.Error) == 0 {
		descriptor.Error = defaultBranchError.Error()
	}

	return descriptor
}

func resolveRepositoryDefaultBranch(executionContext context.Context, gitExecutor execshellGitExecutor, repositoryPath string) (string, error) {
	defaultBranchResult, defaultBranchError := gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitSymbolicRefSubcommandConstant, gitSymbolicRefQuietArgumentConstant, gitShortArgumentConstant, gitOriginHeadReferenceConstant},
		WorkingDirectory: repositoryPath,
	})
	if defaultBranchError != nil {
		return "", defaultBranchError
	}

	trimmedDefaultBranch := strings.TrimSpace(defaultBranchResult.StandardOutput)
	trimmedDefaultBranch = strings.TrimPrefix(trimmedDefaultBranch, gitOriginPrefixConstant)

	return trimmedDefaultBranch, nil
}

func parseBranchCatalog(rawOutput string) []web.BranchDescriptor {
	trimmedOutput := strings.TrimSpace(rawOutput)
	if len(trimmedOutput) == 0 {
		return nil
	}

	branchDescriptors := make([]web.BranchDescriptor, 0)
	for _, line := range strings.Split(trimmedOutput, "\n") {
		trimmedLine := strings.TrimSpace(line)
		if len(trimmedLine) == 0 {
			continue
		}

		parts := strings.SplitN(trimmedLine, "|", 3)
		if len(parts) < 2 {
			continue
		}

		branchName := strings.TrimSpace(parts[1])
		if len(branchName) == 0 {
			continue
		}

		branchDescriptor := web.BranchDescriptor{
			Name:    branchName,
			Current: strings.TrimSpace(parts[0]) == "*",
		}
		if len(parts) == 3 {
			branchDescriptor.Upstream = strings.TrimSpace(parts[2])
		}

		branchDescriptors = append(branchDescriptors, branchDescriptor)
	}

	sort.Slice(branchDescriptors, func(leftIndex int, rightIndex int) bool {
		leftBranch := branchDescriptors[leftIndex]
		rightBranch := branchDescriptors[rightIndex]
		if leftBranch.Current != rightBranch.Current {
			return leftBranch.Current
		}
		return leftBranch.Name < rightBranch.Name
	})

	return branchDescriptors
}

func (application *Application) commandCatalog() web.CommandCatalog {
	commandDescriptors := collectCommandCatalogEntries(application.rootCommand)
	return web.CommandCatalog{
		Application: applicationNameConstant,
		Commands:    commandDescriptors,
	}
}

func collectCommandCatalogEntries(rootCommand *cobra.Command) []web.CommandDescriptor {
	if rootCommand == nil {
		return nil
	}

	commandDescriptors := make([]web.CommandDescriptor, 0)
	var collectCommands func(*cobra.Command)
	collectCommands = func(command *cobra.Command) {
		if command == nil || command.Hidden {
			return
		}

		commandDescriptors = append(commandDescriptors, buildCommandDescriptor(command))

		subcommands := visibleSubcommands(command)
		for _, subcommand := range subcommands {
			collectCommands(subcommand)
		}
	}
	collectCommands(rootCommand)

	sort.Slice(commandDescriptors, func(leftIndex int, rightIndex int) bool {
		return commandDescriptors[leftIndex].Path < commandDescriptors[rightIndex].Path
	})

	return commandDescriptors
}

func buildCommandDescriptor(command *cobra.Command) web.CommandDescriptor {
	visibleSubcommandNames := make([]string, 0)
	for _, subcommand := range visibleSubcommands(command) {
		visibleSubcommandNames = append(visibleSubcommandNames, strings.TrimSpace(subcommand.CommandPath()))
	}

	return web.CommandDescriptor{
		Path:        strings.TrimSpace(command.CommandPath()),
		Use:         strings.TrimSpace(command.UseLine()),
		Name:        strings.TrimSpace(command.Name()),
		Short:       strings.TrimSpace(command.Short),
		Long:        strings.TrimSpace(command.Long),
		Example:     strings.TrimSpace(command.Example),
		Aliases:     sortedStrings(command.Aliases),
		Runnable:    command.Runnable(),
		Actionable:  commandIsActionable(command),
		Target:      buildCommandTargetDescriptor(command),
		Flags:       collectFlagDescriptors(command),
		Subcommands: visibleSubcommandNames,
	}
}

func buildCommandTargetDescriptor(command *cobra.Command) web.CommandTargetDescriptor {
	commandPath := ""
	if command != nil {
		commandPath = strings.TrimSpace(command.CommandPath())
	}

	descriptor := web.CommandTargetDescriptor{
		Group:         webCommandGroupGeneralConstant,
		Repository:    web.CommandTargetRequirementNone,
		Ref:           web.CommandTargetRequirementNone,
		Path:          web.CommandTargetRequirementNone,
		SupportsBatch: false,
	}

	switch commandPath {
	case applicationNameConstant + " " + branchChangeTopLevelUseNameConstant,
		applicationNameConstant + " " + defaultCommandUseNameConstant:
		descriptor.Group = webCommandGroupBranchConstant
		descriptor.Repository = web.CommandTargetRequirementRequired
		descriptor.Ref = web.CommandTargetRequirementRequired
		descriptor.SupportsBatch = true
	case applicationNameConstant + " " + auditOperationNameConstant,
		applicationNameConstant + " " + repoFolderNamespaceUseNameConstant + " " + renameCommandUseNameConstant:
		descriptor.Group = webCommandGroupRepositoryConstant
		descriptor.Repository = web.CommandTargetRequirementRequired
		descriptor.SupportsBatch = true
	case applicationNameConstant + " " + repoRemoteNamespaceUseNameConstant + " " + updateRemoteCanonicalUseNameConstant:
		descriptor.Group = webCommandGroupRemoteConstant
		descriptor.Repository = web.CommandTargetRequirementRequired
		descriptor.SupportsBatch = true
	case applicationNameConstant + " " + repoPullRequestsNamespaceUseNameConstant + " " + prsDeleteCommandUseNameConstant:
		descriptor.Group = webCommandGroupPullRequestsConstant
		descriptor.Repository = web.CommandTargetRequirementRequired
		descriptor.SupportsBatch = true
	case applicationNameConstant + " " + repoPackagesNamespaceUseNameConstant + " " + packagesDeleteCommandUseNameConstant:
		descriptor.Group = webCommandGroupPackagesConstant
		descriptor.Repository = web.CommandTargetRequirementRequired
		descriptor.SupportsBatch = true
	case applicationNameConstant + " " + repoFilesNamespaceUseNameConstant + " " + filesAddCommandUseNameConstant:
		descriptor.Group = webCommandGroupFilesConstant
		descriptor.Repository = web.CommandTargetRequirementRequired
		descriptor.Path = web.CommandTargetRequirementRequired
		descriptor.SupportsBatch = true
		descriptor.DraftTemplate = webDraftTemplateFilesAddConstant
	case applicationNameConstant + " " + repoFilesNamespaceUseNameConstant + " " + filesReplaceCommandUseNameConstant:
		descriptor.Group = webCommandGroupFilesConstant
		descriptor.Repository = web.CommandTargetRequirementRequired
		descriptor.Ref = web.CommandTargetRequirementOptional
		descriptor.Path = web.CommandTargetRequirementRequired
		descriptor.SupportsBatch = true
		descriptor.DraftTemplate = webDraftTemplateFilesReplaceConstant
	case applicationNameConstant + " " + repoFilesNamespaceUseNameConstant + " " + removeCommandUseNameConstant:
		descriptor.Group = webCommandGroupFilesConstant
		descriptor.Repository = web.CommandTargetRequirementRequired
		descriptor.Path = web.CommandTargetRequirementRequired
		descriptor.SupportsBatch = true
		descriptor.DraftTemplate = webDraftTemplateFilesRemoveConstant
	case applicationNameConstant + " " + workflowCommandOperationNameConstant:
		descriptor.Group = webCommandGroupGeneralConstant
		descriptor.Repository = web.CommandTargetRequirementRequired
		descriptor.SupportsBatch = true
	case applicationNameConstant + " " + versionCommandUseNameConstant:
		descriptor.Group = webCommandGroupGeneralConstant
	}

	return descriptor
}

func commandIsActionable(command *cobra.Command) bool {
	if command == nil || !command.Runnable() {
		return false
	}

	commandPath := strings.TrimSpace(command.CommandPath())
	if _, excluded := nonActionableCommandPaths[commandPath]; excluded {
		return false
	}

	return commandSupportsBareExecution(command)
}

func commandSupportsBareExecution(command *cobra.Command) bool {
	if command == nil {
		return false
	}

	if validationError := command.ValidateArgs([]string{}); validationError != nil {
		return false
	}

	if validationError := command.ValidateRequiredFlags(); validationError != nil {
		return false
	}

	if validationError := command.ValidateFlagGroups(); validationError != nil {
		return false
	}

	return true
}

func visibleSubcommands(command *cobra.Command) []*cobra.Command {
	if command == nil {
		return nil
	}

	visibleCommands := make([]*cobra.Command, 0)
	for _, subcommand := range command.Commands() {
		if subcommand == nil || subcommand.Hidden {
			continue
		}
		visibleCommands = append(visibleCommands, subcommand)
	}

	sort.Slice(visibleCommands, func(leftIndex int, rightIndex int) bool {
		return visibleCommands[leftIndex].CommandPath() < visibleCommands[rightIndex].CommandPath()
	})

	return visibleCommands
}

func collectFlagDescriptors(command *cobra.Command) []web.FlagDescriptor {
	if command == nil {
		return nil
	}

	flagDescriptorsByName := map[string]web.FlagDescriptor{}
	collectFromFlagSet := func(flagSet *pflag.FlagSet) {
		if flagSet == nil {
			return
		}
		flagSet.VisitAll(func(flag *pflag.Flag) {
			if flag == nil || flag.Hidden {
				return
			}
			flagDescriptorsByName[flag.Name] = web.FlagDescriptor{
				Name:         flag.Name,
				Shorthand:    flag.Shorthand,
				Usage:        strings.TrimSpace(flag.Usage),
				Type:         flag.Value.Type(),
				Default:      strings.TrimSpace(flag.DefValue),
				NoOptDefault: strings.TrimSpace(flag.NoOptDefVal),
				Required:     flagIsRequired(flag),
			}
		})
	}

	collectFromFlagSet(command.NonInheritedFlags())
	collectFromFlagSet(command.InheritedFlags())

	flagDescriptors := make([]web.FlagDescriptor, 0, len(flagDescriptorsByName))
	for _, flagDescriptor := range flagDescriptorsByName {
		flagDescriptors = append(flagDescriptors, flagDescriptor)
	}

	sort.Slice(flagDescriptors, func(leftIndex int, rightIndex int) bool {
		return flagDescriptors[leftIndex].Name < flagDescriptors[rightIndex].Name
	})

	return flagDescriptors
}

func flagIsRequired(flag *pflag.Flag) bool {
	if flag == nil {
		return false
	}
	if flag.Annotations == nil {
		return false
	}
	_, required := flag.Annotations[cobra.BashCompOneRequiredFlag]
	return required
}

func sortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	sanitizedValues := make([]string, 0, len(values))
	for _, value := range values {
		trimmedValue := strings.TrimSpace(value)
		if len(trimmedValue) == 0 {
			continue
		}
		sanitizedValues = append(sanitizedValues, trimmedValue)
	}

	sort.Strings(sanitizedValues)
	return sanitizedValues
}
