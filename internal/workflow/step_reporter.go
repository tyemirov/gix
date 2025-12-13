package workflow

import (
	"path/filepath"
	"strings"

	"github.com/tyemirov/gix/internal/repos/shared"
)

type stepDecoratingReporter struct {
	base        shared.Reporter
	environment *Environment
}

func (reporter *stepDecoratingReporter) Report(event shared.Event) {
	if reporter == nil || reporter.base == nil {
		return
	}

	stepName := ""
	if reporter.environment != nil {
		stepName = strings.TrimSpace(reporter.environment.currentStepName)
	}

	if stepName == "" {
		if reporter.environment != nil {
			reporter.environment.observeStepEvent(event)
		}
		reporter.base.Report(event)
		return
	}

	if event.Details == nil {
		event.Details = map[string]string{"step": stepName}
		if reporter.environment != nil {
			reporter.environment.observeStepEvent(event)
		}
		reporter.base.Report(event)
		return
	}

	if _, exists := event.Details["step"]; exists {
		if reporter.environment != nil {
			reporter.environment.observeStepEvent(event)
		}
		reporter.base.Report(event)
		return
	}

	details := make(map[string]string, len(event.Details)+1)
	for key, value := range event.Details {
		details[key] = value
	}
	details["step"] = stepName
	event.Details = details

	if reporter.environment != nil {
		reporter.environment.observeStepEvent(event)
	}
	reporter.base.Report(event)
}

func (environment *Environment) stepScopedReporter() shared.Reporter {
	if environment == nil || environment.Reporter == nil {
		return nil
	}
	return &stepDecoratingReporter{
		base:        environment.Reporter,
		environment: environment,
	}
}

type stepOutcomeKind int

const (
	stepOutcomeUnknown stepOutcomeKind = iota
	stepOutcomeNoop
	stepOutcomeSkipped
	stepOutcomeApplied
	stepOutcomeFailed
)

type stepOutcome struct {
	kind   stepOutcomeKind
	reason string
}

func (environment *Environment) beginStep(repositoryPath string, stepName string) {
	if environment == nil {
		return
	}
	normalizedPath := filepath.Clean(strings.TrimSpace(repositoryPath))
	normalizedStep := strings.TrimSpace(stepName)
	if normalizedPath == "" || normalizedStep == "" {
		return
	}

	environment.ensureSharedState()
	environment.sharedState.mutex.Lock()
	defer environment.sharedState.mutex.Unlock()

	if environment.sharedState.stepOutcomes == nil {
		environment.sharedState.stepOutcomes = make(map[string]map[string]stepOutcome)
	}
	stepMap, exists := environment.sharedState.stepOutcomes[normalizedPath]
	if !exists {
		stepMap = make(map[string]stepOutcome)
		environment.sharedState.stepOutcomes[normalizedPath] = stepMap
	}
	stepMap[normalizedStep] = stepOutcome{kind: stepOutcomeUnknown}
}

func (environment *Environment) readStepOutcome(repositoryPath string, stepName string) (stepOutcome, bool) {
	if environment == nil {
		return stepOutcome{}, false
	}
	normalizedPath := filepath.Clean(strings.TrimSpace(repositoryPath))
	normalizedStep := strings.TrimSpace(stepName)
	if normalizedPath == "" || normalizedStep == "" {
		return stepOutcome{}, false
	}

	environment.ensureSharedState()
	environment.sharedState.mutex.Lock()
	defer environment.sharedState.mutex.Unlock()

	stepMap := environment.sharedState.stepOutcomes[normalizedPath]
	if len(stepMap) == 0 {
		return stepOutcome{}, false
	}
	outcome, exists := stepMap[normalizedStep]
	return outcome, exists
}

func (environment *Environment) observeStepEvent(event shared.Event) {
	if environment == nil {
		return
	}

	details := event.Details
	if details == nil {
		details = map[string]string{}
	}

	stepName := strings.TrimSpace(details["step"])
	if stepName == "" {
		stepName = strings.TrimSpace(environment.currentStepName)
	}

	repositoryPath := strings.TrimSpace(event.RepositoryPath)
	if repositoryPath == "" {
		repositoryPath = environment.repositoryKey()
	}

	normalizedPath := filepath.Clean(strings.TrimSpace(repositoryPath))
	if normalizedPath == "" || stepName == "" {
		return
	}

	event.Details = details
	kind, reason := classifyStepOutcome(event)

	environment.ensureSharedState()
	environment.sharedState.mutex.Lock()
	defer environment.sharedState.mutex.Unlock()

	if environment.sharedState.stepOutcomes == nil {
		environment.sharedState.stepOutcomes = make(map[string]map[string]stepOutcome)
	}

	stepMap, exists := environment.sharedState.stepOutcomes[normalizedPath]
	if !exists {
		stepMap = make(map[string]stepOutcome)
		environment.sharedState.stepOutcomes[normalizedPath] = stepMap
	}

	current := stepMap[stepName]
	if kind > current.kind {
		stepMap[stepName] = stepOutcome{kind: kind, reason: reason}
		return
	}
	if kind == current.kind && current.reason == "" && reason != "" {
		current.reason = reason
		stepMap[stepName] = current
	}
}

