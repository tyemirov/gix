package history

import (
	"errors"
	"fmt"
	"sort"

	"github.com/temirov/gix/internal/repos/shared"
)

var (
	// ErrPathsMissing indicates that no valid purge paths were provided.
	ErrPathsMissing = errors.New("history paths missing")
)

// Options configures the history purge workflow.
type Options struct {
	repositoryPath shared.RepositoryPath
	paths          []shared.RepositoryPathSegment
	remoteName     *shared.RemoteName
	push           bool
	restore        bool
	pushMissing    bool
}

// NewOptions validates and constructs an Options instance.
func NewOptions(
	repositoryPath shared.RepositoryPath,
	paths []shared.RepositoryPathSegment,
	remoteName *shared.RemoteName,
	push bool,
	restore bool,
	pushMissing bool,
) (Options, error) {
	if len(paths) == 0 {
		return Options{}, ErrPathsMissing
	}

	sanitized := make([]shared.RepositoryPathSegment, len(paths))
	copy(sanitized, paths)
	sort.SliceStable(sanitized, func(i, j int) bool {
		return sanitized[i].String() < sanitized[j].String()
	})

	return Options{
		repositoryPath: repositoryPath,
		paths:          sanitized,
		remoteName:     remoteName,
		push:           push,
		restore:        restore,
		pushMissing:    pushMissing,
	}, nil
}

// NewPaths normalizes raw path inputs into typed segments.
func NewPaths(entries []string) ([]shared.RepositoryPathSegment, error) {
	segments := make([]shared.RepositoryPathSegment, 0, len(entries))
	seen := make(map[string]struct{})

	for _, entry := range entries {
		segment, err := shared.NewRepositoryPathSegment(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid history path %q: %w", entry, err)
		}
		key := segment.String()
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		segments = append(segments, segment)
	}

	if len(segments) == 0 {
		return nil, ErrPathsMissing
	}

	sort.SliceStable(segments, func(i, j int) bool {
		return segments[i].String() < segments[j].String()
	})

	return segments, nil
}

func (options Options) repositoryPathString() string {
	return options.repositoryPath.String()
}

func (options Options) remoteNameString() string {
	if options.remoteName == nil {
		return ""
	}
	return options.remoteName.String()
}

func (options Options) pathStrings() []string {
	results := make([]string, len(options.paths))
	for index := range options.paths {
		results[index] = options.paths[index].String()
	}
	return results
}
