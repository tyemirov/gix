package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/zap"

	changelogcmd "github.com/tyemirov/gix/cmd/cli/changelog"
	commitcmd "github.com/tyemirov/gix/cmd/cli/commit"
	"github.com/tyemirov/gix/internal/audit"
	internalchangelog "github.com/tyemirov/gix/internal/changelog"
	"github.com/tyemirov/gix/internal/commitmsg"
	"github.com/tyemirov/gix/internal/execshell"
	reposdeps "github.com/tyemirov/gix/internal/repos/dependencies"
	"github.com/tyemirov/gix/internal/repos/shared"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	rootutils "github.com/tyemirov/gix/internal/utils/roots"
	"github.com/tyemirov/gix/internal/web"
	"github.com/tyemirov/gix/internal/workflow"
	"github.com/tyemirov/gix/pkg/taskrunner"
	"github.com/tyemirov/utils/llm"
)

const (
	webLaunchModeCurrentRepositoryConstant      = "current_repo"
	webLaunchModeConfiguredRootsConstant        = "configured_roots"
	webLaunchModeDiscoveredRepositoriesConstant = "discovered_repositories"
	webNoRepositoriesErrorTemplateConstant      = "no Git repositories found beneath %s"
	webRepositoryIDTemplateConstant             = "repo-%03d"
	webDynamicRepositoryIDTemplateConstant      = "repo-path-%016x"
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
	gitHeadReferenceConstant                    = "HEAD"
	gitOriginHeadReferenceConstant              = "refs/remotes/origin/HEAD"
	gitOriginPrefixConstant                     = "origin/"
	webGitMetadataDirectoryNameConstant         = ".git"
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
	webAuditChangePathAbsoluteRequiredConstant  = "audit change path must be absolute"
	webAuditChangeDeleteRejectedConstant        = "delete_folder requires confirm_delete"
	webAuditChangeDeleteRootRejectedConstant    = "delete_folder does not allow deleting filesystem roots"
	webAuditChangeProtocolMissingConstant       = "convert_protocol requires source_protocol and target_protocol"
	webAuditChangeSyncStrategyTemplateConstant  = "unsupported sync strategy %q"
	webAuditChangeKindTemplateConstant          = "unsupported audit change kind %q"
	webAuditChangeChangelogBranchRejected       = "update_changelog requires the current branch to match the default branch"
	webAuditChangeChangelogTaggedRejected       = "update_changelog requires HEAD to be untagged"
	webAuditChangeCommitMessageTemplateConstant = "Generated commit message:\n%s\n"
	webAuditChangeUpdatedFileTemplateConstant   = "UPDATED: %s\n"
	webAuditChangelogFileNameConstant           = "CHANGELOG.md"
	webAuditDefaultReleaseVersionConstant       = "v0.1.0"
	webAuditGitAddSubcommandConstant            = "add"
	webAuditGitCommitSubcommandConstant         = "commit"
	webAuditGitAllPathsArgumentConstant         = "-A"
	webAuditGitCommitMessageArgumentConstant    = "-m"
	webAuditGitDescribeSubcommandConstant       = "describe"
	webAuditGitDescribeTagsArgumentConstant     = "--tags"
	webAuditGitDescribeAbbrevArgumentConstant   = "--abbrev=0"
	webAuditGitTagSubcommandConstant            = "tag"
	webAuditGitPointsAtArgumentConstant         = "--points-at"
	webAuditChangeStatusSucceededConstant       = "succeeded"
	webAuditChangeStatusSkippedConstant         = "skipped"
	webAuditChangeStatusFailedConstant          = "failed"
)

var webReleaseVersionPattern = regexp.MustCompile(`^(v?)(\d+)\.(\d+)\.(\d+)(?:-rc(\d+))?$`)

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
	bind string
	port int
}

func newWebLaunchConfiguration(rawPortValue string, rawBindValue string, bindProvided bool) (webLaunchConfiguration, error) {
	portValue, portError := parseWebLaunchPort(rawPortValue)
	if portError != nil {
		return webLaunchConfiguration{}, portError
	}

	bindValue, bindError := parseWebLaunchBind(rawBindValue, bindProvided)
	if bindError != nil {
		return webLaunchConfiguration{}, bindError
	}

	return webLaunchConfiguration{
		bind: bindValue,
		port: portValue,
	}, nil
}

func parseWebLaunchPort(rawValue string) (int, error) {
	trimmedValue := strings.TrimSpace(rawValue)
	if len(trimmedValue) == 0 {
		trimmedValue = webDefaultPortConstant
	}

	portValue, parseError := strconv.Atoi(trimmedValue)
	if parseError != nil {
		return 0, fmt.Errorf(webPortInvalidTemplateConstant, rawValue)
	}
	if portValue < 1 || portValue > 65535 {
		return 0, fmt.Errorf(webPortRangeTemplateConstant, portValue)
	}

	return portValue, nil
}

func parseWebLaunchBind(rawValue string, bindProvided bool) (string, error) {
	if !bindProvided {
		return webListenHostConstant, nil
	}

	trimmedValue := strings.TrimSpace(rawValue)
	if len(trimmedValue) == 0 {
		return "", errors.New(webBindRequiredConstant)
	}

	return trimmedValue, nil
}

func (configuration webLaunchConfiguration) listenAddress() string {
	return net.JoinHostPort(configuration.bind, strconv.Itoa(configuration.port))
}

func (configuration webLaunchConfiguration) launchURL() string {
	return (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(configuration.bind, strconv.Itoa(configuration.port)),
	}).String()
}

