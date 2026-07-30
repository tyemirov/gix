package syncflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/workflow"
	"github.com/tyemirov/utils/llm"
)

const (
	gitDiffNameOnlyFlagConstant                 = "--name-only"
	gitDiffFilterUnmergedFlagConstant           = "--diff-filter=U"
	gitLsFilesSubcommandConstant                = "ls-files"
	gitLsFilesUnmergedFlagConstant              = "-u"
	gitShowSubcommandConstant                   = "show"
	gitCheckoutSubcommandConstant               = "checkout"
	gitCheckoutConflictDiff3FlagConstant        = "--conflict=diff3"
	gitRmSubcommandConstant                     = "rm"
	gitRmForceFlagConstant                      = "-f"
	gitCommitNoEditFlagConstant                 = "--no-edit"
	gitMergeAbortFlagConstant                   = "--abort"
	gitMergeHeadReferenceConstant               = "MERGE_HEAD"
	gitDiffCachedFlagConstant                   = "--cached"
	gitDiffCheckFlagConstant                    = "--check"
	mergeConflictResolutionMaxTokens            = 8192
	mergeConflictResolutionMaxSemanticAttempts  = 4
	mergeConflictResolutionFailureTemplate      = "failed to resolve merge conflicts: %w"
	mergeConflictResolutionInspectFailure       = "inspect unmerged files: %w"
	mergeConflictResolutionStageInspectTemplate = "inspect conflict stages for %s: %w"
	mergeConflictResolutionStageReadTemplate    = "read %s stage %d: %w"
	mergeConflictResolutionWorktreeReadTemplate = "read conflicted worktree file %s: %w"
	mergeConflictResolutionDiff3Template        = "render diff3 conflict regions for %s: %w"
	mergeConflictResolutionEmptyResponse        = "llm returned an empty merge resolution for %s"
	mergeConflictResolutionEnvelopeTemplate     = "llm merge resolution for %s did not use the required content envelope"
	mergeConflictResolutionConflictMarkers      = "llm left conflict markers in merge resolution for %s"
	mergeConflictResolutionOursTemplate         = "llm merge resolution for %s conflict region %d does not preserve OURS replacement intent %q"
	mergeConflictResolutionTheirsTemplate       = "llm merge resolution for %s conflict region %d does not preserve THEIRS replacement intent %q"
	mergeConflictResolutionAdditiveTemplate     = "llm merge resolution for %s additive conflict region %d is not an exact ordering of OURS and THEIRS"
	mergeConflictResolutionIntentTemplate       = "llm merge resolution for %s conflict region %d cannot be validated against %s replacement intent"
	mergeConflictResolutionBaseOnlyTemplate     = "llm merge resolution for %s conflict region %d returned BASE without either side's changes"
	mergeConflictResolutionExhaustedTemplate    = "semantic resolution for %s conflict region %d exhausted %d validated attempts: %w"
	mergeConflictResolutionStructureTemplate    = "conflicted worktree file %s has invalid conflict marker structure"
	mergeConflictResolutionWriteTemplate        = "write resolved merge file %s: %w"
	mergeConflictResolutionStageTemplate        = "stage resolved merge file %s: %w"
	mergeConflictResolutionDeleteTemplate       = "stage deleted merge file %s: %w"
	mergeConflictResolutionIndexCheckTemplate   = "validate resolved merge index: %w"
	mergeConflictResolutionCommitTemplate       = "complete resolved merge commit: %w"
	mergeConflictResolutionPathTemplate         = "invalid conflicted path %q"
	mergeConflictResolutionTimeoutTemplate      = "AI merge resolution timed out after %s"
	mergeConflictResolutionCanceledMessage      = "AI merge resolution was canceled"
	mergeConflictResolutionStateInspectTemplate = "inspect operation-owned merge state: %w"
	mergeConflictResolutionRollbackTemplate     = "All automatic merge resolution strategies stopped after: %s. The failed merge was aborted as the final recovery strategy; branch %s was restored to its pre-merge state and gix did not push."
	mergeConflictResolutionRollbackFailure      = "automatic merge rollback failed: %w"
	mergeConflictResolutionHandoffTemplate      = "All automatic merge resolution strategies stopped after: %s. Final recovery rollback also failed: %s. gix did not push. Inspect git status before manual recovery."
	mergeConflictResolutionContentBegin         = "GIX_MERGE_RESOLUTION_CONTENT_BEGIN"
	mergeConflictResolutionContentEnd           = "GIX_MERGE_RESOLUTION_CONTENT_END"
	mergeConflictResolutionReviewApproved       = "GIX_MERGE_REVIEW_APPROVED"
	mergeConflictResolutionRegionSystemPrompt   = "You are an expert merge engineer resolving one genuinely overlapping Git conflict region after deterministic merge strategies were exhausted. Preserve the semantic intent of every BASE-to-OURS change and integrate every compatible BASE-to-THEIRS change. Return only the replacement contents for this region, never the complete file, between the required " + mergeConflictResolutionContentBegin + " and " + mergeConflictResolutionContentEnd + " lines. Remove conflict markers. Do not include surrounding file content, explanations, markdown fences, or quotes."
	mergeConflictResolutionRegionUserPrompt     = "Repository: %s\nPath: %s\nConflict region: %d of %d\nTarget branch: %s\nMerged reference: %s\n\nBASE common ancestor region:\n%s\n\nOURS current branch region that must be preserved:\n%s\n\nTHEIRS incoming branch region to integrate:\n%s\n\nReturn exactly:\n" + mergeConflictResolutionContentBegin + "\n<resolved replacement contents for this conflict region>\n" + mergeConflictResolutionContentEnd
	mergeConflictResolutionRepairPrompt         = "The previous candidate was rejected by deterministic validation:\n%s\n\nProduce a corrected candidate from BASE, OURS, and THEIRS. Do not repeat the rejected candidate blindly."
	mergeConflictResolutionReviewSystemPrompt   = "You are the final semantic fidelity auditor for one Git conflict region. Compare the candidate against BASE, OURS, and THEIRS. Approve only when every BASE-to-OURS change and every compatible BASE-to-THEIRS change is preserved without conflict markers, duplicated obsolete content, or invented behavior. Return exactly " + mergeConflictResolutionReviewApproved + " when the candidate is correct. Otherwise return a corrected candidate between the required content sentinels. Do not include explanations, markdown fences, or quotes."
	mergeConflictResolutionReviewUserPrompt     = "Repository: %s\nPath: %s\nConflict region: %d of %d\nTarget branch: %s\nMerged reference: %s\n\nBASE common ancestor region:\n%s\n\nOURS current branch region:\n%s\n\nTHEIRS incoming branch region:\n%s\n\nLOCALLY VALIDATED CANDIDATE:\n%s\n\nReturn exactly " + mergeConflictResolutionReviewApproved + " only after semantic audit. Otherwise return exactly:\n" + mergeConflictResolutionContentBegin + "\n<corrected replacement contents for this conflict region>\n" + mergeConflictResolutionContentEnd
	mergeConflictResolutionAbsentStage          = "(file absent in this stage)"
	mergeConflictResolutionProgressMaximum      = 10 * time.Second
	mergeConflictResolutionRollbackTimeout      = 30 * time.Second
)

