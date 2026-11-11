package syncflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	strictSyncStashProjectionMessage = "gix strict-sync stash projection"
	gitReadTreeSubcommand            = "read-tree"
	gitWriteTreeSubcommand           = "write-tree"
	gitCommitTreeSubcommand          = "commit-tree"
	gitRestoreSubcommand             = "restore"
	gitRestoreStagedFlag             = "--staged"
	gitSourceFlag                    = "--source"
	gitCommitParentFlag              = "-p"
)

type strictSyncStashProjection struct {
	stash         strictSyncStash
	indexTree     string
	untrackedTree string
	remaining     []string
}

// Project only untracked additions that became tracked at the destination.
// The original stash remains the transaction's recovery and cleanup object.
func projectStrictSyncStash(ctx context.Context, executor shared.GitExecutor, stash strictSyncStash) (projection strictSyncStashProjection, resultErr error) {
	projection.stash = stash
	defer func() {
		if resultErr != nil {
			resultErr = fmt.Errorf("prepare strict sync stash %s at %s: %w", stash.CommitID, stash.Path, resultErr)
		}
	}()
	command := strictSyncStashGit{ctx: ctx, executor: executor, path: stash.Path}
	parentOutput, err := command.run("show", "-s", "--format=%P", stash.CommitID)
	if err != nil {
		return projection, err
	}
	parents := strings.Fields(parentOutput)
	if len(parents) == 2 {
		return projection, nil
	}
	if len(parents) != 3 {
		return projection, errors.New("stash must have two or three parents")
	}
	untrackedOutput, err := command.run("ls-tree", "-r", "--name-only", "-z", parents[2])
	if err != nil {
		return projection, err
	}
	untracked, err := parseStrictSyncNULTerminatedPaths(untrackedOutput)
	if err != nil {
		return projection, err
	}
	trackedOutput, err := command.run("ls-files", "-z")
	if err != nil {
		return projection, err
	}
	tracked, err := parseStrictSyncNULTerminatedPaths(trackedOutput)
	if err != nil {
		return projection, err
	}
	trackedPaths := newStrictSyncTrackedPaths(tracked)
	var collisions []string
	for _, path := range untracked {
		if trackedPath := trackedPaths.prefixConflict(path); trackedPath != "" {
			return projection, fmt.Errorf("untracked path %q conflicts with destination tracked path %q", path, trackedPath)
		}
		if _, exists := trackedPaths.exact[path]; exists {
			collisions = append(collisions, path)
		} else {
			projection.remaining = append(projection.remaining, path)
		}
	}
	if len(collisions) == 0 {
		return projection, nil
	}
	cleanup, err := command.privateIndex()
	if err != nil {
		return projection, err
	}
	defer func() { resultErr = errors.Join(resultErr, cleanup()) }()

	// Save C + (B -> I) before stash apply, which omits index restoration
	// when its working-tree merge conflicts.
	if _, err := command.run(gitReadTreeSubcommand, gitHeadReferenceConstant); err != nil {
		return projection, err
	}
	patch, err := command.run(gitDiffSubcommandConstant, "--binary", "--no-textconv", "--no-ext-diff", "--no-color",
		"--src-prefix=a/", "--dst-prefix=b/", "--no-relative", "--unified=3", gitDiffNoRenamesFlagConstant,
		parents[0], parents[1], gitPathspecSeparatorConstant)
	if err != nil {
		return projection, err
	}
	if patch != "" {
		if _, err := command.execute([]string{"apply", "--cached"}, []byte(patch)); err != nil {
			return projection, err
		}
	}
	projection.indexTree, err = command.object(gitWriteTreeSubcommand)
	if err != nil {
		return projection, err
	}
	if _, err := command.run(gitReadTreeSubcommand, stash.CommitID); err != nil {
		return projection, err
	}
	if _, err := command.run(append([]string{gitRestoreSubcommand, gitRestoreStagedFlag, gitSourceFlag, parents[2], gitPathspecSeparatorConstant}, collisions...)...); err != nil {
		return projection, err
	}
	workingTree, err := command.object(gitWriteTreeSubcommand)
	if err != nil {
		return projection, err
	}
	if _, err := command.run(gitReadTreeSubcommand, parents[2]); err != nil {
		return projection, err
	}
	if _, err := command.run(append([]string{"update-index", "--force-remove", gitPathspecSeparatorConstant}, collisions...)...); err != nil {
		return projection, err
	}
	projection.untrackedTree, err = command.object(gitWriteTreeSubcommand)
	if err != nil {
		return projection, err
	}
	remainingCommit, err := command.object(gitCommitTreeSubcommand, projection.untrackedTree, gitCommitMessageFlagConstant, strictSyncStashProjectionMessage)
	if err != nil {
		return projection, err
	}
	projection.stash.CommitID, err = command.object(gitCommitTreeSubcommand, workingTree,
		gitCommitParentFlag, parents[0], gitCommitParentFlag, parents[1], gitCommitParentFlag, remainingCommit,
		gitCommitMessageFlagConstant, strictSyncStashProjectionMessage)
	return projection, err
}

