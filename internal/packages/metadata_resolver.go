package packages

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tyemirov/gix/internal/ghcr"
	"github.com/tyemirov/gix/internal/gitrepo"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	repositoryPathEmptyErrorMessageConstant            = "repository path not provided"
	repositoryManagerMissingErrorMessageConstant       = "repository manager must be provided"
	gitHubResolverMissingErrorMessageConstant          = "github metadata resolver must be provided"
	originRemoteResolutionErrorTemplateConstant        = "unable to resolve origin remote: %w"
	originRemoteParseErrorTemplateConstant             = "unable to parse origin remote: %w"
	originRemoteOwnerMissingErrorMessageConstant       = "origin remote did not include owner information"
	repositoryMetadataResolutionErrorTemplateConstant  = "unable to resolve repository metadata: %w"
	repositoryMetadataOwnerMissingErrorMessageConstant = "repository metadata did not include owner"
	repositoryIdentifierFormatTemplateConstant         = "%s/%s"
	repositoryNameMissingErrorMessageConstant          = "origin remote did not include repository name"
)

// RepositoryMetadata captures canonical owner and package information for a repository.
type RepositoryMetadata struct {
	Owner              string
	OwnerType          ghcr.OwnerType
	DefaultPackageName string
}

// RepositoryMetadataResolver resolves repository metadata used by the purge command.
type RepositoryMetadataResolver interface {
	ResolveMetadata(executionContext context.Context, repositoryPath string) (RepositoryMetadata, error)
}

// DefaultRepositoryMetadataResolver resolves metadata using git remotes and GitHub CLI metadata.
type DefaultRepositoryMetadataResolver struct {
	RepositoryManager shared.GitRepositoryManager
	GitHubResolver    shared.GitHubMetadataResolver
}

// ResolveMetadata extracts the owner, owner type, and default package name for a repository.
func (resolver *DefaultRepositoryMetadataResolver) ResolveMetadata(
	executionContext context.Context,
	repositoryPath string,
) (RepositoryMetadata, error) {
	trimmedRepositoryPath := strings.TrimSpace(repositoryPath)
	if len(trimmedRepositoryPath) == 0 {
		return RepositoryMetadata{}, errors.New(repositoryPathEmptyErrorMessageConstant)
	}

	if resolver.RepositoryManager == nil {
		return RepositoryMetadata{}, errors.New(repositoryManagerMissingErrorMessageConstant)
	}
	if resolver.GitHubResolver == nil {
		return RepositoryMetadata{}, errors.New(gitHubResolverMissingErrorMessageConstant)
	}

	originURL, originError := resolver.RepositoryManager.GetRemoteURL(
		executionContext,
		trimmedRepositoryPath,
		shared.OriginRemoteNameConstant,
	)
	if originError != nil {
		return RepositoryMetadata{}, fmt.Errorf(originRemoteResolutionErrorTemplateConstant, originError)
	}

	parsedRemote, parseError := gitrepo.ParseRemoteURL(originURL)
	if parseError != nil {
		return RepositoryMetadata{}, fmt.Errorf(originRemoteParseErrorTemplateConstant, parseError)
	}

	ownerCandidate := strings.TrimSpace(parsedRemote.Owner)
	if len(ownerCandidate) == 0 {
		return RepositoryMetadata{}, fmt.Errorf(
			originRemoteParseErrorTemplateConstant,
			errors.New(originRemoteOwnerMissingErrorMessageConstant),
		)
	}

	repositoryName := strings.TrimSpace(parsedRemote.Repository)
	if len(repositoryName) == 0 {
		return RepositoryMetadata{}, errors.New(repositoryNameMissingErrorMessageConstant)
	}

	repositoryIdentifier := fmt.Sprintf(
		repositoryIdentifierFormatTemplateConstant,
		parsedRemote.Owner,
		parsedRemote.Repository,
	)

	metadata, metadataError := resolver.GitHubResolver.ResolveRepoMetadata(
		executionContext,
		repositoryIdentifier,
	)
	if metadataError != nil {
		return RepositoryMetadata{}, fmt.Errorf(repositoryMetadataResolutionErrorTemplateConstant, metadataError)
	}

	resolvedOwner := ownerCandidate
	trimmedNameWithOwner := strings.TrimSpace(metadata.NameWithOwner)
	if len(trimmedNameWithOwner) > 0 {
		ownerFromMetadata, ownerParseError := parseOwnerFromNameWithOwner(trimmedNameWithOwner)
		if ownerParseError != nil {
			return RepositoryMetadata{}, fmt.Errorf(
				repositoryMetadataResolutionErrorTemplateConstant,
				ownerParseError,
			)
		}
		resolvedOwner = ownerFromMetadata
	}

	ownerType := ghcr.UserOwnerType
	if metadata.IsInOrganization {
		ownerType = ghcr.OrganizationOwnerType
	}

	return RepositoryMetadata{
		Owner:              resolvedOwner,
		OwnerType:          ownerType,
		DefaultPackageName: repositoryName,
	}, nil
}

func parseOwnerFromNameWithOwner(nameWithOwner string) (string, error) {
	components := strings.Split(nameWithOwner, ownerRepoSeparatorConstant)
	if len(components) < 2 {
		return "", errors.New(repositoryMetadataOwnerMissingErrorMessageConstant)
	}

	owner := strings.TrimSpace(components[0])
	if len(owner) == 0 {
		return "", errors.New(repositoryMetadataOwnerMissingErrorMessageConstant)
	}

	return owner, nil
}
