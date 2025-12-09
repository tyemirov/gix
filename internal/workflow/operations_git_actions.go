package workflow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tyemirov/gix/internal/repos/shared"
)

type gitStageOperation struct {
	paths       []string
	ensureClean bool
}

var _ RepositoryScopedOperation = (*gitStageOperation)(nil)

func buildGitStageOperation(options map[string]any) (Operation, error) {
	reader := newOptionReader(options)
	pathValues, pathExists, pathErr := reader.stringSlice(optionPathsKeyConstant)
	if pathErr != nil {
		return nil, pathErr
	}
	if !pathExists || len(pathValues) == 0 {
		return nil, errors.New("git stage step requires at least one path")
	}

	ensureClean, _, ensureCleanErr := reader.boolValue(optionTaskEnsureCleanKeyConstant)
	if ensureCleanErr != nil {
		return nil, ensureCleanErr
	}

	return &gitStageOperation{paths: pathValues, ensureClean: ensureClean}, nil
}

func (operation *gitStageOperation) Name() string {
	return commandGitStageKey
}

func (operation *gitStageOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	return iterateRepositories(state, func(repository *RepositoryState) error {
		return operation.ExecuteForRepository(ctx, environment, repository)
	})
}

func (operation *gitStageOperation) ExecuteForRepository(ctx context.Context, environment *Environment, repository *RepositoryState) error {
	variableSnapshot := snapshotVariables(environment)
	templateData := buildTaskTemplateData(repository, TaskDefinition{Name: "Git Stage"}, variableSnapshot)
	changes := make([]taskFileChange, 0, len(operation.paths))
	for _, rawPath := range operation.paths {
		rendered, renderErr := renderTemplateValue(rawPath, "", templateData)
		if renderErr != nil {
			return renderErr
		}
		if len(strings.TrimSpace(rendered)) == 0 {
			continue
		}
		cleanPath := strings.TrimSpace(rendered)
		changes = append(changes, taskFileChange{
			relativePath: cleanPath,
			absolutePath: filepath.Join(repository.Path, cleanPath),
			apply:        true,
		})
	}

	if len(changes) == 0 {
		return nil
	}

	plan := taskPlan{
		task: TaskDefinition{
			Name:        "Git Stage",
			EnsureClean: operation.ensureClean,
		},
		repository:    repository,
		fileChanges:   changes,
		workflowSteps: []workflowAction{gitStageAction{changes: changes}},
		variables:     variableSnapshot,
	}
	return newTaskExecutor(environment, repository, plan).Execute(ctx)
}

func (operation *gitStageOperation) IsRepositoryScoped() bool {
	return true
}

type gitCommitOperation struct {
	messageTemplate string
	allowEmpty      bool
}

var _ RepositoryScopedOperation = (*gitCommitOperation)(nil)

func buildGitCommitOperation(options map[string]any) (Operation, error) {
	reader := newOptionReader(options)
	message, messageExists, messageErr := reader.stringValue(optionTaskCommitMessageKeyConstant)
	if messageErr != nil {
		return nil, messageErr
	}
	if !messageExists || len(message) == 0 {
		return nil, errors.New("git commit step requires commit_message")
	}

	allowEmpty, _, allowEmptyErr := reader.boolValue("allow_empty")
	if allowEmptyErr != nil {
		return nil, allowEmptyErr
	}

	return &gitCommitOperation{
		messageTemplate: message,
		allowEmpty:      allowEmpty,
	}, nil
}

func (operation *gitCommitOperation) Name() string {
	return commandGitCommitKey
}

func (operation *gitCommitOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	return iterateRepositories(state, func(repository *RepositoryState) error {
		return operation.ExecuteForRepository(ctx, environment, repository)
	})
}

func (operation *gitCommitOperation) ExecuteForRepository(ctx context.Context, environment *Environment, repository *RepositoryState) error {
	variableSnapshot := snapshotVariables(environment)
	templateData := buildTaskTemplateData(repository, TaskDefinition{Name: "Git Commit"}, variableSnapshot)
	message, messageErr := renderTemplateValue(operation.messageTemplate, "", templateData)
	if messageErr != nil {
		return messageErr
	}
	if len(strings.TrimSpace(message)) == 0 {
		return errors.New("commit message resolved to empty value")
	}

	plan := taskPlan{
		task:          TaskDefinition{Name: "Git Commit"},
		repository:    repository,
		commitMessage: message,
		workflowSteps: []workflowAction{gitCommitAction{message: message, allowEmpty: operation.allowEmpty}},
		variables:     variableSnapshot,
	}
	return newTaskExecutor(environment, repository, plan).Execute(ctx)
}

