package workflow

import (
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/audit"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	// Task action identifiers shared by CLI commands, web primitives, and workflow builders.
	TaskActionCanonicalRemoteType    = taskActionCanonicalRemote
	TaskActionProtocolConversionType = taskActionProtocolConversion
	TaskActionRenameDirectoriesType  = taskActionRenameDirectories
	TaskActionBranchDefaultType      = taskActionBranchDefault
	TaskActionReleaseTagType         = taskActionReleaseTag
	TaskActionReleaseRetagType       = taskActionReleaseRetag
	TaskActionAuditReportType        = taskActionAuditReport
	TaskActionHistoryPurgeType       = taskActionHistoryPurge
	TaskActionFileReplaceType        = taskActionFileReplace
	TaskActionNamespaceRewriteType   = taskActionNamespaceRewrite
)

const (
	actionOptionSourceKeyConstant             = "source"
	actionOptionTargetKeyConstant             = "target"
	actionOptionRemoteKeyConstant             = "remote"
	actionOptionPushKeyConstant               = "push"
	actionOptionDeleteSourceBranchKeyConstant = "delete_source_branch"
	actionOptionTagKeyConstant                = "tag"
	actionOptionMessageKeyConstant            = "message"
	actionOptionMappingsKeyConstant           = "mappings"
	actionOptionIncludeAllKeyConstant         = "include_all"
	actionOptionDebugKeyConstant              = "debug"
	actionOptionDepthKeyConstant              = "depth"
	actionOptionRestoreKeyConstant            = "restore"
	actionOptionPushMissingKeyConstant        = "push_missing"
	safeguardHardStopKeyConstant              = "hard_stop"
	safeguardSoftSkipKeyConstant              = "soft_skip"
	safeguardBranchKeyConstant                = "branch"
)

// CanonicalRemoteActionOptions serializes repo.remote.update options.
type CanonicalRemoteActionOptions struct {
	Owner string
}

// Options returns workflow action options for canonical remote updates.
func (options CanonicalRemoteActionOptions) Options() map[string]any {
	return map[string]any{
		optionOwnerKeyConstant: strings.TrimSpace(options.Owner),
	}
}

// ProtocolConversionActionOptions serializes repo.remote.convert-protocol options.
type ProtocolConversionActionOptions struct {
	From shared.RemoteProtocol
	To   shared.RemoteProtocol
}

// Options returns workflow action options for remote protocol conversion.
func (options ProtocolConversionActionOptions) Options() map[string]any {
	return compactStringOptions(map[string]string{
		optionFromKeyConstant: string(options.From),
		optionToKeyConstant:   string(options.To),
	})
}

// RenameDirectoriesActionOptions serializes repo.folder.rename options.
type RenameDirectoriesActionOptions struct {
	RequireClean bool
	IncludeOwner bool
}

// Options returns workflow action options for repository folder renames.
func (options RenameDirectoriesActionOptions) Options() map[string]any {
	return map[string]any{
		optionRequireCleanKeyConstant: options.RequireClean,
		optionIncludeOwnerKeyConstant: options.IncludeOwner,
	}
}

// BranchDefaultActionOptions serializes branch.default action options.
type BranchDefaultActionOptions struct {
	RemoteName         string
	SourceBranch       string
	TargetBranch       string
	PushToRemote       bool
	DeleteSourceBranch bool
}

// Options returns workflow action options for default branch promotion.
func (options BranchDefaultActionOptions) Options() map[string]any {
	serialized := compactStringOptions(map[string]string{
		actionOptionRemoteKeyConstant: options.RemoteName,
		actionOptionSourceKeyConstant: options.SourceBranch,
		actionOptionTargetKeyConstant: options.TargetBranch,
	})
	serialized[actionOptionPushKeyConstant] = options.PushToRemote
	serialized[actionOptionDeleteSourceBranchKeyConstant] = options.DeleteSourceBranch
	return serialized
}

// ReleaseTagActionOptions serializes repo.release.tag options.
type ReleaseTagActionOptions struct {
	TagName    string
	Message    string
	RemoteName string
}

// Options returns workflow action options for release tag creation.
func (options ReleaseTagActionOptions) Options() map[string]any {
	return compactStringOptions(map[string]string{
		actionOptionTagKeyConstant:     options.TagName,
		actionOptionMessageKeyConstant: options.Message,
		actionOptionRemoteKeyConstant:  options.RemoteName,
	})
}

// ReleaseRetagMappingOptions describes one repo.release.retag mapping.
type ReleaseRetagMappingOptions struct {
	TagName         string
	TargetReference string
	Message         string
}

// ReleaseRetagActionOptions serializes repo.release.retag options.
type ReleaseRetagActionOptions struct {
	RemoteName string
	Mappings   []ReleaseRetagMappingOptions
}

