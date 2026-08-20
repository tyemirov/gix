package migrate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/tyemirov/gix/internal/execshell"
)

const (
	gitFetchCommandNameConstant                 = "fetch"
	gitFetchNoTagsFlagConstant                  = "--no-tags"
	gitMergeBaseCommandNameConstant             = "merge-base"
	gitMergeBaseIsAncestorFlagConstant          = "--is-ancestor"
	gitDiffCommandNameConstant                  = "diff"
	gitDiffQuietFlagConstant                    = "--quiet"
	gitRevParseCommandNameConstant              = "rev-parse"
	gitRevParseVerifyFlagConstant               = "--verify"
	gitEndOfOptionsFlagConstant                 = "--end-of-options"
	gitCommitPeelSuffixConstant                 = "^{commit}"
	gitRemoteTrackingReferenceTemplateConstant  = "refs/remotes/%s/%s"
	gitFetchRemoteBranchRefspecTemplateConstant = "+refs/heads/%s:refs/remotes/%s/%s"
	remoteBranchFetchErrorTemplateConstant      = "unable to fetch remote source and target branches: %w"
	remoteBranchCommitErrorTemplateConstant     = "unable to resolve remote branch %q commit: %w"
	remoteBranchCommitInvalidTemplateConstant   = "remote branch %q returned invalid commit %q"
	sourceChangeSafetyErrorTemplateConstant     = "unable to verify source branch changes: %w"
	sha1CommitOIDLengthConstant                 = 40
	sha256CommitOIDLengthConstant               = 64
)

type commitOID string

type sourceDeletionSafety struct {
	SourceCommit         commitOID
	SourceChangesMissing bool
}

func verifyRemoteSourceDeletionSafety(executionContext context.Context, executor CommandExecutor, options MigrationOptions) (sourceDeletionSafety, error) {
	remoteName := strings.TrimSpace(options.RepositoryRemoteName)
	sourceBranch := strings.TrimSpace(string(options.SourceBranch))
	targetBranch := strings.TrimSpace(string(options.TargetBranch))
	fetchArguments := []string{
		gitFetchCommandNameConstant,
		gitFetchNoTagsFlagConstant,
		remoteName,
		fmt.Sprintf(gitFetchRemoteBranchRefspecTemplateConstant, sourceBranch, remoteName, sourceBranch),
		fmt.Sprintf(gitFetchRemoteBranchRefspecTemplateConstant, targetBranch, remoteName, targetBranch),
	}
	if _, fetchError := executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        fetchArguments,
		WorkingDirectory: options.RepositoryPath,
	}); fetchError != nil {
		return sourceDeletionSafety{}, fmt.Errorf(remoteBranchFetchErrorTemplateConstant, fetchError)
	}

	sourceReference := fmt.Sprintf(gitRemoteTrackingReferenceTemplateConstant, remoteName, sourceBranch)
	targetReference := fmt.Sprintf(gitRemoteTrackingReferenceTemplateConstant, remoteName, targetBranch)
	sourceCommit, sourceCommitError := resolveRemoteBranchCommit(executionContext, executor, options.RepositoryPath, sourceReference)
	if sourceCommitError != nil {
		return sourceDeletionSafety{}, sourceCommitError
	}
	targetCommit, targetCommitError := resolveRemoteBranchCommit(executionContext, executor, options.RepositoryPath, targetReference)
	if targetCommitError != nil {
		return sourceDeletionSafety{}, targetCommitError
	}

	sourceChangesMissing, sourceChangesError := sourceChangesMissingBetweenReferences(
		executionContext,
		executor,
		options.RepositoryPath,
		string(sourceCommit),
		string(targetCommit),
	)
	if sourceChangesError != nil {
		return sourceDeletionSafety{}, sourceChangesError
	}

	return sourceDeletionSafety{
		SourceCommit:         sourceCommit,
		SourceChangesMissing: sourceChangesMissing,
	}, nil
}

func resolveRemoteBranchCommit(executionContext context.Context, executor CommandExecutor, repositoryPath string, reference string) (commitOID, error) {
	result, resolveError := executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments: []string{
			gitRevParseCommandNameConstant,
			gitRevParseVerifyFlagConstant,
			gitEndOfOptionsFlagConstant,
			reference + gitCommitPeelSuffixConstant,
		},
		WorkingDirectory: repositoryPath,
	})
	if resolveError != nil {
		return "", fmt.Errorf(remoteBranchCommitErrorTemplateConstant, reference, resolveError)
	}

	commit, commitError := newCommitOID(result.StandardOutput)
	if commitError != nil {
		return "", fmt.Errorf(remoteBranchCommitInvalidTemplateConstant, reference, strings.TrimSpace(result.StandardOutput))
	}
	return commit, nil
}

func newCommitOID(value string) (commitOID, error) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) != sha1CommitOIDLengthConstant && len(trimmed) != sha256CommitOIDLengthConstant {
		return "", errors.New("commit OID has an invalid length")
	}
	for _, character := range trimmed {
		if !unicode.Is(unicode.ASCII_Hex_Digit, character) {
			return "", errors.New("commit OID contains a non-hexadecimal character")
		}
	}
	return commitOID(strings.ToLower(trimmed)), nil
}

func sourceChangesMissingBetweenReferences(
	executionContext context.Context,
	executor CommandExecutor,
	repositoryPath string,
	sourceReference string,
	targetReference string,
) (bool, error) {
	ancestorArguments := []string{
		gitMergeBaseCommandNameConstant,
		gitMergeBaseIsAncestorFlagConstant,
		sourceReference,
		targetReference,
	}
	_, ancestorError := executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        ancestorArguments,
		WorkingDirectory: repositoryPath,
	})
	if ancestorError == nil {
		return false, nil
	}
	if !isGitDifferenceResult(ancestorError) {
		return false, fmt.Errorf(sourceChangeSafetyErrorTemplateConstant, ancestorError)
	}

	diffArguments := []string{
		gitDiffCommandNameConstant,
		gitDiffQuietFlagConstant,
		sourceReference,
		targetReference,
	}
	_, diffError := executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        diffArguments,
		WorkingDirectory: repositoryPath,
	})
	if diffError == nil {
		return false, nil
	}
	if isGitDifferenceResult(diffError) {
		return true, nil
	}
	return false, fmt.Errorf(sourceChangeSafetyErrorTemplateConstant, diffError)
}

func isGitDifferenceResult(executionError error) bool {
	var commandFailure execshell.CommandFailedError
	return errors.As(executionError, &commandFailure) && commandFailure.Result.ExitCode == 1
}
