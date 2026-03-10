package cli

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/web"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	webWorkflowPrimitiveCanonicalRemoteConstant    = "repo.remote.update"
	webWorkflowPrimitiveProtocolConversionConstant = "repo.remote.convert-protocol"
	webWorkflowPrimitiveRenameFolderConstant       = "repo.folder.rename"
	webWorkflowPrimitiveDefaultBranchConstant      = "branch.default"
	webWorkflowPrimitiveReleaseTagConstant         = "repo.release.tag"
	webWorkflowPrimitiveReleaseRetagConstant       = "repo.release.retag"
	webWorkflowPrimitiveAuditReportConstant        = "audit.report"
	webWorkflowPrimitiveHistoryPurgeConstant       = "repo.history.purge"
	webWorkflowPrimitiveFileReplaceConstant        = "repo.files.replace"
	webWorkflowPrimitiveNamespaceRewriteConstant   = "repo.namespace.rewrite"

	webWorkflowPrimitiveQueuedActionsRequiredConstant       = "at least one queued workflow action is required"
	webWorkflowPrimitiveRepositoryPathRequiredConstant      = "workflow action repository path is required"
	webWorkflowPrimitiveRepositoryPathAbsoluteRequiredConst = "workflow action repository path must be absolute"
	webWorkflowPrimitiveIdentifierRequiredConstant          = "workflow primitive id is required"
	webWorkflowPrimitiveUnsupportedTemplateConstant         = "unsupported workflow primitive %q"
	webWorkflowPrimitiveParameterRequiredTemplateConstant   = "workflow primitive %s requires %q"
	webWorkflowPrimitiveParameterTypeTemplateConstant       = "workflow primitive %s requires %q to be %s"
	webWorkflowPrimitiveParameterValueTemplateConstant      = "workflow primitive %s does not allow %q for %q"
	webWorkflowPrimitiveRetagMappingsFormatConstant         = "workflow primitive repo.release.retag requires CSV lines: tag,target[,message]"
	webWorkflowPrimitiveAppliedTemplateConstant             = "Applied %s"
	webWorkflowPrimitiveSkippedTemplateConstant             = "Skipped %s"

	webWorkflowPrimitiveParameterTypeStringConstant   = "a string"
	webWorkflowPrimitiveParameterTypeBooleanConstant  = "a boolean"
	webWorkflowPrimitiveProtocolPlaceholderConstant   = "ssh"
	webWorkflowPrimitiveOwnerPlaceholderConstant      = "owner"
	webWorkflowPrimitiveBranchPlaceholderConstant     = "master"
	webWorkflowPrimitiveRemotePlaceholderConstant     = "origin"
	webWorkflowPrimitiveTagPlaceholderConstant        = "v1.2.3"
	webWorkflowPrimitiveOutputPlaceholderConstant     = "./audit-report.csv"
	webWorkflowPrimitivePathsPlaceholderConstant      = "path/one\npath/two"
	webWorkflowPrimitivePatternPlaceholderConstant    = "README.md\n**/*.go"
	webWorkflowPrimitiveFindPlaceholderConstant       = "TEXT_TO_FIND"
	webWorkflowPrimitiveReplacePlaceholderConstant    = "TEXT_TO_REPLACE"
	webWorkflowPrimitiveCommandPlaceholderConstant    = "go test ./..."
	webWorkflowPrimitiveNamespaceOldPlaceholderConst  = "github.com/old/module"
	webWorkflowPrimitiveNamespaceNewPlaceholderConst  = "github.com/new/module"
	webWorkflowPrimitiveBranchPrefixPlaceholderConst  = "rewrite/namespace"
	webWorkflowPrimitiveCommitMessagePlaceholderConst = "Rewrite module namespace"
	webWorkflowPrimitiveMappingsPlaceholderConstant   = "v1.2.3,main,Retag v1.2.3"

	webWorkflowPrimitiveParameterOwnerConstant              = "owner"
	webWorkflowPrimitiveParameterFromConstant               = "from"
	webWorkflowPrimitiveParameterToConstant                 = "to"
	webWorkflowPrimitiveParameterIncludeOwnerConstant       = "include_owner"
	webWorkflowPrimitiveParameterRequireCleanConstant       = "require_clean"
	webWorkflowPrimitiveParameterSourceConstant             = "source"
	webWorkflowPrimitiveParameterTargetConstant             = "target"
	webWorkflowPrimitiveParameterRemoteConstant             = "remote"
	webWorkflowPrimitiveParameterPushConstant               = "push"
	webWorkflowPrimitiveParameterDeleteSourceBranchConstant = "delete_source_branch"
	webWorkflowPrimitiveParameterTagConstant                = "tag"
	webWorkflowPrimitiveParameterMessageConstant            = "message"
	webWorkflowPrimitiveParameterMappingsConstant           = "mappings"
	webWorkflowPrimitiveParameterIncludeAllConstant         = "include_all"
	webWorkflowPrimitiveParameterDebugConstant              = "debug"
	webWorkflowPrimitiveParameterDepthConstant              = "depth"
	webWorkflowPrimitiveParameterOutputConstant             = "output"
	webWorkflowPrimitiveParameterPathsConstant              = "paths"
	webWorkflowPrimitiveParameterRestoreConstant            = "restore"
	webWorkflowPrimitiveParameterPushMissingConstant        = "push_missing"
	webWorkflowPrimitiveParameterPatternsConstant           = "patterns"
	webWorkflowPrimitiveParameterFindConstant               = "find"
	webWorkflowPrimitiveParameterReplaceConstant            = "replace"
	webWorkflowPrimitiveParameterCommandConstant            = "command"
	webWorkflowPrimitiveParameterOldConstant                = "old"
	webWorkflowPrimitiveParameterNewConstant                = "new"
	webWorkflowPrimitiveParameterBranchPrefixConstant       = "branch_prefix"
	webWorkflowPrimitiveParameterCommitMessageConstant      = "commit_message"

	webWorkflowPrimitiveProtocolSSHConstant   = string(shared.RemoteProtocolSSH)
	webWorkflowPrimitiveProtocolHTTPSConstant = string(shared.RemoteProtocolHTTPS)
	webWorkflowPrimitiveAuditDepthFullConst   = string(audit.InspectionDepthFull)
	webWorkflowPrimitiveAuditDepthMinConst    = string(audit.InspectionDepthMinimal)
)

