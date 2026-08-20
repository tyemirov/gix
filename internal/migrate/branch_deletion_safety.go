package migrate

import (
	"context"
	"errors"
	"fmt"

	"github.com/tyemirov/gix/internal/execshell"
)

const (
	gitMergeBaseCommandNameConstant         = "merge-base"
	gitMergeBaseIsAncestorFlagConstant      = "--is-ancestor"
	gitDiffCommandNameConstant              = "diff"
	gitDiffQuietFlagConstant                = "--quiet"
	sourceChangeSafetyErrorTemplateConstant = "unable to verify source branch changes: %w"
)

func sourceChangesMissingFromTarget(executionContext context.Context, executor CommandExecutor, options MigrationOptions) (bool, error) {
	ancestorArguments := []string{
		gitMergeBaseCommandNameConstant,
		gitMergeBaseIsAncestorFlagConstant,
		string(options.SourceBranch),
		string(options.TargetBranch),
	}
	_, ancestorError := executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        ancestorArguments,
		WorkingDirectory: options.RepositoryPath,
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
		string(options.SourceBranch),
		string(options.TargetBranch),
	}
	_, diffError := executor.ExecuteGit(executionContext, execshell.CommandDetails{
		Arguments:        diffArguments,
		WorkingDirectory: options.RepositoryPath,
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
