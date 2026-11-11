package syncflow

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStrictSyncTrackedPathsPrefixConflicts(t *testing.T) {
	for _, scenario := range []struct {
		name, path, conflict string
		tracked              []string
		exact                bool
	}{
		{name: "empty", path: "docs/ci.md"},
		{name: "exact", path: "docs/ci.md", tracked: []string{"docs/ci.md"}, exact: true},
		{name: "ancestor", path: "docs/ci.md", tracked: []string{"docs"}, conflict: "docs"},
		{name: "deep ancestor", path: "docs/nested/ci.md", tracked: []string{"docs/nested"}, conflict: "docs/nested"},
		{name: "descendant", path: "docs", tracked: []string{"z.txt", "docs/nested/ci.md", "docs-1"}, conflict: "docs/nested/ci.md"},
		{name: "literal ancestor", path: "docs [draft]*/ci.md", tracked: []string{"docs [draft]*"}, conflict: "docs [draft]*"},
		{name: "literal descendant", path: "docs [draft]*", tracked: []string{"docs [draft]*/ci\nnotes.md"}, conflict: "docs [draft]*/ci\nnotes.md"},
		{name: "ancestor boundary", path: "docs-old/ci.md", tracked: []string{"docs"}},
		{name: "descendant boundary", path: "docs", tracked: []string{"docs-old/ci.md"}},
		{name: "sibling", path: "docs/ci.md", tracked: []string{"docs/code.md", "docs/cd.md"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			paths := newStrictSyncTrackedPaths(scenario.tracked)
			require.Equal(t, scenario.conflict, paths.prefixConflict(scenario.path))
			_, exact := paths.exact[scenario.path]
			require.Equal(t, scenario.exact, exact)
		})
	}
}

func BenchmarkStrictSyncStashProjectionNoCollisions(b *testing.B) {
	var tracked, untracked strings.Builder
	for index := range 5000 {
		fmt.Fprintf(&tracked, "tracked/file-%05d\x00", index)
	}
	for index := range 40000 {
		fmt.Fprintf(&untracked, "untracked/file-%05d\x00", index)
	}
	executor := &mockGitExecutor{outputs: map[string]string{
		"show": "base index untracked", "ls-tree": untracked.String(), "ls-files": tracked.String(),
	}}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		projection, err := projectStrictSyncStash(context.Background(), executor, strictSyncStash{CommitID: "stash"})
		if err != nil || len(projection.remaining) != 40000 {
			b.Fatalf("project stash: remaining=%d, error=%v", len(projection.remaining), err)
		}
	}
}
