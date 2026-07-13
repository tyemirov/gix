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
	strictSyncDirtyIgnoreFailureTemplate  = "failed to inspect ignored dirty sync paths: %w"
	strictSyncDirtyRestoreFailureTemplate = "failed to restore ignored dirty sync paths: %w"
	strictSyncGeneratedBranchPrefix       = "gix"
	strictSyncGeneratedSemanticFallback   = "work"
	strictSyncGeneratedSemanticSlugLimit  = 56
	strictSyncGeneratedBranchFailure      = "failed to generate dirty sync branch name: %w"
	strictSyncGeneratedBranchLimit        = 100
	strictSyncGeneratedBranchLimitMessage = "unable to select generated sync branch after 100 attempts for %q"
)

var syncConventionalCommitTypes = map[string]struct{}{
	"build":    {},
	"chore":    {},
	"ci":       {},
	"docs":     {},
	"feat":     {},
	"fix":      {},
	"perf":     {},
	"refactor": {},
	"revert":   {},
	"style":    {},
	"test":     {},
}

type syncCommitCluster struct {
	Root  string
	Paths []string
}

type syncStatusFilterResult struct {
	StageableEntries      []string
	IgnoredTrackedEntries []string
}

type strictSyncDirtyBranchStartPoint uint8

const (
	strictSyncDirtyBranchStartCurrentCheckout strictSyncDirtyBranchStartPoint = iota
	strictSyncDirtyBranchStartRemoteBase
)