func (operation *gitCommitOperation) IsRepositoryScoped() bool {
	return true
}

type gitStageCommitOperation struct {
	paths           []string
	messageTemplate string
	allowEmpty      bool
	ensureClean     bool
	hardSafeguards  map[string]any
	softSafeguards  map[string]any
}

var _ RepositoryScopedOperation = (*gitStageCommitOperation)(nil)

func buildGitStageCommitOperation(options map[string]any) (Operation, error) {
	reader := newOptionReader(options)
	pathValues, pathExists, pathErr := reader.stringSlice(optionPathsKeyConstant)
	if pathErr != nil {
		return nil, pathErr
	}
	if !pathExists || len(pathValues) == 0 {
		return nil, errors.New("git stage-commit step requires at least one path")
	}

	message, messageExists, messageErr := reader.stringValue(optionTaskCommitMessageKeyConstant)
	if messageErr != nil {
		return nil, messageErr
	}
	if !messageExists || len(message) == 0 {
		return nil, errors.New("git stage-commit step requires commit_message")
	}

	allowEmpty, _, allowEmptyErr := reader.boolValue("allow_empty")
	if allowEmptyErr != nil {
		return nil, allowEmptyErr
	}

	ensureClean, _, ensureCleanErr := reader.boolValue(optionTaskEnsureCleanKeyConstant)
	if ensureCleanErr != nil {
		return nil, ensureCleanErr
	}

	safeguards, _, safeguardsErr := reader.mapValue(optionTaskSafeguardsKeyConstant)
	if safeguardsErr != nil {
		return nil, safeguardsErr
	}
	hardSafeguards, softSafeguards := splitSafeguardSets(safeguards, safeguardDefaultSoftSkip)

	return &gitStageCommitOperation{
		paths:           pathValues,
		messageTemplate: message,
		allowEmpty:      allowEmpty,
		ensureClean:     ensureClean,
		hardSafeguards:  hardSafeguards,
		softSafeguards:  softSafeguards,
	}, nil
}

func (operation *gitStageCommitOperation) Name() string {
	return commandGitStageCommitKey
}

func (operation *gitStageCommitOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	return iterateRepositories(state, func(repository *RepositoryState) error {
		return operation.ExecuteForRepository(ctx, environment, repository)
	})
}

func (operation *gitStageCommitOperation) ExecuteForRepository(ctx context.Context, environment *Environment, repository *RepositoryState) error {
	variableSnapshot := snapshotVariables(environment)
	templateData := buildTaskTemplateData(repository, TaskDefinition{Name: "Git Stage Commit"}, variableSnapshot)
	if skip, guardErr := evaluateOperationSafeguards(ctx, environment, repository, "Git Stage Commit", operation.hardSafeguards, operation.softSafeguards); guardErr != nil {
		return guardErr
	} else if skip {
		return nil
	}
	renderedPaths := make([]string, 0, len(operation.paths))
	for _, rawPath := range operation.paths {
		rendered, renderErr := renderTemplateValue(rawPath, "", templateData)
		if renderErr != nil {
			return renderErr
		}
		if len(strings.TrimSpace(rendered)) == 0 {
			continue
		}
		cleanPath := strings.TrimSpace(rendered)
		renderedPaths = append(renderedPaths, cleanPath)
	}

	stageTargets := renderedPaths
	mutatedPaths := environment.ConsumeMutatedFiles(repository)
	if len(mutatedPaths) > 0 {
		selected := filterMutatedStageTargets(mutatedPaths, renderedPaths)
		if len(selected) == 0 {
			stageTargets = nil
		} else {
			stageTargets = selected
		}
	}

	if len(stageTargets) == 0 {
		return nil
	}

	changes := make([]taskFileChange, 0, len(stageTargets))
	for _, path := range stageTargets {
		changes = append(changes, taskFileChange{
			relativePath: path,
			absolutePath: filepath.Join(repository.Path, path),
			apply:        true,
		})
	}

	message, messageErr := renderTemplateValue(operation.messageTemplate, "", templateData)
	if messageErr != nil {
		return messageErr
	}
	if len(strings.TrimSpace(message)) == 0 {
		return errors.New("commit message resolved to empty value")
	}

	plan := taskPlan{
		task: TaskDefinition{
			Name:        "Git Stage Commit",
			EnsureClean: operation.ensureClean,
		},
		repository:    repository,
		fileChanges:   changes,
		commitMessage: message,
		workflowSteps: []workflowAction{
			gitStageCommitAction{
				changes:    changes,
				message:    message,
				allowEmpty: operation.allowEmpty,
			},
		},
		variables: variableSnapshot,
	}

	return newTaskExecutor(environment, repository, plan).Execute(ctx)
}

