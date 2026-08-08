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

	"github.com/tyemirov/gix/v5/internal/ghcr"
)

const (
	integrationOwnerNameConstant    = "integration-owner"
	integrationPackageNameConstant  = "integration-package"
	integrationTokenConstant        = "integration-token"
	expectedAcceptHeaderName        = "Accept"
	expectedAuthorizationHeaderName = "Authorization"
	expectedAcceptHeaderValue       = "application/vnd.github+json"
	expectedBearerHeaderTemplate    = "Bearer %s"
	pageQueryParameterName          = "page"
)

func TestPackageVersionServiceIntegration(testingInstance *testing.T) {
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

	result, retentionError := service.ApplyRetention(context.Background(), ghcr.RetentionRequest{
		Owner:       integrationOwnerNameConstant,
		PackageName: integrationPackageNameConstant,
		OwnerType:   ghcr.OrganizationOwnerType,
		Token:       integrationTokenConstant,
		Keep:        mustKeepCount(testingInstance, 3),
	})
	require.NoError(testingInstance, retentionError)
	require.Equal(testingInstance, ghcr.RetentionResult{TotalVersions: 5, RetainedVersions: 3, DeletedVersions: 2}, result)
	require.Equal(testingInstance, []int64{101, 202}, recordedDeleteIdentifiers)
	require.Equal(testingInstance, 6, requestCount)
}

func handleIntegrationGet(testingInstance *testing.T, responseWriter http.ResponseWriter, httpRequest *http.Request) {
	query := httpRequest.URL.Query()
	pageValue := query.Get(pageQueryParameterName)
	require.NotEmpty(testingInstance, pageValue)

	var payload string
	switch pageValue {
	case "1":
		payload = `[
			{"id":505,"created_at":"2026-01-05T00:00:00Z","metadata":{"container":{"tags":["latest","v5"]}}},
			{"id":404,"created_at":"2026-01-04T00:00:00Z","metadata":{"container":{"tags":[]}}}
		]`
	case "2":
		payload = `[
			{"id":303,"created_at":"2026-01-03T00:00:00Z","metadata":{"container":{"tags":["v3"]}}},
			{"id":202,"created_at":"2026-01-02T00:00:00Z","metadata":{"container":{"tags":["v2"]}}}
		]`
	case "3":
		payload = `[{"id":101,"created_at":"2026-01-01T00:00:00Z","metadata":{"container":{"tags":[]}}}]`
	case "4":
		payload = `[]`
	default:
		testingInstance.Fatalf("unexpected page %s", pageValue)
	}

	responseWriter.Header().Set("Content-Type", "application/json")
	_, _ = responseWriter.Write([]byte(payload))
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
