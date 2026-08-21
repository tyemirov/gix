package syncflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tyemirov/gix/internal/execshell"
)

type mergeConflictIndexCheckExecutor struct {
	commands          []execshell.CommandDetails
	conflictsResolved bool
}

func (executor *mergeConflictIndexCheckExecutor) ExecuteGit(_ context.Context, details execshell.CommandDetails) (execshell.ExecutionResult, error) {
	executor.commands = append(executor.commands, details)
	command := strings.Join(details.Arguments, " ")
	switch command {
	case "diff --name-only --diff-filter=U":
		if executor.conflictsResolved {
			return execshell.ExecutionResult{}, nil
		}
		return execshell.ExecutionResult{StandardOutput: ".mprlab/ISSUES.md\n"}, nil
	case "ls-files -u -- .mprlab/ISSUES.md":
		return execshell.ExecutionResult{
			StandardOutput: "100644 aaaaaaa 1\t.mprlab/ISSUES.md\n100644 bbbbbbb 2\t.mprlab/ISSUES.md\n100644 ccccccc 3\t.mprlab/ISSUES.md\n",
		}, nil
	case "show :1:.mprlab/ISSUES.md":
		return execshell.ExecutionResult{StandardOutput: "stable prefix\nstable suffix\n"}, nil
	case "show :2:.mprlab/ISSUES.md":
		return execshell.ExecutionResult{StandardOutput: "stable prefix\nlocal insertion\nstable suffix\n"}, nil
	case "show :3:.mprlab/ISSUES.md":
		return execshell.ExecutionResult{StandardOutput: "stable prefix\nincoming insertion\nstable suffix\n"}, nil
	case "checkout --conflict=diff3 -- .mprlab/ISSUES.md":
		return execshell.ExecutionResult{}, nil
	case "add -- .mprlab/ISSUES.md":
		executor.conflictsResolved = true
		return execshell.ExecutionResult{}, nil
	case "rev-parse --verify --end-of-options MERGE_HEAD^{commit}":
		return execshell.ExecutionResult{StandardOutput: "dddddddddddddddddddddddddddddddddddddddd\n"}, nil
	case "diff --cached --check":
		return execshell.ExecutionResult{}, errors.New("trailing whitespace in staged resolution")
	case "diff --cached --name-only --no-renames -z --":
		return execshell.ExecutionResult{StandardOutput: ".mprlab/ISSUES.md\x00"}, nil
	case "diff --cached --check -- .mprlab/ISSUES.md":
		return execshell.ExecutionResult{}, errors.New("trailing whitespace in staged resolution")
	case "diff --cached --check dddddddddddddddddddddddddddddddddddddddd -- .mprlab/ISSUES.md":
		return execshell.ExecutionResult{}, errors.New("trailing whitespace added by merge resolution")
	case "commit --no-edit":
		return execshell.ExecutionResult{}, errors.New("commit must not run after failed cached diff validation")
	default:
		return execshell.ExecutionResult{}, nil
	}
}

func (executor *mergeConflictIndexCheckExecutor) ExecuteGitHubCLI(context.Context, execshell.CommandDetails) (execshell.ExecutionResult, error) {
	return execshell.ExecutionResult{}, nil
}

func TestParseMergeConflictDocumentReconstructsMultipleDiff3Regions(t *testing.T) {
	content := "stable prefix\n" +
		"<<<<<<< HEAD\n" +
		"ours first\n" +
		"||||||| parent\n" +
		"base first\n" +
		"=======\n" +
		"theirs first\n" +
		">>>>>>> origin/master\n" +
		"stable middle\n" +
		"<<<<<<< HEAD\n" +
		"ours insertion\n" +
		"||||||| parent\n" +
		"=======\n" +
		"theirs insertion\n" +
		">>>>>>> origin/master\n" +
		"stable suffix\n"

	document, parseErr := parseMergeConflictDocument(content)

	require.NoError(t, parseErr)
	require.Equal(t, []string{"stable prefix\n", "stable middle\n", "stable suffix\n"}, document.NonConflictingRegions)
	require.Equal(
		t,
		[]mergeConflictRegion{
			{
				Ours:        "ours first\n",
				Base:        "base first\n",
				BasePresent: true,
				Theirs:      "theirs first\n",
			},
			{
				Ours:        "ours insertion\n",
				BasePresent: true,
				Theirs:      "theirs insertion\n",
			},
		},
		document.ConflictRegions,
	)
	require.Equal(
		t,
		"stable prefix\nresolved first\nstable middle\nours insertion\ntheirs insertion\nstable suffix\n",
		document.resolve([]string{"resolved first\n", "ours insertion\ntheirs insertion\n"}),
	)
}