func classifyStepOutcome(event shared.Event) (stepOutcomeKind, string) {
	level := shared.EventLevel(strings.ToUpper(strings.TrimSpace(string(event.Level))))

	details := event.Details
	if details == nil {
		details = map[string]string{}
	}

	message := strings.TrimSpace(event.Message)
	detailsReason := strings.TrimSpace(details["reason"])
	reason := message
	if reason == "" {
		reason = detailsReason
	}

	if level == shared.EventLevelError {
		if reason == "" {
			reason = strings.TrimSpace(event.Code)
		}
		return stepOutcomeFailed, reason
	}

	switch event.Code {
	case shared.EventCodeRemoteUpdate,
		shared.EventCodeProtocolUpdate,
		shared.EventCodeFolderRename,
		shared.EventCodeNamespaceApply:
		if reason == "" {
			reason = strings.TrimSpace(event.Code)
		}
		return stepOutcomeApplied, reason
	case shared.EventCodeRepoSwitched:
		branchName := strings.TrimSpace(details["branch"])
		if branchName != "" {
			return stepOutcomeApplied, branchName
		}
		if reason == "" {
			reason = strings.TrimSpace(event.Code)
		}
		return stepOutcomeApplied, reason
	case shared.EventCodeTaskApply:
		taskName := strings.TrimSpace(details["task"])
		if taskName != "" {
			return stepOutcomeApplied, taskName
		}
		if reason == "" {
			reason = strings.TrimSpace(event.Code)
		}
		return stepOutcomeApplied, reason
	case shared.EventCodeRemoteDeclined,
		shared.EventCodeProtocolDeclined:
		if reason == "" {
			reason = "user declined"
		}
		return stepOutcomeSkipped, reason
	case shared.EventCodeRemoteSkip,
		shared.EventCodeProtocolSkip,
		shared.EventCodeFolderSkip,
		shared.EventCodeNamespaceNoop,
		shared.EventCodeNamespaceSkip,
		shared.EventCodeTaskSkip:
		if strings.Contains(strings.ToLower(message), "declined") || strings.Contains(strings.ToLower(reason), "declined") {
			if reason == "" {
				reason = "declined"
			}
			return stepOutcomeSkipped, reason
		}
		if strings.Contains(strings.ToLower(message), "requires changes") {
			return stepOutcomeNoop, message
		}
		if reason == "" {
			reason = "no-op"
		}
		return stepOutcomeNoop, reason
	default:
		if level == shared.EventLevelWarn {
			if reason == "" {
				reason = message
			}
			return stepOutcomeNoop, reason
		}
		return stepOutcomeUnknown, ""
	}
}

func (environment *Environment) reportStepSummary(repository *RepositoryState, stepName string, executionError error, repositorySkipped bool) {
	if environment == nil || repository == nil {
		return
	}

	normalizedStep := strings.TrimSpace(stepName)
	if normalizedStep == "" {
		normalizedStep = strings.TrimSpace(environment.currentStepName)
	}
	if normalizedStep == "" {
		return
	}

	outcome, exists := environment.readStepOutcome(repository.Path, normalizedStep)
	if !exists {
		outcome = stepOutcome{kind: stepOutcomeUnknown}
	}

	outcomeLabel := ""
	switch outcome.kind {
	case stepOutcomeFailed:
		outcomeLabel = "failed"
	case stepOutcomeApplied:
		outcomeLabel = "applied"
	case stepOutcomeSkipped:
		outcomeLabel = "skipped"
	case stepOutcomeNoop:
		outcomeLabel = "no-op"
	default:
		outcomeLabel = "unknown"
	}

	reason := strings.TrimSpace(outcome.reason)
	if repositorySkipped {
		outcomeLabel = "skipped"
		if reason == "" {
			reason = "repository skipped"
		}
	}

	if executionError != nil && !repositorySkipped {
		outcomeLabel = "failed"
		if reason == "" {
			reason = strings.TrimSpace(executionError.Error())
		}
	}

	if outcomeLabel == "unknown" && executionError == nil && !repositorySkipped {
		outcomeLabel = "ok"
	}

	details := map[string]string{
		"step":     normalizedStep,
		"outcome":  outcomeLabel,
		"reason":   reason,
		"executed": "true",
	}

	environment.ReportRepositoryEvent(
		repository,
		shared.EventLevelInfo,
		shared.EventCodeWorkflowStepSummary,
		"",
		details,
	)
}