var errMergeConflictResolutionDeadline = errors.New("AI merge resolution deadline exceeded")

type mergeConflictResolutionService struct {
	executor       shared.GitExecutor
	repositoryPath string
	commitMessages worktreeAdoptionCommitMessageOptions
	reporter       mergeConflictResolutionReporter
}

type mergeConflictResolutionReporter func(level shared.EventLevel, code string, message string, details map[string]string)

type mergeConflictResolutionClientProvider func() (llm.ChatClient, error)

type mergeConflictResolutionOptions struct {
	SourceReference string
	TargetBranch    string
	Completion      mergeConflictCompletion
}

type mergeConflictCompletion uint8

const (
	mergeConflictCompletionCommit mergeConflictCompletion = iota
	mergeConflictCompletionPreserveIndex
)

type mergeConflictFile struct {
	Path            string
	Base            string
	Ours            string
	OursPresent     bool
	Theirs          string
	WorktreeContent string
}

type mergeConflictFileResolution struct {
	Delete  bool
	Content string
}

type mergeConflictDocument struct {
	NonConflictingRegions []string
	ConflictRegions       []mergeConflictRegion
}

type mergeConflictRegion struct {
	Ours        string
	Base        string
	BasePresent bool
	Theirs      string
}

type mergeConflictMarkerState uint8

const (
	mergeConflictMarkerStateOutside mergeConflictMarkerState = iota
	mergeConflictMarkerStateOurs
	mergeConflictMarkerStateBase
	mergeConflictMarkerStateTheirs
)

func resolveMergeConflictOrError(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, executor shared.GitExecutor, repositoryPath string, sourceReference string, targetBranch string, conflictMessage string, commitMessages worktreeAdoptionCommitMessageOptions, mergeErr error) error {
	service := mergeConflictResolutionService{
		executor:       executor,
		repositoryPath: repositoryPath,
		commitMessages: commitMessages,
		reporter: func(level shared.EventLevel, code string, message string, details map[string]string) {
			environment.ReportRepositoryEvent(repository, level, code, message, details)
		},
	}
	conflictObserved, resolveErr := service.Resolve(ctx, mergeConflictResolutionOptions{
		SourceReference: sourceReference,
		TargetBranch:    targetBranch,
	})
	if resolveErr != nil {
		rollbackRequired := conflictObserved
		if !rollbackRequired {
			var mergeStateErr error
			rollbackRequired, mergeStateErr = service.operationOwnedMergeInProgress(ctx)
			if mergeStateErr != nil {
				service.reportMergeConflictHandoff(resolveErr, mergeStateErr, sourceReference, targetBranch)
				resolveErr = errors.Join(resolveErr, mergeStateErr)
			}
		}
		if rollbackRequired {
			rollbackErr := service.rollbackFailedMerge(ctx, resolveErr, sourceReference, targetBranch)
			if rollbackErr != nil {
				resolveErr = errors.Join(resolveErr, rollbackErr)
			}
		}
		return fmt.Errorf("%s: %w", conflictMessage, errors.Join(fmt.Errorf(mergeConflictResolutionFailureTemplate, resolveErr), mergeErr))
	}
	if !conflictObserved {
		return fmt.Errorf("%s: %w", conflictMessage, mergeErr)
	}
	return nil
}

func (service mergeConflictResolutionService) operationOwnedMergeInProgress(ctx context.Context) (bool, error) {
	inspectionContext, cancelInspection := context.WithTimeout(context.WithoutCancel(ctx), mergeConflictResolutionRollbackTimeout)
	defer cancelInspection()

	_, inspectionErr := service.executor.ExecuteGit(inspectionContext, execshell.CommandDetails{
		Arguments:        []string{gitRevParseSubcommandConstant, gitVerifyFlagConstant, gitRevParseQuietFlagConstant, gitMergeHeadReferenceConstant},
		WorkingDirectory: service.repositoryPath,
	})
	if inspectionErr == nil {
		return true, nil
	}
	var commandFailure execshell.CommandFailedError
	if errors.As(inspectionErr, &commandFailure) && commandFailure.Result.ExitCode == 1 {
		return false, nil
	}
	return false, fmt.Errorf(mergeConflictResolutionStateInspectTemplate, inspectionErr)
}

