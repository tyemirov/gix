package workflow

import (
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
		reporter.base.Report(event)
		return
	}

	if event.Details == nil {
		event.Details = map[string]string{"step": stepName}
		reporter.base.Report(event)
		return
	}

	if _, exists := event.Details["step"]; exists {
		reporter.base.Report(event)
		return
	}

	details := make(map[string]string, len(event.Details)+1)
	for key, value := range event.Details {
		details[key] = value
	}
	details["step"] = stepName
	event.Details = details

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
