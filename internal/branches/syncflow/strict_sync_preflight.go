package syncflow

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	gitRevertHeadReferenceConstant          = "REVERT_HEAD"
	gitCherryPickHeadReferenceConstant      = "CHERRY_PICK_HEAD"
	gitRebaseMergePathConstant              = "rebase-merge"
	gitRebaseApplyPathConstant              = "rebase-apply"
	gitRebaseApplyApplyingPathConstant      = "applying"
	gitBisectStartPathConstant              = "BISECT_START"
	gitSequencerPathConstant                = "sequencer"
	gitPathFlagConstant                     = "--git-path"
	gitCommonDirectoryFlagConstant          = "--git-common-dir"
	gitEndOfOptionsFlagConstant             = "--end-of-options"
	gitCommitPeelSuffixConstant             = "^{commit}"
	gitDirectoryFileNameConstant            = ".git"
	gitDirectoryFilePrefixConstant          = "gitdir:"
	strictSyncOperationInProgressTemplate   = "strict sync cannot run while an operator-owned Git %s is in progress at %q; %s, then retry"
	strictSyncOperationInspectionFailure    = "inspect operator-owned Git state before strict sync at %q: %w"
	strictSyncOperationPathMissingTemplate  = "git returned an empty %s path"
	strictSyncOperationPathKindTemplate     = "%s has unexpected filesystem kind"
	strictSyncOperationReadFailureTemplate  = "read %s: %w"
	strictSyncOperationValidationTemplate   = "validate %s: %w"
	strictSyncOperationNonCanonicalTemplate = "%s does not contain canonical commit identifiers"
	strictSyncUnmergedIndexGuidance         = "resolve or restore it explicitly"
	strictSyncWorktreeRepairFailureTemplate = "repair worktree metadata before strict sync at %q: %w"
	strictSyncWorktreeLinkFailureTemplate   = "inspect registered worktree metadata at %q for repository %q: %w"
	strictSyncWorktreeLinkMissingTemplate   = "%s does not contain a canonical gitdir record"
	strictSyncWorktreeLiveLinkTemplate      = "registered worktree %q belongs to live common repository %q, not %q; remove the conflicting registration explicitly or run sync from its owning repository"
	strictSyncWorktreeLiveTargetTemplate    = "registered worktree %q cannot be validated while its gitdir target %q still exists; refusing to rewrite live metadata: %w"
)

type strictSyncPlan struct {
	targetBranch     string
	startingWorktree listedWorktree
	worktrees        []listedWorktree
	touchedWorktrees []listedWorktree
}

type strictSyncGitOperationDescriptor struct {
	Kind               string
	AdministrativePath string
	Directory          bool
	CommitFile         bool
	Guidance           string
}

var strictSyncGitOperationDescriptors = [...]strictSyncGitOperationDescriptor{
	{
		Kind:               "merge",
		AdministrativePath: gitMergeHeadReferenceConstant,
		CommitFile:         true,
		Guidance:           "resolve it explicitly with \"git merge --continue\" or \"git merge --abort\"",
	},
	{
		Kind:               "revert",
		AdministrativePath: gitRevertHeadReferenceConstant,
		CommitFile:         true,
		Guidance:           "resolve it explicitly with \"git revert --continue\", \"git revert --abort\", or \"git revert --quit\"",
	},
	{
		Kind:               "cherry-pick",
		AdministrativePath: gitCherryPickHeadReferenceConstant,
		CommitFile:         true,
		Guidance:           "resolve it explicitly with \"git cherry-pick --continue\", \"git cherry-pick --abort\", or \"git cherry-pick --quit\"",
	},
	{
		Kind:               "rebase",
		AdministrativePath: gitRebaseMergePathConstant,
		Directory:          true,
		Guidance:           "resolve it explicitly with \"git rebase --continue\", \"git rebase --abort\", or \"git rebase --quit\"",
	},
	{
		Kind:               "rebase",
		AdministrativePath: gitRebaseApplyPathConstant,
		Directory:          true,
		Guidance:           "resolve it explicitly with \"git rebase --continue\", \"git rebase --abort\", or \"git rebase --quit\"",
	},
	{
		Kind:               "bisect",
		AdministrativePath: gitBisectStartPathConstant,
		Guidance:           "finish the bisect explicitly or restore the starting checkout with \"git bisect reset\"",
	},
	{
		Kind:               "sequencer",
		AdministrativePath: gitSequencerPathConstant,
		Directory:          true,
		Guidance:           "finish or abort the sequenced operation explicitly",
	},
}

