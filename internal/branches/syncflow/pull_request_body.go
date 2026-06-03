package syncflow

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/utils/llm"
)

const (
	strictSyncPullRequestDescriptionMaxTokens       = 768
	strictSyncPullRequestDescriptionFragmentLimit   = 16000
	strictSyncPullRequestDescriptionNoDiffMessage   = "strict sync pull request description.empty_diff: branch diff is empty for %s"
	strictSyncPullRequestDescriptionEmptyResponse   = "strict sync pull request description.empty_response: llm returned an empty pull request description"
	strictSyncPullRequestDescriptionLLMTemplate     = "strict sync pull request description.llm: %w"
	strictSyncPullRequestDescriptionGitTemplate     = "strict sync pull request description.%s: %w"
	strictSyncPullRequestDescriptionSystemPrompt    = "You are an expert maintainer writing pull request descriptions from code differences. Explain why the change exists using only the provided git diff context. Do not mention gix, automation, command names, or how the PR was created. Do not quote diff excerpts or include code fences."
	strictSyncPullRequestDescriptionUserPrompt      = "Repository: %s\nBase branch: %s\nHead branch: %s\nComparison range: %s\nCommit range: %s\n\nCommit subjects:\n%s\n\nChange summary:\n%s\n\nPatch:\n%s\n\nReturn only a concise Markdown pull request description generated from this code difference."
	strictSyncPullRequestDescriptionNoCommitSubject = "No unique commit subjects."
	gitLogSubcommandConstant                        = "log"
	gitLogSubjectFormatFlagConstant                 = "--format=%s"
	gitDiffSubcommandConstant                       = "diff"
	gitDiffStatFlagConstant                         = "--stat"
	gitDiffUnifiedFlagConstant                      = "--unified=3"
)

type strictSyncPullRequestDescriptionOptions struct {
	RepositoryPath string
	RemoteName     string
	BaseBranch     string
	BranchName     string
	CommitMessages worktreeAdoptionCommitMessageOptions
}

type strictSyncPullRequestMetadata struct {
	Title string
	Body  string
}

type strictSyncPullRequestMetadataOptions struct {
	RepositoryPath string
	RemoteName     string
	BaseBranch     string
	BranchName     string
	PullRequest    strictSyncPullRequestMetadata
	CommitMessages worktreeAdoptionCommitMessageOptions
}

type strictSyncPullRequestDescriptionContext struct {
	RepositoryLabel string
	BaseReference   string
	BaseBranch      string
	BranchName      string
	ComparisonRange string
	CommitRange     string
	CommitSubjects  string
	DiffStat        string
	Patch           string
}

func strictSyncPullRequestMetadataFromParameters(parameters map[string]any) (strictSyncPullRequestMetadata, error) {
	title, titleErr := optionalStringOption(parameters, taskOptionPullRequestTitle)
	if titleErr != nil {
		return strictSyncPullRequestMetadata{}, titleErr
	}
	body, bodyErr := optionalStringOption(parameters, taskOptionPullRequestBody)
	if bodyErr != nil {
		return strictSyncPullRequestMetadata{}, bodyErr
	}
	return strictSyncPullRequestMetadata{
		Title: title,
		Body:  body,
	}, nil
}

func resolveStrictSyncPullRequestMetadata(ctx context.Context, executor shared.GitExecutor, options strictSyncPullRequestMetadataOptions) (strictSyncPullRequestMetadata, error) {
	title := strings.TrimSpace(options.PullRequest.Title)
	if title == "" {
		title = strings.TrimSpace(options.BranchName)
	}
	body := strings.TrimSpace(options.PullRequest.Body)
	if body != "" {
		return strictSyncPullRequestMetadata{
			Title: title,
			Body:  body,
		}, nil
	}

	generatedBody, bodyErr := generateStrictSyncPullRequestBody(ctx, executor, strictSyncPullRequestDescriptionOptions{
		RepositoryPath: options.RepositoryPath,
		RemoteName:     options.RemoteName,
		BaseBranch:     options.BaseBranch,
		BranchName:     options.BranchName,
		CommitMessages: options.CommitMessages,
	})
	if bodyErr != nil {
		return strictSyncPullRequestMetadata{}, bodyErr
	}
	return strictSyncPullRequestMetadata{
		Title: title,
		Body:  generatedBody,
	}, nil
}

func generateStrictSyncPullRequestBody(ctx context.Context, executor shared.GitExecutor, options strictSyncPullRequestDescriptionOptions) (string, error) {
	descriptionContext, contextErr := collectStrictSyncPullRequestDescriptionContext(ctx, executor, options)
	if contextErr != nil {
		return "", contextErr
	}
	client, clientErr := resolveCommitMessageClient(options.CommitMessages)
	if clientErr != nil {
		return "", clientErr
	}
	request := buildStrictSyncPullRequestDescriptionRequest(descriptionContext, options.CommitMessages)
	response, responseErr := client.Chat(ctx, request)
	if responseErr != nil {
		return "", fmt.Errorf(strictSyncPullRequestDescriptionLLMTemplate, responseErr)
	}
	trimmedResponse := strings.TrimSpace(response)
	if trimmedResponse == "" {
		return "", errors.New(strictSyncPullRequestDescriptionEmptyResponse)
	}
	return trimmedResponse, nil
}