func TestParseMergeConflictDocumentRejectsInvalidMarkerStructures(t *testing.T) {
	testCases := map[string]string{
		"unexpected marker": "stable\n=======\nstable\n",
		"invalid ours":      "<<<<<<< HEAD\nours\n<<<<<<< nested\n=======\ntheirs\n>>>>>>> source\n",
		"invalid base":      "<<<<<<< HEAD\nours\n||||||| base\n>>>>>>> source\n=======\ntheirs\n>>>>>>> source\n",
		"invalid theirs":    "<<<<<<< HEAD\nours\n=======\ntheirs\n=======\n>>>>>>> source\n",
		"unterminated":      "<<<<<<< HEAD\nours\n=======\ntheirs\n",
	}

	for testName, content := range testCases {
		t.Run(testName, func(t *testing.T) {
			_, parseErr := parseMergeConflictDocument(content)
			require.Error(t, parseErr)
		})
	}
}

func TestMergeConflictResolutionContentPreservesBoundaryWhitespace(t *testing.T) {
	expectedContent := "\nresolved region\n\n"
	response := "\n" +
		mergeConflictResolutionContentBegin + "\n" +
		expectedContent +
		"\n" + mergeConflictResolutionContentEnd +
		"\n"

	content, contentErr := mergeConflictResolutionContent("ISSUES.md", response)

	require.NoError(t, contentErr)
	require.Equal(t, expectedContent, content)
}

func TestMergeConflictResolutionContentRejectsMissingEnvelopeAndAcceptsEmptyRegion(t *testing.T) {
	_, missingEnvelopeErr := mergeConflictResolutionContent("ISSUES.md", "resolved region")
	require.Error(t, missingEnvelopeErr)
	require.Contains(t, missingEnvelopeErr.Error(), "required content envelope")

	adjacentSentinelResponse := mergeConflictResolutionContentBegin + "\n" + mergeConflictResolutionContentEnd
	adjacentContent, adjacentContentErr := mergeConflictResolutionContent("ISSUES.md", adjacentSentinelResponse)
	require.NoError(t, adjacentContentErr)
	require.Empty(t, adjacentContent)

	blankLineResponse := mergeConflictResolutionContentBegin + "\n\n" + mergeConflictResolutionContentEnd
	blankLineContent, blankLineContentErr := mergeConflictResolutionContent("ISSUES.md", blankLineResponse)
	require.NoError(t, blankLineContentErr)
	require.Empty(t, blankLineContent)
}

func TestValidateMergeConflictRegionResponseContracts(t *testing.T) {
	additiveRegion := mergeConflictRegion{
		Ours:        "ours insertion\n",
		BasePresent: true,
		Theirs:      "theirs insertion\n",
	}
	require.NoError(t, validateMergeConflictRegionResponse("ISSUES.md", 0, additiveRegion, additiveRegion.Ours+additiveRegion.Theirs))
	require.NoError(t, validateMergeConflictRegionResponse("ISSUES.md", 0, additiveRegion, additiveRegion.Theirs+additiveRegion.Ours))

	additiveLossErr := validateMergeConflictRegionResponse("ISSUES.md", 0, additiveRegion, additiveRegion.Ours)
	require.Error(t, additiveLossErr)
	require.Contains(t, additiveLossErr.Error(), "not an exact ordering of OURS and THEIRS")
	require.NotErrorIs(t, additiveLossErr, errMergeConflictReplacementIntentProofUnavailable)

	nonAdditiveRegion := mergeConflictRegion{
		Ours:        "reviewers: alice, bob\n",
		Base:        "reviewers: alice\n",
		BasePresent: true,
		Theirs:      "reviewers: alice, carol\n",
	}
	require.NoError(t, validateMergeConflictRegionResponse("ISSUES.md", 1, nonAdditiveRegion, "reviewers: alice, bob, carol\n"))

	oursLossErr := validateMergeConflictRegionResponse("ISSUES.md", 1, nonAdditiveRegion, nonAdditiveRegion.Theirs)
	require.Error(t, oursLossErr)
	require.Contains(t, oursLossErr.Error(), "does not preserve OURS replacement intent")
	require.ErrorIs(t, oursLossErr, errMergeConflictReplacementIntentProofUnavailable)

	theirsLossErr := validateMergeConflictRegionResponse("ISSUES.md", 1, nonAdditiveRegion, nonAdditiveRegion.Ours)
	require.Error(t, theirsLossErr)
	require.Contains(t, theirsLossErr.Error(), "does not preserve THEIRS replacement intent")
	require.ErrorIs(t, theirsLossErr, errMergeConflictReplacementIntentProofUnavailable)

	baseOnlyErr := validateMergeConflictRegionResponse("ISSUES.md", 1, nonAdditiveRegion, nonAdditiveRegion.Base)
	require.Error(t, baseOnlyErr)
	require.Contains(t, baseOnlyErr.Error(), "returned BASE without either side's changes")
	require.NotErrorIs(t, baseOnlyErr, errMergeConflictReplacementIntentProofUnavailable)
}

