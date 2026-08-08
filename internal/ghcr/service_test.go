package ghcr_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/tyemirov/gix/v5/internal/ghcr"
)

const (
	testOwnerNameConstant        = "test-owner"
	testPackageNameConstant      = "test-package"
	testTokenValueConstant       = "test-token"
	errorMessageTemplateConstant = "request %d not configured"
)

type stubHTTPClient struct {
	responses        []stubHTTPResponse
	recordedRequests []*http.Request
}

type stubHTTPResponse struct {
	response *http.Response
	err      error
}

func (client *stubHTTPClient) Do(request *http.Request) (*http.Response, error) {
	client.recordedRequests = append(client.recordedRequests, request)
	if len(client.responses) == 0 {
		return nil, fmt.Errorf(errorMessageTemplateConstant, len(client.recordedRequests))
	}

	next := client.responses[0]
	client.responses = client.responses[1:]

	if next.err != nil {
		return nil, next.err
	}

	next.response.Request = request
	return next.response, nil
}

func TestKeepCountRequiresPositiveValue(testingInstance *testing.T) {
	_, zeroError := ghcr.NewKeepCount(0)
	require.EqualError(testingInstance, zeroError, "keep count must be greater than zero")

	_, negativeError := ghcr.NewKeepCount(-1)
	require.EqualError(testingInstance, negativeError, "keep count must be greater than zero")

	keepCount, keepCountError := ghcr.NewKeepCount(3)
	require.NoError(testingInstance, keepCountError)
	require.Equal(testingInstance, 3, keepCount.Value())
}

func TestPackageVersionServiceInputValidation(testingInstance *testing.T) {
	httpClient := &stubHTTPClient{}
	service, serviceError := ghcr.NewPackageVersionService(zap.NewNop(), httpClient, ghcr.ServiceConfiguration{})
	require.NoError(testingInstance, serviceError)
	keepCount := mustKeepCount(testingInstance, 3)

	testCases := []struct {
		name          string
		request       ghcr.RetentionRequest
		expectedError string
	}{
		{
			name: "missing_token",
			request: ghcr.RetentionRequest{
				Owner:       testOwnerNameConstant,
				PackageName: testPackageNameConstant,
				OwnerType:   ghcr.UserOwnerType,
				Keep:        keepCount,
			},
			expectedError: "authentication token must be provided",
		},
		{
			name: "missing_owner",
			request: ghcr.RetentionRequest{
				Token:       testTokenValueConstant,
				PackageName: testPackageNameConstant,
				OwnerType:   ghcr.UserOwnerType,
				Keep:        keepCount,
			},
			expectedError: "owner must be provided",
		},
		{
			name: "missing_package",
			request: ghcr.RetentionRequest{
				Owner:     testOwnerNameConstant,
				Token:     testTokenValueConstant,
				OwnerType: ghcr.UserOwnerType,
				Keep:      keepCount,
			},
			expectedError: "package name must be provided",
		},
		{
			name: "missing_owner_type",
			request: ghcr.RetentionRequest{
				Owner:       testOwnerNameConstant,
				Token:       testTokenValueConstant,
				PackageName: testPackageNameConstant,
				Keep:        keepCount,
			},
			expectedError: "owner type must be provided",
		},
		{
			name: "invalid_owner_type",
			request: ghcr.RetentionRequest{
				Owner:       testOwnerNameConstant,
				Token:       testTokenValueConstant,
				PackageName: testPackageNameConstant,
				OwnerType:   ghcr.OwnerType("team"),
				Keep:        keepCount,
			},
			expectedError: "owner type \"team\" is not supported",
		},
		{
			name: "invalid_keep_count",
			request: ghcr.RetentionRequest{
				Owner:       testOwnerNameConstant,
				Token:       testTokenValueConstant,
				PackageName: testPackageNameConstant,
				OwnerType:   ghcr.UserOwnerType,
			},
			expectedError: "keep count must be greater than zero",
		},
	}

	for index := range testCases {
		testCase := testCases[index]
		testingInstance.Run(testCase.name, func(testingSubInstance *testing.T) {
			_, retentionError := service.ApplyRetention(context.Background(), testCase.request)
			require.ErrorContains(testingSubInstance, retentionError, testCase.expectedError)
		})
	}

	require.Empty(testingInstance, httpClient.recordedRequests)
}

func TestPackageVersionServiceHandlesListFailures(testingInstance *testing.T) {
	testCases := []struct {
		name          string
		client        *stubHTTPClient
		expectedError string
	}{
		{
			name: "network_error",
			client: &stubHTTPClient{
				responses: []stubHTTPResponse{{err: errors.New("network error")}},
			},
			expectedError: "request execution failed",
		},
		{
			name: "unexpected_status",
			client: &stubHTTPClient{
				responses: []stubHTTPResponse{{response: buildHTTPResponse(http.StatusInternalServerError, "failure")}},
			},
			expectedError: "unexpected status code 500",
		},
	}

	for index := range testCases {
		testCase := testCases[index]
		testingInstance.Run(testCase.name, func(testingSubInstance *testing.T) {
			service, serviceError := ghcr.NewPackageVersionService(zap.NewNop(), testCase.client, ghcr.ServiceConfiguration{})
			require.NoError(testingSubInstance, serviceError)

			_, retentionError := service.ApplyRetention(context.Background(), validRetentionRequest(testingSubInstance, 1))
			require.ErrorContains(testingSubInstance, retentionError, testCase.expectedError)
		})
	}
}

