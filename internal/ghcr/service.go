package ghcr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"go.uber.org/zap"
)

const (
	defaultBaseURLConstant                       = "https://api.github.com"
	acceptHeaderNameConstant                     = "Accept"
	acceptHeaderValueConstant                    = "application/vnd.github+json"
	authorizationHeaderNameConstant              = "Authorization"
	bearerTokenTemplateConstant                  = "Bearer %s"
	perPageQueryParameterNameConstant            = "per_page"
	pageQueryParameterNameConstant               = "page"
	defaultPageSizeConstant                      = 100
	packagesPathSegmentConstant                  = "packages"
	containerPathSegmentConstant                 = "container"
	versionsPathSegmentConstant                  = "versions"
	requestCreationErrorTemplateConstant         = "unable to create %s request for %s: %w"
	requestExecutionErrorTemplateConstant        = "request execution failed: %w"
	unexpectedStatusCodeWithBodyTemplateConstant = "unexpected status code %d for %s %s: %s"
	responseDecodeErrorTemplateConstant          = "unable to decode package versions: %w"
	deletionFailureTemplateConstant              = "failed to delete version %d: %s"
	purgeStartMessageConstant                    = "Starting GHCR untagged version purge"
	purgePageMessageConstant                     = "Fetched GHCR package versions page"
	purgeDeleteMessageConstant                   = "Deleting untagged GHCR package version"
	purgeCompleteMessageConstant                 = "Completed GHCR untagged version purge"
	ownerLogFieldNameConstant                    = "owner"
	packageLogFieldNameConstant                  = "package"
	ownerTypeLogFieldNameConstant                = "owner_type"
	pageLogFieldNameConstant                     = "page_number"
	pageSizeLogFieldNameConstant                 = "page_size"
	versionIdentifierLogFieldNameConstant        = "version_id"
	totalVersionsLogFieldNameConstant            = "total_versions"
	untaggedVersionsLogFieldNameConstant         = "untagged_versions"
	deletedVersionsLogFieldNameConstant          = "deleted_versions"
	tokenMissingErrorMessageConstant             = "authentication token must be provided"
	ownerMissingErrorMessageConstant             = "owner must be provided"
	packageMissingErrorMessageConstant           = "package name must be provided"
	ownerTypeMissingErrorMessageConstant         = "owner type must be provided"
)

var deleteSuccessStatusCodes = map[int]struct{}{
	http.StatusNoContent: {},
	http.StatusAccepted:  {},
}

// HTTPClient abstracts the Do method of http.Client for easier testing.
type HTTPClient interface {
	Do(request *http.Request) (*http.Response, error)
}

// ServiceConfiguration specifies HTTP behavior for the GHCR client.
type ServiceConfiguration struct {
	BaseURL  string
	PageSize int
}

// PurgeRequest captures the information required to delete untagged versions.
type PurgeRequest struct {
	Owner       string
	PackageName string
	OwnerType   OwnerType
	Token       string
}

// PurgeResult contains summary statistics from a purge operation.
type PurgeResult struct {
	TotalVersions    int
	UntaggedVersions int
	DeletedVersions  int
}

// PackageVersionService interacts with the GHCR REST API.
type PackageVersionService struct {
	logger     *zap.Logger
	httpClient HTTPClient
	baseURL    string
	pageSize   int
}

// NewPackageVersionService constructs a service with sane defaults.
func NewPackageVersionService(logger *zap.Logger, httpClient HTTPClient, configuration ServiceConfiguration) (*PackageVersionService, error) {
	resolvedLogger := logger
	if resolvedLogger == nil {
		resolvedLogger = zap.NewNop()
	}

	resolvedClient := httpClient
	if resolvedClient == nil {
		resolvedClient = http.DefaultClient
	}

	resolvedBaseURL := strings.TrimSpace(configuration.BaseURL)
	if len(resolvedBaseURL) == 0 {
		resolvedBaseURL = defaultBaseURLConstant
	}

	resolvedPageSize := configuration.PageSize
	if resolvedPageSize <= 0 {
		resolvedPageSize = defaultPageSizeConstant
	}

	return &PackageVersionService{
		logger:     resolvedLogger,
		httpClient: resolvedClient,
		baseURL:    resolvedBaseURL,
		pageSize:   resolvedPageSize,
	}, nil
}

