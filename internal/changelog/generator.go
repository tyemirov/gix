package changelog

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/pkg/llm"
)

const (
	defaultMaxTokens                 = 1200
	defaultCommitLogCharacterLimit   = 12000
	defaultDiffSummaryCharacterLimit = 6000
	defaultDiffCharacterLimit        = 18000
)

// Options configure changelog generation.
type Options struct {
	RepositoryPath string
	Version        string
	ReleaseDate    string
	SinceReference string
	SinceDate      *time.Time
	MaxTokens      int
	Temperature    *float64
}

// Result contains the generated changelog section and request context.
type Result struct {
	Section string
	Request llm.ChatRequest
}

// Generator produces changelog sections summarizing git history via an LLM.
type Generator struct {
	GitExecutor shared.GitExecutor
	Client      llm.ChatClient
	Logger      *zap.Logger
}

// ErrNoChanges indicates the selected range contains no commits.
var ErrNoChanges = errors.New("no changes detected for changelog generation")

// Generate builds the prompt and returns the LLM response.
func (generator Generator) Generate(ctx context.Context, options Options) (Result, error) {
	request, buildError := generator.BuildRequest(ctx, options)
	if buildError != nil {
		return Result{}, buildError
	}
	response, llmError := generator.Client.Chat(ctx, request)
	if llmError != nil {
		return Result{}, fmt.Errorf("changelog generation.llm: %w", llmError)
	}
	section := strings.TrimSpace(response)
	if section == "" {
		return Result{}, errors.New("llm returned an empty changelog section")
	}
	return Result{Section: section, Request: request}, nil
}

// BuildRequest prepares the chat request without invoking the LLM.
func (generator Generator) BuildRequest(ctx context.Context, options Options) (llm.ChatRequest, error) {
	if generator.GitExecutor == nil {
		return llm.ChatRequest{}, errors.New("git executor is not configured")
	}
	if generator.Client == nil {
		return llm.ChatRequest{}, errors.New("llm client is not configured")
	}

	repositoryPath := strings.TrimSpace(options.RepositoryPath)
	if repositoryPath == "" {
		return llm.ChatRequest{}, errors.New("repository path is required")
	}

	baseline, baselineError := generator.resolveBaseline(ctx, repositoryPath, options)
	if baselineError != nil {
		return llm.ChatRequest{}, baselineError
	}

	gitContext, contextError := generator.collectGitContext(ctx, repositoryPath, baseline)
	if contextError != nil {
		return llm.ChatRequest{}, contextError
	}
	if gitContext.commitLog == "" && gitContext.diffSummary == "" && gitContext.diffExcerpt == "" {
		return llm.ChatRequest{}, ErrNoChanges
	}

	releaseVersion := strings.TrimSpace(options.Version)
	if releaseVersion == "" {
		releaseVersion = "Unreleased"
	}
	releaseDate := strings.TrimSpace(options.ReleaseDate)

	systemMessage := llm.Message{
		Role: "system",
		Content: strings.Join([]string{
			"You are an expert release engineer creating Markdown changelog sections.",
			"Return Markdown only; do not use code fences or surrounding commentary.",
			"Start with a heading `## [VERSION] - DATE` when a release date is provided, otherwise `## [VERSION]`.",
			"Include the following subsections in order: `### Features ✨`, `### Improvements ⚙️`, `### Bug Fixes 🐛`, `### Testing 🧪`, `### Docs 📚`.",
			"Each subsection must contain concise bullet points (`- ` prefix). If no updates exist, include `- _No changes._`.",
			"Summaries should be user-facing, avoid commit hashes, and stay under three bullets per section unless critical.",
		}, " "),
	}

	repositoryLabel := filepath.Base(repositoryPath)
	if repositoryLabel == "." || repositoryLabel == "/" {
		repositoryLabel = repositoryPath
	}

	userSections := []string{
		fmt.Sprintf("Repository: %s", repositoryLabel),
		fmt.Sprintf("Release version: %s", releaseVersion),
		fmt.Sprintf("Baseline: %s", gitContext.baselineDescription),
	}
	if releaseDate != "" {
		userSections = append(userSections, fmt.Sprintf("Release date: %s", releaseDate))
	}
	userSections = append(userSections,
		"",
		"Commit summary:",
		fallbackText(gitContext.commitLog, "No commits found."),
		"",
		"Diff summary:",
		fallbackText(gitContext.diffSummary, "No diff summary available."),
	)
	if gitContext.diffExcerpt != "" {
		userSections = append(userSections, "", "Diff excerpt:", gitContext.diffExcerpt)
	}
	userSections = append(userSections, "", "Generate the section now.")

	userMessage := llm.Message{
		Role:    "user",
		Content: strings.Join(userSections, "\n"),
	}

	maxTokens := options.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}

	request := llm.ChatRequest{
		Messages:    []llm.Message{systemMessage, userMessage},
		MaxTokens:   maxTokens,
		Temperature: options.Temperature,
	}

	return request, nil
}

