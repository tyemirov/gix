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

	return executeTemplate(trimmed, data)
}

func renderTemplateContent(rawTemplate string, fallback string, data TaskTemplateData) (string, error) {
	if len(strings.TrimSpace(rawTemplate)) == 0 {
		return fallback, nil
	}
	return executeTemplate(rawTemplate, data)
}

func executeTemplate(rawTemplate string, data TaskTemplateData) (string, error) {
	tmpl, parseError := template.New("task").Parse(rawTemplate)
	if parseError != nil {
		return "", parseError
	}

	var buffer bytes.Buffer
	if executeError := tmpl.Execute(&buffer, data); executeError != nil {
		return "", executeError
	}
	return buffer.String(), nil
}
