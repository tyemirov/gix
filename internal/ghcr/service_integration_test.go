package ghcr_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/ghcr"
)

const (
	integrationOwnerNameConstant    = "integration-owner"
	integrationPackageNameConstant  = "integration-package"
	integrationTokenConstant        = "integration-token"
	untaggedVersionIdentifier       = int64(501)
	taggedVersionIdentifier         = int64(999)
	expectedAcceptHeaderName        = "Accept"
	expectedAuthorizationHeaderName = "Authorization"
	expectedAcceptHeaderValue       = "application/vnd.github+json"
	expectedBearerHeaderTemplate    = "Bearer %s"
	pageQueryParameterName          = "page"
)

func TestPackageVersionServiceIntegration(testingInstance *testing.T) {
	testingInstance.Parallel()

	testCases := []struct {
		name                string
		dryRun              bool
		expectedDeleteCount int
	}{
		{
			name:                "dry_run_does_not_delete",
			dryRun:              true,
			expectedDeleteCount: 0,
		},
		{
			name:                "deletes_when_not_dry_run",
			dryRun:              false,
			expectedDeleteCount: 1,
		},
	}

	for index := range testCases {
		testCase := testCases[index]

		testingInstance.Run(testCase.name, func(testingSubInstance *testing.T) {
			testingSubInstance.Parallel()

			recordedDeleteIdentifiers := make([]int64, 0)
			requestCount := 0

			server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
				requestCount++
				require.Equal(testingSubInstance, expectedAcceptHeaderValue, httpRequest.Header.Get(expectedAcceptHeaderName))
				require.Equal(testingSubInstance, fmt.Sprintf(expectedBearerHeaderTemplate, integrationTokenConstant), httpRequest.Header.Get(expectedAuthorizationHeaderName))

				switch httpRequest.Method {
				case http.MethodGet:
					handleIntegrationGet(testingSubInstance, responseWriter, httpRequest)
				case http.MethodDelete:
					versionIdentifier, parseError := parseVersionIdentifierFromPath(httpRequest.URL.Path)
					require.NoError(testingSubInstance, parseError)
					recordedDeleteIdentifiers = append(recordedDeleteIdentifiers, versionIdentifier)
					responseWriter.WriteHeader(http.StatusNoContent)
				default:
					testingSubInstance.Fatalf("unexpected method %s", httpRequest.Method)
				}
			}))
			defer server.Close()

			service, serviceError := ghcr.NewPackageVersionService(zap.NewNop(), server.Client(), ghcr.ServiceConfiguration{
				BaseURL:  server.URL,
				PageSize: 2,
			})
			require.NoError(testingSubInstance, serviceError)

			result, purgeError := service.PurgeUntaggedVersions(context.Background(), ghcr.PurgeRequest{
				Owner:       integrationOwnerNameConstant,
				PackageName: integrationPackageNameConstant,
				OwnerType:   ghcr.OrganizationOwnerType,
				Token:       integrationTokenConstant,
				DryRun:      testCase.dryRun,
			})
			require.NoError(testingSubInstance, purgeError)
			require.Equal(testingSubInstance, 2, result.TotalVersions)
			require.Equal(testingSubInstance, 1, result.UntaggedVersions)
			require.Equal(testingSubInstance, testCase.expectedDeleteCount, result.DeletedVersions)
			require.Len(testingSubInstance, recordedDeleteIdentifiers, testCase.expectedDeleteCount)
			require.GreaterOrEqual(testingSubInstance, requestCount, 2)
		})
	}
}

func handleIntegrationGet(testingInstance *testing.T, responseWriter http.ResponseWriter, httpRequest *http.Request) {
	query := httpRequest.URL.Query()
	pageValue := query.Get(pageQueryParameterName)
	require.NotEmpty(testingInstance, pageValue)

	switch pageValue {
	case "1":
		payload := fmt.Sprintf(`[{"id":%d,"metadata":{"container":{"tags":[]}}},{"id":%d,"metadata":{"container":{"tags":["latest"]}}}]`, untaggedVersionIdentifier, taggedVersionIdentifier)
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte(payload))
	default:
		responseWriter.Header().Set("Content-Type", "application/json")
		_, _ = responseWriter.Write([]byte("[]"))
	}
}

func parseVersionIdentifierFromPath(requestPath string) (int64, error) {
	trimmedPath := strings.Trim(requestPath, "/")
	segments := strings.Split(trimmedPath, "/")
	if len(segments) == 0 {
		return 0, fmt.Errorf("invalid path %s", requestPath)
	}

	identifierSegment := segments[len(segments)-1]
	return strconv.ParseInt(identifierSegment, 10, 64)
}
