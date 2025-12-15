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

var consoleSuppressedEventCodes = map[string]struct{}{
	EventCodeTaskPlan:            {},
	EventCodeTaskApply:           {},
	EventCodeWorkflowStepSummary: {},
}

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
	SummaryData() SummaryData
	PrintSummary()
	RecordEvent(code string, level EventLevel)
	RecordOperationDuration(operationName string, duration time.Duration)
	RecordStageDuration(stageName string, duration time.Duration)
}

// SummaryData captures aggregated reporter metrics suitable for telemetry export.
type SummaryData struct {
	TotalRepositories    int                                 `json:"total_repositories"`
	EventCounts          map[string]int                      `json:"event_counts"`
	LevelCounts          map[EventLevel]int                  `json:"level_counts"`
	DurationHuman        string                              `json:"duration_human"`
	DurationMilliseconds int64                               `json:"duration_ms"`
	OperationDurations   map[string]OperationDurationSummary `json:"operation_durations"`
	StageDurations       map[string]OperationDurationSummary `json:"stage_durations"`
}

// OperationDurationSummary captures aggregated timing metrics for a workflow operation.
type OperationDurationSummary struct {
	Count                       int   `json:"count"`
	TotalDurationMilliseconds   int64 `json:"total_duration_ms"`
	AverageDurationMilliseconds int64 `json:"average_duration_ms"`
}

// ReporterOption customises StructuredReporter behaviour.
type ReporterOption func(*StructuredReporter)

// EventFormatter customises how human-readable events are rendered.
type EventFormatter interface {
	HandleEvent(event Event, writer io.Writer)
}

// WithRepositoryHeaders toggles per-repository headers.
func WithRepositoryHeaders(enabled bool) ReporterOption {
	return func(reporter *StructuredReporter) {
		reporter.includeRepositoryHeaders = enabled
	}
}

