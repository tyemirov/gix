package release

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tyemirov/gix/internal/repos/shared"
	"github.com/tyemirov/gix/internal/semverdecision"
)

const releaseEvidenceContract = "gix.release-evidence/v1"
const releaseEvidenceDirectory = "gix/release-decisions"

type evidenceModel struct {
	Connection string `json:"connection"`
	Priority   int    `json:"priority"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model"`
	Effort     string `json:"effort,omitempty"`
	MaxTokens  int    `json:"max_tokens"`
}

type releaseEvidenceRecord struct {
	Contract   string                `json:"contract"`
	RecordedAt time.Time             `json:"recorded_at"`
	ToolBuild  *debug.BuildInfo      `json:"tool_build,omitempty"`
	Decision   VersionDecision       `json:"decision"`
	Models     []evidenceModel       `json:"configured_models"`
	Analysis   semverdecision.Result `json:"analysis"`
	Failure    string                `json:"failure,omitempty"`
}

func (builder NextCommandBuilder) prepareSemVerRecorder(command *cobra.Command, executor shared.GitExecutor, root string) (func(VersionDecision, semverdecision.Result, NextConfiguration, error) error, error) {
	if builder.DecisionRecorder != nil {
		return builder.DecisionRecorder, nil
	}
	storage, err := executeGit(command.Context(), executor, root, "rev-parse", "--git-path", releaseEvidenceDirectory)
	if err != nil {
		return nil, fmt.Errorf("resolve release evidence directory: %w", err)
	}
	storage = strings.TrimSpace(storage)
	if storage == "" {
		return nil, fmt.Errorf("resolve release evidence directory: Git returned an empty path")
	}
	if !filepath.IsAbs(storage) {
		storage = filepath.Join(root, storage)
	}
	return func(decision VersionDecision, analysis semverdecision.Result, configuration NextConfiguration, analysisError error) error {
		return saveSemVerDecision(storage, decision, analysis, configuration, analysisError)
	}, nil
}

func saveSemVerDecision(storage string, decision VersionDecision, analysis semverdecision.Result, configuration NextConfiguration, analysisError error) error {
	record := releaseEvidenceRecord{
		Contract:   releaseEvidenceContract,
		RecordedAt: time.Now().UTC(),
		Decision:   decision,
		Analysis:   analysis,
	}
	record.ToolBuild, _ = debug.ReadBuildInfo()
	if analysisError != nil {
		record.Failure = analysisError.Error()
	}
	profiles := configuration.ConnectionProfiles
	if profiles.OpenAI.Credential != "" {
		record.Models = append(record.Models, configuredEvidenceModel("openai", profiles.OpenAI.Priority, "", profiles.OpenAI.Model, profiles.OpenAI.Effort, profiles.OpenAI.MaxCompletionTokens, configuration))
	}
	if profiles.LLMProxy.Credential != "" {
		provider, model := profiles.LLMProxy.Provider, profiles.LLMProxy.Model
		if configuration.LLMProxy.Provider != "" {
			provider = configuration.LLMProxy.Provider
		}
		if configuration.LLMProxy.Model != "" {
			model = configuration.LLMProxy.Model
		}
		record.Models = append(record.Models, configuredEvidenceModel("llm_proxy", profiles.LLMProxy.Priority, provider, model, profiles.LLMProxy.Effort, profiles.LLMProxy.MaxCompletionTokens, configuration))
	}
	content, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode release evidence: %w", err)
	}
	if err := os.MkdirAll(storage, 0o755); err != nil {
		return fmt.Errorf("create release evidence directory: %w", err)
	}
	file, err := os.CreateTemp(storage, "decision-*.json")
	if err != nil {
		return fmt.Errorf("create release evidence record: %w", err)
	}
	defer os.Remove(file.Name())
	_, writeError := file.Write(content)
	closeError := file.Close()
	if writeError != nil {
		return fmt.Errorf("write release evidence: %w", writeError)
	}
	if closeError != nil {
		return fmt.Errorf("close release evidence: %w", closeError)
	}
	identity := sha256.Sum256(content)
	if err := os.Rename(file.Name(), filepath.Join(storage, fmt.Sprintf("%x.json", identity))); err != nil {
		return fmt.Errorf("save release evidence: %w", err)
	}
	return nil
}

func configuredEvidenceModel(connection string, priority int, provider, model, effort string, maxTokens int, configuration NextConfiguration) evidenceModel {
	if configuration.Effort != "" {
		effort = configuration.Effort
	}
	if configuration.MaxTokens > 0 {
		maxTokens = configuration.MaxTokens
	}
	return evidenceModel{Connection: connection, Priority: priority, Provider: provider, Model: model, Effort: effort, MaxTokens: maxTokens}
}
