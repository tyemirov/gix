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
	Root           string
	TrackedPaths   []string
	UntrackedPaths []string
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
		if len(cluster.TrackedPaths) > 0 {
			trackedStageArguments := []string{gitAddSubcommandConstant, gitAddForceFlagConstant, gitAddAllFlagConstant, gitPathspecSeparatorConstant}
			trackedStageArguments = append(trackedStageArguments, cluster.TrackedPaths...)
			if stageErr := executeGit(ctx, executor, repositoryPath, trackedStageArguments); stageErr != nil {
				return committedClusters, fmt.Errorf(strictSyncDirtyStageFailureTemplate, cluster.Root, stageErr)
			}
		}
		if len(cluster.UntrackedPaths) > 0 {
			untrackedStageArguments := []string{gitAddSubcommandConstant, gitAddAllFlagConstant, gitPathspecSeparatorConstant}
			untrackedStageArguments = append(untrackedStageArguments, cluster.UntrackedPaths...)
			if stageErr := executeGit(ctx, executor, repositoryPath, untrackedStageArguments); stageErr != nil {
				return committedClusters, fmt.Errorf(strictSyncDirtyStageFailureTemplate, cluster.Root, stageErr)
			}
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

func filterIgnoredUntrackedSyncStatusEntries(statusEntries []string) []string {
	stageableEntries := make([]string, 0, len(statusEntries))
	for entryIndex := range statusEntries {
		entry := statusEntries[entryIndex]
		if strings.HasPrefix(strings.TrimLeft(entry, " \t"), "!!") {
			continue
		}
		stageableEntries = append(stageableEntries, entry)
	}
	return stageableEntries
}

func prepareStrictSyncBranchForDirtyWork(ctx context.Context, environment *workflow.Environment, repository *workflow.RepositoryState, remoteName string, baseBranch string, branchName string, startPoint strictSyncDirtyBranchStartPoint, commitMessages worktreeAdoptionCommitMessageOptions) error {
	remoteReference := fmt.Sprintf("%s/%s", remoteName, branchName)
	remoteExists, remoteExistsErr := remoteReferenceExists(ctx, environment.GitExecutor, repository.Path, remoteReference)
	if remoteExistsErr != nil {
		return remoteExistsErr
	}

	if remoteExists {
		if branchName == baseBranch {
			return switchToLocalOrRemoteBranchWithAdoption(ctx, environment, repository, remoteName, branchName, commitMessages)
		}
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
		trackedEntries, untrackedEntries := worktree.SplitStatusEntries([]string{entry}, nil)
		tracked := len(trackedEntries) > 0
		if !tracked && len(untrackedEntries) == 0 {
			continue
		}
		for pathIndex := range paths {
			normalizedPath := normalizeSyncStatusPath(paths[pathIndex])
			if normalizedPath == "" {
				continue
			}
			if _, exists := seenPaths[clusterRoot][normalizedPath]; exists {
				continue
			}
			seenPaths[clusterRoot][normalizedPath] = struct{}{}
			if tracked {
				clusters[clusterIndex].TrackedPaths = append(clusters[clusterIndex].TrackedPaths, normalizedPath)
				continue
			}
			clusters[clusterIndex].UntrackedPaths = append(clusters[clusterIndex].UntrackedPaths, normalizedPath)
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
	return worktree.StatusEntryPaths(statusEntry)
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
