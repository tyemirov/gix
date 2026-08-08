package syncflow

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/v5/internal/execshell"
	"github.com/tyemirov/gix/v5/internal/repos/shared"
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
)

type strictSyncStash struct {
	CommitID string
	Path     string
}

func pushStrictSyncStash(ctx context.Context, executor shared.GitExecutor, repositoryPath string, message string) (strictSyncStash, error) {
	if pushErr := executeGit(ctx, executor, repositoryPath, []string{
		gitStashSubcommandConstant,
		gitStashPushSubcommandConstant,
		gitStashIncludeUntrackedFlagConstant,
		gitStashMessageFlagConstant,
		message,
	}); pushErr != nil {
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
	return strictSyncStash{CommitID: commitID, Path: repositoryPath}, nil
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
	return dropStrictSyncStash(ctx, executor, stash)
}
