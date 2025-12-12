package workflow

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/tyemirov/gix/internal/repos/shared"
)

var phaseLabels = map[LogPhase]string{
	LogPhaseRemoteFolder: "remote/folder",
	LogPhaseBranch:       "branch",
	LogPhaseFiles:        "files",
	LogPhaseGit:          "git",
	LogPhasePullRequest:  "pull request",
}

// NewHumanEventFormatter returns the workflow human-readable formatter.
func NewHumanEventFormatter() shared.EventFormatter {
	return &workflowHumanFormatter{
		headersPrinted:   make(map[string]struct{}),
		pendingTasks:     make(map[string]string),
		repositoryStates: make(map[string]*repositoryLogState),
	}
}

type workflowHumanFormatter struct {
	headersPrinted   map[string]struct{}
	pendingTasks     map[string]string
	repositoryStates map[string]*repositoryLogState
	printedAnyRepo   bool
}

type repositoryLogState struct {
	printedPhases          map[LogPhase]struct{}
	suppressNextBranchTask bool
	issuesPrinted          bool
}

func (formatter *workflowHumanFormatter) HandleEvent(event shared.Event, writer io.Writer) {
	if formatter == nil || writer == nil {
		return
	}

	identifier := strings.TrimSpace(event.RepositoryIdentifier)
	path := strings.TrimSpace(event.RepositoryPath)
	repositoryLabel := strings.TrimSpace(formatRepositoryHeaderLabel(identifier, path))
	headerRepository := repositoryLabel
	if strings.TrimSpace(identifier) == "" || headerRepository == "" {
		headerRepository = "workflow"
	}
	if strings.TrimSpace(identifier) != "" {
		formatter.ensureHeader(writer, headerRepository)
	}
	repositoryKey := headerRepository
	state := formatter.ensureRepositoryState(repositoryKey)

	switch event.Code {
	case shared.EventCodeTaskPlan:
		formatter.recordTaskPlan(repositoryKey, event)
		return
	case shared.EventCodeTaskApply:
		formatter.handleTaskApply(writer, repositoryKey, state, event)
		return
	case shared.EventCodeTaskSkip:
		delete(formatter.pendingTasks, repositoryKey)
		operation := strings.TrimSpace(event.Details["operation"])
		if len(operation) > 0 {
			rawMessage := strings.TrimSpace(event.Message)
			lowerMessage := strings.ToLower(rawMessage)

			var suffix string
			if strings.Contains(lowerMessage, "requires changes") {
				switch operation {
				case "Git Stage Commit":
					suffix = "no-op: no workflow-edited files to commit for this repository (require_changes safeguard; clean worktree)"
				case "Git Push":
					suffix = "no-op: no commit produced by this workflow (require_changes safeguard)"
				case "Open Pull Request", "Create Pull Request":
					suffix = "no-op: no branch changes to review for this workflow (require_changes safeguard)"
				default:
					if len(rawMessage) > 0 {
						suffix = fmt.Sprintf("no-op: %s", rawMessage)
					} else {
						suffix = "no-op (safeguard)"
					}
				}
			} else {
				if len(rawMessage) > 0 {
					suffix = fmt.Sprintf("skipped: %s", rawMessage)
				} else {
					suffix = "skipped"
				}
			}

			label := operation
			if stepName := strings.TrimSpace(event.Details["step"]); len(stepName) > 0 {
				label = stepName
			}

			formatter.writePhaseEntry(
				writer,
				repositoryKey,
				LogPhaseGit,
				formatter.decoratePhaseMessage(
					event.Level,
					fmt.Sprintf("%s (%s)", label, suffix),
				),
			)
			return
		}
		formatter.writeIssue(writer, state, event)
		return
	case shared.EventCodeRepoSwitched:
		formatter.handleBranchSwitch(writer, repositoryKey, state, event)
		return
	}

	if formatter.handlePhaseEventByCode(writer, repositoryKey, event) {
		return
	}

	switch event.Level {
	case shared.EventLevelWarn:
		formatter.writeIssue(writer, state, event)
	case shared.EventLevelError:
		formatter.writeIssue(writer, state, event)
	default:
		formatter.writeEventSummary(writer, event)
	}
}

