package semverdecision

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/tyemirov/gix/v5/internal/execshell"
	"github.com/tyemirov/gix/v5/internal/releaseversion"
	"github.com/tyemirov/gix/v5/internal/repos/shared"
	"github.com/tyemirov/utils/llm"
)

const (
	commitLogCharacterLimit   = 24000
	diffSummaryCharacterLimit = 12000
	diffCharacterLimit        = 60000
)

var (
	conventionalHeaderPattern = regexp.MustCompile(`(?m)^[a-z][a-z0-9-]*(?:\([^\r\n)]+\))?(!)?:[ \t]+\S`)
	featureHeaderPattern      = regexp.MustCompile(`(?m)^feat(?:\([^\r\n)]+\))?!?:[ \t]+\S`)
	breakingFooterPattern     = regexp.MustCompile(`(?m)^BREAKING(?: CHANGE|-CHANGE):[ \t]+\S`)
)

type Bump = releaseversion.Bump

const (
	BumpPatch = releaseversion.BumpPatch
	BumpMinor = releaseversion.BumpMinor
	BumpMajor = releaseversion.BumpMajor
)

// Result contains the final decision and its evidence.
type Result struct {
	Bump               Bump
	Reason             string
	DeterministicFloor Bump
	Request            llm.ChatRequest
	EvidenceSHA256     string
}

// Options configure one repository release decision.
type Options struct {
	RepositoryPath string
	SinceReference string
	MaxTokens      int
}

// Generator decides the next SemVer level from committed repository changes.
type Generator struct {
	GitExecutor shared.GitExecutor
	Client      llm.ChatClient
}

type decisionPayload struct {
	Bump   Bump   `json:"bump"`
	Reason string `json:"reason"`
}

type repositoryEvidence struct {
	commitLog       string
	diffSummary     string
	diffExcerpt     string
	unreleasedNotes string
}

// Generate asks the configured LLM for one closed decision and enforces the
// minimum level declared by Conventional Commit evidence.
func (generator Generator) Generate(ctx context.Context, options Options) (Result, error) {
	request, floor, buildError := generator.BuildRequest(ctx, options)
	if buildError != nil {
		return Result{}, buildError
	}

	response, chatError := generator.Client.Chat(ctx, request)
	if chatError != nil {
		return Result{}, fmt.Errorf("semver decision.llm: %w", chatError)
	}

	payload, parseError := parseDecision(response)
	if parseError != nil {
		return Result{}, parseError
	}

	finalBump := releaseversion.MaximumBump(payload.Bump, floor)
	reason := payload.Reason
	if finalBump != payload.Bump {
		reason = fmt.Sprintf("%s Conventional Commit evidence requires at least a %s release.", reason, floor)
	}
	evidenceDigest, digestError := requestDigest(request)
	if digestError != nil {
		return Result{}, digestError
	}

	return Result{
		Bump:               finalBump,
		Reason:             reason,
		DeterministicFloor: floor,
		Request:            request,
		EvidenceSHA256:     evidenceDigest,
	}, nil
}

// BuildRequest collects the exact committed range and defines the SemVer
// decision protocol for the model.
func (generator Generator) BuildRequest(ctx context.Context, options Options) (llm.ChatRequest, Bump, error) {
	if generator.GitExecutor == nil {
		return llm.ChatRequest{}, "", errors.New("semver decision git executor is not configured")
	}
	if generator.Client == nil {
		return llm.ChatRequest{}, "", errors.New("semver decision llm client is not configured")
	}

	repositoryPath := strings.TrimSpace(options.RepositoryPath)
	if repositoryPath == "" {
		return llm.ChatRequest{}, "", errors.New("semver decision repository path is required")
	}
	boundary := strings.TrimSpace(options.SinceReference)
	if boundary == "" {
		return llm.ChatRequest{}, "", errors.New("semver decision boundary is required")
	}

	evidence, evidenceError := generator.collectEvidence(ctx, repositoryPath, boundary)
	if evidenceError != nil {
		return llm.ChatRequest{}, "", evidenceError
	}
	floor := deterministicFloor(evidence.commitLog)

	systemMessage := llm.Message{
		Role: "system",
		Content: strings.Join([]string{
			"You are the release decision node for a Semantic Versioning 2.0.0 lifecycle.",
			"Return exactly one JSON object with two string fields: bump and reason.",
			"bump must be exactly major, minor, or patch.",
			"Select major when any committed change is incompatible with a current public contract, including removed or renamed APIs, CLI behavior, configuration keys, schemas, persisted data, protocols, or required operator behavior.",
			"Select minor when the release adds backward-compatible public functionality without any incompatible change.",
			"Select patch when all changes are backward-compatible fixes, performance work, internal refactoring, tests, or documentation and add no public functionality.",
			"Use the highest level required by any change in the complete range.",
			"Inspect behavior and diffs instead of trusting commit type labels alone.",
			"Keep reason to one concise sentence and do not return Markdown or a code fence.",
		}, " "),
	}

	userMessage := llm.Message{
		Role: "user",
		Content: strings.Join([]string{
			fmt.Sprintf("Release range: %s..HEAD", boundary),
			fmt.Sprintf("Deterministic Conventional Commit floor: %s", floor),
			"",
			"Commit messages:",
			fallbackText(truncateText(evidence.commitLog, commitLogCharacterLimit), "No commit messages."),
			"",
			"Diff summary:",
			fallbackText(evidence.diffSummary, "No diff summary."),
			"",
			"Unreleased changelog notes:",
			fallbackText(evidence.unreleasedNotes, "No Unreleased changelog notes."),
			"",
			"Diff excerpt:",
			fallbackText(evidence.diffExcerpt, "No diff excerpt."),
			"",
			"Decide the required SemVer bump now.",
		}, "\n"),
	}

	return llm.ChatRequest{
		Messages:  []llm.Message{systemMessage, userMessage},
		MaxTokens: options.MaxTokens,
	}, floor, nil
}