func (service mergeConflictResolutionService) Resolve(ctx context.Context, options mergeConflictResolutionOptions) (bool, error) {
	timeout := worktreeAdoptionMessageTimeout(service.commitMessages)
	paths, pathsErr := service.unmergedPaths(ctx)
	if pathsErr != nil {
		return false, service.normalizeResolutionError(ctx, fmt.Errorf(mergeConflictResolutionInspectFailure, pathsErr))
	}
	if len(paths) == 0 {
		return false, nil
	}
	service.reportConflictDetected(paths, options, timeout)

	var client llm.ChatClient
	clientProvider := func() (llm.ChatClient, error) {
		if client != nil {
			return client, nil
		}
		resolvedClient, clientErr := resolveMergeConflictResolutionClient(service.commitMessages)
		if clientErr != nil {
			return nil, clientErr
		}
		client = resolvedClient
		return client, nil
	}

	for pathIndex := range paths {
		conflictFile, conflictFileErr := service.collectConflictFile(ctx, paths[pathIndex])
		if conflictFileErr != nil {
			return true, service.normalizeResolutionError(ctx, conflictFileErr)
		}
		resolution, resolutionErr := service.resolveConflictFile(ctx, clientProvider, options, conflictFile, timeout)
		if resolutionErr != nil {
			return true, service.normalizeResolutionError(ctx, resolutionErr)
		}
		if resolution.Delete {
			if deleteErr := service.stageDeletedFile(ctx, conflictFile.Path); deleteErr != nil {
				return true, service.normalizeResolutionError(ctx, deleteErr)
			}
		} else {
			if writeErr := service.writeResolvedFile(conflictFile.Path, resolution.Content); writeErr != nil {
				return true, service.normalizeResolutionError(ctx, writeErr)
			}
			if stageErr := service.stageResolvedFile(ctx, conflictFile.Path); stageErr != nil {
				return true, service.normalizeResolutionError(ctx, stageErr)
			}
		}
	}

	remainingPaths, remainingErr := service.unmergedPaths(ctx)
	if remainingErr != nil {
		return true, service.normalizeResolutionError(ctx, fmt.Errorf(mergeConflictResolutionInspectFailure, remainingErr))
	}
	if len(remainingPaths) > 0 {
		return true, fmt.Errorf("unresolved merge conflicts remain: %s", strings.Join(remainingPaths, ", "))
	}
	if indexCheckErr := executeGit(ctx, service.executor, service.repositoryPath, []string{gitDiffSubcommandConstant, gitDiffCachedFlagConstant, gitDiffCheckFlagConstant}); indexCheckErr != nil {
		return true, service.normalizeResolutionError(ctx, fmt.Errorf(mergeConflictResolutionIndexCheckTemplate, indexCheckErr))
	}

	if options.Completion == mergeConflictCompletionPreserveIndex {
		service.report(shared.EventLevelInfo, shared.EventCodeAIMergeResolution, "all conflict resolution strategies validated; preserving the restored index", map[string]string{
			"paths": strings.Join(paths, ", "),
		})
	} else {
		service.report(shared.EventLevelInfo, shared.EventCodeAIMergeResolution, "all conflict resolution strategies validated; completing merge commit", map[string]string{
			"paths": strings.Join(paths, ", "),
		})
		if commitErr := executeGit(ctx, service.executor, service.repositoryPath, []string{gitCommitSubcommandConstant, gitCommitNoEditFlagConstant}); commitErr != nil {
			return true, service.normalizeResolutionError(ctx, fmt.Errorf(mergeConflictResolutionCommitTemplate, commitErr))
		}
	}
	service.report(shared.EventLevelInfo, shared.EventCodeAIMergeResolution, "merge conflict resolution completed", map[string]string{
		"paths": strings.Join(paths, ", "),
	})
	return true, nil
}

func (service mergeConflictResolutionService) normalizeResolutionError(ctx context.Context, resolutionErr error) error {
	if errors.Is(context.Cause(ctx), context.Canceled) || errors.Is(resolutionErr, context.Canceled) {
		return fmt.Errorf(mergeConflictResolutionCanceledMessage+": %w", resolutionErr)
	}
	if errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
		return fmt.Errorf("merge resolution caller deadline exceeded: %w", resolutionErr)
	}
	return resolutionErr
}

func (service mergeConflictResolutionService) reportConflictDetected(paths []string, options mergeConflictResolutionOptions, timeout time.Duration) {
	pathNoun := "paths"
	if len(paths) == 1 {
		pathNoun = "path"
	}
	service.report(
		shared.EventLevelInfo,
		shared.EventCodeMergeConflict,
		fmt.Sprintf("detected %d conflicted %s while merging %s into %s; exhausting fidelity-first resolution strategies before rollback", len(paths), pathNoun, strings.TrimSpace(options.SourceReference), strings.TrimSpace(options.TargetBranch)),
		map[string]string{
			"paths":            strings.Join(paths, ", "),
			"source_reference": strings.TrimSpace(options.SourceReference),
			"target_branch":    strings.TrimSpace(options.TargetBranch),
			"timeout":          timeout.String(),
		},
	)
}

func (service mergeConflictResolutionService) rollbackFailedMerge(ctx context.Context, resolutionErr error, sourceReference string, targetBranch string) error {
	rollbackContext, cancelRollback := context.WithTimeout(context.WithoutCancel(ctx), mergeConflictResolutionRollbackTimeout)
	defer cancelRollback()

	if abortErr := executeGit(rollbackContext, service.executor, service.repositoryPath, []string{gitMergeSubcommandConstant, gitMergeAbortFlagConstant}); abortErr != nil {
		rollbackErr := fmt.Errorf(mergeConflictResolutionRollbackFailure, abortErr)
		service.reportMergeConflictHandoff(resolutionErr, rollbackErr, sourceReference, targetBranch)
		return rollbackErr
	}
	service.reportMergeConflictRollback(resolutionErr, sourceReference, targetBranch)
	return nil
}

func (service mergeConflictResolutionService) reportMergeConflictRollback(resolutionErr error, sourceReference string, targetBranch string) {
	reason := strings.ReplaceAll(strings.TrimSpace(resolutionErr.Error()), "\n", "; ")
	service.report(
		shared.EventLevelError,
		shared.EventCodeAIMergeRollback,
		fmt.Sprintf(mergeConflictResolutionRollbackTemplate, reason, strings.TrimSpace(targetBranch)),
		map[string]string{
			"source_reference": strings.TrimSpace(sourceReference),
			"target_branch":    strings.TrimSpace(targetBranch),
			"reason":           reason,
		},
	)
}

func (service mergeConflictResolutionService) reportMergeConflictHandoff(resolutionErr error, rollbackErr error, sourceReference string, targetBranch string) {
	reason := strings.ReplaceAll(strings.TrimSpace(resolutionErr.Error()), "\n", "; ")
	rollbackReason := strings.ReplaceAll(strings.TrimSpace(rollbackErr.Error()), "\n", "; ")
	service.report(
		shared.EventLevelError,
		shared.EventCodeAIMergeHandoff,
		fmt.Sprintf(mergeConflictResolutionHandoffTemplate, reason, rollbackReason),
		map[string]string{
			"source_reference": strings.TrimSpace(sourceReference),
			"target_branch":    strings.TrimSpace(targetBranch),
			"reason":           reason,
			"rollback_reason":  rollbackReason,
		},
	)
}

