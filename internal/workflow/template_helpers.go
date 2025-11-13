package workflow

import (
	"bytes"
	"strings"
	"text/template"
)

func renderTemplateValue(rawTemplate string, fallback string, data TaskTemplateData) (string, error) {
	trimmed := strings.TrimSpace(rawTemplate)
	if len(trimmed) == 0 {
		return fallback, nil
	}

	tmpl, parseError := template.New("task").Parse(trimmed)
	if parseError != nil {
		return "", parseError
	}

	var buffer bytes.Buffer
	if executeError := tmpl.Execute(&buffer, data); executeError != nil {
		return "", executeError
	}
	return buffer.String(), nil
}
