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
	gitNullFlagConstant                         = "-z"
	gitLsFilesSubcommandConstant                = "ls-files"
	gitLsFilesUnmergedFlagConstant              = "-u"
	gitShowSubcommandConstant                   = "show"
	gitMergeFileSubcommandConstant              = "merge-file"
	gitMergeFileStdoutFlagConstant              = "--stdout"
	gitMergeFileDiff3FlagConstant               = "--diff3"
	gitMergeFileMarkerSizeFlagTemplate          = "--marker-size=%d"
	gitMergeFileLabelFlagConstant               = "-L"
	gitRestoreSubcommandConstant                = "restore"
	gitRestoreStagedFlagConstant                = "--staged"
	gitCommitNoEditFlagConstant                 = "--no-edit"
	mergeConflictResolutionMaxTokens            = 8192
	mergeConflictResolutionFailureTemplate      = "failed to resolve merge conflicts automatically: %w"
	mergeConflictResolutionInspectFailure       = "inspect unmerged files: %w"
	mergeConflictResolutionStageInspectTemplate = "inspect conflict stages for %s: %w"
	mergeConflictResolutionStageReadTemplate    = "read %s stage %d: %w"
	mergeConflictResolutionWorktreeReadTemplate = "read conflicted worktree file %s: %w"
	mergeConflictResolutionWriteTemplate        = "write resolved merge file %s: %w"
	mergeConflictResolutionStageTemplate        = "stage resolved merge file %s: %w"
	mergeConflictResolutionCommitTemplate       = "complete resolved merge commit: %w"
	mergeConflictResolutionPathTemplate         = "invalid conflicted path %q"
	mergeConflictResolutionTimeoutTemplate      = "AI merge resolution timed out after %s"
	mergeConflictResolutionCanceledMessage      = "AI merge resolution was canceled"
	mergeConflictResolutionHandoffTemplate      = "AI merge resolution stopped after: %s. gix did not push. Inspect git status, then resolve and commit the merge, or run git merge --abort."
	stashConflictResolutionHandoffTemplate      = "AI stash restoration stopped after: %s. The stash remains. Inspect git status before retrying sync."
	mergeConflictResolutionSystemPrompt         = "You resolve one bounded Git conflict hunk. Return exactly one JSON object with string fields hunk_id and content. Preserve the target intent while integrating the incoming intent. content replaces only this hunk; never reproduce the complete file. Do not return markdown, prose, conflict markers, or additional fields."
	stashConflictResolutionSystemPrompt         = "You resolve one bounded Git stash-restoration conflict hunk. Return exactly one JSON object with string fields hunk_id and content. Preserve both the TARGET branch intent and the STASHED operator intent byte-for-byte wherever supplied. content replaces only this hunk; never reproduce the complete file. Do not return markdown, prose, conflict markers, or additional fields."
	mergeConflictResolutionUserPrompt           = "Repository: %s\nPath: %s\nTarget branch: %s\nIncoming reference: %s\nHunk ID: %s\n\nCONTEXT BEFORE (read-only):\n%s\n\nBASE:\n%s\n\nTARGET:\n%s\n\nINCOMING:\n%s\n\nCONTEXT AFTER (read-only):\n%s\n\nReturn {\"hunk_id\":\"%s\",\"content\":\"...\"}."
	mergeConflictResolutionRepairPrompt         = "The prior hunk response was rejected: %s\nReturn a corrected JSON object for hunk %s. Do not change the hunk id or return any other text."
	mergeConflictResolutionProgressMaximum      = 10 * time.Second
	mergeConflictResolutionAttemptLimit         = 2
)

var errMergeConflictResolutionDeadline = errors.New("AI merge resolution deadline exceeded")

type mergeConflictResolutionService struct {
	executor       shared.GitExecutor
	repositoryPath string
	commitMessages worktreeAdoptionCommitMessageOptions
	reporter       mergeConflictResolutionReporter
}

type mergeConflictResolutionReporter func(level shared.EventLevel, code string, message string, details map[string]string)

type mergeConflictResolutionOptions struct {
	SourceReference string
	TargetBranch    string
	Mode            mergeConflictResolutionMode
}

type mergeConflictResolutionMode uint8

const (
	mergeConflictResolutionModeMerge mergeConflictResolutionMode = iota
	mergeConflictResolutionModeStash
)

func (options mergeConflictResolutionOptions) conflictEventCode() string {
	if options.Mode == mergeConflictResolutionModeStash {
		return shared.EventCodeStashConflict
	}
	return shared.EventCodeMergeConflict
}

func (options mergeConflictResolutionOptions) resolutionEventCode() string {
	if options.Mode == mergeConflictResolutionModeStash {
		return shared.EventCodeAIStashResolution
	}
	return shared.EventCodeAIMergeResolution
}