func buildStrictSyncPlan(ctx context.Context, executor shared.GitExecutor, repositoryPath string, targetBranch string) (strictSyncPlan, error) {
	worktrees, listErr := listRepositoryWorktrees(ctx, executor, repositoryPath, targetBranch)
	if listErr != nil {
		return strictSyncPlan{}, fmt.Errorf(strictSyncOperationInspectionFailure, repositoryPath, listErr)
	}
	repairPaths, repairPlanErr := strictSyncWorktreeRepairPaths(ctx, executor, repositoryPath, worktrees, true)
	if repairPlanErr != nil {
		return strictSyncPlan{}, repairPlanErr
	}
	if len(repairPaths) > 0 {
		if repairErr := repairStrictSyncWorktreeMetadata(ctx, executor, repositoryPath, repairPaths); repairErr != nil {
			return strictSyncPlan{}, repairErr
		}
		worktrees, listErr = listRepositoryWorktrees(ctx, executor, repositoryPath, targetBranch)
		if listErr != nil {
			return strictSyncPlan{}, fmt.Errorf(strictSyncOperationInspectionFailure, repositoryPath, listErr)
		}
		if _, validationErr := strictSyncWorktreeRepairPaths(ctx, executor, repositoryPath, worktrees, false); validationErr != nil {
			return strictSyncPlan{}, validationErr
		}
	}

	startingWorktree, exists := findListedWorktreeByPath(worktrees, repositoryPath)
	if !exists {
		return strictSyncPlan{}, fmt.Errorf(strictSyncStartingWorktreeMissingTemplate, repositoryPath)
	}

	for worktreeIndex := range worktrees {
		worktree := worktrees[worktreeIndex]
		if worktree.Prunable {
			continue
		}
		if operationErr := ensureWorktreeHasNoOperatorOwnedGitOperation(ctx, executor, worktree.Path); operationErr != nil {
			return strictSyncPlan{}, operationErr
		}
		if unmergedErr := ensureWorktreeHasNoOperatorOwnedUnmergedIndex(ctx, executor, worktree.Path); unmergedErr != nil {
			return strictSyncPlan{}, unmergedErr
		}
	}

	touchedWorktrees := []listedWorktree{startingWorktree}
	trimmedTargetBranch := strings.TrimSpace(targetBranch)
	for worktreeIndex := range worktrees {
		worktree := worktrees[worktreeIndex]
		if worktree.Prunable || sameFilesystemPath(worktree.Path, repositoryPath) || worktree.BranchName != trimmedTargetBranch {
			continue
		}
		touchedWorktrees = append(touchedWorktrees, worktree)
	}

	return strictSyncPlan{
		targetBranch:     trimmedTargetBranch,
		startingWorktree: startingWorktree,
		worktrees:        worktrees,
		touchedWorktrees: touchedWorktrees,
	}, nil
}

func repairStrictSyncWorktreeMetadata(ctx context.Context, executor shared.GitExecutor, repositoryPath string, worktreePaths []string) error {
	arguments := []string{gitWorktreeSubcommandConstant, gitWorktreeRepairSubcommandConstant}
	arguments = append(arguments, worktreePaths...)
	_, repairErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        arguments,
		WorkingDirectory: repositoryPath,
	})
	if repairErr != nil {
		return fmt.Errorf(strictSyncWorktreeRepairFailureTemplate, repositoryPath, repairErr)
	}
	return nil
}

