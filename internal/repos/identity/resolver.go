package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubauth"
	"github.com/tyemirov/gix/internal/githubcli"
	"github.com/tyemirov/gix/internal/repos/shared"
)

const (
	queryArgumentNameConstant      = "query"
	searchTermArgumentNameConstant = "term"
	graphqlCommandNameConstant     = "graphql"
	apiCommandNameConstant         = "api"
	searchRepositoryQueryTemplate  = "query($term:String!){search(query:$term,type:REPOSITORY,first:10){nodes{... on Repository{name nameWithOwner defaultBranchRef{name}}}}}"
)

// RemoteResolutionDependencies provide collaborators required for remote introspection.
type RemoteResolutionDependencies struct {
	RepositoryManager shared.GitRepositoryManager
	GitExecutor       shared.GitExecutor
	MetadataResolver  shared.GitHubMetadataResolver
}

// RemoteResolutionOptions control remote introspection behaviour.
type RemoteResolutionOptions struct {
	RepositoryPath            string
	RemoteName                string
	ReportedOwnerRepository   string
	ReportedDefaultBranchName string
}

// RemoteResolutionResult exposes canonical remote metadata when available.
type RemoteResolutionResult struct {
	RemoteDetected  bool
	OwnerRepository *shared.OwnerRepository
	DefaultBranch   *shared.BranchName
}

// ResolveRemoteIdentity canonicalises repository owner/name and default branch for the configured remote.
func ResolveRemoteIdentity(
	executionContext context.Context,
	dependencies RemoteResolutionDependencies,
	options RemoteResolutionOptions,
) (RemoteResolutionResult, error) {
	if isNilInterface(dependencies.RepositoryManager) {
		return RemoteResolutionResult{RemoteDetected: false}, nil
	}

	trimmedRemoteName := strings.TrimSpace(options.RemoteName)
	if len(trimmedRemoteName) == 0 {
		return RemoteResolutionResult{}, errors.New("remote identity resolution requires remote name")
	}

	remoteURL, remoteURLError := dependencies.RepositoryManager.GetRemoteURL(executionContext, options.RepositoryPath, trimmedRemoteName)
	if remoteURLError != nil {
		return RemoteResolutionResult{RemoteDetected: false}, nil
	}

	ownerRepositoryCandidate, ownerRepositoryAvailable := ownerRepoFromRemoteURL(remoteURL)
	if !ownerRepositoryAvailable {
		candidate, parsed := parseOwnerRepository(options.ReportedOwnerRepository)
		if parsed {
			ownerRepositoryCandidate = candidate
			ownerRepositoryAvailable = true
		}
	}

	defaultBranchCandidate, defaultBranchAvailable := parseBranchName(options.ReportedDefaultBranchName)

	if dependencies.MetadataResolver != nil && ownerRepositoryAvailable {
		metadata, metadataError := dependencies.MetadataResolver.ResolveRepoMetadata(executionContext, ownerRepositoryCandidate.String())
		if metadataError != nil {
			metadata, metadataError = resolveRepoMetadataWithSearch(
				executionContext,
				dependencies.GitExecutor,
				ownerRepositoryCandidate.Repository().String(),
			)
			if metadataError != nil {
				return RemoteResolutionResult{
					RemoteDetected:  true,
					OwnerRepository: ownerRepositoryCandidate,
					DefaultBranch:   defaultBranchCandidate,
				}, nil
			}
		}

		if canonicalOwner, canonicalOwnerError := shared.NewOwnerRepository(metadata.NameWithOwner); canonicalOwnerError == nil {
			canonicalOwnerCopy := canonicalOwner
			ownerRepositoryCandidate = &canonicalOwnerCopy
			ownerRepositoryAvailable = true
		}

		if canonicalBranch, canonicalBranchAvailable := parseBranchName(metadata.DefaultBranch); canonicalBranchAvailable {
			defaultBranchCandidate = canonicalBranch
			defaultBranchAvailable = true
		}
	}

	if !ownerRepositoryAvailable {
		return RemoteResolutionResult{RemoteDetected: true}, nil
	}

	result := RemoteResolutionResult{
		RemoteDetected:  true,
		OwnerRepository: ownerRepositoryCandidate,
	}
	if defaultBranchAvailable {
		result.DefaultBranch = defaultBranchCandidate
	}
	return result, nil
}