func (application *Application) handleWebLaunch(command *cobra.Command, arguments []string) (bool, error) {
	launchConfiguration, requested, configurationError := application.resolveWebLaunchConfiguration(command, arguments)
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

	launchRoots, launchRootsError := resolveWebLaunchRoots(command)
	if launchRootsError != nil {
		return true, launchRootsError
	}

	repositoryCatalog := application.repositoryCatalog(executionContext, launchRoots)

	return true, application.webRunner(executionContext, web.ServerOptions{
		Address:           launchConfiguration.listenAddress(),
		Repositories:      repositoryCatalog,
		BrowseDirectories: application.newWebDirectoryBrowser(),
		InspectAudit:      application.newWebAuditInspector(),
		ApplyAuditChanges: application.newWebAuditChangeExecutor(),
	})
}

func (application *Application) resolveWebLaunchConfiguration(command *cobra.Command, arguments []string) (webLaunchConfiguration, bool, error) {
	if command == nil {
		return webLaunchConfiguration{}, false, nil
	}

	webEnabled, _, flagError := flagutils.BoolFlag(command, webFlagNameConstant)
	switch {
	case flagError == nil:
	case errors.Is(flagError, flagutils.ErrFlagNotDefined):
		return webLaunchConfiguration{}, false, nil
	default:
		return webLaunchConfiguration{}, false, flagError
	}

	rawBindValue, bindChanged, bindFlagError := flagutils.StringFlag(command, webBindFlagNameConstant)
	switch {
	case bindFlagError == nil:
	case errors.Is(bindFlagError, flagutils.ErrFlagNotDefined):
		rawBindValue = ""
		bindChanged = false
	default:
		return webLaunchConfiguration{}, false, bindFlagError
	}

	rawPortValue, portChanged, portFlagError := flagutils.StringFlag(command, webPortFlagNameConstant)
	switch {
	case portFlagError == nil:
	case errors.Is(portFlagError, flagutils.ErrFlagNotDefined):
		rawPortValue = ""
		portChanged = false
	default:
		return webLaunchConfiguration{}, false, portFlagError
	}

	if !webEnabled && !bindChanged && !portChanged {
		return webLaunchConfiguration{}, false, nil
	}

	if !webEnabled && (bindChanged || portChanged) {
		return webLaunchConfiguration{}, true, errors.New(webNetworkFlagsRequireWebConstant)
	}

	if webEnabled && len(arguments) > 0 {
		return webLaunchConfiguration{}, true, errors.New(webPositionalArgumentsRequirePortFlagConstant)
	}

	launchConfiguration, configurationError := newWebLaunchConfiguration(rawPortValue, rawBindValue, bindChanged)
	if configurationError != nil {
		return webLaunchConfiguration{}, true, configurationError
	}
	return launchConfiguration, true, nil
}

func resolveWebLaunchRoots(command *cobra.Command) ([]string, error) {
	if command == nil {
		return nil, nil
	}

	return rootutils.FlagValues(command)
}

func (application *Application) launchWebInterface(executionContext context.Context, options web.ServerOptions) error {
	return web.Run(executionContext, options)
}

func (application *Application) newWebDirectoryBrowser() web.DirectoryBrowser {
	repositoryDiscoverer := reposdeps.ResolveRepositoryDiscoverer(nil)
	gitExecutor, repositoryManager, dependencyError := application.webGitDependencies()

	return func(executionContext context.Context, path string) web.DirectoryListing {
		normalizedPath := canonicalWebPath(path)
		if len(normalizedPath) == 0 {
			return web.DirectoryListing{Error: webRepositoryPathRequiredErrorConstant}
		}

		discoveredRepositories, discoverError := repositoryDiscoverer.DiscoverRepositories([]string{normalizedPath})
		if discoverError != nil {
			return web.DirectoryListing{Path: normalizedPath, Error: discoverError.Error()}
		}

		folderIndex := make(map[string]int, len(discoveredRepositories))
		folders := make([]web.FolderDescriptor, 0, len(discoveredRepositories))
		for _, repositoryPath := range discoveredRepositories {
			normalizedRepositoryPath := canonicalWebPath(repositoryPath)
			if normalizedRepositoryPath == normalizedPath {
				continue
			}

			relativeRepositoryPath := strings.TrimPrefix(normalizedRepositoryPath, normalizedPath)
			relativeRepositoryPath = strings.TrimPrefix(relativeRepositoryPath, string(os.PathSeparator))
			if len(relativeRepositoryPath) == 0 {
				continue
			}

			relativeSegments := strings.Split(filepath.ToSlash(relativeRepositoryPath), "/")
			immediateChildName := strings.TrimSpace(relativeSegments[0])
			if len(immediateChildName) == 0 || immediateChildName == "." {
				continue
			}

			immediateChildPath := canonicalWebPath(filepath.Join(normalizedPath, immediateChildName))
			folderPosition, folderExists := folderIndex[immediateChildPath]
			if !folderExists {
				folderIndex[immediateChildPath] = len(folders)
				folders = append(folders, web.FolderDescriptor{
					Name: immediateChildName,
					Path: immediateChildPath,
				})
				folderPosition = len(folders) - 1
			}

			if len(relativeSegments) != 1 || normalizedRepositoryPath != immediateChildPath {
				continue
			}

			repositoryDescriptor := newDynamicWebRepositoryDescriptor(immediateChildPath)
			if dependencyError != nil {
				repositoryDescriptor.Error = dependencyError.Error()
			} else {
				repositoryDescriptor = application.inspectDynamicRepository(
					executionContext,
					immediateChildPath,
					gitExecutor,
					repositoryManager,
				)
			}

			folders[folderPosition].Repository = &repositoryDescriptor
		}

		sort.SliceStable(folders, func(first int, second int) bool {
			return folders[first].Name < folders[second].Name
		})

		return web.DirectoryListing{
			Path:    normalizedPath,
			Folders: folders,
		}
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
	executionOutcome := workflow.ExecutionOutcome{}
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
	case web.AuditChangeKindUpdateChangelog, web.AuditChangeKindCommitChanges:
		dependencies, dependencyError := application.webTaskRunnerDependencies(outputBuffer, errorBuffer)
		if dependencyError != nil {
			applyError = dependencyError
			break
		}

		switch change.Kind {
		case web.AuditChangeKindUpdateChangelog:
			applyError = application.applyWebAuditUpdateChangelog(
				executionContext,
				dependencies.GitExecutor,
				dependencies.FileSystem,
				normalizedPath,
				outputBuffer,
			)
		case web.AuditChangeKindCommitChanges:
			applyError = application.applyWebAuditCommitChanges(
				executionContext,
				dependencies.GitExecutor,
				normalizedPath,
				outputBuffer,
			)
		}
	default:
		dependencies, dependencyError := application.webTaskRunnerDependencies(outputBuffer, errorBuffer)
		if dependencyError != nil {
			applyError = dependencyError
			break
		}

		executionOutcome, applyError = executeWebAuditWorkflowChange(executionContext, dependencies.Workflow, normalizedPath, change)
	}

	result.Stdout = outputBuffer.String()
	result.Stderr = errorBuffer.String()
	result.Status = webAuditChangeResultStatus(executionOutcome, applyError)
	if result.Status == webAuditChangeStatusFailedConstant {
		result.Status = webAuditChangeStatusFailedConstant
		result.Error = applyError.Error()
		return result
	}

	if result.Status == webAuditChangeStatusSkippedConstant {
		message = webAuditChangeSkippedMessage(change.Kind)
	} else {
		message = webAuditChangeMessage(change.Kind)
	}
	if len(message) > 0 {
		result.Message = message
	}

	return result
}

