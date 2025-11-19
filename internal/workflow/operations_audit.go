package workflow

import (
	"context"
	"encoding/csv"
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
	auditCSVHeaderFinalRepositoryConstant = "final_github_repo"
	auditCSVHeaderFolderNameConstant      = "folder_name"
	auditCSVHeaderNameMatchesConstant     = "name_matches"
	auditCSVHeaderRemoteDefaultConstant   = "remote_default_branch"
	auditCSVHeaderLocalBranchConstant     = "local_branch"
	auditCSVHeaderInSyncConstant          = "in_sync"
	auditCSVHeaderRemoteProtocolConstant  = "remote_protocol"
	auditCSVHeaderOriginCanonicalConstant = "origin_matches_canonical"
	auditCSVHeaderWorktreeDirtyConstant   = "worktree_dirty"
	auditCSVHeaderDirtyFilesConstant      = "dirty_files"
)

// AuditReportOperation emits an audit CSV summarizing repository state.
type AuditReportOperation struct {
	OutputPath  string
	WriteToFile bool
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

	csvWriter := csv.NewWriter(writer)
	header := []string{
		auditCSVHeaderFolderNameConstant,
		auditCSVHeaderFinalRepositoryConstant,
		auditCSVHeaderNameMatchesConstant,
		auditCSVHeaderRemoteDefaultConstant,
		auditCSVHeaderLocalBranchConstant,
		auditCSVHeaderInSyncConstant,
		auditCSVHeaderRemoteProtocolConstant,
		auditCSVHeaderOriginCanonicalConstant,
		auditCSVHeaderWorktreeDirtyConstant,
		auditCSVHeaderDirtyFilesConstant,
	}

	if writeError := csvWriter.Write(header); writeError != nil {
		return writeError
	}

	for repositoryIndex := range state.Repositories {
		repository := state.Repositories[repositoryIndex]
		row := buildAuditReportRow(repository.Inspection)
		if writeError := csvWriter.Write(row); writeError != nil {
			return writeError
		}
	}

	csvWriter.Flush()
	if flushError := csvWriter.Error(); flushError != nil {
		return flushError
	}

	if operation.WriteToFile && environment.Output != nil {
		fmt.Fprintf(environment.Output, auditWriteMessageTemplateConstant, destination)
	}

	return nil
}

func buildAuditReportRow(inspection audit.RepositoryInspection) []string {
	finalRepository := strings.TrimSpace(inspection.CanonicalOwnerRepo)
	if len(finalRepository) == 0 {
		finalRepository = inspection.OriginOwnerRepo
	}

	nameMatches := audit.TernaryValueNotApplicable
	if inspection.IsGitRepository {
		nameMatches = audit.TernaryValueNo
		if len(inspection.DesiredFolderName) > 0 && inspection.DesiredFolderName == inspection.FolderName {
			nameMatches = audit.TernaryValueYes
		}
	}

	remoteDefaultBranch := inspection.RemoteDefaultBranch
	localBranch := inspection.LocalBranch
	inSync := inspection.InSyncStatus
	remoteProtocol := string(inspection.RemoteProtocol)
	originMatches := string(inspection.OriginMatchesCanonical)

	var worktreeDirty audit.TernaryValue
	dirtyFiles := ""

	if !inspection.IsGitRepository {
		finalRepository = string(audit.TernaryValueNotApplicable)
		remoteDefaultBranch = string(audit.TernaryValueNotApplicable)
		localBranch = string(audit.TernaryValueNotApplicable)
		inSync = audit.TernaryValueNotApplicable
		remoteProtocol = string(audit.TernaryValueNotApplicable)
		originMatches = string(audit.TernaryValueNotApplicable)
		worktreeDirty = audit.TernaryValueNotApplicable
	} else {
		if len(inspection.WorktreeDirtyFiles) > 0 {
			worktreeDirty = audit.TernaryValueYes
			dirtyFiles = strings.Join(inspection.WorktreeDirtyFiles, "; ")
		} else {
			worktreeDirty = audit.TernaryValueNo
		}
	}

	return []string{
		inspection.FolderName,
		finalRepository,
		string(nameMatches),
		remoteDefaultBranch,
		localBranch,
		string(inSync),
		remoteProtocol,
		originMatches,
		string(worktreeDirty),
		dirtyFiles,
	}
}