func (service mergeConflictResolutionService) report(level shared.EventLevel, code string, message string, details map[string]string) {
	if service.reporter == nil {
		return
	}
	service.reporter(level, code, message, details)
}

func (service mergeConflictResolutionService) unmergedPaths(ctx context.Context) ([]string, error) {
	result, executionErr := service.executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitDiffSubcommandConstant, gitDiffNameOnlyFlagConstant, gitDiffFilterUnmergedFlagConstant},
		WorkingDirectory: service.repositoryPath,
	})
	if executionErr != nil {
		return nil, executionErr
	}

	paths := make([]string, 0)
	seenPaths := map[string]struct{}{}
	for _, line := range strings.Split(result.StandardOutput, "\n") {
		path := strings.TrimSpace(line)
		if path == "" {
			continue
		}
		if _, seen := seenPaths[path]; seen {
			continue
		}
		seenPaths[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths, nil
}

func (service mergeConflictResolutionService) collectConflictFile(ctx context.Context, path string) (mergeConflictFile, error) {
	stages, stagesErr := service.conflictStages(ctx, path)
	if stagesErr != nil {
		return mergeConflictFile{}, stagesErr
	}

	base, baseErr := service.conflictStageContent(ctx, path, stages, 1)
	if baseErr != nil {
		return mergeConflictFile{}, baseErr
	}
	ours, oursErr := service.conflictStageContent(ctx, path, stages, 2)
	if oursErr != nil {
		return mergeConflictFile{}, oursErr
	}
	theirs, theirsErr := service.conflictStageContent(ctx, path, stages, 3)
	if theirsErr != nil {
		return mergeConflictFile{}, theirsErr
	}
	worktreeContent, worktreeContentErr := service.conflictedWorktreeContent(path)
	if worktreeContentErr != nil {
		return mergeConflictFile{}, worktreeContentErr
	}
	if containsConflictMarker(worktreeContent) {
		if diff3Err := service.renderDiff3ConflictRegions(ctx, path); diff3Err != nil {
			return mergeConflictFile{}, diff3Err
		}
		worktreeContent, worktreeContentErr = service.conflictedWorktreeContent(path)
		if worktreeContentErr != nil {
			return mergeConflictFile{}, worktreeContentErr
		}
	}
	_, oursPresent := stages[2]
	return mergeConflictFile{
		Path:            path,
		Base:            base,
		Ours:            ours,
		OursPresent:     oursPresent,
		Theirs:          theirs,
		WorktreeContent: worktreeContent,
	}, nil
}

func (service mergeConflictResolutionService) renderDiff3ConflictRegions(ctx context.Context, path string) error {
	if checkoutErr := executeGit(
		ctx,
		service.executor,
		service.repositoryPath,
		[]string{
			gitCheckoutSubcommandConstant,
			gitCheckoutConflictDiff3FlagConstant,
			gitPathspecSeparatorConstant,
			path,
		},
	); checkoutErr != nil {
		return fmt.Errorf(mergeConflictResolutionDiff3Template, path, checkoutErr)
	}
	return nil
}

func (service mergeConflictResolutionService) conflictedWorktreeContent(path string) (string, error) {
	worktreePath, pathErr := mergeConflictResolutionFilesystemPath(service.repositoryPath, path)
	if pathErr != nil {
		return "", pathErr
	}
	content, readErr := os.ReadFile(worktreePath)
	if readErr == nil {
		return string(content), nil
	}
	if os.IsNotExist(readErr) {
		return "", nil
	}
	return "", fmt.Errorf(mergeConflictResolutionWorktreeReadTemplate, path, readErr)
}

func (service mergeConflictResolutionService) conflictStages(ctx context.Context, path string) (map[int]struct{}, error) {
	result, executionErr := service.executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitLsFilesSubcommandConstant, gitLsFilesUnmergedFlagConstant, gitPathspecSeparatorConstant, path},
		WorkingDirectory: service.repositoryPath,
	})
	if executionErr != nil {
		return nil, fmt.Errorf(mergeConflictResolutionStageInspectTemplate, path, executionErr)
	}

	stages := map[int]struct{}{}
	for _, line := range strings.Split(result.StandardOutput, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		stage, stageErr := strconv.Atoi(fields[2])
		if stageErr != nil {
			return nil, fmt.Errorf("parse conflict stage for %s: %w", path, stageErr)
		}
		stages[stage] = struct{}{}
	}
	return stages, nil
}

func (service mergeConflictResolutionService) conflictStageContent(ctx context.Context, path string, stages map[int]struct{}, stage int) (string, error) {
	if _, exists := stages[stage]; !exists {
		return mergeConflictResolutionAbsentStage, nil
	}
	stageReference := fmt.Sprintf(":%d:%s", stage, path)
	result, executionErr := service.executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitShowSubcommandConstant, stageReference},
		WorkingDirectory: service.repositoryPath,
	})
	if executionErr != nil {
		return "", fmt.Errorf(mergeConflictResolutionStageReadTemplate, path, stage, executionErr)
	}
	return result.StandardOutput, nil
}