func (application *Application) repositoryCatalog(executionContext context.Context, launchRoots []string) web.RepositoryCatalog {
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

	discoverer := reposdeps.ResolveRepositoryDiscoverer(nil)
	currentRepositoryRoot, currentRepositoryAvailable := application.currentRepositoryRoot(executionContext, gitExecutor, workingDirectory)
	if len(launchRoots) > 0 {
		return application.repositoryCatalogForLaunchRoots(
			executionContext,
			launchRoots,
			workingDirectory,
			gitExecutor,
			repositoryManager,
			discoverer,
		)
	}

	if currentRepositoryAvailable {
		explorerRoot := currentRepositoryExplorerRoot(currentRepositoryRoot)
		discoveredRepositories, discoverError := discoverer.DiscoverRepositories([]string{explorerRoot})
		if discoverError != nil || len(discoveredRepositories) == 0 {
			repositoryDescriptor := application.inspectRepository(executionContext, currentRepositoryRoot, 1, true, gitExecutor, repositoryManager)
			return web.RepositoryCatalog{
				LaunchPath:           workingDirectory,
				ExplorerRoot:         explorerRoot,
				LaunchMode:           webLaunchModeCurrentRepositoryConstant,
				SelectedRepositoryID: repositoryDescriptor.ID,
				Repositories:         []web.RepositoryDescriptor{repositoryDescriptor},
			}
		}

		repositoryDescriptors := make([]web.RepositoryDescriptor, 0, len(discoveredRepositories))
		selectedRepositoryID := ""
		for repositoryIndex, repositoryPath := range discoveredRepositories {
			contextCurrent := canonicalWebPath(repositoryPath) == canonicalWebPath(currentRepositoryRoot)
			repositoryDescriptor := application.inspectRepository(
				executionContext,
				repositoryPath,
				repositoryIndex+1,
				contextCurrent,
				gitExecutor,
				repositoryManager,
			)
			if contextCurrent {
				selectedRepositoryID = repositoryDescriptor.ID
			}
			repositoryDescriptors = append(repositoryDescriptors, repositoryDescriptor)
		}
		if len(selectedRepositoryID) == 0 && len(repositoryDescriptors) > 0 {
			selectedRepositoryID = repositoryDescriptors[0].ID
		}

		return web.RepositoryCatalog{
			LaunchPath:           workingDirectory,
			ExplorerRoot:         explorerRoot,
			LaunchMode:           webLaunchModeCurrentRepositoryConstant,
			SelectedRepositoryID: selectedRepositoryID,
			Repositories:         repositoryDescriptors,
		}
	}

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

	return web.RepositoryCatalog{
		LaunchPath:   workingDirectory,
		ExplorerRoot: workingDirectory,
		LaunchMode:   webLaunchModeDiscoveredRepositoriesConstant,
		Repositories: repositoryDescriptors,
	}
}

func (application *Application) repositoryCatalogForLaunchRoots(
	executionContext context.Context,
	launchRoots []string,
	workingDirectory string,
	gitExecutor execshellGitExecutor,
	repositoryManager webGitRepositoryManager,
	discoverer shared.RepositoryDiscoverer,
) web.RepositoryCatalog {
	normalizedRoots, launchPath := webLaunchRootsDetails(launchRoots)
	if len(normalizedRoots) == 0 {
		return web.RepositoryCatalog{
			LaunchPath:  workingDirectory,
			LaunchRoots: append([]string(nil), launchRoots...),
			Error:       fmt.Sprintf(webNoRepositoriesErrorTemplateConstant, workingDirectory),
		}
	}

	discoveredRepositories, discoverError := discoverer.DiscoverRepositories(normalizedRoots)
	if discoverError != nil {
		return web.RepositoryCatalog{
			LaunchPath:   launchPath,
			LaunchRoots:  append([]string(nil), normalizedRoots...),
			ExplorerRoot: launchPath,
			LaunchMode:   webLaunchModeConfiguredRootsConstant,
			Error:        discoverError.Error(),
		}
	}

	if len(discoveredRepositories) == 0 {
		return web.RepositoryCatalog{
			LaunchPath:   launchPath,
			LaunchRoots:  append([]string(nil), normalizedRoots...),
			ExplorerRoot: launchPath,
			LaunchMode:   webLaunchModeConfiguredRootsConstant,
		}
	}

	sort.Strings(discoveredRepositories)
	repositoryDescriptors, selectedRepositoryID := application.webRepositoryDescriptors(
		executionContext,
		discoveredRepositories,
		"",
		gitExecutor,
		repositoryManager,
	)

	return web.RepositoryCatalog{
		LaunchPath:           launchPath,
		LaunchRoots:          append([]string(nil), normalizedRoots...),
		ExplorerRoot:         launchPath,
		LaunchMode:           webLaunchModeConfiguredRootsConstant,
		SelectedRepositoryID: selectedRepositoryID,
		Repositories:         repositoryDescriptors,
	}
}

