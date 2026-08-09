package packages_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/tyemirov/gix/internal/ghcr"
	packages "github.com/tyemirov/gix/internal/packages"
)

func TestRetentionServiceValidatesOptions(testingInstance *testing.T) {
	packageService := &stubPackageVersionAPI{}
	service, serviceError := packages.NewRetentionService(zap.NewNop(), packageService)
	require.NoError(testingInstance, serviceError)
	keepCount := mustPackageKeepCount(testingInstance, 3)

	testCases := []struct {
		name          string
		options       packages.RetentionOptions
		expectedError string
	}{
		{
			name:          "missing_owner",
			options:       packages.RetentionOptions{PackageName: "package", OwnerType: ghcr.UserOwnerType, Credential: "token", Keep: keepCount},
			expectedError: "owner option must be provided",
		},
		{
			name:          "missing_package",
			options:       packages.RetentionOptions{Owner: "owner", OwnerType: ghcr.UserOwnerType, Credential: "token", Keep: keepCount},
			expectedError: "package option must be provided",
		},
		{
			name:          "missing_owner_type",
			options:       packages.RetentionOptions{Owner: "owner", PackageName: "package", Credential: "token", Keep: keepCount},
			expectedError: "owner type option must be provided",
		},
		{
			name:          "invalid_owner_type",
			options:       packages.RetentionOptions{Owner: "owner", PackageName: "package", OwnerType: ghcr.OwnerType("team"), Credential: "token", Keep: keepCount},
			expectedError: "owner type \"team\" is not supported",
		},
		{
			name:          "missing_credential",
			options:       packages.RetentionOptions{Owner: "owner", PackageName: "package", OwnerType: ghcr.UserOwnerType, Keep: keepCount},
			expectedError: "packages credential must be provided",
		},
		{
			name:          "missing_keep_count",
			options:       packages.RetentionOptions{Owner: "owner", PackageName: "package", OwnerType: ghcr.UserOwnerType, Credential: "token"},
			expectedError: "keep count must be greater than zero",
		},
	}

	for index := range testCases {
		testCase := testCases[index]
		testingInstance.Run(testCase.name, func(testingSubInstance *testing.T) {
			_, executionError := service.Execute(context.Background(), testCase.options)
			require.ErrorContains(testingSubInstance, executionError, testCase.expectedError)
		})
	}
}

func TestRetentionServiceInvokesPackageService(testingInstance *testing.T) {
	observedCore, observedLogs := observer.New(zap.DebugLevel)
	logger := zap.New(observedCore)

	packageService := &stubPackageVersionAPI{
		result: ghcr.RetentionResult{TotalVersions: 10, RetainedVersions: 3, DeletedVersions: 7},
	}
	service, serviceError := packages.NewRetentionService(logger, packageService)
	require.NoError(testingInstance, serviceError)

	options := packages.RetentionOptions{
		Owner:       "owner",
		PackageName: "package",
		OwnerType:   ghcr.OrganizationOwnerType,
		Credential:  "configured-token",
		Keep:        mustPackageKeepCount(testingInstance, 3),
	}

	result, executionError := service.Execute(context.Background(), options)
	require.NoError(testingInstance, executionError)
	require.Equal(testingInstance, packageService.result, result)
	require.True(testingInstance, packageService.called)
	require.Equal(testingInstance, options.Owner, packageService.request.Owner)
	require.Equal(testingInstance, options.PackageName, packageService.request.PackageName)
	require.Equal(testingInstance, options.OwnerType, packageService.request.OwnerType)
	require.Equal(testingInstance, options.Credential, packageService.request.Token)
	require.Equal(testingInstance, options.Keep, packageService.request.Keep)

	infoLogs := observedLogs.FilterLevelExact(zap.InfoLevel)
	require.GreaterOrEqual(testingInstance, infoLogs.Len(), 2)
}

func TestRetentionServicePreservesPartialResultOnFailure(testingInstance *testing.T) {
	expectedResult := ghcr.RetentionResult{TotalVersions: 6, RetainedVersions: 3, DeletedVersions: 1}
	packageService := &stubPackageVersionAPI{
		result: expectedResult,
		err:    errors.New("delete failed"),
	}
	service, serviceError := packages.NewRetentionService(zap.NewNop(), packageService)
	require.NoError(testingInstance, serviceError)

	result, executionError := service.Execute(context.Background(), packages.RetentionOptions{
		Owner:       "owner",
		PackageName: "package",
		OwnerType:   ghcr.UserOwnerType,
		Credential:  "token",
		Keep:        mustPackageKeepCount(testingInstance, 3),
	})
	require.ErrorContains(testingInstance, executionError, "unable to apply package retention")
	require.Equal(testingInstance, expectedResult, result)
}

type stubPackageVersionAPI struct {
	request ghcr.RetentionRequest
	result  ghcr.RetentionResult
	err     error
	called  bool
}

func (service *stubPackageVersionAPI) ApplyRetention(_ context.Context, request ghcr.RetentionRequest) (ghcr.RetentionResult, error) {
	service.called = true
	service.request = request
	return service.result, service.err
}

func mustPackageKeepCount(testingInstance *testing.T, value int) ghcr.KeepCount {
	testingInstance.Helper()
	keepCount, keepCountError := ghcr.NewKeepCount(value)
	require.NoError(testingInstance, keepCountError)
	return keepCount
}