// PurgeUntaggedVersions removes untagged container versions and returns summary counts.
func (service *PackageVersionService) PurgeUntaggedVersions(executionContext context.Context, request PurgeRequest) (PurgeResult, error) {
	trimmedToken := strings.TrimSpace(request.Token)
	if len(trimmedToken) == 0 {
		return PurgeResult{}, errors.New(tokenMissingErrorMessageConstant)
	}
	trimmedOwner := strings.TrimSpace(request.Owner)
	if len(trimmedOwner) == 0 {
		return PurgeResult{}, errors.New(ownerMissingErrorMessageConstant)
	}
	trimmedPackageName := strings.TrimSpace(request.PackageName)
	if len(trimmedPackageName) == 0 {
		return PurgeResult{}, errors.New(packageMissingErrorMessageConstant)
	}
	if len(strings.TrimSpace(string(request.OwnerType))) == 0 {
		return PurgeResult{}, errors.New(ownerTypeMissingErrorMessageConstant)
	}

	request.Token = trimmedToken
	request.Owner = trimmedOwner
	request.PackageName = trimmedPackageName

	service.logger.Info(
		purgeStartMessageConstant,
		zap.String(ownerLogFieldNameConstant, trimmedOwner),
		zap.String(packageLogFieldNameConstant, trimmedPackageName),
		zap.String(ownerTypeLogFieldNameConstant, string(request.OwnerType)),
		zap.Int(pageSizeLogFieldNameConstant, service.pageSize),
	)

	result := PurgeResult{}
	pageNumber := 1
	for {
		versions, fetchError := service.fetchPage(executionContext, request, pageNumber)
		if fetchError != nil {
			return result, fetchError
		}

		versionCount := len(versions)
		if versionCount == 0 {
			break
		}

		service.logger.Debug(
			purgePageMessageConstant,
			zap.String(ownerLogFieldNameConstant, trimmedOwner),
			zap.String(packageLogFieldNameConstant, trimmedPackageName),
			zap.Int(pageLogFieldNameConstant, pageNumber),
			zap.Int(totalVersionsLogFieldNameConstant, versionCount),
		)

		result.TotalVersions += versionCount

		for versionIndex := range versions {
			version := versions[versionIndex]
			if version.HasTags() {
				continue
			}

			result.UntaggedVersions++
			service.logger.Info(
				purgeDeleteMessageConstant,
				zap.Int64(versionIdentifierLogFieldNameConstant, version.ID),
			)

			deleteError := service.deleteVersion(executionContext, request, version.ID)
			if deleteError != nil {
				return result, deleteError
			}
			result.DeletedVersions++
		}

		pageNumber++
	}

	service.logger.Info(
		purgeCompleteMessageConstant,
		zap.String(ownerLogFieldNameConstant, trimmedOwner),
		zap.String(packageLogFieldNameConstant, trimmedPackageName),
		zap.Int(totalVersionsLogFieldNameConstant, result.TotalVersions),
		zap.Int(untaggedVersionsLogFieldNameConstant, result.UntaggedVersions),
		zap.Int(deletedVersionsLogFieldNameConstant, result.DeletedVersions),
	)

	return result, nil
}

func (service *PackageVersionService) fetchPage(executionContext context.Context, request PurgeRequest, pageNumber int) ([]packageVersion, error) {
	versionsURL, urlBuildError := service.buildVersionsURL(request.OwnerType, request.Owner, request.PackageName, pageNumber)
	if urlBuildError != nil {
		return nil, urlBuildError
	}

	httpRequest, requestCreationError := http.NewRequestWithContext(executionContext, http.MethodGet, versionsURL, nil)
	if requestCreationError != nil {
		return nil, fmt.Errorf(requestCreationErrorTemplateConstant, http.MethodGet, versionsURL, requestCreationError)
	}

	httpRequest.Header.Set(acceptHeaderNameConstant, acceptHeaderValueConstant)
	httpRequest.Header.Set(authorizationHeaderNameConstant, fmt.Sprintf(bearerTokenTemplateConstant, request.Token))

	httpResponse, requestError := service.httpClient.Do(httpRequest)
	if requestError != nil {
		return nil, fmt.Errorf(requestExecutionErrorTemplateConstant, requestError)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(httpResponse.Body)
		return nil, fmt.Errorf(
			unexpectedStatusCodeWithBodyTemplateConstant,
			httpResponse.StatusCode,
			http.MethodGet,
			versionsURL,
			strings.TrimSpace(string(responseBody)),
		)
	}

	var versions []packageVersion
	decodeError := json.NewDecoder(httpResponse.Body).Decode(&versions)
	if decodeError != nil {
		return nil, fmt.Errorf(responseDecodeErrorTemplateConstant, decodeError)
	}

	return versions, nil
}