func (options mergeConflictResolutionOptions) validationEventCode() string {
	if options.Mode == mergeConflictResolutionModeStash {
		return shared.EventCodeAIStashValidation
	}
	return shared.EventCodeAIMergeValidation
}

func (options mergeConflictResolutionOptions) handoffEventCode() string {
	if options.Mode == mergeConflictResolutionModeStash {
		return shared.EventCodeAIStashHandoff
	}
	return shared.EventCodeAIMergeHandoff
}

type mergeConflictFile struct {
	Path     string
	Base     mergeConflictStage
	Target   mergeConflictStage
	Incoming mergeConflictStage
	Snapshot mergeConflictWorktreeSnapshot
}

type mergeConflictFileResolution struct {
	Path        string
	Delete      bool
	Content     string
	Permissions os.FileMode
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
		if conflictObserved {
			service.reportMergeConflictHandoff(resolveErr, sourceReference, targetBranch)
		}
		return fmt.Errorf("%s: %w", conflictMessage, errors.Join(fmt.Errorf(mergeConflictResolutionFailureTemplate, resolveErr), mergeErr))
	}
	if !conflictObserved {
		return fmt.Errorf("%s: %w", conflictMessage, mergeErr)
	}
	return nil
}

func (service mergeConflictResolutionService) Resolve(ctx context.Context, options mergeConflictResolutionOptions) (bool, error) {
	timeout := worktreeAdoptionMessageTimeout(service.commitMessages)
	resolutionContext, cancel := context.WithTimeoutCause(ctx, timeout, errMergeConflictResolutionDeadline)
	defer cancel()
	deadline := time.Now().Add(timeout)

	paths, pathsErr := service.unmergedPaths(resolutionContext)
	if pathsErr != nil {
		return false, service.normalizeResolutionError(resolutionContext, timeout, fmt.Errorf(mergeConflictResolutionInspectFailure, pathsErr))
	}
	if len(paths) == 0 {
		return false, nil
	}
	service.reportConflictDetected(paths, options, timeout)

	conflictFiles := make([]mergeConflictFile, 0, len(paths))
	for pathIndex := range paths {
		conflictFile, conflictFileErr := service.collectConflictFile(resolutionContext, paths[pathIndex])
		if conflictFileErr != nil {
			return true, service.normalizeResolutionError(resolutionContext, timeout, conflictFileErr)
		}
		conflictFiles = append(conflictFiles, conflictFile)
	}

	var client llm.ChatClient
	resolutions := make([]mergeConflictFileResolution, 0, len(conflictFiles))
	for conflictFileIndex := range conflictFiles {
		resolution, resolvedClient, resolutionErr := service.prepareConflictFileResolution(
			resolutionContext,
			client,
			options,
			conflictFiles[conflictFileIndex],
			deadline,
			timeout,
		)
		if resolutionErr != nil {
			return true, service.normalizeResolutionError(resolutionContext, timeout, resolutionErr)
		}
		client = resolvedClient
		resolutions = append(resolutions, resolution)
	}

	service.report(shared.EventLevelInfo, options.validationEventCode(), "all conflict resolutions validated; applying one worktree transaction", map[string]string{
		"paths": strings.Join(paths, ", "),
	})
	if applyErr := service.applyConflictFileResolutions(resolutionContext, conflictFiles, resolutions); applyErr != nil {
		return true, service.normalizeResolutionError(resolutionContext, timeout, applyErr)
	}

	remainingPaths, remainingErr := service.unmergedPaths(resolutionContext)
	if remainingErr != nil {
		return true, service.normalizeResolutionError(resolutionContext, timeout, fmt.Errorf(mergeConflictResolutionInspectFailure, remainingErr))
	}
	if len(remainingPaths) > 0 {
		return true, fmt.Errorf("unresolved merge conflicts remain: %s", strings.Join(remainingPaths, ", "))
	}

	if options.Mode == mergeConflictResolutionModeStash {
		service.report(shared.EventLevelInfo, options.resolutionEventCode(), "all stash resolutions applied; restoring uncommitted work", map[string]string{
			"paths": strings.Join(paths, ", "),
		})
		if unstageErr := service.unstageResolvedFiles(resolutionContext, paths); unstageErr != nil {
			return true, service.normalizeResolutionError(resolutionContext, timeout, unstageErr)
		}
		service.report(shared.EventLevelInfo, options.resolutionEventCode(), "stash conflict resolution completed", map[string]string{
			"paths": strings.Join(paths, ", "),
		})
		return true, nil
	}

	service.report(shared.EventLevelInfo, options.resolutionEventCode(), "all merge resolutions applied; completing merge commit", map[string]string{
		"paths": strings.Join(paths, ", "),
	})
	if commitErr := executeGit(resolutionContext, service.executor, service.repositoryPath, []string{gitCommitSubcommandConstant, gitCommitNoEditFlagConstant}); commitErr != nil {
		return true, service.normalizeResolutionError(resolutionContext, timeout, fmt.Errorf(mergeConflictResolutionCommitTemplate, commitErr))
	}
	service.report(shared.EventLevelInfo, options.resolutionEventCode(), "merge conflict resolution completed", map[string]string{
		"paths": strings.Join(paths, ", "),
	})
	return true, nil
}

