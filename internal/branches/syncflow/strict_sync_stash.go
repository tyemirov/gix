package syncflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	gitStashApplySubcommandConstant        = "apply"
	gitStashDropSubcommandConstant         = "drop"
	gitStashListSubcommandConstant         = "list"
	gitStashIndexFlagConstant              = "--index"
	gitStashMessageFlagConstant            = "--message"
	gitStashFormatFlagConstant             = "--format=%H"
	gitStashReferenceConstant              = "refs/stash"
	strictSyncTransactionStashMessage      = "gix strict-sync transaction snapshot"
	strictSyncInvocationStashMessage       = "gix strict-sync invocation stash"
	strictSyncStashReferenceMissingMessage = "strict sync stash push did not create refs/stash"
	strictSyncStashApplyFailureTemplate    = "apply strict sync stash %s at %s: %w"
	strictSyncStashListFailureTemplate     = "list strict sync stashes at %s: %w"
	strictSyncStashMissingTemplate         = "strict sync stash %s is missing at %s"
	strictSyncStashDropFailureTemplate     = "drop strict sync stash %s at %s: %w"
	strictSyncStashPushFailureTemplate     = "create strict sync stash at %s: %w"
	strictSyncStashResolveFailureTemplate  = "resolve strict sync stash conflicts at %s: %w"
	strictSyncStashUnmergedStateMessage    = "strict sync stash apply failed without an unmerged index"
	gitDiffITAVisibleFlagConstant          = "--ita-visible-in-index"
	gitDiffAddedFlagConstant               = "--diff-filter=A"
	gitAddIntentFlagConstant               = "--intent-to-add"
	gitLiteralPathspecsEnvironmentName     = "GIT_LITERAL_PATHSPECS"
	strictSyncStashIndexFailureTemplate    = "prepare strict sync stash index at %s: %w"
	strictSyncStashIntentFailureTemplate   = "restore strict sync intent-to-add entries at %s: %w"
)

type strictSyncStash struct {
	CommitID    string
	Path        string
	IntentPaths []string
}

func pushStrictSyncStash(ctx context.Context, executor shared.GitExecutor, repositoryPath string, message string) (stash strictSyncStash, resultErr error) {
	intentPaths, intentErr := strictSyncIntentToAddPaths(ctx, executor, repositoryPath)
	if intentErr != nil {
		return strictSyncStash{}, fmt.Errorf(strictSyncStashIndexFailureTemplate, repositoryPath, intentErr)
	}
	var indexLock *strictSyncIndexLock
	var environment map[string]string
	if len(intentPaths) > 0 {
		var prepareErr error
		indexLock, prepareErr = prepareStrictSyncStashIndex(ctx, executor, repositoryPath, intentPaths)
		if prepareErr != nil {
			return strictSyncStash{}, fmt.Errorf(strictSyncStashIndexFailureTemplate, repositoryPath, prepareErr)
		}
		defer func() {
			if releaseErr := indexLock.release(); releaseErr != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf(strictSyncStashIndexFailureTemplate, repositoryPath, releaseErr))
			}
		}()
		environment = map[string]string{gitIndexFileEnvironmentNameConstant: indexLock.lockPath}
	}
	if pushErr := executeGitDetails(ctx, executor, execshell.CommandDetails{Arguments: []string{
		gitStashSubcommandConstant,
		gitStashPushSubcommandConstant,
		gitStashIncludeUntrackedFlagConstant,
		gitStashMessageFlagConstant,
		message,
	}, WorkingDirectory: repositoryPath, EnvironmentVariables: environment}); pushErr != nil {
		return strictSyncStash{}, fmt.Errorf(strictSyncStashPushFailureTemplate, repositoryPath, pushErr)
	}

	result, referenceErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments: []string{
			gitRevParseSubcommandConstant,
			gitVerifyFlagConstant,
			gitStashReferenceConstant,
		},
		WorkingDirectory: repositoryPath,
	})
	if referenceErr != nil {
		return strictSyncStash{}, fmt.Errorf(strictSyncStashPushFailureTemplate, repositoryPath, errors.Join(errors.New(strictSyncStashReferenceMissingMessage), referenceErr))
	}
	commitID := strings.TrimSpace(result.StandardOutput)
	if commitID == "" {
		return strictSyncStash{}, fmt.Errorf(strictSyncStashPushFailureTemplate, repositoryPath, errors.New(strictSyncStashReferenceMissingMessage))
	}
	stash = strictSyncStash{CommitID: commitID, Path: repositoryPath, IntentPaths: intentPaths}
	if indexLock != nil {
		if replaceErr := os.Rename(indexLock.lockPath, indexLock.indexPath); replaceErr != nil {
			return stash, fmt.Errorf(strictSyncStashIndexFailureTemplate, repositoryPath, replaceErr)
		}
	}
	return stash, nil
}