func (application *Application) webRepositoryDescriptors(
	executionContext context.Context,
	discoveredRepositories []string,
	currentRepositoryRoot string,
	gitExecutor execshellGitExecutor,
	repositoryManager webGitRepositoryManager,
) ([]web.RepositoryDescriptor, string) {
	repositoryDescriptors := make([]web.RepositoryDescriptor, 0, len(discoveredRepositories))
	selectedRepositoryID := ""
	currentRepositoryPath := canonicalWebPath(currentRepositoryRoot)

	for repositoryIndex, repositoryPath := range discoveredRepositories {
		contextCurrent := len(currentRepositoryPath) > 0 && canonicalWebPath(repositoryPath) == currentRepositoryPath
		repositoryDescriptor := application.inspectRepository(
			executionContext,
			repositoryPath,
			repositoryIndex+1,
			contextCurrent,
			gitExecutor,
			repositoryManager,
		)
		if contextCurrent {
			selectedRepositoryID = repositoryDescriptor.ID
		}
		repositoryDescriptors = append(repositoryDescriptors, repositoryDescriptor)
	}
	return repositoryDescriptors, selectedRepositoryID
}

func webLaunchRootsDetails(launchRoots []string) ([]string, string) {
	normalizedRoots := make([]string, 0, len(launchRoots))
	for _, launchRoot := range launchRoots {
		canonicalRoot := canonicalWebPath(launchRoot)
		if len(canonicalRoot) == 0 {
			continue
		}
		normalizedRoots = append(normalizedRoots, canonicalRoot)
	}

	return normalizedRoots, commonWebLaunchPath(normalizedRoots)
}

func commonWebLaunchPath(launchRoots []string) string {
	if len(launchRoots) == 0 {
		return ""
	}

	commonPath := canonicalWebPath(launchRoots[0])
	for _, launchRoot := range launchRoots[1:] {
		commonPath = sharedWebLaunchAncestorPath(commonPath, launchRoot)
		if len(commonPath) == 0 {
			return ""
		}
	}

	return commonPath
}

func sharedWebLaunchAncestorPath(basePath string, candidatePath string) string {
	normalizedBasePath := canonicalWebPath(basePath)
	normalizedCandidatePath := canonicalWebPath(candidatePath)
	if len(normalizedBasePath) == 0 || len(normalizedCandidatePath) == 0 {
		return ""
	}

	for {
		relativePath, relativePathError := filepath.Rel(normalizedBasePath, normalizedCandidatePath)
		if relativePathError == nil && relativeWebPathWithinBase(relativePath) {
			return normalizedBasePath
		}

		parentPath := filepath.Dir(normalizedBasePath)
		if parentPath == normalizedBasePath {
			return ""
		}
		normalizedBasePath = parentPath
	}
}

func relativeWebPathWithinBase(relativePath string) bool {
	trimmedRelativePath := strings.TrimSpace(relativePath)
	if len(trimmedRelativePath) == 0 || trimmedRelativePath == "." {
		return true
	}

	normalizedRelativePath := filepath.Clean(trimmedRelativePath)
	if normalizedRelativePath == ".." {
		return false
	}

	return !strings.HasPrefix(normalizedRelativePath, ".."+string(os.PathSeparator))
}

func currentRepositoryExplorerRoot(currentRepositoryRoot string) string {
	trimmedRepositoryRoot := strings.TrimSpace(currentRepositoryRoot)
	if len(trimmedRepositoryRoot) == 0 {
		return ""
	}

	return trimmedRepositoryRoot
}

func canonicalWebPath(path string) string {
	trimmedPath := strings.TrimSpace(path)
	if len(trimmedPath) == 0 {
		return ""
	}

	absolutePath, absolutePathError := filepath.Abs(trimmedPath)
	if absolutePathError != nil {
		return filepath.Clean(trimmedPath)
	}

	return filepath.Clean(absolutePath)
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
		HeadTagged:             inspection.HeadTagged,
		InSync:                 string(row.InSync),
		RemoteProtocol:         string(row.RemoteProtocol),
		OriginMatchesCanonical: string(row.OriginMatchesCanonical),
		WorktreeDirty:          string(row.WorktreeDirty),
		DirtyFiles:             row.DirtyFiles,
		DirtyFileEntries:       parseWebAuditDirtyFileEntries(inspection.WorktreeDirtyFiles),
	}
}

func parseWebAuditDirtyFileEntries(worktreeDirtyFiles []string) []web.AuditDirtyFileEntry {
	entries := make([]web.AuditDirtyFileEntry, 0, len(worktreeDirtyFiles))
	for _, worktreeDirtyFile := range worktreeDirtyFiles {
		dirtyFileEntry, ok := parseWebAuditDirtyFileEntry(worktreeDirtyFile)
		if !ok {
			continue
		}
		entries = append(entries, dirtyFileEntry)
	}
	return entries
}

