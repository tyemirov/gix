package audit

import (
	"encoding/csv"
	"fmt"
	"html"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	auditReportTitleConstant               = "gix audit report"
	auditHTMLDocumentTitlePrefixConstant   = "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<title>"
	auditHTMLDocumentTitleSuffixConstant   = "</title>\n<style>body{font-family:system-ui,sans-serif;margin:2rem}table{border-collapse:collapse}th,td{border:1px solid #999;padding:.4rem;text-align:left;vertical-align:top;white-space:pre-wrap}th{background:#f3f3f3}</style>\n</head>\n<body>\n<h1>"
	auditHTMLDocumentHeadingSuffixConstant = "</h1>\n<table>\n<thead>\n<tr>"
	auditHTMLDocumentCloseConstant         = "\n</tbody>\n</table>\n</body>\n</html>\n"
)

// WriteReport serializes repository inspections in the requested report format.
func WriteReport(writer io.Writer, reportFormat ReportFormat, inspections []RepositoryInspection) error {
	normalizedFormat, formatError := normalizeReportFormat(reportFormat)
	if formatError != nil {
		return formatError
	}

	rows := make([]AuditReportRow, len(inspections))
	for inspectionIndex := range inspections {
		rows[inspectionIndex] = inspectionReportRow(inspections[inspectionIndex])
	}

	switch normalizedFormat {
	case ReportFormatTable:
		if writeError := writeTableReport(writer, rows); writeError != nil {
			return fmt.Errorf("write table audit report: %w", writeError)
		}
	case ReportFormatCSV:
		if writeError := writeCSVReport(writer, rows); writeError != nil {
			return fmt.Errorf("write CSV audit report: %w", writeError)
		}
	case ReportFormatHTML:
		if writeError := writeHTMLReport(writer, rows); writeError != nil {
			return fmt.Errorf("write HTML audit report: %w", writeError)
		}
	}

	return nil
}

func writeCSVReport(writer io.Writer, rows []AuditReportRow) error {
	csvWriter := csv.NewWriter(writer)
	if writeError := csvWriter.Write(auditReportCSVHeaders()); writeError != nil {
		return writeError
	}

	for rowIndex := range rows {
		if writeError := csvWriter.Write(rows[rowIndex].CSVRecord()); writeError != nil {
			return writeError
		}
	}

	csvWriter.Flush()
	return csvWriter.Error()
}

func writeTableReport(writer io.Writer, rows []AuditReportRow) error {
	header := auditReportDisplayHeaders()
	values := make([][]string, 0, len(rows))
	for rowIndex := range rows {
		values = append(values, normalizeTableRecord(rows[rowIndex].CSVRecord()))
	}

	widths := auditTableColumnWidths(header, values)
	border := auditTableBorder(widths)
	if _, writeError := fmt.Fprintln(writer, border); writeError != nil {
		return writeError
	}
	if writeError := writeAuditTableRow(writer, header, widths); writeError != nil {
		return writeError
	}
	if _, writeError := fmt.Fprintln(writer, border); writeError != nil {
		return writeError
	}
	for rowIndex := range values {
		if writeError := writeAuditTableRow(writer, values[rowIndex], widths); writeError != nil {
			return writeError
		}
	}
	_, writeError := fmt.Fprintln(writer, border)
	return writeError
}

func writeHTMLReport(writer io.Writer, rows []AuditReportRow) error {
	var document strings.Builder
	document.WriteString(auditHTMLDocumentTitlePrefixConstant)
	document.WriteString(auditReportTitleConstant)
	document.WriteString(auditHTMLDocumentTitleSuffixConstant)
	document.WriteString(auditReportTitleConstant)
	document.WriteString(auditHTMLDocumentHeadingSuffixConstant)
	for _, header := range auditReportDisplayHeaders() {
		document.WriteString("\n<th>")
		document.WriteString(html.EscapeString(header))
		document.WriteString("</th>")
	}
	document.WriteString("\n</tr>\n</thead>\n<tbody>")
	for rowIndex := range rows {
		document.WriteString("\n<tr>")
		for _, value := range rows[rowIndex].CSVRecord() {
			document.WriteString("\n<td>")
			document.WriteString(html.EscapeString(value))
			document.WriteString("</td>")
		}
		document.WriteString("\n</tr>")
	}
	document.WriteString(auditHTMLDocumentCloseConstant)
	_, writeError := io.WriteString(writer, document.String())
	return writeError
}

func auditReportCSVHeaders() []string {
	return []string{
		csvHeaderFolderName,
		csvHeaderFinalRepository,
		csvHeaderOriginRemoteStatus,
		csvHeaderNameMatches,
		csvHeaderRemoteDefault,
		csvHeaderLocalBranch,
		csvHeaderInSync,
		csvHeaderRemoteProtocol,
		csvHeaderOriginCanonical,
		csvHeaderWorktreeDirty,
		csvHeaderDirtyFiles,
	}
}

func auditReportDisplayHeaders() []string {
	return []string{
		"Folder",
		"Final Repository",
		"Origin",
		"Name Matches",
		"Remote Default",
		"Local Branch",
		"In Sync",
		"Protocol",
		"Origin Canonical",
		"Worktree Dirty",
		"Dirty Files",
	}
}

func normalizeTableRecord(record []string) []string {
	normalized := make([]string, len(record))
	for valueIndex := range record {
		normalized[valueIndex] = strings.NewReplacer("\r\n", "\\n", "\n", "\\n", "\r", "\\r", "\t", "\\t").Replace(record[valueIndex])
	}
	return normalized
}

func auditTableColumnWidths(header []string, rows [][]string) []int {
	widths := make([]int, len(header))
	for columnIndex := range header {
		widths[columnIndex] = utf8.RuneCountInString(header[columnIndex])
	}
	for rowIndex := range rows {
		for columnIndex := range rows[rowIndex] {
			width := utf8.RuneCountInString(rows[rowIndex][columnIndex])
			if width > widths[columnIndex] {
				widths[columnIndex] = width
			}
		}
	}
	return widths
}

func auditTableBorder(widths []int) string {
	var border strings.Builder
	border.WriteByte('+')
	for widthIndex := range widths {
		border.WriteString(strings.Repeat("-", widths[widthIndex]+2))
		border.WriteByte('+')
	}
	return border.String()
}

func writeAuditTableRow(writer io.Writer, values []string, widths []int) error {
	var line strings.Builder
	line.WriteByte('|')
	for valueIndex := range values {
		line.WriteByte(' ')
		line.WriteString(values[valueIndex])
		line.WriteString(strings.Repeat(" ", widths[valueIndex]-utf8.RuneCountInString(values[valueIndex])+1))
		line.WriteByte('|')
	}
	_, writeError := fmt.Fprintln(writer, line.String())
	return writeError
}
