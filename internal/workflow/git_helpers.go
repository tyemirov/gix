package workflow

import (
	"context"
	"errors"
	"strings"

	"github.com/temirov/gix/internal/execshell"
)

func branchExists(ctx context.Context, executor sharedGitExecutor, repositoryPath string, branchName string) (bool, error) {
	trimmedBranch := strings.TrimSpace(branchName)
	if len(trimmedBranch) == 0 || executor == nil {
		return false, nil
	}

	arguments := []string{"rev-parse", "--verify", trimmedBranch}
	_, err := executor.ExecuteGit(ctx, execshell.CommandDetails{Arguments: arguments, WorkingDirectory: repositoryPath})
	if err == nil {
		return true, nil
	}

	var commandErr execshell.CommandFailedError
	if errors.As(err, &commandErr) {
		return false, nil
	}

	if strings.Contains(err.Error(), "unknown revision") || strings.Contains(err.Error(), "Needed a single revision") {
		return false, nil
	}

	return false, err
}

type sharedGitExecutor interface {
	ExecuteGit(ctx context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error)
}
