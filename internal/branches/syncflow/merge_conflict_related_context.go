package syncflow

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tyemirov/gix/internal/execshell"
)

const (
	mergeRelatedFileLimit   = 8
	mergeRelatedByteLimit   = 256 * 1024
	mergeRelatedIndexFormat = "--format=%(objectname) %(stage)%x00%(path)"
	mergeRelatedLiteralPath = ":(top,literal)"
	mergeRelatedIndexSource = "index"
	mergeRelatedBaseSource  = "base"
	gitCatFileSubcommand    = "cat-file"
	gitCatFileSizeFlag      = "-s"
	gitBlobObjectType       = "blob"
)

type mergeRelatedVersion struct {
	Source  string `json:"source"`
	Content string `json:"content"`
}

type mergeRelatedFile struct {
	Path     string                `json:"path"`
	Resolved bool                  `json:"resolved"`
	Deleted  bool                  `json:"deleted"`
	Versions []mergeRelatedVersion `json:"versions"`
}

// Read immutable Git blobs, never filesystem targets or untracked inputs.
func (service mergeConflictResolutionService) relatedMergeContext(ctx context.Context, current string, supplied []mergeRelatedFile, requested []string) ([]mergeRelatedFile, error) {
	known := make(map[string]bool, len(supplied))
	remaining := mergeRelatedByteLimit
	for _, file := range supplied {
		known[file.Path] = true
		for _, version := range file.Versions {
			remaining -= len(version.Content)
		}
	}
	seen := make(map[string]bool, len(requested))
	var additions []string
	for _, requestedPath := range requested {
		if requestedPath == "" || path.IsAbs(requestedPath) || path.Clean(requestedPath) != requestedPath ||
			requestedPath == "." || requestedPath == ".." || strings.HasPrefix(requestedPath, "../") ||
			strings.ContainsAny(requestedPath, "\\\x00\r\n") || requestedPath == current {
			return nil, fmt.Errorf("invalid related repository path %q", requestedPath)
		}
		if seen[requestedPath] {
			return nil, fmt.Errorf("duplicate related repository path %q", requestedPath)
		}
		seen[requestedPath] = true
		if !known[requestedPath] {
			additions = append(additions, requestedPath)
		}
	}
	if len(additions) == 0 {
		return nil, errors.New("requested related files were already supplied")
	}
	if len(supplied)+len(additions) > mergeRelatedFileLimit {
		return nil, fmt.Errorf("related context exceeds %d files", mergeRelatedFileLimit)
	}
	expanded := append([]mergeRelatedFile(nil), supplied...)
	for _, requestedPath := range additions {
		file, err := service.readRelatedMergeFile(ctx, requestedPath, &remaining)
		if err != nil {
			return nil, fmt.Errorf("read related repository path %q: %w", requestedPath, err)
		}
		expanded = append(expanded, file)
	}
	return expanded, nil
}

func (service mergeConflictResolutionService) readRelatedMergeFile(ctx context.Context, requestedPath string, remaining *int) (mergeRelatedFile, error) {
	file := mergeRelatedFile{Path: requestedPath}
	if original, resolved := service.resolvedSources[requestedPath]; resolved {
		file.Resolved = true
		for _, version := range []struct {
			source, content string
			present         bool
		}{
			{mergeRelatedBaseSource, original.Base, original.BasePresent},
			{mergeSideOurs, original.Ours, original.OursPresent},
			{mergeSideTheirs, original.Theirs, original.TheirsPresent},
		} {
			if !version.present {
				continue
			}
			if len(version.content) > *remaining {
				return file, fmt.Errorf("related context exceeds %d content bytes", mergeRelatedByteLimit)
			}
			*remaining -= len(version.content)
			file.Versions = append(file.Versions, mergeRelatedVersion{Source: version.source, Content: version.content})
		}
	}
	entries, err := service.executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitLsFilesSubcommandConstant, mergeRelatedIndexFormat, gitNullOutputFlagConstant, gitPathspecSeparatorConstant, mergeRelatedLiteralPath + requestedPath},
		WorkingDirectory: service.repositoryPath,
	})
	if err != nil {
		return file, fmt.Errorf("inspect related Git index entries: %w", err)
	}
	if entries.StandardOutput == "" {
		if file.Resolved {
			file.Deleted = true
			return file, nil
		}
		return file, errors.New("path is not a tracked file in the merge index")
	}
	if !strings.HasSuffix(entries.StandardOutput, "\x00") {
		return file, errors.New("related Git index output is not NUL terminated")
	}
	records := strings.Split(strings.TrimSuffix(entries.StandardOutput, "\x00"), "\x00")
	if len(records)%2 != 0 {
		return file, errors.New("invalid related Git index records")
	}
	for index := 0; index < len(records); index += 2 {
		if records[index+1] != requestedPath {
			return file, errors.New("path must select one exact tracked file")
		}
		fields := strings.Fields(records[index])
		if len(fields) != 2 {
			return file, errors.New("invalid related Git object entry")
		}
		source := ""
		switch fields[1] {
		case "0":
			source = mergeRelatedIndexSource
		case "1":
			source = mergeRelatedBaseSource
		case "2":
			source = mergeSideOurs
		case "3":
			source = mergeSideTheirs
		default:
			return file, fmt.Errorf("invalid related Git stage %q", fields[1])
		}
		version, err := service.readRelatedMergeBlob(ctx, fields[0], source, remaining)
		if err != nil {
			return file, err
		}
		file.Versions = append(file.Versions, version)
	}
	return file, nil
}

func (service mergeConflictResolutionService) readRelatedMergeBlob(ctx context.Context, object, source string, remaining *int) (mergeRelatedVersion, error) {
	size, err := service.executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitCatFileSubcommand, gitCatFileSizeFlag, object},
		WorkingDirectory: service.repositoryPath,
	})
	if err != nil {
		return mergeRelatedVersion{}, fmt.Errorf("read related Git object size: %w", err)
	}
	count, err := strconv.ParseUint(strings.TrimSpace(size.StandardOutput), 10, 64)
	if err != nil {
		return mergeRelatedVersion{}, fmt.Errorf("parse related Git object size: %w", err)
	}
	if count > uint64(*remaining) {
		return mergeRelatedVersion{}, fmt.Errorf("related context exceeds %d content bytes", mergeRelatedByteLimit)
	}
	result, err := service.executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        []string{gitCatFileSubcommand, gitBlobObjectType, object},
		WorkingDirectory: service.repositoryPath,
	})
	if err != nil {
		return mergeRelatedVersion{}, fmt.Errorf("read related Git blob: %w", err)
	}
	if !utf8.ValidString(result.StandardOutput) || strings.ContainsRune(result.StandardOutput, 0) {
		return mergeRelatedVersion{}, errors.New("related Git blob is not UTF-8 text")
	}
	*remaining -= len(result.StandardOutput)
	return mergeRelatedVersion{Source: source, Content: result.StandardOutput}, nil
}
