package workflow

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	repoerrors "github.com/tyemirov/gix/internal/repos/errors"
)

const (
	commandRunCommandOptionKeyConstant          = "command"
	commandRunWorkingDirectoryOptionKeyConstant = "working_directory"
	commandRunMissingCommandMessageConstant     = "command run step requires command"
	commandRunEmptyCommandMessageConstant       = "command resolved to empty value"
	commandRunExecutorMissingMessageConstant    = "command run step requires shell command executor"
	commandRunTaskNameConstant                  = "Command Run"
	commandRunActionNameConstant                = "command.run"
	commandRunDescriptorConstant                = "command run"
)

type commandRunOperation struct {
	commandArguments         []string
	workingDirectoryTemplate string
	ensureClean              bool
	hardSafeguards           map[string]any
	softSafeguards           map[string]any
}

var _ RepositoryScopedOperation = (*commandRunOperation)(nil)

func buildCommandRunOperation(options map[string]any) (Operation, error) {
	reader := newOptionReader(options)
	rawCommand, commandExists := reader.entries[commandRunCommandOptionKeyConstant]
	if !commandExists {
		return nil, errors.New(commandRunMissingCommandMessageConstant)
	}

	commandArguments, commandError := parseShellCommandArguments(rawCommand, commandRunDescriptorConstant)
	if commandError != nil {
		return nil, commandError
	}
	if len(commandArguments) == 0 {
		return nil, errors.New(commandRunMissingCommandMessageConstant)
	}

	workingDirectory, _, workingDirectoryError := reader.stringValue(commandRunWorkingDirectoryOptionKeyConstant)
	if workingDirectoryError != nil {
		return nil, workingDirectoryError
	}

	ensureClean, _, ensureCleanError := reader.boolValue(optionTaskEnsureCleanKeyConstant)
	if ensureCleanError != nil {
		return nil, ensureCleanError
	}

	safeguards, _, safeguardsError := reader.mapValue(optionTaskSafeguardsKeyConstant)
	if safeguardsError != nil {
		return nil, safeguardsError
	}
	hardSafeguards, softSafeguards := splitSafeguardSets(safeguards, safeguardDefaultSoftSkip)

	return &commandRunOperation{
		commandArguments:         commandArguments,
		workingDirectoryTemplate: workingDirectory,
		ensureClean:              ensureClean,
		hardSafeguards:           hardSafeguards,
		softSafeguards:           softSafeguards,
	}, nil
}

func (operation *commandRunOperation) Name() string {
	return commandRunKey
}

func (operation *commandRunOperation) Execute(ctx context.Context, environment *Environment, state *State) error {
	return iterateRepositories(state, func(repository *RepositoryState) error {
		return operation.ExecuteForRepository(ctx, environment, repository)
	})
}

func (operation *commandRunOperation) ExecuteForRepository(ctx context.Context, environment *Environment, repository *RepositoryState) error {
	if environment == nil || repository == nil {
		return nil
	}
	if skip, guardErr := evaluateOperationSafeguards(ctx, environment, repository, commandRunTaskNameConstant, operation.hardSafeguards, operation.softSafeguards); guardErr != nil {
		return guardErr
	} else if skip {
		return nil
	}

	commandExecutor, ok := environment.GitExecutor.(shellCommandExecutor)
	if !ok || commandExecutor == nil {
		return repoerrors.WrapMessage(
			repoerrors.OperationCommandRun,
			repository.Path,
			repoerrors.ErrExecutorDependenciesMissing,
			commandRunExecutorMissingMessageConstant,
		)
	}

	variableSnapshot := snapshotVariables(environment)
	templateData := buildTaskTemplateData(repository, TaskDefinition{Name: commandRunTaskNameConstant}, variableSnapshot)

	renderedArguments, renderError := renderCommandArguments(operation.commandArguments, templateData)
	if renderError != nil {
		return renderError
	}
	if len(renderedArguments) == 0 || strings.TrimSpace(renderedArguments[0]) == "" {
		return errors.New(commandRunEmptyCommandMessageConstant)
	}

	workingDirectoryTemplate, workingDirectoryError := renderTemplateValue(operation.workingDirectoryTemplate, "", templateData)
	if workingDirectoryError != nil {
		return workingDirectoryError
	}
	resolvedWorkingDirectory := resolveCommandWorkingDirectory(repository.Path, workingDirectoryTemplate)

	plan := taskPlan{
		task: TaskDefinition{
			Name:        commandRunTaskNameConstant,
			EnsureClean: operation.ensureClean,
		},
		repository: repository,
		workflowSteps: []workflowAction{
			commandRunAction{
				executor:         commandExecutor,
				commandName:      execshell.CommandName(renderedArguments[0]),
				arguments:        renderedArguments[1:],
				workingDirectory: resolvedWorkingDirectory,
			},
		},
		variables: variableSnapshot,
	}

	executionError := newTaskExecutor(environment, repository, plan).Execute(ctx)
	if executionError == nil {
		return nil
	}
	if errors.Is(executionError, errRepositorySkipped) {
		return executionError
	}

	return repoerrors.Wrap(repoerrors.OperationCommandRun, repository.Path, repoerrors.ErrCommandExecutionFailed, executionError)
}

func (operation *commandRunOperation) IsRepositoryScoped() bool {
	return true
}

type commandRunAction struct {
	executor         shellCommandExecutor
	commandName      execshell.CommandName
	arguments        []string
	workingDirectory string
}

func (action commandRunAction) Name() string {
	return commandRunActionNameConstant
}

func (action commandRunAction) Guards() []actionGuard {
	return []actionGuard{newCleanWorktreeGuard()}
}

func (action commandRunAction) Execute(ctx context.Context, execCtx *ExecutionContext) error {
	if action.executor == nil {
		return errors.New(commandRunExecutorMissingMessageConstant)
	}
	if strings.TrimSpace(string(action.commandName)) == "" {
		return errors.New(commandRunEmptyCommandMessageConstant)
	}

	command := execshell.ShellCommand{
		Name: action.commandName,
		Details: execshell.CommandDetails{
			Arguments:        action.arguments,
			WorkingDirectory: action.workingDirectory,
		},
	}
	if _, executionError := action.executor.Execute(ctx, command); executionError != nil {
		return executionError
	}
	if execCtx != nil {
		execCtx.recordCustomAction()
	}
	return nil
}

func renderCommandArguments(arguments []string, templateData TaskTemplateData) ([]string, error) {
	if len(arguments) == 0 {
		return nil, nil
	}
	rendered := make([]string, 0, len(arguments))
	for argumentIndex := range arguments {
		rawArgument := arguments[argumentIndex]
		renderedArgument, renderError := renderTemplateValue(rawArgument, "", templateData)
		if renderError != nil {
			return nil, renderError
		}
		trimmedArgument := strings.TrimSpace(renderedArgument)
		if argumentIndex == 0 {
			rendered = append(rendered, trimmedArgument)
			continue
		}
		if trimmedArgument == "" {
			continue
		}
		rendered = append(rendered, trimmedArgument)
	}
	return rendered, nil
}

func resolveCommandWorkingDirectory(repositoryPath string, workingDirectory string) string {
	trimmedDirectory := strings.TrimSpace(workingDirectory)
	if trimmedDirectory == "" {
		return strings.TrimSpace(repositoryPath)
	}
	if filepath.IsAbs(trimmedDirectory) || strings.TrimSpace(repositoryPath) == "" {
		return trimmedDirectory
	}
	return filepath.Join(repositoryPath, trimmedDirectory)
}