func (service mergeConflictResolutionService) resolveConflictFile(ctx context.Context, clientProvider mergeConflictResolutionClientProvider, options mergeConflictResolutionOptions, conflictFile mergeConflictFile, timeout time.Duration) (mergeConflictFileResolution, error) {
	document, parseErr := parseMergeConflictDocument(conflictFile.WorktreeContent)
	if parseErr != nil {
		return mergeConflictFileResolution{}, fmt.Errorf(mergeConflictResolutionStructureTemplate+": %w", conflictFile.Path, parseErr)
	}
	if len(document.ConflictRegions) == 0 {
		return service.resolveMarkerFreeConflictFile(conflictFile), nil
	}

	resolvedRegions := make([]string, len(document.ConflictRegions))
	for regionIndex := range document.ConflictRegions {
		region := document.ConflictRegions[regionIndex]
		initialCandidate := ""
		if deterministicResolution, resolved := deterministicMergeConflictRegionResolution(region); resolved {
			if !deterministicResolution.RequiresSemanticAudit {
				resolvedRegions[regionIndex] = deterministicResolution.Content
				service.report(
					shared.EventLevelInfo,
					shared.EventCodeAIMergeResolution,
					fmt.Sprintf(
						"resolved %s conflict region %d/%d deterministically using %s",
						conflictFile.Path,
						regionIndex+1,
						len(document.ConflictRegions),
						deterministicResolution.Strategy,
					),
					map[string]string{
						"path":     conflictFile.Path,
						"region":   strconv.Itoa(regionIndex + 1),
						"regions":  strconv.Itoa(len(document.ConflictRegions)),
						"strategy": deterministicResolution.Strategy,
					},
				)
				continue
			}
			if validationErr := validateMergeConflictRegionResponse(
				conflictFile.Path,
				regionIndex,
				region,
				deterministicResolution.Content,
			); validationErr == nil {
				initialCandidate = deterministicResolution.Content
				service.report(
					shared.EventLevelInfo,
					shared.EventCodeAIMergeResolution,
					fmt.Sprintf(
						"derived %s conflict region %d/%d candidate using %s; requesting semantic audit",
						conflictFile.Path,
						regionIndex+1,
						len(document.ConflictRegions),
						deterministicResolution.Strategy,
					),
					map[string]string{
						"path":     conflictFile.Path,
						"region":   strconv.Itoa(regionIndex + 1),
						"regions":  strconv.Itoa(len(document.ConflictRegions)),
						"strategy": deterministicResolution.Strategy,
					},
				)
			}
		}

		client, clientErr := clientProvider()
		if clientErr != nil {
			return mergeConflictFileResolution{}, clientErr
		}
		resolvedRegion, resolutionErr := service.resolveSemanticConflictRegion(
			ctx,
			client,
			options,
			conflictFile,
			region,
			regionIndex,
			len(document.ConflictRegions),
			timeout,
			initialCandidate,
		)
		if resolutionErr != nil {
			return mergeConflictFileResolution{}, resolutionErr
		}
		resolvedRegions[regionIndex] = resolvedRegion
	}

	resolution := mergeConflictFileResolution{Content: document.resolve(resolvedRegions)}
	service.report(
		shared.EventLevelInfo,
		shared.EventCodeAIMergeResolution,
		fmt.Sprintf("validated all conflict regions for %s; staging", conflictFile.Path),
		map[string]string{
			"path":    conflictFile.Path,
			"regions": strconv.Itoa(len(document.ConflictRegions)),
		},
	)
	return resolution, nil
}

func (service mergeConflictResolutionService) resolveMarkerFreeConflictFile(conflictFile mergeConflictFile) mergeConflictFileResolution {
	var resolution mergeConflictFileResolution
	strategy := "current-stage content preservation"
	if conflictFile.OursPresent {
		resolution = mergeConflictFileResolution{Content: conflictFile.Ours}
	} else {
		resolution = mergeConflictFileResolution{Delete: true}
		strategy = "current-stage deletion preservation"
	}
	service.report(
		shared.EventLevelInfo,
		shared.EventCodeAIMergeResolution,
		fmt.Sprintf("resolved marker-free conflict for %s deterministically using %s", conflictFile.Path, strategy),
		map[string]string{
			"path":     conflictFile.Path,
			"strategy": strategy,
		},
	)
	return resolution
}

func (service mergeConflictResolutionService) resolveSemanticConflictRegion(ctx context.Context, client llm.ChatClient, options mergeConflictResolutionOptions, conflictFile mergeConflictFile, region mergeConflictRegion, regionIndex int, regionCount int, timeout time.Duration, initialCandidate string) (string, error) {
	attemptTimeout := mergeConflictResolutionSemanticAttemptTimeout(service.commitMessages, timeout)
	attemptErrors := make([]error, 0, mergeConflictResolutionMaxSemanticAttempts)
	candidate := initialCandidate
	feedback := ""

	for attempt := 1; attempt <= mergeConflictResolutionMaxSemanticAttempts; attempt++ {
		reviewing := candidate != ""
		var request llm.ChatRequest
		strategy := "semantic candidate"
		if reviewing {
			strategy = "semantic audit"
			request = service.buildRegionReviewRequest(
				options,
				conflictFile,
				region,
				regionIndex,
				regionCount,
				candidate,
				feedback,
			)
		} else {
			request = service.buildRegionResolutionRequest(
				options,
				conflictFile,
				region,
				regionIndex,
				regionCount,
				feedback,
			)
		}

		subject := fmt.Sprintf(
			"%s conflict region %d/%d %s attempt %d/%d",
			conflictFile.Path,
			regionIndex+1,
			regionCount,
			strategy,
			attempt,
			mergeConflictResolutionMaxSemanticAttempts,
		)
		response, responseErr := service.requestMergeConflictResolution(
			ctx,
			client,
			request,
			subject,
			conflictFile.Path,
			attemptTimeout,
		)
		if responseErr != nil {
			if ctx.Err() != nil {
				return "", responseErr
			}
			attemptErr := fmt.Errorf("%s attempt %d: %w", strategy, attempt, responseErr)
			attemptErrors = append(attemptErrors, attemptErr)
			feedback = attemptErr.Error()
			service.reportSemanticAttemptRejected(conflictFile.Path, regionIndex, regionCount, attempt, strategy, feedback, candidate != "")
			continue
		}

		if reviewing && strings.TrimSpace(response) == mergeConflictResolutionReviewApproved {
			service.report(
				shared.EventLevelInfo,
				shared.EventCodeAIMergeValidation,
				fmt.Sprintf(
					"semantic audit approved %s conflict region %d/%d on attempt %d/%d",
					conflictFile.Path,
					regionIndex+1,
					regionCount,
					attempt,
					mergeConflictResolutionMaxSemanticAttempts,
				),
				map[string]string{
					"path":    conflictFile.Path,
					"region":  strconv.Itoa(regionIndex + 1),
					"attempt": strconv.Itoa(attempt),
				},
			)
			return candidate, nil
		}

		resolvedRegion, envelopeErr := mergeConflictResolutionContent(conflictFile.Path, response)
		if envelopeErr != nil {
			attemptErr := fmt.Errorf("%s attempt %d: %w", strategy, attempt, envelopeErr)
			attemptErrors = append(attemptErrors, attemptErr)
			feedback = attemptErr.Error()
			service.reportSemanticAttemptRejected(conflictFile.Path, regionIndex, regionCount, attempt, strategy, feedback, candidate != "")
			continue
		}
		if validationErr := validateMergeConflictRegionResponse(
			conflictFile.Path,
			regionIndex,
			region,
			resolvedRegion,
		); validationErr != nil {
			attemptErr := fmt.Errorf("%s attempt %d: %w", strategy, attempt, validationErr)
			attemptErrors = append(attemptErrors, attemptErr)
			feedback = attemptErr.Error()
			service.reportSemanticAttemptRejected(conflictFile.Path, regionIndex, regionCount, attempt, strategy, feedback, candidate != "")
			continue
		}

		candidate = resolvedRegion
		feedback = ""
		service.report(
			shared.EventLevelInfo,
			shared.EventCodeAIMergeValidation,
			fmt.Sprintf(
				"%s attempt %d/%d passed local validation for %s conflict region %d/%d; requesting semantic audit",
				strategy,
				attempt,
				mergeConflictResolutionMaxSemanticAttempts,
				conflictFile.Path,
				regionIndex+1,
				regionCount,
			),
			map[string]string{
				"path":    conflictFile.Path,
				"region":  strconv.Itoa(regionIndex + 1),
				"attempt": strconv.Itoa(attempt),
			},
		)
	}

	return "", fmt.Errorf(
		mergeConflictResolutionExhaustedTemplate,
		conflictFile.Path,
		regionIndex+1,
		mergeConflictResolutionMaxSemanticAttempts,
		errors.Join(attemptErrors...),
	)
}

