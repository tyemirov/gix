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
	evidencePacketRuneLimit   = 32000
	submittedRequestRuneLimit = 64000
	contextRuneLimit          = 24000
	providerCallLimit         = 128
	contextRoundLimit         = 2
)

type Bump = releaseversion.Bump

const (
	BumpPatch = releaseversion.BumpPatch
	BumpMinor = releaseversion.BumpMinor
	BumpMajor = releaseversion.BumpMajor
)

// Result contains the final decision and its evidence.
type Result struct {
	SinceReference     string             `json:"since_reference"`
	SourceReference    string             `json:"source_reference"`
	Bump               Bump               `json:"bump"`
	Reason             string             `json:"reason"`
	DeterministicFloor Bump               `json:"deterministic_floor"`
	Requests           []llm.ChatRequest  `json:"requests"`
	Responses          []ProviderResponse `json:"responses"`
	EvidenceSHA256     string             `json:"evidence_sha256"`
	Packets            []EvidencePacket   `json:"packets"`
	Findings           []Finding          `json:"findings"`
	Context            []ContextEvidence  `json:"context"`
	Coverage           []EvidenceCoverage `json:"coverage"`
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
	publicImpactUnresolved   publicImpact = "unresolved"
)

type decisionPayload struct {
	Impact          publicImpact     `json:"impact"`
	PublicContract  string           `json:"public_contract"`
	Reason          string           `json:"reason"`
	Bump            Bump             `json:"-"`
	ContextRequests []ContextRequest `json:"context_requests,omitempty"`
}

type repositoryEvidence struct {
	commitLog        string
	diffSummary      string
	diffExcerpt      string
	changelogChanges string
}

// Generate asks the configured LLM for candidate and audit decisions for each
// evidence packet. It returns the highest audited public-contract impact.
func (generator Generator) Generate(ctx context.Context, options Options) (result Result, returnedError error) {
	result.SinceReference, result.SourceReference = options.SinceReference, options.SourceReference
	defer func() {
		digest, digestError := requestDigest(result.Requests)
		result.EvidenceSHA256 = digest
		if returnedError == nil && digestError != nil {
			returnedError = digestError
		}
	}()
	requests, floor, packets, coverage, buildError := generator.buildRequests(ctx, options)
	result.DeterministicFloor, result.Packets, result.Coverage = floor, packets, coverage
	if buildError != nil {
		return result, buildError
	}
	if len(requests)*2 > providerCallLimit {
		return result, fmt.Errorf("semver decision complete evidence requires %d calls; provider call limit is %d", len(requests)*2, providerCallLimit)
	}
	for _, packet := range packets {
		for _, fragment := range packet.Fragments {
			if fragment.Kind != "Diff excerpt" && fragment.Kind != "Changelog changes in the release range" {
				continue
			}
			for _, line := range strings.Split(fragment.Content, "\n") {
				if strings.HasPrefix(line, "Binary files ") || line == "GIT binary patch" {
					return result, fmt.Errorf("semver decision packet %s contains unsupported binary evidence; public impact remains unresolved", packet.ID)
				}
			}
		}
	}
	for index, request := range requests {
		packetContext := []ContextEvidence{}
		candidate, stageError := generator.analyzeStage(ctx, options, request, packets[index].ID, "candidate", &packetContext, &result)
		if stageError != nil {
			return result, fmt.Errorf("semver decision evidence packet %d/%d %w", index+1, len(requests), stageError)
		}
		auditRequest, auditError := buildAuditRequest(request, candidate, options.MaxTokens, options.FixedMajor)
		if auditError != nil {
			return result, auditError
		}
		audited, stageError := generator.analyzeStage(ctx, options, auditRequest, packets[index].ID, "audit", &packetContext, &result)
		if stageError != nil {
			return result, fmt.Errorf("semver decision evidence packet %d/%d %w", index+1, len(requests), stageError)
		}
		if result.Reason == "" || audited.Bump.Rank() > result.Bump.Rank() {
			result.Bump, result.Reason = audited.Bump, audited.Reason
		}
	}
	return result, nil
}