func parseWebAuditDirtyFileEntry(rawEntry string) (web.AuditDirtyFileEntry, bool) {
	trimmedEntry := strings.TrimSpace(rawEntry)
	if len(trimmedEntry) == 0 {
		return web.AuditDirtyFileEntry{}, false
	}

	if len(trimmedEntry) >= 2 {
		statusValue := strings.TrimSpace(trimmedEntry[:2])
		fileValue := strings.TrimSpace(trimmedEntry[2:])
		if len(statusValue) > 0 && len(fileValue) > 0 {
			return web.AuditDirtyFileEntry{
				Status: statusValue,
				File:   fileValue,
			}, true
		}
	}

	statusValue := strings.TrimSpace(trimmedEntry[:1])
	fileValue := strings.TrimSpace(trimmedEntry[1:])
	if len(statusValue) == 0 || len(fileValue) == 0 {
		return web.AuditDirtyFileEntry{}, false
	}

	return web.AuditDirtyFileEntry{
		Status: statusValue,
		File:   fileValue,
	}, true
}

func (application *Application) applyWebAuditCommitChanges(
	executionContext context.Context,
	gitExecutor shared.GitExecutor,
	repositoryPath string,
	outputWriter io.Writer,
) error {
	client, configuration, clientError := application.webCommitMessageClient()
	if clientError != nil {
		return fmt.Errorf("audit.commit.client %s: %w", repositoryPath, clientError)
	}

	generator := commitmsg.Generator{
		GitExecutor: gitExecutor,
		Client:      client,
		Logger:      application.logger,
	}
	result, generateError := generator.Generate(executionContext, commitmsg.Options{
		RepositoryPath: repositoryPath,
		Source:         commitmsg.DiffSourceAll,
		MaxTokens:      configuration.MaxTokens,
		Temperature:    webOptionalTemperature(configuration.Temperature),
	})
	if generateError != nil {
		return fmt.Errorf("audit.commit.generate %s: %w", repositoryPath, generateError)
	}

	if outputWriter != nil {
		_, _ = fmt.Fprintf(outputWriter, webAuditChangeCommitMessageTemplateConstant, result.Message)
	}

	addResult, addError := gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{webAuditGitAddSubcommandConstant, webAuditGitAllPathsArgumentConstant},
		WorkingDirectory: repositoryPath,
	})
	if addError != nil {
		return fmt.Errorf("audit.commit.stage %s: %w", repositoryPath, addError)
	}
	writeWebExecutionOutput(outputWriter, addResult)

	commitResult, commitError := gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments: []string{
			webAuditGitCommitSubcommandConstant,
			webAuditGitCommitMessageArgumentConstant,
			result.Message,
		},
		WorkingDirectory: repositoryPath,
	})
	if commitError != nil {
		return fmt.Errorf("audit.commit.apply %s: %w", repositoryPath, commitError)
	}
	writeWebExecutionOutput(outputWriter, commitResult)
	return nil
}

func (application *Application) applyWebAuditUpdateChangelog(
	executionContext context.Context,
	gitExecutor shared.GitExecutor,
	fileSystem shared.FileSystem,
	repositoryPath string,
	outputWriter io.Writer,
) error {
	changelogEligibilityError := validateWebAuditChangelogEligibility(executionContext, gitExecutor, repositoryPath)
	if changelogEligibilityError != nil {
		return fmt.Errorf("audit.changelog.guard %s: %w", repositoryPath, changelogEligibilityError)
	}

	client, configuration, clientError := application.webChangelogMessageClient()
	if clientError != nil {
		return fmt.Errorf("audit.changelog.client %s: %w", repositoryPath, clientError)
	}

	versionValue, versionError := detectWebReleaseVersion(executionContext, gitExecutor, fileSystem, repositoryPath)
	if versionError != nil {
		return fmt.Errorf("audit.changelog.version %s: %w", repositoryPath, versionError)
	}

	releaseDateValue := time.Now().Format("2006-01-02")
	generator := internalchangelog.Generator{
		GitExecutor: gitExecutor,
		Client:      client,
		Logger:      application.logger,
	}
	result, generateError := generator.Generate(executionContext, internalchangelog.Options{
		RepositoryPath:  repositoryPath,
		Version:         versionValue,
		ReleaseDate:     releaseDateValue,
		IncludeWorktree: true,
		MaxTokens:       configuration.MaxTokens,
		Temperature:     webOptionalTemperature(configuration.Temperature),
	})
	if generateError != nil {
		return fmt.Errorf("audit.changelog.generate %s: %w", repositoryPath, generateError)
	}

	changelogPath := filepath.Join(repositoryPath, webAuditChangelogFileNameConstant)
	currentContents, readError := fileSystem.ReadFile(changelogPath)
	if readError != nil && !errors.Is(readError, os.ErrNotExist) {
		return fmt.Errorf("audit.changelog.read %s: %w", changelogPath, readError)
	}

	nextContents, contentsError := upsertWebChangelogSection(string(currentContents), result.Section)
	if contentsError != nil {
		return fmt.Errorf("audit.changelog.section %s: %w", changelogPath, contentsError)
	}

	if writeError := fileSystem.WriteFile(changelogPath, []byte(nextContents), 0o644); writeError != nil {
		return fmt.Errorf("audit.changelog.write %s: %w", changelogPath, writeError)
	}

	if outputWriter != nil {
		_, _ = fmt.Fprintf(outputWriter, webAuditChangeUpdatedFileTemplateConstant, changelogPath)
		_, _ = fmt.Fprintln(outputWriter, strings.TrimSpace(result.Section))
	}
	return nil
}

func (application *Application) webCommitMessageClient() (llm.ChatClient, commitcmd.MessageConfiguration, error) {
	configuration := application.commitMessageConfiguration()
	client, clientError := application.newWebLLMClient(
		configuration.BaseURL,
		configuration.APIKeyEnv,
		configuration.Model,
		configuration.MaxTokens,
		configuration.Temperature,
		configuration.TimeoutSeconds,
	)
	return client, configuration, clientError
}

func (application *Application) webChangelogMessageClient() (llm.ChatClient, changelogcmd.MessageConfiguration, error) {
	configuration := application.changelogMessageConfiguration()
	client, clientError := application.newWebLLMClient(
		configuration.BaseURL,
		configuration.APIKeyEnv,
		configuration.Model,
		configuration.MaxTokens,
		configuration.Temperature,
		configuration.TimeoutSeconds,
	)
	return client, configuration, clientError
}

