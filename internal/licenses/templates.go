package licenses

import (
	"embed"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	TemplateNameBSL         = "bsl"
	TemplateNameMIT         = "mit"
	TemplateNameProprietary = "proprietary"

	VariableTemplate         = "license_template"
	VariableTemplateAlias    = "template"
	VariableContent          = "license_content"
	VariableMode             = "license_mode"
	VariableTarget           = "license_target"
	VariableCommercialTarget = "license_commercial_target"
	VariableYear             = "license_year"
	VariableAuthor           = "license_author"
	VariableCompany          = "license_company"
	VariableLicensor         = "license_licensor"
	VariableProjectName      = "license_project_name"
	VariableChangeDate       = "license_change_date"
	VariableChangeLicense    = "license_change_license"
	DefaultChangeDate        = "2029-01-01"
	DefaultChangeLicense     = "Apache License 2.0"
	OutputLicenseFileName    = "LICENSE"
	OutputCommercialFileName = "COMMERCIAL_LICENSE.md"
)

const (
	placeholderYearConstant          = "{{YEAR}}"
	placeholderAuthorConstant        = "{{AUTHOR}}"
	placeholderCompanyConstant       = "{{COMPANY}}"
	placeholderLicensorConstant      = "{{LICENSOR}}"
	placeholderProjectNameConstant   = "{{PROJECT_NAME}}"
	placeholderChangeDateConstant    = "{{CHANGE_DATE}}"
	placeholderChangeLicenseConstant = "{{CHANGE_LICENSE}}"
)

const (
	environmentValueTemplateConstant    = "{{ .Environment.%s }}"
	environmentFallbackTemplateConstant = "{{ if .Environment.%s }}{{ .Environment.%s }}{{ else }}%s{{ end }}"
	repositoryOwnerTemplateConstant     = "{{ .Repository.Owner }}"
	repositoryNameTemplateConstant      = "{{ .Repository.Name }}"
)

const (
	templatePathBSLLicenseConstant         = "templates/bsl/LICENSE.md"
	templatePathBSLCommercialConstant      = "templates/bsl/COMMERCIAL_LICENSE.md"
	templatePathMITLicenseConstant         = "templates/mit/LICENSE.md"
	templatePathProprietaryLicenseConstant = "templates/proprietary/LICENSE.md"
)

const (
	templateMissingNameErrorTemplateConstant = "license template name is required"
	templateUnsupportedErrorTemplateConstant = "unsupported license template %q"
	templateReadErrorTemplateConstant        = "failed to load license template %q: %w"
)

// TemplateBundle contains rendered license templates for workflow file writes.
type TemplateBundle struct {
	Name               string
	PrimaryOutputPath  string
	PrimaryContent     string
	AdditionalContents map[string]string
}

type templateDescriptor struct {
	Name                    string
	PrimaryTemplatePath     string
	AdditionalTemplatePaths map[string]string
}

//go:embed templates/*/*.md
var embeddedTemplates embed.FS

// LoadTemplateBundle reads embedded license templates and renders placeholder expressions.
func LoadTemplateBundle(templateName string) (TemplateBundle, error) {
	normalizedName := strings.ToLower(strings.TrimSpace(templateName))
	if len(normalizedName) == 0 {
		return TemplateBundle{}, errors.New(templateMissingNameErrorTemplateConstant)
	}

	descriptor, exists := templateCatalog()[normalizedName]
	if !exists {
		return TemplateBundle{}, fmt.Errorf(templateUnsupportedErrorTemplateConstant, templateName)
	}

	primaryContent, primaryError := readTemplateContent(descriptor.PrimaryTemplatePath)
	if primaryError != nil {
		return TemplateBundle{}, primaryError
	}

	additionalContents := make(map[string]string, len(descriptor.AdditionalTemplatePaths))
	for outputPath, templatePath := range descriptor.AdditionalTemplatePaths {
		content, readError := readTemplateContent(templatePath)
		if readError != nil {
			return TemplateBundle{}, readError
		}
		additionalContents[outputPath] = applyPlaceholderReplacements(content)
	}

	return TemplateBundle{
		Name:               descriptor.Name,
		PrimaryOutputPath:  OutputLicenseFileName,
		PrimaryContent:     applyPlaceholderReplacements(primaryContent),
		AdditionalContents: additionalContents,
	}, nil
}

// TemplateNames returns the supported embedded license template names.
func TemplateNames() []string {
	descriptors := templateCatalog()
	names := make([]string, 0, len(descriptors))
	for name := range descriptors {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func templateCatalog() map[string]templateDescriptor {
	return map[string]templateDescriptor{
		TemplateNameBSL: {
			Name:                TemplateNameBSL,
			PrimaryTemplatePath: templatePathBSLLicenseConstant,
			AdditionalTemplatePaths: map[string]string{
				OutputCommercialFileName: templatePathBSLCommercialConstant,
			},
		},
		TemplateNameMIT: {
			Name:                    TemplateNameMIT,
			PrimaryTemplatePath:     templatePathMITLicenseConstant,
			AdditionalTemplatePaths: map[string]string{},
		},
		TemplateNameProprietary: {
			Name:                    TemplateNameProprietary,
			PrimaryTemplatePath:     templatePathProprietaryLicenseConstant,
			AdditionalTemplatePaths: map[string]string{},
		},
	}
}

func readTemplateContent(path string) (string, error) {
	contentBytes, readError := embeddedTemplates.ReadFile(path)
	if readError != nil {
		return "", fmt.Errorf(templateReadErrorTemplateConstant, path, readError)
	}
	return string(contentBytes), nil
}

func applyPlaceholderReplacements(rawTemplate string) string {
	content := rawTemplate
	for placeholderKey, placeholderValue := range placeholderReplacementMap() {
		content = strings.ReplaceAll(content, placeholderKey, placeholderValue)
	}
	return content
}

func placeholderReplacementMap() map[string]string {
	return map[string]string{
		placeholderYearConstant:          environmentValueExpression(VariableYear),
		placeholderAuthorConstant:        environmentFallbackExpression(VariableAuthor, repositoryOwnerTemplateConstant),
		placeholderCompanyConstant:       environmentFallbackExpression(VariableCompany, repositoryOwnerTemplateConstant),
		placeholderLicensorConstant:      environmentFallbackExpression(VariableLicensor, repositoryOwnerTemplateConstant),
		placeholderProjectNameConstant:   environmentFallbackExpression(VariableProjectName, repositoryNameTemplateConstant),
		placeholderChangeDateConstant:    environmentValueExpression(VariableChangeDate),
		placeholderChangeLicenseConstant: environmentValueExpression(VariableChangeLicense),
	}
}

func environmentValueExpression(variableName string) string {
	return fmt.Sprintf(environmentValueTemplateConstant, variableName)
}

func environmentFallbackExpression(variableName string, fallback string) string {
	return fmt.Sprintf(environmentFallbackTemplateConstant, variableName, variableName, fallback)
}
