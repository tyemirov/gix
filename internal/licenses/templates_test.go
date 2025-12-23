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

func TestLoadTemplateBundleRejectsUnknownTemplate(testInstance *testing.T) {
	_, loadError := LoadTemplateBundle("unknown")
	require.Error(testInstance, loadError)
}
