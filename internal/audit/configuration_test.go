package audit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/audit"
)

const (
	auditConfigurationAbsoluteSuffixConstant = "audit-configuration-absolute"
	auditConfigurationRelativePathConstant   = "repositories/relative"
	auditConfigurationTildePathConstant      = "~/repositories/home"
	auditConfigurationTildePrefixConstant    = "~/"
	auditConfigurationWhitespacePrefix       = "   "
	auditConfigurationWhitespaceSuffix       = "  "
)

func TestCommandConfigurationSanitizeExpandsHomeDirectories(testInstance *testing.T) {
	testInstance.Helper()

	homeDirectory, homeDirectoryError := os.UserHomeDir()
	require.NoError(testInstance, homeDirectoryError)

	temporaryRoot := testInstance.TempDir()
	absolutePath := filepath.Join(temporaryRoot, auditConfigurationAbsoluteSuffixConstant)
	trimmedTilde := strings.TrimPrefix(auditConfigurationTildePathConstant, auditConfigurationTildePrefixConstant)
	expectedTildePath := filepath.Join(homeDirectory, trimmedTilde)

	testCases := []struct {
		name           string
		roots          []string
		expectedResult []string
	}{
		{
			name:           "absolute_paths_preserved",
			roots:          []string{absolutePath},
			expectedResult: []string{absolutePath},
		},
		{
			name:           "relative_paths_preserved",
			roots:          []string{auditConfigurationRelativePathConstant},
			expectedResult: []string{auditConfigurationRelativePathConstant},
		},
		{
			name:           "tilde_paths_expanded",
			roots:          []string{auditConfigurationTildePathConstant},
			expectedResult: []string{expectedTildePath},
		},
		{
			name:           "whitespace_trimmed_before_expansion",
			roots:          []string{auditConfigurationWhitespacePrefix + auditConfigurationTildePathConstant + auditConfigurationWhitespaceSuffix},
			expectedResult: []string{expectedTildePath},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(subTest *testing.T) {
			configuration := audit.CommandConfiguration{Roots: testCase.roots}
			sanitized := configuration.Sanitize()
			require.Equal(subTest, testCase.expectedResult, sanitized.Roots)
		})
	}
}