func (service mergeConflictResolutionService) normalizeResolutionError(ctx context.Context, timeout time.Duration, resolutionErr error) error {
	if errors.Is(context.Cause(ctx), errMergeConflictResolutionDeadline) {
		return fmt.Errorf(mergeConflictResolutionTimeoutTemplate+": %w", timeout, resolutionErr)
	}
	if errors.Is(resolutionErr, context.DeadlineExceeded) {
		return fmt.Errorf("AI merge resolution request timed out: %w", resolutionErr)
	}
	if errors.Is(context.Cause(ctx), context.Canceled) || errors.Is(resolutionErr, context.Canceled) {
		return fmt.Errorf(mergeConflictResolutionCanceledMessage+": %w", resolutionErr)
	}
	return resolutionErr
}

func (service mergeConflictResolutionService) reportConflictDetected(paths []string, options mergeConflictResolutionOptions, timeout time.Duration) {
	pathNoun := "paths"
	if len(paths) == 1 {
		pathNoun = "path"
	}
	message := fmt.Sprintf("detected %d conflicted %s while merging %s into %s; resolving automatically", len(paths), pathNoun, strings.TrimSpace(options.SourceReference), strings.TrimSpace(options.TargetBranch))
	if options.Mode == mergeConflictResolutionModeStash {
		message = fmt.Sprintf("detected %d conflicted %s while restoring %s onto %s; resolving automatically", len(paths), pathNoun, strings.TrimSpace(options.SourceReference), strings.TrimSpace(options.TargetBranch))
	}
	service.report(
		shared.EventLevelInfo,
		options.conflictEventCode(),
		message,
		map[string]string{
			"paths":            strings.Join(paths, ", "),
			"source_reference": strings.TrimSpace(options.SourceReference),
			"target_branch":    strings.TrimSpace(options.TargetBranch),
			"timeout":          timeout.String(),
		},
	)
}

func (service mergeConflictResolutionService) reportMergeConflictHandoff(resolutionErr error, sourceReference string, targetBranch string) {
	service.reportConflictHandoff(resolutionErr, mergeConflictResolutionOptions{
		SourceReference: sourceReference,
		TargetBranch:    targetBranch,
	})
}

func (service mergeConflictResolutionService) reportConflictHandoff(resolutionErr error, options mergeConflictResolutionOptions) {
	reason := strings.ReplaceAll(strings.TrimSpace(resolutionErr.Error()), "\n", "; ")
	message := fmt.Sprintf(mergeConflictResolutionHandoffTemplate, reason)
	if options.Mode == mergeConflictResolutionModeStash {
		message = fmt.Sprintf(stashConflictResolutionHandoffTemplate, reason)
	}
	service.report(
		shared.EventLevelError,
		options.handoffEventCode(),
		message,
		map[string]string{
			"source_reference": strings.TrimSpace(options.SourceReference),
			"target_branch":    strings.TrimSpace(options.TargetBranch),
			"reason":           reason,
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
		Arguments:        []string{gitDiffSubcommandConstant, gitDiffNameOnlyFlagConstant, gitNullFlagConstant, gitDiffFilterUnmergedFlagConstant},
		WorkingDirectory: service.repositoryPath,
	})
	if executionErr != nil {
		return nil, executionErr
	}

	paths := make([]string, 0)
	seenPaths := map[string]struct{}{}
	for _, path := range strings.Split(result.StandardOutput, "\x00") {
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

	if populateErr := service.populateConflictStageContents(ctx, path, stages); populateErr != nil {
		return mergeConflictFile{}, populateErr
	}
	snapshot, snapshotErr := service.conflictedWorktreeSnapshot(path)
	if snapshotErr != nil {
		return mergeConflictFile{}, snapshotErr
	}
	conflictFile := mergeConflictFile{
		Path:     path,
		Base:     stages[1],
		Target:   stages[2],
		Incoming: stages[3],
		Snapshot: snapshot,
	}
	if validationErr := validateMergeConflictFile(conflictFile); validationErr != nil {
		return mergeConflictFile{}, validationErr
	}
	return conflictFile, nil
}

func (service mergeConflictResolutionService) populateConflictStageContents(ctx context.Context, path string, stages map[int]mergeConflictStage) error {
	for _, stageNumber := range []int{1, 2, 3} {
		stage, exists := stages[stageNumber]
		if !exists {
			continue
		}
		content, contentErr := service.conflictStageContent(ctx, path, stageNumber)
		if contentErr != nil {
			return contentErr
		}
		stage.Content = content
		stages[stageNumber] = stage
	}
	return nil
}

func (service mergeConflictResolutionService) conflictedWorktreeSnapshot(path string) (mergeConflictWorktreeSnapshot, error) {
	worktreePath, pathErr := mergeConflictResolutionFilesystemPath(service.repositoryPath, path)
	if pathErr != nil {
		return mergeConflictWorktreeSnapshot{}, pathErr
	}
	fileInfo, statErr := os.Lstat(worktreePath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return mergeConflictWorktreeSnapshot{}, nil
		}
		return mergeConflictWorktreeSnapshot{}, fmt.Errorf(mergeConflictResolutionWorktreeReadTemplate, path, statErr)
	}
	if !fileInfo.Mode().IsRegular() {
		return mergeConflictWorktreeSnapshot{}, fmt.Errorf("conflicted worktree path %s is not a regular file", path)
	}
	content, readErr := os.ReadFile(worktreePath)
	if readErr != nil {
		return mergeConflictWorktreeSnapshot{}, fmt.Errorf(mergeConflictResolutionWorktreeReadTemplate, path, readErr)
	}
	return mergeConflictWorktreeSnapshot{
		Content:     string(content),
		Permissions: fileInfo.Mode().Perm(),
		Present:     true,
	}, nil
}