func saveDirtyWorkClusters(ctx context.Context, executor shared.GitExecutor, repositoryPath string, statusEntries []string, options worktreeAdoptionCommitMessageOptions) (int, error) {
	clusters := buildSyncCommitClusters(statusEntries)
	if len(clusters) == 0 {
		return 0, nil
	}
	filteredClusters, filterErr := filterIgnoredSyncCommitClusters(ctx, executor, repositoryPath, clusters)
	if filterErr != nil {
		return 0, filterErr
	}
	clusters = filteredClusters
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

func filterIgnoredSyncCommitClusters(ctx context.Context, executor shared.GitExecutor, repositoryPath string, clusters []syncCommitCluster) ([]syncCommitCluster, error) {
	clusterPaths := syncCommitClusterPaths(clusters)
	if len(clusterPaths) == 0 {
		return nil, nil
	}
	ignoredPaths, ignoredErr := shared.CheckIgnoredPaths(ctx, executor, repositoryPath, clusterPaths)
	if ignoredErr != nil {
		return nil, fmt.Errorf(strictSyncDirtyIgnoreFailureTemplate, ignoredErr)
	}
	if len(ignoredPaths) == 0 {
		return clusters, nil
	}

	filteredClusters := make([]syncCommitCluster, 0, len(clusters))
	for clusterIndex := range clusters {
		cluster := clusters[clusterIndex]
		filteredPaths := make([]string, 0, len(cluster.Paths))
		for pathIndex := range cluster.Paths {
			path := cluster.Paths[pathIndex]
			if _, ignored := ignoredPaths[path]; ignored {
				continue
			}
			filteredPaths = append(filteredPaths, path)
		}
		if len(filteredPaths) == 0 {
			continue
		}
		filteredClusters = append(filteredClusters, syncCommitCluster{
			Root:  cluster.Root,
			Paths: filteredPaths,
		})
	}
	return filteredClusters, nil
}

func filterIgnoredSyncStatusEntries(ctx context.Context, executor shared.GitExecutor, repositoryPath string, statusEntries []string) (syncStatusFilterResult, error) {
	statusPaths := syncStatusEntriesPaths(statusEntries)
	if len(statusPaths) == 0 {
		return syncStatusFilterResult{}, nil
	}
	ignoredPaths, ignoredErr := shared.CheckIgnoredPaths(ctx, executor, repositoryPath, statusPaths)
	if ignoredErr != nil {
		return syncStatusFilterResult{}, fmt.Errorf(strictSyncDirtyIgnoreFailureTemplate, ignoredErr)
	}
	if len(ignoredPaths) == 0 {
		return syncStatusFilterResult{StageableEntries: statusEntries}, nil
	}

	result := syncStatusFilterResult{
		StageableEntries:      make([]string, 0, len(statusEntries)),
		IgnoredTrackedEntries: make([]string, 0, len(ignoredPaths)),
	}
	for entryIndex := range statusEntries {
		entryPaths := syncStatusEntryPaths(statusEntries[entryIndex])
		hasIgnoredPath := false
		hasStageablePath := false
		for pathIndex := range entryPaths {
			normalizedPath := normalizeSyncStatusPath(entryPaths[pathIndex])
			if normalizedPath == "" {
				continue
			}
			if _, ignored := ignoredPaths[normalizedPath]; ignored {
				hasIgnoredPath = true
				continue
			}
			hasStageablePath = true
		}
		if hasStageablePath {
			result.StageableEntries = append(result.StageableEntries, statusEntries[entryIndex])
			continue
		}
		trackedEntries, _ := worktree.SplitStatusEntries([]string{statusEntries[entryIndex]}, nil)
		if hasIgnoredPath && len(trackedEntries) > 0 {
			result.IgnoredTrackedEntries = append(result.IgnoredTrackedEntries, statusEntries[entryIndex])
		}
	}
	return result, nil
}

func restoreIgnoredSyncStatusEntries(ctx context.Context, executor shared.GitExecutor, repositoryPath string, statusEntries []string) error {
	ignoredTrackedPaths := syncStatusEntriesPaths(statusEntries)
	if len(ignoredTrackedPaths) == 0 {
		return nil
	}
	restoreArguments := []string{gitRestoreSubcommandConstant, "--staged", "--worktree", gitPathspecSeparatorConstant}
	restoreArguments = append(restoreArguments, ignoredTrackedPaths...)
	if restoreErr := executeGit(ctx, executor, repositoryPath, restoreArguments); restoreErr != nil {
		return fmt.Errorf(strictSyncDirtyRestoreFailureTemplate, restoreErr)
	}
	return nil
}

func syncCommitClusterPaths(clusters []syncCommitCluster) []string {
	paths := make([]string, 0)
	seenPaths := make(map[string]struct{})
	for clusterIndex := range clusters {
		cluster := clusters[clusterIndex]
		for pathIndex := range cluster.Paths {
			path := cluster.Paths[pathIndex]
			if _, seen := seenPaths[path]; seen {
				continue
			}
			seenPaths[path] = struct{}{}
			paths = append(paths, path)
		}
	}
	return paths
}

func syncStatusEntriesPaths(statusEntries []string) []string {
	paths := make([]string, 0)
	seenPaths := make(map[string]struct{})
	for entryIndex := range statusEntries {
		entryPaths := syncStatusEntryPaths(statusEntries[entryIndex])
		for pathIndex := range entryPaths {
			path := normalizeSyncStatusPath(entryPaths[pathIndex])
			if path == "" {
				continue
			}
			if _, seen := seenPaths[path]; seen {
				continue
			}
			seenPaths[path] = struct{}{}
			paths = append(paths, path)
		}
	}
	return paths
}

func prepareStrictSyncBranchForDirtyWork(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, remoteName string, baseBranch string, branchName string, startPoint strictSyncDirtyBranchStartPoint, commitMessages worktreeAdoptionCommitMessageOptions) error {
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
		openPullRequest, pullRequestErr := openPullRequestForBranch(ctx, environment, repositoryIdentifier, branchName)
		if pullRequestErr != nil {
			return pullRequestErr
		}
		if openPullRequest == nil {
			mergedPullRequest, mergedPullRequestErr := branchHasMergedPullRequest(ctx, environment, repositoryIdentifier, baseBranch, branchName)
			if mergedPullRequestErr != nil {
				return mergedPullRequestErr
			}
			if mergedPullRequest {
				return fmt.Errorf(strictSyncStackedMergedDirtyTemplate, branchName)
			}
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
	if startPoint == strictSyncDirtyBranchStartRemoteBase {
		return createStrictSyncBranchFromReference(ctx, environment.GitExecutor, repository.Path, branchName, baseReference)
	}
	return createStrictSyncBranchFromCurrentCheckout(ctx, environment.GitExecutor, repository.Path, branchName)
}

func createStrictSyncBranchFromCurrentCheckout(ctx context.Context, executor shared.GitExecutor, repositoryPath string, branchName string) error {
	return executeGit(ctx, executor, repositoryPath, []string{gitSwitchSubcommandConstant, gitCreateBranchFlagConstant, branchName})
}

func createStrictSyncBranchFromReference(ctx context.Context, executor shared.GitExecutor, repositoryPath string, branchName string, startReference string) error {
	return executeGit(ctx, executor, repositoryPath, []string{gitSwitchSubcommandConstant, gitCreateBranchFlagConstant, branchName, startReference})
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

func generatedSyncBranchName(ctx context.Context, executor shared.GitExecutor, repositoryPath string, options worktreeAdoptionCommitMessageOptions) (string, error) {
	message, messageErr := generateSyncBranchMessage(ctx, executor, repositoryPath, options)
	if messageErr != nil {
		return "", fmt.Errorf(strictSyncGeneratedBranchFailure, messageErr)
	}
	slug := syncBranchSlug(syncBranchSemanticSubject(message))
	if slug == "" {
		slug = strictSyncGeneratedSemanticFallback
	}
	slug = truncateSyncBranchSlug(slug, "")
	return strings.Join([]string{strictSyncGeneratedBranchPrefix, slug}, "/"), nil
}

func generateSyncBranchMessage(ctx context.Context, executor shared.GitExecutor, repositoryPath string, options worktreeAdoptionCommitMessageOptions) (string, error) {
	client, clientErr := resolveCommitMessageClient(options)
	if clientErr != nil {
		return "", clientErr
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
	result, generateErr := generator.Generate(ctx, commitmsg.Options{
		RepositoryPath: repositoryPath,
		Source:         commitmsg.DiffSourceAll,
		MaxTokens:      options.MaxTokens,
		Temperature:    temperature,
	})
	if generateErr != nil {
		return "", generateErr
	}
	return result.Message, nil
}

func selectGeneratedSyncBranchName(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, remoteName string, baseBranch string, options worktreeAdoptionCommitMessageOptions) (string, error) {
	initialBranchName, initialBranchErr := generatedSyncBranchName(ctx, environment.GitExecutor, repository.Path, options)
	if initialBranchErr != nil {
		return "", initialBranchErr
	}
	repositoryIdentifier := strictSyncRepositoryIdentifier(repository)
	for candidateIndex := 0; candidateIndex < strictSyncGeneratedBranchLimit; candidateIndex++ {
		candidateBranchName := generatedSyncBranchCandidateName(initialBranchName, candidateIndex)
		remoteReference := fmt.Sprintf("%s/%s", remoteName, candidateBranchName)
		remoteExists, remoteExistsErr := remoteReferenceExists(ctx, environment.GitExecutor, repository.Path, remoteReference)
		if remoteExistsErr != nil {
			return "", remoteExistsErr
		}
		if !remoteExists {
			localExists, localExistsErr := localBranchExists(ctx, environment.GitExecutor, repository.Path, candidateBranchName)
			if localExistsErr != nil {
				return "", localExistsErr
			}
			if localExists {
				continue
			}
			return candidateBranchName, nil
		}
		if repositoryIdentifier == "" {
			return "", errors.New(strictSyncMissingRepositoryMessage)
		}
		openPullRequest, pullRequestErr := openPullRequestForBranch(ctx, environment, repositoryIdentifier, candidateBranchName)
		if pullRequestErr != nil {
			return "", pullRequestErr
		}
		if openPullRequest != nil {
			return candidateBranchName, nil
		}
	}
	return "", fmt.Errorf(strictSyncGeneratedBranchLimitMessage, initialBranchName)
}

func generatedSyncBranchCandidateName(initialBranchName string, candidateIndex int) string {
	if candidateIndex == 0 {
		return initialBranchName
	}
	suffix := fmt.Sprintf("-%d", candidateIndex+1)
	prefix, slug, found := strings.Cut(initialBranchName, "/")
	if !found {
		return truncateSyncBranchSlug(initialBranchName, suffix) + suffix
	}
	return prefix + "/" + truncateSyncBranchSlug(slug, suffix) + suffix
}

func truncateSyncBranchSlug(slug string, suffix string) string {
	availableLength := strictSyncGeneratedSemanticSlugLimit - len(suffix)
	if availableLength <= 0 {
		return strictSyncGeneratedSemanticFallback
	}
	trimmedSlug := strings.Trim(slug, "-")
	if len(trimmedSlug) <= availableLength {
		if trimmedSlug == "" {
			return strictSyncGeneratedSemanticFallback
		}
		return trimmedSlug
	}
	rawTruncated := trimmedSlug[:availableLength]
	truncated := strings.Trim(rawTruncated, "-")
	if trimmedSlug[availableLength] != '-' && !strings.HasSuffix(rawTruncated, "-") {
		if separatorIndex := strings.LastIndex(truncated, "-"); separatorIndex > 0 {
			truncated = strings.Trim(truncated[:separatorIndex], "-")
		}
	}
	if truncated == "" {
		return strictSyncGeneratedSemanticFallback
	}
	return truncated
}

func syncBranchSemanticSubject(message string) string {
	lines := strings.Split(message, "\n")
	for lineIndex := range lines {
		trimmed := strings.TrimSpace(lines[lineIndex])
		if trimmed == "" {
			continue
		}
		return strings.TrimSpace(stripConventionalCommitPrefix(trimmed))
	}
	return ""
}

func stripConventionalCommitPrefix(subject string) string {
	colonIndex := strings.Index(subject, ":")
	if colonIndex <= 0 {
		return subject
	}
	prefix := strings.TrimSpace(subject[:colonIndex])
	normalizedType := strings.TrimSuffix(prefix, "!")
	if scopeIndex := strings.Index(normalizedType, "("); scopeIndex >= 0 {
		if !strings.HasSuffix(normalizedType, ")") {
			return subject
		}
		normalizedType = strings.TrimSpace(normalizedType[:scopeIndex])
	}
	if _, exists := syncConventionalCommitTypes[normalizedType]; !exists {
		return subject
	}
	return subject[colonIndex+1:]
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
