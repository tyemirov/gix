package workflow

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
)

type gitStageOperation struct {
	paths       []string
	ensureClean bool
}

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
	variableSnapshot := snapshotVariables(environment)
	return iterateRepositories(state, func(repository *RepositoryState) error {
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
	})
}

type gitCommitOperation struct {
	messageTemplate string
	allowEmpty      bool
}

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
	variableSnapshot := snapshotVariables(environment)
	return iterateRepositories(state, func(repository *RepositoryState) error {
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
	})
}

type gitStageCommitOperation struct {
	paths           []string
	messageTemplate string
	allowEmpty      bool
	ensureClean     bool
}

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

	return &gitStageCommitOperation{
		paths:           pathValues,
		messageTemplate: message,
		allowEmpty:      allowEmpty,
		ensureClean:     ensureClean,
	}, nil
}

func (operation *gitStageCommitOperation) Name() string {
	return commandGitStageCommitKey
}

func (operation *gitStageCommitOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	variableSnapshot := snapshotVariables(environment)
	return iterateRepositories(state, func(repository *RepositoryState) error {
		templateData := buildTaskTemplateData(repository, TaskDefinition{Name: "Git Stage Commit"}, variableSnapshot)
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
	})
}

type gitPushOperation struct {
	branchTemplate string
	remoteName     string
}

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

	return &gitPushOperation{branchTemplate: branch, remoteName: remote}, nil
}

func (operation *gitPushOperation) Name() string {
	return commandGitPushKey
}

func (operation *gitPushOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	return iterateRepositories(state, func(repository *RepositoryState) error {
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
	})
}

type pullRequestCreateOperation struct {
	branchTemplate string
	titleTemplate  string
	bodyTemplate   string
	baseTemplate   string
	draft          bool
}

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

	return &pullRequestCreateOperation{
		branchTemplate: branch,
		titleTemplate:  title,
		bodyTemplate:   body,
		baseTemplate:   base,
		draft:          draft,
	}, nil
}

func (operation *pullRequestCreateOperation) Name() string {
	return commandPullRequestCreateKey
}

func (operation *pullRequestCreateOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	variableSnapshot := snapshotVariables(environment)
	return iterateRepositories(state, func(repository *RepositoryState) error {
		templateData := buildTaskTemplateData(repository, TaskDefinition{Name: "Pull Request"}, variableSnapshot)
		branchName, branchErr := renderTemplateValue(operation.branchTemplate, "", templateData)
		if branchErr != nil {
			return branchErr
		}
		branchName = sanitizeBranchName(branchName)
		if len(branchName) == 0 {
			return errors.New("pull request branch resolved to empty value")
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
	})
}

type pullRequestOpenOperation struct {
	branchTemplate string
	titleTemplate  string
	bodyTemplate   string
	baseTemplate   string
	remoteTemplate string
	draft          bool
}

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

	return &pullRequestOpenOperation{
		branchTemplate: branch,
		titleTemplate:  title,
		bodyTemplate:   body,
		baseTemplate:   base,
		remoteTemplate: remote,
		draft:          draft,
	}, nil
}

func (operation *pullRequestOpenOperation) Name() string {
	return commandPullRequestOpenKey
}

func (operation *pullRequestOpenOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	variableSnapshot := snapshotVariables(environment)
	return iterateRepositories(state, func(repository *RepositoryState) error {
		templateData := buildTaskTemplateData(repository, TaskDefinition{Name: "Pull Request"}, variableSnapshot)
		branchName, branchErr := renderTemplateValue(operation.branchTemplate, "", templateData)
		if branchErr != nil {
			return branchErr
		}
		if len(strings.TrimSpace(branchName)) == 0 {
			return errors.New("pull request branch resolved to empty value")
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
	})
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
