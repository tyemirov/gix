package taskrunner

import "github.com/tyemirov/gix/internal/repos/shared"

// RenderSummaryLine returns the summary line printed after multi-repository runs.
func RenderSummaryLine(data shared.SummaryData, roots []string) string {
	repositoryCount := data.TotalRepositories
	if repositoryCount == 0 {
		repositoryCount = deduplicateRoots(roots)
	}
	if repositoryCount <= 1 {
		return ""
	}

	return shared.FormatSummaryLine(data, repositoryCount)
}
