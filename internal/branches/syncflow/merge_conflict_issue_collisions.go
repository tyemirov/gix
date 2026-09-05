package syncflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	mergeConflictIssueTrackerName           = "ISSUES.md"
	mergeConflictIssueArchiveName           = "ARCHIVE.md"
	mergeConflictIssueMaximumNumber         = 999
	mergeConflictIssueMergeMaximumConflicts = 127
)

var (
	mergeConflictIssueMetadata  = regexp.MustCompile(`^(?:\(P[0-2]\)[ \t]+)?(?:\{[^}\r\n]*\}[ \t]+)?`)
	mergeConflictIssueReference = regexp.MustCompile(`\b[BIMFP][0-9]{3}R?\b`)
)

type mergeConflictIssueIdentity struct {
	ID    string
	Title string
}

func mergeConflictIssueIdentities(content string) []mergeConflictIssueIdentity {
	var identities []mergeConflictIssueIdentity
	for _, line := range mergeConflictLines(content) {
		header := mergeConflictIssueHeader.FindStringSubmatch(line)
		if header == nil {
			continue
		}
		title := strings.TrimSpace(mergeConflictIssueMetadata.ReplaceAllString(strings.TrimSpace(line[len(header[0]):]), ""))
		identities = append(identities, mergeConflictIssueIdentity{ID: header[1], Title: title})
	}
	return identities
}

// Concurrent new records with different titles have separate identities.
// Existing BASE identities stay with the ordinary three-way merge.
func (service mergeConflictResolutionService) resolveIssueIdentifierCollisions(ctx context.Context, file mergeConflictFile) (result mergeConflictFile, clean bool, resultErr error) {
	if filepath.Base(file.Path) != mergeConflictIssueTrackerName || !file.OursPresent || !file.TheirsPresent {
		return file, false, nil
	}
	baseIDs := map[string]bool{}
	for _, identity := range mergeConflictIssueIdentities(file.Base) {
		baseIDs[identity.ID] = true
	}
	localTitles := map[string]string{}
	for _, identity := range mergeConflictIssueIdentities(file.Ours) {
		localTitles[identity.ID] = identity.Title
	}
	var collisions []string
	for _, incoming := range mergeConflictIssueIdentities(file.Theirs) {
		localTitle, exists := localTitles[incoming.ID]
		if exists && !baseIDs[incoming.ID] && localTitle != incoming.Title {
			collisions = append(collisions, incoming.ID)
		}
	}
	if len(collisions) == 0 {
		return file, false, nil
	}

	archivePath, pathErr := mergeConflictResolutionFilesystemPath(service.repositoryPath, filepath.Join(filepath.Dir(file.Path), mergeConflictIssueArchiveName))
	if pathErr != nil {
		return mergeConflictFile{}, false, pathErr
	}
	archive, archiveErr := os.ReadFile(archivePath)
	if archiveErr != nil && !os.IsNotExist(archiveErr) {
		return mergeConflictFile{}, false, fmt.Errorf("read issue identifiers from %s: %w", archivePath, archiveErr)
	}
	maximumNumbers := map[byte]int{}
	for _, content := range []string{file.Base, file.Ours, file.Theirs, string(archive)} {
		for _, identity := range mergeConflictIssueIdentities(content) {
			number := int(identity.ID[1]-'0')*100 + int(identity.ID[2]-'0')*10 + int(identity.ID[3]-'0')
			if number > maximumNumbers[identity.ID[0]] {
				maximumNumbers[identity.ID[0]] = number
			}
		}
	}
	replacements := map[string]string{}
	for _, id := range collisions {
		next := maximumNumbers[id[0]] + 1
		if next > mergeConflictIssueMaximumNumber {
			return mergeConflictFile{}, false, fmt.Errorf("allocate an issue identifier for %s in %s: section %c exhausted its three-digit identifiers", id, file.Path, id[0])
		}
		maximumNumbers[id[0]] = next
		replacements[id] = fmt.Sprintf("%c%03d%s", id[0], next, id[4:])
		service.report(shared.EventLevelInfo, shared.EventCodeAIMergeResolution,
			fmt.Sprintf("renumbered independent incoming issue %s to %s in %s", id, replacements[id], file.Path),
			map[string]string{"path": file.Path, "old_issue_id": id, "new_issue_id": replacements[id]})
	}
	file.Theirs = mergeConflictIssueReference.ReplaceAllStringFunc(file.Theirs, func(id string) string {
		if replacement, exists := replacements[id]; exists {
			return replacement
		}
		return id
	})

	directory, directoryErr := os.MkdirTemp("", "gix-issue-collision-")
	if directoryErr != nil {
		return mergeConflictFile{}, false, fmt.Errorf("prepare issue merge for %s: %w", file.Path, directoryErr)
	}
	defer func() {
		if cleanupErr := os.RemoveAll(directory); cleanupErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove issue merge inputs for %s: %w", file.Path, cleanupErr))
		}
	}()
	base := file.Base
	if !file.BasePresent {
		base = ""
	}
	arguments := []string{"merge-file", "--diff3", "--stdout"}
	for index, content := range []string{file.Ours, base, file.Theirs} {
		path := filepath.Join(directory, strconv.Itoa(index))
		if writeErr := os.WriteFile(path, []byte(content), 0o600); writeErr != nil {
			return mergeConflictFile{}, false, fmt.Errorf("write issue merge input for %s: %w", file.Path, writeErr)
		}
		arguments = append(arguments, path)
	}
	merged, mergeErr := service.executor.ExecuteGit(ctx, execshell.CommandDetails{Arguments: arguments, WorkingDirectory: service.repositoryPath})
	if mergeErr != nil {
		var commandErr execshell.CommandFailedError
		if !errors.As(mergeErr, &commandErr) || commandErr.Result.ExitCode < 1 || commandErr.Result.ExitCode > mergeConflictIssueMergeMaximumConflicts {
			return mergeConflictFile{}, false, fmt.Errorf("merge renumbered issue records for %s: %w", file.Path, mergeErr)
		}
		merged = commandErr.Result
	}
	file.WorktreeContent = merged.StandardOutput
	return file, merged.ExitCode == 0, nil
}
