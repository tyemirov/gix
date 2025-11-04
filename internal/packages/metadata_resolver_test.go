package packages_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/ghcr"
	"github.com/tyemirov/gix/internal/githubcli"
	packages "github.com/tyemirov/gix/internal/packages"
)

const (
	metadataResolverPrimaryRepositoryPathConstant      = "/repositories/example"
	metadataResolverAlternateRepositoryPathConstant    = "/repositories/example-two"
	metadataResolverPrimaryRemoteURLConstant           = "https://github.com/source/example.git"
	metadataResolverAlternateRemoteURLConstant         = "https://github.com/source/example-two.git"
	metadataResolverPrimaryRemoteOwnerConstant         = "source"
	metadataResolverPrimaryPackageNameConstant         = "example"
	metadataResolverAlternatePackageNameConstant       = "example-two"
	metadataResolverMetadataOwnerConstant              = "metadata-owner"
	metadataResolverMetadataNameWithOwnerConstant      = metadataResolverMetadataOwnerConstant + "/" + metadataResolverPrimaryPackageNameConstant
	metadataResolverAlternateMetadataOwnerConstant     = "alternate-owner"
	metadataResolverAlternateNameWithOwnerConstant     = metadataResolverAlternateMetadataOwnerConstant + "/" + metadataResolverAlternatePackageNameConstant
	metadataResolverInvalidNameWithOwnerConstant       = "invalid"
	metadataResolverInvalidRemoteURLConstant           = "https://github.com/source/.git"
	metadataResolverPathMissingErrorMessageConstant    = "repository path not provided"
	metadataResolverManagerMissingErrorMessageConstant = "repository manager must be provided"
	metadataResolverGitHubMissingErrorMessageConstant  = "github metadata resolver must be provided"
	metadataResolverOriginResolutionErrorIndicator     = "unable to resolve origin remote"
	metadataResolverOriginParseErrorIndicator          = "unable to parse origin remote"
	metadataResolverMetadataErrorIndicator             = "unable to resolve repository metadata"
	metadataResolverMetadataOwnerMissingIndicator      = "repository metadata did not include owner"
)