func TestValidateMergeConflictRegionResponseIgnoresWhitespaceAndReportsAllMissingIntent(t *testing.T) {
	reflowedRegion := mergeConflictRegion{
		Ours:        "policy: strict\n  timeout=30\n",
		Base:        "policy: standard timeout=30\n",
		BasePresent: true,
		Theirs:      "policy: standard timeout=60\n",
	}
	require.NoError(
		t,
		validateMergeConflictRegionResponse(
			"policy.txt",
			0,
			reflowedRegion,
			"policy: strict timeout=60\n",
		),
	)

	multipleLossRegion := mergeConflictRegion{
		Ours:        "owners: carol; reviewers: dave; timeout=30\n",
		Base:        "owners: alice; reviewers: bob; timeout=30\n",
		BasePresent: true,
		Theirs:      "owners: alice; reviewers: bob; timeout=60; mode=strict\n",
	}
	multipleLossErr := validateMergeConflictRegionResponse(
		"policy.txt",
		1,
		multipleLossRegion,
		"owners: alice; reviewers: bob; timeout=45\n",
	)
	require.Error(t, multipleLossErr)
	require.Contains(t, multipleLossErr.Error(), "does not preserve OURS replacement intent")
	require.Contains(t, multipleLossErr.Error(), "carol")
	require.Contains(t, multipleLossErr.Error(), "dave")
	require.Contains(t, multipleLossErr.Error(), "does not preserve THEIRS replacement intent")
	require.Contains(t, multipleLossErr.Error(), "60")
	require.Contains(t, multipleLossErr.Error(), "mode=strict")
}

func TestValidateMergeConflictRegionResponseRejectsReplacementIntentFromUnrelatedOccurrence(t *testing.T) {
	region := mergeConflictRegion{
		Ours:        "primary: foo bar\nalias: foobar\nmode: standard\n",
		Base:        "primary: old\nalias: foobar\nmode: standard\n",
		BasePresent: true,
		Theirs:      "primary: old\nalias: foobar\nmode: strict\n",
	}

	lossyCandidateErr := validateMergeConflictRegionResponse(
		"policy.txt",
		0,
		region,
		"primary: old\nalias: foobar\nmode: strict\n",
	)
	require.Error(t, lossyCandidateErr)
	require.Contains(t, lossyCandidateErr.Error(), "does not preserve OURS replacement intent")
	require.Contains(t, lossyCandidateErr.Error(), "foo bar")

	require.NoError(
		t,
		validateMergeConflictRegionResponse(
			"policy.txt",
			0,
			region,
			"primary: foo\n  bar\nalias: foobar\nmode: strict\n",
		),
	)
	require.NoError(
		t,
		validateMergeConflictRegionResponse(
			"policy.txt",
			0,
			region,
			"primary: foo bar\nmode: strict\n",
		),
	)
}