type webWorkflowPrimitiveDefinition struct {
	descriptor   web.WorkflowPrimitiveDescriptor
	buildOptions func(map[string]any) (map[string]any, error)
}

func (application *Application) newWebWorkflowPrimitiveCatalogLoader() web.WorkflowPrimitiveCatalogLoader {
	definitions := application.webWorkflowPrimitiveDefinitions()
	descriptors := make([]web.WorkflowPrimitiveDescriptor, 0, len(definitions))
	for _, definition := range definitions {
		descriptors = append(descriptors, definition.descriptor)
	}

	return func(context.Context) web.WorkflowPrimitiveCatalog {
		return web.WorkflowPrimitiveCatalog{Primitives: append([]web.WorkflowPrimitiveDescriptor(nil), descriptors...)}
	}
}

func (application *Application) newWebWorkflowPrimitiveExecutor() web.WorkflowPrimitiveExecutor {
	definitions := application.webWorkflowPrimitiveDefinitions()
	definitionIndex := make(map[string]webWorkflowPrimitiveDefinition, len(definitions))
	for _, definition := range definitions {
		definitionIndex[definition.descriptor.ID] = definition
	}

	return func(executionContext context.Context, request web.WorkflowPrimitiveApplyRequest) web.WorkflowPrimitiveApplyResponse {
		if len(request.Actions) == 0 {
			return web.WorkflowPrimitiveApplyResponse{Error: webWorkflowPrimitiveQueuedActionsRequiredConstant}
		}

		results := make([]web.WorkflowPrimitiveApplyResult, 0, len(request.Actions))
		for _, action := range request.Actions {
			results = append(results, application.applyWebWorkflowPrimitive(executionContext, definitionIndex, action))
		}
		return web.WorkflowPrimitiveApplyResponse{Results: results}
	}
}

func (application *Application) applyWebWorkflowPrimitive(
	executionContext context.Context,
	definitionIndex map[string]webWorkflowPrimitiveDefinition,
	action web.WorkflowPrimitiveQueuedAction,
) web.WorkflowPrimitiveApplyResult {
	primitiveID := strings.ToLower(strings.TrimSpace(action.PrimitiveID))
	normalizedPath, pathError := normalizeWebAbsolutePath(
		action.RepositoryPath,
		webWorkflowPrimitiveRepositoryPathRequiredConstant,
		webWorkflowPrimitiveRepositoryPathAbsoluteRequiredConst,
	)
	result := web.WorkflowPrimitiveApplyResult{
		ID:             action.ID,
		RepositoryPath: normalizedPath,
		PrimitiveID:    primitiveID,
	}
	if pathError != nil {
		result.Status = webAuditChangeStatusFailedConstant
		result.Error = pathError.Error()
		return result
	}
	if len(primitiveID) == 0 {
		result.Status = webAuditChangeStatusFailedConstant
		result.Error = webWorkflowPrimitiveIdentifierRequiredConstant
		return result
	}

	definition, exists := definitionIndex[primitiveID]
	if !exists {
		result.Status = webAuditChangeStatusFailedConstant
		result.Error = fmt.Sprintf(webWorkflowPrimitiveUnsupportedTemplateConstant, primitiveID)
		return result
	}

	options, optionsError := definition.buildOptions(action.Parameters)
	if optionsError != nil {
		result.Status = webAuditChangeStatusFailedConstant
		result.Error = optionsError.Error()
		return result
	}

	outputBuffer := &bytes.Buffer{}
	errorBuffer := &bytes.Buffer{}

	dependencies, dependencyError := application.webTaskRunnerDependencies(outputBuffer, errorBuffer)
	if dependencyError != nil {
		result.Status = webAuditChangeStatusFailedConstant
		result.Error = dependencyError.Error()
		return result
	}

	taskDefinition := workflow.TaskDefinition{
		Name:  definition.descriptor.Label,
		Steps: []workflow.TaskExecutionStep{workflow.TaskExecutionStepCustomActions},
		Actions: []workflow.TaskActionDefinition{
			{
				Type:    primitiveID,
				Options: options,
			},
		},
	}

	executionOutcome, executionError := executeWebAuditTasks(
		executionContext,
		dependencies.Workflow,
		normalizedPath,
		[]workflow.TaskDefinition{taskDefinition},
	)
	result.Stdout = outputBuffer.String()
	result.Stderr = errorBuffer.String()
	result.Status = webWorkflowPrimitiveResultStatus(executionOutcome, executionError)
	if result.Status == webAuditChangeStatusFailedConstant {
		result.Error = executionError.Error()
		return result
	}
	if result.Status == webAuditChangeStatusSkippedConstant {
		result.Message = fmt.Sprintf(webWorkflowPrimitiveSkippedTemplateConstant, definition.descriptor.Label)
		return result
	}

	result.Message = fmt.Sprintf(webWorkflowPrimitiveAppliedTemplateConstant, definition.descriptor.Label)
	return result
}

