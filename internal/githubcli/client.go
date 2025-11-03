package githubcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/githubauth"
)

const (
	repoSubcommandConstant                     = "repo"
	viewSubcommandConstant                     = "view"
	pullRequestSubcommandConstant              = "pr"
	listSubcommandConstant                     = "list"
	editSubcommandConstant                     = "edit"
	createSubcommandConstant                   = "create"
	apiSubcommandConstant                      = "api"
	jsonFlagConstant                           = "--json"
	repoFlagConstant                           = "--repo"
	stateFlagConstant                          = "--state"
	baseFlagConstant                           = "--base"
	limitFlagConstant                          = "--limit"
	methodFlagConstant                         = "-X"
	fieldFlagConstant                          = "-f"
	inputFlagConstant                          = "--input"
	stdinReferenceConstant                     = "-"
	headFlagConstant                           = "--head"
	titleFlagConstant                          = "--title"
	bodyFlagConstant                           = "--body"
	draftFlagConstant                          = "--draft"
	acceptHeaderFlagConstant                   = "-H"
	acceptHeaderValueConstant                  = "Accept: application/vnd.github+json"
	repositoryFieldNameConstant                = "repository"
	baseBranchFieldNameConstant                = "base_branch"
	sourceBranchFieldNameConstant              = "source_branch"
	defaultBranchFieldNameConstant             = "default_branch"
	pullRequestNumberFieldNameConstant         = "pull_request_number"
	stateFieldNameConstant                     = "state"
	requiredValueMessageConstant               = "value required"
	executorNotConfiguredMessageConstant       = "github cli executor not configured"
	pullRequestLimitDefaultValueConstant       = 100
	pullRequestJSONFieldsConstant              = "number,title,headRefName"
	repoViewJSONFieldsConstant                 = "defaultBranchRef,nameWithOwner,description,isInOrganization"
	operationErrorMessageTemplateConstant      = "%s operation failed"
	operationErrorWithCauseTemplateConstant    = "%s operation failed: %s"
	responseDecodingErrorTemplateConstant      = "%s response decoding failed: %s"
	payloadEncodingErrorTemplateConstant       = "%s payload encoding failed: %s"
	invalidInputErrorTemplateConstant          = "%s: %s"
	pagesEndpointTemplateConstant              = "repos/%s/pages"
	repositoryEndpointTemplateConstant         = "repos/%s"
	branchProtectionEndpointTemplateConstant   = "repos/%s/branches/%s/protection"
	pagesNullResponseConstant                  = "null"
	httpMethodGetConstant                      = "GET"
	httpMethodPutConstant                      = "PUT"
	httpMethodPatchConstant                    = "PATCH"
	repositoryMetadataOperationNameConstant    = OperationName("ResolveRepoMetadata")
	listPullRequestsOperationNameConstant      = OperationName("ListPullRequests")
	updatePagesOperationNameConstant           = OperationName("UpdatePagesConfig")
	getPagesOperationNameConstant              = OperationName("GetPagesConfig")
	updateDefaultBranchOperationNameConstant   = OperationName("UpdateDefaultBranch")
	updatePullRequestOperationNameConstant     = OperationName("UpdatePullRequestBase")
	checkBranchProtectionOperationNameConstant = OperationName("CheckBranchProtection")
	createPullRequestOperationNameConstant     = OperationName("CreatePullRequest")
	httpNotFoundIndicatorConstant              = "http 404"
	statusNotFoundIndicatorConstant            = "status 404"
)

// OperationName describes a named GitHub CLI workflow supported by the client.
type OperationName string

// PullRequestState describes acceptable GitHub pull request states.
type PullRequestState string

// Pull request state enumerations.
const (
	PullRequestStateOpen   PullRequestState = PullRequestState("open")
	PullRequestStateClosed PullRequestState = PullRequestState("closed")
	PullRequestStateMerged PullRequestState = PullRequestState("merged")
)

// RepositoryMetadata contains key details resolved from GitHub.
type RepositoryMetadata struct {
	NameWithOwner    string
	Description      string
	DefaultBranch    string
	IsInOrganization bool
}

// PullRequest represents minimal PR details returned by GitHub CLI.
type PullRequest struct {
	Number      int
	Title       string
	HeadRefName string
}

// PullRequestListOptions configures ListPullRequests queries.
type PullRequestListOptions struct {
	State       PullRequestState
	BaseBranch  string
	ResultLimit int
}

// PullRequestCreateOptions configures pull request creation parameters.
type PullRequestCreateOptions struct {
	Repository string
	Title      string
	Body       string
	Base       string
	Head       string
	Draft      bool
}

