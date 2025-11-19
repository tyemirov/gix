package roots_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	flagutils "github.com/tyemirov/gix/internal/utils/flags"
	rootutils "github.com/tyemirov/gix/internal/utils/roots"
)

func TestResolveRootSelectionScenarios(testInstance *testing.T) {
	homeDirectory, homeDirectoryError := os.UserHomeDir()
	require.NoError(testInstance, homeDirectoryError)

	tildeInput := "~/integration"
	expectedTilde := filepath.Join(homeDirectory, "integration")

	testCases := []struct {
		name          string
		flagArguments []string
		positional    []string
		configured    []string
		expectedRoots []string
		expectedError string
	}{
		{
			name:          "returns_flag_roots_when_provided",
			flagArguments: []string{"--" + flagutils.DefaultRootFlagName, strings.TrimSpace("  " + tildeInput + "  ")},
			configured:    []string{"/configured/root"},
			expectedRoots: []string{expectedTilde},
		},
		{
			name:          "falls_back_to_configured_roots",
			flagArguments: nil,
			configured:    []string{"  ~/configured  "},
			expectedRoots: []string{filepath.Join(homeDirectory, "configured")},
		},
		{
			name:          "errors_when_positional_roots_provided",
			positional:    []string{"relative/root"},
			expectedError: rootutils.PositionalRootsUnsupportedMessage(),
		},
		{
			name:          "errors_when_roots_missing",
			flagArguments: nil,
			configured:    nil,
			expectedError: rootutils.MissingRootsMessage(),
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]
		testInstance.Run(testCase.name, func(subtest *testing.T) {
			command := &cobra.Command{Use: "root-test"}
			flagutils.BindRootFlags(command, flagutils.RootFlagValues{}, flagutils.RootFlagDefinition{Name: flagutils.DefaultRootFlagName, Enabled: true})

			if len(testCase.flagArguments) > 0 {
				parseError := command.ParseFlags(testCase.flagArguments)
				require.NoError(subtest, parseError)
			}

			resolvedRoots, resolveError := rootutils.Resolve(command, append([]string{}, testCase.positional...), append([]string{}, testCase.configured...))

			if len(testCase.expectedError) > 0 {
				require.Error(subtest, resolveError)
				require.EqualError(subtest, resolveError, testCase.expectedError)
				return
			}

			require.NoError(subtest, resolveError)
			require.Equal(subtest, testCase.expectedRoots, resolvedRoots)
		})
	}
}

func TestSanitizeConfiguredRemovesEmptyValues(testInstance *testing.T) {
	sanitized := rootutils.SanitizeConfigured([]string{"  ", "~/configured"})
	require.Len(testInstance, sanitized, 1)

	homeDirectory, homeDirectoryError := os.UserHomeDir()
	require.NoError(testInstance, homeDirectoryError)
	require.Equal(testInstance, filepath.Join(homeDirectory, "configured"), sanitized[0])
}
