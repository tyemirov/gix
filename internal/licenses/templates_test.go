package licenses

import (
	"crypto/sha256"
	"fmt"
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

func TestLoadTemplateBundleUsesUnmodifiedPolyFormNoncommercialLicense(testInstance *testing.T) {
	templateBundle, loadError := LoadTemplateBundle(TemplateNamePolyFormNoncommercial)
	require.NoError(testInstance, loadError)

	contentHash := sha256.Sum256([]byte(templateBundle.PrimaryContent))
	require.Equal(
		testInstance,
		"ffcca38841adb694b6f380647e15f17c446a4d1656fed51a1e2041d064c94cc8",
		fmt.Sprintf("%x", contentHash),
	)
	require.Contains(testInstance, templateBundle.PrimaryContent, "## Noncommercial Organizations")
	require.Contains(testInstance, templateBundle.PrimaryContent, "regardless of the source of funding")

	noticeContent, noticeExists := templateBundle.AdditionalContents[OutputNoticeFileName]
	require.True(testInstance, noticeExists)
	require.Contains(testInstance, noticeContent, "Required Notice: Copyright")
	require.Contains(testInstance, noticeContent, VariableYear)
	require.Contains(testInstance, noticeContent, VariableLicensor)

	commercialContent, commercialExists := templateBundle.AdditionalContents[OutputCommercialFileName]
	require.True(testInstance, commercialExists)
	require.Contains(testInstance, commercialContent, VariableContact)
	require.Contains(testInstance, commercialContent, "not itself a commercial license")
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

	noticeContent, noticeExists := templateBundle.AdditionalContents[OutputNoticeFileName]
	require.True(testInstance, noticeExists)
	require.Contains(testInstance, noticeContent, VariableCompany)

	commercialContent, commercialExists := templateBundle.AdditionalContents[OutputCommercialFileName]
	require.True(testInstance, commercialExists)
	require.Contains(testInstance, commercialContent, VariableContact)
	require.Contains(testInstance, commercialContent, "not itself a commercial license")
}

func TestLoadTemplateBundleRejectsUnknownTemplate(testInstance *testing.T) {
	_, loadError := LoadTemplateBundle("unknown")
	require.Error(testInstance, loadError)
}
