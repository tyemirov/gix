package workflow

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/temirov/gix/internal/audit"
	"github.com/temirov/gix/internal/releases"
	"github.com/temirov/gix/internal/repos/history"
	"github.com/temirov/gix/internal/repos/shared"
)

const (
	taskActionCanonicalRemote    = "repo.remote.update"
	taskActionProtocolConversion = "repo.remote.convert-protocol"
	taskActionRenameDirectories  = "repo.folder.rename"
	taskActionBranchDefault      = "branch.default"
	taskActionReleaseTag         = "repo.release.tag"
	taskActionAuditReport        = "audit.report"
	taskActionHistoryPurge       = "repo.history.purge"
	taskActionFileReplace        = "repo.files.replace"
	taskActionNamespaceRewrite   = "repo.namespace.rewrite"

	releaseActionMessageTemplate = "RELEASED: %s -> %s"
)

var taskActionHandlers = map[string]taskActionHandlerFunc{
	taskActionCanonicalRemote:    handleCanonicalRemoteAction,
	taskActionProtocolConversion: handleProtocolConversionAction,
	taskActionRenameDirectories:  handleRenameDirectoriesAction,
	taskActionBranchDefault:      handleBranchDefaultAction,
	taskActionReleaseTag:         handleReleaseTagAction,
	taskActionAuditReport:        handleAuditReportAction,
	taskActionHistoryPurge:       handleHistoryPurgeAction,
	taskActionFileReplace:        handleFileReplaceAction,
	taskActionNamespaceRewrite:   handleNamespaceRewriteAction,
}

type taskActionHandlerFunc func(ctx context.Context, environment *Environment, repository *RepositoryState, parameters map[string]any) error

type taskActionExecutor struct {
	environment *Environment
	handlers    map[string]taskActionHandlerFunc
}

func newTaskActionExecutor(environment *Environment) taskActionExecutor {
	handlers := make(map[string]taskActionHandlerFunc, len(taskActionHandlers))
	for actionType, handler := range taskActionHandlers {
		handlers[actionType] = handler
	}
	return taskActionExecutor{environment: environment, handlers: handlers}
}

func (executor taskActionExecutor) execute(ctx context.Context, repository *RepositoryState, action taskAction) error {
	if executor.environment == nil || repository == nil {
		return nil
	}

	normalizedType := strings.ToLower(strings.TrimSpace(action.actionType))
	if len(normalizedType) == 0 {
		return nil
	}

	handler, exists := executor.handlers[normalizedType]
	if !exists {
		return fmt.Errorf("unsupported task action %s", action.actionType)
	}

	return handler(ctx, executor.environment, repository, action.parameters)
}

// RegisterTaskAction adds a handler for a custom task action type.
func RegisterTaskAction(actionType string, handler taskActionHandlerFunc) {
	normalized := strings.ToLower(strings.TrimSpace(actionType))
	if len(normalized) == 0 || handler == nil {
		return
	}
	taskActionHandlers[normalized] = handler
}

func handleCanonicalRemoteAction(ctx context.Context, environment *Environment, repository *RepositoryState, parameters map[string]any) error {
	reader := newOptionReader(parameters)
	ownerConstraint, _, ownerError := reader.stringValue("owner")
	if ownerError != nil {
		return ownerError
	}

	operation := &CanonicalRemoteOperation{OwnerConstraint: strings.TrimSpace(ownerConstraint)}
	state := &State{Repositories: []*RepositoryState{repository}}
	return operation.Execute(ctx, environment, state)
}

func handleProtocolConversionAction(ctx context.Context, environment *Environment, repository *RepositoryState, parameters map[string]any) error {
	reader := newOptionReader(parameters)

	targetValue, targetExists, targetError := reader.stringValue("to")
	if targetError != nil {
		return targetError
	}
	if !targetExists || len(targetValue) == 0 {
		return errors.New("protocol conversion action requires 'to'")
	}

	targetProtocol, parseTargetError := parseProtocolValue(targetValue)
	if parseTargetError != nil {
		return parseTargetError
	}

	fromProtocol := shared.RemoteProtocol(strings.TrimSpace(string(repository.Inspection.RemoteProtocol)))
	sourceValue, sourceExists, sourceError := reader.stringValue("from")
	if sourceError != nil {
		return sourceError
	}
	if sourceExists && len(sourceValue) > 0 {
		parsedSource, parseSourceError := parseProtocolValue(sourceValue)
		if parseSourceError != nil {
			return parseSourceError
		}
		fromProtocol = parsedSource
	}

	operation := &ProtocolConversionOperation{FromProtocol: fromProtocol, ToProtocol: targetProtocol}
	state := &State{Repositories: []*RepositoryState{repository}}
	return operation.Execute(ctx, environment, state)
}