// PagesConfiguration describes the desired GitHub Pages configuration.
type PagesConfiguration struct {
	SourceBranch string
	SourcePath   string
}

// PagesBuildType describes GitHub Pages build modes.
type PagesBuildType string

// Supported Pages build types.
const (
	PagesBuildTypeLegacy   PagesBuildType = PagesBuildType("legacy")
	PagesBuildTypeWorkflow PagesBuildType = PagesBuildType("workflow")
)

// PagesStatus captures the GitHub Pages configuration state.
type PagesStatus struct {
	Enabled      bool
	BuildType    PagesBuildType
	SourceBranch string
	SourcePath   string
}

// GitHubCommandExecutor is the minimal interface required from execshell.ShellExecutor.
type GitHubCommandExecutor interface {
	ExecuteGitHubCLI(executionContext context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error)
}

// Client coordinates GitHub CLI invocations through execshell.
type Client struct {
	executor GitHubCommandExecutor
}

var (
	// ErrExecutorNotConfigured indicates the client was constructed without an executor.
	ErrExecutorNotConfigured = errors.New(executorNotConfiguredMessageConstant)
)

// InvalidInputError surfaces validation issues for operation inputs.
type InvalidInputError struct {
	FieldName string
	Message   string
}

// Error describes the invalid input.
func (inputError InvalidInputError) Error() string {
	return fmt.Sprintf(invalidInputErrorTemplateConstant, inputError.FieldName, inputError.Message)
}

// OperationError wraps execution issues for GitHub CLI operations.
type OperationError struct {
	Operation OperationName
	Cause     error
}

// Error describes the operation failure.
func (operationError OperationError) Error() string {
	if operationError.Cause == nil {
		return fmt.Sprintf(operationErrorMessageTemplateConstant, operationError.Operation)
	}
	return fmt.Sprintf(operationErrorWithCauseTemplateConstant, operationError.Operation, operationError.Cause)
}

// Unwrap exposes the underlying cause.
func (operationError OperationError) Unwrap() error {
	return operationError.Cause
}

// ResponseDecodingError indicates JSON decoding failures.
type ResponseDecodingError struct {
	Operation OperationName
	Cause     error
}

// Error describes the decoding failure.
func (decodingError ResponseDecodingError) Error() string {
	return fmt.Sprintf(responseDecodingErrorTemplateConstant, decodingError.Operation, decodingError.Cause)
}

// Unwrap exposes the underlying JSON error.
func (decodingError ResponseDecodingError) Unwrap() error {
	return decodingError.Cause
}

// PayloadEncodingError indicates JSON encoding issues.
type PayloadEncodingError struct {
	Operation OperationName
	Cause     error
}

// Error describes the encoding failure.
func (encodingError PayloadEncodingError) Error() string {
	return fmt.Sprintf(payloadEncodingErrorTemplateConstant, encodingError.Operation, encodingError.Cause)
}

// Unwrap exposes the underlying error.
func (encodingError PayloadEncodingError) Unwrap() error {
	return encodingError.Cause
}

// NewClient constructs a GitHub CLI client.
func NewClient(executor GitHubCommandExecutor) (*Client, error) {
	if executor == nil {
		return nil, ErrExecutorNotConfigured
	}
	return &Client{executor: executor}, nil
}

// ResolveRepoMetadata retrieves canonical metadata for a repository using gh repo view.
func (client *Client) ResolveRepoMetadata(executionContext context.Context, repository string) (RepositoryMetadata, error) {
	repositoryIdentifier := strings.TrimSpace(repository)
	if len(repositoryIdentifier) == 0 {
		return RepositoryMetadata{}, InvalidInputError{FieldName: repositoryFieldNameConstant, Message: requiredValueMessageConstant}
	}

	commandDetails := execshell.CommandDetails{
		Arguments: []string{
			repoSubcommandConstant,
			viewSubcommandConstant,
			repositoryIdentifier,
			jsonFlagConstant,
			repoViewJSONFieldsConstant,
		},
		GitHubTokenRequirement: githubauth.TokenRequired,
	}

	executionResult, executionError := client.executor.ExecuteGitHubCLI(executionContext, commandDetails)
	if executionError != nil {
		return RepositoryMetadata{}, OperationError{Operation: repositoryMetadataOperationNameConstant, Cause: executionError}
	}

	var response struct {
		NameWithOwner    string `json:"nameWithOwner"`
		Description      string `json:"description"`
		DefaultBranchRef struct {
			Name string `json:"name"`
		} `json:"defaultBranchRef"`
		IsInOrganization bool `json:"isInOrganization"`
	}

	decodingError := json.Unmarshal([]byte(executionResult.StandardOutput), &response)
	if decodingError != nil {
		return RepositoryMetadata{}, ResponseDecodingError{Operation: repositoryMetadataOperationNameConstant, Cause: decodingError}
	}

	return RepositoryMetadata{
		NameWithOwner:    response.NameWithOwner,
		Description:      response.Description,
		DefaultBranch:    response.DefaultBranchRef.Name,
		IsInOrganization: response.IsInOrganization,
	}, nil
}