func parseOwnerRepository(raw string) (*shared.OwnerRepository, bool) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, false
	}

	ownerRepository, parseError := shared.NewOwnerRepository(trimmed)
	if parseError != nil {
		return nil, false
	}
	return &ownerRepository, true
}

func parseBranchName(raw string) (*shared.BranchName, bool) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, false
	}
	branchName, branchError := shared.NewBranchName(trimmed)
	if branchError != nil {
		return nil, false
	}
	return &branchName, true
}

func resolveRepoMetadataWithSearch(
	executionContext context.Context,
	executor shared.GitExecutor,
	repositoryName string,
) (githubcli.RepositoryMetadata, error) {
	if isNilInterface(executor) {
		return githubcli.RepositoryMetadata{}, errors.New("search requires git executor for GitHub CLI")
	}

	searchTerm := fmt.Sprintf("%s in:name", strings.TrimSpace(repositoryName))

	commandDetails := execshell.CommandDetails{
		Arguments: []string{
			apiCommandNameConstant,
			graphqlCommandNameConstant,
			"-f", queryArgumentNameConstant + "=" + searchRepositoryQueryTemplate,
			"-F", searchTermArgumentNameConstant + "=" + searchTerm,
		},
		GitHubTokenRequirement: githubauth.TokenRequired,
	}

	executionResult, executionError := executor.ExecuteGitHubCLI(executionContext, commandDetails)
	if executionError != nil {
		return githubcli.RepositoryMetadata{}, executionError
	}

	var response struct {
		Data struct {
			Search struct {
				Nodes []struct {
					Name             string `json:"name"`
					NameWithOwner    string `json:"nameWithOwner"`
					DefaultBranchRef struct {
						Name string `json:"name"`
					} `json:"defaultBranchRef"`
				} `json:"nodes"`
			} `json:"search"`
		} `json:"data"`
	}

	if err := json.Unmarshal([]byte(executionResult.StandardOutput), &response); err != nil {
		return githubcli.RepositoryMetadata{}, err
	}

	for _, node := range response.Data.Search.Nodes {
		if strings.EqualFold(strings.TrimSpace(node.Name), repositoryName) && len(strings.TrimSpace(node.NameWithOwner)) > 0 {
			return githubcli.RepositoryMetadata{
				NameWithOwner: node.NameWithOwner,
				DefaultBranch: node.DefaultBranchRef.Name,
			}, nil
		}
	}

	return githubcli.RepositoryMetadata{}, errors.New("repository metadata search yielded no canonical match")
}

func ownerRepoFromRemoteURL(raw string) (*shared.OwnerRepository, bool) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, false
	}

	trimmed = strings.TrimSuffix(trimmed, ".git")
	if strings.Contains(trimmed, "://") {
		parsedURL, parseError := url.Parse(trimmed)
		if parseError != nil {
			return nil, false
		}
		return ownerRepoFromPath(parsedURL.Path)
	}

	colonIndex := strings.Index(trimmed, ":")
	if colonIndex >= 0 {
		trimmed = trimmed[colonIndex+1:]
	}

	return ownerRepoFromPath(trimmed)
}

func ownerRepoFromPath(path string) (*shared.OwnerRepository, bool) {
	trimmedPath := strings.TrimPrefix(strings.TrimSpace(path), "/")
	segments := strings.Split(trimmedPath, "/")
	if len(segments) < 2 {
		return nil, false
	}
	owner := strings.TrimSpace(segments[len(segments)-2])
	repository := strings.TrimSpace(segments[len(segments)-1])
	if len(owner) == 0 || len(repository) == 0 {
		return nil, false
	}
	ownerRepository, err := shared.NewOwnerRepository(owner + "/" + repository)
	if err != nil {
		return nil, false
	}
	return &ownerRepository, true
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	reflectedValue := reflect.ValueOf(value)
	switch reflectedValue.Kind() {
	case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func:
		return reflectedValue.IsNil()
	default:
		return false
	}
}