func (service mergeConflictResolutionService) conflictStages(ctx context.Context, path string) (map[int]mergeConflictStage, error) {
	result, executionErr := service.executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitLsFilesSubcommandConstant, gitLsFilesUnmergedFlagConstant, gitNullFlagConstant, gitPathspecSeparatorConstant, path},
		WorkingDirectory: service.repositoryPath,
	})
	if executionErr != nil {
		return nil, fmt.Errorf(mergeConflictResolutionStageInspectTemplate, path, executionErr)
	}

	stages := map[int]mergeConflictStage{}
	for _, entry := range strings.Split(result.StandardOutput, "\x00") {
		if entry == "" {
			continue
		}
		metadata, returnedPath, found := strings.Cut(entry, "\t")
		if !found {
			return nil, fmt.Errorf("parse conflict stage metadata for %s", path)
		}
		if returnedPath != path {
			return nil, fmt.Errorf("parse conflict stage metadata for %s: unexpected path %q", path, returnedPath)
		}
		fields := strings.Fields(metadata)
		if len(fields) != 3 {
			return nil, fmt.Errorf("parse conflict stage metadata for %s", path)
		}
		stageNumber, stageErr := strconv.Atoi(fields[2])
		if stageErr != nil {
			return nil, fmt.Errorf("parse conflict stage for %s: %w", path, stageErr)
		}
		if stageNumber < 1 || stageNumber > 3 {
			return nil, fmt.Errorf("parse conflict stage for %s: unsupported stage %d", path, stageNumber)
		}
		if _, exists := stages[stageNumber]; exists {
			return nil, fmt.Errorf("parse conflict stage for %s: duplicate stage %d", path, stageNumber)
		}
		stages[stageNumber] = mergeConflictStage{
			Mode:     fields[0],
			ObjectID: fields[1],
			Present:  true,
		}
	}
	if len(stages) < 2 {
		return nil, fmt.Errorf("inspect conflict stages for %s: expected at least two stages", path)
	}
	return stages, nil
}