func (application *Application) newWebLLMClient(
	baseURL string,
	apiKeyEnv string,
	modelIdentifier string,
	maxTokens int,
	temperature float64,
	timeoutSeconds int,
) (llm.ChatClient, error) {
	trimmedAPIKeyEnv := strings.TrimSpace(apiKeyEnv)
	if trimmedAPIKeyEnv == "" {
		return nil, errors.New("llm api key environment is required")
	}

	apiKeyValue := strings.TrimSpace(os.Getenv(trimmedAPIKeyEnv))
	if apiKeyValue == "" {
		return nil, fmt.Errorf("environment variable %s must be set with an API key", trimmedAPIKeyEnv)
	}

	clientFactory := application.llmClientFactory
	if clientFactory == nil {
		clientFactory = func(configuration llm.Config) (llm.ChatClient, error) {
			return llm.NewFactory(configuration)
		}
	}

	clientConfiguration := llm.Config{
		BaseURL:             strings.TrimSpace(baseURL),
		APIKey:              apiKeyValue,
		Model:               strings.TrimSpace(modelIdentifier),
		MaxCompletionTokens: maxTokens,
		RequestTimeout:      time.Duration(timeoutSeconds) * time.Second,
	}
	if temperature > 0 {
		clientConfiguration.Temperature = temperature
	}

	return clientFactory(clientConfiguration)
}

func webOptionalTemperature(temperature float64) *float64 {
	if temperature == 0 {
		return nil
	}
	return &temperature
}

func writeWebExecutionOutput(outputWriter io.Writer, result execshell.ExecutionResult) {
	if outputWriter == nil {
		return
	}
	if strings.TrimSpace(result.StandardOutput) != "" {
		_, _ = fmt.Fprintln(outputWriter, strings.TrimSpace(result.StandardOutput))
	}
}

func validateWebAuditChangelogEligibility(
	executionContext context.Context,
	gitExecutor shared.GitExecutor,
	repositoryPath string,
) error {
	defaultBranch, defaultBranchError := webAuditDefaultBranch(executionContext, gitExecutor, repositoryPath)
	if defaultBranchError != nil {
		return defaultBranchError
	}

	currentBranch, currentBranchError := webAuditCurrentBranch(executionContext, gitExecutor, repositoryPath)
	if currentBranchError != nil {
		return currentBranchError
	}

	if currentBranch == "" || defaultBranch == "" || currentBranch != defaultBranch {
		return errors.New(webAuditChangeChangelogBranchRejected)
	}

	headTagged, headTaggedError := webAuditHeadTagged(executionContext, gitExecutor, repositoryPath)
	if headTaggedError != nil {
		return headTaggedError
	}
	if headTagged {
		return errors.New(webAuditChangeChangelogTaggedRejected)
	}

	return nil
}

func webAuditDefaultBranch(
	executionContext context.Context,
	gitExecutor shared.GitExecutor,
	repositoryPath string,
) (string, error) {
	remoteHeadResult, remoteHeadError := gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitSymbolicRefSubcommandConstant, gitSymbolicRefQuietArgumentConstant, gitShortArgumentConstant, gitOriginHeadReferenceConstant},
		WorkingDirectory: repositoryPath,
	})
	if remoteHeadError == nil {
		remoteReference := strings.TrimSpace(remoteHeadResult.StandardOutput)
		remoteReference = strings.TrimPrefix(remoteReference, gitOriginPrefixConstant)
		if remoteReference != "" {
			return remoteReference, nil
		}
	}

	localHeadResult, localHeadError := gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitSymbolicRefSubcommandConstant, gitSymbolicRefQuietArgumentConstant, gitShortArgumentConstant, gitHeadReferenceConstant},
		WorkingDirectory: repositoryPath,
	})
	if localHeadError != nil {
		return "", localHeadError
	}

	return strings.TrimSpace(localHeadResult.StandardOutput), nil
}

func webAuditCurrentBranch(
	executionContext context.Context,
	gitExecutor shared.GitExecutor,
	repositoryPath string,
) (string, error) {
	branchResult, branchError := gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{gitSymbolicRefSubcommandConstant, gitSymbolicRefQuietArgumentConstant, gitShortArgumentConstant, gitHeadReferenceConstant},
		WorkingDirectory: repositoryPath,
	})
	if branchError != nil {
		return "", branchError
	}
	return strings.TrimSpace(branchResult.StandardOutput), nil
}

func webAuditHeadTagged(
	executionContext context.Context,
	gitExecutor shared.GitExecutor,
	repositoryPath string,
) (bool, error) {
	tagResult, tagError := gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        []string{webAuditGitTagSubcommandConstant, webAuditGitPointsAtArgumentConstant, gitHeadReferenceConstant},
		WorkingDirectory: repositoryPath,
	})
	if tagError != nil {
		return false, tagError
	}
	return strings.TrimSpace(tagResult.StandardOutput) != "", nil
}

func detectWebReleaseVersion(
	executionContext context.Context,
	gitExecutor shared.GitExecutor,
	fileSystem shared.FileSystem,
	repositoryPath string,
) (string, error) {
	changelogCandidate := ""
	changelogPath := filepath.Join(repositoryPath, webAuditChangelogFileNameConstant)
	if changelogContents, readError := fileSystem.ReadFile(changelogPath); readError == nil {
		changelogCandidate = findWebChangelogVersionCandidate(string(changelogContents))
	} else if !errors.Is(readError, os.ErrNotExist) {
		return "", readError
	}

	describeResult, describeError := gitExecutor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments: []string{
			webAuditGitDescribeSubcommandConstant,
			webAuditGitDescribeTagsArgumentConstant,
			webAuditGitDescribeAbbrevArgumentConstant,
		},
		WorkingDirectory: repositoryPath,
	})
	if describeError == nil {
		tagCandidate := strings.TrimSpace(describeResult.StandardOutput)
		if nextVersion, ok := nextWebReleaseVersion(tagCandidate); ok {
			if strings.TrimSpace(changelogCandidate) == nextVersion {
				return changelogCandidate, nil
			}
			return nextVersion, nil
		}
	}

	if nextVersion, ok := nextWebReleaseVersion(changelogCandidate); ok {
		return nextVersion, nil
	}

	return webAuditDefaultReleaseVersionConstant, nil
}

