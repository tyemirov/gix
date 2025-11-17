package audit

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/temirov/gix/internal/execshell"
	"github.com/temirov/gix/internal/repos/shared"
)

const gitMetadataDirectoryNameConstant = ".git"

// Service coordinates repository discovery, reporting, and reconciliation.
type Service struct {
	discoverer   RepositoryDiscoverer
	gitManager   GitRepositoryManager
	gitExecutor  GitExecutor
	githubClient GitHubMetadataResolver
	outputWriter io.Writer
	errorWriter  io.Writer
}

// NewService constructs a Service using the provided dependencies.
func NewService(discoverer RepositoryDiscoverer, gitManager GitRepositoryManager, gitExecutor GitExecutor, githubClient GitHubMetadataResolver, outputWriter io.Writer, errorWriter io.Writer) *Service {
	return &Service{
		discoverer:   discoverer,
		gitManager:   gitManager,
		gitExecutor:  gitExecutor,
		githubClient: githubClient,
		outputWriter: outputWriter,
		errorWriter:  errorWriter,
	}
}

// Run executes the service according to the provided options.
func (service *Service) Run(executionContext context.Context, options CommandOptions) error {
	roots := options.Roots
	if len(roots) == 0 {
		return errors.New(missingRootsErrorMessageConstant)
	}

	inspections, inspectionError := service.DiscoverInspections(executionContext, roots, options.IncludeAllFolders, options.DebugOutput, options.InspectionDepth)
	if inspectionError != nil {
		return inspectionError
	}

	return service.writeAuditReport(inspections)
}

// DiscoverInspections collects repository inspections for the provided roots.
func (service *Service) DiscoverInspections(executionContext context.Context, roots []string, includeAll bool, debug bool, inspectionDepth InspectionDepth) ([]RepositoryInspection, error) {
	normalizedDepth := normalizeInspectionDepth(inspectionDepth)

	normalizedRoots, rootsNormalizationError := normalizeRepositoryPaths(roots)
	if rootsNormalizationError != nil {
		return nil, rootsNormalizationError
	}

	repositories, discoveryError := service.discoverer.DiscoverRepositories(roots)
	if discoveryError != nil {
		return nil, discoveryError
	}

	normalizedRepositories, normalizationError := normalizeRepositoryPaths(repositories)
	if normalizationError != nil {
		return nil, normalizationError
	}

	filteredRepositories, filterError := shared.FilterIgnoredRepositories(executionContext, service.gitExecutor, normalizedRepositories)
	if filterError != nil {
		return nil, filterError
	}

	normalizedRepositories = filteredRepositories

	if debug {
		fmt.Fprintf(service.errorWriter, debugDiscoveredTemplate, len(repositories), strings.Join(roots, " "))
	}

	repositoryRootSet := make(map[string]struct{}, len(normalizedRepositories))
	for _, repositoryPath := range normalizedRepositories {
		repositoryRootSet[repositoryPath] = struct{}{}
	}

	candidatePaths := deduplicatePaths(normalizedRepositories)
	if includeAll {
		expandedCandidates, candidateError := collectAllFolders(roots)
		if candidateError != nil {
			return nil, candidateError
		}
		candidatePaths = mergeCandidatePaths(candidatePaths, expandedCandidates)
	}

	inspections := make([]RepositoryInspection, 0, len(candidatePaths))

	for _, repositoryPath := range candidatePaths {
		if includeAll && isPathWithinRepository(repositoryPath, repositoryRootSet) {
			continue
		}
		if debug {
			fmt.Fprintf(service.errorWriter, debugCheckingTemplate, repositoryPath)
		}

		folderName := relativeFolderName(repositoryPath, normalizedRoots)

		if !service.isGitRepository(executionContext, repositoryPath) {
			if includeAll {
				inspections = append(inspections, buildNonRepositoryInspection(repositoryPath, folderName))
			}
			continue
		}

		inspection, inspectError := service.inspectRepository(executionContext, repositoryPath, normalizedDepth)
		if inspectError != nil {
			continue
		}

		inspection.FolderName = folderName
		inspections = append(inspections, inspection)
	}

	return inspections, nil
}