func (service mergeConflictResolutionService) conflictStageContent(ctx context.Context, path string, stage int) (string, error) {
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

func validateMergeConflictFile(conflictFile mergeConflictFile) error {
	stages := []struct {
		Name  string
		Stage mergeConflictStage
	}{
		{Name: "base", Stage: conflictFile.Base},
		{Name: "target", Stage: conflictFile.Target},
		{Name: "incoming", Stage: conflictFile.Incoming},
	}
	for stageIndex := range stages {
		if !stages[stageIndex].Stage.Present {
			continue
		}
		if _, permissionsErr := mergeConflictRegularFilePermissions(stages[stageIndex].Stage.Mode); permissionsErr != nil {
			return fmt.Errorf("%s stage for %s: %w", stages[stageIndex].Name, conflictFile.Path, permissionsErr)
		}
		if strings.IndexByte(stages[stageIndex].Stage.Content, 0) >= 0 {
			return fmt.Errorf("%s stage for %s is binary and cannot be resolved as text", stages[stageIndex].Name, conflictFile.Path)
		}
	}
	return nil
}

func mergeConflictRegularFilePermissions(mode string) (os.FileMode, error) {
	switch mode {
	case "100644":
		return 0o644, nil
	case "100755":
		return 0o755, nil
	default:
		return 0, fmt.Errorf("unsupported Git object mode %q", mode)
	}
}

func mergeConflictResolvedPermissions(options mergeConflictResolutionOptions, conflictFile mergeConflictFile) (os.FileMode, error) {
	baseMode := conflictFile.Base.Mode
	targetMode := conflictFile.Target.Mode
	incomingMode := conflictFile.Incoming.Mode
	selectedMode := targetMode
	if options.Mode == mergeConflictResolutionModeStash {
		if conflictFile.Incoming.Present && incomingMode != baseMode {
			selectedMode = incomingMode
		} else if !conflictFile.Target.Present {
			selectedMode = incomingMode
		}
	} else if conflictFile.Target.Present && targetMode == baseMode && conflictFile.Incoming.Present {
		selectedMode = incomingMode
	}
	if selectedMode == "" {
		return 0, fmt.Errorf("resolve file mode for %s: no surviving regular-file stage", conflictFile.Path)
	}
	return mergeConflictRegularFilePermissions(selectedMode)
}

func (service mergeConflictResolutionService) prepareConflictFileResolution(
	ctx context.Context,
	client llm.ChatClient,
	options mergeConflictResolutionOptions,
	conflictFile mergeConflictFile,
	deadline time.Time,
	timeout time.Duration,
) (mergeConflictFileResolution, llm.ChatClient, error) {
	if !conflictFile.Target.Present || !conflictFile.Incoming.Present {
		resolution, resolutionErr := deterministicMarkerFreeConflictResolution(options, conflictFile)
		if resolutionErr != nil {
			return mergeConflictFileResolution{}, client, resolutionErr
		}
		service.report(shared.EventLevelInfo, options.resolutionEventCode(), fmt.Sprintf("resolved marker-free conflict for %s deterministically", conflictFile.Path), map[string]string{
			"path":     conflictFile.Path,
			"strategy": "deterministic",
		})
		return resolution, client, nil
	}

	diff3Content, diff3Err := service.mergeConflictDiff3(ctx, conflictFile)
	if diff3Err != nil {
		return mergeConflictFileResolution{}, client, diff3Err
	}
	plan, planErr := newMergeConflictPlan(conflictFile.Path, diff3Content)
	if planErr != nil {
		return mergeConflictFileResolution{}, client, planErr
	}
	hunkResolutions := make(map[string]string, len(plan.Hunks))
	for hunkIndex := range plan.Hunks {
		hunk := plan.Hunks[hunkIndex]
		if deterministicResolution, deterministic := resolveDeterministicConflictHunk(hunk); deterministic {
			hunkResolutions[hunk.ID] = deterministicResolution
			service.report(shared.EventLevelInfo, options.resolutionEventCode(), fmt.Sprintf("resolved hunk %s in %s deterministically", hunk.ID, conflictFile.Path), map[string]string{
				"hunk_id":  hunk.ID,
				"path":     conflictFile.Path,
				"strategy": "deterministic",
			})
			continue
		}
		if client == nil {
			resolvedClient, clientErr := resolveCommitMessageClient(service.commitMessages)
			if clientErr != nil {
				return mergeConflictFileResolution{}, client, clientErr
			}
			client = resolvedClient
		}
		aiResolution, aiResolutionErr := service.resolveConflictHunk(ctx, client, options, conflictFile.Path, hunk, deadline, timeout)
		if aiResolutionErr != nil {
			return mergeConflictFileResolution{}, client, aiResolutionErr
		}
		hunkResolutions[hunk.ID] = aiResolution
	}
	content, renderErr := plan.render(hunkResolutions)
	if renderErr != nil {
		return mergeConflictFileResolution{}, client, renderErr
	}
	if containsConflictMarker(content) {
		return mergeConflictFileResolution{}, client, fmt.Errorf("compiled resolution for %s contains conflict markers", conflictFile.Path)
	}
	permissions, permissionsErr := mergeConflictResolvedPermissions(options, conflictFile)
	if permissionsErr != nil {
		return mergeConflictFileResolution{}, client, permissionsErr
	}
	return mergeConflictFileResolution{
		Path:        conflictFile.Path,
		Content:     content,
		Permissions: permissions,
	}, client, nil
}

func deterministicMarkerFreeConflictResolution(options mergeConflictResolutionOptions, conflictFile mergeConflictFile) (mergeConflictFileResolution, error) {
	selectedStage := conflictFile.Target
	if options.Mode == mergeConflictResolutionModeStash {
		selectedStage = conflictFile.Incoming
	}
	if !selectedStage.Present {
		return mergeConflictFileResolution{Path: conflictFile.Path, Delete: true}, nil
	}
	permissions, permissionsErr := mergeConflictResolvedPermissions(options, conflictFile)
	if permissionsErr != nil {
		return mergeConflictFileResolution{}, permissionsErr
	}
	return mergeConflictFileResolution{
		Path:        conflictFile.Path,
		Content:     selectedStage.Content,
		Permissions: permissions,
	}, nil
}

func (service mergeConflictResolutionService) mergeConflictDiff3(ctx context.Context, conflictFile mergeConflictFile) (content string, returnErr error) {
	temporaryDirectory, temporaryDirectoryErr := os.MkdirTemp("", "gix-conflict-plan-*")
	if temporaryDirectoryErr != nil {
		return "", fmt.Errorf("create conflict-plan workspace for %s: %w", conflictFile.Path, temporaryDirectoryErr)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(temporaryDirectory); cleanupErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("remove conflict-plan workspace for %s: %w", conflictFile.Path, cleanupErr))
		}
	}()

	targetPath := filepath.Join(temporaryDirectory, "target")
	basePath := filepath.Join(temporaryDirectory, "base")
	incomingPath := filepath.Join(temporaryDirectory, "incoming")
	stages := []struct {
		Path    string
		Content string
	}{
		{Path: targetPath, Content: conflictFile.Target.Content},
		{Path: basePath, Content: conflictFile.Base.Content},
		{Path: incomingPath, Content: conflictFile.Incoming.Content},
	}
	for stageIndex := range stages {
		if writeErr := os.WriteFile(stages[stageIndex].Path, []byte(stages[stageIndex].Content), 0o600); writeErr != nil {
			return "", fmt.Errorf("write conflict-plan stage for %s: %w", conflictFile.Path, writeErr)
		}
	}

	arguments := []string{
		gitMergeFileSubcommandConstant,
		gitMergeFileStdoutFlagConstant,
		gitMergeFileDiff3FlagConstant,
		fmt.Sprintf(gitMergeFileMarkerSizeFlagTemplate, mergeConflictPlanMarkerSize),
		gitMergeFileLabelFlagConstant,
		mergeConflictPlanTargetLabel,
		gitMergeFileLabelFlagConstant,
		mergeConflictPlanBaseLabel,
		gitMergeFileLabelFlagConstant,
		mergeConflictPlanIncomingLabel,
		targetPath,
		basePath,
		incomingPath,
	}
	result, executionErr := service.executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        arguments,
		WorkingDirectory: service.repositoryPath,
	})
	if executionErr == nil {
		return result.StandardOutput, nil
	}
	var commandFailure execshell.CommandFailedError
	if errors.As(executionErr, &commandFailure) &&
		commandFailure.Result.ExitCode > 0 &&
		commandFailure.Result.ExitCode <= 127 &&
		commandFailure.Result.StandardOutput != "" {
		return commandFailure.Result.StandardOutput, nil
	}
	return "", fmt.Errorf("compile diff3 conflict plan for %s: %w", conflictFile.Path, executionErr)
}

