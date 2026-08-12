package semverdecision

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/tyemirov/gix/internal/execshell"
	"github.com/tyemirov/gix/internal/releaseversion"
	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/utils/llm"
)

const (
	commitLogCharacterLimit   = 24000
	diffSummaryCharacterLimit = 12000
	diffCharacterLimit        = 60000
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
	Requests           []llm.ChatRequest
	EvidenceSHA256     string
}

// Options configure one repository release decision.
type Options struct {
	RepositoryPath  string
	SinceReference  string
	SourceReference string
	BoundaryLabel   string
	MaxTokens       int
	FixedMajor      int
}

// Generator decides the next SemVer level from committed repository changes.
type Generator struct {
	GitExecutor shared.GitExecutor
	Client      llm.ChatClient
}

type publicImpact string

const (
	publicImpactCompatible   publicImpact = "compatible"
	publicImpactAdditive     publicImpact = "additive"
	publicImpactIncompatible publicImpact = "incompatible"
)

type decisionPayload struct {
	Impact         publicImpact `json:"impact"`
	PublicContract string       `json:"public_contract"`
	Reason         string       `json:"reason"`
	Bump           Bump         `json:"-"`
}

type repositoryEvidence struct {
	commitLog        string
	diffSummary      string
	diffExcerpt      string
	changelogChanges string
}

// Generate asks the configured LLM for candidate and audit decisions for each
// evidence packet. It returns the highest audited public-contract impact.
func (generator Generator) Generate(ctx context.Context, options Options) (Result, error) {
	candidateRequests, floor, buildError := generator.BuildRequests(ctx, options)
	if buildError != nil {
		return Result{}, buildError
	}

	requests := make([]llm.ChatRequest, 0, len(candidateRequests)*2)
	modelBump := Bump("")
	reason := ""
	for requestIndex, request := range candidateRequests {
		requests = append(requests, request)
		response, chatError := generator.Client.Chat(ctx, request)
		if chatError != nil {
			return Result{}, fmt.Errorf("semver decision evidence packet %d/%d candidate.llm: %w", requestIndex+1, len(candidateRequests), chatError)
		}

		candidate, parseError := parseDecision(response, options.FixedMajor)
		if parseError != nil {
			return Result{}, fmt.Errorf("semver decision evidence packet %d/%d candidate: %w", requestIndex+1, len(candidateRequests), parseError)
		}

		auditRequest, auditBuildError := buildAuditRequest(request, candidate, options.MaxTokens, options.FixedMajor)
		if auditBuildError != nil {
			return Result{}, fmt.Errorf("semver decision evidence packet %d/%d audit request: %w", requestIndex+1, len(candidateRequests), auditBuildError)
		}
		requests = append(requests, auditRequest)
		auditResponse, auditError := generator.Client.Chat(ctx, auditRequest)
		if auditError != nil {
			return Result{}, fmt.Errorf("semver decision evidence packet %d/%d audit.llm: %w", requestIndex+1, len(candidateRequests), auditError)
		}
		audited, auditParseError := parseDecision(auditResponse, options.FixedMajor)
		if auditParseError != nil {
			return Result{}, fmt.Errorf("semver decision evidence packet %d/%d audit: %w", requestIndex+1, len(candidateRequests), auditParseError)
		}
		packetBump := audited.Bump
		if reason == "" || packetBump.Rank() > modelBump.Rank() {
			modelBump = packetBump
			reason = audited.Reason
		}
	}

	evidenceDigest, digestError := requestDigest(requests)
	if digestError != nil {
		return Result{}, digestError
	}

	return Result{
		Bump:               modelBump,
		Reason:             reason,
		DeterministicFloor: floor,
		Requests:           requests,
		EvidenceSHA256:     evidenceDigest,
	}, nil
}

