package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	flagutils "github.com/tyemirov/gix/internal/utils/flags"
)

var errVersionHandled = errors.New("version handled")

// ExecutionOptions describes a single in-process CLI execution.
type ExecutionOptions struct {
	Arguments      []string
	Context        context.Context
	StandardInput  io.Reader
	StandardOutput io.Writer
	StandardError  io.Writer
	ExitOnVersion  bool
}

// ExecuteWithOptions runs the configured Cobra command hierarchy with explicit arguments and I/O streams.
func (application *Application) ExecuteWithOptions(options ExecutionOptions) error {
	executionContext := options.Context
	if executionContext == nil {
		executionContext = context.Background()
	}

	normalizedArguments := flagutils.NormalizeToggleArguments(options.Arguments)
	normalizedArguments = normalizeInitializationScopeArguments(normalizedArguments)
	normalizedArguments = normalizeWebArguments(normalizedArguments)

	application.rootCommand.SetContext(executionContext)
	application.rootCommand.SetArgs(normalizedArguments)
	if options.StandardInput != nil {
		application.rootCommand.SetIn(options.StandardInput)
	}
	if options.StandardOutput != nil {
		application.rootCommand.SetOut(options.StandardOutput)
	}
	if options.StandardError != nil {
		application.rootCommand.SetErr(options.StandardError)
	}
	application.versionExitEnabled = options.ExitOnVersion

	executionError := application.rootCommand.Execute()
	if errors.Is(executionError, errVersionHandled) {
		executionError = nil
	}
	if syncError := application.flushLogger(); syncError != nil {
		return fmt.Errorf(loggerSyncErrorTemplateConstant, syncError)
	}
	return executionError
}
