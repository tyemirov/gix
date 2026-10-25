package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tyemirov/gix/internal/execshell"
	reposdeps "github.com/tyemirov/gix/internal/repos/dependencies"
	"github.com/tyemirov/gix/internal/repos/shared"
	flagutils "github.com/tyemirov/gix/internal/utils/flags"
)

const (
	discardCommandName = "discard"
	discardAllFlagName = "all"
	discardNUL         = "\x00"
)

var discardOperationMarkers = []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply", "sequencer", "BISECT_START"}

type discardTreeEntry struct {
	objectType string
	objectID   string
}

type discardFileIdentity struct {
	device uint64
	inode  uint64
}

type discardGit struct {
	executor  shared.GitExecutor
	directory string
}

func (git discardGit) execute(ctx context.Context, input []byte, arguments ...string) (string, error) {
	result, err := git.executor.ExecuteGit(ctx, execshell.CommandDetails{WorkingDirectory: git.directory, Arguments: arguments, StandardInput: input})
	if err != nil {
		return "", fmt.Errorf("discard: git %s in %q: %w", arguments[0], git.directory, err)
	}
	return result.StandardOutput, nil
}

func (application *Application) newDiscardCommand() *cobra.Command {
	var all bool
	command := &cobra.Command{
		Use:   discardCommandName + " <path>... | --all",
		Short: "Discard staged and unstaged tracked changes in the current repository",
		Long:  "Restore tracked paths to the current HEAD commit. Paths are literal and relative to the current directory. --all selects the complete repository. Local commits, refs, and untracked or ignored files are preserved. In-progress operations, sparse paths, and changed submodule entries are rejected. Submodule checkouts are not changed.",
		Args: func(command *cobra.Command, arguments []string) error {
			for _, flagName := range []string{flagutils.DefaultRootFlagName, flagutils.RemoteFlagName, webFlagNameConstant, webBindFlagNameConstant, webPortFlagNameConstant, versionFlagNameConstant} {
				if command.Flags().Changed(flagName) {
					return fmt.Errorf("discard does not support --%s; it uses the current repository", flagName)
				}
			}
			if len(arguments) > 0 && command.Flags().Changed(discardAllFlagName) {
				return fmt.Errorf("discard requires paths or --all, not both")
			}
			if len(arguments) == 0 && !all {
				return fmt.Errorf("discard requires at least one path or --all")
			}
			return nil
		},
		RunE: func(command *cobra.Command, arguments []string) error {
			return application.runDiscard(command, arguments)
		},
	}
	command.Flags().BoolVar(&all, discardAllFlagName, false, "Discard all tracked changes in the current repository")
	return command
}

func (application *Application) runDiscard(command *cobra.Command, selectors []string) error {
	ctx := command.Context()
	executor, err := reposdeps.ResolveGitExecutor(nil, application.logger, false)
	if err != nil {
		return fmt.Errorf("discard: create Git executor: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("discard: read current directory: %w", err)
	}
	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return fmt.Errorf("discard: resolve current directory: %w", err)
	}
	git := discardGit{executor: executor, directory: cwd}
	rootOutput, err := git.execute(ctx, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	root, err := filepath.EvalSymlinks(strings.TrimSuffix(rootOutput, "\n"))
	if err != nil {
		return fmt.Errorf("discard: resolve repository root: %w", err)
	}
	git.directory = root
	headOutput, err := git.execute(ctx, nil, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return fmt.Errorf("discard requires an existing HEAD commit: %w", err)
	}
	head := strings.TrimSpace(headOutput)
	if err := git.validateOperationState(ctx); err != nil {
		return err
	}
	treeOutput, err := git.execute(ctx, nil, "ls-tree", "-r", "-z", "--full-tree", "--format=%(objecttype)%x00%(objectname)", head)
	if err != nil {
		return err
	}
	tree := make(map[string]discardTreeEntry)
	paths := make(map[string]struct{})
	treeFields := discardFields(treeOutput)
	treePathsOutput, err := git.execute(ctx, nil, "ls-tree", "-r", "-z", "--full-tree", "--name-only", head)
	if err != nil {
		return err
	}
	treePaths := discardFields(treePathsOutput)
	if len(treeFields) != 2*len(treePaths) {
		return fmt.Errorf("discard: invalid Git tree records")
	}
	for index := 0; index < len(treeFields); index += 2 {
		path := treePaths[index/2]
		tree[path] = discardTreeEntry{objectType: treeFields[index], objectID: treeFields[index+1]}
		paths[path] = struct{}{}
	}
	indexOutput, err := git.execute(ctx, nil, "ls-files", "--cached", "-z", "--format=%(objectname)%x00%(path)")
	if err != nil {
		return err
	}
	indexEntries := make(map[string]string)
	indexFields := discardFields(indexOutput)
	if len(indexFields)%2 != 0 {
		return fmt.Errorf("discard: invalid Git index records")
	}
	for index := 0; index < len(indexFields); index += 2 {
		path := indexFields[index+1]
		indexEntries[path] = indexFields[index]
		paths[path] = struct{}{}
	}
	selected, err := discardSelectPaths(root, cwd, selectors, paths, tree)
	if err != nil {
		return err
	}
	selected, err = git.validateEntries(ctx, selected, tree, indexEntries)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		_, err = fmt.Fprintln(command.OutOrStdout(), "No tracked paths to discard.")
		return err
	}
	if err := git.validateSparsePaths(ctx, selected); err != nil {
		return err
	}
	if err := git.validateUntrackedPaths(ctx, selected, tree, paths); err != nil {
		return err
	}
	pathInput := []byte(strings.Join(discardPathspecs(selected), discardNUL) + discardNUL)
	_, err = git.execute(ctx, pathInput, "--literal-pathspecs", "restore", "--source="+head, "--staged", "--worktree", "--no-recurse-submodules", "--pathspec-from-file=-", "--pathspec-file-nul")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(command.OutOrStdout(), "Restored %d tracked path(s) to HEAD.\n", len(selected))
	return err
}