func webWorkflowPrimitiveResultStatus(executionOutcome workflow.ExecutionOutcome, executionError error) string {
	if executionError != nil {
		return webAuditChangeStatusFailedConstant
	}
	if webAuditExecutionOutcomeContainsStepOutcome(executionOutcome, webAuditChangeStatusSkippedConstant) {
		return webAuditChangeStatusSkippedConstant
	}
	return webAuditChangeStatusSucceededConstant
}

func (application *Application) webWorkflowPrimitiveDefinitions() []webWorkflowPrimitiveDefinition {
	return []webWorkflowPrimitiveDefinition{
		{
			descriptor: web.WorkflowPrimitiveDescriptor{
				ID:          webWorkflowPrimitiveCanonicalRemoteConstant,
				Label:       "Fix canonical remote",
				Description: "Update the origin remote to the canonical GitHub repository for the selected repo.",
				Parameters: []web.WorkflowPrimitiveParameterDescriptor{
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterOwnerConstant,
						"Owner constraint",
						"Restrict canonical remotes to one GitHub owner when needed.",
						false,
						"",
						webWorkflowPrimitiveOwnerPlaceholderConstant,
					),
				},
			},
			buildOptions: func(parameters map[string]any) (map[string]any, error) {
				owner, ownerError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveCanonicalRemoteConstant, webWorkflowPrimitiveParameterOwnerConstant, false, "")
				if ownerError != nil {
					return nil, ownerError
				}
				return workflowPrimitiveOptions(map[string]string{
					webWorkflowPrimitiveParameterOwnerConstant: owner,
				}), nil
			},
		},
		{
			descriptor: web.WorkflowPrimitiveDescriptor{
				ID:          webWorkflowPrimitiveProtocolConversionConstant,
				Label:       "Convert remote protocol",
				Description: "Switch the origin remote between SSH and HTTPS for the selected repo.",
				Parameters: []web.WorkflowPrimitiveParameterDescriptor{
					selectWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterFromConstant,
						"From protocol",
						"Leave blank to use the repo's current remote protocol.",
						false,
						"",
						protocolWorkflowPrimitiveOptions(),
					),
					selectWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterToConstant,
						"To protocol",
						"The destination protocol for the origin remote.",
						true,
						webWorkflowPrimitiveProtocolSSHConstant,
						protocolWorkflowPrimitiveOptions(),
					),
				},
			},
			buildOptions: func(parameters map[string]any) (map[string]any, error) {
				fromProtocol, fromError := readWebWorkflowSelectParameter(
					parameters,
					webWorkflowPrimitiveProtocolConversionConstant,
					webWorkflowPrimitiveParameterFromConstant,
					false,
					"",
					allowedProtocolValues(),
				)
				if fromError != nil {
					return nil, fromError
				}
				toProtocol, toError := readWebWorkflowSelectParameter(
					parameters,
					webWorkflowPrimitiveProtocolConversionConstant,
					webWorkflowPrimitiveParameterToConstant,
					true,
					webWorkflowPrimitiveProtocolSSHConstant,
					allowedProtocolValues(),
				)
				if toError != nil {
					return nil, toError
				}
				return workflowPrimitiveOptions(map[string]string{
					webWorkflowPrimitiveParameterFromConstant: fromProtocol,
					webWorkflowPrimitiveParameterToConstant:   toProtocol,
				}), nil
			},
		},
		{
			descriptor: web.WorkflowPrimitiveDescriptor{
				ID:          webWorkflowPrimitiveRenameFolderConstant,
				Label:       "Rename folder",
				Description: "Rename the selected repository folder to match its desired GitHub repository name.",
				Parameters: []web.WorkflowPrimitiveParameterDescriptor{
					checkboxWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterIncludeOwnerConstant,
						"Include owner",
						"Prefix the folder name with the GitHub owner.",
						false,
					),
					checkboxWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterRequireCleanConstant,
						"Require clean worktree",
						"Block the rename when the worktree is dirty.",
						true,
					),
				},
			},
			buildOptions: func(parameters map[string]any) (map[string]any, error) {
				includeOwner, includeOwnerError := readWebWorkflowBooleanParameter(parameters, webWorkflowPrimitiveRenameFolderConstant, webWorkflowPrimitiveParameterIncludeOwnerConstant, false)
				if includeOwnerError != nil {
					return nil, includeOwnerError
				}
				requireClean, requireCleanError := readWebWorkflowBooleanParameter(parameters, webWorkflowPrimitiveRenameFolderConstant, webWorkflowPrimitiveParameterRequireCleanConstant, true)
				if requireCleanError != nil {
					return nil, requireCleanError
				}
				return map[string]any{
					webWorkflowPrimitiveParameterIncludeOwnerConstant: includeOwner,
					webWorkflowPrimitiveParameterRequireCleanConstant: requireClean,
				}, nil
			},
		},
		{
			descriptor: web.WorkflowPrimitiveDescriptor{
				ID:          webWorkflowPrimitiveDefaultBranchConstant,
				Label:       "Promote default branch",
				Description: "Promote a source branch to the target default branch and optionally push or delete the source branch.",
				Parameters: []web.WorkflowPrimitiveParameterDescriptor{
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterSourceConstant,
						"Source branch",
						"Leave blank to detect the current default branch source automatically.",
						false,
						"",
						"main",
					),
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterTargetConstant,
						"Target branch",
						"The branch that should become the default.",
						true,
						webWorkflowPrimitiveBranchPlaceholderConstant,
						webWorkflowPrimitiveBranchPlaceholderConstant,
					),
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterRemoteConstant,
						"Remote name",
						"The remote that receives the promoted default branch.",
						false,
						webWorkflowPrimitiveRemotePlaceholderConstant,
						webWorkflowPrimitiveRemotePlaceholderConstant,
					),
					checkboxWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterPushConstant,
						"Push to remote",
						"Push the promoted default branch to the remote.",
						true,
					),
					checkboxWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterDeleteSourceBranchConstant,
						"Delete source branch",
						"Delete the remote source branch after promotion.",
						false,
					),
				},
			},
			buildOptions: func(parameters map[string]any) (map[string]any, error) {
				sourceBranch, sourceError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveDefaultBranchConstant, webWorkflowPrimitiveParameterSourceConstant, false, "")
				if sourceError != nil {
					return nil, sourceError
				}
				targetBranch, targetError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveDefaultBranchConstant, webWorkflowPrimitiveParameterTargetConstant, true, webWorkflowPrimitiveBranchPlaceholderConstant)
				if targetError != nil {
					return nil, targetError
				}
				remoteName, remoteError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveDefaultBranchConstant, webWorkflowPrimitiveParameterRemoteConstant, false, webWorkflowPrimitiveRemotePlaceholderConstant)
				if remoteError != nil {
					return nil, remoteError
				}
				pushToRemote, pushError := readWebWorkflowBooleanParameter(parameters, webWorkflowPrimitiveDefaultBranchConstant, webWorkflowPrimitiveParameterPushConstant, true)
				if pushError != nil {
					return nil, pushError
				}
				deleteSourceBranch, deleteError := readWebWorkflowBooleanParameter(parameters, webWorkflowPrimitiveDefaultBranchConstant, webWorkflowPrimitiveParameterDeleteSourceBranchConstant, false)
				if deleteError != nil {
					return nil, deleteError
				}
				return map[string]any{
					webWorkflowPrimitiveParameterSourceConstant:             sourceBranch,
					webWorkflowPrimitiveParameterTargetConstant:             targetBranch,
					webWorkflowPrimitiveParameterRemoteConstant:             remoteName,
					webWorkflowPrimitiveParameterPushConstant:               pushToRemote,
					webWorkflowPrimitiveParameterDeleteSourceBranchConstant: deleteSourceBranch,
				}, nil
			},
		},
		{
			descriptor: web.WorkflowPrimitiveDescriptor{
				ID:          webWorkflowPrimitiveReleaseTagConstant,
				Label:       "Create release tag",
				Description: "Create and optionally push one annotated Git tag for the selected repo.",
				Parameters: []web.WorkflowPrimitiveParameterDescriptor{
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterTagConstant,
						"Tag name",
						"The Git tag to create.",
						true,
						"",
						webWorkflowPrimitiveTagPlaceholderConstant,
					),
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterMessageConstant,
						"Tag message",
						"Optional annotation message for the created tag.",
						false,
						"",
						"Release message",
					),
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterRemoteConstant,
						"Remote name",
						"Push the tag to this remote when provided.",
						false,
						"",
						webWorkflowPrimitiveRemotePlaceholderConstant,
					),
				},
			},
			buildOptions: func(parameters map[string]any) (map[string]any, error) {
				tagName, tagError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveReleaseTagConstant, webWorkflowPrimitiveParameterTagConstant, true, "")
				if tagError != nil {
					return nil, tagError
				}
				message, messageError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveReleaseTagConstant, webWorkflowPrimitiveParameterMessageConstant, false, "")
				if messageError != nil {
					return nil, messageError
				}
				remoteName, remoteError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveReleaseTagConstant, webWorkflowPrimitiveParameterRemoteConstant, false, "")
				if remoteError != nil {
					return nil, remoteError
				}
				return workflowPrimitiveOptions(map[string]string{
					webWorkflowPrimitiveParameterTagConstant:     tagName,
					webWorkflowPrimitiveParameterMessageConstant: message,
					webWorkflowPrimitiveParameterRemoteConstant:  remoteName,
				}), nil
			},
		},
		{
			descriptor: web.WorkflowPrimitiveDescriptor{
				ID:          webWorkflowPrimitiveReleaseRetagConstant,
				Label:       "Retag releases",
				Description: "Move existing tags to new targets for the selected repo.",
				Parameters: []web.WorkflowPrimitiveParameterDescriptor{
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterRemoteConstant,
						"Remote name",
						"Push retagged refs to this remote when provided.",
						false,
						"",
						webWorkflowPrimitiveRemotePlaceholderConstant,
					),
					textareaWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterMappingsConstant,
						"Mappings",
						"Use CSV lines in the form tag,target[,message].",
						true,
						"",
						webWorkflowPrimitiveMappingsPlaceholderConstant,
					),
				},
			},
			buildOptions: func(parameters map[string]any) (map[string]any, error) {
				remoteName, remoteError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveReleaseRetagConstant, webWorkflowPrimitiveParameterRemoteConstant, false, "")
				if remoteError != nil {
					return nil, remoteError
				}
				mappings, mappingsError := readWebWorkflowRetagMappingsParameter(parameters)
				if mappingsError != nil {
					return nil, mappingsError
				}
				options := map[string]any{
					webWorkflowPrimitiveParameterMappingsConstant: mappings,
				}
				if len(remoteName) > 0 {
					options[webWorkflowPrimitiveParameterRemoteConstant] = remoteName
				}
				return options, nil
			},
		},
		{
			descriptor: web.WorkflowPrimitiveDescriptor{
				ID:          webWorkflowPrimitiveAuditReportConstant,
				Label:       "Audit report",
				Description: "Run the audit report action against the selected repository scope.",
				Parameters: []web.WorkflowPrimitiveParameterDescriptor{
					checkboxWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterIncludeAllConstant,
						"Include non-Git folders",
						"Include non-repository folders under the selected root.",
						false,
					),
					checkboxWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterDebugConstant,
						"Debug output",
						"Enable verbose audit discovery output.",
						false,
					),
					selectWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterDepthConstant,
						"Inspection depth",
						"Choose between full and minimal audit inspection.",
						true,
						webWorkflowPrimitiveAuditDepthFullConst,
						[]web.WorkflowPrimitiveParameterOption{
							{Value: webWorkflowPrimitiveAuditDepthFullConst, Label: "Full"},
							{Value: webWorkflowPrimitiveAuditDepthMinConst, Label: "Minimal"},
						},
					),
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterOutputConstant,
						"Output file",
						"Write the audit report to a CSV file when provided.",
						false,
						"",
						webWorkflowPrimitiveOutputPlaceholderConstant,
					),
				},
			},
			buildOptions: func(parameters map[string]any) (map[string]any, error) {
				includeAll, includeAllError := readWebWorkflowBooleanParameter(parameters, webWorkflowPrimitiveAuditReportConstant, webWorkflowPrimitiveParameterIncludeAllConstant, false)
				if includeAllError != nil {
					return nil, includeAllError
				}
				debugOutput, debugError := readWebWorkflowBooleanParameter(parameters, webWorkflowPrimitiveAuditReportConstant, webWorkflowPrimitiveParameterDebugConstant, false)
				if debugError != nil {
					return nil, debugError
				}
				inspectionDepth, depthError := readWebWorkflowSelectParameter(
					parameters,
					webWorkflowPrimitiveAuditReportConstant,
					webWorkflowPrimitiveParameterDepthConstant,
					true,
					webWorkflowPrimitiveAuditDepthFullConst,
					map[string]struct{}{
						webWorkflowPrimitiveAuditDepthFullConst: {},
						webWorkflowPrimitiveAuditDepthMinConst:  {},
					},
				)
				if depthError != nil {
					return nil, depthError
				}
				outputPath, outputError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveAuditReportConstant, webWorkflowPrimitiveParameterOutputConstant, false, "")
				if outputError != nil {
					return nil, outputError
				}
				return workflowPrimitiveOptions(map[string]string{
					webWorkflowPrimitiveParameterDepthConstant:  inspectionDepth,
					webWorkflowPrimitiveParameterOutputConstant: outputPath,
				}, map[string]bool{
					webWorkflowPrimitiveParameterIncludeAllConstant: includeAll,
					webWorkflowPrimitiveParameterDebugConstant:      debugOutput,
				}), nil
			},
		},
		{
			descriptor: web.WorkflowPrimitiveDescriptor{
				ID:          webWorkflowPrimitiveHistoryPurgeConstant,
				Label:       "Purge history",
				Description: "Rewrite repository history to remove selected paths.",
				Parameters: []web.WorkflowPrimitiveParameterDescriptor{
					textareaWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterPathsConstant,
						"Paths",
						"Enter one repository-relative path per line.",
						true,
						"",
						webWorkflowPrimitivePathsPlaceholderConstant,
					),
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterRemoteConstant,
						"Remote name",
						"Push rewritten history to this remote when provided.",
						false,
						"",
						webWorkflowPrimitiveRemotePlaceholderConstant,
					),
					checkboxWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterPushConstant,
						"Push rewritten history",
						"Push updated refs after the rewrite.",
						true,
					),
					checkboxWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterRestoreConstant,
						"Restore worktree",
						"Restore the working tree after rewriting history.",
						true,
					),
					checkboxWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterPushMissingConstant,
						"Push missing refs",
						"Push refs that do not already exist on the remote.",
						false,
					),
				},
			},
			buildOptions: func(parameters map[string]any) (map[string]any, error) {
				paths, pathsError := readWebWorkflowStringListParameter(parameters, webWorkflowPrimitiveHistoryPurgeConstant, webWorkflowPrimitiveParameterPathsConstant, true)
				if pathsError != nil {
					return nil, pathsError
				}
				remoteName, remoteError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveHistoryPurgeConstant, webWorkflowPrimitiveParameterRemoteConstant, false, "")
				if remoteError != nil {
					return nil, remoteError
				}
				pushEnabled, pushError := readWebWorkflowBooleanParameter(parameters, webWorkflowPrimitiveHistoryPurgeConstant, webWorkflowPrimitiveParameterPushConstant, true)
				if pushError != nil {
					return nil, pushError
				}
				restoreEnabled, restoreError := readWebWorkflowBooleanParameter(parameters, webWorkflowPrimitiveHistoryPurgeConstant, webWorkflowPrimitiveParameterRestoreConstant, true)
				if restoreError != nil {
					return nil, restoreError
				}
				pushMissing, pushMissingError := readWebWorkflowBooleanParameter(parameters, webWorkflowPrimitiveHistoryPurgeConstant, webWorkflowPrimitiveParameterPushMissingConstant, false)
				if pushMissingError != nil {
					return nil, pushMissingError
				}
				options := map[string]any{
					webWorkflowPrimitiveParameterPathsConstant:       paths,
					webWorkflowPrimitiveParameterPushConstant:        pushEnabled,
					webWorkflowPrimitiveParameterRestoreConstant:     restoreEnabled,
					webWorkflowPrimitiveParameterPushMissingConstant: pushMissing,
				}
				if len(remoteName) > 0 {
					options[webWorkflowPrimitiveParameterRemoteConstant] = remoteName
				}
				return options, nil
			},
		},
		{
			descriptor: web.WorkflowPrimitiveDescriptor{
				ID:          webWorkflowPrimitiveFileReplaceConstant,
				Label:       "Replace in files",
				Description: "Run text replacements across matching files in the selected repository.",
				Parameters: []web.WorkflowPrimitiveParameterDescriptor{
					textareaWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterPatternsConstant,
						"Patterns",
						"Use one glob pattern per line.",
						true,
						"",
						webWorkflowPrimitivePatternPlaceholderConstant,
					),
					textareaWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterFindConstant,
						"Find",
						"Text to search for in matching files.",
						true,
						"",
						webWorkflowPrimitiveFindPlaceholderConstant,
					),
					textareaWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterReplaceConstant,
						"Replace with",
						"Replacement text written into matching files.",
						false,
						"",
						webWorkflowPrimitiveReplacePlaceholderConstant,
					),
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterCommandConstant,
						"Post-replacement command",
						"Optional shell command to run after files are updated.",
						false,
						"",
						webWorkflowPrimitiveCommandPlaceholderConstant,
					),
				},
			},
			buildOptions: func(parameters map[string]any) (map[string]any, error) {
				patterns, patternsError := readWebWorkflowStringListParameter(parameters, webWorkflowPrimitiveFileReplaceConstant, webWorkflowPrimitiveParameterPatternsConstant, true)
				if patternsError != nil {
					return nil, patternsError
				}
				findValue, findError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveFileReplaceConstant, webWorkflowPrimitiveParameterFindConstant, true, "")
				if findError != nil {
					return nil, findError
				}
				replaceValue, replaceError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveFileReplaceConstant, webWorkflowPrimitiveParameterReplaceConstant, false, "")
				if replaceError != nil {
					return nil, replaceError
				}
				commandValue, commandError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveFileReplaceConstant, webWorkflowPrimitiveParameterCommandConstant, false, "")
				if commandError != nil {
					return nil, commandError
				}
				options := map[string]any{
					webWorkflowPrimitiveParameterPatternsConstant: patterns,
					webWorkflowPrimitiveParameterFindConstant:     findValue,
					webWorkflowPrimitiveParameterReplaceConstant:  replaceValue,
				}
				if len(commandValue) > 0 {
					options[webWorkflowPrimitiveParameterCommandConstant] = commandValue
				}
				return options, nil
			},
		},
		{
			descriptor: web.WorkflowPrimitiveDescriptor{
				ID:          webWorkflowPrimitiveNamespaceRewriteConstant,
				Label:       "Rewrite namespace",
				Description: "Rewrite the Go module namespace across the selected repository.",
				Parameters: []web.WorkflowPrimitiveParameterDescriptor{
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterOldConstant,
						"Old namespace",
						"The namespace prefix to replace.",
						true,
						"",
						webWorkflowPrimitiveNamespaceOldPlaceholderConst,
					),
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterNewConstant,
						"New namespace",
						"The replacement namespace prefix.",
						true,
						"",
						webWorkflowPrimitiveNamespaceNewPlaceholderConst,
					),
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterBranchPrefixConstant,
						"Branch prefix",
						"Optional prefix for the working branch name.",
						false,
						"",
						webWorkflowPrimitiveBranchPrefixPlaceholderConst,
					),
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterCommitMessageConstant,
						"Commit message",
						"Optional commit message for the namespace rewrite.",
						false,
						"",
						webWorkflowPrimitiveCommitMessagePlaceholderConst,
					),
					checkboxWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterPushConstant,
						"Push rewrite branch",
						"Push the rewrite branch to the remote.",
						true,
					),
					textWorkflowPrimitiveParameter(
						webWorkflowPrimitiveParameterRemoteConstant,
						"Remote name",
						"Remote used when pushing the rewrite branch.",
						false,
						"",
						webWorkflowPrimitiveRemotePlaceholderConstant,
					),
				},
			},
			buildOptions: func(parameters map[string]any) (map[string]any, error) {
				oldNamespace, oldError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveNamespaceRewriteConstant, webWorkflowPrimitiveParameterOldConstant, true, "")
				if oldError != nil {
					return nil, oldError
				}
				newNamespace, newError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveNamespaceRewriteConstant, webWorkflowPrimitiveParameterNewConstant, true, "")
				if newError != nil {
					return nil, newError
				}
				branchPrefix, branchPrefixError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveNamespaceRewriteConstant, webWorkflowPrimitiveParameterBranchPrefixConstant, false, "")
				if branchPrefixError != nil {
					return nil, branchPrefixError
				}
				commitMessage, commitMessageError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveNamespaceRewriteConstant, webWorkflowPrimitiveParameterCommitMessageConstant, false, "")
				if commitMessageError != nil {
					return nil, commitMessageError
				}
				pushEnabled, pushError := readWebWorkflowBooleanParameter(parameters, webWorkflowPrimitiveNamespaceRewriteConstant, webWorkflowPrimitiveParameterPushConstant, true)
				if pushError != nil {
					return nil, pushError
				}
				remoteName, remoteError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveNamespaceRewriteConstant, webWorkflowPrimitiveParameterRemoteConstant, false, "")
				if remoteError != nil {
					return nil, remoteError
				}
				return workflowPrimitiveOptions(map[string]string{
					webWorkflowPrimitiveParameterOldConstant:           oldNamespace,
					webWorkflowPrimitiveParameterNewConstant:           newNamespace,
					webWorkflowPrimitiveParameterBranchPrefixConstant:  branchPrefix,
					webWorkflowPrimitiveParameterCommitMessageConstant: commitMessage,
					webWorkflowPrimitiveParameterRemoteConstant:        remoteName,
				}, map[string]bool{
					webWorkflowPrimitiveParameterPushConstant: pushEnabled,
				}), nil
			},
		},
	}
}