func (generator Generator) collectEvidence(ctx context.Context, repositoryPath string, boundary string) (repositoryEvidence, error) {
	rangeExpression := boundary + "..HEAD"
	commitLog, commitError := generator.runGit(ctx, repositoryPath, []string{
		"log", "--pretty=format:%s%n%b%x1e", rangeExpression,
	})
	if commitError != nil {
		return repositoryEvidence{}, fmt.Errorf("semver decision commit log: %w", commitError)
	}
	diffSummary, summaryError := generator.runGit(ctx, repositoryPath, []string{"diff", "--stat", rangeExpression})
	if summaryError != nil {
		return repositoryEvidence{}, fmt.Errorf("semver decision diff summary: %w", summaryError)
	}
	diffExcerpt, diffError := generator.runGit(ctx, repositoryPath, []string{"diff", "--unified=3", rangeExpression})
	if diffError != nil {
		return repositoryEvidence{}, fmt.Errorf("semver decision diff: %w", diffError)
	}
	changelog, changelogError := generator.runGit(ctx, repositoryPath, []string{"show", "HEAD:CHANGELOG.md"})
	if changelogError != nil {
		return repositoryEvidence{}, fmt.Errorf("semver decision changelog: %w", changelogError)
	}

	if strings.TrimSpace(commitLog) == "" || strings.TrimSpace(diffSummary) == "" {
		return repositoryEvidence{}, errors.New("semver decision range contains no committed changes")
	}

	return repositoryEvidence{
		commitLog:       commitLog,
		diffSummary:     truncateText(diffSummary, diffSummaryCharacterLimit),
		diffExcerpt:     truncateText(diffExcerpt, diffCharacterLimit),
		unreleasedNotes: extractUnreleasedNotes(changelog),
	}, nil
}

func (generator Generator) runGit(ctx context.Context, repositoryPath string, arguments []string) (string, error) {
	result, executionError := generator.GitExecutor.ExecuteGit(ctx, execshell.CommandDetails{
		Arguments:        arguments,
		WorkingDirectory: repositoryPath,
	})
	if executionError != nil {
		return "", executionError
	}
	return result.StandardOutput, nil
}

func parseDecision(response string) (decisionPayload, error) {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(response)))
	decoder.DisallowUnknownFields()
	payload := decisionPayload{}
	if decodeError := decoder.Decode(&payload); decodeError != nil {
		return decisionPayload{}, fmt.Errorf("semver decision response is not the required JSON object: %w", decodeError)
	}
	var trailingValue any
	if trailingError := decoder.Decode(&trailingValue); !errors.Is(trailingError, io.EOF) {
		return decisionPayload{}, errors.New("semver decision response contains extra JSON values")
	}
	if !payload.Bump.Valid() {
		return decisionPayload{}, fmt.Errorf("semver decision response has invalid bump %q", payload.Bump)
	}
	payload.Reason = strings.Join(strings.Fields(payload.Reason), " ")
	if payload.Reason == "" {
		return decisionPayload{}, errors.New("semver decision response has an empty reason")
	}
	return payload, nil
}

func deterministicFloor(commitLog string) Bump {
	if breakingFooterPattern.MatchString(commitLog) {
		return BumpMajor
	}
	for _, match := range conventionalHeaderPattern.FindAllStringSubmatch(commitLog, -1) {
		if len(match) > 1 && match[1] == "!" {
			return BumpMajor
		}
	}
	if featureHeaderPattern.MatchString(commitLog) {
		return BumpMinor
	}
	return BumpPatch
}

func requestDigest(request llm.ChatRequest) (string, error) {
	encoded, encodeError := json.Marshal(request)
	if encodeError != nil {
		return "", fmt.Errorf("encode SemVer decision evidence: %w", encodeError)
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}

func extractUnreleasedNotes(changelog string) string {
	lines := strings.Split(changelog, "\n")
	start := -1
	for index, line := range lines {
		if strings.TrimSpace(line) == "## [Unreleased]" {
			start = index + 1
			break
		}
	}
	if start < 0 {
		return ""
	}
	end := len(lines)
	for index := start; index < len(lines); index++ {
		if strings.HasPrefix(strings.TrimSpace(lines[index]), "## ") {
			end = index
			break
		}
	}
	return truncateText(strings.Join(lines[start:end], "\n"), diffSummaryCharacterLimit)
}

func truncateText(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	runes := []rune(trimmed)
	if limit <= 0 || len(runes) <= limit {
		return trimmed
	}
	return strings.TrimSpace(string(runes[:limit])) + "\n..."
}

func fallbackText(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
