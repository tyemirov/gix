package workflow

import (
	"fmt"
	"io"
	"strings"

	"github.com/tyemirov/gix/internal/repos/shared"
)

// NewHumanEventFormatter returns the workflow human-readable formatter.
func NewHumanEventFormatter() shared.EventFormatter {
	return &workflowHumanFormatter{
		headersPrinted: make(map[string]struct{}),
	}
}

type workflowHumanFormatter struct {
	headersPrinted map[string]struct{}
	printedAnyRepo bool
}

func (formatter *workflowHumanFormatter) HandleEvent(event shared.Event, writer io.Writer) {
	if formatter == nil || writer == nil {
		return
	}

	if event.Code != shared.EventCodeWorkflowStepSummary {
		return
	}

	identifier := strings.TrimSpace(event.RepositoryIdentifier)
	path := strings.TrimSpace(event.RepositoryPath)
	repositoryLabel := strings.TrimSpace(formatRepositoryHeaderLabel(identifier, path))
	if repositoryLabel == "" {
		repositoryLabel = "workflow"
	}

	formatter.ensureHeader(writer, repositoryLabel)

	stepName := strings.TrimSpace(event.Details["step"])
	if stepName == "" {
		stepName = strings.TrimSpace(event.Message)
	}
	if stepName == "" {
		stepName = "step"
	}

	outcome := strings.TrimSpace(event.Details["outcome"])
	if outcome == "" {
		outcome = "ok"
	}

	reason := strings.TrimSpace(event.Details["reason"])

	fmt.Fprintf(writer, "  step name: %s\n", stepName)
	fmt.Fprintf(writer, "  outcome: %s\n", outcome)
	fmt.Fprintf(writer, "  reason: %s\n", reason)
}

func (formatter *workflowHumanFormatter) ensureHeader(writer io.Writer, repository string) {
	if _, exists := formatter.headersPrinted[repository]; exists {
		return
	}
	if formatter.printedAnyRepo {
		fmt.Fprintln(writer)
	}
	fmt.Fprintf(writer, "-- %s --\n", repository)
	formatter.headersPrinted[repository] = struct{}{}
	formatter.printedAnyRepo = true
}

func formatRepositoryHeaderLabel(identifier string, path string) string {
	trimmedIdentifier := strings.TrimSpace(identifier)
	trimmedPath := strings.TrimSpace(path)
	switch {
	case trimmedIdentifier != "" && trimmedPath != "":
		return fmt.Sprintf("%s (%s)", trimmedIdentifier, trimmedPath)
	case trimmedIdentifier != "":
		return trimmedIdentifier
	case trimmedPath != "":
		return trimmedPath
	default:
		return ""
	}
}