func handleRenameDirectoriesAction(ctx context.Context, environment *Environment, repository *RepositoryState, parameters map[string]any) error {
	reader := newOptionReader(parameters)

	requireClean := true
	requireCleanExplicit := false
	if value, exists, err := reader.boolValue("require_clean"); err != nil {
		return err
	} else if exists {
		requireClean = value
		requireCleanExplicit = true
	}

	includeOwner := false
	if value, exists, err := reader.boolValue("include_owner"); err != nil {
		return err
	} else if exists {
		includeOwner = value
	}

	if requireClean && repository != nil && repository.HasNestedRepositories && repository.InitialCleanWorktree {
		requireClean = false
	}

	operation := &RenameOperation{RequireCleanWorktree: requireClean, IncludeOwner: includeOwner, requireCleanExplicit: requireCleanExplicit}
	state := &State{Repositories: []*RepositoryState{repository}}
	return operation.Execute(ctx, environment, state)
}

func handleBranchDefaultAction(ctx context.Context, environment *Environment, repository *RepositoryState, parameters map[string]any) error {
	reader := newOptionReader(parameters)

	targetBranchValue, _, targetBranchError := reader.stringValue("target")
	if targetBranchError != nil {
		return targetBranchError
	}

	sourceBranchValue, _, sourceBranchError := reader.stringValue("source")
	if sourceBranchError != nil {
		return sourceBranchError
	}

	remoteNameValue, remoteNameExists, remoteNameError := reader.stringValue("remote")
	if remoteNameError != nil {
		return remoteNameError
	}
	remoteName := defaultMigrationRemoteNameConstant
	if remoteNameExists && len(remoteNameValue) > 0 {
		remoteName = remoteNameValue
	}

	pushToRemote := true
	if value, exists, err := reader.boolValue("push"); err != nil {
		return err
	} else if exists {
		pushToRemote = value
	}

	deleteSource := false
	if value, exists, err := reader.boolValue("delete_source_branch"); err != nil {
		return err
	} else if exists {
		deleteSource = value
	}

	target := BranchMigrationTarget{
		RemoteName:         remoteName,
		SourceBranch:       sourceBranchValue,
		TargetBranch:       targetBranchValue,
		PushToRemote:       pushToRemote,
		DeleteSourceBranch: deleteSource,
	}

	operation := &BranchMigrationOperation{Targets: []BranchMigrationTarget{target}}
	state := &State{Repositories: []*RepositoryState{repository}}
	return operation.Execute(ctx, environment, state)
}

func handleReleaseTagAction(ctx context.Context, environment *Environment, repository *RepositoryState, parameters map[string]any) error {
	if environment == nil || repository == nil {
		return nil
	}

	reader := newOptionReader(parameters)

	tagValue, tagExists, tagError := reader.stringValue("tag")
	if tagError != nil {
		return tagError
	}
	if !tagExists || len(tagValue) == 0 {
		return errors.New("release action requires 'tag'")
	}

	messageValue, _, messageError := reader.stringValue("message")
	if messageError != nil {
		return messageError
	}

	remoteValue, _, remoteError := reader.stringValue("remote")
	if remoteError != nil {
		return remoteError
	}

	service, serviceError := releases.NewService(releases.ServiceDependencies{GitExecutor: environment.GitExecutor})
	if serviceError != nil {
		return serviceError
	}

	result, releaseError := service.Release(ctx, releases.Options{
		RepositoryPath: repository.Path,
		TagName:        tagValue,
		Message:        messageValue,
		RemoteName:     remoteValue,
		DryRun:         environment.DryRun,
	})
	if releaseError != nil {
		return releaseError
	}

	if environment.Output != nil {
		fmt.Fprintf(environment.Output, releaseActionMessageTemplate+"\n", result.RepositoryPath, result.TagName)
	}

	return nil
}