func (operation *gitStageCommitOperation) IsRepositoryScoped() bool {
	return true
}

func filterMutatedStageTargets(mutated []string, requested []string) []string {
	if len(mutated) == 0 {
		return nil
	}
	if len(requested) == 0 {
		return append([]string(nil), mutated...)
	}
	selected := make([]string, 0, len(mutated))
	for _, path := range mutated {
		if stagePatternMatchesAny(requested, path) {
			selected = append(selected, path)
		}
	}
	return selected
}

func stagePatternMatchesAny(patterns []string, path string) bool {
	for _, pattern := range patterns {
		if stagePatternMatches(pattern, path) {
			return true
		}
	}
	return false
}

func stagePatternMatches(pattern string, path string) bool {
	trimmedPattern := strings.TrimSpace(pattern)
	if len(trimmedPattern) == 0 {
		return false
	}
	normalizedPattern := filepath.ToSlash(strings.TrimPrefix(trimmedPattern, "./"))
	if normalizedPattern == "" || normalizedPattern == "." {
		return true
	}
	normalizedPath := filepath.ToSlash(strings.TrimPrefix(path, "./"))
	if normalizedPath == normalizedPattern {
		return true
	}
	if strings.HasSuffix(normalizedPattern, "/") {
		normalizedPattern = strings.TrimSuffix(normalizedPattern, "/")
		if len(normalizedPattern) == 0 {
			return true
		}
		if normalizedPath == normalizedPattern || strings.HasPrefix(normalizedPath, normalizedPattern+"/") {
			return true
		}
	}
	if strings.ContainsAny(normalizedPattern, "*?[") {
		matched, err := filepath.Match(normalizedPattern, normalizedPath)
		if err == nil && matched {
			return true
		}
	}
	return false
}

type gitPushOperation struct {
	branchTemplate string
	remoteName     string
	hardSafeguards map[string]any
	softSafeguards map[string]any
}

var _ RepositoryScopedOperation = (*gitPushOperation)(nil)

func buildGitPushOperation(options map[string]any) (Operation, error) {
	reader := newOptionReader(options)
	branch, branchExists, branchErr := reader.stringValue("branch")
	if branchErr != nil {
		return nil, branchErr
	}
	if !branchExists || len(branch) == 0 {
		return nil, errors.New("git push step requires branch")
	}

	remote, remoteExists, remoteErr := reader.stringValue(optionTaskBranchPushRemoteKeyConstant)
	if remoteErr != nil {
		return nil, remoteErr
	}
	if !remoteExists || len(remote) == 0 {
		remote = defaultTaskPushRemote
	}

	safeguards, _, safeguardsErr := reader.mapValue(optionTaskSafeguardsKeyConstant)
	if safeguardsErr != nil {
		return nil, safeguardsErr
	}
	hardSafeguards, softSafeguards := splitSafeguardSets(safeguards, safeguardDefaultSoftSkip)

	return &gitPushOperation{
		branchTemplate: branch,
		remoteName:     remote,
		hardSafeguards: hardSafeguards,
		softSafeguards: softSafeguards,
	}, nil
}

func (operation *gitPushOperation) Name() string {
	return commandGitPushKey
}

func (operation *gitPushOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	return iterateRepositories(state, func(repository *RepositoryState) error {
		return operation.ExecuteForRepository(ctx, environment, repository)
	})
}