// BuildRequests collects the committed range and creates bounded evidence packets.
func (generator Generator) BuildRequests(ctx context.Context, options Options) ([]llm.ChatRequest, Bump, error) {
	requests, floor, _, _, err := generator.buildRequests(ctx, options)
	return requests, floor, err
}

func (generator Generator) buildRequests(ctx context.Context, options Options) ([]llm.ChatRequest, Bump, []EvidencePacket, []EvidenceCoverage, error) {
	if generator.GitExecutor == nil {
		return nil, "", nil, nil, errors.New("semver decision git executor is not configured")
	}
	if generator.Client == nil {
		return nil, "", nil, nil, errors.New("semver decision llm client is not configured")
	}
	if strings.TrimSpace(options.RepositoryPath) == "" {
		return nil, "", nil, nil, errors.New("semver decision repository path is required")
	}
	if strings.TrimSpace(options.SinceReference) == "" {
		return nil, "", nil, nil, errors.New("semver decision boundary is required")
	}
	if strings.TrimSpace(options.SourceReference) == "" {
		return nil, "", nil, nil, errors.New("semver decision source is required")
	}
	evidence, err := generator.collectEvidence(ctx, options.RepositoryPath, options.SinceReference, options.SourceReference)
	if err != nil {
		return nil, "", nil, nil, err
	}
	packets, coverage, err := buildEvidencePackets(evidence, options)
	if err != nil {
		return nil, BumpPatch, packets, coverage, err
	}
	requests := make([]llm.ChatRequest, 0, len(packets))
	for _, packet := range packets {
		requests = append(requests, llm.ChatRequest{Messages: []llm.Message{
			{Role: "system", Content: decisionSystemContent(false, options.FixedMajor)},
			{Role: "user", Content: packet.Content},
		}, MaxTokens: options.MaxTokens})
	}
	return requests, BumpPatch, packets, coverage, nil
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
		diffExcerpt:      diffExcerpt,
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
	if (payload.Impact == publicImpactIncompatible || payload.Impact == publicImpactAdditive) && payload.PublicContract == "" {
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
		"Return exactly one JSON object with string fields impact, public_contract, and reason; optional context_requests is an array of source requests.",
		"A context request has path (repository-relative), source (old or new), start_line (one-based), and line_count (positive). Request only committed source relevant to an unresolved public behavior.",
		"impact must be exactly incompatible, additive, compatible, or unresolved.",
		"Use unresolved when material evidence is incomplete, a fragment needs related source, or public impact cannot be established. Request bounded context rather than assume compatibility.",
		"Use incompatible only when a previously supported external use stops working or requires user migration.",
		"Use additive only when the release adds optional public functionality for users or external consumers.",
		"A newly required input is incompatible when an existing supported use needs migration, even if the change is called a bug fix.",
		"Use compatible for fixes, restored behavior, performance work, internal refactoring, tests, documentation, and release implementation changes.",
		"A public contract includes supported CLI behavior, documented configuration, persisted user data, network protocols, and explicitly supported library APIs.",
		"For an executable product, Go module paths, package paths, and internal imports are implementation details unless evidence proves a supported library API.",
		"A repaired installation route is compatible when the change restores the canonical documented command.",
		"Commit types, scopes, exclamation marks, and BREAKING CHANGE text are evidence claims, not SemVer authority.",
		"For incompatible or additive impact, public_contract must name the exact supported external contract.",
		"Use compatible with an empty public_contract only when evidence establishes implementation-only changes; absent evidence does not establish compatibility.",
		"Review every supplied fragment. Correlate file and hunk continuations with committed source when needed. No file type alone establishes public impact.",
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
	return impact == publicImpactCompatible || impact == publicImpactAdditive || impact == publicImpactIncompatible || impact == publicImpactUnresolved
}

func requestDigest(requests []llm.ChatRequest) (string, error) {
	encoded, encodeError := json.Marshal(requests)
	if encodeError != nil {
		return "", fmt.Errorf("encode SemVer decision evidence: %w", encodeError)
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}