func (formatter *workflowHumanFormatter) recordTaskPlan(repository string, event shared.Event) {
	taskName := strings.TrimSpace(event.Details["task"])
	if len(taskName) == 0 {
		taskName = strings.TrimSpace(event.Message)
	}
	if len(taskName) == 0 {
		return
	}
	formatter.pendingTasks[repository] = taskName
}

func (formatter *workflowHumanFormatter) handleTaskApply(writer io.Writer, repository string, state *repositoryLogState, event shared.Event) {
	taskName := formatter.resolveTaskName(repository, event)
	phase := formatter.determinePhase(event)
	if phase == LogPhaseUnknown {
		phase = LogPhaseFiles
	}

	if phase == LogPhaseBranch {
		if state.suppressNextBranchTask {
			state.suppressNextBranchTask = false
			return
		}
		formatter.writeBranchLine(writer, repository, taskName)
		return
	}

	stepName := strings.TrimSpace(event.Details["step"])
	message := taskName
	if len(stepName) > 0 && !strings.EqualFold(stepName, taskName) {
		message = fmt.Sprintf("%s: %s", stepName, taskName)
	}

	formatter.writePhaseEntry(writer, repository, phase, message)
}

func (formatter *workflowHumanFormatter) determinePhase(event shared.Event) LogPhase {
	rawPhase := strings.TrimSpace(event.Details["phase"])
	switch rawPhase {
	case string(LogPhaseRemoteFolder):
		return LogPhaseRemoteFolder
	case string(LogPhaseBranch):
		return LogPhaseBranch
	case string(LogPhaseFiles):
		return LogPhaseFiles
	case string(LogPhaseGit):
		return LogPhaseGit
	case string(LogPhasePullRequest):
		return LogPhasePullRequest
	default:
		return LogPhaseUnknown
	}
}

func (formatter *workflowHumanFormatter) resolveTaskName(repository string, event shared.Event) string {
	if taskName := strings.TrimSpace(event.Details["task"]); len(taskName) > 0 {
		return taskName
	}
	return formatter.consumeTaskName(repository, strings.TrimSpace(event.Message))
}

func (formatter *workflowHumanFormatter) handleBranchSwitch(writer io.Writer, repository string, state *repositoryLogState, event shared.Event) {
	branch := strings.TrimSpace(event.Details["branch"])
	if len(branch) == 0 {
		branch = strings.TrimSpace(event.Message)
	}
	if len(branch) == 0 {
		branch = "branch"
	}
	if strings.EqualFold(strings.TrimSpace(event.Details["created"]), "true") {
		branch += " (created)"
	}
	formatter.writeBranchLine(writer, repository, branch)
	state.suppressNextBranchTask = true
}

func (formatter *workflowHumanFormatter) handlePhaseEventByCode(writer io.Writer, repository string, event shared.Event) bool {
	phase, handled := phaseFromEventCode(event.Code)
	if !handled {
		return false
	}
	message := strings.TrimSpace(event.Message)
	if len(message) == 0 {
		message = strings.TrimSpace(event.Code)
	}
	if stepName := strings.TrimSpace(event.Details["step"]); len(stepName) > 0 {
		message = fmt.Sprintf("%s: %s", stepName, message)
	}
	formatter.writePhaseEntry(writer, repository, phase, formatter.decoratePhaseMessage(event.Level, message))
	return true
}

func (formatter *workflowHumanFormatter) decoratePhaseMessage(level shared.EventLevel, message string) string {
	switch level {
	case shared.EventLevelWarn:
		return fmt.Sprintf("⚠ %s", message)
	case shared.EventLevelError:
		return fmt.Sprintf("✖ %s", message)
	default:
		return message
	}
}

