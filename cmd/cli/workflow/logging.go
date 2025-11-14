package workflow

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/temirov/gix/internal/repos/shared"
)

func newWorkflowHumanFormatter() shared.EventFormatter {
	return &workflowHumanFormatter{
		headersPrinted: make(map[string]struct{}),
		pendingTasks:   make(map[string]string),
	}
}

type workflowHumanFormatter struct {
	headersPrinted map[string]struct{}
	pendingTasks   map[string]string
	printedAnyRepo bool
}

func (formatter *workflowHumanFormatter) HandleEvent(event shared.Event, writer io.Writer) {
	if formatter == nil || writer == nil {
		return
	}

	repository := strings.TrimSpace(event.RepositoryIdentifier)
	if len(repository) == 0 {
		repository = strings.TrimSpace(event.RepositoryPath)
	}
	if len(repository) == 0 {
		repository = "workflow"
	}

	formatter.ensureHeader(writer, repository)

	switch event.Code {
	case shared.EventCodeTaskPlan:
		taskName := strings.TrimSpace(event.Details["task"])
		if len(taskName) == 0 {
			taskName = strings.TrimSpace(event.Message)
		}
		if len(taskName) > 0 {
			formatter.pendingTasks[repository] = taskName
		}
	case shared.EventCodeTaskApply:
		taskName := formatter.consumeTaskName(repository, strings.TrimSpace(event.Message))
		if len(taskName) > 0 {
			fmt.Fprintf(writer, "  ✓ %s\n", taskName)
		}
	case shared.EventCodeRepoSwitched:
		branch := strings.TrimSpace(event.Details["branch"])
		if len(branch) == 0 {
			branch = strings.TrimSpace(event.Message)
		}
		if len(branch) == 0 {
			branch = "branch"
		}
		suffix := ""
		if strings.EqualFold(strings.TrimSpace(event.Details["created"]), "true") {
			suffix = " (created)"
		}
		fmt.Fprintf(writer, "  ↪ switched to %s%s\n", branch, suffix)
	case shared.EventCodeTaskSkip:
		delete(formatter.pendingTasks, repository)
		formatter.writeWarning(writer, strings.TrimSpace(event.Message))
	default:
		switch event.Level {
		case shared.EventLevelWarn:
			formatter.writeWarning(writer, strings.TrimSpace(event.Message))
		case shared.EventLevelError:
			message := strings.TrimSpace(event.Message)
			if len(message) == 0 {
				message = "error"
			}
			fmt.Fprintf(writer, "  ✖ %s\n", message)
		default:
			formatter.writeEventSummary(writer, event)
		}
	}
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

func (formatter *workflowHumanFormatter) consumeTaskName(repository string, fallback string) string {
	if formatter == nil {
		return fallback
	}
	if taskName, exists := formatter.pendingTasks[repository]; exists && len(strings.TrimSpace(taskName)) > 0 {
		delete(formatter.pendingTasks, repository)
		return taskName
	}
	return fallback
}

func (formatter *workflowHumanFormatter) writeWarning(writer io.Writer, message string) {
	if len(message) == 0 {
		return
	}
	fmt.Fprintf(writer, "  ⚠ %s\n", message)
}

func (formatter *workflowHumanFormatter) writeEventSummary(writer io.Writer, event shared.Event) {
	if event.Code == "" {
		return
	}
	detailSegments := formatter.buildDetailSegments(event)
	if len(detailSegments) > 0 {
		fmt.Fprintf(writer, "event=%s %s\n", event.Code, strings.Join(detailSegments, " "))
		return
	}
	fmt.Fprintf(writer, "event=%s\n", event.Code)
}

func (formatter *workflowHumanFormatter) buildDetailSegments(event shared.Event) []string {
	segments := make([]string, 0)
	message := strings.TrimSpace(event.Message)
	if len(message) > 0 {
		segments = append(segments, message)
	}
	if len(event.Details) == 0 {
		return segments
	}
	keys := make([]string, 0, len(event.Details))
	for key := range event.Details {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := strings.TrimSpace(event.Details[key])
		if len(value) == 0 {
			continue
		}
		segments = append(segments, fmt.Sprintf("%s=%s", key, value))
	}
	return segments
}