func workflowPrimitiveOptions(stringOptions map[string]string, booleanOptions ...map[string]bool) map[string]any {
	options := map[string]any{}
	for key, value := range stringOptions {
		if len(strings.TrimSpace(value)) == 0 {
			continue
		}
		options[key] = value
	}
	for _, booleanOptionSet := range booleanOptions {
		for key, value := range booleanOptionSet {
			options[key] = value
		}
	}
	return options
}

func textWorkflowPrimitiveParameter(key string, label string, description string, required bool, defaultValue string, placeholder string) web.WorkflowPrimitiveParameterDescriptor {
	return web.WorkflowPrimitiveParameterDescriptor{
		Key:          key,
		Label:        label,
		Description:  description,
		Control:      web.WorkflowPrimitiveParameterControlText,
		Required:     required,
		DefaultValue: defaultValue,
		Placeholder:  placeholder,
	}
}

func textareaWorkflowPrimitiveParameter(key string, label string, description string, required bool, defaultValue string, placeholder string) web.WorkflowPrimitiveParameterDescriptor {
	return web.WorkflowPrimitiveParameterDescriptor{
		Key:          key,
		Label:        label,
		Description:  description,
		Control:      web.WorkflowPrimitiveParameterControlTextarea,
		Required:     required,
		DefaultValue: defaultValue,
		Placeholder:  placeholder,
	}
}

