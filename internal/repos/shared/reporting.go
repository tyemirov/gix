package shared

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultLevelFieldWidth   = 5
	defaultEventFieldWidth   = 18
	defaultRepositoryWidth   = 34
	defaultMessageFieldWidth = 40
	defaultHeaderWidth       = 80
	defaultTimestampLayout   = "15:04:05"
)

// EventLevel describes the severity of a reported executor event.
type EventLevel string

// Supported event levels.
const (
	EventLevelInfo  EventLevel = "INFO"
	EventLevelWarn  EventLevel = "WARN"
	EventLevelError EventLevel = "ERROR"
)

// Event captures the structured information associated with a workflow action.
type Event struct {
	Timestamp            time.Time
	Level                EventLevel
	Code                 string
	RepositoryIdentifier string
	RepositoryPath       string
	Message              string
	Details              map[string]string
}

// Reporter emits structured executor events.
type Reporter interface {
	Report(event Event)
}

// SummaryReporter augments Reporter with summary emission support.
type SummaryReporter interface {
	Reporter
	Summary() string
	PrintSummary()
}

// ReporterOption customises StructuredReporter behaviour.
type ReporterOption func(*StructuredReporter)

// WithRepositoryHeaders toggles per-repository headers.
func WithRepositoryHeaders(enabled bool) ReporterOption {
	return func(reporter *StructuredReporter) {
		reporter.includeRepositoryHeaders = enabled
	}
}

// WithNowProvider overrides the time source used for timestamps and duration calculations.
func WithNowProvider(provider func() time.Time) ReporterOption {
	return func(reporter *StructuredReporter) {
		if provider != nil {
			reporter.now = provider
			reporter.startTime = provider()
		}
	}
}

// StructuredReporter formats events according to GX-212 requirements.
type StructuredReporter struct {
	outputWriter             io.Writer
	errorWriter              io.Writer
	includeRepositoryHeaders bool
	now                      func() time.Time

	mutex            sync.Mutex
	lastRepository   string
	startTime        time.Time
	eventCounts      map[string]int
	levelCounts      map[EventLevel]int
	seenRepositories map[string]struct{}
	columns          columnConfiguration
}

type columnConfiguration struct {
	levelWidth      int
	codeWidth       int
	repositoryWidth int
	messageWidth    int
	headerWidth     int
}

// NewStructuredReporter constructs a StructuredReporter that writes to the provided sinks.
func NewStructuredReporter(output io.Writer, errors io.Writer, options ...ReporterOption) *StructuredReporter {
	if output == nil || output == io.Discard {
		output = os.Stdout
	}
	if errors == nil || errors == io.Discard {
		errors = output
	}

	reporter := &StructuredReporter{
		outputWriter:             output,
		errorWriter:              errors,
		includeRepositoryHeaders: true,
		now:                      time.Now,
		startTime:                time.Now(),
		eventCounts:              make(map[string]int),
		levelCounts:              make(map[EventLevel]int),
		seenRepositories:         make(map[string]struct{}),
		columns: columnConfiguration{
			levelWidth:      defaultLevelFieldWidth,
			codeWidth:       defaultEventFieldWidth,
			repositoryWidth: defaultRepositoryWidth,
			messageWidth:    defaultMessageFieldWidth,
			headerWidth:     defaultHeaderWidth,
		},
	}

	for _, option := range options {
		option(reporter)
	}

	return reporter
}

// Report logs the provided event using the configured formatting rules.
func (reporter *StructuredReporter) Report(event Event) {
	if reporter == nil {
		return
	}

	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()

	timestamp := event.Timestamp
	if timestamp.IsZero() {
		timestamp = reporter.now()
	}

	level := normalizeLevel(event.Level)
	code := normalizeCode(event.Code)
	repositoryIdentifier := strings.TrimSpace(event.RepositoryIdentifier)
	repositoryPath := strings.TrimSpace(event.RepositoryPath)
	message := strings.TrimSpace(event.Message)

	writer := reporter.outputWriter
	if level == EventLevelError && reporter.errorWriter != nil {
		writer = reporter.errorWriter
	}

	if seenKey := reporter.computeSeenRepositoryKey(repositoryIdentifier, repositoryPath); len(seenKey) > 0 {
		reporter.seenRepositories[seenKey] = struct{}{}
	}
	reporter.eventCounts[code]++
	reporter.levelCounts[level]++

	if reporter.includeRepositoryHeaders && len(repositoryIdentifier) > 0 && repositoryIdentifier != reporter.lastRepository {
		reporter.printRepositoryHeader(writer, repositoryIdentifier)
		reporter.lastRepository = repositoryIdentifier
	}

	humanPart := reporter.formatHumanPart(timestamp, level, code, repositoryIdentifier, message)
	machinePart := reporter.formatMachinePart(code, repositoryIdentifier, repositoryPath, event.Details)

	fmt.Fprintf(writer, "%s | %s\n", humanPart, machinePart)
}

