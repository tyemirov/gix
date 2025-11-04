package workflow

import (
	"errors"
	"fmt"
	"strings"

	"github.com/temirov/gix/internal/repos/shared"
)

const (
	protocolConversionInvalidFromMessageConstant  = "repo remote update-protocol step requires a valid 'from' protocol"
	protocolConversionInvalidToMessageConstant    = "repo remote update-protocol step requires a valid 'to' protocol"
	protocolConversionSameProtocolMessageConstant = "repo remote update-protocol step requires distinct source and target protocols"
	branchMigrationTargetsRequiredMessageConstant = "branch default step requires at least one target"
)

// BuildOperations converts the declarative configuration into executable operations with dependency metadata.
func BuildOperations(configuration Configuration) ([]*OperationNode, error) {
	nodes := make([]*OperationNode, 0, len(configuration.Steps))
	stepNames := make(map[string]struct{}, len(configuration.Steps))
	stepOrder := make([]string, 0, len(configuration.Steps))

	for stepIndex := range configuration.Steps {
		step := configuration.Steps[stepIndex]
		operation, buildError := buildOperationFromStep(step)
		if buildError != nil {
			return nil, buildError
		}

		stepName := strings.TrimSpace(step.Name)
		if len(stepName) == 0 {
			stepName = fmt.Sprintf("%s-%d", CommandPathKey(step.Command), stepIndex+1)
		}
		if _, exists := stepNames[stepName]; exists {
			return nil, fmt.Errorf("workflow step name %q defined multiple times", stepName)
		}

		dependencies := make([]string, 0)
		if step.After != nil {
			seenDependencies := make(map[string]struct{})
			for _, dependencyName := range step.After {
				trimmedDependency := strings.TrimSpace(dependencyName)
				if len(trimmedDependency) == 0 {
					continue
				}
				if trimmedDependency == stepName {
					return nil, fmt.Errorf("workflow step %q cannot depend on itself", stepName)
				}
				if _, alreadyIncluded := seenDependencies[trimmedDependency]; alreadyIncluded {
					continue
				}
				seenDependencies[trimmedDependency] = struct{}{}
				dependencies = append(dependencies, trimmedDependency)
			}
		} else if len(stepOrder) > 0 {
			dependencies = append(dependencies, stepOrder[len(stepOrder)-1])
		}

		node := &OperationNode{
			Name:         stepName,
			Operation:    operation,
			Dependencies: dependencies,
		}

		nodes = append(nodes, node)
		stepNames[stepName] = struct{}{}
		stepOrder = append(stepOrder, stepName)
	}

	for nodeIndex := range nodes {
		node := nodes[nodeIndex]
		for _, dependencyName := range node.Dependencies {
			if _, exists := stepNames[dependencyName]; !exists {
				return nil, fmt.Errorf("workflow step %q depends on unknown step %q", node.Name, dependencyName)
			}
		}
	}

	return nodes, nil
}

func buildOperationFromStep(step StepConfiguration) (Operation, error) {
	commandKey := CommandPathKey(step.Command)
	if len(commandKey) == 0 {
		return nil, errors.New(configurationCommandMissingMessageConstant)
	}

	normalizedOptions := step.Options

	switch commandKey {
	case commandRepoRemoteConvertProtocolKey:
		return buildProtocolConversionOperation(normalizedOptions)
	case commandRepoRemoteCanonicalKey:
		return buildCanonicalRemoteOperation(normalizedOptions)
	case commandRepoFolderRenameKey:
		return buildRenameOperation(normalizedOptions)
	case commandBranchDefaultKey:
		return buildBranchMigrationOperation(normalizedOptions)
	case commandAuditReportKey:
		return buildAuditReportOperation(normalizedOptions)
	case commandRepoTasksApplyKey:
		return buildTaskOperation(normalizedOptions)
	default:
		return nil, fmt.Errorf("unsupported workflow command: %s", commandKey)
	}
}

func buildProtocolConversionOperation(options map[string]any) (Operation, error) {
	reader := newOptionReader(options)
	fromValue, fromExists, fromError := reader.stringValue(optionFromKeyConstant)
	if fromError != nil {
		return nil, fromError
	}
	if !fromExists || len(fromValue) == 0 {
		return nil, errors.New(protocolConversionInvalidFromMessageConstant)
	}

	toValue, toExists, toError := reader.stringValue(optionToKeyConstant)
	if toError != nil {
		return nil, toError
	}
	if !toExists || len(toValue) == 0 {
		return nil, errors.New(protocolConversionInvalidToMessageConstant)
	}

	fromProtocol, fromParseError := parseProtocolValue(fromValue)
	if fromParseError != nil {
		return nil, fromParseError
	}

	toProtocol, toParseError := parseProtocolValue(toValue)
	if toParseError != nil {
		return nil, toParseError
	}

	if fromProtocol == toProtocol {
		return nil, errors.New(protocolConversionSameProtocolMessageConstant)
	}

	return &ProtocolConversionOperation{FromProtocol: fromProtocol, ToProtocol: toProtocol}, nil
}