func (service *PackageVersionService) deleteVersion(executionContext context.Context, request PurgeRequest, versionID int64) error {
	deleteURL, urlBuildError := service.buildVersionURL(request.OwnerType, request.Owner, request.PackageName, versionID)
	if urlBuildError != nil {
		return urlBuildError
	}

	deleteRequest, deleteRequestCreationError := http.NewRequestWithContext(executionContext, http.MethodDelete, deleteURL, nil)
	if deleteRequestCreationError != nil {
		return fmt.Errorf(requestCreationErrorTemplateConstant, http.MethodDelete, deleteURL, deleteRequestCreationError)
	}

	deleteRequest.Header.Set(acceptHeaderNameConstant, acceptHeaderValueConstant)
	deleteRequest.Header.Set(authorizationHeaderNameConstant, fmt.Sprintf(bearerTokenTemplateConstant, request.Token))

	deleteResponse, deleteError := service.httpClient.Do(deleteRequest)
	if deleteError != nil {
		return fmt.Errorf(requestExecutionErrorTemplateConstant, deleteError)
	}
	defer deleteResponse.Body.Close()

	if _, ok := deleteSuccessStatusCodes[deleteResponse.StatusCode]; !ok {
		responseBody, _ := io.ReadAll(deleteResponse.Body)
		return fmt.Errorf(deletionFailureTemplateConstant, versionID, strings.TrimSpace(string(responseBody)))
	}

	return nil
}

func (service *PackageVersionService) buildVersionsURL(ownerType OwnerType, owner string, packageName string, pageNumber int) (string, error) {
	baseURL, parseError := url.Parse(service.baseURL)
	if parseError != nil {
		return "", parseError
	}

	baseURL.Path = strings.TrimSuffix(baseURL.Path, "/")
	escapedOwner := url.PathEscape(owner)
	escapedPackageName := url.PathEscape(packageName)

	pathSegments := []string{
		baseURL.Path,
		ownerType.PathSegment(),
		escapedOwner,
		packagesPathSegmentConstant,
		containerPathSegmentConstant,
		escapedPackageName,
		versionsPathSegmentConstant,
	}

	baseURL.Path = strings.Join(pathSegments, "/")

	queryParameters := baseURL.Query()
	queryParameters.Set(perPageQueryParameterNameConstant, fmt.Sprintf("%d", service.pageSize))
	queryParameters.Set(pageQueryParameterNameConstant, fmt.Sprintf("%d", pageNumber))
	baseURL.RawQuery = queryParameters.Encode()

	return baseURL.String(), nil
}

func (service *PackageVersionService) buildVersionURL(ownerType OwnerType, owner string, packageName string, versionID int64) (string, error) {
	baseURL, parseError := url.Parse(service.baseURL)
	if parseError != nil {
		return "", parseError
	}

	baseURL.Path = strings.TrimSuffix(baseURL.Path, "/")
	escapedOwner := url.PathEscape(owner)
	escapedPackageName := url.PathEscape(packageName)

	pathSegments := []string{
		baseURL.Path,
		ownerType.PathSegment(),
		escapedOwner,
		packagesPathSegmentConstant,
		containerPathSegmentConstant,
		escapedPackageName,
		versionsPathSegmentConstant,
		fmt.Sprintf("%d", versionID),
	}

	baseURL.Path = strings.Join(pathSegments, "/")
	baseURL.RawQuery = ""

	return baseURL.String(), nil
}

type packageVersion struct {
	ID       int64                  `json:"id"`
	Metadata packageVersionMetadata `json:"metadata"`
}

type packageVersionMetadata struct {
	Container packageVersionContainerMetadata `json:"container"`
}

type packageVersionContainerMetadata struct {
	Tags []string `json:"tags"`
}

func (version packageVersion) HasTags() bool {
	return len(version.Metadata.Container.Tags) > 0
}