func discardFields(output string) []string {
	if output == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(output, discardNUL), discardNUL)
}

func (git discardGit) validateOperationState(ctx context.Context) error {
	for _, marker := range discardOperationMarkers {
		markerOutput, err := git.execute(ctx, nil, "rev-parse", "--path-format=absolute", "--git-path", marker)
		if err != nil {
			return err
		}
		markerPath := strings.TrimSuffix(markerOutput, "\n")
		if _, err := os.Lstat(markerPath); err == nil {
			return fmt.Errorf("discard: Git operation is in progress (%s)", marker)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("discard: inspect Git operation %s: %w", marker, err)
		}
	}
	unmerged, err := git.execute(ctx, nil, "diff", "--name-only", "--diff-filter=U", "-z")
	if err != nil {
		return err
	}
	if unmerged != "" {
		return fmt.Errorf("discard: unmerged paths require completion of the Git operation")
	}
	return nil
}

func discardSelectPaths(root, cwd string, selectors []string, paths map[string]struct{}, tree map[string]discardTreeEntry) ([]string, error) {
	selected := make(map[string]struct{})
	if len(selectors) == 0 {
		selected = paths
	}
	for _, selector := range selectors {
		if selector == "" {
			return nil, fmt.Errorf("discard: empty path selector")
		}
		absolute := selector
		if !filepath.IsAbs(selector) {
			absolute = filepath.Join(cwd, selector)
		}
		relative, err := filepath.Rel(root, absolute)
		if err != nil {
			return nil, fmt.Errorf("discard: resolve selector %q: %w", selector, err)
		}
		if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("discard: path %q is outside the current repository", selector)
		}
		relative = filepath.ToSlash(relative)
		for path, entry := range tree {
			if entry.objectType == "commit" && (relative == path || strings.HasPrefix(relative, path+"/")) {
				return nil, fmt.Errorf("discard: submodule path %q is not supported", selector)
			}
		}
		matched := false
		for path := range paths {
			if relative == "." || path == relative || strings.HasPrefix(path, relative+"/") {
				selected[path] = struct{}{}
				matched = true
			}
		}
		if !matched {
			return nil, fmt.Errorf("discard: path %q does not match a tracked file or directory", selector)
		}
	}
	result := make([]string, 0, len(selected))
	for path := range selected {
		result = append(result, path)
	}
	sort.Strings(result)
	return result, nil
}

func (git discardGit) validateEntries(ctx context.Context, selected []string, tree map[string]discardTreeEntry, indexEntries map[string]string) ([]string, error) {
	result := make([]string, 0, len(selected))
	objects := make(map[string]struct{})
	for _, path := range selected {
		headEntry := tree[path]
		indexObject := indexEntries[path]
		if headEntry.objectType == "commit" {
			if indexObject != headEntry.objectID {
				return nil, fmt.Errorf("discard: changed submodule entry %q is not supported", path)
			}
			continue
		}
		if indexObject != "" && indexObject != headEntry.objectID {
			objects[indexObject] = struct{}{}
		}
		result = append(result, path)
	}
	if len(objects) == 0 {
		return result, nil
	}
	objectIDs := make([]string, 0, len(objects))
	for objectID := range objects {
		objectIDs = append(objectIDs, objectID)
	}
	sort.Strings(objectIDs)
	output, err := git.execute(ctx, []byte(strings.Join(objectIDs, "\n")+"\n"), "cat-file", "--batch-check=%(objectname) %(objecttype)")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(strings.TrimSuffix(output, "\n"), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[1] != "blob" {
			return nil, fmt.Errorf("discard: index entry %q is not a supported file; changed submodule entries are not supported", line)
		}
	}
	return result, nil
}

func (git discardGit) validateSparsePaths(ctx context.Context, selected []string) error {
	output, err := git.execute(ctx, nil, "ls-files", "-v", "-z")
	if err != nil {
		return err
	}
	sparse := make(map[string]struct{})
	for _, record := range discardFields(output) {
		if len(record) < 3 || record[1] != ' ' {
			return fmt.Errorf("discard: invalid Git sparse record %q", record)
		}
		if strings.ToUpper(record[:1]) == "S" {
			sparse[record[2:]] = struct{}{}
		}
	}
	for _, path := range selected {
		if _, exists := sparse[path]; exists {
			return fmt.Errorf("discard: sparse path %q is not supported", path)
		}
	}
	return nil
}

