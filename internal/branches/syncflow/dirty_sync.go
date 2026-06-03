package syncflow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tyemirov/gix/internal/commitmsg"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/repos/worktree"
	"github.com/tyemirov/gix/internal/workflow"
)

const (
	gitPathspecSeparatorConstant          = "--"
	strictSyncConflictWorktreeMessage     = "worktree has unresolved conflicts; resolve them before syncing"
	strictSyncDirtyCommitFailureTemplate  = "failed to save dirty work before syncing: %w"
	strictSyncDirtyStageFailureTemplate   = "failed to stage dirty sync cluster %q: %w"
	strictSyncDirtyMessageFailureTemplate = "failed to generate dirty sync commit message for %q: %w"
	strictSyncDirtyClusterFailureTemplate = "failed to commit dirty sync cluster %q: %w"
	strictSyncGeneratedBranchPrefix       = "sync"
	strictSyncGeneratedRepositoryFallback = "repository"
	strictSyncGeneratedPathFallback       = "work"
)

type syncCommitCluster struct {
	Root  string
	Paths []string
}

func saveDirtyWorkClusters(ctx context.Context, executor shared.GitExecutor, repositoryPath string, statusEntries []string, options worktreeAdoptionCommitMessageOptions) (int, error) {
	clusters := buildSyncCommitClusters(statusEntries)
	if len(clusters) == 0 {
		return 0, nil
	}

	client, clientErr := resolveCommitMessageClient(options)
	if clientErr != nil {
		return 0, clientErr
	}

	var temperature *float64
	if options.Temperature != 0 {
		temperatureValue := options.Temperature
		temperature = &temperatureValue
	}

	generator := commitmsg.Generator{
		GitExecutor: executor,
		Client:      client,
	}

	committedClusters := 0
	for clusterIndex := range clusters {
		cluster := clusters[clusterIndex]
		if resetErr := executeGit(ctx, executor, repositoryPath, []string{gitResetSubcommandConstant}); resetErr != nil {
			return committedClusters, resetErr
		}
		stageArguments := []string{gitAddSubcommandConstant, gitAddAllFlagConstant, gitPathspecSeparatorConstant}
		stageArguments = append(stageArguments, cluster.Paths...)
		if stageErr := executeGit(ctx, executor, repositoryPath, stageArguments); stageErr != nil {
			return committedClusters, fmt.Errorf(strictSyncDirtyStageFailureTemplate, cluster.Root, stageErr)
		}
		result, generateErr := generator.Generate(ctx, commitmsg.Options{
			RepositoryPath: repositoryPath,
			Source:         commitmsg.DiffSourceStaged,
			MaxTokens:      options.MaxTokens,
			Temperature:    temperature,
		})
		if generateErr != nil {
			return committedClusters, fmt.Errorf(strictSyncDirtyMessageFailureTemplate, cluster.Root, generateErr)
		}
		if commitErr := executeGit(ctx, executor, repositoryPath, []string{gitCommitSubcommandConstant, gitCommitMessageFlagConstant, result.Message}); commitErr != nil {
			return committedClusters, fmt.Errorf(strictSyncDirtyClusterFailureTemplate, cluster.Root, commitErr)
		}
		committedClusters++
	}

	return committedClusters, nil
}

func prepareStrictSyncBranchForDirtyWork(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, remoteName string, baseBranch string, branchName string, commitMessages worktreeAdoptionCommitMessageOptions) error {
	remoteReference := fmt.Sprintf("%s/%s", remoteName, branchName)
	remoteExists, remoteExistsErr := remoteReferenceExists(ctx, environment.GitExecutor, repository.Path, remoteReference)
	if remoteExistsErr != nil {
		return remoteExistsErr
	}

	if remoteExists {
		repositoryIdentifier := strictSyncRepositoryIdentifier(repository)
		if repositoryIdentifier == "" {
			return errors.New(strictSyncMissingRepositoryMessage)
		}
		openPullRequest, pullRequestErr := branchHasOpenPullRequest(ctx, environment, repositoryIdentifier, baseBranch, branchName)
		if pullRequestErr != nil {
			return pullRequestErr
		}
		if !openPullRequest {
			return fmt.Errorf(strictSyncMissingPullRequestTemplate, branchName, baseBranch)
		}
		return switchToLocalOrRemoteBranchWithAdoption(ctx, environment, repository, remoteName, branchName, commitMessages)
	}

	localExists, localExistsErr := localBranchExists(ctx, environment.GitExecutor, repository.Path, branchName)
	if localExistsErr != nil {
		return localExistsErr
	}
	if localExists {
		return switchToLocalOrRemoteBranchWithAdoption(ctx, environment, repository, remoteName, branchName, commitMessages)
	}

	baseReference := fmt.Sprintf("%s/%s", remoteName, baseBranch)
	baseExists, baseExistsErr := remoteReferenceExists(ctx, environment.GitExecutor, repository.Path, baseReference)
	if baseExistsErr != nil {
		return baseExistsErr
	}
	if !baseExists {
		return fmt.Errorf("remote base branch %q does not exist", baseReference)
	}
	return executeGit(ctx, environment.GitExecutor, repository.Path, []string{gitSwitchSubcommandConstant, gitCreateBranchFlagConstant, branchName, baseReference})
}