func TestValidateMergeConflictRegionResponseAllowsReplacementAlternativeAndRequiresCompatibleIntent(t *testing.T) {
	region := mergeConflictRegion{
		Ours:        "contract: SchemaV5 mode=standard\n",
		Base:        "contract: SchemaV4 mode=standard\n",
		BasePresent: true,
		Theirs:      "contract: Versionless mode=strict\n",
	}

	require.NoError(
		t,
		validateMergeConflictRegionResponse(
			"lifecycle_contract_test.go",
			0,
			region,
			region.Theirs,
		),
	)

	missingCompatibleIntentErr := validateMergeConflictRegionResponse(
		"lifecycle_contract_test.go",
		0,
		region,
		"contract: Versionless mode=standard\n",
	)
	require.Error(t, missingCompatibleIntentErr)
	require.Contains(t, missingCompatibleIntentErr.Error(), "does not preserve THEIRS replacement intent")
	require.Contains(t, missingCompatibleIntentErr.Error(), "strict")

	missingAlternativeErr := validateMergeConflictRegionResponse(
		"lifecycle_contract_test.go",
		0,
		region,
		"contract: Unified mode=strict\n",
	)
	require.Error(t, missingAlternativeErr)
	require.Contains(t, missingAlternativeErr.Error(), "does not preserve OURS replacement intent")
	require.Contains(t, missingAlternativeErr.Error(), "SchemaV5")
	require.Contains(t, missingAlternativeErr.Error(), "does not preserve THEIRS replacement intent")
	require.Contains(t, missingAlternativeErr.Error(), "Versionless")
}

func TestValidateMergeConflictRegionResponseIntegratesCompatibleWordingInsideReportedRegion(t *testing.T) {
	base := "- `[ ]` means open.\n- `[-]` means taken.\n- `[!]` means blocked and must include a `Blocked:` body line.\n- `[x]` means closed.\n- The external ID is required.\n- Priority `(P0)` through `(P2)` is optional.\n- Dependencies `{ID,ID}` are optional.\n- The title is required.\n"
	ours := "- `[ ]` means open (unresolved), `[-]` means taken (actively being worked, but still unresolved), `[!]` means blocked (unresolved), `[x]` means closed (resolved).\n- The external ID is required.\n- Priority and dependencies are optional and appear immediately after the ID.\n- The title is required.\n- Blocked issues (`[!]`) MUST include a short explanation in the body (at minimum one indented line starting with `Blocked:`).\n"
	theirs := "- `[ ]` means open.\n- `[-]` means taken.\n- `[!]` means blocked and must include a `Blocked:` body line.\n- `[x]` means closed.\n- The external ID is necessary.\n- Priority `(P0)` through `(P2)` is optional.\n- Dependencies `{ID,ID}` are optional.\n- The title is necessary.\n- Write each new or changed title in ASD-STE100 Simplified Technical English.\n"
	candidate := "- `[ ]` means open (unresolved), `[-]` means taken (actively being worked, but still unresolved), `[!]` means blocked (unresolved), `[x]` means closed (resolved).\n- The external ID is necessary.\n- Priority and dependencies are optional and appear immediately after the ID.\n- The title is necessary.\n- Blocked issues (`[!]`) MUST include a short explanation in the body (at minimum one indented line starting with `Blocked:`).\n- Write each new or changed title in ASD-STE100 Simplified Technical English.\n"
	require.NoError(t, validateMergeConflictRegionResponse(
		".mprlab/issues-md-format.md",
		1,
		mergeConflictRegion{Base: base, BasePresent: true, Ours: ours, Theirs: theirs},
		candidate,
	))
}

