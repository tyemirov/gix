package workflow

import (
	"fmt"
	"io"
	"sort"
	"strings"

	workflowruntime "github.com/temirov/gix/internal/workflow"

	"github.com/temirov/gix/internal/repos/shared"
)

var phaseLabels = map[workflowruntime.LogPhase]string{
	workflowruntime.LogPhaseRemoteFolder: "remote/folder",
	workflowruntime.LogPhaseFiles:        "files",
	workflowruntime.LogPhaseGit:          "git",
	workflowruntime.LogPhasePullRequest:  "pull request",
}

func newWorkflowHumanFormatter() shared.EventFormatter {
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
	printedPhases          map[workflowruntime.LogPhase]struct{}
	suppressNextBranchTask bool
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
		formatter.writeWarning(writer, strings.TrimSpace(event.Message))
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
	if phase == workflowruntime.LogPhaseUnknown {
		phase = workflowruntime.LogPhaseFiles
	}

	if phase == workflowruntime.LogPhaseBranch {
		if state.suppressNextBranchTask {
			state.suppressNextBranchTask = false
			return
		}
		formatter.writeBranchLine(writer, repository, taskName)
		return
	}

	formatter.writePhaseEntry(writer, repository, phase, taskName)
}

func (formatter *workflowHumanFormatter) determinePhase(event shared.Event) workflowruntime.LogPhase {
	rawPhase := strings.TrimSpace(event.Details["phase"])
	switch rawPhase {
	case string(workflowruntime.LogPhaseRemoteFolder):
		return workflowruntime.LogPhaseRemoteFolder
	case string(workflowruntime.LogPhaseBranch):
		return workflowruntime.LogPhaseBranch
	case string(workflowruntime.LogPhaseFiles):
		return workflowruntime.LogPhaseFiles
	case string(workflowruntime.LogPhaseGit):
		return workflowruntime.LogPhaseGit
	case string(workflowruntime.LogPhasePullRequest):
		return workflowruntime.LogPhasePullRequest
	default:
		return workflowruntime.LogPhaseUnknown
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
	formatter.writePhaseEntry(writer, repository, phase, message)
	return true
}

func phaseFromEventCode(code string) (workflowruntime.LogPhase, bool) {
	switch code {
	case shared.EventCodeRemotePlan,
		shared.EventCodeRemoteSkip,
		shared.EventCodeRemoteUpdate,
		shared.EventCodeRemoteMissing,
		shared.EventCodeRemoteDeclined,
		shared.EventCodeProtocolPlan,
		shared.EventCodeProtocolSkip,
		shared.EventCodeProtocolUpdate,
		shared.EventCodeProtocolDeclined,
		shared.EventCodeFolderPlan,
		shared.EventCodeFolderSkip,
		shared.EventCodeFolderRename,
		shared.EventCodeFolderError:
		return workflowruntime.LogPhaseRemoteFolder, true
	case shared.EventCodeNamespacePlan,
		shared.EventCodeNamespaceApply,
		shared.EventCodeNamespaceSkip,
		shared.EventCodeNamespaceNoop,
		shared.EventCodeNamespaceError:
		return workflowruntime.LogPhaseFiles, true
	default:
		return workflowruntime.LogPhaseUnknown, false
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
		printedPhases: make(map[workflowruntime.LogPhase]struct{}),
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

func (formatter *workflowHumanFormatter) writePhaseEntry(writer io.Writer, repository string, phase workflowruntime.LogPhase, message string) {
	if len(message) == 0 {
		return
	}
	state := formatter.ensureRepositoryState(repository)
	if _, exists := state.printedPhases[phase]; !exists {
		if label, ok := phaseLabels[phase]; ok {
			fmt.Fprintf(writer, "  %s:\n", label)
		}
		state.printedPhases[phase] = struct{}{}
	}
	fmt.Fprintf(writer, "    - %s\n", message)
}

func (formatter *workflowHumanFormatter) writeBranchLine(writer io.Writer, repository string, message string) {
	if len(message) == 0 {
		return
	}
	fmt.Fprintf(writer, "  branch: %s\n", message)
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