func (operation *gitPushOperation) ExecuteForRepository(ctx context.Context, environment *Environment, repository *RepositoryState) error {
	if repository == nil {
		return nil
	}
	if skip, guardErr := evaluateOperationSafeguards(ctx, environment, repository, "Git Push", operation.hardSafeguards, operation.softSafeguards); guardErr != nil {
		return guardErr
	} else if skip {
		return nil
	}
	var variableSnapshot map[string]string
	if environment != nil && environment.Variables != nil {
		variableSnapshot = environment.Variables.Snapshot()
	}
	templateData := buildTaskTemplateData(repository, TaskDefinition{Name: "Git Push"}, variableSnapshot)
	branchName, branchErr := renderTemplateValue(operation.branchTemplate, "", templateData)
	if branchErr != nil {
		return branchErr
	}
	if len(strings.TrimSpace(branchName)) == 0 {
		return errors.New("branch resolved to empty value")
	}

	remoteName, remoteErr := renderTemplateValue(operation.remoteName, defaultTaskPushRemote, templateData)
	if remoteErr != nil {
		return remoteErr
	}

	plan := taskPlan{
		task: TaskDefinition{
			Name: "Git Push",
			Branch: TaskBranchDefinition{
				PushRemote: strings.TrimSpace(remoteName),
			},
		},
		repository:    repository,
		branchName:    strings.TrimSpace(branchName),
		workflowSteps: []workflowAction{gitPushAction{branch: strings.TrimSpace(branchName), remote: strings.TrimSpace(remoteName)}},
		variables:     variableSnapshot,
	}
	return newTaskExecutor(environment, repository, plan).Execute(ctx)
}

func (operation *gitPushOperation) IsRepositoryScoped() bool {
	return true
}

type pullRequestCreateOperation struct {
	branchTemplate string
	titleTemplate  string
	bodyTemplate   string
	baseTemplate   string
	draft          bool
	hardSafeguards map[string]any
	softSafeguards map[string]any
}

var _ RepositoryScopedOperation = (*pullRequestCreateOperation)(nil)

func buildPullRequestCreateOperation(options map[string]any) (Operation, error) {
	reader := newOptionReader(options)
	branch, branchExists, branchErr := reader.stringValue("branch")
	if branchErr != nil {
		return nil, branchErr
	}
	if !branchExists || len(branch) == 0 {
		return nil, errors.New("pull-request create step requires branch")
	}

	title, titleExists, titleErr := reader.stringValue(optionTaskPRTitleKeyConstant)
	if titleErr != nil {
		return nil, titleErr
	}
	if !titleExists || len(title) == 0 {
		return nil, errors.New("pull-request create step requires title")
	}

	body, bodyExists, bodyErr := reader.stringValue(optionTaskPRBodyKeyConstant)
	if bodyErr != nil {
		return nil, bodyErr
	}
	if !bodyExists || len(body) == 0 {
		return nil, errors.New("pull-request create step requires body")
	}

	base, baseExists, baseErr := reader.stringValue(optionTaskPRBaseKeyConstant)
	if baseErr != nil {
		return nil, baseErr
	}
	if !baseExists || len(base) == 0 {
		return nil, errors.New("pull-request create step requires base")
	}

	draft, _, draftErr := reader.boolValue(optionTaskPRDraftKeyConstant)
	if draftErr != nil {
		return nil, draftErr
	}

	safeguards, _, safeguardsErr := reader.mapValue(optionTaskSafeguardsKeyConstant)
	if safeguardsErr != nil {
		return nil, safeguardsErr
	}
	hardSafeguards, softSafeguards := splitSafeguardSets(safeguards, safeguardDefaultSoftSkip)

	return &pullRequestCreateOperation{
		branchTemplate: branch,
		titleTemplate:  title,
		bodyTemplate:   body,
		baseTemplate:   base,
		draft:          draft,
		hardSafeguards: hardSafeguards,
		softSafeguards: softSafeguards,
	}, nil
}

func (operation *pullRequestCreateOperation) Name() string {
	return commandPullRequestCreateKey
}

func (operation *pullRequestCreateOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	return iterateRepositories(state, func(repository *RepositoryState) error {
		return operation.ExecuteForRepository(ctx, environment, repository)
	})
}

