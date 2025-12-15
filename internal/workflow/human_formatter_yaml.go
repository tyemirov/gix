package workflow

import (
	"fmt"
	"io"
	"strings"

	"github.com/tyemirov/gix/internal/repos/shared"
)

// NewWorkflowYAMLFormatter returns a workflow event formatter that emits step summaries as a YAML array.
func NewWorkflowYAMLFormatter() shared.EventFormatter {
	return &workflowYAMLFormatter{
		headersPrinted: make(map[string]struct{}),
	}
}

type workflowYAMLFormatter struct {
	headersPrinted map[string]struct{}
	printedAnyRepo bool
}

func (formatter *workflowYAMLFormatter) HandleEvent(event shared.Event, writer io.Writer) {
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

	fmt.Fprintf(writer, "- stepName: %s\n", stepName)
	fmt.Fprintf(writer, "  outcome: %s\n", outcome)
	fmt.Fprintf(writer, "  reason: %s\n", formatYAMLScalar(reason))
}

func (formatter *workflowYAMLFormatter) ensureHeader(writer io.Writer, repository string) {
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

func formatYAMLScalar(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "''"
	}
	collapsed := strings.Join(strings.Fields(trimmed), " ")
	escaped := strings.ReplaceAll(collapsed, "'", "''")
	return "'" + escaped + "'"
}