func TestResolveSemanticConflictRegionUsesFirstValidAuditedResult(t *testing.T) {
	region := mergeConflictRegion{
		Ours:        "contract: SchemaV5\n",
		Base:        "contract: SchemaV4\n",
		BasePresent: true,
		Theirs:      "contract: Versionless\n",
	}
	semanticResponse := func(content string) string {
		return mergeConflictResolutionContentBegin + "\n" + content + mergeConflictResolutionContentEnd
	}
	testCases := map[string]struct {
		initialCandidate string
		candidateExists  bool
		responses        []string
		expectedResult   string
		expectedRequests int
		expectedError    string
	}{
		"approve derived candidate": {
			initialCandidate: region.Ours,
			candidateExists:  true,
			responses:        []string{mergeConflictResolutionReviewApproved},
			expectedResult:   region.Ours,
			expectedRequests: 1,
		},
		"correct derived candidate": {
			initialCandidate: region.Ours,
			candidateExists:  true,
			responses:        []string{semanticResponse(region.Theirs)},
			expectedResult:   strings.TrimSpace(region.Theirs),
			expectedRequests: 1,
		},
		"repair invalid audit response": {
			initialCandidate: region.Ours,
			candidateExists:  true,
			responses: []string{
				semanticResponse("unrelated contract\n"),
				semanticResponse(region.Theirs),
			},
			expectedResult:   strings.TrimSpace(region.Theirs),
			expectedRequests: 2,
		},
		"approve generated candidate": {
			responses: []string{
				semanticResponse(region.Ours),
				mergeConflictResolutionReviewApproved,
			},
			expectedResult:   strings.TrimSpace(region.Ours),
			expectedRequests: 2,
		},
		"correct generated candidate": {
			responses: []string{
				semanticResponse(region.Ours),
				semanticResponse(region.Theirs),
			},
			expectedResult:   strings.TrimSpace(region.Theirs),
			expectedRequests: 2,
		},
		"exhaust invalid audit responses": {
			initialCandidate: region.Ours,
			candidateExists:  true,
			responses: []string{
				semanticResponse("unrelated contract\n"),
				semanticResponse("unrelated contract\n"),
				semanticResponse("unrelated contract\n"),
				semanticResponse("unrelated contract\n"),
			},
			expectedRequests: mergeConflictResolutionMaxSemanticAttempts,
			expectedError:    "exhausted 4 semantic attempts",
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			client := &strictSyncChatClient{responses: testCase.responses}
			service := mergeConflictResolutionService{
				repositoryPath: "/repo",
				commitMessages: worktreeAdoptionCommitMessageOptions{
					Client: client,
				},
			}

			resolution, resolutionErr := service.resolveSemanticConflictRegion(
				context.Background(),
				client,
				mergeConflictResolutionOptions{
					SourceReference: "origin/master",
					TargetBranch:    "feature/current-contract",
				},
				mergeConflictFile{Path: "contract.txt"},
				region,
				0,
				1,
				time.Second,
				testCase.initialCandidate,
				testCase.candidateExists,
			)
			if testCase.expectedError != "" {
				require.ErrorContains(t, resolutionErr, testCase.expectedError)
			} else {
				require.NoError(t, resolutionErr)
				require.Equal(t, testCase.expectedResult, resolution)
			}
			require.Len(t, client.requests, testCase.expectedRequests)
		})
	}
}

func TestResolveSemanticConflictRegionAuditsTheExactUnprovenCorrection(t *testing.T) {
	region := mergeConflictRegion{
		Base:        "`Blocked:` is required only for blocked issues and must name the external dependency, missing input, or policy decision preventing progress.\n",
		BasePresent: true,
		Ours: reportedConflictRegionLines(
			"  Blocked: waiting on upstream API credentials.",
			"  ```bash",
			"  timeout -k 30s -s SIGKILL 30s make test",
			"  ```",
			"```",
		),
		Theirs: reportedConflictRegionLines(
			"`Blocked:` is necessary only for blocked issues. It must identify the dependency, input, or policy decision that prevents progress.",
			"",
			"Write each new or changed body in ASD-STE100. Use `.mprlab/AGENTS.DOCS.md` and `.mprlab/TERMINOLOGY.md`.",
		),
	}
	semanticCorrection := region.Ours + "\n" + reportedConflictRegionLines(
		"`Blocked:` is required only for blocked issues. It must name the dependency, input, or policy decision that stops progress.",
		"",
		"Write each new or changed body in ASD-STE100. Use `.mprlab/AGENTS.DOCS.md` and `.mprlab/TERMINOLOGY.md`.",
	)
	locallyValidCorrection := region.Ours + "\n" + region.Theirs
	initialResolution, resolved := deterministicMergeConflictRegionResolution(region)
	require.True(t, resolved)
	require.True(t, initialResolution.RequiresSemanticAudit)
	require.NoError(t, validateMergeConflictRegionResponse(".mprlab/issues-md-format.md", 3, region, initialResolution.Content))
	proofErr := validateMergeConflictRegionResponse(".mprlab/issues-md-format.md", 3, region, semanticCorrection)
	require.Error(t, proofErr)
	require.ErrorIs(t, proofErr, errMergeConflictReplacementIntentProofUnavailable)

	client := &strictSyncChatClient{responses: []string{
		mergeConflictResolutionContentBegin + "\n" + semanticCorrection + mergeConflictResolutionContentEnd,
		mergeConflictResolutionReviewApproved,
		mergeConflictResolutionContentBegin + "\n" + locallyValidCorrection + mergeConflictResolutionContentEnd,
	}}
	service := mergeConflictResolutionService{
		repositoryPath: "/repo",
		commitMessages: worktreeAdoptionCommitMessageOptions{Client: client},
	}

	resolution, resolutionErr := service.resolveSemanticConflictRegion(
		context.Background(),
		client,
		mergeConflictResolutionOptions{SourceReference: "origin/master", TargetBranch: "master"},
		mergeConflictFile{Path: ".mprlab/issues-md-format.md"},
		region,
		3,
		4,
		time.Second,
		initialResolution.Content,
		true,
	)

	require.NoError(t, resolutionErr)
	require.Equal(t, strings.TrimSuffix(locallyValidCorrection, "\n"), resolution)
	require.Len(t, client.requests, 3)
	followupPrompt := client.requests[1].Messages[1].Content
	require.Contains(t, followupPrompt, "SEMANTIC CORRECTION CANDIDATE:\n"+strings.TrimSuffix(semanticCorrection, "\n"))
	require.Contains(t, followupPrompt, "deterministic replacement-intent proof is unavailable")
	repairPrompt := client.requests[2].Messages[1].Content
	require.Contains(t, repairPrompt, "SEMANTIC CORRECTION CANDIDATE:\n"+strings.TrimSuffix(semanticCorrection, "\n"))
	require.Contains(t, repairPrompt, "cannot accept a candidate without deterministic replacement-intent proof")
}