func strictSyncWorktreeRepairPaths(ctx context.Context, executor shared.GitExecutor, repositoryPath string, worktrees []listedWorktree, allowRepair bool) ([]string, error) {
	callerCommonDirectory, callerCommonErr := resolveStrictSyncCommonDirectory(ctx, executor, repositoryPath)
	if callerCommonErr != nil {
		return nil, fmt.Errorf(strictSyncWorktreeLinkFailureTemplate, repositoryPath, repositoryPath, callerCommonErr)
	}

	repairPaths := make([]string, 0)
	for worktreeIndex := range worktrees {
		worktree := worktrees[worktreeIndex]
		if worktree.Prunable || sameFilesystemPath(worktree.Path, repositoryPath) {
			continue
		}

		worktreeCommonDirectory, commonDirectoryErr := resolveStrictSyncCommonDirectory(ctx, executor, worktree.Path)
		if commonDirectoryErr == nil {
			if !sameFilesystemPath(worktreeCommonDirectory, callerCommonDirectory) {
				return nil, fmt.Errorf(
					strictSyncWorktreeLiveLinkTemplate,
					worktree.Path,
					worktreeCommonDirectory,
					callerCommonDirectory,
				)
			}
			continue
		}
		if !allowRepair {
			return nil, fmt.Errorf(strictSyncWorktreeLinkFailureTemplate, worktree.Path, repositoryPath, commonDirectoryErr)
		}

		gitDirectoryTarget, targetErr := readStrictSyncGitDirectoryTarget(worktree.Path)
		if targetErr != nil {
			return nil, fmt.Errorf(strictSyncWorktreeLinkFailureTemplate, worktree.Path, repositoryPath, errors.Join(commonDirectoryErr, targetErr))
		}
		targetInfo, targetInspectionErr := os.Stat(gitDirectoryTarget)
		if errors.Is(targetInspectionErr, fs.ErrNotExist) {
			repairPaths = append(repairPaths, worktree.Path)
			continue
		}
		if targetInspectionErr != nil {
			return nil, fmt.Errorf(strictSyncWorktreeLinkFailureTemplate, worktree.Path, repositoryPath, targetInspectionErr)
		}
		if targetInfo.IsDir() {
			return nil, fmt.Errorf(strictSyncWorktreeLiveTargetTemplate, worktree.Path, gitDirectoryTarget, commonDirectoryErr)
		}
		return nil, fmt.Errorf(
			strictSyncWorktreeLinkFailureTemplate,
			worktree.Path,
			repositoryPath,
			fmt.Errorf("%s has unexpected filesystem kind", gitDirectoryTarget),
		)
	}
	return repairPaths, nil
}

func resolveStrictSyncCommonDirectory(ctx context.Context, executor shared.GitExecutor, repositoryPath string) (string, error) {
	result, commonDirectoryErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitRevParseSubcommandConstant, gitCommonDirectoryFlagConstant},
		WorkingDirectory: repositoryPath,
	})
	if commonDirectoryErr != nil {
		return "", commonDirectoryErr
	}
	commonDirectory := strings.TrimSpace(result.StandardOutput)
	if commonDirectory == "" {
		return "", fmt.Errorf(strictSyncOperationPathMissingTemplate, gitCommonDirectoryFlagConstant)
	}
	if !filepath.IsAbs(commonDirectory) {
		commonDirectory = filepath.Join(repositoryPath, commonDirectory)
	}
	return filepath.Clean(commonDirectory), nil
}

func readStrictSyncGitDirectoryTarget(worktreePath string) (string, error) {
	gitDirectoryFile := filepath.Join(worktreePath, gitDirectoryFileNameConstant)
	contents, readErr := os.ReadFile(gitDirectoryFile)
	if readErr != nil {
		return "", readErr
	}
	record := strings.TrimSpace(string(contents))
	if !strings.HasPrefix(record, gitDirectoryFilePrefixConstant) || strings.Contains(record, "\n") {
		return "", fmt.Errorf(strictSyncWorktreeLinkMissingTemplate, gitDirectoryFile)
	}
	target := strings.TrimSpace(strings.TrimPrefix(record, gitDirectoryFilePrefixConstant))
	if target == "" {
		return "", fmt.Errorf(strictSyncWorktreeLinkMissingTemplate, gitDirectoryFile)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(worktreePath, target)
	}
	return filepath.Clean(target), nil
}

func ensureWorktreeHasNoOperatorOwnedGitOperation(ctx context.Context, executor shared.GitExecutor, repositoryPath string) error {
	for descriptorIndex := range strictSyncGitOperationDescriptors {
		descriptor := strictSyncGitOperationDescriptors[descriptorIndex]
		operationPresent, inspectionErr := inspectStrictSyncGitOperation(ctx, executor, repositoryPath, descriptor)
		if inspectionErr != nil {
			return fmt.Errorf(strictSyncOperationInspectionFailure, repositoryPath, inspectionErr)
		}
		if !operationPresent {
			continue
		}
		operationKind := descriptor.Kind
		guidance := descriptor.Guidance
		if descriptor.AdministrativePath == gitRebaseApplyPathConstant {
			rebaseApplyPath, pathErr := resolveStrictSyncAdministrativePath(ctx, executor, repositoryPath, descriptor.AdministrativePath)
			if pathErr != nil {
				return fmt.Errorf(strictSyncOperationInspectionFailure, repositoryPath, pathErr)
			}
			applyingPath := filepath.Join(rebaseApplyPath, gitRebaseApplyApplyingPathConstant)
			if _, applyingErr := os.Lstat(applyingPath); applyingErr == nil {
				operationKind = "apply-mailbox"
				guidance = "resolve it explicitly with \"git am --continue\", \"git am --abort\", or \"git am --quit\""
			} else if !errors.Is(applyingErr, fs.ErrNotExist) {
				return fmt.Errorf(strictSyncOperationInspectionFailure, repositoryPath, applyingErr)
			}
		}
		return fmt.Errorf(strictSyncOperationInProgressTemplate, operationKind, repositoryPath, guidance)
	}
	return nil
}