func (operation *pullRequestCreateOperation) ExecuteForRepository(ctx context.Context, environment *Environment, repository *RepositoryState) error {
	if repository == nil {
		return nil
	}
	if skip, guardErr := evaluateOperationSafeguards(ctx, environment, repository, "Create Pull Request", operation.hardSafeguards, operation.softSafeguards); guardErr != nil {
		return guardErr
	} else if skip {
		return nil
	}
	variableSnapshot := snapshotVariables(environment)
	templateData := buildTaskTemplateData(repository, TaskDefinition{Name: "Pull Request"}, variableSnapshot)
	branchName, branchErr := renderTemplateValue(operation.branchTemplate, "", templateData)
	if branchErr != nil {
		return branchErr
	}
	if len(strings.TrimSpace(branchName)) == 0 {
		return nil
	}
	branchName = sanitizeBranchName(branchName)
	if len(branchName) == 0 {
		return nil
	}

	title, titleErr := renderTemplateValue(operation.titleTemplate, "", templateData)
	if titleErr != nil {
		return titleErr
	}
	body, bodyErr := renderTemplateValue(operation.bodyTemplate, "", templateData)
	if bodyErr != nil {
		return bodyErr
	}
	base, baseErr := renderTemplateValue(operation.baseTemplate, "", templateData)
	if baseErr != nil {
		return baseErr
	}

	pullRequest := &taskPlanPullRequest{
		title: title,
		body:  body,
		base:  base,
		draft: operation.draft,
	}

	plan := taskPlan{
		task:          TaskDefinition{Name: "Create Pull Request"},
		repository:    repository,
		branchName:    strings.TrimSpace(branchName),
		pullRequest:   pullRequest,
		workflowSteps: []workflowAction{pullRequestAction{title: title, body: body, base: base}},
		variables:     variableSnapshot,
	}
	return newTaskExecutor(environment, repository, plan).Execute(ctx)
}

func (operation *pullRequestCreateOperation) IsRepositoryScoped() bool {
	return true
}

type pullRequestOpenOperation struct {
	branchTemplate string
	titleTemplate  string
	bodyTemplate   string
	baseTemplate   string
	remoteTemplate string
	draft          bool
	hardSafeguards map[string]any
	softSafeguards map[string]any
}

var _ RepositoryScopedOperation = (*pullRequestOpenOperation)(nil)

func buildPullRequestOpenOperation(options map[string]any) (Operation, error) {
	reader := newOptionReader(options)
	branch, branchExists, branchErr := reader.stringValue("branch")
	if branchErr != nil {
		return nil, branchErr
	}
	if !branchExists || len(branch) == 0 {
		return nil, errors.New("pull-request open step requires branch")
	}

	title, titleExists, titleErr := reader.stringValue(optionTaskPRTitleKeyConstant)
	if titleErr != nil {
		return nil, titleErr
	}
	if !titleExists || len(title) == 0 {
		return nil, errors.New("pull-request open step requires title")
	}

	body, bodyExists, bodyErr := reader.stringValue(optionTaskPRBodyKeyConstant)
	if bodyErr != nil {
		return nil, bodyErr
	}
	if !bodyExists || len(body) == 0 {
		return nil, errors.New("pull-request open step requires body")
	}

	base, baseExists, baseErr := reader.stringValue(optionTaskPRBaseKeyConstant)
	if baseErr != nil {
		return nil, baseErr
	}
	if !baseExists || len(base) == 0 {
		return nil, errors.New("pull-request open step requires base")
	}

	remote, remoteExists, remoteErr := reader.stringValue(optionTaskBranchPushRemoteKeyConstant)
	if remoteErr != nil {
		return nil, remoteErr
	}
	if !remoteExists || len(remote) == 0 {
		remote = defaultTaskPushRemote
	}

	draft, _, draftErr := reader.boolValue(optionTaskPRDraftKeyConstant)
	if draftErr != nil {
		return nil, draftErr
	}

	safeguards, _, safeguardsErr := reader.mapValue(optionTaskSafeguardsKeyConstant)
	if safeguardsErr != nil {
		return nil, safeguardsErr
	}
	hardSafeguards, softSafeguards := splitSafeguardSets(safeguards, safeguardDefaultSoftSkip)

	return &pullRequestOpenOperation{
		branchTemplate: branch,
		titleTemplate:  title,
		bodyTemplate:   body,
		baseTemplate:   base,
		remoteTemplate: remote,
		draft:          draft,
		hardSafeguards: hardSafeguards,
		softSafeguards: softSafeguards,
	}, nil
}

func (operation *pullRequestOpenOperation) Name() string {
	return commandPullRequestOpenKey
}

func (operation *pullRequestOpenOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	return iterateRepositories(state, func(repository *RepositoryState) error {
		return operation.ExecuteForRepository(ctx, environment, repository)
	})
}