func (service mergeConflictResolutionService) reportSemanticAttemptRejected(path string, regionIndex int, regionCount int, attempt int, strategy string, reason string, candidateAvailable bool) {
	nextAction := "requesting validation-guided repair"
	if candidateAvailable {
		nextAction = "retrying semantic audit"
	}
	if attempt == mergeConflictResolutionMaxSemanticAttempts {
		nextAction = "all semantic attempts exhausted"
	}
	service.report(
		shared.EventLevelWarn,
		shared.EventCodeAIMergeValidation,
		fmt.Sprintf(
			"%s attempt %d/%d rejected for %s conflict region %d/%d: %s; %s",
			strategy,
			attempt,
			mergeConflictResolutionMaxSemanticAttempts,
			path,
			regionIndex+1,
			regionCount,
			strings.ReplaceAll(strings.TrimSpace(reason), "\n", "; "),
			nextAction,
		),
		map[string]string{
			"path":     path,
			"region":   strconv.Itoa(regionIndex + 1),
			"attempt":  strconv.Itoa(attempt),
			"strategy": strategy,
			"reason":   reason,
		},
	)
}

func (service mergeConflictResolutionService) requestMergeConflictResolution(ctx context.Context, client llm.ChatClient, request llm.ChatRequest, subject string, path string, timeout time.Duration) (string, error) {
	requestContext, cancelRequest := context.WithTimeoutCause(ctx, timeout, errMergeConflictResolutionDeadline)
	defer cancelRequest()
	deadline := time.Now().Add(timeout)
	service.report(
		shared.EventLevelInfo,
		shared.EventCodeAIMergeResolution,
		fmt.Sprintf("resolving %s with AI (attempt deadline %s; rollback remains deferred until bounded strategies are exhausted)", subject, timeout),
		map[string]string{
			"path":      path,
			"timeout":   timeout.String(),
			"remaining": mergeConflictResolutionRemaining(deadline),
		},
	)
	stopProgress := service.startMergeConflictResolutionProgress(requestContext, subject, deadline)
	response, responseErr := client.Chat(requestContext, request)
	stopProgress()
	if responseErr != nil {
		if errors.Is(context.Cause(requestContext), errMergeConflictResolutionDeadline) {
			return "", fmt.Errorf(mergeConflictResolutionTimeoutTemplate+": %w", timeout, responseErr)
		}
		if ctx.Err() != nil {
			return "", fmt.Errorf(mergeConflictResolutionCanceledMessage+": %w", responseErr)
		}
		return "", responseErr
	}
	service.report(
		shared.EventLevelInfo,
		shared.EventCodeAIMergeValidation,
		fmt.Sprintf("validating AI resolution for %s", subject),
		map[string]string{"path": path},
	)
	resolvedContent := strings.TrimSpace(response)
	if resolvedContent == "" {
		return "", fmt.Errorf(mergeConflictResolutionEmptyResponse, path)
	}
	if containsConflictMarker(resolvedContent) {
		return "", fmt.Errorf(mergeConflictResolutionConflictMarkers, path)
	}
	return response, nil
}

func (service mergeConflictResolutionService) startMergeConflictResolutionProgress(ctx context.Context, path string, deadline time.Time) func() {
	if service.reporter == nil {
		return func() {}
	}

	remaining := time.Until(deadline)
	interval := mergeConflictResolutionProgressInterval(remaining)
	if interval <= 0 {
		return func() {}
	}

	startedAt := time.Now()
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				remainingDuration := time.Until(deadline)
				if remainingDuration <= 0 {
					return
				}
				service.report(
					shared.EventLevelInfo,
					shared.EventCodeAIMergeResolution,
					fmt.Sprintf("still resolving %s with AI (%s elapsed; %s remaining)", path, mergeConflictResolutionElapsed(startedAt), mergeConflictResolutionRemaining(deadline)),
					map[string]string{
						"path":      path,
						"elapsed":   mergeConflictResolutionElapsed(startedAt),
						"remaining": mergeConflictResolutionRemaining(deadline),
					},
				)
			}
		}
	}()

	return func() {
		close(done)
		<-stopped
	}
}

func mergeConflictResolutionProgressInterval(remaining time.Duration) time.Duration {
	if remaining <= 0 {
		return 0
	}
	interval := mergeConflictResolutionProgressMaximum
	if halfRemaining := remaining / 2; halfRemaining < interval {
		interval = halfRemaining
	}
	if interval < time.Second {
		interval = time.Second
	}
	return interval
}