func phaseFromEventCode(code string) (LogPhase, bool) {
	switch code {
	case shared.EventCodeRemotePlan,
		shared.EventCodeRemoteSkip,
		shared.EventCodeRemoteUpdate,
		shared.EventCodeRemoteMissing,
		shared.EventCodeRemoteDeclined,
		shared.EventCodeProtocolPlan,
		shared.EventCodeProtocolSkip,
		shared.EventCodeProtocolUpdate,
		shared.EventCodeProtocolDeclined:
		return LogPhaseRemoteFolder, true
	case shared.EventCodeFolderPlan,
		shared.EventCodeFolderSkip,
		shared.EventCodeFolderRename,
		shared.EventCodeFolderError:
		return LogPhaseFiles, true
	case shared.EventCodeNamespacePlan,
		shared.EventCodeNamespaceApply,
		shared.EventCodeNamespaceSkip,
		shared.EventCodeNamespaceNoop,
		shared.EventCodeNamespaceError:
		return LogPhaseFiles, true
	default:
		return LogPhaseUnknown, false
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

func (formatter *workflowHumanFormatter) ensureRepositoryState(repository string) *repositoryLogState {
	state, exists := formatter.repositoryStates[repository]
	if exists {
		return state
	}
	state = &repositoryLogState{
		printedPhases: make(map[LogPhase]struct{}),
	}
	formatter.repositoryStates[repository] = state
	return state
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

func (formatter *workflowHumanFormatter) writePhaseEntry(writer io.Writer, repository string, phase LogPhase, message string) {
	if len(message) == 0 {
		return
	}
	state := formatter.ensureRepositoryState(repository)
	if _, exists := state.printedPhases[phase]; !exists {
		if label, ok := phaseLabels[phase]; ok {
			fmt.Fprintf(writer, "  • %s:\n", label)
		}
		state.printedPhases[phase] = struct{}{}
	}
	fmt.Fprintf(writer, "    - %s\n", message)
}

func (formatter *workflowHumanFormatter) writeBranchLine(writer io.Writer, repository string, message string) {
	formatter.writePhaseEntry(writer, repository, LogPhaseBranch, message)
}

func (formatter *workflowHumanFormatter) writeIssue(writer io.Writer, state *repositoryLogState, event shared.Event) {
	if state == nil {
		return
	}

	message := strings.TrimSpace(event.Message)
	if len(message) == 0 && event.Level == shared.EventLevelError {
		message = "error"
	}
	if len(message) == 0 {
		return
	}

	if !state.issuesPrinted {
		fmt.Fprintln(writer, "    issues:")
		state.issuesPrinted = true
	}

	prefix := ""
	switch event.Level {
	case shared.EventLevelWarn:
		prefix = "⚠ "
	case shared.EventLevelError:
		prefix = "✖ "
	}

	taskLabel := strings.TrimSpace(event.Details["task"])
	operationLabel := strings.TrimSpace(event.Details["operation"])

	label := ""
	if len(taskLabel) > 0 {
		label = taskLabel
	} else if len(operationLabel) > 0 {
		label = operationLabel
	}

	trimmedMessage := message
	if label != "" {
		lowerLabel := strings.ToLower(label)
		lowerMessage := strings.ToLower(trimmedMessage)
		if strings.HasPrefix(lowerMessage, lowerLabel) {
			stripped := strings.TrimSpace(trimmedMessage[len(label):])
			if strings.HasPrefix(stripped, ":") {
				stripped = strings.TrimSpace(stripped[1:])
			}
			if len(stripped) > 0 {
				trimmedMessage = stripped
			}
		}
	}

	var line string
	switch {
	case label != "" && len(trimmedMessage) > 0:
		line = fmt.Sprintf("%s%s: %s", prefix, label, trimmedMessage)
	case label != "":
		line = fmt.Sprintf("%s%s", prefix, label)
	default:
		line = fmt.Sprintf("%s%s", prefix, trimmedMessage)
	}

	fmt.Fprintf(writer, "      - %s\n", strings.TrimSpace(line))
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

func (formatter *workflowHumanFormatter) buildDetailSegments(event shared.Event) []string {
	segments := make([]string, 0)
	if path := strings.TrimSpace(event.RepositoryPath); len(path) > 0 {
		segments = append(segments, fmt.Sprintf("path=%s", path))
	}
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
