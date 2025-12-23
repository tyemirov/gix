package workflow

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tyemirov/gix/internal/licenses"
	workflowpkg "github.com/tyemirov/gix/internal/workflow"
)

const (
	tasksApplyCommandKeyConstant                       = "tasks apply"
	tasksOptionKeyConstant                             = "tasks"
	taskNameKeyConstant                                = "name"
	taskFilesKeyConstant                               = "files"
	taskFileContentKeyConstant                         = "content"
	taskFileModeKeyConstant                            = "mode"
	taskFilePathKeyConstant                            = "path"
	taskFilePermissionsKeyConstant                     = "permissions"
	licenseTaskNameConstant                            = "Distribute license file"
	defaultLicenseModeConstant                         = "overwrite"
	defaultLicensePermissionsConstant                  = 420
	licenseTemplateConflictErrorTemplateConstant       = "license template %q cannot be used with %s"
	licenseTemplateMissingStepErrorTemplateConstant    = "license template %q did not match any workflow steps"
	licenseTemplateTasksTypeErrorTemplateConstant      = "license workflow tasks must be a list"
	licenseTemplateTaskTypeErrorTemplateConstant       = "license workflow task entry must be a map"
	licenseTemplateFilesTypeErrorTemplateConstant      = "license workflow files must be a list"
	licenseTemplateFileTypeErrorTemplateConstant       = "license workflow file entry must be a map"
	licenseTemplateContentTypeErrorTemplateConstant    = "license workflow file content must be a string"
	licenseTemplateContentMissingErrorTemplateConstant = "license workflow task %q did not include license content"
	licenseContentMarkerTemplateConstant               = "{{ .Environment.%s }}"
	licenseCommercialTargetTemplateConstant            = "{{ if .Environment.%s }}{{ .Environment.%s }}{{ else }}%s{{ end }}"
)

func applyLicenseTemplateOverrides(configuration *workflowpkg.Configuration, variables map[string]string) error {
	if configuration == nil || variables == nil {
		return nil
	}

	templateName, hasTemplate := resolveLicenseTemplateName(variables)
	if !hasTemplate {
		return nil
	}

	if hasLicenseContentVariable(variables) {
		return fmt.Errorf(licenseTemplateConflictErrorTemplateConstant, templateName, licenses.VariableContent)
	}

	templateBundle, loadError := licenses.LoadTemplateBundle(templateName)
	if loadError != nil {
		return loadError
	}

	applyLicenseTemplateDefaults(variables, templateBundle.Name)
	if len(strings.TrimSpace(variables[licenses.VariableTemplate])) == 0 {
		variables[licenses.VariableTemplate] = templateBundle.Name
	}

	updated := false
	for stepIndex := range configuration.Steps {
		step := &configuration.Steps[stepIndex]
		if workflowpkg.CommandPathKey(step.Command) != tasksApplyCommandKeyConstant {
			continue
		}
		stepUpdated, stepError := applyTemplateToStep(step, templateBundle)
		if stepError != nil {
			return stepError
		}
		if stepUpdated {
			updated = true
		}
	}

	if !updated {
		return fmt.Errorf(licenseTemplateMissingStepErrorTemplateConstant, templateBundle.Name)
	}
	return nil
}

func resolveLicenseTemplateName(variables map[string]string) (string, bool) {
	if variables == nil {
		return "", false
	}
	templateValue := strings.TrimSpace(variables[licenses.VariableTemplate])
	if len(templateValue) > 0 {
		return templateValue, true
	}
	templateValue = strings.TrimSpace(variables[licenses.VariableTemplateAlias])
	if len(templateValue) > 0 {
		return templateValue, true
	}
	return "", false
}

func hasLicenseContentVariable(variables map[string]string) bool {
	if variables == nil {
		return false
	}
	contentValue, exists := variables[licenses.VariableContent]
	return exists && len(strings.TrimSpace(contentValue)) > 0
}

func applyLicenseTemplateDefaults(variables map[string]string, templateName string) {
	if variables == nil {
		return
	}
	if len(strings.TrimSpace(variables[licenses.VariableYear])) == 0 {
		variables[licenses.VariableYear] = currentYearString()
	}
	if !strings.EqualFold(templateName, licenses.TemplateNameBSL) {
		return
	}
	if len(strings.TrimSpace(variables[licenses.VariableChangeDate])) == 0 {
		variables[licenses.VariableChangeDate] = licenses.DefaultChangeDate
	}
	if len(strings.TrimSpace(variables[licenses.VariableChangeLicense])) == 0 {
		variables[licenses.VariableChangeLicense] = licenses.DefaultChangeLicense
	}
}

func currentYearString() string {
	yearValue := time.Now().UTC().Year()
	return strconv.Itoa(yearValue)
}

