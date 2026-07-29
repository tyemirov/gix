package syncflow

import (
	"context"
	"errors"
	"fmt"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	gitRevertHeadReferenceConstant            = "REVERT_HEAD"
	strictSyncRevertInProgressMessage         = "strict sync cannot run while an operator-owned Git revert is in progress; resolve it explicitly with \"git revert --continue\", \"git revert --abort\", or \"git revert --quit\", then retry"
	strictSyncRevertInspectionFailureTemplate = "inspect operator-owned Git revert before strict sync: %w"
)

func ensureNoOperatorOwnedRevert(ctx context.Context, executor shared.GitExecutor, repositoryPath string) error {
	_, inspectionErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitRevParseSubcommandConstant, gitVerifyFlagConstant, gitRevParseQuietFlagConstant, gitRevertHeadReferenceConstant},
		WorkingDirectory: repositoryPath,
	})
	if inspectionErr == nil {
		return errors.New(strictSyncRevertInProgressMessage)
	}

	var commandFailure execshell.CommandFailedError
	if errors.As(inspectionErr, &commandFailure) && commandFailure.Result.ExitCode == 1 {
		return nil
	}
	return fmt.Errorf(strictSyncRevertInspectionFailureTemplate, inspectionErr)
}