func (service *Service) writeAuditReport(inspections []RepositoryInspection) error {
	csvWriter := csv.NewWriter(service.outputWriter)
	header := []string{
		csvHeaderFolderName,
		csvHeaderFinalRepository,
		csvHeaderNameMatches,
		csvHeaderRemoteDefault,
		csvHeaderLocalBranch,
		csvHeaderInSync,
		csvHeaderRemoteProtocol,
		csvHeaderOriginCanonical,
	}
	if writeError := csvWriter.Write(header); writeError != nil {
		return writeError
	}

	for inspectionIndex := range inspections {
		record := inspectionReportRow(inspections[inspectionIndex])
		if writeError := csvWriter.Write(record.CSVRecord()); writeError != nil {
			return writeError
		}
	}

	csvWriter.Flush()
	return csvWriter.Error()
}

func deduplicatePaths(paths []string) []string {
	seen := make(map[string]struct{})
	var unique []string
	for _, path := range paths {
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		unique = append(unique, path)
	}
	sort.Strings(unique)
	return unique
}

func normalizeRepositoryPaths(paths []string) ([]string, error) {
	normalized := make([]string, 0, len(paths))
	for _, repositoryPath := range paths {
		cleanedPath := filepath.Clean(repositoryPath)
		if filepath.IsAbs(cleanedPath) {
			normalized = append(normalized, cleanedPath)
			continue
		}

		absolutePath, absoluteError := filepath.Abs(cleanedPath)
		if absoluteError != nil {
			return nil, fmt.Errorf("%s: %w", normalizeRepositoryPathErrorMessageConstant, absoluteError)
		}

		normalized = append(normalized, absolutePath)
	}

	return normalized, nil
}

func normalizeInspectionDepth(depth InspectionDepth) InspectionDepth {
	switch depth {
	case InspectionDepthMinimal:
		return InspectionDepthMinimal
	default:
		return InspectionDepthFull
	}
}

func (service *Service) isGitRepository(executionContext context.Context, repositoryPath string) bool {
	commandDetails := execshell.CommandDetails{
		Arguments:        []string{gitRevParseSubcommandConstant, gitIsInsideWorkTreeFlagConstant},
		WorkingDirectory: repositoryPath,
	}

	executionResult, executionError := service.gitExecutor.ExecuteGit(executionContext, commandDetails)
	if executionError != nil {
		return false
	}

	return strings.TrimSpace(executionResult.StandardOutput) == gitTrueOutputConstant
}