func (service mergeConflictResolutionService) resolveConflictHunk(
	ctx context.Context,
	client llm.ChatClient,
	options mergeConflictResolutionOptions,
	path string,
	hunk mergeConflictHunk,
	deadline time.Time,
	timeout time.Duration,
) (string, error) {
	var priorResponse string
	var validationErr error
	for attempt := 1; attempt <= mergeConflictResolutionAttemptLimit; attempt++ {
		request := service.buildHunkResolutionRequest(options, path, hunk, priorResponse, validationErr)
		progressMessage := fmt.Sprintf("resolving hunk %s in %s with AI (attempt %d/%d; deadline %s; Ctrl-C leaves the merge intact)", hunk.ID, path, attempt, mergeConflictResolutionAttemptLimit, timeout)
		if options.Mode == mergeConflictResolutionModeStash {
			progressMessage = fmt.Sprintf("resolving hunk %s in %s with AI (attempt %d/%d; deadline %s; Ctrl-C leaves the stash and conflict intact)", hunk.ID, path, attempt, mergeConflictResolutionAttemptLimit, timeout)
		}
		service.report(shared.EventLevelInfo, options.resolutionEventCode(), progressMessage, map[string]string{
			"attempt":   strconv.Itoa(attempt),
			"hunk_id":   hunk.ID,
			"path":      path,
			"remaining": mergeConflictResolutionRemaining(deadline),
			"strategy":  "ai",
			"timeout":   timeout.String(),
		})
		stopProgress := service.startMergeConflictResolutionProgress(ctx, options, path, deadline)
		response, responseErr := client.Chat(ctx, request)
		stopProgress()
		if responseErr != nil {
			return "", responseErr
		}
		service.report(shared.EventLevelInfo, options.validationEventCode(), fmt.Sprintf("validating AI resolution for hunk %s in %s", hunk.ID, path), map[string]string{
			"attempt": strconv.Itoa(attempt),
			"hunk_id": hunk.ID,
			"path":    path,
		})
		parsedResponse, parseErr := parseMergeConflictHunkResponse(response)
		if parseErr == nil {
			validatedContent, contentErr := validateMergeConflictHunkResponse(options, hunk, parsedResponse)
			if contentErr == nil {
				return validatedContent, nil
			}
			validationErr = contentErr
		} else {
			validationErr = parseErr
		}
		priorResponse = response
	}
	return "", fmt.Errorf("AI hunk resolution for %s in %s failed validation after %d attempts: %w", hunk.ID, path, mergeConflictResolutionAttemptLimit, validationErr)
}