func mergeConflictResolutionRemaining(deadline time.Time) string {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return "0s"
	}
	if remaining < time.Second {
		return "<1s"
	}
	return remaining.Round(time.Second).String()
}

func mergeConflictResolutionElapsed(startedAt time.Time) string {
	elapsed := time.Since(startedAt)
	if elapsed < time.Second {
		return "<1s"
	}
	return elapsed.Round(time.Second).String()
}

func mergeConflictResolutionSemanticAttemptTimeout(options worktreeAdoptionCommitMessageOptions, connectionTimeout time.Duration) time.Duration {
	if options.Client != nil {
		return connectionTimeout
	}
	connectionCount := 0
	if strings.TrimSpace(options.ConnectionProfiles.OpenAI.Credential) != "" {
		connectionCount++
	}
	if strings.TrimSpace(options.ConnectionProfiles.LLMProxy.Credential) != "" {
		connectionCount++
	}
	if connectionCount == 0 {
		connectionCount = 1
	}
	return time.Duration(connectionCount) * connectionTimeout
}

func (service mergeConflictResolutionService) buildRegionResolutionRequest(options mergeConflictResolutionOptions, conflictFile mergeConflictFile, region mergeConflictRegion, regionIndex int, regionCount int, feedback string) llm.ChatRequest {
	userPrompt := fmt.Sprintf(
		mergeConflictResolutionRegionUserPrompt,
		filepath.Base(filepath.Clean(service.repositoryPath)),
		conflictFile.Path,
		regionIndex+1,
		regionCount,
		strings.TrimSpace(options.TargetBranch),
		strings.TrimSpace(options.SourceReference),
		region.Base,
		region.Ours,
		region.Theirs,
	)
	if strings.TrimSpace(feedback) != "" {
		userPrompt += "\n\n" + fmt.Sprintf(mergeConflictResolutionRepairPrompt, feedback)
	}
	return llm.ChatRequest{
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: mergeConflictResolutionRegionSystemPrompt,
			},
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
		MaxTokens:   mergeConflictResolutionMaxTokens,
		Temperature: service.mergeConflictResolutionTemperature(),
	}
}

func (service mergeConflictResolutionService) buildRegionReviewRequest(options mergeConflictResolutionOptions, conflictFile mergeConflictFile, region mergeConflictRegion, regionIndex int, regionCount int, candidate string, feedback string) llm.ChatRequest {
	userPrompt := fmt.Sprintf(
		mergeConflictResolutionReviewUserPrompt,
		filepath.Base(filepath.Clean(service.repositoryPath)),
		conflictFile.Path,
		regionIndex+1,
		regionCount,
		strings.TrimSpace(options.TargetBranch),
		strings.TrimSpace(options.SourceReference),
		region.Base,
		region.Ours,
		region.Theirs,
		candidate,
	)
	if strings.TrimSpace(feedback) != "" {
		userPrompt += "\n\nA previous audit response was rejected:\n" + feedback
	}
	return llm.ChatRequest{
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: mergeConflictResolutionReviewSystemPrompt,
			},
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
		MaxTokens:   mergeConflictResolutionMaxTokens,
		Temperature: service.mergeConflictResolutionTemperature(),
	}
}

func (service mergeConflictResolutionService) mergeConflictResolutionTemperature() *float64 {
	if service.commitMessages.Temperature == 0 {
		return nil
	}
	temperature := service.commitMessages.Temperature
	return &temperature
}

func (service mergeConflictResolutionService) writeResolvedFile(path string, content string) error {
	resolvedPath, pathErr := mergeConflictResolutionFilesystemPath(service.repositoryPath, path)
	if pathErr != nil {
		return pathErr
	}
	if writeErr := os.WriteFile(resolvedPath, []byte(content), 0o644); writeErr != nil {
		return fmt.Errorf(mergeConflictResolutionWriteTemplate, path, writeErr)
	}
	return nil
}

func (service mergeConflictResolutionService) stageResolvedFile(ctx context.Context, path string) error {
	if stageErr := executeGit(ctx, service.executor, service.repositoryPath, []string{gitAddSubcommandConstant, gitPathspecSeparatorConstant, path}); stageErr != nil {
		return fmt.Errorf(mergeConflictResolutionStageTemplate, path, stageErr)
	}
	return nil
}

func (service mergeConflictResolutionService) stageDeletedFile(ctx context.Context, path string) error {
	if _, pathErr := mergeConflictResolutionFilesystemPath(service.repositoryPath, path); pathErr != nil {
		return pathErr
	}
	if deleteErr := executeGit(ctx, service.executor, service.repositoryPath, []string{gitRmSubcommandConstant, gitRmForceFlagConstant, gitPathspecSeparatorConstant, path}); deleteErr != nil {
		return fmt.Errorf(mergeConflictResolutionDeleteTemplate, path, deleteErr)
	}
	return nil
}

func mergeConflictResolutionFilesystemPath(repositoryPath string, path string) (string, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "." || cleanPath == string(filepath.Separator) || cleanPath == ".." || filepath.IsAbs(cleanPath) || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf(mergeConflictResolutionPathTemplate, path)
	}
	return filepath.Join(repositoryPath, cleanPath), nil
}

func containsConflictMarker(value string) bool {
	for _, line := range strings.Split(value, "\n") {
		if strings.HasPrefix(line, "<<<<<<<") || strings.HasPrefix(line, "=======") || strings.HasPrefix(line, ">>>>>>>") {
			return true
		}
	}
	return false
}

func mergeConflictResolutionContent(path string, response string) (string, error) {
	envelope := strings.TrimSpace(response)
	prefix := mergeConflictResolutionContentBegin + "\n"
	suffix := "\n" + mergeConflictResolutionContentEnd
	if !strings.HasPrefix(envelope, prefix) || !strings.HasSuffix(envelope, suffix) {
		return "", fmt.Errorf(mergeConflictResolutionEnvelopeTemplate, path)
	}
	content := strings.TrimSuffix(strings.TrimPrefix(envelope, prefix), suffix)
	if content == "" {
		return "", fmt.Errorf(mergeConflictResolutionEmptyResponse, path)
	}
	return content, nil
}

