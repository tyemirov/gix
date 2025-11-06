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

	"github.com/temirov/gix/internal/ghcr"
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

	recordedDeleteIdentifiers := make([]int64, 0)
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, httpRequest *http.Request) {
		requestCount++
		require.Equal(testingInstance, expectedAcceptHeaderValue, httpRequest.Header.Get(expectedAcceptHeaderName))
		require.Equal(testingInstance, fmt.Sprintf(expectedBearerHeaderTemplate, integrationTokenConstant), httpRequest.Header.Get(expectedAuthorizationHeaderName))

		switch httpRequest.Method {
		case http.MethodGet:
			handleIntegrationGet(testingInstance, responseWriter, httpRequest)
		case http.MethodDelete:
			versionIdentifier, parseError := parseVersionIdentifierFromPath(httpRequest.URL.Path)
			require.NoError(testingInstance, parseError)
			recordedDeleteIdentifiers = append(recordedDeleteIdentifiers, versionIdentifier)
			responseWriter.WriteHeader(http.StatusNoContent)
		default:
			testingInstance.Fatalf("unexpected method %s", httpRequest.Method)
		}
	}))
	defer server.Close()

	service, serviceError := ghcr.NewPackageVersionService(zap.NewNop(), server.Client(), ghcr.ServiceConfiguration{
		BaseURL:  server.URL,
		PageSize: 2,
	})
	require.NoError(testingInstance, serviceError)

	result, purgeError := service.PurgeUntaggedVersions(context.Background(), ghcr.PurgeRequest{
		Owner:       integrationOwnerNameConstant,
		PackageName: integrationPackageNameConstant,
		OwnerType:   ghcr.OrganizationOwnerType,
		Token:       integrationTokenConstant,
	})
	require.NoError(testingInstance, purgeError)
	require.Equal(testingInstance, 2, result.TotalVersions)
	require.Equal(testingInstance, 1, result.UntaggedVersions)
	require.Equal(testingInstance, 1, result.DeletedVersions)
	require.Len(testingInstance, recordedDeleteIdentifiers, 1)
	require.GreaterOrEqual(testingInstance, requestCount, 2)
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
