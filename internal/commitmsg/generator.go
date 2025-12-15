package commitmsg

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/utils/llm"
)

const (
	defaultMaxTokens         = 256
	defaultPatchCharacterCap = 16000
)

// DiffSource controls which changes are summarized.
type DiffSource string

const (
	// DiffSourceStaged captures staged changes.
	DiffSourceStaged DiffSource = "staged"
	// DiffSourceWorktree captures unstaged working tree changes.
	DiffSourceWorktree DiffSource = "worktree"
)

// Options configure commit message generation.
type Options struct {
	RepositoryPath string
	Source         DiffSource
	MaxTokens      int
	Temperature    *float64
}

// Result contains the generated commit message and the prompt that produced it.
type Result struct {
	Message string
	Request llm.ChatRequest
}

// Generator produces commit messages from git diffs via an LLM.
type Generator struct {
	GitExecutor shared.GitExecutor
	Client      llm.ChatClient
	Logger      *zap.Logger
}

// ErrNoChanges indicates the selected diff source is empty.
var ErrNoChanges = errors.New("no changes detected for commit message generation")

// Generate builds the prompt and returns the LLM response.
func (generator Generator) Generate(ctx context.Context, options Options) (Result, error) {
	request, gatherError := generator.BuildRequest(ctx, options)
	if gatherError != nil {
		return Result{}, gatherError
	}
	response, llmError := generator.Client.Chat(ctx, request)
	if llmError != nil {
		return Result{}, fmt.Errorf("commit message generation.llm: %w", llmError)
	}
	trimmed := strings.TrimSpace(response)
	if trimmed == "" {
		return Result{}, errors.New("llm returned an empty commit message")
	}
	return Result{Message: trimmed, Request: request}, nil
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
	source := options.Source
	if source == "" {
		source = DiffSourceStaged
	}
	gitContext, contextError := generator.collectGitContext(ctx, repositoryPath, source)
	if contextError != nil {
		return llm.ChatRequest{}, contextError
	}
	if gitContext.patch == "" && gitContext.summary == "" && gitContext.status == "" {
		return llm.ChatRequest{}, ErrNoChanges
	}

	systemMessage := llm.Message{
		Role: "system",
		Content: strings.Join([]string{
			"You are an expert release engineer composing Conventional Commit messages.",
			"Produce a concise subject line (imperative mood, <= 72 characters).",
			"If additional detail is essential, follow the subject with a blank line and short bullet list.",
			"Do not include explanations, quotes, code fences, or diff excerpts.",
		}, " "),
	}

	repositoryLabel := filepath.Base(repositoryPath)
	if repositoryLabel == "." || repositoryLabel == "/" {
		repositoryLabel = repositoryPath
	}
	userMessage := llm.Message{
		Role: "user",
		Content: fmt.Sprintf(
			"Repository: %s\nDiff source: %s\n\nGit status:\n%s\n\nChange summary:\n%s\n\nPatch:\n%s\n\nReturn only the commit message.",
			repositoryLabel,
			strings.ToUpper(string(source)),
			fallbackText(gitContext.status, "No pending changes."),
			fallbackText(gitContext.summary, "No summary available."),
			fallbackText(gitContext.patch, "No diff available."),
		),
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

type gitContextFragments struct {
	status  string
	summary string
	patch   string
}

func (generator Generator) collectGitContext(ctx context.Context, repositoryPath string, source DiffSource) (gitContextFragments, error) {
	statusOutput, statusError := generator.runGit(ctx, repositoryPath, []string{"status", "--short"})
	if statusError != nil {
		return gitContextFragments{}, statusError
	}

	diffArguments := []string{"diff", "--unified=3"}
	switch source {
	case DiffSourceStaged:
		diffArguments = append(diffArguments, "--cached")
	case DiffSourceWorktree:
	default:
		return gitContextFragments{}, fmt.Errorf("unsupported diff source %q", source)
	}

	summaryOutput, summaryError := generator.runGit(ctx, repositoryPath, append(diffArguments, "--stat"))
	if summaryError != nil {
		return gitContextFragments{}, summaryError
	}

	patchOutput, patchError := generator.runGit(ctx, repositoryPath, diffArguments)
	if patchError != nil {
		return gitContextFragments{}, patchError
	}

	return gitContextFragments{
		status:  strings.TrimSpace(summaryTruncate(statusOutput, defaultPatchCharacterCap)),
		summary: strings.TrimSpace(summaryTruncate(summaryOutput, defaultPatchCharacterCap)),
		patch:   strings.TrimSpace(summaryTruncate(patchOutput, defaultPatchCharacterCap)),
	}, nil
}

func (generator Generator) runGit(ctx context.Context, repositoryPath string, arguments []string) (string, error) {
	result, execError := generator.GitExecutor.ExecuteGit(
		ctx,
		execshell.CommandDetails{
			Arguments:        arguments,
			WorkingDirectory: repositoryPath,
		},
	)
	if execError != nil {
		return "", execError
	}
	return result.StandardOutput, nil
}

func summaryTruncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "\n…"
}

func fallbackText(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