func applyTemplateToStep(step *workflowpkg.StepConfiguration, templateBundle licenses.TemplateBundle) (bool, error) {
	if step == nil || step.Options == nil {
		return false, nil
	}

	tasksValue, exists := step.Options[tasksOptionKeyConstant]
	if !exists {
		return false, nil
	}
	tasksSlice, ok := tasksValue.([]any)
	if !ok {
		return false, errors.New(licenseTemplateTasksTypeErrorTemplateConstant)
	}

	updated := false
	for taskIndex := range tasksSlice {
		taskEntry, ok := tasksSlice[taskIndex].(map[string]any)
		if !ok {
			return false, errors.New(licenseTemplateTaskTypeErrorTemplateConstant)
		}
		taskUpdated, taskError := applyTemplateToTask(taskEntry, templateBundle)
		if taskError != nil {
			return false, taskError
		}
		if taskUpdated {
			tasksSlice[taskIndex] = taskEntry
			updated = true
		}
	}

	if updated {
		step.Options[tasksOptionKeyConstant] = tasksSlice
	}
	return updated, nil
}

func applyTemplateToTask(taskEntry map[string]any, templateBundle licenses.TemplateBundle) (bool, error) {
	if taskEntry == nil {
		return false, nil
	}

	taskNameValue, _ := taskEntry[taskNameKeyConstant].(string)
	taskName := strings.TrimSpace(taskNameValue)
	taskNameMatches := strings.EqualFold(taskName, licenseTaskNameConstant)

	filesValue, exists := taskEntry[taskFilesKeyConstant]
	if !exists {
		if taskNameMatches {
			return false, errors.New(licenseTemplateFilesTypeErrorTemplateConstant)
		}
		return false, nil
	}
	filesSlice, ok := filesValue.([]any)
	if !ok {
		return false, errors.New(licenseTemplateFilesTypeErrorTemplateConstant)
	}

	licenseFileIndex, licenseFile, fileFound, fileError := findLicenseFileEntry(filesSlice, taskNameMatches)
	if fileError != nil {
		return false, fileError
	}
	if !fileFound {
		if taskNameMatches {
			return false, fmt.Errorf(licenseTemplateContentMissingErrorTemplateConstant, taskName)
		}
		return false, nil
	}
	licenseFile[taskFileContentKeyConstant] = templateBundle.PrimaryContent
	filesSlice[licenseFileIndex] = licenseFile

	modeValue := findFileModeValue(licenseFile)
	permissionsValue := findFilePermissionsValue(licenseFile)

	for outputPath, content := range templateBundle.AdditionalContents {
		exists, existsError := hasFilePath(filesSlice, outputPath)
		if existsError != nil {
			return false, existsError
		}
		if exists {
			continue
		}
		filesSlice = append(filesSlice, map[string]any{
			taskFilePathKeyConstant:        additionalFilePathTemplate(outputPath),
			taskFileContentKeyConstant:     content,
			taskFileModeKeyConstant:        modeValue,
			taskFilePermissionsKeyConstant: permissionsValue,
		})
	}

	taskEntry[taskFilesKeyConstant] = filesSlice
	return true, nil
}

func findLicenseFileEntry(filesSlice []any, strict bool) (int, map[string]any, bool, error) {
	contentMarker := fmt.Sprintf(licenseContentMarkerTemplateConstant, licenses.VariableContent)
	for fileIndex, fileValue := range filesSlice {
		fileEntry, ok := fileValue.(map[string]any)
		if !ok {
			if strict {
				return 0, nil, false, errors.New(licenseTemplateFileTypeErrorTemplateConstant)
			}
			continue
		}
		contentValue, exists := fileEntry[taskFileContentKeyConstant]
		if !exists {
			continue
		}
		contentString, ok := contentValue.(string)
		if !ok {
			if strict {
				return 0, nil, false, errors.New(licenseTemplateContentTypeErrorTemplateConstant)
			}
			continue
		}
		if strings.Contains(contentString, contentMarker) {
			return fileIndex, fileEntry, true, nil
		}
	}
	return 0, nil, false, nil
}

func findFileModeValue(fileEntry map[string]any) string {
	modeValue, ok := fileEntry[taskFileModeKeyConstant].(string)
	if ok && len(strings.TrimSpace(modeValue)) > 0 {
		return modeValue
	}
	return defaultLicenseModeConstant
}

func findFilePermissionsValue(fileEntry map[string]any) int {
	permissionsValue, ok := fileEntry[taskFilePermissionsKeyConstant]
	if ok {
		if intValue, ok := permissionsValue.(int); ok {
			return intValue
		}
		if intValue, ok := permissionsValue.(int64); ok {
			return int(intValue)
		}
	}
	return defaultLicensePermissionsConstant
}

func hasFilePath(filesSlice []any, outputPath string) (bool, error) {
	for _, fileValue := range filesSlice {
		fileEntry, ok := fileValue.(map[string]any)
		if !ok {
			return false, errors.New(licenseTemplateFileTypeErrorTemplateConstant)
		}
		pathValue, ok := fileEntry[taskFilePathKeyConstant].(string)
		if !ok {
			return false, errors.New(licenseTemplateFileTypeErrorTemplateConstant)
		}
		if strings.Contains(pathValue, outputPath) {
			return true, nil
		}
	}
	return false, nil
}

func additionalFilePathTemplate(outputPath string) string {
	if outputPath != licenses.OutputCommercialFileName {
		return outputPath
	}
	return fmt.Sprintf(
		licenseCommercialTargetTemplateConstant,
		licenses.VariableCommercialTarget,
		licenses.VariableCommercialTarget,
		licenses.OutputCommercialFileName,
	)
}