func (service *Service) inspectRepository(executionContext context.Context, repositoryPath string, inspectionDepth InspectionDepth) (RepositoryInspection, error) {
	folderName := filepath.Base(repositoryPath)

	originURL, originError := service.gitManager.GetRemoteURL(executionContext, repositoryPath, shared.OriginRemoteNameConstant)
	if originError != nil || len(strings.TrimSpace(originURL)) == 0 {
		localBranch := ""
		if inspectionDepth == InspectionDepthFull {
			branchName, localBranchError := service.gitManager.GetCurrentBranch(executionContext, repositoryPath)
			if localBranchError == nil {
				localBranch = sanitizeBranchName(branchName)
			}
		}

		return RepositoryInspection{
			Path:                   repositoryPath,
			FolderName:             folderName,
			OriginURL:              "",
			OriginOwnerRepo:        "",
			CanonicalOwnerRepo:     "",
			FinalOwnerRepo:         "",
			DesiredFolderName:      folderName,
			RemoteProtocol:         RemoteProtocolOther,
			RemoteDefaultBranch:    "",
			LocalBranch:            localBranch,
			InSyncStatus:           TernaryValueNotApplicable,
			OriginMatchesCanonical: TernaryValueNotApplicable,
			IsGitRepository:        true,
		}, nil
	}

	if !strings.Contains(strings.ToLower(originURL), githubHostConstant) {
		return RepositoryInspection{}, errors.New(notGitHubRemoteMessageConstant)
	}

	originOwnerRepo, ownerError := canonicalizeOwnerRepo(originURL)
	if ownerError != nil {
		originOwnerRepo = ""
	}

	remoteProtocol := detectRemoteProtocol(originURL)

	canonicalOwnerRepo := ""
	remoteDefaultBranch := ""
	if service.githubClient != nil {
		metadata, metadataError := service.githubClient.ResolveRepoMetadata(executionContext, originOwnerRepo)
		if metadataError == nil {
			canonicalOwnerRepo = strings.TrimSpace(metadata.NameWithOwner)
			remoteDefaultBranch = strings.TrimSpace(metadata.DefaultBranch)
		}
	}

	if len(remoteDefaultBranch) == 0 {
		remoteDefaultBranch = service.resolveDefaultBranchFromGit(executionContext, repositoryPath)
	}

	localBranch := ""
	inSyncStatus := TernaryValueNotApplicable
	if inspectionDepth == InspectionDepthFull {
		branchName, localBranchError := service.gitManager.GetCurrentBranch(executionContext, repositoryPath)
		if localBranchError == nil {
			sanitizedBranch := sanitizeBranchName(branchName)
			localBranch = sanitizedBranch
			inSyncStatus = service.computeInSync(executionContext, repositoryPath, remoteDefaultBranch, sanitizedBranch, remoteProtocol)
		}
	}

	finalOwnerRepo := originOwnerRepo
	if len(strings.TrimSpace(canonicalOwnerRepo)) > 0 {
		finalOwnerRepo = canonicalOwnerRepo
	}

	inspection := RepositoryInspection{
		Path:                   repositoryPath,
		FolderName:             folderName,
		OriginURL:              originURL,
		OriginOwnerRepo:        originOwnerRepo,
		CanonicalOwnerRepo:     canonicalOwnerRepo,
		FinalOwnerRepo:         finalOwnerRepo,
		DesiredFolderName:      finalRepositoryName(finalOwnerRepo),
		RemoteProtocol:         remoteProtocol,
		RemoteDefaultBranch:    remoteDefaultBranch,
		LocalBranch:            localBranch,
		InSyncStatus:           inSyncStatus,
		OriginMatchesCanonical: matchesCanonical(originOwnerRepo, canonicalOwnerRepo),
		IsGitRepository:        true,
	}
	return inspection, nil
}

func matchesCanonical(origin string, canonical string) TernaryValue {
	if len(strings.TrimSpace(origin)) == 0 || len(strings.TrimSpace(canonical)) == 0 {
		return TernaryValueNotApplicable
	}
	if ownerRepoCaseInsensitiveEqual(origin, canonical) {
		return TernaryValueYes
	}
	return TernaryValueNo
}

func inspectionReportRow(inspection RepositoryInspection) AuditReportRow {
	finalRepo := inspection.CanonicalOwnerRepo
	if len(strings.TrimSpace(finalRepo)) == 0 {
		finalRepo = inspection.OriginOwnerRepo
	}
	nameMatches := TernaryValueNotApplicable
	if inspection.IsGitRepository {
		nameMatches = TernaryValueNo
		folderBaseName := filepath.Base(inspection.FolderName)
		if len(inspection.DesiredFolderName) > 0 && inspection.DesiredFolderName == folderBaseName {
			nameMatches = TernaryValueYes
		}
	}

	remoteDefaultBranch := inspection.RemoteDefaultBranch
	localBranch := inspection.LocalBranch
	inSync := inspection.InSyncStatus
	remoteProtocol := inspection.RemoteProtocol
	originMatches := inspection.OriginMatchesCanonical

	if !inspection.IsGitRepository {
		finalRepo = string(TernaryValueNotApplicable)
		remoteDefaultBranch = string(TernaryValueNotApplicable)
		localBranch = string(TernaryValueNotApplicable)
		inSync = TernaryValueNotApplicable
		remoteProtocol = RemoteProtocolType(string(TernaryValueNotApplicable))
		originMatches = TernaryValueNotApplicable
	}
	return AuditReportRow{
		FolderName:             inspection.FolderName,
		FinalRepository:        finalRepo,
		NameMatches:            nameMatches,
		RemoteDefaultBranch:    remoteDefaultBranch,
		LocalBranch:            localBranch,
		InSync:                 inSync,
		RemoteProtocol:         remoteProtocol,
		OriginMatchesCanonical: originMatches,
	}
}