func checkboxWorkflowPrimitiveParameter(key string, label string, description string, defaultValue bool) web.WorkflowPrimitiveParameterDescriptor {
	return web.WorkflowPrimitiveParameterDescriptor{
		Key:         key,
		Label:       label,
		Description: description,
		Control:     web.WorkflowPrimitiveParameterControlCheckbox,
		DefaultBool: boolPointer(defaultValue),
	}
}

func selectWorkflowPrimitiveParameter(key string, label string, description string, required bool, defaultValue string, options []web.WorkflowPrimitiveParameterOption) web.WorkflowPrimitiveParameterDescriptor {
	return web.WorkflowPrimitiveParameterDescriptor{
		Key:          key,
		Label:        label,
		Description:  description,
		Control:      web.WorkflowPrimitiveParameterControlSelect,
		Required:     required,
		DefaultValue: defaultValue,
		Options:      append([]web.WorkflowPrimitiveParameterOption(nil), options...),
	}
}

func boolPointer(value bool) *bool {
	return &value
}

func protocolWorkflowPrimitiveOptions() []web.WorkflowPrimitiveParameterOption {
	return []web.WorkflowPrimitiveParameterOption{
		{Value: webWorkflowPrimitiveProtocolSSHConstant, Label: "SSH"},
		{Value: webWorkflowPrimitiveProtocolHTTPSConstant, Label: "HTTPS"},
	}
}