func collectStrictSyncPullRequestDescriptionContext(ctx context.Context, executor shared.GitExecutor, options strictSyncPullRequestDescriptionOptions) (strictSyncPullRequestDescriptionContext, error) {
	baseReference := fmt.Sprintf("%s/%s", options.RemoteName, options.BaseBranch)
	comparisonRange := fmt.Sprintf("%s...%s", baseReference, options.BranchName)
	commitRange := fmt.Sprintf("%s..%s", baseReference, options.BranchName)

	commitSubjects, commitErr := strictSyncPullRequestDescriptionGitOutput(ctx, executor, options.RepositoryPath, []string{gitLogSubcommandConstant, gitLogSubjectFormatFlagConstant, commitRange}, "commit_log")
	if commitErr != nil {
		return strictSyncPullRequestDescriptionContext{}, commitErr
	}
	diffStat, statErr := strictSyncPullRequestDescriptionGitOutput(ctx, executor, options.RepositoryPath, []string{gitDiffSubcommandConstant, gitDiffStatFlagConstant, comparisonRange}, "diff_stat")
	if statErr != nil {
		return strictSyncPullRequestDescriptionContext{}, statErr
	}
	patch, patchErr := strictSyncPullRequestDescriptionGitOutput(ctx, executor, options.RepositoryPath, []string{gitDiffSubcommandConstant, gitDiffUnifiedFlagConstant, comparisonRange}, "patch")
	if patchErr != nil {
		return strictSyncPullRequestDescriptionContext{}, patchErr
	}

	trimmedCommitSubjects := truncateStrictSyncPullRequestDescriptionFragment(commitSubjects)
	trimmedDiffStat := truncateStrictSyncPullRequestDescriptionFragment(diffStat)
	trimmedPatch := truncateStrictSyncPullRequestDescriptionFragment(patch)
	if trimmedCommitSubjects == "" && trimmedDiffStat == "" && trimmedPatch == "" {
		return strictSyncPullRequestDescriptionContext{}, fmt.Errorf(strictSyncPullRequestDescriptionNoDiffMessage, comparisonRange)
	}

	repositoryLabel := filepath.Base(filepath.Clean(options.RepositoryPath))
	if repositoryLabel == "." || repositoryLabel == string(filepath.Separator) {
		repositoryLabel = options.RepositoryPath
	}

	return strictSyncPullRequestDescriptionContext{
		RepositoryLabel: repositoryLabel,
		BaseReference:   baseReference,
		BaseBranch:      options.BaseBranch,
		BranchName:      options.BranchName,
		ComparisonRange: comparisonRange,
		CommitRange:     commitRange,
		CommitSubjects:  trimmedCommitSubjects,
		DiffStat:        trimmedDiffStat,
		Patch:           trimmedPatch,
	}, nil
}

func strictSyncPullRequestDescriptionGitOutput(ctx context.Context, executor shared.GitExecutor, repositoryPath string, arguments []string, operation string) (string, error) {
	result, executionErr := executor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        arguments,
		WorkingDirectory: repositoryPath,
	})
	if executionErr != nil {
		return "", fmt.Errorf(strictSyncPullRequestDescriptionGitTemplate, operation, executionErr)
	}
	return result.StandardOutput, nil
}

func buildStrictSyncPullRequestDescriptionRequest(descriptionContext strictSyncPullRequestDescriptionContext, options worktreeAdoptionCommitMessageOptions) llm.ChatRequest {
	var temperature *float64
	if options.Temperature != 0 {
		temperatureValue := options.Temperature
		temperature = &temperatureValue
	}
	maxTokens := options.MaxTokens
	if maxTokens <= 0 {
		maxTokens = strictSyncPullRequestDescriptionMaxTokens
	}
	return llm.ChatRequest{
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: strictSyncPullRequestDescriptionSystemPrompt,
			},
			{
				Role: "user",
				Content: fmt.Sprintf(
					strictSyncPullRequestDescriptionUserPrompt,
					descriptionContext.RepositoryLabel,
					descriptionContext.BaseBranch,
					descriptionContext.BranchName,
					descriptionContext.ComparisonRange,
					descriptionContext.CommitRange,
					fallbackStrictSyncPullRequestDescriptionText(descriptionContext.CommitSubjects, strictSyncPullRequestDescriptionNoCommitSubject),
					descriptionContext.DiffStat,
					descriptionContext.Patch,
				),
			},
		},
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}
}

func truncateStrictSyncPullRequestDescriptionFragment(value string) string {
	trimmedValue := strings.TrimSpace(value)
	runes := []rune(trimmedValue)
	if len(runes) <= strictSyncPullRequestDescriptionFragmentLimit {
		return trimmedValue
	}
	return string(runes[:strictSyncPullRequestDescriptionFragmentLimit])
}

func fallbackStrictSyncPullRequestDescriptionText(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