func relativeFolderName(path string, roots []string) string {
	cleanedPath := filepath.Clean(path)
	var bestRelative string
	for _, root := range roots {
		cleanedRoot := filepath.Clean(root)
		relativePath, relativeError := filepath.Rel(cleanedRoot, cleanedPath)
		if relativeError != nil {
			continue
		}
		if strings.HasPrefix(relativePath, "..") {
			continue
		}
		if relativePath == "." {
			baseName := filepath.Base(cleanedPath)
			if len(baseName) == 0 {
				continue
			}
			return baseName
		}
		relativePath = filepath.ToSlash(relativePath)
		if len(bestRelative) == 0 || len(relativePath) < len(bestRelative) {
			bestRelative = relativePath
		}
	}
	if len(bestRelative) > 0 {
		return bestRelative
	}
	return filepath.Base(cleanedPath)
}

func mergeCandidatePaths(existing []string, extras []string) []string {
	seen := make(map[string]struct{}, len(existing))
	for _, path := range existing {
		seen[path] = struct{}{}
	}
	for _, extra := range extras {
		cleaned := filepath.Clean(extra)
		if _, exists := seen[cleaned]; exists {
			continue
		}
		existing = append(existing, cleaned)
		seen[cleaned] = struct{}{}
	}
	sort.Strings(existing)
	return existing
}

func collectAllFolders(roots []string) ([]string, error) {
	seen := make(map[string]struct{})
	var folders []string

	for _, root := range roots {
		absoluteRoot, absoluteError := filepath.Abs(root)
		if absoluteError != nil {
			return nil, absoluteError
		}

		directoryEntries, readError := os.ReadDir(absoluteRoot)
		if readError != nil {
			return nil, readError
		}

		for _, directoryEntry := range directoryEntries {
			if directoryEntry.Type()&fs.ModeSymlink != 0 {
				continue
			}
			if !directoryEntry.IsDir() {
				continue
			}
			if directoryEntry.Name() == gitMetadataDirectoryNameConstant {
				continue
			}

			folderPath := filepath.Join(absoluteRoot, directoryEntry.Name())
			cleanedFolderPath := filepath.Clean(folderPath)
			if _, exists := seen[cleanedFolderPath]; exists {
				continue
			}
			seen[cleanedFolderPath] = struct{}{}
			folders = append(folders, cleanedFolderPath)
		}
	}

	sort.Strings(folders)
	return folders, nil
}

func buildNonRepositoryInspection(path string, folderName string) RepositoryInspection {
	placeholder := string(TernaryValueNotApplicable)

	return RepositoryInspection{
		Path:                   filepath.Clean(path),
		FolderName:             folderName,
		RemoteDefaultBranch:    placeholder,
		LocalBranch:            placeholder,
		InSyncStatus:           TernaryValueNotApplicable,
		RemoteProtocol:         RemoteProtocolOther,
		OriginMatchesCanonical: TernaryValueNotApplicable,
		IsGitRepository:        false,
	}
}

func isPathWithinRepository(path string, repositories map[string]struct{}) bool {
	cleaned := filepath.Clean(path)
	if _, exists := repositories[cleaned]; exists {
		return false
	}
	for repositoryPath := range repositories {
		repositoryPrefix := repositoryPath + string(os.PathSeparator)
		if strings.HasPrefix(cleaned, repositoryPrefix) {
			return true
		}
	}
	return false
}