func (operation *pullRequestOpenOperation) ExecuteForRepository(ctx context.Context, environment *Environment, repository *RepositoryState) error {
	if skip, guardErr := evaluateOperationSafeguards(ctx, environment, repository, "Open Pull Request", operation.hardSafeguards, operation.softSafeguards); guardErr != nil {
		return guardErr
	} else if skip {
		return nil
	}
	variableSnapshot := snapshotVariables(environment)
	templateData := buildTaskTemplateData(repository, TaskDefinition{Name: "Pull Request"}, variableSnapshot)
	branchName, branchErr := renderTemplateValue(operation.branchTemplate, "", templateData)
	if branchErr != nil {
		return branchErr
	}
	if len(strings.TrimSpace(branchName)) == 0 {
		return nil
	}

	title, titleErr := renderTemplateValue(operation.titleTemplate, "", templateData)
	if titleErr != nil {
		return titleErr
	}
	body, bodyErr := renderTemplateValue(operation.bodyTemplate, "", templateData)
	if bodyErr != nil {
		return bodyErr
	}
	base, baseErr := renderTemplateValue(operation.baseTemplate, repository.Inspection.RemoteDefaultBranch, templateData)
	if baseErr != nil {
		return baseErr
	}
	remoteName, remoteErr := renderTemplateValue(operation.remoteTemplate, defaultTaskPushRemote, templateData)
	if remoteErr != nil {
		return remoteErr
	}

	pullRequest := &taskPlanPullRequest{
		title: title,
		body:  body,
		base:  base,
		draft: operation.draft,
	}

	plan := taskPlan{
		task: TaskDefinition{
			Name: "Open Pull Request",
			Branch: TaskBranchDefinition{
				PushRemote: strings.TrimSpace(remoteName),
			},
		},
		repository:  repository,
		branchName:  strings.TrimSpace(branchName),
		pullRequest: pullRequest,
		workflowSteps: []workflowAction{
			pullRequestOpenAction{
				branch: strings.TrimSpace(branchName),
				remote: strings.TrimSpace(remoteName),
				title:  title,
				body:   body,
				base:   base,
				draft:  operation.draft,
			},
		},
		variables: variableSnapshot,
	}
	return newTaskExecutor(environment, repository, plan).Execute(ctx)
}

func (operation *pullRequestOpenOperation) IsRepositoryScoped() bool {
	return true
}

func evaluateOperationSafeguards(ctx context.Context, environment *Environment, repository *RepositoryState, operationName string, hard, soft map[string]any) (bool, error) {
	if len(hard) > 0 {
		pass, reason, evalErr := EvaluateSafeguards(ctx, environment, repository, hard)
		if evalErr != nil {
			return false, evalErr
		}
		if !pass {
			logOperationSafeguard(environment, repository, operationName, reason, shared.EventLevelError)
			return true, repositorySkipError{reason: reason}
		}
	}

	if len(soft) > 0 {
		pass, reason, evalErr := EvaluateSafeguards(ctx, environment, repository, soft)
		if evalErr != nil {
			return false, evalErr
		}
		if !pass {
			logOperationSafeguard(environment, repository, operationName, reason, shared.EventLevelWarn)
			return true, nil
		}
	}

	return false, nil
}

func logOperationSafeguard(environment *Environment, repository *RepositoryState, operationName string, reason string, level shared.EventLevel) {
	if environment == nil {
		return
	}
	message := strings.TrimSpace(reason)
	if len(message) == 0 {
		message = "safeguard check failed"
	}
	environment.ReportRepositoryEvent(
		repository,
		level,
		shared.EventCodeTaskSkip,
		fmt.Sprintf("%s: %s", operationName, message),
		map[string]string{
			"operation": operationName,
			"reason":    sanitizeOperationReason(message),
		},
	)
}

func sanitizeOperationReason(reason string) string {
	replaced := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		switch r {
		case '-', '_', '.':
			return r
		default:
			return '_'
		}
	}, strings.TrimSpace(reason))
	replaced = strings.Trim(replaced, "_")
	if replaced == "" {
		return "safeguard"
	}
	return replaced
}

func snapshotVariables(environment *Environment) map[string]string {
	if environment == nil || environment.Variables == nil {
		return nil
	}
	return environment.Variables.Snapshot()
}

func iterateRepositories(state *State, handler func(*RepositoryState) error) error {
	if state == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(state.Repositories))
	var errs []error
	for _, repository := range state.Repositories {
		if repository == nil {
			continue
		}
		pathKey := strings.TrimSpace(repository.Path)
		if len(pathKey) > 0 {
			if _, exists := seen[pathKey]; exists {
				continue
			}
			seen[pathKey] = struct{}{}
		}
		if err := handler(repository); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