func TestDefaultRepositoryMetadataResolverResolvesMetadata(testInstance *testing.T) {
	testInstance.Parallel()

	testCases := []struct {
		name                string
		repositoryPath      string
		remoteURL           string
		metadata            githubcli.RepositoryMetadata
		expectedOwner       string
		expectedOwnerType   ghcr.OwnerType
		expectedPackageName string
	}{
		{
			name:           "metadata_owner_overrides_remote",
			repositoryPath: metadataResolverPrimaryRepositoryPathConstant,
			remoteURL:      metadataResolverPrimaryRemoteURLConstant,
			metadata: githubcli.RepositoryMetadata{
				NameWithOwner:    metadataResolverMetadataNameWithOwnerConstant,
				IsInOrganization: true,
			},
			expectedOwner:       metadataResolverMetadataOwnerConstant,
			expectedOwnerType:   ghcr.OrganizationOwnerType,
			expectedPackageName: metadataResolverPrimaryPackageNameConstant,
		},
		{
			name:           "falls_back_to_remote_owner_when_metadata_missing",
			repositoryPath: metadataResolverPrimaryRepositoryPathConstant,
			remoteURL:      metadataResolverPrimaryRemoteURLConstant,
			metadata: githubcli.RepositoryMetadata{
				NameWithOwner:    "",
				IsInOrganization: false,
			},
			expectedOwner:       metadataResolverPrimaryRemoteOwnerConstant,
			expectedOwnerType:   ghcr.UserOwnerType,
			expectedPackageName: metadataResolverPrimaryPackageNameConstant,
		},
		{
			name:           "supports_multiple_repositories",
			repositoryPath: metadataResolverAlternateRepositoryPathConstant,
			remoteURL:      metadataResolverAlternateRemoteURLConstant,
			metadata: githubcli.RepositoryMetadata{
				NameWithOwner:    metadataResolverAlternateNameWithOwnerConstant,
				IsInOrganization: true,
			},
			expectedOwner:       metadataResolverAlternateMetadataOwnerConstant,
			expectedOwnerType:   ghcr.OrganizationOwnerType,
			expectedPackageName: metadataResolverAlternatePackageNameConstant,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		testInstance.Run(testCase.name, func(subTest *testing.T) {
			subTest.Parallel()

			repositoryManager := &stubRepositoryManager{remoteURLByPath: map[string]string{
				testCase.repositoryPath: testCase.remoteURL,
			}}
			githubResolver := &stubGitHubResolver{metadata: testCase.metadata}

			resolver := packages.DefaultRepositoryMetadataResolver{
				RepositoryManager: repositoryManager,
				GitHubResolver:    githubResolver,
			}

			resolvedMetadata, resolutionError := resolver.ResolveMetadata(
				context.Background(),
				testCase.repositoryPath,
			)
			require.NoError(subTest, resolutionError)
			require.Equal(subTest, testCase.expectedOwner, resolvedMetadata.Owner)
			require.Equal(subTest, testCase.expectedOwnerType, resolvedMetadata.OwnerType)
			require.Equal(subTest, testCase.expectedPackageName, resolvedMetadata.DefaultPackageName)
		})
	}
}

func TestDefaultRepositoryMetadataResolverPropagatesErrors(testInstance *testing.T) {
	testInstance.Parallel()

	baseRepositoryManager := &stubRepositoryManager{remoteURLByPath: map[string]string{
		metadataResolverPrimaryRepositoryPathConstant: metadataResolverPrimaryRemoteURLConstant,
	}}
	baseGitHubResolver := &stubGitHubResolver{metadata: githubcli.RepositoryMetadata{NameWithOwner: metadataResolverMetadataNameWithOwnerConstant}}

	testCases := []struct {
		name           string
		repositoryPath string
		manager        *stubRepositoryManager
		githubResolver *stubGitHubResolver
		expectedError  string
	}{
		{
			name:           "missing_repository_path",
			repositoryPath: " ",
			manager:        baseRepositoryManager,
			githubResolver: baseGitHubResolver,
			expectedError:  metadataResolverPathMissingErrorMessageConstant,
		},
		{
			name:           "missing_repository_manager",
			repositoryPath: metadataResolverPrimaryRepositoryPathConstant,
			manager:        nil,
			githubResolver: baseGitHubResolver,
			expectedError:  metadataResolverManagerMissingErrorMessageConstant,
		},
		{
			name:           "missing_github_resolver",
			repositoryPath: metadataResolverPrimaryRepositoryPathConstant,
			manager:        baseRepositoryManager,
			githubResolver: nil,
			expectedError:  metadataResolverGitHubMissingErrorMessageConstant,
		},
		{
			name:           "remote_resolution_error",
			repositoryPath: metadataResolverPrimaryRepositoryPathConstant,
			manager: &stubRepositoryManager{
				errorByPath: map[string]error{metadataResolverPrimaryRepositoryPathConstant: errors.New("unable to locate remote")},
			},
			githubResolver: baseGitHubResolver,
			expectedError:  metadataResolverOriginResolutionErrorIndicator,
		},
		{
			name:           "remote_parse_error",
			repositoryPath: metadataResolverPrimaryRepositoryPathConstant,
			manager: &stubRepositoryManager{remoteURLByPath: map[string]string{
				metadataResolverPrimaryRepositoryPathConstant: metadataResolverInvalidRemoteURLConstant,
			}},
			githubResolver: baseGitHubResolver,
			expectedError:  metadataResolverOriginParseErrorIndicator,
		},
		{
			name:           "metadata_resolution_error",
			repositoryPath: metadataResolverPrimaryRepositoryPathConstant,
			manager:        baseRepositoryManager,
			githubResolver: &stubGitHubResolver{err: errors.New("gh error")},
			expectedError:  metadataResolverMetadataErrorIndicator,
		},
		{
			name:           "metadata_owner_missing",
			repositoryPath: metadataResolverPrimaryRepositoryPathConstant,
			manager:        baseRepositoryManager,
			githubResolver: &stubGitHubResolver{metadata: githubcli.RepositoryMetadata{NameWithOwner: metadataResolverInvalidNameWithOwnerConstant}},
			expectedError:  metadataResolverMetadataOwnerMissingIndicator,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		testInstance.Run(testCase.name, func(subTest *testing.T) {
			subTest.Parallel()

			resolver := packages.DefaultRepositoryMetadataResolver{}
			if testCase.manager != nil {
				resolver.RepositoryManager = testCase.manager
			}
			if testCase.githubResolver != nil {
				resolver.GitHubResolver = testCase.githubResolver
			}

			_, resolutionError := resolver.ResolveMetadata(context.Background(), testCase.repositoryPath)
			require.Error(subTest, resolutionError)
			require.ErrorContains(subTest, resolutionError, testCase.expectedError)
		})
	}
}

type stubRepositoryManager struct {
	remoteURLByPath map[string]string
	errorByPath     map[string]error
}

func (manager *stubRepositoryManager) CheckCleanWorktree(context.Context, string) (bool, error) {
	return true, nil
}

func (manager *stubRepositoryManager) GetCurrentBranch(context.Context, string) (string, error) {
	return "main", nil
}

func (manager *stubRepositoryManager) GetRemoteURL(_ context.Context, repositoryPath string, _ string) (string, error) {
	if manager.errorByPath != nil {
		if err, exists := manager.errorByPath[repositoryPath]; exists {
			return "", err
		}
	}
	if manager.remoteURLByPath == nil {
		return "", errors.New("remote not configured")
	}
	if remote, exists := manager.remoteURLByPath[repositoryPath]; exists {
		return remote, nil
	}
	return "", errors.New("remote not configured")
}

func (manager *stubRepositoryManager) SetRemoteURL(context.Context, string, string, string) error {
	return nil
}

type stubGitHubResolver struct {
	metadata       githubcli.RepositoryMetadata
	metadataByRepo map[string]githubcli.RepositoryMetadata
	err            error
}

func (resolver *stubGitHubResolver) ResolveRepoMetadata(_ context.Context, repository string) (githubcli.RepositoryMetadata, error) {
	if resolver.metadataByRepo != nil {
		if metadata, exists := resolver.metadataByRepo[repository]; exists {
			return metadata, resolver.err
		}
	}
	return resolver.metadata, resolver.err
}