func (git discardGit) validateUntrackedPaths(ctx context.Context, selected []string, tree map[string]discardTreeEntry, trackedPaths map[string]struct{}) error {
	output, err := git.execute(ctx, nil, "ls-files", "--others", "-z")
	if err != nil {
		return err
	}
	untrackedPaths := discardFields(output)
	restoredPaths := make(map[string]struct{}, len(selected))
	restoredAncestors := make(map[string]string)
	for _, path := range selected {
		if _, exists := tree[path]; !exists {
			continue
		}
		restoredPaths[path] = struct{}{}
		for prefix := path; prefix != "."; prefix = filepath.ToSlash(filepath.Dir(prefix)) {
			if _, exists := restoredAncestors[prefix]; exists {
				break
			}
			restoredAncestors[prefix] = path
		}
	}
	for index, untracked := range untrackedPaths {
		untracked = strings.TrimSuffix(untracked, "/")
		untrackedPaths[index] = untracked
		if path, exists := restoredAncestors[untracked]; exists {
			return fmt.Errorf("discard: untracked path %q obstructs restoration of %q", untracked, path)
		}
		for parent := filepath.ToSlash(filepath.Dir(untracked)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			if _, exists := restoredPaths[parent]; exists {
				return fmt.Errorf("discard: untracked path %q obstructs restoration of %q", untracked, parent)
			}
		}
	}
	protectedPaths := make(map[string]struct{}, len(trackedPaths)+len(untrackedPaths))
	for path := range trackedPaths {
		protectedPaths[path] = struct{}{}
	}
	for _, path := range selected {
		delete(protectedPaths, path)
	}
	for _, path := range untrackedPaths {
		protectedPaths[path] = struct{}{}
	}
	return git.validateFilesystemAliases(selected, protectedPaths)
}

func discardPathspecs(selected []string) []string {
	specs := make([]string, 0, len(selected))
	selectedSet := make(map[string]struct{}, len(selected))
	for _, path := range selected {
		selectedSet[path] = struct{}{}
	}
	for _, path := range selected {
		covered := false
		for parent := filepath.ToSlash(filepath.Dir(path)); parent != "."; parent = filepath.ToSlash(filepath.Dir(parent)) {
			if _, exists := selectedSet[parent]; exists {
				covered = true
				break
			}
		}
		if !covered {
			specs = append(specs, path)
		}
	}
	return specs
}

func (git discardGit) validateFilesystemAliases(selected []string, protectedPaths map[string]struct{}) error {
	if len(protectedPaths) == 0 {
		return nil
	}
	cache := make(map[string]*discardFileIdentity)
	inspect := func(path string) (*discardFileIdentity, error) {
		if identity, exists := cache[path]; exists {
			return identity, nil
		}
		info, err := os.Lstat(filepath.Join(git.directory, filepath.FromSlash(path)))
		if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
			cache[path] = nil
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("discard: inspect filesystem path %q: %w", path, err)
		}
		identity, err := discardFilesystemIdentity(filepath.Join(git.directory, filepath.FromSlash(path)), info)
		if err != nil {
			return nil, fmt.Errorf("discard: identify filesystem path %q: %w", path, err)
		}
		cache[path] = &identity
		return &identity, nil
	}
	selectedIdentities := make(map[discardFileIdentity]string)
	selectedAncestorIdentities := make(map[discardFileIdentity]string)
	indexedPrefixes := make(map[string]struct{})
	for _, path := range selected {
		identity, err := inspect(path)
		if err != nil {
			return err
		}
		if identity != nil {
			selectedIdentities[*identity] = path
		}
		for prefix := path; prefix != "."; prefix = filepath.ToSlash(filepath.Dir(prefix)) {
			if _, exists := indexedPrefixes[prefix]; exists {
				break
			}
			identity, err := inspect(prefix)
			if err != nil {
				return err
			}
			if identity != nil {
				selectedAncestorIdentities[*identity] = path
			}
			indexedPrefixes[prefix] = struct{}{}
		}
	}
	checkedPrefixes := make(map[string]struct{})
	for protectedPath := range protectedPaths {
		identity, err := inspect(protectedPath)
		if err != nil {
			return err
		}
		if identity != nil {
			if selectedPath, exists := selectedAncestorIdentities[*identity]; exists {
				return fmt.Errorf("discard: selected path %q aliases protected path %q on this filesystem", selectedPath, protectedPath)
			}
		}
		for prefix := protectedPath; prefix != "."; prefix = filepath.ToSlash(filepath.Dir(prefix)) {
			if _, exists := checkedPrefixes[prefix]; exists {
				break
			}
			identity, err := inspect(prefix)
			if err != nil {
				return err
			}
			if identity != nil {
				if selectedPath, exists := selectedIdentities[*identity]; exists {
					return fmt.Errorf("discard: selected path %q aliases protected path %q on this filesystem", selectedPath, protectedPath)
				}
			}
			checkedPrefixes[prefix] = struct{}{}
		}
	}
	return nil
}
