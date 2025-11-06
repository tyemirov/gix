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

	"github.com/temirov/gix/internal/ghcr"
)

const (
	testOwnerNameConstant        = "test-owner"
	testPackageNameConstant      = "test-package"
	testTokenValueConstant       = "test-token"
	testUntaggedVersionID        = int64(1001)
	testTaggedVersionID          = int64(1002)
	errorMessageTemplateConstant = "request %d not configured"
)

type stubHTTPClient struct {
	responses       []stubHTTPResponse
	recordedMethods []string
}

type stubHTTPResponse struct {
	response *http.Response
	err      error
}

func (client *stubHTTPClient) Do(request *http.Request) (*http.Response, error) {
	client.recordedMethods = append(client.recordedMethods, request.Method)
	if len(client.responses) == 0 {
		return nil, fmt.Errorf(errorMessageTemplateConstant, len(client.recordedMethods))
	}

	next := client.responses[0]
	client.responses = client.responses[1:]

	if next.err != nil {
		return nil, next.err
	}

	next.response.Request = request
	return next.response, nil
}

func TestPackageVersionServiceInputValidation(testingInstance *testing.T) {
	testingInstance.Parallel()

	httpClient := &stubHTTPClient{}
	service, serviceError := ghcr.NewPackageVersionService(zap.NewNop(), httpClient, ghcr.ServiceConfiguration{})
	require.NoError(testingInstance, serviceError)

	testCases := []struct {
		name          string
		request       ghcr.PurgeRequest
		expectedError string
	}{
		{
			name: "missing_token",
			request: ghcr.PurgeRequest{
				Owner:       testOwnerNameConstant,
				PackageName: testPackageNameConstant,
				OwnerType:   ghcr.UserOwnerType,
			},
			expectedError: "authentication token must be provided",
		},
		{
			name: "missing_owner",
			request: ghcr.PurgeRequest{
				Token:       testTokenValueConstant,
				PackageName: testPackageNameConstant,
				OwnerType:   ghcr.UserOwnerType,
			},
			expectedError: "owner must be provided",
		},
		{
			name: "missing_package",
			request: ghcr.PurgeRequest{
				Owner:     testOwnerNameConstant,
				Token:     testTokenValueConstant,
				OwnerType: ghcr.UserOwnerType,
			},
			expectedError: "package name must be provided",
		},
		{
			name: "missing_owner_type",
			request: ghcr.PurgeRequest{
				Owner:       testOwnerNameConstant,
				Token:       testTokenValueConstant,
				PackageName: testPackageNameConstant,
			},
			expectedError: "owner type must be provided",
		},
	}

	for index := range testCases {
		testCase := testCases[index]

		testingInstance.Run(testCase.name, func(testingSubInstance *testing.T) {
			testingSubInstance.Parallel()

			_, purgeError := service.PurgeUntaggedVersions(context.Background(), testCase.request)
			require.Error(testingSubInstance, purgeError)
			require.ErrorContains(testingSubInstance, purgeError, testCase.expectedError)
		})
	}
}

func TestPackageVersionServiceHandlesHTTPFailures(testingInstance *testing.T) {
	testingInstance.Parallel()

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
			testingSubInstance.Parallel()

			service, serviceError := ghcr.NewPackageVersionService(zap.NewNop(), testCase.client, ghcr.ServiceConfiguration{})
			require.NoError(testingSubInstance, serviceError)

			_, purgeError := service.PurgeUntaggedVersions(context.Background(), ghcr.PurgeRequest{
				Owner:       testOwnerNameConstant,
				PackageName: testPackageNameConstant,
				OwnerType:   ghcr.UserOwnerType,
				Token:       testTokenValueConstant,
			})
			require.Error(testingSubInstance, purgeError)
			require.ErrorContains(testingSubInstance, purgeError, testCase.expectedError)
		})
	}
}

func TestPackageVersionServiceDeletesUntaggedVersions(testingInstance *testing.T) {
	testingInstance.Parallel()

	pageOneVersions := fmt.Sprintf(`[{"id":%d,"metadata":{"container":{"tags":[]}}}]`, testUntaggedVersionID)
	emptyPage := "[]"

	client := &stubHTTPClient{
		responses: []stubHTTPResponse{
			{response: buildHTTPResponse(http.StatusOK, pageOneVersions)},
			{response: buildHTTPResponse(http.StatusNoContent, "")},
			{response: buildHTTPResponse(http.StatusOK, emptyPage)},
		},
	}

	service, serviceError := ghcr.NewPackageVersionService(zap.NewNop(), client, ghcr.ServiceConfiguration{PageSize: 1})
	require.NoError(testingInstance, serviceError)

	result, purgeError := service.PurgeUntaggedVersions(context.Background(), ghcr.PurgeRequest{
		Owner:       testOwnerNameConstant,
		PackageName: testPackageNameConstant,
		OwnerType:   ghcr.UserOwnerType,
		Token:       testTokenValueConstant,
	})
	require.NoError(testingInstance, purgeError)
	require.Equal(testingInstance, 1, result.TotalVersions)
	require.Equal(testingInstance, 1, result.UntaggedVersions)
	require.Equal(testingInstance, 1, result.DeletedVersions)
	require.Equal(testingInstance, []string{http.MethodGet, http.MethodDelete, http.MethodGet}, client.recordedMethods)
}

func buildHTTPResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}