type baselineInfo struct {
	rangeExpression string
	description     string
}

func (generator Generator) resolveBaseline(ctx context.Context, repositoryPath string, options Options) (baselineInfo, error) {
	sinceRef := strings.TrimSpace(options.SinceReference)
	if sinceRef != "" {
		return baselineInfo{
			rangeExpression: sinceRef + "..HEAD",
			description:     fmt.Sprintf("changes since %s", sinceRef),
		}, nil
	}

	if options.SinceDate != nil {
		dateValue := options.SinceDate.UTC().Format(time.RFC3339)
		revListArgs := []string{"rev-list", "--max-count=1", "--before=" + dateValue, "HEAD"}
		commitHash, revListError := generator.runGit(ctx, repositoryPath, revListArgs)
		if revListError == nil && strings.TrimSpace(commitHash) != "" {
			trimmed := strings.TrimSpace(commitHash)
			return baselineInfo{
				rangeExpression: trimmed + "..HEAD",
				description:     fmt.Sprintf("changes since %s", options.SinceDate.Format(time.RFC3339)),
			}, nil
		}
	}

	describeArgs := []string{"describe", "--tags", "--abbrev=0"}
	tagName, describeError := generator.runGit(ctx, repositoryPath, describeArgs)
	tagName = strings.TrimSpace(tagName)
	if describeError == nil && tagName != "" {
		return baselineInfo{
			rangeExpression: tagName + "..HEAD",
			description:     fmt.Sprintf("changes since tag %s", tagName),
		}, nil
	}

	// Fall back to the root diff if no baseline could be detected.
	return baselineInfo{
		rangeExpression: "",
		description:     "full history (no prior tags)",
	}, nil
}

type gitContextFragments struct {
	baselineDescription string
	commitLog           string
	diffSummary         string
	diffExcerpt         string
}

func (generator Generator) collectGitContext(ctx context.Context, repositoryPath string, baseline baselineInfo) (gitContextFragments, error) {
	logArguments := []string{"log", "--no-merges", "--date=short", "--pretty=format:%h %ad %an %s", "--max-count=200"}
	if baseline.rangeExpression != "" {
		logArguments = append(logArguments, baseline.rangeExpression)
	}
	logOutput, logError := generator.runGit(ctx, repositoryPath, logArguments)
	if logError != nil {
		return gitContextFragments{}, logError
	}

	diffSummaryArguments := []string{"diff", "--stat"}
	diffArguments := []string{"diff", "--unified=3"}
	if baseline.rangeExpression != "" {
		diffSummaryArguments = append(diffSummaryArguments, baseline.rangeExpression)
		diffArguments = append(diffArguments, baseline.rangeExpression)
	} else {
		diffSummaryArguments = append(diffSummaryArguments, "--root", "HEAD")
		diffArguments = append(diffArguments, "--root", "HEAD")
	}

	diffSummaryOutput, diffSummaryError := generator.runGit(ctx, repositoryPath, diffSummaryArguments)
	if diffSummaryError != nil {
		return gitContextFragments{}, diffSummaryError
	}
	diffOutput, diffError := generator.runGit(ctx, repositoryPath, diffArguments)
	if diffError != nil {
		return gitContextFragments{}, diffError
	}

	commitLog := truncateRunes(strings.TrimSpace(filterChangelogArtifacts(logOutput)), defaultCommitLogCharacterLimit)
	diffSummary := truncateRunes(strings.TrimSpace(filterChangelogArtifacts(diffSummaryOutput)), defaultDiffSummaryCharacterLimit)
	diffExcerpt := truncateRunes(strings.TrimSpace(filterChangelogArtifacts(diffOutput)), defaultDiffCharacterLimit)

	return gitContextFragments{
		baselineDescription: baseline.description,
		commitLog:           commitLog,
		diffSummary:         diffSummary,
		diffExcerpt:         diffExcerpt,
	}, nil
}

func (generator Generator) runGit(ctx context.Context, repositoryPath string, arguments []string) (string, error) {
	result, execError := generator.GitExecutor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        arguments,
		WorkingDirectory: repositoryPath,
	})
	if execError != nil {
		return "", execError
	}
	return result.StandardOutput, nil
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return strings.TrimSpace(value)
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(string(runes[:limit])) + "\n…"
}

func filterChangelogArtifacts(value string) string {
	if value == "" {
		return value
	}
	lines := strings.Split(value, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			filtered = append(filtered, line)
			continue
		}
		if strings.Contains(trimmed, "CHANGELOG.md") || strings.Contains(trimmed, "Changelog") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func fallbackText(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
