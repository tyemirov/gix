package audit

import (
	"encoding/csv"
	"fmt"
	"html"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

const (
	auditReportTitleConstant               = "gix audit report"
	auditHTMLDocumentTitlePrefixConstant   = "<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<title>"
	auditHTMLDocumentTitleSuffixConstant   = "</title>\n<style>body{font-family:system-ui,sans-serif;margin:2rem}table{border-collapse:collapse}th,td{border:1px solid #999;padding:.4rem;text-align:left;vertical-align:top;white-space:pre-wrap}th{background:#f3f3f3}</style>\n</head>\n<body>\n<h1>"
	auditHTMLDocumentHeadingSuffixConstant = "</h1>\n<table>\n<thead>\n<tr>"
	auditHTMLDocumentCloseConstant         = "\n</tbody>\n</table>\n</body>\n</html>\n"
	auditTableColumnsEnvironmentConstant   = "COLUMNS"
	auditTableEllipsisConstant             = "…"
	auditTableMinimumCellWidthConstant     = 12
	auditTableNoRowsMessageConstant        = "No repositories found."
)

var auditTableValueReplacer = strings.NewReplacer(
	"\r\n", "\\n",
	"\n", "\\n",
	"\r", "\\r",
	"\t", "\\t",
	"|", "\\|",
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
	terminalWidth := auditTableTerminalWidth(writer)
	if terminalWidth == 0 || auditTableRenderedWidth(widths) <= terminalWidth {
		return writeAuditTable(writer, header, values, widths)
	}

	minimumWidths := auditTableMinimumColumnWidths(header)
	if auditTableRenderedWidth(minimumWidths) <= terminalWidth {
		fittedWidths := auditTableFitColumnWidths(minimumWidths, widths, terminalWidth)
		return writeAuditTable(writer, header, values, fittedWidths)
	}

	return writeAuditTableFields(writer, header, values, terminalWidth)
}

func writeAuditTable(writer io.Writer, header []string, values [][]string, widths []int) error {
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

func writeAuditTableFields(writer io.Writer, header []string, values [][]string, terminalWidth int) error {
	if len(values) == 0 {
		return writeAuditTableMessage(writer, auditTableNoRowsMessageConstant, terminalWidth)
	}

	labelWidth := auditTableLargestWidth(header)
	valueWidth := terminalWidth - auditTableRenderedWidth([]int{labelWidth, 0})
	if valueWidth < 1 {
		return writeStackedAuditTableFields(writer, header, values, terminalWidth)
	}

	widths := []int{labelWidth, valueWidth}
	border := auditTableBorder(widths)
	if _, writeError := fmt.Fprintln(writer, border); writeError != nil {
		return writeError
	}
	for rowIndex := range values {
		for columnIndex := range header {
			if writeError := writeAuditTableRow(writer, []string{header[columnIndex], values[rowIndex][columnIndex]}, widths); writeError != nil {
				return writeError
			}
		}
		if _, writeError := fmt.Fprintln(writer, border); writeError != nil {
			return writeError
		}
	}
	return nil
}

func writeStackedAuditTableFields(writer io.Writer, header []string, values [][]string, terminalWidth int) error {
	if len(values) == 0 {
		return writeAuditTableMessage(writer, auditTableNoRowsMessageConstant, terminalWidth)
	}

	for rowIndex := range values {
		for columnIndex := range header {
			fieldValue := fmt.Sprintf("%s: %s", header[columnIndex], values[rowIndex][columnIndex])
			if writeError := writeAuditTableMessage(writer, fieldValue, terminalWidth); writeError != nil {
				return writeError
			}
		}
		if rowIndex < len(values)-1 {
			if _, writeError := fmt.Fprintln(writer, strings.Repeat("-", terminalWidth)); writeError != nil {
				return writeError
			}
		}
	}
	return nil
}

func writeAuditTableMessage(writer io.Writer, message string, terminalWidth int) error {
	for _, line := range strings.Split(runewidth.Wrap(message, terminalWidth), "\n") {
		if _, writeError := fmt.Fprintln(writer, auditTableTruncate(line, terminalWidth)); writeError != nil {
			return writeError
		}
	}
	return nil
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
		normalized[valueIndex] = auditTableValueReplacer.Replace(record[valueIndex])
	}
	return normalized
}

func auditTableColumnWidths(header []string, rows [][]string) []int {
	widths := make([]int, len(header))
	for columnIndex := range header {
		widths[columnIndex] = auditTableDisplayWidth(header[columnIndex])
	}
	for rowIndex := range rows {
		for columnIndex := range rows[rowIndex] {
			width := auditTableDisplayWidth(rows[rowIndex][columnIndex])
			if width > widths[columnIndex] {
				widths[columnIndex] = width
			}
		}
	}
	return widths
}

func auditTableMinimumColumnWidths(header []string) []int {
	widths := make([]int, len(header))
	for columnIndex := range header {
		widths[columnIndex] = auditTableDisplayWidth(header[columnIndex])
		if widths[columnIndex] < auditTableMinimumCellWidthConstant {
			widths[columnIndex] = auditTableMinimumCellWidthConstant
		}
	}
	return widths
}

func auditTableFitColumnWidths(minimumWidths []int, naturalWidths []int, terminalWidth int) []int {
	widths := append([]int{}, minimumWidths...)
	remainingWidth := terminalWidth - auditTableRenderedWidth(widths)
	for remainingWidth > 0 {
		expanded := false
		for columnIndex := range widths {
			if widths[columnIndex] >= naturalWidths[columnIndex] {
				continue
			}
			widths[columnIndex]++
			remainingWidth--
			expanded = true
			if remainingWidth == 0 {
				break
			}
		}
		if !expanded {
			break
		}
	}
	return widths
}

func auditTableLargestWidth(values []string) int {
	largestWidth := 0
	for valueIndex := range values {
		valueWidth := auditTableDisplayWidth(values[valueIndex])
		if valueWidth > largestWidth {
			largestWidth = valueWidth
		}
	}
	return largestWidth
}

func auditTableDisplayWidth(value string) int {
	return runewidth.StringWidth(value)
}

func auditTableRenderedWidth(widths []int) int {
	width := 1
	for widthIndex := range widths {
		width += widths[widthIndex] + 3
	}
	return width
}

func auditTableTerminalWidth(writer io.Writer) int {
	outputFile, isFile := writer.(*os.File)
	if !isFile || outputFile.Fd() != os.Stdout.Fd() {
		return 0
	}

	terminalWidth, _, terminalError := term.GetSize(int(outputFile.Fd()))
	if terminalError == nil && terminalWidth > 0 {
		return terminalWidth
	}

	columnsWidth, columnsError := strconv.Atoi(strings.TrimSpace(os.Getenv(auditTableColumnsEnvironmentConstant)))
	if columnsError != nil || columnsWidth < 1 {
		return 0
	}
	return columnsWidth
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
		value := auditTableTruncate(values[valueIndex], widths[valueIndex])
		line.WriteByte(' ')
		line.WriteString(value)
		line.WriteString(strings.Repeat(" ", widths[valueIndex]-auditTableDisplayWidth(value)+1))
		line.WriteByte('|')
	}
	_, writeError := fmt.Fprintln(writer, line.String())
	return writeError
}

func auditTableTruncate(value string, width int) string {
	if auditTableDisplayWidth(value) <= width {
		return value
	}
	return runewidth.Truncate(value, width, auditTableEllipsisConstant)
}