type strictSyncTrackedPaths struct {
	exact  map[string]struct{}
	sorted []string
}

func newStrictSyncTrackedPaths(paths []string) strictSyncTrackedPaths {
	sort.Strings(paths)
	indexed := strictSyncTrackedPaths{exact: make(map[string]struct{}, len(paths)), sorted: paths}
	for _, path := range paths {
		indexed.exact[path] = struct{}{}
	}
	return indexed
}

func (paths strictSyncTrackedPaths) prefixConflict(path string) string {
	for index := range path {
		if path[index] == '/' {
			if _, exists := paths.exact[path[:index]]; exists {
				return path[:index]
			}
		}
	}
	prefix := path + "/"
	index := sort.SearchStrings(paths.sorted, prefix)
	if index < len(paths.sorted) && strings.HasPrefix(paths.sorted[index], prefix) {
		return paths.sorted[index]
	}
	return ""
}

func (projection strictSyncStashProjection) rejectSymbolicLinkConflicts(ctx context.Context, service mergeConflictResolutionService) error {
	if projection.indexTree == "" {
		return nil
	}
	paths, err := service.unmergedPaths(ctx)
	if err != nil {
		return err
	}
	for _, path := range paths {
		entries, err := os.ReadDir(filepath.Dir(filepath.Join(projection.stash.Path, path)))
		if err != nil {
			return fmt.Errorf("inspect strict sync conflict file type for %q: %w", path, err)
		}
		for _, entry := range entries {
			if entry.Name() == filepath.Base(path) && entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("symbolic link conflict for %q requires an explicit Git resolution", path)
			}
		}
	}
	return nil
}

func (projection strictSyncStashProjection) restoreIndex(ctx context.Context, executor shared.GitExecutor) (resultErr error) {
	if projection.indexTree == "" {
		return nil
	}
	command := strictSyncStashGit{ctx: ctx, executor: executor, path: projection.stash.Path}
	// A remaining untracked restoration failure must not be mistaken for a
	// resolved tracked conflict and permit the recovery stash to be dropped.
	if len(projection.remaining) > 0 {
		cleanup, err := command.privateIndex()
		if err != nil {
			return err
		}
		defer func() { resultErr = errors.Join(resultErr, cleanup()) }()
		if _, err := command.run(gitReadTreeSubcommand, projection.untrackedTree); err != nil {
			return err
		}
		if _, err := command.run(append([]string{"update-index", "--refresh", gitPathspecSeparatorConstant}, projection.remaining...)...); err != nil {
			return fmt.Errorf("verify restored strict sync untracked files: %w", err)
		}
	}
	command.environment = nil
	// Merge the saved tree into the index to retain skip-worktree metadata.
	_, err := command.run(gitReadTreeSubcommand, "-m", projection.indexTree)
	return err
}

type strictSyncStashGit struct {
	ctx         context.Context
	executor    shared.GitExecutor
	path        string
	environment map[string]string
}

func (command strictSyncStashGit) execute(arguments []string, input []byte) (string, error) {
	result, err := command.executor.ExecuteGit(command.ctx, execshell.CommandDetails{
		Arguments: arguments, WorkingDirectory: command.path, EnvironmentVariables: command.environment, StandardInput: input,
	})
	return result.StandardOutput, err
}

func (command strictSyncStashGit) run(arguments ...string) (string, error) {
	return command.execute(arguments, nil)
}

func (command strictSyncStashGit) object(arguments ...string) (string, error) {
	output, err := command.run(arguments...)
	if err != nil {
		return "", err
	}
	object := strings.TrimSpace(output)
	if object == "" {
		return "", fmt.Errorf("git %s returned an empty object identifier", arguments[0])
	}
	return object, nil
}

func (command *strictSyncStashGit) privateIndex() (func() error, error) {
	output, err := command.run(gitRevParseSubcommandConstant, gitPathFlagConstant, gitIndexPathNameConstant)
	if err != nil {
		return nil, err
	}
	indexPath, err := resolveStrictSyncIndexPath(command.path, output)
	if err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp(filepath.Dir(indexPath), "gix-stash-index-")
	if err != nil {
		return nil, err
	}
	command.environment = map[string]string{
		gitIndexFileEnvironmentNameConstant: filepath.Join(directory, gitIndexPathNameConstant),
		gitLiteralPathspecsEnvironmentName:  "1",
	}
	return func() error { return os.RemoveAll(directory) }, nil
}
