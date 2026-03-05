package licenses

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadTemplateBundleIncludesCommercialLicenseForBSL(testInstance *testing.T) {
	templateBundle, loadError := LoadTemplateBundle(TemplateNameBSL)
	require.NoError(testInstance, loadError)
	require.Contains(testInstance, templateBundle.PrimaryContent, VariableChangeDate)

	_, exists := templateBundle.AdditionalContents[OutputCommercialFileName]
	require.True(testInstance, exists)
}

func TestLoadTemplateBundleUsesEnvironmentExpressions(testInstance *testing.T) {
	templateBundle, loadError := LoadTemplateBundle(TemplateNameMIT)
	require.NoError(testInstance, loadError)
	require.True(testInstance, strings.Contains(templateBundle.PrimaryContent, VariableYear))
	require.True(testInstance, strings.Contains(templateBundle.PrimaryContent, VariableAuthor))
}

func TestLoadTemplateBundleUsesSPDXTaggedProprietaryTemplate(testInstance *testing.T) {
	templateBundle, loadError := LoadTemplateBundle(TemplateNameProprietary)
	require.NoError(testInstance, loadError)
	require.Contains(testInstance, templateBundle.PrimaryContent, "SPDX-License-Identifier: LicenseRef-MPRL-Proprietary")
	require.Contains(testInstance, templateBundle.PrimaryContent, VariableCompany)
	require.Contains(testInstance, templateBundle.PrimaryContent, `The Software is provided "AS IS"`)
	require.Contains(testInstance, templateBundle.PrimaryContent, "No license is granted to the public.")
}

func TestLoadTemplateBundleRejectsUnknownTemplate(testInstance *testing.T) {
	_, loadError := LoadTemplateBundle("unknown")
	require.Error(testInstance, loadError)
}