func handleAuditReportAction(ctx context.Context, environment *Environment, repository *RepositoryState, parameters map[string]any) error {
	if environment == nil || environment.AuditService == nil {
		return nil
	}

	if environment.auditReportExecuted {
		return nil
	}

	reader := newOptionReader(parameters)
	includeAll, _, includeAllError := reader.boolValue("include_all")
	if includeAllError != nil {
		return includeAllError
	}
	debugOutput, _, debugError := reader.boolValue("debug")
	if debugError != nil {
		return debugError
	}

	depthValue, _, depthError := reader.stringValue("depth")
	if depthError != nil {
		return depthError
	}
	depth := audit.InspectionDepthFull
	if strings.EqualFold(strings.TrimSpace(depthValue), string(audit.InspectionDepthMinimal)) {
		depth = audit.InspectionDepthMinimal
	}

	roots := collectAuditRoots(environment.State, repository)
	if len(roots) == 0 {
		environment.auditReportExecuted = true
		return nil
	}

	outputValue, outputExists, outputError := reader.stringValue("output")
	if outputError != nil {
		return outputError
	}
	sanitizedOutput := strings.TrimSpace(outputValue)
	writeToFile := outputExists && len(sanitizedOutput) > 0

	if environment.DryRun {
		target := auditReportDestinationStdoutConstant
		if writeToFile {
			target = sanitizedOutput
		}
		if environment.Output != nil {
			fmt.Fprintf(environment.Output, auditPlanMessageTemplateConstant, target)
		}
		environment.auditReportExecuted = true
		return nil
	}

	if writeToFile {
		inspections, discoveryError := environment.AuditService.DiscoverInspections(ctx, roots, includeAll, debugOutput, depth)
		if discoveryError != nil {
			environment.auditReportExecuted = true
			return discoveryError
		}

		if writeError := writeAuditReportFile(sanitizedOutput, inspections); writeError != nil {
			environment.auditReportExecuted = true
			return writeError
		}

		if environment.Output != nil {
			fmt.Fprintf(environment.Output, auditWriteMessageTemplateConstant, sanitizedOutput)
		}
		environment.auditReportExecuted = true
		return nil
	}

	commandOptions := audit.CommandOptions{
		Roots:             roots,
		DebugOutput:       debugOutput,
		IncludeAllFolders: includeAll,
		InspectionDepth:   depth,
	}

	if runError := environment.AuditService.Run(ctx, commandOptions); runError != nil {
		environment.auditReportExecuted = true
		return runError
	}

	environment.auditReportExecuted = true
	return nil
}

func collectAuditRoots(state *State, repository *RepositoryState) []string {
	seen := make(map[string]struct{})
	roots := []string{}
	appendRoot := func(path string) {
		sanitized := strings.TrimSpace(path)
		if len(sanitized) == 0 {
			return
		}
		if _, exists := seen[sanitized]; exists {
			return
		}
		seen[sanitized] = struct{}{}
		roots = append(roots, sanitized)
	}

	if state != nil {
		repositoryPaths := make([]string, 0, len(state.Repositories))
		for _, repositoryState := range state.Repositories {
			if repositoryState == nil {
				continue
			}
			sanitizedRepositoryPath := strings.TrimSpace(repositoryState.Path)
			if len(sanitizedRepositoryPath) == 0 {
				continue
			}
			repositoryPaths = append(repositoryPaths, sanitizedRepositoryPath)
			appendRoot(sanitizedRepositoryPath)
		}

		for _, root := range state.Roots {
			sanitizedRoot := strings.TrimSpace(root)
			if len(sanitizedRoot) == 0 {
				continue
			}
			if len(repositoryPaths) == 0 {
				appendRoot(sanitizedRoot)
				continue
			}

			for _, repositoryPath := range repositoryPaths {
				relative, relativeError := filepath.Rel(sanitizedRoot, repositoryPath)
				if relativeError != nil {
					continue
				}
				if relative == "." || (len(relative) > 0 && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != "..") {
					appendRoot(sanitizedRoot)
					break
				}
			}
		}
	}

	if len(roots) == 0 && repository != nil {
		appendRoot(repository.Path)
	}

	return roots
}