func TestResolveSemanticConflictRegionRejectsEmptyProviderResponse(t *testing.T) {
	client := &strictSyncChatClient{response: "   "}
	service := mergeConflictResolutionService{
		repositoryPath: "/repo",
		commitMessages: worktreeAdoptionCommitMessageOptions{
			Client: client,
		},
	}

	_, resolutionErr := service.resolveSemanticConflictRegion(
		context.Background(),
		client,
		mergeConflictResolutionOptions{SourceReference: "origin/master", TargetBranch: "master"},
		mergeConflictFile{Path: "policy.txt"},
		mergeConflictRegion{Base: "base\n", BasePresent: true, Ours: "ours\n", Theirs: "theirs\n"},
		0,
		1,
		time.Second,
		"",
		false,
	)
	require.Error(t, resolutionErr)
	require.Contains(t, resolutionErr.Error(), "llm returned an empty merge resolution")
	require.Len(t, client.requests, 1)
}

func TestDeterministicMergeConflictRegionResolutionBuildsAuthoritativeResultsAndAuditedCandidates(t *testing.T) {
	testCases := map[string]struct {
		region                mergeConflictRegion
		expectedContent       string
		expectedStrategy      string
		requiresSemanticAudit bool
	}{
		"identical sides": {
			region: mergeConflictRegion{
				Ours:        "same\n",
				Base:        "base\n",
				BasePresent: true,
				Theirs:      "same\n",
			},
			expectedContent:  "same\n",
			expectedStrategy: "identical sides",
		},
		"ours unchanged": {
			region: mergeConflictRegion{
				Ours:        "base\n",
				Base:        "base\n",
				BasePresent: true,
				Theirs:      "incoming\n",
			},
			expectedContent:  "incoming\n",
			expectedStrategy: "incoming-only change",
		},
		"theirs unchanged": {
			region: mergeConflictRegion{
				Ours:        "local\n",
				Base:        "base\n",
				BasePresent: true,
				Theirs:      "base\n",
			},
			expectedContent:  "local\n",
			expectedStrategy: "local-only change",
		},
		"concurrent insertions require audit": {
			region: mergeConflictRegion{
				Ours:        "local insertion\n",
				BasePresent: true,
				Theirs:      "incoming insertion\n",
			},
			expectedContent:       "local insertion\nincoming insertion\n",
			expectedStrategy:      "concurrent insertions",
			requiresSemanticAudit: true,
		},
		"compatible token edits": {
			region: mergeConflictRegion{
				Ours:        "policy: strict timeout=30\n",
				Base:        "policy: standard timeout=30\n",
				BasePresent: true,
				Theirs:      "policy: standard timeout=60\n",
			},
			expectedContent:       "policy: strict timeout=60\n",
			expectedStrategy:      "compatible token edits",
			requiresSemanticAudit: true,
		},
	}

	for testName, testCase := range testCases {
		t.Run(testName, func(t *testing.T) {
			resolution, resolved := deterministicMergeConflictRegionResolution(testCase.region)

			require.True(t, resolved)
			require.Equal(t, testCase.expectedContent, resolution.Content)
			require.Equal(t, testCase.expectedStrategy, resolution.Strategy)
			require.Equal(t, testCase.requiresSemanticAudit, resolution.RequiresSemanticAudit)
		})
	}
}

