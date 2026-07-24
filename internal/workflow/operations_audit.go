package workflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tyemirov/gix/internal/audit"
)

const (
	auditWriteMessageTemplateConstant     = "WORKFLOW-AUDIT: wrote report to %s\n"
	auditReportDestinationStdoutConstant  = "stdout"
	auditCurrentDirectorySentinelConstant = "."
	auditDirectoryPermissionsConstant     = 0o755
)

// AuditReportOperation emits an audit report summarizing repository state.
type AuditReportOperation struct {
	OutputPath  string
	WriteToFile bool
	Format      audit.ReportFormat
}

// Name identifies the workflow command handled by this operation.
func (operation *AuditReportOperation) Name() string {
	return commandAuditReportKey
}

// Execute writes the audit report using the current repository state.
func (operation *AuditReportOperation) Execute(executionContext context.Context, environment *Environment, state *State) (executionError error) {
	if environment == nil || state == nil {
		return nil
	}

	destination := auditReportDestinationStdoutConstant
	sanitizedOutputPath := strings.TrimSpace(operation.OutputPath)
	if operation.WriteToFile {
		destination = sanitizedOutputPath
	}

	var writer io.Writer
	var closeFunction func() error
	if operation.WriteToFile {
		sanitizedOutputDirectory := filepath.Dir(sanitizedOutputPath)
		if sanitizedOutputDirectory != auditCurrentDirectorySentinelConstant {
			if directoryCreationError := os.MkdirAll(sanitizedOutputDirectory, auditDirectoryPermissionsConstant); directoryCreationError != nil {
				return directoryCreationError
			}
		}

		fileHandle, createError := os.Create(sanitizedOutputPath)
		if createError != nil {
			return createError
		}
		writer = fileHandle
		closeFunction = fileHandle.Close
	} else {
		if environment.Output != nil {
			writer = environment.Output
		} else {
			writer = io.Discard
		}
	}

	if closeFunction != nil {
		defer func() {
			closeError := closeFunction()
			if closeError != nil && executionError == nil {
				executionError = closeError
			}
		}()
	}

	inspections := make([]audit.RepositoryInspection, 0, len(state.Repositories))
	for repositoryIndex := range state.Repositories {
		repository := state.Repositories[repositoryIndex]
		inspections = append(inspections, repository.Inspection)
	}
	if writeError := audit.WriteReport(writer, operation.Format, inspections); writeError != nil {
		return writeError
	}

	if operation.WriteToFile && environment.Output != nil {
		fmt.Fprintf(environment.Output, auditWriteMessageTemplateConstant, destination)
	}

	return nil
}