func (service mergeConflictResolutionService) startMergeConflictResolutionProgress(ctx context.Context, options mergeConflictResolutionOptions, path string, deadline time.Time) func() {
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
					options.resolutionEventCode(),
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

func (service mergeConflictResolutionService) buildHunkResolutionRequest(
	options mergeConflictResolutionOptions,
	path string,
	hunk mergeConflictHunk,
	priorResponse string,
	validationErr error,
) llm.ChatRequest {
	var temperature *float64
	if service.commitMessages.Temperature != 0 {
		temperatureValue := service.commitMessages.Temperature
		temperature = &temperatureValue
	}
	systemPrompt := mergeConflictResolutionSystemPrompt
	if options.Mode == mergeConflictResolutionModeStash {
		systemPrompt = stashConflictResolutionSystemPrompt
	}
	messages := []llm.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role: "user",
			Content: fmt.Sprintf(
				mergeConflictResolutionUserPrompt,
				filepath.Base(filepath.Clean(service.repositoryPath)),
				path,
				strings.TrimSpace(options.TargetBranch),
				strings.TrimSpace(options.SourceReference),
				hunk.ID,
				hunk.ContextBefore,
				hunk.Base,
				hunk.Target,
				hunk.Incoming,
				hunk.ContextAfter,
				hunk.ID,
			),
		},
	}
	if validationErr != nil {
		messages = append(messages,
			llm.Message{Role: "assistant", Content: priorResponse},
			llm.Message{Role: "user", Content: fmt.Sprintf(mergeConflictResolutionRepairPrompt, validationErr.Error(), hunk.ID)},
		)
	}
	return llm.ChatRequest{
		Messages:    messages,
		MaxTokens:   mergeConflictResolutionMaxTokens,
		Temperature: temperature,
	}
}

func (service mergeConflictResolutionService) applyConflictFileResolutions(
	ctx context.Context,
	conflictFiles []mergeConflictFile,
	resolutions []mergeConflictFileResolution,
) error {
	if len(conflictFiles) != len(resolutions) {
		return errors.New("apply conflict resolutions: file and resolution counts differ")
	}
	paths := make([]string, 0, len(resolutions))
	for resolutionIndex := range resolutions {
		if conflictFiles[resolutionIndex].Path != resolutions[resolutionIndex].Path {
			return fmt.Errorf("apply conflict resolutions: path %q does not match %q", conflictFiles[resolutionIndex].Path, resolutions[resolutionIndex].Path)
		}
		if applyErr := service.applyConflictFileResolution(resolutions[resolutionIndex]); applyErr != nil {
			rollbackErr := service.restoreConflictWorktreeSnapshots(conflictFiles)
			return errors.Join(applyErr, rollbackErr)
		}
		paths = append(paths, resolutions[resolutionIndex].Path)
	}
	arguments := []string{gitAddSubcommandConstant, gitAddAllFlagConstant, gitPathspecSeparatorConstant}
	arguments = append(arguments, paths...)
	if stageErr := executeGit(ctx, service.executor, service.repositoryPath, arguments); stageErr != nil {
		rollbackErr := service.restoreConflictWorktreeSnapshots(conflictFiles)
		return errors.Join(fmt.Errorf(mergeConflictResolutionStageTemplate, strings.Join(paths, ", "), stageErr), rollbackErr)
	}
	return nil
}

func (service mergeConflictResolutionService) applyConflictFileResolution(resolution mergeConflictFileResolution) error {
	resolvedPath, pathErr := mergeConflictResolutionFilesystemPath(service.repositoryPath, resolution.Path)
	if pathErr != nil {
		return pathErr
	}
	if resolution.Delete {
		if removeErr := os.Remove(resolvedPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("delete resolved merge file %s: %w", resolution.Path, removeErr)
		}
		return nil
	}
	return service.writeResolvedFile(resolution.Path, resolution.Content, resolution.Permissions)
}