// WithEventFormatter installs a custom event formatter for human-readable output.
func WithEventFormatter(formatter EventFormatter) ReporterOption {
	return func(reporter *StructuredReporter) {
		reporter.eventFormatter = formatter
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

	mutex              sync.Mutex
	lastRepository     string
	startTime          time.Time
	eventCounts        map[string]int
	levelCounts        map[EventLevel]int
	seenRepositories   map[string]struct{}
	operationDurations map[string]*operationDurationAccumulator
	stageDurations     map[string]*operationDurationAccumulator
	columns            columnConfiguration
	eventFormatter     EventFormatter
}

type columnConfiguration struct {
	levelWidth      int
	codeWidth       int
	repositoryWidth int
	messageWidth    int
	headerWidth     int
}

type operationDurationAccumulator struct {
	count int
	total time.Duration
}

// NewStructuredReporter constructs a StructuredReporter that writes to the provided sinks.
func NewStructuredReporter(output io.Writer, errors io.Writer, options ...ReporterOption) *StructuredReporter {
	if output == nil {
		output = os.Stdout
	}
	if errors == nil {
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
		operationDurations:       make(map[string]*operationDurationAccumulator),
		stageDurations:           make(map[string]*operationDurationAccumulator),
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

// RecordRepository registers a repository as observed without emitting an event.
func (reporter *StructuredReporter) RecordRepository(identifier string, path string) {
	if reporter == nil {
		return
	}

	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()

	key := reporter.computeSeenRepositoryKey(strings.TrimSpace(identifier), strings.TrimSpace(path))
	if len(key) == 0 {
		return
	}

	reporter.seenRepositories[key] = struct{}{}
}

// RecordEvent increments event and level counters without emitting log output.
func (reporter *StructuredReporter) RecordEvent(code string, level EventLevel) {
	if reporter == nil {
		return
	}

	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()

	normalizedCode := normalizeCode(code)
	normalizedLevel := normalizeLevel(level)

	reporter.eventCounts[normalizedCode]++
	reporter.levelCounts[normalizedLevel]++
}

// RecordOperationDuration aggregates timing information for the provided operation.
func (reporter *StructuredReporter) RecordOperationDuration(operationName string, duration time.Duration) {
	if reporter == nil {
		return
	}

	trimmedName := strings.TrimSpace(operationName)
	if len(trimmedName) == 0 {
		return
	}
	if duration < 0 {
		duration = 0
	}

	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()

	accumulator, exists := reporter.operationDurations[trimmedName]
	if !exists || accumulator == nil {
		accumulator = &operationDurationAccumulator{}
		reporter.operationDurations[trimmedName] = accumulator
	}

	accumulator.count++
	accumulator.total += duration
}

// RecordStageDuration aggregates timing information for a workflow stage.
func (reporter *StructuredReporter) RecordStageDuration(stageName string, duration time.Duration) {
	if reporter == nil {
		return
	}

	trimmedName := strings.TrimSpace(stageName)
	if len(trimmedName) == 0 {
		return
	}

	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()

	accumulator, exists := reporter.stageDurations[trimmedName]
	if !exists {
		accumulator = &operationDurationAccumulator{}
		reporter.stageDurations[trimmedName] = accumulator
	}
	accumulator.count++
	accumulator.total += duration
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

	if reporter.eventFormatter != nil {
		reporter.eventFormatter.HandleEvent(event, writer)
		return
	}

	if reporter.includeRepositoryHeaders {
		if reporter.shouldSuppressConsoleEvent(code, level) {
			return
		}

		if len(repositoryIdentifier) > 0 && repositoryIdentifier != reporter.lastRepository {
			reporter.printRepositoryHeader(writer, repositoryIdentifier)
			reporter.lastRepository = repositoryIdentifier
		}

		consolePart := reporter.formatConsolePart(timestamp, level, code, message)
		if len(consolePart) == 0 {
			return
		}
		fmt.Fprintln(writer, consolePart)
		return
	}

	humanPart := reporter.formatHumanPart(timestamp, level, code, repositoryIdentifier, message)
	machinePart := reporter.formatMachinePart(code, repositoryIdentifier, repositoryPath, event.Details)

	fmt.Fprintf(writer, "%s | %s\n", humanPart, machinePart)
}

// SummaryData produces a serializable snapshot of reporter metrics.
func (reporter *StructuredReporter) SummaryData() SummaryData {
	if reporter == nil {
		return SummaryData{
			EventCounts:        make(map[string]int),
			LevelCounts:        make(map[EventLevel]int),
			OperationDurations: make(map[string]OperationDurationSummary),
			StageDurations:     make(map[string]OperationDurationSummary),
			DurationHuman:      "0s",
		}
	}

	reporter.mutex.Lock()
	defer reporter.mutex.Unlock()

	duration := reporter.now().Sub(reporter.startTime)
	if duration < 0 {
		duration = 0
	}

	eventCounts := cloneStringIntMap(reporter.eventCounts)
	levelCounts := cloneLevelCountMap(reporter.levelCounts)
	operationDurations := make(map[string]OperationDurationSummary, len(reporter.operationDurations))
	stageDurations := make(map[string]OperationDurationSummary, len(reporter.stageDurations))

	for name, accumulator := range reporter.operationDurations {
		if accumulator == nil || accumulator.count == 0 {
			continue
		}
		total := accumulator.total
		operationDurations[name] = OperationDurationSummary{
			Count:                       accumulator.count,
			TotalDurationMilliseconds:   reporter.durationMilliseconds(total),
			AverageDurationMilliseconds: reporter.averageDurationMilliseconds(total, accumulator.count),
		}
	}

	for name, accumulator := range reporter.stageDurations {
		if accumulator == nil || accumulator.count == 0 {
			continue
		}
		total := accumulator.total
		stageDurations[name] = OperationDurationSummary{
			Count:                       accumulator.count,
			TotalDurationMilliseconds:   reporter.durationMilliseconds(total),
			AverageDurationMilliseconds: reporter.averageDurationMilliseconds(total, accumulator.count),
		}
	}

	return SummaryData{
		TotalRepositories:    len(reporter.seenRepositories),
		EventCounts:          eventCounts,
		LevelCounts:          levelCounts,
		DurationHuman:        reporter.formatDuration(duration),
		DurationMilliseconds: reporter.durationMilliseconds(duration),
		OperationDurations:   operationDurations,
		StageDurations:       stageDurations,
	}
}

// Summary renders the aggregate statistics collected during reporting.
func (reporter *StructuredReporter) Summary() string {
	data := reporter.SummaryData()
	if data.TotalRepositories == 0 && len(data.EventCounts) == 0 {
		return "Summary: total.repos=0 duration_human=0s duration_ms=0"
	}

	keys := make([]string, 0, len(data.EventCounts))
	for key := range data.EventCounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys)+4)
	parts = append(parts, fmt.Sprintf("Summary: total.repos=%d", data.TotalRepositories))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, data.EventCounts[key]))
	}

	parts = append(parts, fmt.Sprintf("%s=%d", EventLevelWarn, data.LevelCounts[EventLevelWarn]))
	parts = append(parts, fmt.Sprintf("%s=%d", EventLevelError, data.LevelCounts[EventLevelError]))
	parts = append(parts, fmt.Sprintf("duration_human=%s", data.DurationHuman))
	parts = append(parts, fmt.Sprintf("duration_ms=%d", data.DurationMilliseconds))

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
	trimmedIdentifier := strings.TrimSpace(identifier)
	trimmedPath := strings.TrimSpace(path)
	if trimmedIdentifier != "" && trimmedPath != "" {
		return trimmedIdentifier + "|" + trimmedPath
	}
	if trimmedIdentifier != "" {
		return trimmedIdentifier
	}
	if trimmedPath != "" {
		return trimmedPath
	}
	return ""
}