func buildSyncCommitClusters(statusEntries []string) []syncCommitCluster {
	if len(statusEntries) == 0 {
		return nil
	}

	clusterIndexes := map[string]int{}
	seenPaths := map[string]map[string]struct{}{}
	clusters := []syncCommitCluster{}
	for entryIndex := range statusEntries {
		entry := strings.TrimSpace(statusEntries[entryIndex])
		if entry == "" {
			continue
		}
		statusPath := normalizeSyncStatusPath(worktree.StatusEntryPath(entry))
		if statusPath == "" {
			continue
		}
		clusterRoot := syncCommitClusterRoot(statusPath)
		clusterIndex, exists := clusterIndexes[clusterRoot]
		if !exists {
			clusterIndex = len(clusters)
			clusterIndexes[clusterRoot] = clusterIndex
			seenPaths[clusterRoot] = map[string]struct{}{}
			clusters = append(clusters, syncCommitCluster{Root: clusterRoot})
		}

		paths := syncStatusEntryPaths(entry)
		for pathIndex := range paths {
			normalizedPath := normalizeSyncStatusPath(paths[pathIndex])
			if normalizedPath == "" {
				continue
			}
			if _, exists := seenPaths[clusterRoot][normalizedPath]; exists {
				continue
			}
			seenPaths[clusterRoot][normalizedPath] = struct{}{}
			clusters[clusterIndex].Paths = append(clusters[clusterIndex].Paths, normalizedPath)
		}
	}

	return clusters
}

func generatedSyncBranchName(repository *workflow.RepositoryState, statusEntries []string) string {
	repositoryName := strictSyncGeneratedRepositoryFallback
	if repository != nil {
		candidate := filepath.Base(filepath.Clean(strings.TrimSpace(repository.Path)))
		if slug := syncBranchSlug(candidate); slug != "" {
			repositoryName = slug
		}
	}

	pathName := strictSyncGeneratedPathFallback
	clusters := buildSyncCommitClusters(statusEntries)
	if len(clusters) > 0 {
		rootLabel := syncGeneratedBranchPathLabel(clusters[0].Root)
		if slug := syncBranchSlug(rootLabel); slug != "" {
			pathName = slug
		}
	}

	return strings.Join([]string{strictSyncGeneratedBranchPrefix, repositoryName, pathName}, "/")
}

func syncStatusEntriesHaveConflicts(statusEntries []string) bool {
	for entryIndex := range statusEntries {
		if syncStatusEntryHasConflict(statusEntries[entryIndex]) {
			return true
		}
	}
	return false
}

func syncStatusEntryHasConflict(statusEntry string) bool {
	trimmed := strings.TrimSpace(statusEntry)
	if len(trimmed) < 2 {
		return false
	}
	indexStatus := trimmed[0]
	worktreeStatus := trimmed[1]
	return indexStatus == 'U' || worktreeStatus == 'U' || (indexStatus == 'A' && worktreeStatus == 'A') || (indexStatus == 'D' && worktreeStatus == 'D')
}

func syncStatusEntryPaths(statusEntry string) []string {
	trimmed := strings.TrimSpace(statusEntry)
	if len(trimmed) <= 2 {
		return nil
	}
	pathPart := strings.TrimSpace(trimmed[2:])
	if pathPart == "" {
		return nil
	}
	if strings.Contains(pathPart, " -> ") {
		sections := strings.Split(pathPart, " -> ")
		paths := make([]string, 0, len(sections))
		for sectionIndex := range sections {
			paths = append(paths, strings.TrimSpace(sections[sectionIndex]))
		}
		return paths
	}
	return []string{pathPart}
}

func syncCommitClusterRoot(statusPath string) string {
	normalized := normalizeSyncStatusPath(statusPath)
	if normalized == "" {
		return ""
	}
	sections := strings.Split(normalized, "/")
	return strings.TrimSpace(sections[0])
}

func normalizeSyncStatusPath(path string) string {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return ""
	}
	normalized := filepath.ToSlash(filepath.Clean(trimmedPath))
	if normalized == "." {
		return ""
	}
	return strings.TrimPrefix(normalized, "./")
}

func syncGeneratedBranchPathLabel(root string) string {
	trimmedRoot := strings.TrimSpace(root)
	if trimmedRoot == "" {
		return ""
	}
	if strings.HasPrefix(trimmedRoot, ".") {
		return strings.TrimLeft(trimmedRoot, ".")
	}
	extension := filepath.Ext(trimmedRoot)
	if extension == "" {
		return trimmedRoot
	}
	withoutExtension := strings.TrimSuffix(trimmedRoot, extension)
	if withoutExtension == "" {
		return trimmedRoot
	}
	return withoutExtension
}

func syncBranchSlug(value string) string {
	lowerValue := strings.ToLower(strings.TrimSpace(value))
	slugBuilder := strings.Builder{}
	lastWasSeparator := false
	for characterIndex := 0; characterIndex < len(lowerValue); characterIndex++ {
		character := lowerValue[characterIndex]
		if syncSlugCharacterAllowed(character) {
			slugBuilder.WriteByte(character)
			lastWasSeparator = false
			continue
		}
		if slugBuilder.Len() == 0 || lastWasSeparator {
			continue
		}
		slugBuilder.WriteByte('-')
		lastWasSeparator = true
	}
	return strings.Trim(slugBuilder.String(), "-")
}

func syncSlugCharacterAllowed(character byte) bool {
	return (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9')
}