func allowedProtocolValues() map[string]struct{} {
	return map[string]struct{}{
		webWorkflowPrimitiveProtocolSSHConstant:   {},
		webWorkflowPrimitiveProtocolHTTPSConstant: {},
	}
}

func readWebWorkflowStringParameter(parameters map[string]any, primitiveID string, key string, required bool, defaultValue string) (string, error) {
	rawValue, exists := parameters[key]
	if !exists || rawValue == nil {
		if required && len(strings.TrimSpace(defaultValue)) == 0 {
			return "", fmt.Errorf(webWorkflowPrimitiveParameterRequiredTemplateConstant, primitiveID, key)
		}
		return strings.TrimSpace(defaultValue), nil
	}

	typedValue, ok := rawValue.(string)
	if !ok {
		return "", fmt.Errorf(webWorkflowPrimitiveParameterTypeTemplateConstant, primitiveID, key, webWorkflowPrimitiveParameterTypeStringConstant)
	}

	normalizedValue := strings.TrimSpace(typedValue)
	if len(normalizedValue) == 0 {
		if required && len(strings.TrimSpace(defaultValue)) == 0 {
			return "", fmt.Errorf(webWorkflowPrimitiveParameterRequiredTemplateConstant, primitiveID, key)
		}
		if len(normalizedValue) == 0 {
			return strings.TrimSpace(defaultValue), nil
		}
	}

	if len(normalizedValue) == 0 && required {
		return "", fmt.Errorf(webWorkflowPrimitiveParameterRequiredTemplateConstant, primitiveID, key)
	}
	return normalizedValue, nil
}