func cloneStringIntMap(source map[string]int) map[string]int {
	if len(source) == 0 {
		return make(map[string]int)
	}
	target := make(map[string]int, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}

func cloneLevelCountMap(source map[EventLevel]int) map[EventLevel]int {
	if len(source) == 0 {
		return make(map[EventLevel]int)
	}
	target := make(map[EventLevel]int, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
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

func (reporter *StructuredReporter) durationMilliseconds(value time.Duration) int64 {
	if value < 0 {
		value = 0
	}
	rounded := value.Round(time.Millisecond)
	if rounded == 0 && value > 0 {
		rounded = time.Millisecond
	}
	return rounded.Milliseconds()
}

func (reporter *StructuredReporter) averageDurationMilliseconds(total time.Duration, count int) int64 {
	if count <= 0 {
		return 0
	}
	average := total / time.Duration(count)
	return reporter.durationMilliseconds(average)
}

func (reporter *StructuredReporter) formatHumanPart(timestamp time.Time, level EventLevel, code string, repositoryIdentifier string, message string) string {
	levelField := fmt.Sprintf("%-*s", reporter.columns.levelWidth, string(level))
	codeField := fmt.Sprintf("%-*s", reporter.columns.codeWidth, code)
	repositoryField := fmt.Sprintf("%-*s", reporter.columns.repositoryWidth, repositoryIdentifier)
	messageField := fmt.Sprintf("%-*s", reporter.columns.messageWidth, message)

	return fmt.Sprintf("%s %s %s %s %s", timestamp.Format(defaultTimestampLayout), levelField, codeField, repositoryField, messageField)
}

func (reporter *StructuredReporter) formatConsolePart(timestamp time.Time, level EventLevel, code string, message string) string {
	trimmedMessage := strings.TrimSpace(message)
	trimmedCode := strings.TrimSpace(code)

	levelField := fmt.Sprintf("%-*s", reporter.columns.levelWidth, string(level))
	codeField := fmt.Sprintf("%-*s", reporter.columns.codeWidth, trimmedCode)

	switch {
	case trimmedMessage != "" && trimmedCode != "":
		return fmt.Sprintf("%s %s %s %s", timestamp.Format(defaultTimestampLayout), levelField, codeField, trimmedMessage)
	case trimmedMessage != "":
		return fmt.Sprintf("%s %s %s", timestamp.Format(defaultTimestampLayout), levelField, trimmedMessage)
	case trimmedCode != "":
		return fmt.Sprintf("%s %s %s", timestamp.Format(defaultTimestampLayout), levelField, trimmedCode)
	default:
		return ""
	}
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

func (reporter *StructuredReporter) shouldSuppressConsoleEvent(code string, level EventLevel) bool {
	if level == EventLevelError {
		return false
	}
	_, suppressed := consoleSuppressedEventCodes[normalizeCode(code)]
	return suppressed
}