func findWebChangelogVersionCandidate(contents string) string {
	for _, line := range strings.Split(strings.ReplaceAll(contents, "\r\n", "\n"), "\n") {
		trimmedLine := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmedLine, "## [") || !strings.HasSuffix(trimmedLine, "]") {
			continue
		}
		return strings.TrimSuffix(strings.TrimPrefix(trimmedLine, "## ["), "]")
	}
	return ""
}

func nextWebReleaseVersion(candidate string) (string, bool) {
	matches := webReleaseVersionPattern.FindStringSubmatch(strings.TrimSpace(candidate))
	if len(matches) == 0 {
		return "", false
	}

	majorValue, majorError := strconv.Atoi(matches[2])
	if majorError != nil {
		return "", false
	}
	minorValue, minorError := strconv.Atoi(matches[3])
	if minorError != nil {
		return "", false
	}
	patchValue, patchError := strconv.Atoi(matches[4])
	if patchError != nil {
		return "", false
	}

	prefixValue := matches[1]
	if prefixValue == "" {
		prefixValue = "v"
	}

	if matches[5] != "" {
		rcValue, rcError := strconv.Atoi(matches[5])
		if rcError != nil {
			return "", false
		}
		return fmt.Sprintf("%s%d.%d.%d-rc%d", prefixValue, majorValue, minorValue, patchValue, rcValue+1), true
	}

	return fmt.Sprintf("%s%d.%d.%d", prefixValue, majorValue, minorValue, patchValue+1), true
}

func upsertWebChangelogSection(existingContents string, section string) (string, error) {
	normalizedSection := strings.TrimSpace(strings.ReplaceAll(section, "\r\n", "\n"))
	if normalizedSection == "" {
		return "", errors.New("changelog section is required")
	}

	sectionLines := strings.Split(normalizedSection, "\n")
	headingLine := strings.TrimSpace(sectionLines[0])
	if !strings.HasPrefix(headingLine, "## ") {
		return "", errors.New("changelog section must start with a level-two heading")
	}

	normalizedExisting := strings.ReplaceAll(existingContents, "\r\n", "\n")
	if strings.TrimSpace(normalizedExisting) == "" {
		return "# Changelog\n\n" + normalizedSection + "\n", nil
	}

	sectionBlock := normalizedSection + "\n"
	headingExpression := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(headingLine) + `$`)
	if existingHeadingIndex := headingExpression.FindStringIndex(normalizedExisting); existingHeadingIndex != nil {
		nextHeadingExpression := regexp.MustCompile(`(?m)^## `)
		nextHeadingIndex := nextHeadingExpression.FindStringIndex(normalizedExisting[existingHeadingIndex[1]:])
		sectionEndIndex := len(normalizedExisting)
		if nextHeadingIndex != nil {
			sectionEndIndex = existingHeadingIndex[1] + nextHeadingIndex[0]
		}

		prefixValue := strings.TrimRight(normalizedExisting[:existingHeadingIndex[0]], "\n")
		suffixValue := strings.TrimLeft(normalizedExisting[sectionEndIndex:], "\n")
		rebuilt := prefixValue + "\n\n" + sectionBlock
		if suffixValue != "" {
			rebuilt += "\n" + suffixValue
		}
		return strings.TrimRight(rebuilt, "\n") + "\n", nil
	}

	if strings.HasPrefix(normalizedExisting, "# ") {
		lines := strings.Split(normalizedExisting, "\n")
		insertIndex := 1
		for insertIndex < len(lines) && strings.TrimSpace(lines[insertIndex]) == "" {
			insertIndex++
		}
		prefixLines := append([]string(nil), lines[:insertIndex]...)
		suffixLines := lines[insertIndex:]
		rebuiltLines := append(prefixLines, "")
		rebuiltLines = append(rebuiltLines, strings.Split(normalizedSection, "\n")...)
		if len(suffixLines) > 0 {
			rebuiltLines = append(rebuiltLines, "")
			rebuiltLines = append(rebuiltLines, suffixLines...)
		}
		return strings.TrimRight(strings.Join(rebuiltLines, "\n"), "\n") + "\n", nil
	}

	return normalizedSection + "\n\n" + strings.TrimLeft(normalizedExisting, "\n"), nil
}

func executeWebAuditWorkflowChange(
	executionContext context.Context,
	dependencies workflow.Dependencies,
	repositoryPath string,
	change web.AuditQueuedChange,
) (workflow.ExecutionOutcome, error) {
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
			return workflow.ExecutionOutcome{}, errors.New(webAuditChangeProtocolMissingConstant)
		}
		targetProtocol, targetError := shared.ParseRemoteProtocol(change.TargetProtocol)
		if targetError != nil {
			return workflow.ExecutionOutcome{}, errors.New(webAuditChangeProtocolMissingConstant)
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
			return workflow.ExecutionOutcome{}, taskDefinitionError
		}
		return executeWebAuditTasks(
			executionContext,
			dependencies,
			repositoryPath,
			[]workflow.TaskDefinition{taskDefinition},
		)
	default:
		return workflow.ExecutionOutcome{}, fmt.Errorf(webAuditChangeKindTemplateConstant, change.Kind)
	}
}

