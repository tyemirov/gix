package audit

import (
	"fmt"

	"github.com/tyemirov/gix/v5/internal/repos/shared"
)

// RemoteProtocolType enumerates supported git remote protocols.
type RemoteProtocolType = shared.RemoteProtocol

// Remote protocol values supported by the audit command.
const (
	RemoteProtocolGit   RemoteProtocolType = shared.RemoteProtocolGit
	RemoteProtocolSSH   RemoteProtocolType = shared.RemoteProtocolSSH
	RemoteProtocolHTTPS RemoteProtocolType = shared.RemoteProtocolHTTPS
	RemoteProtocolOther RemoteProtocolType = shared.RemoteProtocolOther
)

// OriginRemoteStatus describes whether the origin remote is configured.
type OriginRemoteStatus string

// Supported origin-remote status values.
const (
	OriginRemoteStatusConfigured    OriginRemoteStatus = "configured"
	OriginRemoteStatusMissing       OriginRemoteStatus = "missing"
	OriginRemoteStatusNotApplicable OriginRemoteStatus = "n/a"
)

// TernaryValue represents yes/no/not-applicable values used in reports.
type TernaryValue string

// Supported ternary values.
const (
	TernaryValueYes           TernaryValue = "yes"
	TernaryValueNo            TernaryValue = "no"
	TernaryValueNotApplicable TernaryValue = "n/a"
)

// InspectionDepth determines how much repository state should be gathered.
type InspectionDepth string

// Supported inspection depth variants.
const (
	InspectionDepthFull    InspectionDepth = "full"
	InspectionDepthMinimal InspectionDepth = "minimal"
)

// ReportFormat identifies the serialization used for an audit report.
type ReportFormat string

// Supported audit report formats.
const (
	ReportFormatTable ReportFormat = "table"
	ReportFormatCSV   ReportFormat = "csv"
	ReportFormatHTML  ReportFormat = "html"
)

// DefaultReportFormat returns the terminal-oriented audit report format.
func DefaultReportFormat() ReportFormat {
	return ReportFormatTable
}

// ParseReportFormat validates one exact audit report format value.
func ParseReportFormat(value string) (ReportFormat, error) {
	reportFormat := ReportFormat(value)
	switch reportFormat {
	case ReportFormatTable, ReportFormatCSV, ReportFormatHTML:
		return reportFormat, nil
	default:
		return "", fmt.Errorf("unsupported audit report format %q; expected table, csv, or html", value)
	}
}

func normalizeReportFormat(reportFormat ReportFormat) (ReportFormat, error) {
	if reportFormat == "" {
		return DefaultReportFormat(), nil
	}
	return ParseReportFormat(string(reportFormat))
}

// CommandOptions captures the configurable parameters for the audit command.
type CommandOptions struct {
	Roots             []string
	DebugOutput       bool
	InspectionDepth   InspectionDepth
	IncludeAllFolders bool
	ReportFormat      ReportFormat
}

// RepositoryInspection captures gathered repository state.
type RepositoryInspection struct {
	Path                   string
	FolderName             string
	OriginURL              string
	OriginOwnerRepo        string
	CanonicalOwnerRepo     string
	FinalOwnerRepo         string
	DesiredFolderName      string
	OriginRemoteStatus     OriginRemoteStatus
	RemoteProtocol         RemoteProtocolType
	RemoteDefaultBranch    string
	LocalBranch            string
	HeadTagged             bool
	InSyncStatus           TernaryValue
	OriginMatchesCanonical TernaryValue
	IsGitRepository        bool
	WorktreeDirtyFiles     []string
}

// AuditReportRow models a single CSV audit result.
type AuditReportRow struct {
	FolderName             string
	FinalRepository        string
	OriginRemoteStatus     OriginRemoteStatus
	NameMatches            TernaryValue
	RemoteDefaultBranch    string
	LocalBranch            string
	InSync                 TernaryValue
	RemoteProtocol         RemoteProtocolType
	OriginMatchesCanonical TernaryValue
	WorktreeDirty          TernaryValue
	DirtyFiles             string
}

// CSVRecord returns the row formatted for CSV encoding.
func (row AuditReportRow) CSVRecord() []string {
	return []string{
		row.FolderName,
		row.FinalRepository,
		string(row.OriginRemoteStatus),
		string(row.NameMatches),
		row.RemoteDefaultBranch,
		row.LocalBranch,
		string(row.InSync),
		string(row.RemoteProtocol),
		string(row.OriginMatchesCanonical),
		string(row.WorktreeDirty),
		row.DirtyFiles,
	}
}