func buildCanonicalRemoteOperation(options map[string]any) (Operation, error) {
	reader := newOptionReader(options)
	ownerValue, _, ownerError := reader.stringValue(optionOwnerKeyConstant)
	if ownerError != nil {
		return nil, ownerError
	}

	return &CanonicalRemoteOperation{OwnerConstraint: strings.TrimSpace(ownerValue)}, nil
}

func buildRenameOperation(options map[string]any) (Operation, error) {
	reader := newOptionReader(options)
	requireClean, requireCleanExplicit, requireCleanError := reader.boolValue(optionRequireCleanKeyConstant)
	if requireCleanError != nil {
		return nil, requireCleanError
	}
	includeOwner, _, includeOwnerError := reader.boolValue(optionIncludeOwnerKeyConstant)
	if includeOwnerError != nil {
		return nil, includeOwnerError
	}
	return &RenameOperation{
		RequireCleanWorktree: requireClean,
		requireCleanExplicit: requireCleanExplicit,
		IncludeOwner:         includeOwner,
	}, nil
}

func buildBranchMigrationOperation(options map[string]any) (Operation, error) {
	reader := newOptionReader(options)
	targetEntries, targetsExist, targetsError := reader.mapSlice(optionTargetsKeyConstant)
	if targetsError != nil {
		return nil, targetsError
	}
	if !targetsExist || len(targetEntries) == 0 {
		return nil, errors.New(branchMigrationTargetsRequiredMessageConstant)
	}

	targets := make([]BranchMigrationTarget, 0, len(targetEntries))
	for targetIndex := range targetEntries {
		targetReader := newOptionReader(targetEntries[targetIndex])
		remoteNameValue, remoteNameExists, remoteNameError := targetReader.stringValue(optionRemoteNameKeyConstant)
		if remoteNameError != nil {
			return nil, remoteNameError
		}
		sourceBranchValue, sourceExists, sourceError := targetReader.stringValue(optionSourceBranchKeyConstant)
		if sourceError != nil {
			return nil, sourceError
		}
		targetBranchValue, targetExists, targetError := targetReader.stringValue(optionTargetBranchKeyConstant)
		if targetError != nil {
			return nil, targetError
		}
		pushToRemoteValue, pushToRemoteExists, pushToRemoteError := targetReader.boolValue(optionPushToRemoteKeyConstant)
		if pushToRemoteError != nil {
			return nil, pushToRemoteError
		}
		deleteSourceBranchValue, deleteSourceBranchExists, deleteSourceBranchError := targetReader.boolValue(optionDeleteSourceBranchKeyConstant)
		if deleteSourceBranchError != nil {
			return nil, deleteSourceBranchError
		}

		targets = append(targets, BranchMigrationTarget{
			RemoteName:         defaultRemoteName(remoteNameExists, remoteNameValue),
			SourceBranch:       defaultSourceBranch(sourceExists, sourceBranchValue),
			TargetBranch:       defaultTargetBranch(targetExists, targetBranchValue),
			PushToRemote:       defaultPushToRemote(pushToRemoteExists, pushToRemoteValue),
			DeleteSourceBranch: defaultDeleteSourceBranch(deleteSourceBranchExists, deleteSourceBranchValue),
		})
	}

	return &BranchMigrationOperation{Targets: targets}, nil
}

func buildAuditReportOperation(options map[string]any) (Operation, error) {
	reader := newOptionReader(options)
	outputPath, outputExists, outputError := reader.stringValue(optionOutputPathKeyConstant)
	if outputError != nil {
		return nil, outputError
	}

	return &AuditReportOperation{OutputPath: strings.TrimSpace(outputPath), WriteToFile: outputExists && len(strings.TrimSpace(outputPath)) > 0}, nil
}

func parseProtocolValue(raw string) (shared.RemoteProtocol, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(shared.RemoteProtocolGit):
		return shared.RemoteProtocolGit, nil
	case string(shared.RemoteProtocolSSH):
		return shared.RemoteProtocolSSH, nil
	case string(shared.RemoteProtocolHTTPS):
		return shared.RemoteProtocolHTTPS, nil
	default:
		return "", fmt.Errorf("unsupported protocol value: %s", raw)
	}
}

func defaultRemoteName(explicit bool, value string) string {
	if explicit {
		trimmed := strings.TrimSpace(value)
		if len(trimmed) > 0 {
			return trimmed
		}
	}
	return defaultMigrationRemoteNameConstant
}

func defaultSourceBranch(explicit bool, value string) string {
	if explicit {
		trimmed := strings.TrimSpace(value)
		if len(trimmed) > 0 {
			return trimmed
		}
	}
	return ""
}

func defaultTargetBranch(explicit bool, value string) string {
	if explicit {
		trimmed := strings.TrimSpace(value)
		if len(trimmed) > 0 {
			return trimmed
		}
	}
	return defaultMigrationTargetBranchConstant
}

func defaultPushToRemote(explicit bool, value bool) bool {
	if explicit {
		return value
	}
	return true
}

func defaultDeleteSourceBranch(explicit bool, value bool) bool {
	if explicit {
		return value
	}
	return false
}