func (service mergeConflictResolutionService) writeResolvedFile(path string, content string, permissions os.FileMode) (returnErr error) {
	resolvedPath, pathErr := mergeConflictResolutionFilesystemPath(service.repositoryPath, path)
	if pathErr != nil {
		return pathErr
	}
	temporaryFile, temporaryFileErr := os.CreateTemp(filepath.Dir(resolvedPath), ".gix-conflict-resolution-*")
	if temporaryFileErr != nil {
		return fmt.Errorf(mergeConflictResolutionWriteTemplate, path, temporaryFileErr)
	}
	temporaryPath := temporaryFile.Name()
	defer func() {
		if cleanupErr := os.Remove(temporaryPath); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
			returnErr = errors.Join(returnErr, fmt.Errorf("remove temporary resolution for %s: %w", path, cleanupErr))
		}
	}()
	if chmodErr := temporaryFile.Chmod(permissions); chmodErr != nil {
		closeErr := temporaryFile.Close()
		return fmt.Errorf(mergeConflictResolutionWriteTemplate, path, errors.Join(chmodErr, closeErr))
	}
	if _, writeErr := temporaryFile.WriteString(content); writeErr != nil {
		closeErr := temporaryFile.Close()
		return fmt.Errorf(mergeConflictResolutionWriteTemplate, path, errors.Join(writeErr, closeErr))
	}
	if closeErr := temporaryFile.Close(); closeErr != nil {
		return fmt.Errorf(mergeConflictResolutionWriteTemplate, path, closeErr)
	}
	if renameErr := os.Rename(temporaryPath, resolvedPath); renameErr != nil {
		return fmt.Errorf(mergeConflictResolutionWriteTemplate, path, renameErr)
	}
	return nil
}

func (service mergeConflictResolutionService) restoreConflictWorktreeSnapshots(conflictFiles []mergeConflictFile) error {
	var restoreErr error
	for conflictFileIndex := range conflictFiles {
		conflictFile := conflictFiles[conflictFileIndex]
		resolvedPath, pathErr := mergeConflictResolutionFilesystemPath(service.repositoryPath, conflictFile.Path)
		if pathErr != nil {
			restoreErr = errors.Join(restoreErr, pathErr)
			continue
		}
		if !conflictFile.Snapshot.Present {
			if removeErr := os.Remove(resolvedPath); removeErr != nil && !os.IsNotExist(removeErr) {
				restoreErr = errors.Join(restoreErr, fmt.Errorf("restore absent conflict path %s: %w", conflictFile.Path, removeErr))
			}
			continue
		}
		if writeErr := service.writeResolvedFile(conflictFile.Path, conflictFile.Snapshot.Content, conflictFile.Snapshot.Permissions); writeErr != nil {
			restoreErr = errors.Join(restoreErr, fmt.Errorf("restore conflicted worktree file %s: %w", conflictFile.Path, writeErr))
		}
	}
	return restoreErr
}

func (service mergeConflictResolutionService) unstageResolvedFiles(ctx context.Context, paths []string) error {
	arguments := []string{gitRestoreSubcommandConstant, gitRestoreStagedFlagConstant, gitPathspecSeparatorConstant}
	arguments = append(arguments, paths...)
	if unstageErr := executeGit(ctx, service.executor, service.repositoryPath, arguments); unstageErr != nil {
		return fmt.Errorf("restore resolved stash paths to the worktree: %w", unstageErr)
	}
	return nil
}

func mergeConflictResolutionFilesystemPath(repositoryPath string, path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf(mergeConflictResolutionPathTemplate, path)
	}
	cleanPath := filepath.Clean(path)
	if cleanPath == "." || cleanPath == string(filepath.Separator) || cleanPath == ".." || filepath.IsAbs(cleanPath) || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf(mergeConflictResolutionPathTemplate, path)
	}
	return filepath.Join(repositoryPath, cleanPath), nil
}

func containsConflictMarker(value string) bool {
	for _, line := range strings.Split(value, "\n") {
		if strings.HasPrefix(line, "<<<<<<<") ||
			strings.HasPrefix(line, "|||||||") ||
			strings.HasPrefix(line, "=======") ||
			strings.HasPrefix(line, ">>>>>>>") {
			return true
		}
	}
	return false
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

func mergeConflictResolutionContainsRegions(resolution string, regions []string) bool {
	cursor := 0
	for regionIndex := range regions {
		region := regions[regionIndex]
		if region == "" {
			continue
		}
		regionOffset := strings.Index(resolution[cursor:], region)
		if regionOffset < 0 {
			return false
		}
		cursor += regionOffset + len(region)
	}
	return true
}
