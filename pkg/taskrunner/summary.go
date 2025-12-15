package taskrunner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tyemirov/gix/internal/repos/shared"
)

// RenderSummaryLine returns the summary line printed after multi-repository runs.
func RenderSummaryLine(data shared.SummaryData, roots []string) string {
	repositoryCount := data.TotalRepositories
	if repositoryCount == 0 {
		repositoryCount = deduplicateRoots(roots)
	}
	if repositoryCount <= 1 {
		return ""
	}

	parts := []string{fmt.Sprintf("Summary: total.repos=%d", repositoryCount)}

	if len(data.EventCounts) > 0 {
		keys := make([]string, 0, len(data.EventCounts))
		for key := range data.EventCounts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s=%d", key, data.EventCounts[key]))
		}
	}

	warnCount := data.LevelCounts[shared.EventLevelWarn]
	errorCount := data.LevelCounts[shared.EventLevelError]

	parts = append(parts, fmt.Sprintf("%s=%d", shared.EventLevelWarn, warnCount))
	parts = append(parts, fmt.Sprintf("%s=%d", shared.EventLevelError, errorCount))

	durationHuman := strings.TrimSpace(data.DurationHuman)
	if durationHuman == "" {
		durationHuman = "0s"
	}

	parts = append(parts, fmt.Sprintf("duration_human=%s", durationHuman))
	parts = append(parts, fmt.Sprintf("duration_ms=%d", data.DurationMilliseconds))

	return strings.Join(parts, " ")
}