// Summary renders the aggregate statistics collected during reporting.
func (reporter *StructuredReporter) Summary() string {
	if reporter == nil {
		return ""
	}

	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()

	if len(reporter.eventCounts) == 0 && len(reporter.seenRepositories) == 0 {
		return "Summary: total.repos=0 duration_human=0s duration_ms=0"
	}

	keys := make([]string, 0, len(reporter.eventCounts))
	for key := range reporter.eventCounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys)+4)
	parts = append(parts, fmt.Sprintf("Summary: total.repos=%d", len(reporter.seenRepositories)))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, reporter.eventCounts[key]))
	}

	if warningCount, exists := reporter.levelCounts[EventLevelWarn]; exists {
		parts = append(parts, fmt.Sprintf("%s=%d", EventLevelWarn, warningCount))
	} else {
		parts = append(parts, fmt.Sprintf("%s=0", EventLevelWarn))
	}
	if errorCount, exists := reporter.levelCounts[EventLevelError]; exists {
		parts = append(parts, fmt.Sprintf("%s=%d", EventLevelError, errorCount))
	} else {
		parts = append(parts, fmt.Sprintf("%s=0", EventLevelError))
	}

	duration := reporter.now().Sub(reporter.startTime)
	if duration < 0 {
		duration = 0
	}
	humanDuration := reporter.formatDuration(duration)
	parts = append(parts, fmt.Sprintf("duration_human=%s", humanDuration))
	parts = append(parts, fmt.Sprintf("duration_ms=%d", duration.Milliseconds()))

	return strings.Join(parts, " ")
}

// PrintSummary writes the computed summary to the primary output writer.
func (reporter *StructuredReporter) PrintSummary() {
	if reporter == nil {
		return
	}
	summary := reporter.Summary()
	if len(strings.TrimSpace(summary)) == 0 {
		return
	}

	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()

	fmt.Fprintln(reporter.outputWriter, summary)
}

func (reporter *StructuredReporter) computeSeenRepositoryKey(identifier string, path string) string {
	if len(identifier) > 0 {
		return identifier
	}
	if len(strings.TrimSpace(path)) > 0 {
		return strings.TrimSpace(path)
	}
	return ""
}

func (reporter *StructuredReporter) formatDuration(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	rounded := value.Round(time.Millisecond)
	if rounded == 0 && value > 0 {
		rounded = time.Millisecond
	}
	return rounded.String()
}

func (reporter *StructuredReporter) printRepositoryHeader(writer io.Writer, repositoryIdentifier string) {
	if writer == nil {
		return
	}

	headerContent := fmt.Sprintf("repo: %s", repositoryIdentifier)
	paddingWidth := reporter.columns.headerWidth - len(headerContent) - 4
	if paddingWidth < 0 {
		paddingWidth = 0
	}
	padding := strings.Repeat("-", paddingWidth)
	fmt.Fprintf(writer, "-- %s %s\n", headerContent, padding)
}

func (reporter *StructuredReporter) formatHumanPart(timestamp time.Time, level EventLevel, code string, repositoryIdentifier string, message string) string {
	levelField := fmt.Sprintf("%-*s", reporter.columns.levelWidth, string(level))
	codeField := fmt.Sprintf("%-*s", reporter.columns.codeWidth, code)
	repositoryField := fmt.Sprintf("%-*s", reporter.columns.repositoryWidth, repositoryIdentifier)
	messageField := fmt.Sprintf("%-*s", reporter.columns.messageWidth, message)

	return fmt.Sprintf("%s %s %s %s %s", timestamp.Format(defaultTimestampLayout), levelField, codeField, repositoryField, messageField)
}

func (reporter *StructuredReporter) formatMachinePart(code string, repositoryIdentifier string, repositoryPath string, details map[string]string) string {
	values := make(map[string]string, len(details)+3)
	values["event"] = code
	if len(repositoryIdentifier) > 0 {
		values["repo"] = repositoryIdentifier
	}
	if len(repositoryPath) > 0 {
		values["path"] = repositoryPath
	}

	for key, value := range details {
		values[key] = value
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%s", key, values[key]))
	}
	return strings.Join(pairs, " ")
}

func normalizeLevel(level EventLevel) EventLevel {
	switch level {
	case EventLevelWarn:
		return EventLevelWarn
	case EventLevelError:
		return EventLevelError
	default:
		return EventLevelInfo
	}
}

func normalizeCode(code string) string {
	trimmed := strings.TrimSpace(code)
	if len(trimmed) == 0 {
		return "UNKNOWN"
	}
	uppercased := strings.ToUpper(trimmed)
	return strings.ReplaceAll(uppercased, " ", "_")
}
