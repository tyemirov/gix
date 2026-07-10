package syncflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/utils/llm"
)

const (
	gitDiffNameOnlyFlagConstant                 = "--name-only"
	gitDiffFilterUnmergedFlagConstant           = "--diff-filter=U"
	gitLsFilesSubcommandConstant                = "ls-files"
	gitLsFilesUnmergedFlagConstant              = "-u"
	gitShowSubcommandConstant                   = "show"
	gitRmSubcommandConstant                     = "rm"
	gitRmForceFlagConstant                      = "-f"
	gitCommitNoEditFlagConstant                 = "--no-edit"
	mergeConflictResolutionMaxTokens            = 8192
	mergeConflictResolutionFailureTemplate      = "failed to resolve merge conflicts with AI: %w"
	mergeConflictResolutionInspectFailure       = "inspect unmerged files: %w"
	mergeConflictResolutionStageInspectTemplate = "inspect conflict stages for %s: %w"
	mergeConflictResolutionStageReadTemplate    = "read %s stage %d: %w"
	mergeConflictResolutionWorktreeReadTemplate = "read conflicted worktree file %s: %w"
	mergeConflictResolutionEmptyResponse        = "llm returned an empty merge resolution for %s"
	mergeConflictResolutionConflictMarkers      = "llm left conflict markers in merge resolution for %s"
	mergeConflictResolutionPreservationTemplate = "llm merge resolution for %s does not preserve non-conflicting content"
	mergeConflictResolutionStructureTemplate    = "conflicted worktree file %s has invalid conflict marker structure"
	mergeConflictResolutionWriteTemplate        = "write resolved merge file %s: %w"
	mergeConflictResolutionStageTemplate        = "stage resolved merge file %s: %w"
	mergeConflictResolutionDeleteTemplate       = "stage deleted merge file %s: %w"
	mergeConflictResolutionCommitTemplate       = "complete resolved merge commit: %w"
	mergeConflictResolutionPathTemplate         = "invalid conflicted path %q"
	mergeConflictResolutionDeleteDirective      = "GIX_MERGE_RESOLUTION_DELETE_FILE"
	mergeConflictResolutionSystemPrompt         = "You are an expert merge engineer resolving Git conflicts. Return only the complete final file contents. If the correct resolution is to delete this path, return exactly " + mergeConflictResolutionDeleteDirective + ". Preserve every intentional local OURS change while integrating compatible remote THEIRS changes. Do not drop local changes to make the merge easier. Remove conflict markers. Do not include explanations, markdown fences, or quotes."
	mergeConflictResolutionUserPrompt           = "Repository: %s\nPath: %s\nTarget branch: %s\nMerged reference: %s\n\nBASE common ancestor:\n%s\n\nOURS current branch with local work that must be preserved:\n%s\n\nTHEIRS incoming branch to integrate:\n%s\n\nReturn only the resolved final contents for this path, or " + mergeConflictResolutionDeleteDirective + " if the path should be deleted."
	mergeConflictResolutionAbsentStage          = "(file absent in this stage)"
)

type mergeConflictResolutionService struct {
	executor       shared.GitExecutor
	repositoryPath string
	commitMessages worktreeAdoptionCommitMessageOptions
}

type mergeConflictResolutionOptions struct {
	SourceReference string
	TargetBranch    string
}

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

type mergeConflictMarkerState uint8

const (
	mergeConflictMarkerStateOutside mergeConflictMarkerState = iota
	mergeConflictMarkerStateOurs
	mergeConflictMarkerStateBase
	mergeConflictMarkerStateTheirs
)

func resolveMergeConflictOrError(ctx context.Context, executor shared.GitExecutor, repositoryPath string, sourceReference string, targetBranch string, conflictMessage string, commitMessages worktreeAdoptionCommitMessageOptions, mergeErr error) error {
	service := mergeConflictResolutionService{
		executor:       executor,
		repositoryPath: repositoryPath,
		commitMessages: commitMessages,
	}
	resolved, resolveErr := service.Resolve(ctx, mergeConflictResolutionOptions{
		SourceReference: sourceReference,
		TargetBranch:    targetBranch,
	})
	if resolveErr != nil {
		return fmt.Errorf("%s: %w", conflictMessage, errors.Join(fmt.Errorf(mergeConflictResolutionFailureTemplate, resolveErr), mergeErr))
	}
	if !resolved {
		return fmt.Errorf("%s: %w", conflictMessage, mergeErr)
	}
	return nil
}

