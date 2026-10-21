package semverdecision

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode"

	"github.com/tyemirov/utils/llm"
)

// ContextRequest identifies a range in one committed source file.
type ContextRequest struct {
	Path      string `json:"path"`
	Source    string `json:"source"`
	StartLine int    `json:"start_line"`
	LineCount int    `json:"line_count"`
}

// ContextEvidence records the source supplied for one context request.
type ContextEvidence struct {
	ContextRequest
	Content string `json:"content"`
	SHA256  string `json:"sha256"`
	Absent  bool   `json:"absent"`
}

// ProviderResponse records a model response before JSON validation.
type ProviderResponse struct {
	PacketID string `json:"packet_id"`
	Stage    string `json:"stage"`
	Content  string `json:"content"`
}

// Finding records one model response and its source packet.
type Finding struct {
	PacketID        string           `json:"packet_id"`
	Stage           string           `json:"stage"`
	Impact          string           `json:"impact"`
	PublicContract  string           `json:"public_contract"`
	Reason          string           `json:"reason"`
	ContextRequests []ContextRequest `json:"context_requests,omitempty"`
}

func (generator Generator) analyzeStage(ctx context.Context, options Options, initial llm.ChatRequest, packetID, stage string, packetContext *[]ContextEvidence, result *Result) (decisionPayload, error) {
	for round := 0; ; round++ {
		request := initial
		request.Messages = append([]llm.Message(nil), initial.Messages...)
		if len(*packetContext) > 0 {
			request.Messages[1].Content += "\nCommitted source context:\n"
			for _, evidence := range *packetContext {
				request.Messages[1].Content += formatContext(evidence)
			}
		}
		if len(result.Requests) >= providerCallLimit {
			return decisionPayload{}, fmt.Errorf("provider call limit %d exhausted; complete evidence remains unresolved", providerCallLimit)
		}
		result.Requests = append(result.Requests, request)
		requestRunes := 0
		for _, message := range request.Messages {
			requestRunes += len([]rune(message.Content))
		}
		if requestRunes > submittedRequestRuneLimit {
			return decisionPayload{}, fmt.Errorf("%s submitted request exceeds %d rune limit; public impact remains unresolved", stage, submittedRequestRuneLimit)
		}
		response, err := generator.Client.Chat(ctx, request)
		result.Responses = append(result.Responses, ProviderResponse{PacketID: packetID, Stage: stage, Content: response})
		if err != nil {
			return decisionPayload{}, fmt.Errorf("%s.llm: %w", stage, err)
		}
		payload, err := parseDecision(response, options.FixedMajor)
		if err != nil {
			return decisionPayload{}, fmt.Errorf("%s: %w", stage, err)
		}
		result.Findings = append(result.Findings, Finding{PacketID: packetID, Stage: stage, Impact: string(payload.Impact), PublicContract: payload.PublicContract, Reason: payload.Reason, ContextRequests: payload.ContextRequests})
		if len(payload.ContextRequests) == 0 {
			if payload.Impact == publicImpactUnresolved {
				return decisionPayload{}, fmt.Errorf("public contract impact remains unresolved: %s", payload.Reason)
			}
			return payload, nil
		}
		if round >= contextRoundLimit {
			return decisionPayload{}, fmt.Errorf("source context round limit %d exhausted; public impact remains unresolved", contextRoundLimit)
		}
		for _, requested := range payload.ContextRequests {
			evidence, err := generator.retrieveContext(ctx, options, requested)
			if err != nil {
				return decisionPayload{}, fmt.Errorf("source context: %w", err)
			}
			contextRunes := len([]rune(formatContext(evidence)))
			for _, prior := range result.Context {
				contextRunes += len([]rune(formatContext(prior)))
			}
			if contextRunes > contextRuneLimit {
				return decisionPayload{}, fmt.Errorf("source context exceeds global %d rune limit", contextRuneLimit)
			}
			result.Context = append(result.Context, evidence)
			*packetContext = append(*packetContext, evidence)
		}
	}
}

func (generator Generator) retrieveContext(ctx context.Context, options Options, request ContextRequest) (ContextEvidence, error) {
	if err := validateContextRequest(request); err != nil {
		return ContextEvidence{}, err
	}
	reference := options.SinceReference
	if request.Source == "new" {
		reference = options.SourceReference
	}
	names, err := generator.runGit(ctx, options.RepositoryPath, []string{"ls-tree", "-z", "--name-only", reference, "--", request.Path})
	if err != nil {
		return ContextEvidence{}, fmt.Errorf("locate %s:%s: %w", reference, request.Path, err)
	}
	if names == "" {
		evidence := ContextEvidence{ContextRequest: request, Absent: true, Content: "File is absent from this committed source."}
		evidence.SHA256 = textDigest(evidence.Content)
		return evidence, nil
	}
	if strings.TrimSuffix(names, "\x00") != request.Path {
		return ContextEvidence{}, fmt.Errorf("context path %q is not one committed file", request.Path)
	}
	objectType, err := generator.runGit(ctx, options.RepositoryPath, []string{"cat-file", "-t", reference + ":" + request.Path})
	if err != nil {
		return ContextEvidence{}, fmt.Errorf("identify %s:%s: %w", reference, request.Path, err)
	}
	if strings.TrimSpace(objectType) != "blob" {
		return ContextEvidence{}, fmt.Errorf("context path %q is not a committed blob", request.Path)
	}
	content, err := generator.runGit(ctx, options.RepositoryPath, []string{"show", reference + ":" + request.Path})
	if err != nil {
		return ContextEvidence{}, fmt.Errorf("read %s:%s: %w", reference, request.Path, err)
	}
	if strings.ContainsRune(content, '\x00') {
		return ContextEvidence{}, fmt.Errorf("context path %q contains unsupported binary source", request.Path)
	}
	lines := strings.SplitAfter(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	start := request.StartLine - 1
	if start >= len(lines) || request.LineCount > len(lines)-start {
		return ContextEvidence{}, fmt.Errorf("context range for %q exceeds committed file lines", request.Path)
	}
	selected := strings.Join(lines[start:start+request.LineCount], "")
	return ContextEvidence{ContextRequest: request, Content: selected, SHA256: textDigest(selected)}, nil
}

func validateContextRequest(request ContextRequest) error {
	if request.Source != "old" && request.Source != "new" {
		return errors.New("context source must be old or new")
	}
	if request.Path == "" || path.IsAbs(request.Path) || path.Clean(request.Path) != request.Path || request.Path == "." || request.Path == ".." || strings.HasPrefix(request.Path, "../") || strings.Contains(request.Path, "\\") || strings.Contains(request.Path, ":") {
		return fmt.Errorf("unsafe repository context path %q", request.Path)
	}
	for _, character := range request.Path {
		if unicode.IsControl(character) {
			return errors.New("context path contains a control character")
		}
	}
	if request.StartLine < 1 || request.LineCount < 1 {
		return errors.New("context line range must be positive")
	}
	return nil
}

func formatContext(evidence ContextEvidence) string {
	return fmt.Sprintf("Source %s, path %s, lines %d count %d, absent=%t:\n%s\n", evidence.Source, evidence.Path, evidence.StartLine, evidence.LineCount, evidence.Absent, evidence.Content)
}