func strictSyncIntentToAddPaths(ctx context.Context, executor shared.GitExecutor, repositoryPath string) ([]string, error) {
	pathsByVisibility := make([][]string, 0, 2)
	for _, visibility := range []string{gitDiffITAVisibleFlagConstant, gitDiffITAInvisibleFlagConstant} {
		result, diffErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
			Arguments: []string{gitDiffSubcommandConstant, gitDiffCachedFlagConstant, gitDiffNameOnlyFlagConstant,
				gitDiffNoRenamesFlagConstant, gitDiffAddedFlagConstant, visibility, gitNullOutputFlagConstant, gitPathspecSeparatorConstant},
			WorkingDirectory: repositoryPath,
		})
		if diffErr != nil {
			return nil, diffErr
		}
		paths, parseErr := parseStrictSyncNULTerminatedPaths(result.StandardOutput)
		if parseErr != nil {
			return nil, parseErr
		}
		pathsByVisibility = append(pathsByVisibility, paths)
	}
	stagedPaths := make(map[string]struct{}, len(pathsByVisibility[1]))
	for _, path := range pathsByVisibility[1] {
		stagedPaths[path] = struct{}{}
	}
	var intentPaths []string
	for _, path := range pathsByVisibility[0] {
		if _, staged := stagedPaths[path]; !staged {
			intentPaths = append(intentPaths, path)
		}
	}
	return intentPaths, nil
}

func prepareStrictSyncStashIndex(ctx context.Context, executor shared.GitExecutor, repositoryPath string, intentPaths []string) (_ *strictSyncIndexLock, resultErr error) {
	result, pathErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments: []string{gitRevParseSubcommandConstant, gitPathFlagConstant, gitIndexPathNameConstant}, WorkingDirectory: repositoryPath,
	})
	if pathErr != nil {
		return nil, pathErr
	}
	indexPath, resolveErr := resolveStrictSyncIndexPath(repositoryPath, result.StandardOutput)
	if resolveErr != nil {
		return nil, resolveErr
	}
	indexLock, lockErr := acquireStrictSyncIndexLock(indexPath)
	if lockErr != nil {
		return nil, lockErr
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, indexLock.release())
		}
	}()
	if copyErr := indexLock.copyIndex(); copyErr != nil {
		return nil, copyErr
	}
	for _, intentPath := range intentPaths {
		if _, inspectErr := os.Lstat(filepath.Join(repositoryPath, intentPath)); inspectErr != nil {
			return nil, fmt.Errorf("inspect intent-to-add file %q: %w", intentPath, inspectErr)
		}
	}
	_, addErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments: append([]string{gitAddSubcommandConstant, gitAddForceFlagConstant, gitPathspecSeparatorConstant}, intentPaths...), WorkingDirectory: repositoryPath,
		EnvironmentVariables: map[string]string{
			gitIndexFileEnvironmentNameConstant: indexLock.lockPath,
			gitLiteralPathspecsEnvironmentName:  "1",
		},
	})
	if addErr != nil {
		return nil, addErr
	}
	return indexLock, nil
}