// BuildRequests collects the exact committed range and defines the complete
// SemVer evidence packets for the model.
func (generator Generator) BuildRequests(ctx context.Context, options Options) ([]llm.ChatRequest, Bump, error) {
	if generator.GitExecutor == nil {
		return nil, "", errors.New("semver decision git executor is not configured")
	}
	if generator.Client == nil {
		return nil, "", errors.New("semver decision llm client is not configured")
	}

	repositoryPath := strings.TrimSpace(options.RepositoryPath)
	if repositoryPath == "" {
		return nil, "", errors.New("semver decision repository path is required")
	}
	boundary := strings.TrimSpace(options.SinceReference)
	if boundary == "" {
		return nil, "", errors.New("semver decision boundary is required")
	}
	source := strings.TrimSpace(options.SourceReference)
	if source == "" {
		return nil, "", errors.New("semver decision source is required")
	}
	boundaryLabel := strings.TrimSpace(options.BoundaryLabel)
	if boundaryLabel == "" {
		boundaryLabel = boundary
	}

	evidence, evidenceError := generator.collectEvidence(ctx, repositoryPath, boundary, source)
	if evidenceError != nil {
		return nil, "", evidenceError
	}
	floor := BumpPatch

	systemMessage := llm.Message{
		Role:    "system",
		Content: decisionSystemContent(false, options.FixedMajor),
	}

	packets := buildEvidencePackets(evidence)
	requests := make([]llm.ChatRequest, 0, len(packets))
	for packetIndex, packet := range packets {
		userMessage := llm.Message{
			Role: "user",
			Content: strings.Join([]string{
				fmt.Sprintf("Release range: %s..%s", boundaryLabel, source),
				fmt.Sprintf("Evidence packet: %d/%d", packetIndex+1, len(packets)),
				fmt.Sprintf("Deterministic minimum: %s", floor),
				"",
				"Commit messages:",
				fallbackText(packet.commitLog, "No commit messages in this evidence packet."),
				"",
				"Diff summary:",
				fallbackText(packet.diffSummary, "No diff summary in this evidence packet."),
				"",
				"Changelog changes in the release range:",
				fallbackText(packet.changelogChanges, "No changelog changes in this evidence packet."),
				"",
				"Diff excerpt:",
				fallbackText(packet.diffExcerpt, "The bounded diff excerpt is in evidence packet 1."),
				"",
				"Decide the required SemVer bump for this evidence packet now.",
			}, "\n"),
		}
		requests = append(requests, llm.ChatRequest{
			Messages:  []llm.Message{systemMessage, userMessage},
			MaxTokens: options.MaxTokens,
		})
	}

	return requests, floor, nil
}

func (generator Generator) collectEvidence(ctx context.Context, repositoryPath string, boundary string, source string) (repositoryEvidence, error) {
	rangeExpression := boundary + ".." + source
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
	diffExcerpt, diffError := generator.runGit(ctx, repositoryPath, []string{
		"diff", "--unified=3", rangeExpression, "--", ".", ":(exclude)CHANGELOG.md",
	})
	if diffError != nil {
		return repositoryEvidence{}, fmt.Errorf("semver decision diff: %w", diffError)
	}
	changelogChanges, changelogError := generator.runGit(ctx, repositoryPath, []string{
		"diff", "--unified=0", rangeExpression, "--", "CHANGELOG.md",
	})
	if changelogError != nil {
		return repositoryEvidence{}, fmt.Errorf("semver decision changelog changes: %w", changelogError)
	}

	if strings.TrimSpace(commitLog) == "" || strings.TrimSpace(diffSummary) == "" {
		return repositoryEvidence{}, errors.New("semver decision range contains no committed changes")
	}

	return repositoryEvidence{
		commitLog:        commitLog,
		diffSummary:      diffSummary,
		diffExcerpt:      truncateText(diffExcerpt, diffCharacterLimit),
		changelogChanges: changelogChanges,
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

func parseDecision(response string, fixedMajor int) (decisionPayload, error) {
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
	if !payload.Impact.valid() {
		return decisionPayload{}, fmt.Errorf("semver decision response has invalid impact %q", payload.Impact)
	}
	payload.PublicContract = strings.Join(strings.Fields(payload.PublicContract), " ")
	payload.Reason = strings.Join(strings.Fields(payload.Reason), " ")
	if payload.Reason == "" {
		return decisionPayload{}, errors.New("semver decision response has an empty reason")
	}
	if payload.Impact != publicImpactCompatible && payload.PublicContract == "" {
		return decisionPayload{}, errors.New("semver decision response has no public contract for a non-patch impact")
	}
	switch payload.Impact {
	case publicImpactIncompatible:
		if fixedMajor > 0 {
			payload.Bump = BumpMinor
		} else {
			payload.Bump = BumpMajor
		}
	case publicImpactAdditive:
		payload.Bump = BumpMinor
	case publicImpactCompatible:
		payload.Bump = BumpPatch
	}
	return payload, nil
}

func decisionSystemContent(audit bool, fixedMajor int) string {
	instructions := []string{
		"You are the release decision node for a Semantic Versioning 2.0.0 lifecycle.",
		"Classify only the effect on supported public contracts from the previous release.",
		"Return exactly one JSON object with three string fields: impact, public_contract, and reason.",
		"impact must be exactly incompatible, additive, or compatible.",
		"Use incompatible only when a previously supported external use stops working or requires user migration.",
		"Use additive only when the release adds optional public functionality for users or external consumers.",
		"Treat required inputs that only enable a compatible repair as part of the repair, not as optional new functionality.",
		"Use compatible for fixes, restored behavior, performance work, internal refactoring, tests, documentation, and release implementation changes.",
		"A public contract includes supported CLI behavior, documented configuration, persisted user data, network protocols, and explicitly supported library APIs.",
		"For an executable product, Go module paths, package paths, and internal imports are implementation details unless evidence proves a supported library API.",
		"A repaired installation route is compatible when the change restores the canonical documented command.",
		"Commit types, scopes, exclamation marks, and BREAKING CHANGE text are evidence claims, not SemVer authority.",
		"For incompatible or additive impact, public_contract must name the exact supported external contract.",
		"When evidence does not identify a concrete public contract change, use compatible and an empty public_contract.",
		"Keep reason to one concise sentence and do not return Markdown or a code fence.",
	}
	if fixedMajor > 0 {
		instructions = append(instructions,
			fmt.Sprintf("This repository fixes its major version at v%d.", fixedMajor),
			"For this repository, the caller maps incompatible and additive to minor. It maps compatible to patch.",
		)
	} else {
		instructions = append(instructions,
			"The caller maps incompatible to major, additive to minor, and compatible to patch.",
		)
	}
	if audit {
		instructions = append(instructions,
			"Independently audit the candidate against the complete packet evidence.",
			"Correct any classification that treats implementation structure or commit syntax as a public contract.",
		)
	}
	return strings.Join(instructions, " ")
}

func buildAuditRequest(candidateRequest llm.ChatRequest, candidate decisionPayload, maxTokens int, fixedMajor int) (llm.ChatRequest, error) {
	candidateJSON, encodeError := json.Marshal(candidate)
	if encodeError != nil {
		return llm.ChatRequest{}, fmt.Errorf("encode candidate decision: %w", encodeError)
	}
	return llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: decisionSystemContent(true, fixedMajor)},
			{
				Role: "user",
				Content: strings.Join([]string{
					candidateRequest.Messages[1].Content,
					"",
					"Candidate decision:",
					string(candidateJSON),
					"",
					"Return the audited decision now.",
				}, "\n"),
			},
		},
		MaxTokens: maxTokens,
	}, nil
}