func readWebWorkflowBooleanParameter(parameters map[string]any, primitiveID string, key string, defaultValue bool) (bool, error) {
	rawValue, exists := parameters[key]
	if !exists || rawValue == nil {
		return defaultValue, nil
	}

	typedValue, ok := rawValue.(bool)
	if !ok {
		return false, fmt.Errorf(webWorkflowPrimitiveParameterTypeTemplateConstant, primitiveID, key, webWorkflowPrimitiveParameterTypeBooleanConstant)
	}

	return typedValue, nil
}

func readWebWorkflowSelectParameter(
	parameters map[string]any,
	primitiveID string,
	key string,
	required bool,
	defaultValue string,
	allowedValues map[string]struct{},
) (string, error) {
	value, valueError := readWebWorkflowStringParameter(parameters, primitiveID, key, required, defaultValue)
	if valueError != nil {
		return "", valueError
	}
	if len(value) == 0 {
		return "", nil
	}
	if _, exists := allowedValues[value]; !exists {
		return "", fmt.Errorf(webWorkflowPrimitiveParameterValueTemplateConstant, primitiveID, value, key)
	}
	return value, nil
}

func readWebWorkflowStringListParameter(parameters map[string]any, primitiveID string, key string, required bool) ([]string, error) {
	value, valueError := readWebWorkflowStringParameter(parameters, primitiveID, key, required, "")
	if valueError != nil {
		return nil, valueError
	}

	lines := strings.Split(value, "\n")
	collected := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if len(trimmedLine) == 0 {
			continue
		}
		collected = append(collected, trimmedLine)
	}

	if required && len(collected) == 0 {
		return nil, fmt.Errorf(webWorkflowPrimitiveParameterRequiredTemplateConstant, primitiveID, key)
	}

	return collected, nil
}