func TestDeterministicMergeConflictRegionResolutionDerivesAuditableReplacementAlternatives(t *testing.T) {
	testCases := []struct {
		name   string
		region mergeConflictRegion
	}{
		{
			name: "conflicting replacement with compatible edit",
			region: mergeConflictRegion{
				Base:        "contract: SchemaV4\nmode: standard\n",
				BasePresent: true,
				Ours:        "contract: SchemaV5\nmode: standard\n",
				Theirs:      "contract: Versionless\nmode: strict\n",
			},
		},
		{
			name: "compatible edits inside one coarse conflict region",
			region: mergeConflictRegion{
				Base: reportedConflictRegionLines(
					"- `[ ]` means open.",
					"- `[-]` means taken.",
					"- `[!]` means blocked and must include a `Blocked:` body line.",
					"- `[x]` means closed.",
					"- The external ID is required.",
					"- Priority `(P0)` through `(P2)` is optional.",
					"- Dependencies `{ID,ID}` are optional.",
					"- The title is required.",
				),
				BasePresent: true,
				Ours: reportedConflictRegionLines(
					"- `[ ]` means open.",
					"- `[-]` means taken.",
					"- `[!]` means blocked and must include a `Blocked:` body line.",
					"- `[x]` means closed.",
					"- The external ID is necessary.",
					"- Priority `(P0)` through `(P2)` is optional.",
					"- Dependencies `{ID,ID}` are optional.",
					"- The title is necessary.",
					"- Write each changed title in Simplified Technical English.",
				),
				Theirs: reportedConflictRegionLines(
					"- `[ ]` means open (unresolved), `[-]` means taken (active), `[!]` means blocked (unresolved), `[x]` means closed (resolved).",
					"- The external ID is required.",
					"- Priority and dependencies are optional and follow the ID.",
					"- The title is required.",
					"- Blocked issues must include a `Blocked:` body line.",
				),
			},
		},
		{
			name: "deletion is one replacement alternative",
			region: mergeConflictRegion{
				Base:        "assert schema version 4\n",
				BasePresent: true,
				Ours:        "assert schema version 5\n",
				Theirs:      "",
			},
		},
		{
			name: "overlapping insertions are replacement alternatives",
			region: mergeConflictRegion{
				Base:        "reviewers: alice\n",
				BasePresent: true,
				Ours:        "reviewers: alice, bob\n",
				Theirs:      "reviewers: alice, carol\n",
			},
		},
		{
			name: "whole replacement with adjacent insertion",
			region: mergeConflictRegion{
				Base:        "`Blocked:` is required only for blocked issues and must name the external dependency, missing input, or policy decision preventing progress.\n",
				BasePresent: true,
				Ours: reportedConflictRegionLines(
					"  Blocked: waiting on upstream API credentials.",
					"  ```bash",
					"  timeout -k 30s -s SIGKILL 30s make test",
					"  ```",
					"```",
				),
				Theirs: reportedConflictRegionLines(
					"`Blocked:` is necessary only for blocked issues. It must identify the dependency, input, or policy decision that prevents progress.",
					"",
					"Write each changed body in Simplified Technical English.",
				),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			resolution, resolved := deterministicMergeConflictRegionResolution(testCase.region)

			require.True(t, resolved)
			require.True(t, resolution.RequiresSemanticAudit)
			require.NotEmpty(t, resolution.Strategy)
			require.NoError(
				t,
				validateMergeConflictRegionResponse("contract.txt", 0, testCase.region, resolution.Content),
				"candidate: %q",
				resolution.Content,
			)
		})
	}
}