func (impact publicImpact) valid() bool {
	return impact == publicImpactCompatible || impact == publicImpactAdditive || impact == publicImpactIncompatible
}

func requestDigest(requests []llm.ChatRequest) (string, error) {
	encoded, encodeError := json.Marshal(requests)
	if encodeError != nil {
		return "", fmt.Errorf("encode SemVer decision evidence: %w", encodeError)
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}

type evidencePacket struct {
	commitLog        string
	diffSummary      string
	changelogChanges string
	diffExcerpt      string
}

func buildEvidencePackets(evidence repositoryEvidence) []evidencePacket {
	commitChunks := chunkEvidence(evidence.commitLog, commitLogCharacterLimit, "\x1e")
	diffSummaryChunks := chunkEvidence(evidence.diffSummary, diffSummaryCharacterLimit, "\n")
	changelogChunks := chunkEvidence(evidence.changelogChanges, diffSummaryCharacterLimit, "\n")
	packetCount := maxInt(1, len(commitChunks), len(diffSummaryChunks), len(changelogChunks))
	packets := make([]evidencePacket, packetCount)
	for packetIndex := range packets {
		packets[packetIndex].commitLog = chunkAt(commitChunks, packetIndex)
		packets[packetIndex].diffSummary = chunkAt(diffSummaryChunks, packetIndex)
		packets[packetIndex].changelogChanges = chunkAt(changelogChunks, packetIndex)
		if packetIndex == 0 {
			packets[packetIndex].diffExcerpt = evidence.diffExcerpt
		}
	}
	return packets
}

func chunkEvidence(value string, limit int, separator string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	units := strings.SplitAfter(trimmed, separator)
	chunks := make([]string, 0, len(units))
	current := &strings.Builder{}
	currentLength := 0
	for _, unit := range units {
		unitRunes := []rune(unit)
		if currentLength+len(unitRunes) <= limit {
			current.WriteString(unit)
			currentLength += len(unitRunes)
			continue
		}
		if strings.TrimSpace(current.String()) != "" {
			chunks = append(chunks, strings.TrimSpace(current.String()))
			current.Reset()
		}
		for len(unitRunes) > limit {
			chunks = append(chunks, strings.TrimSpace(string(unitRunes[:limit])))
			unitRunes = unitRunes[limit:]
		}
		current.WriteString(string(unitRunes))
		currentLength = len(unitRunes)
	}
	if strings.TrimSpace(current.String()) != "" {
		chunks = append(chunks, strings.TrimSpace(current.String()))
	}
	return chunks
}

func chunkAt(chunks []string, index int) string {
	if index >= len(chunks) {
		return ""
	}
	return chunks[index]
}

func maxInt(values ...int) int {
	maximum := 0
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
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