// Options returns workflow action options for release retagging.
func (options ReleaseRetagActionOptions) Options() map[string]any {
	serialized := compactStringOptions(map[string]string{
		actionOptionRemoteKeyConstant: options.RemoteName,
	})
	if len(options.Mappings) > 0 {
		mappings := make([]map[string]any, 0, len(options.Mappings))
		for mappingIndex := range options.Mappings {
			mapping := options.Mappings[mappingIndex]
			mappingOptions := compactStringOptions(map[string]string{
				actionOptionTagKeyConstant:     mapping.TagName,
				actionOptionTargetKeyConstant:  mapping.TargetReference,
				actionOptionMessageKeyConstant: mapping.Message,
			})
			mappings = append(mappings, mappingOptions)
		}
		serialized[actionOptionMappingsKeyConstant] = mappings
	}
	return serialized
}

// AuditReportActionOptions serializes audit.report options.
type AuditReportActionOptions struct {
	IncludeAll bool
	Debug      bool
	Depth      audit.InspectionDepth
	OutputPath string
}

// Options returns workflow action options for audit report generation.
func (options AuditReportActionOptions) Options() map[string]any {
	serialized := compactStringOptions(map[string]string{
		actionOptionDepthKeyConstant: string(options.Depth),
		optionOutputPathKeyConstant:  options.OutputPath,
	})
	serialized[actionOptionIncludeAllKeyConstant] = options.IncludeAll
	serialized[actionOptionDebugKeyConstant] = options.Debug
	return serialized
}

// HistoryPurgeActionOptions serializes repo.history.purge options.
type HistoryPurgeActionOptions struct {
	Paths       []string
	RemoteName  string
	Push        bool
	Restore     bool
	PushMissing bool
}

// Options returns workflow action options for repository history purges.
func (options HistoryPurgeActionOptions) Options() map[string]any {
	serialized := map[string]any{
		optionPathsKeyConstant:             cloneStringSlice(options.Paths),
		actionOptionPushKeyConstant:        options.Push,
		actionOptionRestoreKeyConstant:     options.Restore,
		actionOptionPushMissingKeyConstant: options.PushMissing,
	}
	if trimmedRemote := strings.TrimSpace(options.RemoteName); trimmedRemote != "" {
		serialized[actionOptionRemoteKeyConstant] = trimmedRemote
	}
	return serialized
}

// FileReplaceActionOptions serializes repo.files.replace options.
type FileReplaceActionOptions struct {
	Patterns []string
	Find     string
	Replace  string
	Command  []string
}

// Options returns workflow action options for repository file replacement.
func (options FileReplaceActionOptions) Options() map[string]any {
	serialized := map[string]any{
		fileReplaceFindOptionKey:    strings.TrimSpace(options.Find),
		fileReplaceReplaceOptionKey: options.Replace,
	}

	normalizedPatterns := compactStringSlice(options.Patterns)
	switch len(normalizedPatterns) {
	case 0:
	case 1:
		serialized[fileReplacePatternOptionKey] = normalizedPatterns[0]
	default:
		serialized[fileReplacePatternsOptionKey] = normalizedPatterns
	}

	command := compactStringSlice(options.Command)
	if len(command) > 0 {
		serialized[fileReplaceCommandOptionKey] = command
	}

	return serialized
}

// FileReplaceSafeguardOptions serializes safeguards for repo.files.replace presets.
type FileReplaceSafeguardOptions struct {
	RequireClean bool
	Branch       string
	Paths        []string
}

// Options returns task safeguard options for repository file replacement.
func (options FileReplaceSafeguardOptions) Options() map[string]any {
	hardStop := map[string]any{}
	if options.RequireClean {
		hardStop[optionRequireCleanKeyConstant] = true
	}

	softSkip := compactStringOptions(map[string]string{
		safeguardBranchKeyConstant: options.Branch,
	})
	if paths := compactStringSlice(options.Paths); len(paths) > 0 {
		softSkip[optionPathsKeyConstant] = paths
	}

	serialized := map[string]any{}
	if len(hardStop) > 0 {
		serialized[safeguardHardStopKeyConstant] = hardStop
	}
	if len(softSkip) > 0 {
		serialized[safeguardSoftSkipKeyConstant] = softSkip
	}
	if len(serialized) == 0 {
		return nil
	}
	return serialized
}

// HistoryPurgeActionOverrides describes variable-driven updates to a history purge action.
type HistoryPurgeActionOverrides struct {
	Paths       []string
	RemoteName  string
	Push        *bool
	Restore     *bool
	PushMissing *bool
}

// Empty reports whether the override carries no action changes.
func (overrides HistoryPurgeActionOverrides) Empty() bool {
	return len(overrides.Paths) == 0 &&
		strings.TrimSpace(overrides.RemoteName) == "" &&
		overrides.Push == nil &&
		overrides.Restore == nil &&
		overrides.PushMissing == nil
}

