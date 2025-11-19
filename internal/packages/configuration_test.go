package packages_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/packages"
)

const (
	packagesConfigurationAbsoluteSuffixConstant = "packages-configuration-absolute"
	packagesConfigurationRelativePathConstant   = "packages/repositories"
	packagesConfigurationTildePathConstant      = "~/packages/repositories"
	packagesConfigurationTildePrefixConstant    = "~/"
)

func TestConfigurationSanitizeExpandsHomeDirectories(testInstance *testing.T) {
	testInstance.Helper()

	homeDirectory, homeDirectoryError := os.UserHomeDir()
	require.NoError(testInstance, homeDirectoryError)

	temporaryRoot := testInstance.TempDir()
	absolutePath := filepath.Join(temporaryRoot, packagesConfigurationAbsoluteSuffixConstant)
	trimmedTilde := strings.TrimPrefix(packagesConfigurationTildePathConstant, packagesConfigurationTildePrefixConstant)
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
			roots:          []string{packagesConfigurationRelativePathConstant},
			expectedResult: []string{packagesConfigurationRelativePathConstant},
		},
		{
			name:           "tilde_paths_expanded",
			roots:          []string{packagesConfigurationTildePathConstant},
			expectedResult: []string{expectedTildePath},
		},
	}

	for _, testCase := range testCases {
		testInstance.Run(testCase.name, func(subTest *testing.T) {
			configuration := packages.Configuration{Purge: packages.PurgeConfiguration{RepositoryRoots: testCase.roots}}
			sanitized := configuration.Sanitize()
			require.Equal(subTest, testCase.expectedResult, sanitized.Purge.RepositoryRoots)
		})
	}
}