func validateMergeConflictRegionResponse(path string, regionIndex int, region mergeConflictRegion, response string) error {
	if region.BasePresent && region.Base == "" {
		oursThenTheirs := region.Ours + region.Theirs
		theirsThenOurs := region.Theirs + region.Ours
		if response != oursThenTheirs && response != theirsThenOurs {
			return fmt.Errorf(mergeConflictResolutionAdditiveTemplate, path, regionIndex+1)
		}
		return nil
	}
	if response == region.Base && (region.Ours != region.Base || region.Theirs != region.Base) {
		return fmt.Errorf(mergeConflictResolutionBaseOnlyTemplate, path, regionIndex+1)
	}
	missingOursIntent, oursIntentMissing, oursIntentAvailable := mergeConflictMissingReplacementIntent(region.Base, region.Ours, response)
	if !oursIntentAvailable {
		return fmt.Errorf(mergeConflictResolutionIntentTemplate, path, regionIndex+1, "OURS")
	}
	if oursIntentMissing {
		return fmt.Errorf(mergeConflictResolutionOursTemplate, path, regionIndex+1, missingOursIntent)
	}
	missingTheirsIntent, theirsIntentMissing, theirsIntentAvailable := mergeConflictMissingReplacementIntent(region.Base, region.Theirs, response)
	if !theirsIntentAvailable {
		return fmt.Errorf(mergeConflictResolutionIntentTemplate, path, regionIndex+1, "THEIRS")
	}
	if theirsIntentMissing {
		return fmt.Errorf(mergeConflictResolutionTheirsTemplate, path, regionIndex+1, missingTheirsIntent)
	}
	return nil
}

func parseMergeConflictDocument(content string) (mergeConflictDocument, error) {
	document := mergeConflictDocument{}
	var nonConflictingRegion strings.Builder
	var oursRegion strings.Builder
	var baseRegion strings.Builder
	var theirsRegion strings.Builder
	state := mergeConflictMarkerStateOutside
	basePresent := false

	for _, line := range mergeConflictLines(content) {
		switch state {
		case mergeConflictMarkerStateOutside:
			switch {
			case mergeConflictLineHasPrefix(line, "<<<<<<<"):
				document.NonConflictingRegions = append(document.NonConflictingRegions, nonConflictingRegion.String())
				nonConflictingRegion.Reset()
				oursRegion.Reset()
				baseRegion.Reset()
				theirsRegion.Reset()
				basePresent = false
				state = mergeConflictMarkerStateOurs
			case mergeConflictLineHasPrefix(line, "|||||||"), mergeConflictLineHasPrefix(line, "======="), mergeConflictLineHasPrefix(line, ">>>>>>>"):
				return mergeConflictDocument{}, errors.New("unexpected conflict marker outside conflict region")
			default:
				nonConflictingRegion.WriteString(line)
			}
		case mergeConflictMarkerStateOurs:
			switch {
			case mergeConflictLineHasPrefix(line, "|||||||"):
				basePresent = true
				state = mergeConflictMarkerStateBase
			case mergeConflictLineHasPrefix(line, "======="):
				state = mergeConflictMarkerStateTheirs
			case mergeConflictLineHasPrefix(line, "<<<<<<<"), mergeConflictLineHasPrefix(line, ">>>>>>>"):
				return mergeConflictDocument{}, errors.New("invalid ours conflict marker sequence")
			default:
				oursRegion.WriteString(line)
			}
		case mergeConflictMarkerStateBase:
			switch {
			case mergeConflictLineHasPrefix(line, "======="):
				state = mergeConflictMarkerStateTheirs
			case mergeConflictLineHasPrefix(line, "<<<<<<<"), mergeConflictLineHasPrefix(line, "|||||||"), mergeConflictLineHasPrefix(line, ">>>>>>>"):
				return mergeConflictDocument{}, errors.New("invalid base conflict marker sequence")
			default:
				baseRegion.WriteString(line)
			}
		case mergeConflictMarkerStateTheirs:
			switch {
			case mergeConflictLineHasPrefix(line, ">>>>>>>"):
				document.ConflictRegions = append(document.ConflictRegions, mergeConflictRegion{
					Ours:        oursRegion.String(),
					Base:        baseRegion.String(),
					BasePresent: basePresent,
					Theirs:      theirsRegion.String(),
				})
				state = mergeConflictMarkerStateOutside
			case mergeConflictLineHasPrefix(line, "<<<<<<<"), mergeConflictLineHasPrefix(line, "|||||||"), mergeConflictLineHasPrefix(line, "======="):
				return mergeConflictDocument{}, errors.New("invalid theirs conflict marker sequence")
			default:
				theirsRegion.WriteString(line)
			}
		}
	}

	if state != mergeConflictMarkerStateOutside {
		return mergeConflictDocument{}, errors.New("unterminated conflict marker sequence")
	}
	if len(document.ConflictRegions) == 0 {
		return mergeConflictDocument{}, nil
	}
	document.NonConflictingRegions = append(document.NonConflictingRegions, nonConflictingRegion.String())
	return document, nil
}

func (document mergeConflictDocument) resolve(resolvedRegions []string) string {
	var resolvedDocument strings.Builder
	for regionIndex := range document.ConflictRegions {
		resolvedDocument.WriteString(document.NonConflictingRegions[regionIndex])
		resolvedDocument.WriteString(resolvedRegions[regionIndex])
	}
	resolvedDocument.WriteString(document.NonConflictingRegions[len(document.NonConflictingRegions)-1])
	return resolvedDocument.String()
}

func mergeConflictLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := make([]string, 0, strings.Count(content, "\n")+1)
	lineStart := 0
	for characterIndex := range content {
		if content[characterIndex] != '\n' {
			continue
		}
		lines = append(lines, content[lineStart:characterIndex+1])
		lineStart = characterIndex + 1
	}
	if lineStart < len(content) {
		lines = append(lines, content[lineStart:])
	}
	return lines
}

func mergeConflictLineHasPrefix(line string, prefix string) bool {
	lineWithoutTerminator := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	return strings.HasPrefix(lineWithoutTerminator, prefix)
}