// ListPullRequests enumerates pull requests using gh pr list.
func (client *Client) ListPullRequests(executionContext context.Context, repository string, options PullRequestListOptions) ([]PullRequest, error) {
	repositoryIdentifier := strings.TrimSpace(repository)
	if len(repositoryIdentifier) == 0 {
		return nil, InvalidInputError{FieldName: repositoryFieldNameConstant, Message: requiredValueMessageConstant}
	}

	if len(strings.TrimSpace(options.BaseBranch)) == 0 {
		return nil, InvalidInputError{FieldName: baseBranchFieldNameConstant, Message: requiredValueMessageConstant}
	}

	if len(options.State) == 0 {
		return nil, InvalidInputError{FieldName: stateFieldNameConstant, Message: requiredValueMessageConstant}
	}

	resultLimit := options.ResultLimit
	if resultLimit <= 0 {
		resultLimit = pullRequestLimitDefaultValueConstant
	}

	commandDetails := execshell.CommandDetails{
		Arguments: []string{
			pullRequestSubcommandConstant,
			listSubcommandConstant,
			repoFlagConstant,
			repositoryIdentifier,
			stateFlagConstant,
			string(options.State),
			baseFlagConstant,
			options.BaseBranch,
			jsonFlagConstant,
			pullRequestJSONFieldsConstant,
			limitFlagConstant,
			strconv.Itoa(resultLimit),
		},
		GitHubTokenRequirement: githubauth.TokenOptional,
	}

	executionResult, executionError := client.executor.ExecuteGitHubCLI(executionContext, commandDetails)
	if executionError != nil {
		return nil, OperationError{Operation: listPullRequestsOperationNameConstant, Cause: executionError}
	}

	var response []struct {
		Number      int    `json:"number"`
		Title       string `json:"title"`
		HeadRefName string `json:"headRefName"`
	}

	decodingError := json.Unmarshal([]byte(executionResult.StandardOutput), &response)
	if decodingError != nil {
		return nil, ResponseDecodingError{Operation: listPullRequestsOperationNameConstant, Cause: decodingError}
	}

	pullRequests := make([]PullRequest, 0, len(response))
	for _, pullRequestEntry := range response {
		pullRequests = append(pullRequests, PullRequest{
			Number:      pullRequestEntry.Number,
			Title:       pullRequestEntry.Title,
			HeadRefName: pullRequestEntry.HeadRefName,
		})
	}

	return pullRequests, nil
}

// CreatePullRequest opens a pull request using gh pr create.
func (client *Client) CreatePullRequest(executionContext context.Context, options PullRequestCreateOptions) error {
	if client.executor == nil {
		return ErrExecutorNotConfigured
	}

	repositoryIdentifier := strings.TrimSpace(options.Repository)
	if len(repositoryIdentifier) == 0 {
		return InvalidInputError{FieldName: repositoryFieldNameConstant, Message: requiredValueMessageConstant}
	}

	title := strings.TrimSpace(options.Title)
	if len(title) == 0 {
		return InvalidInputError{FieldName: titleFlagConstant, Message: requiredValueMessageConstant}
	}

	head := strings.TrimSpace(options.Head)
	if len(head) == 0 {
		return InvalidInputError{FieldName: sourceBranchFieldNameConstant, Message: requiredValueMessageConstant}
	}

	base := strings.TrimSpace(options.Base)
	if len(base) == 0 {
		return InvalidInputError{FieldName: baseBranchFieldNameConstant, Message: requiredValueMessageConstant}
	}

	arguments := []string{
		pullRequestSubcommandConstant,
		createSubcommandConstant,
		repoFlagConstant,
		repositoryIdentifier,
		baseFlagConstant,
		base,
		headFlagConstant,
		head,
		titleFlagConstant,
		title,
		bodyFlagConstant,
		options.Body,
	}

	if options.Draft {
		arguments = append(arguments, draftFlagConstant)
	}

	commandDetails := execshell.CommandDetails{
		Arguments:              arguments,
		GitHubTokenRequirement: githubauth.TokenRequired,
	}
	_, executionError := client.executor.ExecuteGitHubCLI(executionContext, commandDetails)
	if executionError != nil {
		return OperationError{Operation: createPullRequestOperationNameConstant, Cause: executionError}
	}

	return nil
}