func handleHistoryPurgeAction(ctx context.Context, environment *Environment, repository *RepositoryState, parameters map[string]any) error {
	if environment == nil || repository == nil {
		return nil
	}

	rawPaths, exists := parameters["paths"]
	if !exists {
		return errors.New("history purge action requires 'paths'")
	}
	paths, pathsError := readHistoryPaths(rawPaths)
	if pathsError != nil {
		return pathsError
	}
	if len(paths) == 0 {
		return errors.New("history purge action requires at least one path")
	}

	reader := newOptionReader(parameters)

	remoteName, _, remoteError := reader.stringValue("remote")
	if remoteError != nil {
		return remoteError
	}

	pushEnabled := true
	if value, exists, err := reader.boolValue("push"); err != nil {
		return err
	} else if exists {
		pushEnabled = value
	}

	restoreEnabled := true
	if value, exists, err := reader.boolValue("restore"); err != nil {
		return err
	} else if exists {
		restoreEnabled = value
	}

	pushMissing := false
	if value, exists, err := reader.boolValue("push_missing"); err != nil {
		return err
	} else if exists {
		pushMissing = value
	}

	repositoryPath, repositoryPathError := shared.NewRepositoryPath(repository.Path)
	if repositoryPathError != nil {
		return fmt.Errorf("history purge action: %w", repositoryPathError)
	}

	var remoteNameValue *shared.RemoteName
	if trimmedRemote := strings.TrimSpace(remoteName); len(trimmedRemote) > 0 {
		parsedRemoteName, remoteNameError := shared.NewRemoteName(trimmedRemote)
		if remoteNameError != nil {
			return fmt.Errorf("history purge action: %w", remoteNameError)
		}
		remoteNameValue = &parsedRemoteName
	}

	executor := history.NewExecutor(history.Dependencies{
		GitExecutor:       environment.GitExecutor,
		RepositoryManager: environment.RepositoryManager,
		FileSystem:        environment.FileSystem,
		Output:            environment.Output,
	})

	options := history.Options{
		RepositoryPath: repositoryPath,
		Paths:          paths,
		RemoteName:     remoteNameValue,
		Push:           pushEnabled,
		Restore:        restoreEnabled,
		PushMissing:    pushMissing,
		DryRun:         environment.DryRun,
	}

	return executor.Execute(ctx, options)
}

func readHistoryPaths(raw any) ([]string, error) {
	switch typed := raw.(type) {
	case []string:
		return append([]string{}, typed...), nil
	case []any:
		paths := make([]string, 0, len(typed))
		for index := range typed {
			value, ok := typed[index].(string)
			if !ok {
				return nil, fmt.Errorf("history purge action paths must be strings")
			}
			trimmed := strings.TrimSpace(value)
			if len(trimmed) == 0 {
				continue
			}
			paths = append(paths, trimmed)
		}
		return paths, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if len(trimmed) == 0 {
			return []string{}, nil
		}
		return []string{trimmed}, nil
	default:
		return nil, fmt.Errorf("history purge action requires 'paths' to be a string or list of strings")
	}
}

func writeAuditReportFile(destination string, inspections []audit.RepositoryInspection) error {
	if len(strings.TrimSpace(destination)) == 0 {
		return errors.New("audit report destination missing")
	}

	targetDirectory := filepath.Dir(destination)
	if targetDirectory != auditCurrentDirectorySentinelConstant {
		if mkdirError := os.MkdirAll(targetDirectory, auditDirectoryPermissionsConstant); mkdirError != nil {
			return mkdirError
		}
	}

	fileHandle, createError := os.Create(destination)
	if createError != nil {
		return createError
	}
	defer fileHandle.Close()

	writer := csv.NewWriter(fileHandle)
	header := []string{
		auditCSVHeaderFolderNameConstant,
		auditCSVHeaderFinalRepositoryConstant,
		auditCSVHeaderNameMatchesConstant,
		auditCSVHeaderRemoteDefaultConstant,
		auditCSVHeaderLocalBranchConstant,
		auditCSVHeaderInSyncConstant,
		auditCSVHeaderRemoteProtocolConstant,
		auditCSVHeaderOriginCanonicalConstant,
	}

	if writeError := writer.Write(header); writeError != nil {
		return writeError
	}

	for inspectionIndex := range inspections {
		row := buildAuditReportRow(inspections[inspectionIndex])
		if writeError := writer.Write(row); writeError != nil {
			return writeError
		}
	}

	writer.Flush()
	return writer.Error()
}