func reportedConflictRegionLines(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

func TestDeterministicMergeConflictRegionResolutionPreservesInsertionInsideWhitespaceSeparatedEdits(t *testing.T) {
	region := mergeConflictRegion{
		Base:        "foo bar\n",
		BasePresent: true,
		Ours:        "FOO BAR\n",
		Theirs:      "foo, bar\n",
	}

	resolution, resolved := deterministicMergeConflictRegionResolution(region)

	require.True(t, resolved)
	require.True(t, resolution.RequiresSemanticAudit)
	require.Equal(t, "FOO, BAR\n", resolution.Content)
	require.NoError(t, validateMergeConflictRegionResponse("contract.txt", 0, region, resolution.Content))

	lossyCandidateErr := validateMergeConflictRegionResponse("contract.txt", 0, region, region.Ours)
	require.Error(t, lossyCandidateErr)
	require.Contains(t, lossyCandidateErr.Error(), "does not preserve THEIRS replacement intent")
	require.Contains(t, lossyCandidateErr.Error(), ",")
}

func TestDeterministicMergeConflictRegionResolutionOrdersSamePositionInsertionsIndependentlyOfSides(t *testing.T) {
	forwardRegion := mergeConflictRegion{
		Base:        "reviewers: alice\n",
		BasePresent: true,
		Ours:        "reviewers: alice, carol\n",
		Theirs:      "reviewers: alice, bob\n",
	}
	reverseRegion := mergeConflictRegion{
		Base:        forwardRegion.Base,
		BasePresent: true,
		Ours:        forwardRegion.Theirs,
		Theirs:      forwardRegion.Ours,
	}

	forward, forwardResolved := deterministicMergeConflictRegionResolution(forwardRegion)
	reverse, reverseResolved := deterministicMergeConflictRegionResolution(reverseRegion)

	require.True(t, forwardResolved)
	require.True(t, reverseResolved)
	require.Equal(t, "reviewers: alice, bob, carol\n", forward.Content)
	require.Equal(t, forward.Content, reverse.Content)
}

func TestResolveRejectsCachedDiffCheckBeforeMergeCommit(t *testing.T) {
	repositoryPath := t.TempDir()
	issuesDirectory := filepath.Join(repositoryPath, ".mprlab")
	require.NoError(t, os.MkdirAll(issuesDirectory, 0o755))
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(issuesDirectory, "ISSUES.md"),
			[]byte(
				"stable prefix\n"+
					"<<<<<<< HEAD\n"+
					"local insertion\n"+
					"||||||| parent\n"+
					"=======\n"+
					"incoming insertion\n"+
					">>>>>>> origin/master\n"+
					"stable suffix\n",
			),
			0o644,
		),
	)

	executor := &mergeConflictIndexCheckExecutor{}
	service := mergeConflictResolutionService{
		executor:       executor,
		repositoryPath: repositoryPath,
		commitMessages: worktreeAdoptionCommitMessageOptions{
			Client: &strictSyncChatClient{response: mergeConflictResolutionReviewApproved},
		},
	}

	conflictObserved, resolutionErr := service.Resolve(
		context.Background(),
		mergeConflictResolutionOptions{
			SourceReference: "origin/master",
			TargetBranch:    "feature/target",
		},
	)

	require.True(t, conflictObserved)
	require.Error(t, resolutionErr)
	require.Contains(t, resolutionErr.Error(), "validate resolved merge index")
	require.Contains(t, resolutionErr.Error(), "trailing whitespace")
	recordedCommands := make([]string, 0, len(executor.commands))
	for _, command := range executor.commands {
		recordedCommands = append(recordedCommands, strings.Join(command.Arguments, " "))
	}
	require.Contains(t, recordedCommands, "diff --cached --check")
	require.Contains(t, recordedCommands, "rev-parse --verify --end-of-options MERGE_HEAD^{commit}")
	require.Contains(t, recordedCommands, "diff --cached --check dddddddddddddddddddddddddddddddddddddddd -- .mprlab/ISSUES.md")
	require.NotContains(t, recordedCommands, "diff --cached --check origin/master -- .mprlab/ISSUES.md")
	require.NotContains(t, recordedCommands, "commit --no-edit")
}