// UpdatePagesConfig updates the GitHub Pages configuration using gh api.
func (client *Client) UpdatePagesConfig(executionContext context.Context, repository string, configuration PagesConfiguration) error {
	repositoryIdentifier := strings.TrimSpace(repository)
	if len(repositoryIdentifier) == 0 {
		return InvalidInputError{FieldName: repositoryFieldNameConstant, Message: requiredValueMessageConstant}
	}

	if len(strings.TrimSpace(configuration.SourceBranch)) == 0 {
		return InvalidInputError{FieldName: sourceBranchFieldNameConstant, Message: requiredValueMessageConstant}
	}

	payload := struct {
		Source struct {
			Branch string `json:"branch"`
			Path   string `json:"path"`
		} `json:"source"`
	}{}

	payload.Source.Branch = configuration.SourceBranch
	payload.Source.Path = configuration.SourcePath

	payloadBytes, encodingError := json.Marshal(payload)
	if encodingError != nil {
		return PayloadEncodingError{Operation: updatePagesOperationNameConstant, Cause: encodingError}
	}

	commandDetails := execshell.CommandDetails{
		Arguments: []string{
			apiSubcommandConstant,
			fmt.Sprintf(pagesEndpointTemplateConstant, repositoryIdentifier),
			methodFlagConstant,
			httpMethodPutConstant,
			inputFlagConstant,
			stdinReferenceConstant,
			acceptHeaderFlagConstant,
			acceptHeaderValueConstant,
		},
		StandardInput:          payloadBytes,
		GitHubTokenRequirement: githubauth.TokenOptional,
	}

	_, executionError := client.executor.ExecuteGitHubCLI(executionContext, commandDetails)
	if executionError != nil {
		return OperationError{Operation: updatePagesOperationNameConstant, Cause: executionError}
	}

	return nil
}

// GetPagesConfig retrieves the GitHub Pages configuration for a repository.
func (client *Client) GetPagesConfig(executionContext context.Context, repository string) (PagesStatus, error) {
	repositoryIdentifier := strings.TrimSpace(repository)
	if len(repositoryIdentifier) == 0 {
		return PagesStatus{}, InvalidInputError{FieldName: repositoryFieldNameConstant, Message: requiredValueMessageConstant}
	}

	commandDetails := execshell.CommandDetails{
		Arguments: []string{
			apiSubcommandConstant,
			fmt.Sprintf(pagesEndpointTemplateConstant, repositoryIdentifier),
			methodFlagConstant,
			httpMethodGetConstant,
			acceptHeaderFlagConstant,
			acceptHeaderValueConstant,
		},
		GitHubTokenRequirement: githubauth.TokenOptional,
	}

	executionResult, executionError := client.executor.ExecuteGitHubCLI(executionContext, commandDetails)
	if executionError != nil {
		return PagesStatus{}, OperationError{Operation: getPagesOperationNameConstant, Cause: executionError}
	}

	trimmedOutput := strings.TrimSpace(executionResult.StandardOutput)
	if len(trimmedOutput) == 0 || trimmedOutput == pagesNullResponseConstant {
		return PagesStatus{Enabled: false}, nil
	}

	var response struct {
		BuildType string `json:"build_type"`
		Source    struct {
			Branch string `json:"branch"`
			Path   string `json:"path"`
		} `json:"source"`
	}

	decodingError := json.Unmarshal([]byte(trimmedOutput), &response)
	if decodingError != nil {
		return PagesStatus{}, ResponseDecodingError{Operation: getPagesOperationNameConstant, Cause: decodingError}
	}

	pagesStatus := PagesStatus{
		Enabled:      true,
		BuildType:    PagesBuildType(response.BuildType),
		SourceBranch: response.Source.Branch,
		SourcePath:   response.Source.Path,
	}

	return pagesStatus, nil
}

