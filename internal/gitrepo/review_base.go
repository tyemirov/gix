package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
)

const (
	branchReviewBaseKeyTemplate         = "branch.%s.gix-review-base"
	branchReviewBaseConfigSubcommand    = "config"
	branchReviewBaseLocalFlag           = "--local"
	branchReviewBaseNoIncludesFlag      = "--no-includes"
	branchReviewBaseGetFlag             = "--get"
	branchReviewBaseUnsetAllFlag        = "--unset-all"
	branchReviewBaseReadErrorTemplate   = "read stacked review base for %q: %w"
	branchReviewBaseRecordErrorTemplate = "record stacked review base for %q: %w"
	branchReviewBaseRemoveErrorTemplate = "remove stacked review base for %q: %w"
)

// BranchReviewBaseKey returns the Git config key for a branch review base.
func BranchReviewBaseKey(branchName string) string {
	return fmt.Sprintf(branchReviewBaseKeyTemplate, strings.TrimSpace(branchName))
}

// BranchReviewBase returns the recorded review base and whether the key exists.
func BranchReviewBase(ctx context.Context, executor GitCommandExecutor, repositoryPath string, branchName string) (string, bool, error) {
	result, configErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments: []string{
			branchReviewBaseConfigSubcommand,
			branchReviewBaseLocalFlag,
			branchReviewBaseNoIncludesFlag,
			branchReviewBaseGetFlag,
			BranchReviewBaseKey(branchName),
		},
		WorkingDirectory: repositoryPath,
	})
	if configErr == nil {
		return strings.TrimSpace(result.StandardOutput), true, nil
	}
	var commandFailure execshell.CommandFailedError
	if errors.As(configErr, &commandFailure) && commandFailure.Result.ExitCode == 1 {
		return "", false, nil
	}
	return "", false, fmt.Errorf(branchReviewBaseReadErrorTemplate, branchName, configErr)
}

// RecordBranchReviewBase records the parent branch for a review branch.
func RecordBranchReviewBase(ctx context.Context, executor GitCommandExecutor, repositoryPath string, branchName string, parentBranch string) error {
	_, configErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments: []string{
			branchReviewBaseConfigSubcommand,
			branchReviewBaseLocalFlag,
			branchReviewBaseNoIncludesFlag,
			BranchReviewBaseKey(branchName),
			parentBranch,
		},
		WorkingDirectory: repositoryPath,
	})
	if configErr != nil {
		return fmt.Errorf(branchReviewBaseRecordErrorTemplate, branchName, configErr)
	}
	return nil
}

// RemoveBranchReviewBase removes all review-base values for a branch.
func RemoveBranchReviewBase(ctx context.Context, executor GitCommandExecutor, repositoryPath string, branchName string) error {
	_, exists, lookupErr := BranchReviewBase(ctx, executor, repositoryPath, branchName)
	if lookupErr != nil {
		return lookupErr
	}
	if !exists {
		return nil
	}
	_, configErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments: []string{
			branchReviewBaseConfigSubcommand,
			branchReviewBaseLocalFlag,
			branchReviewBaseNoIncludesFlag,
			branchReviewBaseUnsetAllFlag,
			BranchReviewBaseKey(branchName),
		},
		WorkingDirectory: repositoryPath,
	})
	if configErr != nil {
		return fmt.Errorf(branchReviewBaseRemoveErrorTemplate, branchName, configErr)
	}
	return nil
}