func TestPackageVersionServiceSnapshotsThenDeletesOldestVersions(testingInstance *testing.T) {
	pageOneVersions := `[
		{"id":400,"created_at":"2026-01-04T00:00:00Z","metadata":{"container":{"tags":["latest"]}}},
		{"id":100,"created_at":"2026-01-01T00:00:00Z","metadata":{"container":{"tags":[]}}}
	]`
	pageTwoVersions := `[
		{"id":300,"created_at":"2026-01-03T00:00:00Z","metadata":{"container":{"tags":["v3"]}}},
		{"id":200,"created_at":"2026-01-02T00:00:00Z","metadata":{"container":{"tags":[]}}}
	]`
	client := &stubHTTPClient{
		responses: []stubHTTPResponse{
			{response: buildHTTPResponse(http.StatusOK, pageOneVersions)},
			{response: buildHTTPResponse(http.StatusOK, pageTwoVersions)},
			{response: buildHTTPResponse(http.StatusOK, "[]")},
			{response: buildHTTPResponse(http.StatusNoContent, "")},
			{response: buildHTTPResponse(http.StatusAccepted, "")},
		},
	}

	service, serviceError := ghcr.NewPackageVersionService(zap.NewNop(), client, ghcr.ServiceConfiguration{PageSize: 2})
	require.NoError(testingInstance, serviceError)

	result, retentionError := service.ApplyRetention(context.Background(), validRetentionRequest(testingInstance, 2))
	require.NoError(testingInstance, retentionError)
	require.Equal(testingInstance, ghcr.RetentionResult{TotalVersions: 4, RetainedVersions: 2, DeletedVersions: 2}, result)

	require.Len(testingInstance, client.recordedRequests, 5)
	require.Equal(testingInstance, []string{
		http.MethodGet,
		http.MethodGet,
		http.MethodGet,
		http.MethodDelete,
		http.MethodDelete,
	}, recordedMethods(client.recordedRequests))
	require.True(testingInstance, strings.HasSuffix(client.recordedRequests[3].URL.Path, "/versions/100"))
	require.True(testingInstance, strings.HasSuffix(client.recordedRequests[4].URL.Path, "/versions/200"))
}

func TestPackageVersionServiceUsesHigherIdentifierAsNewestTimestampTieBreaker(testingInstance *testing.T) {
	client := &stubHTTPClient{
		responses: []stubHTTPResponse{
			{response: buildHTTPResponse(http.StatusOK, `[
				{"id":300,"created_at":"2026-01-03T00:00:00Z"},
				{"id":400,"created_at":"2026-01-03T00:00:00Z"},
				{"id":200,"created_at":"2026-01-02T00:00:00Z"}
			]`)},
			{response: buildHTTPResponse(http.StatusOK, "[]")},
			{response: buildHTTPResponse(http.StatusNoContent, "")},
			{response: buildHTTPResponse(http.StatusNoContent, "")},
		},
	}
	service, serviceError := ghcr.NewPackageVersionService(zap.NewNop(), client, ghcr.ServiceConfiguration{})
	require.NoError(testingInstance, serviceError)

	result, retentionError := service.ApplyRetention(context.Background(), validRetentionRequest(testingInstance, 1))
	require.NoError(testingInstance, retentionError)
	require.Equal(testingInstance, ghcr.RetentionResult{TotalVersions: 3, RetainedVersions: 1, DeletedVersions: 2}, result)
	require.True(testingInstance, strings.HasSuffix(client.recordedRequests[2].URL.Path, "/versions/200"))
	require.True(testingInstance, strings.HasSuffix(client.recordedRequests[3].URL.Path, "/versions/300"))
}

func TestPackageVersionServiceKeepsAllVersionsWhenCountExceedsTotal(testingInstance *testing.T) {
	client := &stubHTTPClient{
		responses: []stubHTTPResponse{
			{response: buildHTTPResponse(http.StatusOK, `[
				{"id":200,"created_at":"2026-01-02T00:00:00Z"},
				{"id":100,"created_at":"2026-01-01T00:00:00Z"}
			]`)},
			{response: buildHTTPResponse(http.StatusOK, "[]")},
		},
	}
	service, serviceError := ghcr.NewPackageVersionService(zap.NewNop(), client, ghcr.ServiceConfiguration{})
	require.NoError(testingInstance, serviceError)

	result, retentionError := service.ApplyRetention(context.Background(), validRetentionRequest(testingInstance, 3))
	require.NoError(testingInstance, retentionError)
	require.Equal(testingInstance, ghcr.RetentionResult{TotalVersions: 2, RetainedVersions: 2}, result)
	require.Equal(testingInstance, []string{http.MethodGet, http.MethodGet}, recordedMethods(client.recordedRequests))
}

