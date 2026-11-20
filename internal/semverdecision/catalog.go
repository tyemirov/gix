package semverdecision

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// sourceBounds describes one complete committed source snapshot.
type sourceBounds struct {
	Path                string `json:"path"`
	Source              string `json:"source"`
	Present             bool   `json:"present"`
	LineCount           int    `json:"line_count"`
	RuneCount           int    `json:"rune_count"`
	MaxLineRunes        int    `json:"max_line_runes"`
	FormatOverheadRunes int    `json:"format_overhead_runes"`
}

func (generator Generator) changedSourceCatalog(ctx context.Context, options Options) (string, error) {
	names, err := generator.runGit(ctx, options.RepositoryPath, []string{"diff", "--name-only", "-z", "--no-renames", options.SinceReference + ".." + options.SourceReference})
	if err != nil {
		return "", fmt.Errorf("list changed source paths: %w", err)
	}
	paths := strings.Split(strings.TrimSuffix(names, "\x00"), "\x00")
	sort.Strings(paths)
	var catalog strings.Builder
	catalog.WriteString("Changed-file source catalog (complete; related unchanged-file context may also be requested):\n")
	catalog.WriteString("JSONL rows [path,old,new]; snapshot tuple [present,line_count,rune_count,max_line_runes,format_overhead_runes]\n")
	for index, path := range paths {
		if path == "" || (index > 0 && path == paths[index-1]) {
			continue
		}
		var snapshots [2]sourceBounds
		for sourceIndex, source := range []string{"old", "new"} {
			request := ContextRequest{Path: path, Source: source, StartLine: 1, LineCount: 1}
			content, present, err := generator.committedSource(ctx, options, request)
			if err != nil {
				return "", fmt.Errorf("changed source catalog %s %q: %w", source, path, err)
			}
			bounds := committedSourceBounds(request, content, present)
			snapshots[sourceIndex] = bounds
		}
		encodedPath, err := json.Marshal(path)
		if err != nil {
			return "", fmt.Errorf("encode changed source path: %w", err)
		}
		fmt.Fprintf(&catalog, "[%s,%s,%s]\n", encodedPath, formatSourceTuple(snapshots[0]), formatSourceTuple(snapshots[1]))
	}
	return catalog.String(), nil
}

func committedSourceBounds(request ContextRequest, content string, present bool) sourceBounds {
	lines := sourceLines(content)
	bounds := sourceBounds{Path: request.Path, Source: request.Source, Present: present, LineCount: len(lines), RuneCount: utf8.RuneCountInString(content)}
	for _, line := range lines {
		bounds.MaxLineRunes = max(bounds.MaxLineRunes, utf8.RuneCountInString(line))
	}
	// The largest integer width bounds this field's own formatted contribution
	// without a recursive size calculation. Actual source ranges use fewer digits.
	bounds.FormatOverheadRunes = int(^uint(0) >> 1)
	request.StartLine, request.LineCount = max(1, len(lines)), max(1, len(lines))
	formatted := ContextEvidence{ContextRequest: request, Absent: !present, Bounds: bounds}
	if !present {
		formatted.Content = absentSourceContent
	}
	bounds.FormatOverheadRunes = utf8.RuneCountInString(formatContext(formatted))
	return bounds
}

func formatSourceTuple(bounds sourceBounds) string {
	return fmt.Sprintf("[%t,%d,%d,%d,%d]", bounds.Present, bounds.LineCount, bounds.RuneCount, bounds.MaxLineRunes, bounds.FormatOverheadRunes)
}

// sourceLines is the exact line boundary contract for metadata and retrieval.
func sourceLines(content string) []string {
	lines := strings.SplitAfter(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
