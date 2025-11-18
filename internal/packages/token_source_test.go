package packages_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	packages "github.com/tyemirov/gix/internal/packages"
)

func TestParseTokenSourceScenarios(testingInstance *testing.T) {
	testingInstance.Parallel()

	testCases := []struct {
		name         string
		input        string
		expectedType packages.TokenSourceType
		expectedRef  string
		expectError  bool
	}{
		{
			name:         "environment_with_prefix",
			input:        "env:GITHUB_TOKEN",
			expectedType: packages.TokenSourceTypeEnvironment,
			expectedRef:  "GITHUB_TOKEN",
		},
		{
			name:         "file_token_source",
			input:        "file:/tmp/token.txt",
			expectedType: packages.TokenSourceTypeFile,
			expectedRef:  "/tmp/token.txt",
		},
		{
			name:         "environment_without_prefix",
			input:        "TOKEN_NAME",
			expectedType: packages.TokenSourceTypeEnvironment,
			expectedRef:  "TOKEN_NAME",
		},
		{
			name:        "missing_value",
			input:       " ",
			expectError: true,
		},
		{
			name:        "unsupported_type",
			input:       "secret:TOKEN",
			expectError: true,
		},
		{
			name:        "missing_environment_reference",
			input:       "env:",
			expectError: true,
		},
		{
			name:        "missing_file_reference",
			input:       "file:",
			expectError: true,
		},
	}

	for index := range testCases {
		testCase := testCases[index]

		testingInstance.Run(testCase.name, func(testingSubInstance *testing.T) {
			testingSubInstance.Parallel()

			configuration, parseError := packages.ParseTokenSource(testCase.input)
			if testCase.expectError {
				require.Error(testingSubInstance, parseError)
				return
			}

			require.NoError(testingSubInstance, parseError)
			require.Equal(testingSubInstance, testCase.expectedType, configuration.Type)
			require.Equal(testingSubInstance, testCase.expectedRef, configuration.Reference)
		})
	}
}

func TestTokenResolverResolvesValues(testingInstance *testing.T) {
	testingInstance.Parallel()

	environment := map[string]string{
		"TOKEN_PRESENT": " secret-token ",
	}

	fileContents := map[string][]byte{
		"/tmp/token.txt": []byte("file-token\n"),
	}

	environmentLookup := func(key string) (string, bool) {
		value, found := environment[key]
		return value, found
	}

	fileReader := func(path string) ([]byte, error) {
		content, found := fileContents[path]
		if !found {
			return nil, errors.New("missing file")
		}
		return content, nil
	}

	resolver := packages.NewTokenResolver(environmentLookup, fileReader)

	testCases := []struct {
		name          string
		configuration packages.TokenSourceConfiguration
		expected      string
		expectError   bool
	}{
		{
			name:          "environment_success",
			configuration: packages.TokenSourceConfiguration{Type: packages.TokenSourceTypeEnvironment, Reference: "TOKEN_PRESENT"},
			expected:      "secret-token",
		},
		{
			name:          "environment_missing",
			configuration: packages.TokenSourceConfiguration{Type: packages.TokenSourceTypeEnvironment, Reference: "MISSING"},
			expectError:   true,
		},
		{
			name:          "file_success",
			configuration: packages.TokenSourceConfiguration{Type: packages.TokenSourceTypeFile, Reference: "/tmp/token.txt"},
			expected:      "file-token",
		},
		{
			name:          "file_missing",
			configuration: packages.TokenSourceConfiguration{Type: packages.TokenSourceTypeFile, Reference: "/tmp/missing.txt"},
			expectError:   true,
		},
		{
			name:          "file_empty",
			configuration: packages.TokenSourceConfiguration{Type: packages.TokenSourceTypeFile, Reference: "/tmp/empty.txt"},
			expectError:   true,
		},
	}

	fileContents["/tmp/empty.txt"] = []byte("   \n")

	for index := range testCases {
		testCase := testCases[index]

		testingInstance.Run(testCase.name, func(testingSubInstance *testing.T) {
			testingSubInstance.Parallel()

			value, resolutionError := resolver.ResolveToken(context.Background(), testCase.configuration)
			if testCase.expectError {
				require.Error(testingSubInstance, resolutionError)
				return
			}

			require.NoError(testingSubInstance, resolutionError)
			require.Equal(testingSubInstance, testCase.expected, value)
		})
	}
}