func TestPackageVersionServiceValidatesCompleteSnapshotBeforeDeletion(testingInstance *testing.T) {
	testCases := []struct {
		name          string
		payload       string
		expectedError string
	}{
		{
			name:          "invalid_identifier",
			payload:       `[{"id":0,"created_at":"2026-01-01T00:00:00Z"}]`,
			expectedError: "package version id must be greater than zero",
		},
		{
			name:          "missing_created_at",
			payload:       `[{"id":100}]`,
			expectedError: "package version 100 must include created_at",
		},
		{
			name:          "duplicate_identifier",
			payload:       `[{"id":100,"created_at":"2026-01-02T00:00:00Z"},{"id":100,"created_at":"2026-01-01T00:00:00Z"}]`,
			expectedError: "package version id 100 appears more than once",
		},
		{
			name:          "malformed_created_at",
			payload:       `[{"id":100,"created_at":"yesterday"}]`,
			expectedError: "unable to decode package versions",
		},
	}

	for index := range testCases {
		testCase := testCases[index]
		testingInstance.Run(testCase.name, func(testingSubInstance *testing.T) {
			client := &stubHTTPClient{
				responses: []stubHTTPResponse{
					{response: buildHTTPResponse(http.StatusOK, testCase.payload)},
					{response: buildHTTPResponse(http.StatusOK, "[]")},
				},
			}
			service, serviceError := ghcr.NewPackageVersionService(zap.NewNop(), client, ghcr.ServiceConfiguration{})
			require.NoError(testingSubInstance, serviceError)

			_, retentionError := service.ApplyRetention(context.Background(), validRetentionRequest(testingSubInstance, 1))
			require.ErrorContains(testingSubInstance, retentionError, testCase.expectedError)
			methods := recordedMethods(client.recordedRequests)
			require.NotEmpty(testingSubInstance, methods)
			require.NotContains(testingSubInstance, methods, http.MethodDelete)
		})
	}
}

func TestPackageVersionServiceDoesNotDeleteWhenPaginationFails(testingInstance *testing.T) {
	client := &stubHTTPClient{
		responses: []stubHTTPResponse{
			{response: buildHTTPResponse(http.StatusOK, `[{"id":200,"created_at":"2026-01-02T00:00:00Z"}]`)},
			{response: buildHTTPResponse(http.StatusInternalServerError, "page failed")},
		},
	}
	service, serviceError := ghcr.NewPackageVersionService(zap.NewNop(), client, ghcr.ServiceConfiguration{PageSize: 1})
	require.NoError(testingInstance, serviceError)

	_, retentionError := service.ApplyRetention(context.Background(), validRetentionRequest(testingInstance, 1))
	require.ErrorContains(testingInstance, retentionError, "unexpected status code 500")
	require.Equal(testingInstance, []string{http.MethodGet, http.MethodGet}, recordedMethods(client.recordedRequests))
}

func TestPackageVersionServiceReturnsPartialDeletionResult(testingInstance *testing.T) {
	client := &stubHTTPClient{
		responses: []stubHTTPResponse{
			{response: buildHTTPResponse(http.StatusOK, `[
				{"id":300,"created_at":"2026-01-03T00:00:00Z"},
				{"id":200,"created_at":"2026-01-02T00:00:00Z"},
				{"id":100,"created_at":"2026-01-01T00:00:00Z"}
			]`)},
			{response: buildHTTPResponse(http.StatusOK, "[]")},
			{response: buildHTTPResponse(http.StatusNoContent, "")},
			{response: buildHTTPResponse(http.StatusInternalServerError, "delete failed")},
		},
	}
	service, serviceError := ghcr.NewPackageVersionService(zap.NewNop(), client, ghcr.ServiceConfiguration{})
	require.NoError(testingInstance, serviceError)

	result, retentionError := service.ApplyRetention(context.Background(), validRetentionRequest(testingInstance, 1))
	require.ErrorContains(testingInstance, retentionError, "failed to delete version 200")
	require.Equal(testingInstance, ghcr.RetentionResult{TotalVersions: 3, RetainedVersions: 1, DeletedVersions: 1}, result)
}

func mustKeepCount(testingInstance *testing.T, value int) ghcr.KeepCount {
	testingInstance.Helper()
	keepCount, keepCountError := ghcr.NewKeepCount(value)
	require.NoError(testingInstance, keepCountError)
	return keepCount
}

func validRetentionRequest(testingInstance *testing.T, keep int) ghcr.RetentionRequest {
	testingInstance.Helper()
	return ghcr.RetentionRequest{
		Owner:       testOwnerNameConstant,
		PackageName: testPackageNameConstant,
		OwnerType:   ghcr.UserOwnerType,
		Token:       testTokenValueConstant,
		Keep:        mustKeepCount(testingInstance, keep),
	}
}

func recordedMethods(requests []*http.Request) []string {
	methods := make([]string, 0, len(requests))
	for requestIndex := range requests {
		methods = append(methods, requests[requestIndex].Method)
	}
	return methods
}

func buildHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
