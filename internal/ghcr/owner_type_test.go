package ghcr_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/ghcr"
)

func TestParseOwnerTypeScenarios(testingInstance *testing.T) {
	testingInstance.Parallel()

	testCases := []struct {
		name                string
		inputValue          string
		expectedOwnerType   ghcr.OwnerType
		expectError         bool
		expectedPathSegment string
	}{
		{
			name:                "user_owner_type",
			inputValue:          "user",
			expectedOwnerType:   ghcr.UserOwnerType,
			expectError:         false,
			expectedPathSegment: "users",
		},
		{
			name:                "organization_owner_type",
			inputValue:          "org",
			expectedOwnerType:   ghcr.OrganizationOwnerType,
			expectError:         false,
			expectedPathSegment: "orgs",
		},
		{
			name:                "trims_whitespace_and_lowercases",
			inputValue:          " USER ",
			expectedOwnerType:   ghcr.UserOwnerType,
			expectError:         false,
			expectedPathSegment: "users",
		},
		{
			name:        "empty_value_returns_error",
			inputValue:  " ",
			expectError: true,
		},
		{
			name:        "invalid_value_returns_error",
			inputValue:  "team",
			expectError: true,
		},
	}

	for testCaseIndex := range testCases {
		testCase := testCases[testCaseIndex]

		testingInstance.Run(testCase.name, func(testingSubInstance *testing.T) {
			testingSubInstance.Parallel()

			ownerType, parseError := ghcr.ParseOwnerType(testCase.inputValue)
			if testCase.expectError {
				require.Error(testingSubInstance, parseError)
				return
			}

			require.NoError(testingSubInstance, parseError)
			require.Equal(testingSubInstance, testCase.expectedOwnerType, ownerType)
			require.Equal(testingSubInstance, testCase.expectedPathSegment, ownerType.PathSegment())
		})
	}
}