// SetDefaultBranch updates the default branch for the repository.
func (client *Client) SetDefaultBranch(executionContext context.Context, repository string, branchName string) error {
	repositoryIdentifier := strings.TrimSpace(repository)
	if len(repositoryIdentifier) == 0 {
		return InvalidInputError{FieldName: repositoryFieldNameConstant, Message: requiredValueMessageConstant}
	}

	trimmedBranch := strings.TrimSpace(branchName)
	if len(trimmedBranch) == 0 {
		return InvalidInputError{FieldName: defaultBranchFieldNameConstant, Message: requiredValueMessageConstant}
	}

	commandDetails := execshell.CommandDetails{
		Arguments: []string{
			apiSubcommandConstant,
			fmt.Sprintf(repositoryEndpointTemplateConstant, repositoryIdentifier),
			methodFlagConstant,
			httpMethodPatchConstant,
			fieldFlagConstant,
			fmt.Sprintf("%s=%s", defaultBranchFieldNameConstant, trimmedBranch),
			acceptHeaderFlagConstant,
			acceptHeaderValueConstant,
		},
		GitHubTokenRequirement: githubauth.TokenRequired,
	}

	_, executionError := client.executor.ExecuteGitHubCLI(executionContext, commandDetails)
	if executionError != nil {
		return OperationError{Operation: updateDefaultBranchOperationNameConstant, Cause: executionError}
	}

	return nil
}

// UpdatePullRequestBase retargets a pull request to a new base branch.
func (client *Client) UpdatePullRequestBase(executionContext context.Context, repository string, pullRequestNumber int, baseBranch string) error {
	repositoryIdentifier := strings.TrimSpace(repository)
	if len(repositoryIdentifier) == 0 {
		return InvalidInputError{FieldName: repositoryFieldNameConstant, Message: requiredValueMessageConstant}
	}

	if pullRequestNumber <= 0 {
		return InvalidInputError{FieldName: pullRequestNumberFieldNameConstant, Message: requiredValueMessageConstant}
	}

	trimmedBase := strings.TrimSpace(baseBranch)
	if len(trimmedBase) == 0 {
		return InvalidInputError{FieldName: baseBranchFieldNameConstant, Message: requiredValueMessageConstant}
	}

	commandDetails := execshell.CommandDetails{
		Arguments: []string{
			pullRequestSubcommandConstant,
			editSubcommandConstant,
			strconv.Itoa(pullRequestNumber),
			repoFlagConstant,
			repositoryIdentifier,
			baseFlagConstant,
			trimmedBase,
		},
		GitHubTokenRequirement: githubauth.TokenOptional,
	}

	_, executionError := client.executor.ExecuteGitHubCLI(executionContext, commandDetails)
	if executionError != nil {
		return OperationError{Operation: updatePullRequestOperationNameConstant, Cause: executionError}
	}

	return nil
}

// CheckBranchProtection verifies whether the branch has protection rules.
func (client *Client) CheckBranchProtection(executionContext context.Context, repository string, branchName string) (bool, error) {
	repositoryIdentifier := strings.TrimSpace(repository)
	if len(repositoryIdentifier) == 0 {
		return false, InvalidInputError{FieldName: repositoryFieldNameConstant, Message: requiredValueMessageConstant}
	}

	trimmedBranch := strings.TrimSpace(branchName)
	if len(trimmedBranch) == 0 {
		return false, InvalidInputError{FieldName: sourceBranchFieldNameConstant, Message: requiredValueMessageConstant}
	}

	commandDetails := execshell.CommandDetails{
		Arguments: []string{
			apiSubcommandConstant,
			fmt.Sprintf(branchProtectionEndpointTemplateConstant, repositoryIdentifier, trimmedBranch),
			methodFlagConstant,
			httpMethodGetConstant,
			acceptHeaderFlagConstant,
			acceptHeaderValueConstant,
		},
		GitHubTokenRequirement: githubauth.TokenOptional,
	}

	_, executionError := client.executor.ExecuteGitHubCLI(executionContext, commandDetails)
	if executionError == nil {
		return true, nil
	}

	var commandFailure execshell.CommandFailedError
	if errors.As(executionError, &commandFailure) {
		if branchProtectionNotFound(commandFailure.Result) {
			return false, nil
		}
	}

	return false, OperationError{Operation: checkBranchProtectionOperationNameConstant, Cause: executionError}
}

func branchProtectionNotFound(result execshell.ExecutionResult) bool {
	if len(result.StandardError) == 0 && len(result.StandardOutput) == 0 {
		return false
	}

	combinedOutput := strings.ToLower(result.StandardError + " " + result.StandardOutput)

	return strings.Contains(combinedOutput, httpNotFoundIndicatorConstant) || strings.Contains(combinedOutput, statusNotFoundIndicatorConstant)
}