func applyStrictSyncStash(ctx context.Context, executor shared.GitExecutor, stash strictSyncStash) error {
	if applyErr := executeGit(ctx, executor, stash.Path, []string{
		gitStashSubcommandConstant,
		gitStashApplySubcommandConstant,
		gitStashIndexFlagConstant,
		stash.CommitID,
	}); applyErr != nil {
		return fmt.Errorf(strictSyncStashApplyFailureTemplate, stash.CommitID, stash.Path, applyErr)
	}
	return restoreStrictSyncIntentToAdd(ctx, executor, stash)
}

func restoreStrictSyncIntentToAdd(ctx context.Context, executor shared.GitExecutor, stash strictSyncStash) error {
	if len(stash.IntentPaths) == 0 {
		return nil
	}
	if resetErr := executeGitDetails(ctx, executor, execshell.CommandDetails{
		Arguments:            append([]string{gitResetSubcommandConstant, gitPathspecSeparatorConstant}, stash.IntentPaths...),
		WorkingDirectory:     stash.Path,
		EnvironmentVariables: map[string]string{gitLiteralPathspecsEnvironmentName: "1"},
	}); resetErr != nil {
		return fmt.Errorf(strictSyncStashIntentFailureTemplate, stash.Path, resetErr)
	}
	if addErr := executeGitDetails(ctx, executor, execshell.CommandDetails{
		Arguments:            append([]string{gitAddSubcommandConstant, gitAddIntentFlagConstant, gitAddForceFlagConstant, gitPathspecSeparatorConstant}, stash.IntentPaths...),
		WorkingDirectory:     stash.Path,
		EnvironmentVariables: map[string]string{gitLiteralPathspecsEnvironmentName: "1"},
	}); addErr != nil {
		return fmt.Errorf(strictSyncStashIntentFailureTemplate, stash.Path, addErr)
	}
	return nil
}

func dropStrictSyncStash(ctx context.Context, executor shared.GitExecutor, stash strictSyncStash) error {
	result, listErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitStashSubcommandConstant, gitStashListSubcommandConstant, gitStashFormatFlagConstant},
		WorkingDirectory: stash.Path,
	})
	if listErr != nil {
		return fmt.Errorf(strictSyncStashListFailureTemplate, stash.Path, listErr)
	}
	stashEntries := strings.Fields(result.StandardOutput)
	for stashIndex := range stashEntries {
		if stashEntries[stashIndex] != stash.CommitID {
			continue
		}
		stashReference := fmt.Sprintf("stash@{%d}", stashIndex)
		if dropErr := executeGit(ctx, executor, stash.Path, []string{gitStashSubcommandConstant, gitStashDropSubcommandConstant, stashReference}); dropErr != nil {
			return fmt.Errorf(strictSyncStashDropFailureTemplate, stash.CommitID, stash.Path, dropErr)
		}
		return nil
	}
	return fmt.Errorf(strictSyncStashMissingTemplate, stash.CommitID, stash.Path)
}

func restoreStrictSyncStash(ctx context.Context, executor shared.GitExecutor, stash strictSyncStash, service mergeConflictResolutionService, options mergeConflictResolutionOptions) error {
	applyErr := applyStrictSyncStash(ctx, executor, stash)
	if applyErr == nil {
		return dropStrictSyncStash(ctx, executor, stash)
	}

	options.Completion = mergeConflictCompletionPreserveIndex
	conflictObserved, resolveErr := service.Resolve(ctx, options)
	if resolveErr != nil {
		return errors.Join(applyErr, fmt.Errorf(strictSyncStashResolveFailureTemplate, stash.Path, resolveErr))
	}
	if !conflictObserved {
		return errors.Join(applyErr, errors.New(strictSyncStashUnmergedStateMessage))
	}
	if intentErr := restoreStrictSyncIntentToAdd(ctx, executor, stash); intentErr != nil {
		return intentErr
	}
	return dropStrictSyncStash(ctx, executor, stash)
}
