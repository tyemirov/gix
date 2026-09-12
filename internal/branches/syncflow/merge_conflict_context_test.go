package syncflow

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestMergePlacementPreservesUTF8Boundaries(t *testing.T) {
	boundaryLimit := mergePlanContextLimit / 2
	for _, fixture := range []struct {
		name          string
		before, after string
		expected      mergeRegionPlacement
	}{
		{name: "empty", expected: mergeRegionPlacement{}},
		{name: "exact_budget", before: strings.Repeat("x", mergePlanContextLimit), expected: mergeRegionPlacement{Before: strings.Repeat("x", mergePlanContextLimit)}},
		{
			name:   "cut_inside_unicode",
			before: strings.Repeat("x", boundaryLimit) + "界" + strings.Repeat("a", boundaryLimit-1),
			after:  strings.Repeat("b", boundaryLimit-1) + "界" + strings.Repeat("y", boundaryLimit),
			expected: mergeRegionPlacement{Before: strings.Repeat("a", boundaryLimit-1), After: strings.Repeat("b", boundaryLimit-1),
				BeforeTruncated: true, AfterTruncated: true},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			actual := initialMergeRegionPlacement(mergeRegionPlacement{Before: fixture.before, After: fixture.after})
			require.Equal(t, fixture.expected, actual)
			require.True(t, utf8.ValidString(actual.Before))
			require.True(t, utf8.ValidString(actual.After))
		})
	}
}
