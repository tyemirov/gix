package packages_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/tyemirov/gix/internal/ghcr"
	packages "github.com/tyemirov/gix/internal/packages"
)

func TestPurgeServiceValidatesOptions(testingInstance *testing.T) {
	testingInstance.Parallel()

	packageService := &stubPackageVersionAPI{}
	service, serviceError := packages.NewPurgeService(zap.NewNop(), packageService)
	require.NoError(testingInstance, serviceError)

	testCases := []struct {
		name          string
		options       packages.PurgeOptions
		expectedError string
	}{
		{
			name:          "missing_owner",
			options:       packages.PurgeOptions{PackageName: "package", OwnerType: ghcr.UserOwnerType, Credential: "token"},
			expectedError: "owner option must be provided",
		},
		{
			name:          "missing_package",
			options:       packages.PurgeOptions{Owner: "owner", OwnerType: ghcr.UserOwnerType, Credential: "token"},
			expectedError: "package option must be provided",
		},
		{
			name:          "missing_owner_type",
			options:       packages.PurgeOptions{Owner: "owner", PackageName: "package", Credential: "token"},
			expectedError: "owner type option must be provided",
		},
		{
			name:          "missing_credential",
			options:       packages.PurgeOptions{Owner: "owner", PackageName: "package", OwnerType: ghcr.UserOwnerType},
			expectedError: "packages credential must be provided",
		},
	}

	for index := range testCases {
		testCase := testCases[index]

		testingInstance.Run(testCase.name, func(testingSubInstance *testing.T) {
			testingSubInstance.Parallel()

			_, executionError := service.Execute(context.Background(), testCase.options)
			require.Error(testingSubInstance, executionError)
			require.ErrorContains(testingSubInstance, executionError, testCase.expectedError)
		})
	}
}

func TestPurgeServiceInvokesPackageService(testingInstance *testing.T) {
	testingInstance.Parallel()

	observedCore, observedLogs := observer.New(zap.DebugLevel)
	logger := zap.New(observedCore)

	packageService := &stubPackageVersionAPI{
		result: ghcr.PurgeResult{TotalVersions: 10, UntaggedVersions: 3, DeletedVersions: 2},
	}
	service, serviceError := packages.NewPurgeService(logger, packageService)
	require.NoError(testingInstance, serviceError)

	options := packages.PurgeOptions{
		Owner:       "owner",
		PackageName: "package",
		OwnerType:   ghcr.OrganizationOwnerType,
		Credential:  "configured-token",
	}

	result, executionError := service.Execute(context.Background(), options)
	require.NoError(testingInstance, executionError)
	require.Equal(testingInstance, packageService.result, result)
	require.True(testingInstance, packageService.called)
	require.Equal(testingInstance, options.Owner, packageService.request.Owner)
	require.Equal(testingInstance, options.PackageName, packageService.request.PackageName)
	require.Equal(testingInstance, options.OwnerType, packageService.request.OwnerType)
	require.Equal(testingInstance, options.Credential, packageService.request.Token)

	infoLogs := observedLogs.FilterLevelExact(zap.InfoLevel)
	require.GreaterOrEqual(testingInstance, infoLogs.Len(), 2)
}

type stubPackageVersionAPI struct {
	request ghcr.PurgeRequest
	result  ghcr.PurgeResult
	err     error
	called  bool
}

func (service *stubPackageVersionAPI) PurgeUntaggedVersions(executionContext context.Context, request ghcr.PurgeRequest) (ghcr.PurgeResult, error) {
	service.called = true
	service.request = request
	if service.err != nil {
		return ghcr.PurgeResult{}, service.err
	}
	return service.result, nil
}