func readWebWorkflowRetagMappingsParameter(parameters map[string]any) ([]map[string]any, error) {
	rawMappings, mappingsError := readWebWorkflowStringParameter(parameters, webWorkflowPrimitiveReleaseRetagConstant, webWorkflowPrimitiveParameterMappingsConstant, true, "")
	if mappingsError != nil {
		return nil, mappingsError
	}

	reader := csv.NewReader(strings.NewReader(rawMappings))
	reader.FieldsPerRecord = -1
	records, readError := reader.ReadAll()
	if readError != nil {
		return nil, fmt.Errorf("%s: %w", webWorkflowPrimitiveRetagMappingsFormatConstant, readError)
	}

	mappings := make([]map[string]any, 0, len(records))
	for _, record := range records {
		trimmedRecord := make([]string, 0, len(record))
		for _, column := range record {
			trimmedColumn := strings.TrimSpace(column)
			if len(trimmedColumn) == 0 {
				trimmedRecord = append(trimmedRecord, "")
				continue
			}
			trimmedRecord = append(trimmedRecord, trimmedColumn)
		}
		if len(trimmedRecord) == 0 {
			continue
		}
		if len(trimmedRecord) < 2 || len(trimmedRecord) > 3 {
			return nil, errors.New(webWorkflowPrimitiveRetagMappingsFormatConstant)
		}
		if len(trimmedRecord[0]) == 0 || len(trimmedRecord[1]) == 0 {
			return nil, errors.New(webWorkflowPrimitiveRetagMappingsFormatConstant)
		}

		mapping := map[string]any{
			webWorkflowPrimitiveParameterTagConstant:    trimmedRecord[0],
			webWorkflowPrimitiveParameterTargetConstant: trimmedRecord[1],
		}
		if len(trimmedRecord) == 3 && len(trimmedRecord[2]) > 0 {
			mapping[webWorkflowPrimitiveParameterMessageConstant] = trimmedRecord[2]
		}
		mappings = append(mappings, mapping)
	}

	if len(mappings) == 0 {
		return nil, errors.New(webWorkflowPrimitiveRetagMappingsFormatConstant)
	}

	return mappings, nil
}