func (service mergeConflictResolutionService) Resolve(ctx context.Context, options mergeConflictResolutionOptions) (bool, error) {
	paths, pathsErr := service.unmergedPaths(ctx)
	if pathsErr != nil {
		return false, fmt.Errorf(mergeConflictResolutionInspectFailure, pathsErr)
	}
	if len(paths) == 0 {
		return false, nil
	}

	client, clientErr := resolveCommitMessageClient(service.commitMessages)
	if clientErr != nil {
		return true, clientErr
	}

	for pathIndex := range paths {
		conflictFile, conflictFileErr := service.collectConflictFile(ctx, paths[pathIndex])
		if conflictFileErr != nil {
			return true, conflictFileErr
		}
		resolution, resolutionErr := service.resolveConflictFile(ctx, client, options, conflictFile)
		if resolutionErr != nil {
			return true, resolutionErr
		}
		if resolution.Delete {
			if deleteErr := service.stageDeletedFile(ctx, conflictFile.Path); deleteErr != nil {
				return true, deleteErr
			}
		} else {
			if writeErr := service.writeResolvedFile(conflictFile.Path, resolution.Content); writeErr != nil {
				return true, writeErr
			}
			if stageErr := service.stageResolvedFile(ctx, conflictFile.Path); stageErr != nil {
				return true, stageErr
			}
		}
	}

	remainingPaths, remainingErr := service.unmergedPaths(ctx)
	if remainingErr != nil {
		return true, fmt.Errorf(mergeConflictResolutionInspectFailure, remainingErr)
	}
	if len(remainingPaths) > 0 {
		return true, fmt.Errorf("unresolved merge conflicts remain: %s", strings.Join(remainingPaths, ", "))
	}

	if commitErr := executeGit(ctx, service.executor, service.repositoryPath, []string{gitCommitSubcommandConstant, gitCommitNoEditFlagConstant}); commitErr != nil {
		return true, fmt.Errorf(mergeConflictResolutionCommitTemplate, commitErr)
	}
	return true, nil
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

func (service mergeConflictResolutionService) resolveConflictFile(ctx context.Context, client llm.ChatClient, options mergeConflictResolutionOptions, conflictFile mergeConflictFile) (mergeConflictFileResolution, error) {
	request := service.buildResolutionRequest(options, conflictFile)
	response, responseErr := client.Chat(ctx, request)
	if responseErr != nil {
		return mergeConflictFileResolution{}, responseErr
	}
	resolvedContent := strings.TrimSpace(response)
	if resolvedContent == "" {
		return mergeConflictFileResolution{}, fmt.Errorf(mergeConflictResolutionEmptyResponse, conflictFile.Path)
	}
	resolution := mergeConflictFileResolution{Content: response}
	if resolvedContent == mergeConflictResolutionDeleteDirective {
		resolution = mergeConflictFileResolution{Delete: true}
	} else if containsConflictMarker(resolvedContent) {
		return mergeConflictFileResolution{}, fmt.Errorf(mergeConflictResolutionConflictMarkers, conflictFile.Path)
	}
	if preservationErr := validateMergeConflictResolutionPreservation(conflictFile, resolution); preservationErr != nil {
		return mergeConflictFileResolution{}, preservationErr
	}
	return resolution, nil
}

func (service mergeConflictResolutionService) buildResolutionRequest(options mergeConflictResolutionOptions, conflictFile mergeConflictFile) llm.ChatRequest {
	var temperature *float64
	if service.commitMessages.Temperature != 0 {
		temperatureValue := service.commitMessages.Temperature
		temperature = &temperatureValue
	}
	return llm.ChatRequest{
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: mergeConflictResolutionSystemPrompt,
			},
			{
				Role: "user",
				Content: fmt.Sprintf(
					mergeConflictResolutionUserPrompt,
					filepath.Base(filepath.Clean(service.repositoryPath)),
					conflictFile.Path,
					strings.TrimSpace(options.TargetBranch),
					strings.TrimSpace(options.SourceReference),
					conflictFile.Base,
					conflictFile.Ours,
					conflictFile.Theirs,
				),
			},
		},
		MaxTokens:   mergeConflictResolutionMaxTokens,
		Temperature: temperature,
	}
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