func ensureWorktreeHasNoOperatorOwnedUnmergedIndex(ctx context.Context, executor shared.GitExecutor, repositoryPath string) error {
	result, inspectionErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitDiffSubcommandConstant, gitDiffNameOnlyFlagConstant, gitDiffFilterUnmergedFlagConstant},
		WorkingDirectory: repositoryPath,
	})
	if inspectionErr != nil {
		return fmt.Errorf(strictSyncOperationInspectionFailure, repositoryPath, inspectionErr)
	}
	if strings.TrimSpace(result.StandardOutput) == "" {
		return nil
	}
	return fmt.Errorf(strictSyncOperationInProgressTemplate, "unmerged index", repositoryPath, strictSyncUnmergedIndexGuidance)
}

func inspectStrictSyncGitOperation(ctx context.Context, executor shared.GitExecutor, repositoryPath string, descriptor strictSyncGitOperationDescriptor) (bool, error) {
	administrativePath, pathErr := resolveStrictSyncAdministrativePath(ctx, executor, repositoryPath, descriptor.AdministrativePath)
	if pathErr != nil {
		return false, pathErr
	}

	pathInfo, inspectErr := os.Lstat(administrativePath)
	if errors.Is(inspectErr, fs.ErrNotExist) {
		return false, nil
	}
	if inspectErr != nil {
		return false, inspectErr
	}
	if descriptor.Directory && !pathInfo.IsDir() {
		return false, fmt.Errorf(strictSyncOperationPathKindTemplate, descriptor.AdministrativePath)
	}
	if !descriptor.Directory && !pathInfo.Mode().IsRegular() {
		return false, fmt.Errorf(strictSyncOperationPathKindTemplate, descriptor.AdministrativePath)
	}
	if descriptor.CommitFile {
		if validationErr := validateStrictSyncOperationCommitFile(ctx, executor, repositoryPath, administrativePath, descriptor.AdministrativePath); validationErr != nil {
			return false, validationErr
		}
	}
	return true, nil
}

func resolveStrictSyncAdministrativePath(ctx context.Context, executor shared.GitExecutor, repositoryPath string, administrativeName string) (string, error) {
	pathResult, pathErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitRevParseSubcommandConstant, gitPathFlagConstant, administrativeName},
		WorkingDirectory: repositoryPath,
	})
	if pathErr != nil {
		return "", pathErr
	}
	resolvedPath := strings.TrimSpace(pathResult.StandardOutput)
	if resolvedPath == "" {
		return "", fmt.Errorf(strictSyncOperationPathMissingTemplate, administrativeName)
	}
	if !filepath.IsAbs(resolvedPath) {
		resolvedPath = filepath.Join(repositoryPath, resolvedPath)
	}
	return filepath.Clean(resolvedPath), nil
}

func validateStrictSyncOperationCommitFile(ctx context.Context, executor shared.GitExecutor, repositoryPath string, administrativePath string, administrativeName string) error {
	contents, readErr := os.ReadFile(administrativePath)
	if readErr != nil {
		return fmt.Errorf(strictSyncOperationReadFailureTemplate, administrativeName, readErr)
	}
	commitIdentifiers := strings.Fields(string(contents))
	if len(commitIdentifiers) == 0 {
		return fmt.Errorf(strictSyncOperationNonCanonicalTemplate, administrativeName)
	}
	for commitIndex := range commitIdentifiers {
		commitIdentifier := commitIdentifiers[commitIndex]
		validationResult, validationErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
			Arguments: []string{
				gitRevParseSubcommandConstant,
				gitVerifyFlagConstant,
				gitRevParseQuietFlagConstant,
				gitEndOfOptionsFlagConstant,
				commitIdentifier + gitCommitPeelSuffixConstant,
			},
			WorkingDirectory: repositoryPath,
		})
		if validationErr != nil {
			return fmt.Errorf(strictSyncOperationValidationTemplate, administrativeName, validationErr)
		}
		if strings.TrimSpace(validationResult.StandardOutput) != commitIdentifier {
			return fmt.Errorf(strictSyncOperationNonCanonicalTemplate, administrativeName)
		}
	}
	return nil
}