func executeWebAuditOperation(
	executionContext context.Context,
	dependencies workflow.Dependencies,
	repositoryPath string,
	operation workflow.Operation,
) (workflow.ExecutionOutcome, error) {
	executor := workflow.NewExecutor([]workflow.Operation{operation}, dependencies)
	return executor.Execute(
		executionContext,
		[]string{repositoryPath},
		workflow.RuntimeOptions{
			AssumeYes:           true,
			WorkflowParallelism: 1,
		},
	)
}

func executeWebAuditTasks(
	executionContext context.Context,
	dependencies workflow.Dependencies,
	repositoryPath string,
	definitions []workflow.TaskDefinition,
) (workflow.ExecutionOutcome, error) {
	runner := workflow.NewTaskRunner(dependencies)
	return runner.Run(
		executionContext,
		[]string{repositoryPath},
		definitions,
		workflow.RuntimeOptions{
			AssumeYes:           true,
			WorkflowParallelism: 1,
		},
	)
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
	case web.AuditChangeKindUpdateChangelog:
		return "Changelog updated"
	case web.AuditChangeKindCommitChanges:
		return "Changes committed"
	case web.AuditChangeKindDeleteFolder:
		return "Folder deleted"
	default:
		return ""
	}
}

func webAuditChangeSkippedMessage(kind web.AuditChangeKind) string {
	switch kind {
	case web.AuditChangeKindRenameFolder:
		return "Rename folder skipped"
	case web.AuditChangeKindUpdateCanonical:
		return "Canonical remote update skipped"
	case web.AuditChangeKindConvertProtocol:
		return "Remote protocol update skipped"
	case web.AuditChangeKindSyncWithRemote:
		return "Repository synchronization skipped"
	case web.AuditChangeKindUpdateChangelog:
		return "Changelog update skipped"
	case web.AuditChangeKindCommitChanges:
		return "Commit skipped"
	case web.AuditChangeKindDeleteFolder:
		return "Folder deletion skipped"
	default:
		return ""
	}
}

func webAuditChangeResultStatus(executionOutcome workflow.ExecutionOutcome, executionError error) string {
	if executionError != nil {
		return webAuditChangeStatusFailedConstant
	}
	if webAuditExecutionOutcomeContainsStepOutcome(executionOutcome, webAuditChangeStatusSkippedConstant) {
		return webAuditChangeStatusSkippedConstant
	}
	return webAuditChangeStatusSucceededConstant
}

func webAuditExecutionOutcomeContainsStepOutcome(executionOutcome workflow.ExecutionOutcome, outcomeLabel string) bool {
	if len(outcomeLabel) == 0 {
		return false
	}

	for _, stepOutcomeCounts := range executionOutcome.ReporterSummaryData.StepOutcomeCounts {
		if stepOutcomeCounts[outcomeLabel] > 0 {
			return true
		}
	}

	return false
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
	case web.AuditChangeKindUpdateChangelog:
		return 35
	case web.AuditChangeKindCommitChanges:
		return 36
	case web.AuditChangeKindDeleteFolder:
		return 50
	case web.AuditChangeKindRenameFolder:
		return 60
	default:
		return 100
	}
}

func normalizeWebAuditChangePath(rawPath string) (string, error) {
	return normalizeWebAbsolutePath(rawPath, webAuditChangePathRequiredConstant, webAuditChangePathAbsoluteRequiredConstant)
}

func normalizeWebAbsolutePath(rawPath string, missingPathError string, relativePathError string) (string, error) {
	trimmedPath := strings.TrimSpace(rawPath)
	if len(trimmedPath) == 0 {
		return "", errors.New(missingPathError)
	}
	cleanPath := filepath.Clean(trimmedPath)
	if !filepath.IsAbs(cleanPath) {
		return "", errors.New(relativePathError)
	}
	return cleanPath, nil
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
	return application.inspectRepositoryDescriptor(
		executionContext,
		repositoryPath,
		fmt.Sprintf(webRepositoryIDTemplateConstant, repositoryIndex),
		contextCurrent,
		gitExecutor,
		repositoryManager,
	)
}

func (application *Application) inspectDynamicRepository(
	executionContext context.Context,
	repositoryPath string,
	gitExecutor execshellGitExecutor,
	repositoryManager webGitRepositoryManager,
) web.RepositoryDescriptor {
	return application.inspectRepositoryDescriptor(
		executionContext,
		repositoryPath,
		dynamicWebRepositoryID(repositoryPath),
		false,
		gitExecutor,
		repositoryManager,
	)
}

func (application *Application) inspectRepositoryDescriptor(
	executionContext context.Context,
	repositoryPath string,
	repositoryID string,
	contextCurrent bool,
	gitExecutor execshellGitExecutor,
	repositoryManager webGitRepositoryManager,
) web.RepositoryDescriptor {
	descriptor := newWebRepositoryDescriptor(repositoryID, repositoryPath, contextCurrent)

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

func newDynamicWebRepositoryDescriptor(repositoryPath string) web.RepositoryDescriptor {
	return newWebRepositoryDescriptor(dynamicWebRepositoryID(repositoryPath), repositoryPath, false)
}

func newWebRepositoryDescriptor(repositoryID string, repositoryPath string, contextCurrent bool) web.RepositoryDescriptor {
	trimmedRepositoryPath := strings.TrimSpace(repositoryPath)
	return web.RepositoryDescriptor{
		ID:             strings.TrimSpace(repositoryID),
		Name:           filepath.Base(trimmedRepositoryPath),
		Path:           trimmedRepositoryPath,
		ContextCurrent: contextCurrent,
	}
}

func dynamicWebRepositoryID(repositoryPath string) string {
	normalizedRepositoryPath := canonicalWebPath(repositoryPath)
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(normalizedRepositoryPath))
	return fmt.Sprintf(webDynamicRepositoryIDTemplateConstant, hasher.Sum64())
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