func validateMergeConflictResolutionPreservation(conflictFile mergeConflictFile, resolution mergeConflictFileResolution) error {
	nonConflictingRegions, conflictCount, parseErr := mergeConflictNonConflictingRegions(conflictFile.WorktreeContent)
	if parseErr != nil {
		return fmt.Errorf(mergeConflictResolutionStructureTemplate+": %w", conflictFile.Path, parseErr)
	}
	if conflictCount == 0 {
		return validateMarkerFreeMergeConflictResolution(conflictFile, resolution)
	}
	if !mergeConflictRegionsContainContent(nonConflictingRegions) {
		return nil
	}
	if resolution.Delete || !mergeConflictResolutionPreservesRegions(resolution.Content, nonConflictingRegions) {
		return fmt.Errorf(mergeConflictResolutionPreservationTemplate, conflictFile.Path)
	}
	return nil
}

func validateMarkerFreeMergeConflictResolution(conflictFile mergeConflictFile, resolution mergeConflictFileResolution) error {
	if !conflictFile.OursPresent {
		if resolution.Delete {
			return nil
		}
		return fmt.Errorf(mergeConflictResolutionPreservationTemplate, conflictFile.Path)
	}
	if resolution.Delete || resolution.Content != conflictFile.Ours {
		return fmt.Errorf(mergeConflictResolutionPreservationTemplate, conflictFile.Path)
	}
	return nil
}

func mergeConflictNonConflictingRegions(content string) ([]string, int, error) {
	regions := make([]string, 0)
	var currentRegion strings.Builder
	state := mergeConflictMarkerStateOutside
	conflictCount := 0

	for _, line := range mergeConflictLines(content) {
		switch state {
		case mergeConflictMarkerStateOutside:
			if mergeConflictLineHasPrefix(line, "<<<<<<<") {
				regions = append(regions, currentRegion.String())
				currentRegion.Reset()
				state = mergeConflictMarkerStateOurs
				conflictCount++
				continue
			}
			currentRegion.WriteString(line)
		case mergeConflictMarkerStateOurs:
			switch {
			case mergeConflictLineHasPrefix(line, "|||||||"):
				state = mergeConflictMarkerStateBase
			case mergeConflictLineHasPrefix(line, "======="):
				state = mergeConflictMarkerStateTheirs
			case mergeConflictLineHasPrefix(line, "<<<<<<<"), mergeConflictLineHasPrefix(line, ">>>>>>>"):
				return nil, 0, errors.New("invalid ours conflict marker sequence")
			}
		case mergeConflictMarkerStateBase:
			switch {
			case mergeConflictLineHasPrefix(line, "======="):
				state = mergeConflictMarkerStateTheirs
			case mergeConflictLineHasPrefix(line, "<<<<<<<"), mergeConflictLineHasPrefix(line, "|||||||"), mergeConflictLineHasPrefix(line, ">>>>>>>"):
				return nil, 0, errors.New("invalid base conflict marker sequence")
			}
		case mergeConflictMarkerStateTheirs:
			switch {
			case mergeConflictLineHasPrefix(line, ">>>>>>>"):
				state = mergeConflictMarkerStateOutside
			case mergeConflictLineHasPrefix(line, "<<<<<<<"):
				return nil, 0, errors.New("invalid theirs conflict marker sequence")
			}
		}
	}

	if state != mergeConflictMarkerStateOutside {
		return nil, 0, errors.New("unterminated conflict marker sequence")
	}
	if conflictCount == 0 {
		return nil, 0, nil
	}
	regions = append(regions, currentRegion.String())
	return regions, conflictCount, nil
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

func mergeConflictRegionsContainContent(regions []string) bool {
	for regionIndex := range regions {
		if regions[regionIndex] != "" {
			return true
		}
	}
	return false
}

func mergeConflictResolutionPreservesRegions(resolution string, regions []string) bool {
	if len(regions) == 0 {
		return true
	}
	cursor := 0
	prefix := regions[0]
	if prefix != "" {
		if !strings.HasPrefix(resolution, prefix) {
			return false
		}
		cursor = len(prefix)
	}

	lastRegionIndex := len(regions) - 1
	for regionIndex := 1; regionIndex < lastRegionIndex; regionIndex++ {
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

	suffix := regions[lastRegionIndex]
	if suffix == "" {
		return true
	}
	if !strings.HasSuffix(resolution, suffix) {
		return false
	}
	return len(resolution)-len(suffix) >= cursor
}