func (service *Service) resolveDefaultBranchFromGit(executionContext context.Context, repositoryPath string) string {
	remoteHeadReference := execshell.CommandDetails{
		Arguments:        remoteHeadSymbolicRefArguments(),
		WorkingDirectory: repositoryPath,
	}

	if executionResult, executionError := service.gitExecutor.ExecuteGit(executionContext, remoteHeadReference); executionError == nil {
		remoteReference := strings.TrimSpace(executionResult.StandardOutput)
		remoteReference = strings.TrimPrefix(remoteReference, fmt.Sprintf("refs/remotes/%s/", shared.OriginRemoteNameConstant))
		remoteReference = strings.TrimPrefix(remoteReference, refsHeadsPrefixConstant)
		if len(remoteReference) > 0 {
			return remoteReference
		}
	}

	localHeadReference := execshell.CommandDetails{
		Arguments:        localHeadSymbolicRefArguments(),
		WorkingDirectory: repositoryPath,
	}

	if executionResult, executionError := service.gitExecutor.ExecuteGit(executionContext, localHeadReference); executionError == nil {
		localReference := strings.TrimSpace(executionResult.StandardOutput)
		localReference = strings.TrimPrefix(localReference, refsHeadsPrefixConstant)
		if len(localReference) > 0 {
			return localReference
		}
	}

	return ""
}

func (service *Service) computeInSync(executionContext context.Context, repositoryPath string, remoteDefaultBranch string, localBranch string, protocol RemoteProtocolType) TernaryValue {
	if len(remoteDefaultBranch) == 0 || len(localBranch) == 0 || !strings.EqualFold(remoteDefaultBranch, localBranch) {
		return TernaryValueNotApplicable
	}

	if protocol != RemoteProtocolGit && protocol != RemoteProtocolSSH {
		return TernaryValueNotApplicable
	}

	fetchDetails := execshell.CommandDetails{
		Arguments:        remoteFetchArguments(remoteDefaultBranch),
		WorkingDirectory: repositoryPath,
	}

	if _, fetchError := service.gitExecutor.ExecuteGit(executionContext, fetchDetails); fetchError != nil {
		return TernaryValueNotApplicable
	}

	upstreamRef := service.resolveUpstreamReference(executionContext, repositoryPath)

	headRevision, headError := service.resolveRevision(executionContext, repositoryPath, headRevisionArguments())
	if headError != nil {
		return TernaryValueNotApplicable
	}

	remoteRevision := service.resolveRemoteRevision(executionContext, repositoryPath, upstreamRef, remoteDefaultBranch)
	if len(remoteRevision) == 0 {
		return TernaryValueNotApplicable
	}

	if headRevision == remoteRevision {
		return TernaryValueYes
	}
	return TernaryValueNo
}

func (service *Service) resolveUpstreamReference(executionContext context.Context, repositoryPath string) string {
	upstreamDetails := execshell.CommandDetails{
		Arguments:        upstreamReferenceArguments(),
		WorkingDirectory: repositoryPath,
	}

	executionResult, executionError := service.gitExecutor.ExecuteGit(executionContext, upstreamDetails)
	if executionError != nil {
		return ""
	}
	return strings.TrimSpace(executionResult.StandardOutput)
}

func (service *Service) resolveRevision(executionContext context.Context, repositoryPath string, arguments []string) (string, error) {
	commandDetails := execshell.CommandDetails{
		Arguments:        arguments,
		WorkingDirectory: repositoryPath,
	}
	executionResult, executionError := service.gitExecutor.ExecuteGit(executionContext, commandDetails)
	if executionError != nil {
		return "", executionError
	}
	return strings.TrimSpace(executionResult.StandardOutput), nil
}

func (service *Service) resolveRemoteRevision(executionContext context.Context, repositoryPath string, upstreamRef string, branch string) string {
	if len(strings.TrimSpace(upstreamRef)) > 0 {
		revision, revisionError := service.resolveRevision(executionContext, repositoryPath, revisionArguments(upstreamRef))
		if revisionError == nil && len(revision) > 0 {
			return revision
		}
	}

	for _, reference := range fallbackRemoteRevisionReferences(branch) {
		revision, revisionError := service.resolveRevision(executionContext, repositoryPath, revisionArguments(reference))
		if revisionError == nil && len(revision) > 0 {
			return revision
		}
	}

	return ""
}