// ApplyHistoryPurgeActionOverrides updates the first history purge action embedded in tasks.apply options.
func ApplyHistoryPurgeActionOverrides(options map[string]any, overrides HistoryPurgeActionOverrides) (map[string]any, bool, error) {
	if len(options) == 0 || overrides.Empty() {
		return options, false, nil
	}

	tasksValue, tasksExist := options[optionTasksKeyConstant].([]any)
	if !tasksExist {
		return options, false, nil
	}

	for taskIndex := range tasksValue {
		taskEntry, taskOk := tasksValue[taskIndex].(map[string]any)
		if !taskOk {
			continue
		}

		actionsValue, actionsExist := taskEntry[optionTaskActionsKeyConstant].([]any)
		if !actionsExist {
			continue
		}

		for actionIndex := range actionsValue {
			actionEntry, actionOk := actionsValue[actionIndex].(map[string]any)
			if !actionOk {
				continue
			}

			actionType, typeOk := actionEntry[optionTaskActionTypeKeyConstant].(string)
			if !typeOk || !strings.EqualFold(strings.TrimSpace(actionType), taskActionHistoryPurge) {
				continue
			}

			actionOptions, mergeError := mergeHistoryPurgeActionOptions(actionEntry[optionTaskActionOptionsKeyConstant], overrides)
			if mergeError != nil {
				return options, false, mergeError
			}

			actionEntry[optionTaskActionOptionsKeyConstant] = actionOptions
			actionsValue[actionIndex] = actionEntry
			taskEntry[optionTaskActionsKeyConstant] = actionsValue
			tasksValue[taskIndex] = taskEntry
			options[optionTasksKeyConstant] = tasksValue
			return options, true, nil
		}
	}

	return options, false, nil
}

func mergeHistoryPurgeActionOptions(rawOptions any, overrides HistoryPurgeActionOverrides) (map[string]any, error) {
	currentOptions := map[string]any{}
	if rawOptions != nil {
		var optionsOk bool
		currentOptions, optionsOk = rawOptions.(map[string]any)
		if !optionsOk {
			return nil, fmt.Errorf("history purge action options must be a map")
		}
	}

	merged, readError := readHistoryPurgeActionOptions(currentOptions)
	if readError != nil {
		return nil, readError
	}
	if len(overrides.Paths) > 0 {
		merged.Paths = compactStringSlice(overrides.Paths)
	}
	if remoteName := strings.TrimSpace(overrides.RemoteName); remoteName != "" {
		merged.RemoteName = remoteName
	}
	if overrides.Push != nil {
		merged.Push = *overrides.Push
	}
	if overrides.Restore != nil {
		merged.Restore = *overrides.Restore
	}
	if overrides.PushMissing != nil {
		merged.PushMissing = *overrides.PushMissing
	}

	return merged.Options(), nil
}

func readHistoryPurgeActionOptions(options map[string]any) (HistoryPurgeActionOptions, error) {
	reader := newOptionReader(options)
	merged := HistoryPurgeActionOptions{
		Push:    true,
		Restore: true,
	}

	if rawPaths, pathsExist := options[optionPathsKeyConstant]; pathsExist {
		paths, pathsError := readHistoryPaths(rawPaths)
		if pathsError != nil {
			return HistoryPurgeActionOptions{}, pathsError
		}
		merged.Paths = paths
	}

	remoteName, _, remoteError := reader.stringValue(actionOptionRemoteKeyConstant)
	if remoteError != nil {
		return HistoryPurgeActionOptions{}, remoteError
	}
	merged.RemoteName = remoteName

	if push, pushExists, pushError := reader.boolValue(actionOptionPushKeyConstant); pushError != nil {
		return HistoryPurgeActionOptions{}, pushError
	} else if pushExists {
		merged.Push = push
	}
	if restore, restoreExists, restoreError := reader.boolValue(actionOptionRestoreKeyConstant); restoreError != nil {
		return HistoryPurgeActionOptions{}, restoreError
	} else if restoreExists {
		merged.Restore = restore
	}
	if pushMissing, pushMissingExists, pushMissingError := reader.boolValue(actionOptionPushMissingKeyConstant); pushMissingError != nil {
		return HistoryPurgeActionOptions{}, pushMissingError
	} else if pushMissingExists {
		merged.PushMissing = pushMissing
	}

	return merged, nil
}

func compactStringOptions(options map[string]string) map[string]any {
	serialized := map[string]any{}
	for key, value := range options {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			serialized[key] = trimmed
		}
	}
	return serialized
}

func compactStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	serialized := make([]string, 0, len(values))
	for valueIndex := range values {
		trimmed := strings.TrimSpace(values[valueIndex])
		if trimmed != "" {
			serialized = append(serialized, trimmed)
		}
	}
	if len(serialized) == 0 {
		return nil
	}
	return serialized
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string{}, values...)
}

// MergeOptions returns a new workflow options map with override values applied.
func MergeOptions(base map[string]any, overrides map[string]any) map[string]any {
	merged := cloneStringAnyMap(base)
	if merged == nil {
		merged = map[string]any{}
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}
